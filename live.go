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
	l := &Live{p: p, t: t, b: b, idx: interact.New(), panel: -1}
	if l.chart, err = p.chart(); err != nil {
		t.Close()
		return nil, err
	}
	l.chart.Observer = l.idx
	l.prev = ir.NewRecorder(b)
	l.next = ir.NewRecorder(b)
	return l, nil
}

// Rebuild resolves the plot again, picking up layers, scales or a facet added
// since the Live was created. It forgets any zoom on a facet's free axes,
// which belong to panels that no longer exist.
func (l *Live) Rebuild() error {
	c, err := l.p.chart()
	if err != nil {
		return err
	}
	c.Observer = l.idx
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
	zoomAxis(p.X, p.Area.Min.X, p.Area.Max.X, pt.X, factor)
	zoomAxis(p.Y, p.Area.Max.Y, p.Area.Min.Y, pt.Y, factor)
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
	setDomain(p.X, r.Min.X, r.Max.X)
	setDomain(p.Y, r.Min.Y, r.Max.Y)
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
	panAxis(p.X, p.Area.Min.X, p.Area.Max.X, float32(dx))
	panAxis(p.Y, p.Area.Max.Y, p.Area.Min.Y, float32(dy))
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
