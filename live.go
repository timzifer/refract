package refract

import (
	"errors"
	"fmt"

	"github.com/timzifer/refract/interact"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/scale"
)

// The interaction vocabulary, re-exported so that a chart that hovers needs
// one import like a chart that does not. See package interact.
type (
	// Event is one thing that happened to a chart. See [interact.Event].
	Event = interact.Event
	// EventKind is what happened. See [interact.EventKind].
	EventKind = interact.EventKind
	// Hit is the mark under a pointer. See [interact.Hit].
	Hit = interact.Hit
)

// The event kinds. See [interact.EventKind].
const (
	Hover = interact.Hover
	Leave = interact.Leave
	Click = interact.Click
	Zoom  = interact.Zoom
	Pan   = interact.Pan
)

// On registers a handler for an event kind.
//
//	p.On(refract.Hover, func(ev refract.Event) {
//	    if ev.Found {
//	        tooltip(ev.Series(), ev.Hit.X, ev.Hit.Y)
//	    }
//	})
//	p.On(refract.Zoom, func(ev refract.Event) { log.Println(ev.Rect) })
//
// Handlers fire in registration order, from [Live]'s input methods, on the
// goroutine that called one. Registering a handler does not by itself make a
// chart interactive — [Plot.Live] is what draws one into a surface that can
// report input.
func (p *Plot) On(kind EventKind, h func(Event)) *Plot {
	if h == nil {
		return p
	}
	if p.handlers == nil {
		p.handlers = map[EventKind][]func(Event){}
	}
	p.handlers[kind] = append(p.handlers[kind], h)
	return p
}

func (p *Plot) emit(ev Event) {
	for _, h := range p.handlers[ev.Kind] {
		h(ev)
	}
}

// Live is a chart drawn into a surface that can be redrawn, pointed at, panned
// and zoomed.
//
// It is the interactive half of a plot: [Plot.Render] draws once into a file,
// Live draws over and over into something that stays open — a browser canvas,
// a window, a test.
//
//	live, err := p.Live(canvas.Element(el))
//	defer live.Close()
//	live.Draw()
//
//	// from the surface's event loop
//	live.Move(x, y)
//	live.Wheel(x, y, dy)
//
// # What a redraw costs
//
// Each Draw records the frame, compares it with the last one and repaints only
// where the two differ — see [ir.Damage]. A backend that cannot repaint part
// of a frame gets the whole one, and a frame that is identical to the last is
// not painted at all.
//
// # The chart is built once
//
// Live resolves the plot into panels when it is created, so that a zoom lands
// on scales that are still there next frame. Adding a layer, changing the
// facet or replacing a scale afterwards needs [Live.Rebuild], which starts
// again from the plot as it now stands — and, like any fresh start, forgets
// where the view was zoomed to.
//
// A Live is not safe for concurrent use.
type Live struct {
	p *Plot
	t Target
	b ir.Backend

	// width, height and dpr describe the surface, which starts as the plot
	// says and changes when the surface does — see [Live.Resize] and
	// [Live.Rescale]. They live here rather than on the Plot because a window
	// being dragged onto another display is not an edit to the chart
	// specification.
	width, height int
	dpr           float64

	chart render.Chart
	idx   *interact.Index

	prev, next *ir.Recorder
	rects      []ir.Rect
	drawn      bool

	last  ir.Point
	over  bool
	panel int
}

// Live opens t and returns a chart drawn into it.
//
// The target stays open until [Live.Close], which is what makes this different
// from [Plot.Render]: a Live draws frame after frame into one surface.
//
// It wants a surface rather than a document. The SVG and PDF emitters build a
// document and write it whole, so drawing many frames into one collects every
// frame in the same file rather than replacing what was there — use
// [Plot.Render] for those, and a Live for a canvas, a window, or anything else
// that is repainted. A single Draw into a document target is exactly a Render,
// and is a reasonable way to export what an interactive chart currently shows.
func (p *Plot) Live(t Target) (*Live, error) {
	if t == nil {
		return nil, errors.New("refract: nil render target")
	}
	if len(p.layers) == 0 && p.x == nil && p.y == nil {
		return nil, ErrNoLayers
	}
	b, err := t.Open(p.width, p.height, p.dpr)
	if err != nil {
		return nil, err
	}
	l := &Live{
		p: p, t: t, b: b,
		width: p.width, height: p.height, dpr: p.dpr,
		idx: interact.New(), panel: -1,
	}
	if l.chart, err = p.chart(); err != nil {
		t.Close()
		return nil, err
	}
	l.chart.Observer = l.idx
	l.prev = ir.NewRecorder(b)
	l.next = ir.NewRecorder(b)
	return l, nil
}

// TrackRows turns row identity on or off and returns l, so the call can be
// chained onto [Plot.Live].
//
//	live, err := p.Live(canvas.Element(el))
//	live.TrackRows(true)
//	// ...
//	p.On(refract.Hover, func(ev refract.Event) {
//	    if ev.Found && ev.Hit.Row >= 0 {
//	        highlightTableRow(ev.Hit.Row)
//	    }
//	})
//
// It is off by default because it is not free. With it on, every layer that
// can report its rows records where each one landed, and the hit index keeps a
// position and a row number per mark on top of the marks it already keeps —
// memory proportional to the marks on screen, which after decimation is
// thousands rather than millions, but not nothing. Without it, [Hit.Row] is
// -1 and a hit still reports the data values under the pointer.
//
// It takes effect on the next [Live.Draw].
//
// Not every mark has a row to report. A boxplot's box aggregates many rows, a
// density raster is not a mark at all, an interpolated point across a gap was
// never measured, and a third-party geom that does not report its rows has
// none to report; all of those leave [Hit.Row] at -1 rather than guessing a
// nearby one.
func (l *Live) TrackRows(on bool) *Live {
	l.idx.TrackRows(on)
	l.chart.RowSink = nil
	if on {
		l.chart.RowSink = l.idx
	}
	l.drawn = false // the next frame is not comparable with the last
	return l
}

// Resize tells the chart its surface has changed size, and redraws it.
//
// It is what a window's resize event and a reflowed canvas element call. The
// scales keep whatever they were zoomed or panned to — a reader who has dragged
// a view into place has not asked to leave it — and the chart is laid out
// again at the new size, so the margins, the tick count and the legend follow.
// A [Responsive] plot also rescales its type and stroke weights here.
//
// The backend is told too, if it can be: a surface that implements
// [ir.Resizer] is resized in place rather than reopened, which is what keeps
// the frame on screen and the zoom in the scales. One that cannot is redrawn
// at the new logical size into the surface it has, which is the best available
// answer and is what a document target would do.
//
// Resizing to the size it already has is not an error and draws nothing.
func (l *Live) Resize(w, h int) error {
	if l.b == nil {
		return errors.New("refract: Resize on a closed Live")
	}
	if w <= 0 || h <= 0 {
		return fmt.Errorf("refract: chart size %dx%d is not positive", w, h)
	}
	if w == l.width && h == l.height {
		return nil
	}
	l.width, l.height = w, h
	l.chart.Width, l.chart.Height = w, h
	l.chart.Theme = l.p.themeFor(w, h)
	if r, ok := l.b.(ir.Resizer); ok {
		if err := r.Resize(w, h, l.dpr); err != nil {
			return err
		}
	}
	// A frame of a different size is not comparable with the last one: every
	// coordinate in it moved, so there is no damage to compute.
	l.drawn = false
	return l.Draw()
}

// Size reports the surface's current size in device-independent pixels.
func (l *Live) Size() (w, h int) { return l.width, l.height }

// Rescale tells the chart its surface's device pixel ratio has changed, and
// redraws it.
//
// It is what a window dragged onto a display with a different one calls. The
// chart is not laid out differently — a device pixel ratio is not a size, and
// coordinates stay in device-independent units either way — but the surface
// behind it wants more pixels, and a backend that can provide them is told to.
// A backend that cannot is left alone and the frame is redrawn as it was.
//
// Rescaling to the ratio it already has is not an error and draws nothing.
func (l *Live) Rescale(dpr float64) error {
	if l.b == nil {
		return errors.New("refract: Rescale on a closed Live")
	}
	if dpr <= 0 {
		return fmt.Errorf("refract: device pixel ratio %v is not positive", dpr)
	}
	if dpr == l.dpr {
		return nil
	}
	l.dpr = dpr
	r, ok := l.b.(ir.Resizer)
	if !ok {
		return nil
	}
	if err := r.Resize(l.width, l.height, dpr); err != nil {
		return err
	}
	// The pixels behind the frame are gone: the surface reallocated them.
	l.drawn = false
	return l.Draw()
}

// DPR reports the surface's current device pixel ratio.
func (l *Live) DPR() float64 { return l.dpr }

// Rebuild resolves the plot again, picking up layers, scales or a facet added
// since the Live was created. It forgets any zoom on a facet's free axes,
// which belong to panels that no longer exist.
func (l *Live) Rebuild() error {
	c, err := l.p.chart()
	if err != nil {
		return err
	}
	c.Observer = l.idx
	if l.idx.TrackingRows() {
		c.RowSink = l.idx
	}
	// A Live that has been resized keeps its size across a rebuild: the plot
	// still says what it was built with, and the surface is the size it is.
	c.Width, c.Height = l.width, l.height
	c.Theme = l.p.themeFor(l.width, l.height)
	l.chart = c
	l.drawn = false
	return nil
}

// Draw renders the current state of the plot.
//
// It returns nil having painted nothing when the frame is identical to the
// last one, which is the common case for a pointer moving over a chart that is
// not being zoomed.
func (l *Live) Draw() error {
	if l.b == nil {
		return errors.New("refract: Draw on a closed Live")
	}
	l.idx.Reset()
	l.next.Reset()
	if err := render.Draw(l.idx.Watch(l.next), l.chart); err != nil {
		return err
	}

	// sameShape rather than "comparable": the word is taken, and what is being
	// asked is whether the two frames are the same chart with different
	// numbers in it.
	rects, sameShape := ir.Damage(l.prev, l.next, l.rects)
	l.rects = rects
	if l.drawn && sameShape && len(rects) == 0 {
		return nil
	}
	if partial, ok := l.b.(ir.Partial); ok {
		if l.drawn && sameShape {
			partial.Damage(rects)
		} else {
			partial.Damage(nil)
		}
	}
	l.next.Replay(l.b)
	if err := l.b.Flush(); err != nil {
		return err
	}
	l.prev, l.next = l.next, l.prev
	l.drawn = true
	return nil
}

// Close finalises the target. The last frame drawn is what it holds.
func (l *Live) Close() error {
	if l.b == nil {
		return nil
	}
	l.b = nil
	return l.t.Close()
}

// Index returns the hit index of the last frame, for a caller drawing its own
// tooltip or crosshair.
func (l *Live) Index() *interact.Index { return l.idx }

// Move reports the pointer at a device position and fires [Hover].
//
// The event carries the mark under the pointer, if there is one within
// [interact.DefaultTolerance], and the panel the pointer is in, or -1 for a
// point in the margins.
//
// The one move that fires something else is the move that leaves the last
// panel: that fires [Leave] instead, once, so that a tooltip opened on a hover
// has a matching event to close on. Moving around in the margins after that
// goes on firing Hover with no hit.
func (l *Live) Move(x, y float64) Event {
	pt := ir.Point{X: float32(x), Y: float32(y)}
	l.last = pt
	panel, in := l.idx.PanelAt(pt)
	if !in {
		l.panel = -1
		if l.over {
			l.over = false
			return l.fire(Event{Kind: Leave, Point: pt, Panel: -1})
		}
		return l.fire(Event{Kind: Hover, Point: pt, Panel: -1})
	}
	l.over, l.panel = true, panel
	ev := Event{Kind: Hover, Point: pt, Panel: panel}
	ev.Hit, ev.Found = l.idx.At(pt, 0)
	return l.fire(ev)
}

// Leave reports the pointer leaving the surface and fires [Leave].
func (l *Live) Leave() Event {
	l.over, l.panel = false, -1
	return l.fire(Event{Kind: Leave, Point: l.last, Panel: -1})
}

// Click reports a click at a device position and fires [Click].
func (l *Live) Click(x, y float64) Event {
	pt := ir.Point{X: float32(x), Y: float32(y)}
	l.last = pt
	ev := Event{Kind: Click, Point: pt, Panel: -1}
	if panel, in := l.idx.PanelAt(pt); in {
		ev.Panel = panel
	}
	ev.Hit, ev.Found = l.idx.At(pt, 0)
	return l.fire(ev)
}

// Wheel zooms about a device position by factor, and redraws.
//
// factor below 1 zooms in and above 1 zooms out: it multiplies the width of
// the view, so 0.8 shows four fifths of what was there. A wheel notch is
// usually turned into 0.9 or 1.1 by the surface.
//
// Both axes zoom, about the pointer, so that the value under the cursor stays
// under the cursor. A scale that cannot be zoomed — an ordinal axis, where
// half a category is not a view of anything — is left alone, and a wheel over
// a chart with two such axes does nothing at all.
func (l *Live) Wheel(x, y, factor float64) error {
	if factor <= 0 {
		return fmt.Errorf("refract: zoom factor %v is not positive", factor)
	}
	pt := ir.Point{X: float32(x), Y: float32(y)}
	p, ok := l.panelAt(pt)
	if !ok {
		return nil
	}
	// The interval each scale maps into is the coord's answer, and so is where
	// the pointer sits within it: under a Cartesian coord those are the
	// rectangle's edges and the pointer's own coordinates, and under a polar
	// one they are an angle range, a radius range, and the angle and radius
	// the pointer landed at. Reading the rectangle here instead would scale a
	// polar chart's domains by numbers that mean nothing on its axes.
	cd := p.Coords()
	x0, x1, y0, y1 := cd.Extent()
	mx, my := cd.Invert(pt)
	zoomAxis(p.X, x0, x1, mx, factor)
	zoomAxis(p.Y, y0, y1, my, factor)
	l.fire(Event{Kind: Zoom, Point: pt, Panel: l.panelIndex(pt), Factor: factor})
	return l.Draw()
}

// ZoomTo zooms into a device-space rectangle — a rubber-band selection — and
// redraws.
func (l *Live) ZoomTo(r ir.Rect) error {
	if r.Empty() {
		return nil
	}
	mid := ir.Point{X: (r.Min.X + r.Max.X) / 2, Y: (r.Min.Y + r.Max.Y) / 2}
	p, ok := l.panelAt(mid)
	if !ok {
		return nil
	}
	cd := p.Coords()
	ax, ay := cd.Invert(r.Min)
	bx, by := cd.Invert(r.Max)
	setDomain(p.X, ax, bx)
	setDomain(p.Y, ay, by)
	l.fire(Event{Kind: Zoom, Point: mid, Panel: l.panelIndex(mid), Rect: r})
	return l.Draw()
}

// PanBy moves the view by a device-space delta and redraws. It is what a drag
// does: the data follows the pointer, so dragging right shows earlier data.
func (l *Live) PanBy(dx, dy float64) error {
	p, ok := l.panelAt(l.last)
	if !ok {
		return nil
	}
	// The drag is a device delta and the scales speak in the interval the
	// coord gave them, so the two ends of the drag are inverted and the
	// difference between them is what moves the domain. Under a Cartesian
	// coord that difference is dx and dy exactly.
	cd := p.Coords()
	x0, x1, y0, y1 := cd.Extent()
	fromX, fromY := cd.Invert(l.last)
	toX, toY := cd.Invert(ir.Point{X: l.last.X + float32(dx), Y: l.last.Y + float32(dy)})
	panAxis(p.X, x0, x1, toX-fromX)
	panAxis(p.Y, y0, y1, toY-fromY)
	l.fire(Event{
		Kind:  Pan,
		Point: l.last,
		Panel: l.panelIndex(l.last),
		Delta: ir.Point{X: float32(dx), Y: float32(dy)},
	})
	return l.Draw()
}

// Autoscale releases every zoom and pan, so the axes come from the data again,
// and redraws. It is the "reset view" every interactive chart needs.
func (l *Live) Autoscale() error {
	for _, p := range l.idx.Panels() {
		autoscale(p.X)
		autoscale(p.Y)
	}
	return l.Draw()
}

func (l *Live) fire(ev Event) Event {
	l.p.emit(ev)
	return ev
}

// panelAt returns the panel a device point is in, falling back to the first
// panel for a point in the margins — a wheel just outside the plot area is a
// wheel over the chart, and doing nothing there reads as a broken control.
func (l *Live) panelAt(pt ir.Point) (interact.Panel, bool) {
	panels := l.idx.Panels()
	if len(panels) == 0 {
		return interact.Panel{}, false
	}
	if i, ok := l.idx.PanelAt(pt); ok {
		return panels[i], true
	}
	return panels[0], true
}

func (l *Live) panelIndex(pt ir.Point) int {
	if i, ok := l.idx.PanelAt(pt); ok {
		return i
	}
	return -1
}

// zoomAxis scales the view about a device position.
//
// The arithmetic is done in device space and handed back through Invert, which
// is what makes one implementation cover every scale: the pointer stays over
// the same value on a log axis and a time axis for the same reason it does on
// a linear one, without this knowing which it has.
func zoomAxis(s scale.Scale, lo, hi, at float32, factor float64) {
	z, ok := s.(scale.Zoomer)
	if !ok {
		return
	}
	f := float32(factor)
	z.SetDomain(s.Invert(at+(lo-at)*f), s.Invert(at+(hi-at)*f))
}

func panAxis(s scale.Scale, lo, hi, delta float32) {
	z, ok := s.(scale.Zoomer)
	if !ok {
		return
	}
	z.SetDomain(s.Invert(lo-delta), s.Invert(hi-delta))
}

func setDomain(s scale.Scale, a, b float32) {
	z, ok := s.(scale.Zoomer)
	if !ok {
		return
	}
	z.SetDomain(s.Invert(a), s.Invert(b))
}

func autoscale(s scale.Scale) {
	if z, ok := s.(scale.Zoomer); ok {
		z.Autoscale()
	}
}
