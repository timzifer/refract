package refract

import (
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/theme"
)

// The overlay vocabulary, re-exported so that a chart with a crosshair needs
// one import like a chart without one. See package render.
type (
	// Overlay paints over a finished chart. See [render.Overlay].
	Overlay = render.Overlay
	// OverlayFrame is what an overlay is told. See [render.OverlayFrame].
	OverlayFrame = render.OverlayFrame
	// OverlayPanel is one panel of that frame. See [render.OverlayPanel].
	OverlayPanel = render.OverlayPanel
)

// Overlay installs something to paint over the chart, and returns l so the call
// can be chained onto [Plot.Live]. Passing nil removes whatever was there.
//
// It takes effect on the next [Live.Draw]. An overlay is a *pointer* to a
// struct whose fields the caller then moves — a crosshair's position, a
// tooltip's lines — so installing it once and mutating it per event is the
// intended shape, and there is nothing to re-install.
//
//	cross := &refract.Crosshair{}
//	live.Overlay(cross)
//	p.On(refract.Hover, func(ev refract.Event) {
//		cross.At, cross.Show = ev.Point, ev.Panel >= 0
//	})
//
// A hover does not redraw by itself — [Live.Move] answers a question and does
// not change the chart — but [Input.Move] does when an overlay is installed,
// which is what makes the crosshair follow the pointer on a real surface. A
// frame identical to the last is still not painted, so a pointer moving over a
// chart with no overlay costs exactly what it did before.
func (l *Live) Overlay(o Overlay) *Live {
	l.chart.Overlay = o
	return l
}

// CurrentOverlay reports what is painting over the chart, or nil.
func (l *Live) CurrentOverlay() Overlay { return l.chart.Overlay }

// Crosshair is a pair of rules through a point, confined to the panel it is in.
//
// It is the cheapest thing a reader can be given for "which value is this":
// two lines meeting where the pointer is, so that a point in the middle of a
// scatter can be read off both axes at once.
//
// The zero value draws nothing. Set [Crosshair.At] and [Crosshair.Show] from a
// hover handler; the colours default to the chart's own axis colour, so a
// crosshair over a dark theme is legible without being told.
type Crosshair struct {
	// At is where the lines cross, in device space.
	At ir.Point
	// Show is whether to draw at all. A crosshair that is not shown draws
	// nothing — which changes the frame's call count and therefore makes that
	// one frame a full repaint. Moving one does not.
	Show bool

	// Panel is which panel to draw in, or -1 to use the one containing At.
	// It is a field rather than always inferred so that a chart linked to
	// another can put a crosshair in a panel the pointer is not in.
	Panel int

	// Color, Width and Dash override the theme. A zero Color takes the
	// theme's axis colour at half opacity, a zero Width takes one device
	// unit, and a nil Dash is the theme's grid dash.
	Color ir.Color
	Width float32
	Dash  []float32

	// Vertical and Horizontal turn each rule off. Both are drawn by default,
	// which is what "crosshair" means; a chart against a time axis often
	// wants only the vertical one.
	NoVertical   bool
	NoHorizontal bool
}

// DrawOverlay implements [render.Overlay].
func (c *Crosshair) DrawOverlay(b ir.Backend, f OverlayFrame) {
	if !c.Show || (c.NoVertical && c.NoHorizontal) {
		return
	}
	p, ok := panelFor(f, c.Panel, c.At)
	if !ok {
		return
	}
	stroke := ir.Stroke{
		Color: orElse(c.Color, fade(f.Theme.AxisColor, 0.55)),
		Width: orElseF(c.Width, 1),
		Dash:  orElseDash(c.Dash, f.Theme.GridDash),
	}
	// Clipped to the panel: a rule that ran into the margin would cross the
	// axis it is being read against.
	var clip ir.Path
	clip.Rect(p.Area)
	b.Push(&clip, ir.Identity)
	defer b.Pop()

	if !c.NoVertical {
		b.Polyline([]ir.Point{
			{X: c.At.X, Y: p.Area.Min.Y},
			{X: c.At.X, Y: p.Area.Max.Y},
		}, stroke)
	}
	if !c.NoHorizontal {
		b.Polyline([]ir.Point{
			{X: p.Area.Min.X, Y: c.At.Y},
			{X: p.Area.Max.X, Y: c.At.Y},
		}, stroke)
	}
}

// Highlight rings a set of marks, to say "these ones".
//
// It is what a chart linked to another draws: the first chart reports which row
// the pointer is on, the host finds where that row landed here with
// [interact.Index.Locate], and this puts a ring round it. Nothing about the
// layer changes, so the highlight cannot disturb the reading.
//
// The zero value draws nothing.
type Highlight struct {
	// At are the points to ring, in device space.
	At []ir.Point
	// Radius is the ring's radius in device units. Zero takes six, which is
	// a little larger than a default scatter marker.
	Radius float32
	// Color and Width override the theme. A zero Color takes the theme's
	// label colour and a zero Width takes two device units — a ring wants to
	// read as an annotation rather than as data.
	Color ir.Color
	Width float32
	// Panel confines the rings to one panel, or -1 to draw each in whichever
	// panel contains it. A point in no panel is not drawn.
	Panel int
}

// DrawOverlay implements [render.Overlay].
func (h *Highlight) DrawOverlay(b ir.Backend, f OverlayFrame) {
	if len(h.At) == 0 {
		return
	}
	r := orElseF(h.Radius, 6)
	stroke := ir.Stroke{
		Color: orElse(h.Color, f.Theme.LabelColor),
		Width: orElseF(h.Width, 2),
	}
	var path ir.Path
	for _, at := range h.At {
		if _, ok := panelFor(f, h.Panel, at); !ok {
			continue
		}
		path.Circle(at, r)
	}
	if !path.Empty() {
		b.StrokePath(&path, stroke)
	}
}

// Brush is the rectangle a reader is dragging out, drawn as feedback while they
// drag it.
//
// It is the visible half of [DragSelects] and [DragZooms]: [Input.Dragged]
// reports the rectangle, and this draws it. The two are separate because a
// surface may want to draw its own, and because a selection means nothing until
// it is released.
//
// The zero value draws nothing: an empty rectangle is not a selection.
type Brush struct {
	// Rect is the region, in device space. An empty one draws nothing.
	Rect ir.Rect
	// Fill and Stroke override the theme. A zero Fill takes the theme's axis
	// colour at a tenth opacity and a zero Stroke takes it at half.
	Fill   ir.Color
	Stroke ir.Color
	// Width is the outline's width. Zero takes one device unit.
	Width float32
}

// DrawOverlay implements [render.Overlay].
func (br *Brush) DrawOverlay(b ir.Backend, f OverlayFrame) {
	if br.Rect.Empty() {
		return
	}
	var path ir.Path
	path.Rect(br.Rect)
	b.FillPath(&path, ir.Solid(orElse(br.Fill, fade(f.Theme.AxisColor, 0.10))), ir.NonZero)
	b.StrokePath(&path, ir.Stroke{
		Color: orElse(br.Stroke, fade(f.Theme.AxisColor, 0.55)),
		Width: orElseF(br.Width, 1),
	})
}

// Tooltip is a box of text beside a point.
//
// It is the one overlay that measures: the box is sized to the lines it holds,
// through the backend that is going to draw them, so a tooltip is the width of
// its text in the font it is actually rendered in rather than in an estimate of
// one.
//
// It keeps itself on the canvas. A tooltip near the right edge flips to the
// left of its anchor and one near the bottom flips above it, because a box that
// ran off the drawing would hide the thing it was explaining.
//
// The zero value draws nothing.
type Tooltip struct {
	// At is the point the tooltip is about, in device space. The box is
	// placed beside it, not over it.
	At ir.Point
	// Lines are the text, one line each. No lines is no tooltip.
	Lines []string

	// Fill, Stroke and Color override the theme. A zero Fill takes the
	// theme's canvas background, a zero Stroke its axis colour, and a zero
	// Color its label colour — so a tooltip over a dark chart is dark.
	Fill   ir.Color
	Stroke ir.Color
	Color  ir.Color

	// Size is the type size in device units. Zero takes the theme's tick
	// size, which is the size the chart's other small text is set in.
	Size float64
	// Pad is the space between the text and the box. Zero takes six.
	Pad float32
	// Offset is how far the box sits from At. Zero takes twelve, which clears
	// a default marker and a fingertip.
	Offset float32
}

// DrawOverlay implements [render.Overlay].
func (t *Tooltip) DrawOverlay(b ir.Backend, f OverlayFrame) {
	if len(t.Lines) == 0 {
		return
	}
	size := t.Size
	if size <= 0 {
		size = f.Theme.TickSize
	}
	font := f.Theme.Font(size)
	pad := orElseF(t.Pad, 6)
	off := orElseF(t.Offset, 12)

	// Measured through the backend that will draw them, which is the whole
	// reason ir.Backend has Measure: a box sized from an estimate is a box
	// that clips its own text in whatever font the surface actually has.
	var w, h float32
	var ascent float32
	for i, line := range t.Lines {
		m := b.Measure(ir.TextRun{Text: line, Font: font})
		w = max(w, m.Advance)
		if i == 0 {
			ascent = m.Ascent
		}
		h += m.Ascent + m.Descent
	}
	if w <= 0 || h <= 0 {
		return
	}

	box := ir.Rect{
		Min: ir.Point{X: t.At.X + off, Y: t.At.Y + off},
		Max: ir.Point{X: t.At.X + off + w + 2*pad, Y: t.At.Y + off + h + 2*pad},
	}
	// Flipped rather than clamped: a box pushed back inside the canvas would
	// sit over the point it is about, and a tooltip covering its own subject
	// is worse than one on the other side.
	if box.Max.X > f.Canvas.Max.X {
		box.Min.X = t.At.X - off - (w + 2*pad)
		box.Max.X = t.At.X - off
	}
	if box.Max.Y > f.Canvas.Max.Y {
		box.Min.Y = t.At.Y - off - (h + 2*pad)
		box.Max.Y = t.At.Y - off
	}

	var path ir.Path
	path.Rect(box)
	b.FillPath(&path, ir.Solid(orElse(t.Fill, f.Theme.Background)), ir.NonZero)
	b.StrokePath(&path, ir.Stroke{
		Color: orElse(t.Stroke, fade(f.Theme.AxisColor, 0.55)),
		Width: 1,
	})

	y := box.Min.Y + pad + ascent
	col := orElse(t.Color, f.Theme.LabelColor)
	for i, line := range t.Lines {
		if i > 0 {
			m := b.Measure(ir.TextRun{Text: line, Font: font})
			y += m.Ascent + m.Descent
		}
		b.Text(ir.TextRun{
			Text:  line,
			At:    ir.Point{X: box.Min.X + pad, Y: y},
			Font:  font,
			Color: col,
			H:     ir.AlignStart,
			V:     ir.AlignBaseline,
		})
	}
}

// Overlays draws several overlays in order, so that a chart can have a
// crosshair and a tooltip at once.
//
// The order is the drawing order: later ones are on top, which is why a
// tooltip belongs after a crosshair rather than before it. A nil member is
// skipped, so a caller may keep a fixed-length list and switch one off by
// clearing it.
type Overlays []Overlay

// DrawOverlay implements [render.Overlay].
func (os Overlays) DrawOverlay(b ir.Backend, f OverlayFrame) {
	for _, o := range os {
		if o != nil {
			o.DrawOverlay(b, f)
		}
	}
}

// panelFor resolves which panel an overlay should draw in: the one it named, or
// the one containing the point.
func panelFor(f OverlayFrame, want int, at ir.Point) (OverlayPanel, bool) {
	if want >= 0 {
		for _, p := range f.Panels {
			if p.Index == want {
				return p, true
			}
		}
		return OverlayPanel{}, false
	}
	return f.PanelAt(at)
}

// orElse returns c, or fallback when c is fully transparent — which is what a
// zero ir.Color is, and therefore what "the caller did not say" looks like.
func orElse(c, fallback ir.Color) ir.Color {
	if c.A == 0 {
		return fallback
	}
	return c
}

func orElseF(v, fallback float32) float32 {
	if v <= 0 {
		return fallback
	}
	return v
}

func orElseDash(d, fallback []float32) []float32 {
	if d == nil {
		return fallback
	}
	return d
}

// fade returns c at a fraction of its opacity, for an overlay that has to be
// visible over the chart without competing with it.
func fade(c ir.Color, by float64) ir.Color {
	c.A = uint8(float64(c.A) * by)
	return c
}

var (
	_ Overlay = (*Crosshair)(nil)
	_ Overlay = (*Highlight)(nil)
	_ Overlay = (*Brush)(nil)
	_ Overlay = (*Tooltip)(nil)
	_ Overlay = Overlays(nil)
	_         = theme.Theme{}
)
