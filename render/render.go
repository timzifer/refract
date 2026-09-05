// Package render lowers a resolved chart into IR.
//
// This is the one place that knows the drawing order of a chart — background,
// grid, axes, data, guides — and the one place that turns a layout rectangle
// and a set of scales into actual primitives. Geoms emit their own marks;
// everything around them is built here.
package render

import (
	"sync"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/layout"
	"github.com/timzifer/refract/mathtext"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// Chart is a fully specified chart, ready to be drawn.
type Chart struct {
	Width, Height int
	DPR           float64
	Theme         theme.Theme

	Title  string
	XTitle string
	YTitle string

	X, Y scale.Scale

	// Coord is the stage between the scales and the IR: what the interval a
	// scale maps into means. Nil is [coord.Cartesian], which is the identity,
	// so a chart that names no coord draws exactly what it always drew.
	//
	// It belongs to the chart rather than to a panel: the panels of a facet are
	// the same plot over different rows, and one of them in a different
	// coordinate system would be a different chart.
	Coord coord.Coord

	Layers []geom.Geom

	// ShowLegend requests a legend. Entries come from the layers.
	ShowLegend bool

	// Description is what the chart says about itself in words, for a backend
	// that can carry it — see [ir.Semantics]. It is announced before anything
	// is drawn and never affects what is.
	Description ir.Description

	// Math typesets the notation in this chart's labels. It is nil for a chart
	// whose labels are text, which is the default and costs nothing: the
	// backend is wrapped only when there is a typesetter to wrap it with.
	Math mathtext.Typesetter

	// Panels, Rows and Cols describe a multi-panel chart: subplots, or the
	// facets of one plot. When Panels is empty the chart is the single panel
	// described by X, Y and Layers.
	Panels     []Panel
	Rows, Cols int

	// Serial draws the panels one at a time. The zero value builds them
	// concurrently where that is possible and worth it — see [drawData] — and
	// produces the same output either way.
	Serial bool

	// Observer is told which panel and which layer is drawing, so that a
	// caller watching the backend can attribute a mark to the layer that made
	// it. It is nil for an ordinary render, and setting it forces the serial
	// path: the observer is told things in order, and two panels drawing at
	// once have no order to be told in.
	Observer Observer

	// RowSink, when non-nil, collects which source row is behind each mark. It
	// is separate from Observer because it is a separate cost: a layer does
	// the bookkeeping only when someone is listening, so this is what turns it
	// on. See [geom.Rows].
	//
	// It is not called Rows because that name is already the facet grid's.
	RowSink geom.Rows
}

// Observer is told the structure a render is drawing, as it draws it.
//
// It is how hit-testing gets built without widening the IR. A backend sees
// primitives — a polyline, some markers — and nothing about which layer of
// which panel emitted them; an Observer is told that separately, so a caller
// wrapping the backend can tag what it sees. Nothing here draws, and nothing
// here can change what is drawn.
//
// The calls come in paint order: one Panel, then a Layer for each of its
// layers, then the next Panel. Only the data pass is announced — the grid, the
// axes and the guides are furniture, and a pointer landing on a grid line has
// not landed on anything.
type Observer interface {
	// Panel opens a panel: its index in the chart, the rectangle it occupies,
	// the scales that place values in it, and the coord that turns a pair of
	// mapped positions into a point there. The scales are ranged for this
	// panel and must not be modified; the coord is framed for it and is what
	// turns a device position back into a pair — which is the only way a
	// tooltip over a pie slice names a value rather than a pixel.
	Panel(i int, area ir.Rect, x, y scale.Scale, cd coord.Coord)

	// Layer opens a layer within the panel just announced: its index among
	// that panel's layers, and its legend label if it has one.
	Layer(i int, label string)
}

// Panel is one Cartesian area of a multi-panel chart.
type Panel struct {
	// Row and Col place the panel in the grid.
	Row, Col int
	// Strip and RightStrip are the labels naming the panel, above it and
	// beside it.
	Strip, RightStrip string

	// X and Y are this panel's scales. Panels sharing an axis share the scale
	// object, which is what makes the axis shared rather than merely similar.
	X, Y scale.Scale

	// Coord overrides the chart's coordinate system for this panel, and is nil
	// for the panels that use it — which is every panel of a facet, because
	// the panels of a facet are one plot over different rows and one of them
	// in a different coordinate system would be a different chart.
	//
	// A grid of subplots is the case that needs it: those panels are separate
	// plots that happen to share a canvas, each with its own scales and its
	// own axes, so a pie beside a bar chart is two coords beside each other.
	Coord coord.Coord
	// Layers are this panel's marks.
	Layers []geom.Geom

	// ShowX and ShowY report whether this panel writes its own tick labels. A
	// panel that shares an axis with its neighbour leaves the labels to the
	// edge of the grid.
	ShowX, ShowY bool
}

// Draw lowers c into b. It does not call Flush: the caller owns the backend's
// lifecycle.
//
// The order is fixed here and nowhere else: background, then every panel's
// grid and axes, then the titles, then every panel's data inside its own clip,
// then the guides. Data is drawn after the furniture so that a mark is never
// hidden by a grid line, and the guides last because they sit outside every
// panel and must not be clipped by one.
func Draw(b ir.Backend, c Chart) error {
	// Every label — measured during layout and drawn during the paint — goes
	// through the backend, so a typesetter is installed by wrapping it once
	// here rather than at each of the dozen places that write text.
	//
	// The unwrapped backend is kept for the one thing that is asked of the
	// backend itself rather than of the drawing: whether it can carry a
	// description. A wrapper forwards drawing calls and hides optional
	// interfaces, so the question has to be put to the backend that answers it.
	raw := b
	b = withMath(b, c.Math)

	th := c.Theme
	canvas := ir.R(0, 0, float32(c.Width), float32(c.Height))
	panels, rows, cols := c.panels()

	// 1. Train the scales. Tick labels depend on the domain, and the layout
	//    depends on the tick labels, so this has to happen before anything is
	//    measured.
	for _, p := range panels {
		for _, g := range p.Layers {
			if err := g.Train(p.X, p.Y); err != nil {
				return err
			}
		}
	}

	// 2. Measure. Tick label *text* depends only on the domain, but
	//    Scale.Ticks also reports positions, which need a range — so
	//    measurePanels gives every scale a provisional unit range, and the
	//    real one is set below once the rectangles exist. The guides are
	//    collected here too, because how wide they are decides how wide the
	//    panels can be — and because collecting a size key is what gives its
	//    scale the diameters the theme asks for, which the marks then read.
	guides := chartGuides(c, panels, th, ir.Rect{})

	lay := layout.Panels(layout.Grid{
		Canvas: canvas,
		Theme:  th,
		Title:  c.Title,
		XTitle: c.XTitle,
		YTitle: c.YTitle,
		Rows:   rows,
		Cols:   cols,
		Panels: measurePanels(panels, th),
		Guides: layoutGuides(guides, th),
	}, b)

	// 3. Paint. Furniture for every panel first, then the titles, then the
	//    data — so a mark is never buried under a grid line — and the guides
	//    last, because they sit outside every panel and must not be clipped
	//    by one.
	//
	//    The description goes first of all, before any ink: a backend that
	//    writes a document header has it in hand when it writes one.
	if s, ok := raw.(ir.Semantics); ok && !c.Description.Empty() {
		s.Describe(c.Description)
	}
	drawBackground(b, canvas, th)

	fur := acquireFurniture()
	for i, p := range panels {
		area := lay.Areas[i]
		cd, xTicks, yTicks := p.rangeTo(c.coordOf(p), area, th)
		fur.Reset()
		cd.Furniture(fur, area, metricsOf(th), xTicks, yTicks)
		drawPanelFill(b, area, th)
		drawGrid(b, th, p, fur, xTicks, yTicks)
		drawAxes(b, th, p, fur, xTicks, yTicks)
		drawStrip(b, lay.Strips[i], th, p.Strip, 0)
		drawStrip(b, lay.RightStrips[i], th, p.RightStrip, halfPi)
	}
	releaseFurniture(fur)

	drawTitles(b, lay, th, c)

	if err := drawData(b, c, panels, lay.Areas, th); err != nil {
		return err
	}

	// The solver reserves one box per guide, in order, so these are parallel.
	// The legend is rebuilt against the real plot rectangle: an entry can
	// depend on where the layer was drawn, and the provisional pass above had
	// no rectangle to give it.
	for i, box := range lay.Guides {
		if i >= len(guides) {
			break
		}
		g := guides[i]
		if g.kind == layout.GuideLegend {
			g.entries = legendEntries(c, panels, lay.Areas[0])
		}
		drawGuide(b, box, th, g)
	}
	return nil
}

// coord is the chart's coordinate system, which is [coord.Cartesian] when it
// names none.
func (c Chart) coord() coord.Coord {
	if c.Coord == nil {
		return coord.Cartesian()
	}
	return c.Coord
}

// coordOf is the coord one panel is drawn in: its own, or the chart's.
func (c Chart) coordOf(p Panel) coord.Coord {
	if p.Coord != nil {
		return p.Coord
	}
	return c.coord()
}

// panels resolves the chart into a panel list, wrapping a single-panel chart
// into a one-by-one grid so that there is one code path rather than two.
func (c Chart) panels() ([]Panel, int, int) {
	if len(c.Panels) > 0 {
		rows, cols := c.Rows, c.Cols
		for _, p := range c.Panels {
			rows = max(rows, p.Row+1)
			cols = max(cols, p.Col+1)
		}
		return c.Panels, rows, cols
	}
	return []Panel{{
		X: c.X, Y: c.Y, Layers: c.Layers, ShowX: true, ShowY: true,
	}}, 1, 1
}

// rangeTo gives the panel's scales their real range, returns the coord framed
// in the panel, and returns the ticks that fall in it.
//
// Which interval each scale maps into is the coord's decision now: Cartesian
// sets the rectangle's edges, with Y inverted so that larger values are higher
// on screen, and Polar sets an angle range and a radius range.
//
// It is called again before the data pass because panels sharing a scale share
// one object, so the range left behind by the last panel of the furniture pass
// is not this panel's.
func (p Panel) rangeTo(cd coord.Coord, area ir.Rect, th theme.Theme) (coord.Coord, []scale.Tick, []scale.Tick) {
	framed := p.setRange(cd, area)
	return framed, p.X.Ticks(th.TickCountHintX), p.Y.Ticks(th.TickCountHintY)
}

// setRange is rangeTo without the ticks, for the data pass, which needs the
// range and has no use for the tick list the furniture pass already drew.
func (p Panel) setRange(cd coord.Coord, area ir.Rect) coord.Coord {
	return cd.Frame(area, p.X, p.Y)
}

// measurePanels reports what each panel will write, so the solver can size the
// gutters around it.
func measurePanels(panels []Panel, th theme.Theme) []layout.Panel {
	out := make([]layout.Panel, len(panels))
	for i, p := range panels {
		p.X.SetRange(0, 1)
		p.Y.SetRange(1, 0)
		out[i] = layout.Panel{
			Row:        p.Row,
			Col:        p.Col,
			Strip:      p.Strip,
			RightStrip: p.RightStrip,
		}
		// A panel that writes no tick labels needs no gutter for them, which is
		// what gives a pie with its axes turned off the whole panel to fill.
		if p.ShowX && th.ShowTicksX {
			out[i].XLabels = labelsOf(p.X.Ticks(th.TickCountHintX))
		}
		if p.ShowY && th.ShowTicksY {
			out[i].YLabels = labelsOf(p.Y.Ticks(th.TickCountHintY))
		}
	}
	return out
}

// cullEps is the tolerance for "is this tick inside the plot area".
//
// Tick positions come out of a float32 mapping, so a tick sitting exactly on
// an edge can land a hair outside it. Half a pixel is far below anything
// visible and far above the rounding error.
const cullEps = 0.5

func inRange(v, lo, hi float32) bool { return v >= lo-cullEps && v <= hi+cullEps }

func labelsOf(ticks []scale.Tick) []string {
	out := make([]string, 0, len(ticks))
	for _, t := range ticks {
		if t.Label != "" {
			out = append(out, t.Label)
		}
	}
	return out
}

// legendEntries collects the legend rows of every panel, keeping the first of
// each label.
//
// Faceted panels carry the same layers over different rows, so every panel
// offers the same entries; a legend that repeated them once per panel would
// grow with the facet rather than with the data. That is also why a grouped
// layer costs nothing extra here: a facet whose panels hold different groups
// contributes the union of them, in the order they were first seen.
//
// A layer contributes as many entries as it has to say — a grouped layer names
// its series, a layer painted from a qualitative palette names its categories —
// through [geom.Legends], which prefers a layer's own list where it has one.
func legendEntries(c Chart, panels []Panel, area ir.Rect) []geom.LegendEntry {
	if !c.ShowLegend {
		return nil
	}
	var out []geom.LegendEntry
	seen := map[string]bool{}
	for _, p := range panels {
		for i, g := range p.Layers {
			f := geom.Frame{Area: area, X: p.X, Y: p.Y, Theme: c.Theme, Index: i}
			for _, e := range geom.Legends(g, f) {
				if e.Label == "" || seen[e.Label] {
					continue
				}
				seen[e.Label] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// Furniture is per panel and sized by the tick count rather than by the data,
// but a chart redrawn every frame still asks for it every frame — so it comes
// out of a pool, like everything else here that is refilled rather than
// rebuilt. One is held for the whole furniture pass and reset between panels.
var furniturePool sync.Pool

func acquireFurniture() *coord.Furniture {
	f, _ := furniturePool.Get().(*coord.Furniture)
	if f == nil {
		return new(coord.Furniture)
	}
	return f
}

func releaseFurniture(f *coord.Furniture) {
	f.Reset()
	furniturePool.Put(f)
}

// metricsOf hands a coord the theme lengths it needs to place furniture. A
// coord must not know what a theme is, so the three numbers travel rather than
// the theme.
func metricsOf(th theme.Theme) coord.Metrics {
	return coord.Metrics{
		TickLen:      th.TickLength,
		MinorTickLen: th.TickLength * minorTickScale,
		LabelPad:     th.TickLabelPad,
	}
}

func drawBackground(b ir.Backend, canvas ir.Rect, th theme.Theme) {
	if th.Background.A == 0 {
		return
	}
	var p ir.Path
	p.Rect(canvas)
	b.FillPath(&p, ir.Solid(th.Background), ir.NonZero)
}

func drawPanelFill(b ir.Backend, area ir.Rect, th theme.Theme) {
	if th.PlotFill.A == 0 || area.Empty() {
		return
	}
	var p ir.Path
	p.Rect(area)
	b.FillPath(&p, ir.Solid(th.PlotFill), ir.NonZero)
}

// drawStrip paints the band naming a facet. rotation turns the label for a
// band down the side of a panel, where it reads top to bottom.
func drawStrip(b ir.Backend, box ir.Rect, th theme.Theme, label string, rotation float64) {
	if label == "" || box.Empty() {
		return
	}
	if th.StripBG.A != 0 {
		var p ir.Path
		p.Rect(box)
		b.FillPath(&p, ir.Solid(th.StripBG), ir.NonZero)
	}
	if th.StripBorder.A != 0 {
		var p ir.Path
		p.Rect(box)
		b.StrokePath(&p, ir.Stroke{Color: th.StripBorder, Width: th.AxisWidth})
	}
	b.Text(ir.TextRun{
		Text:     label,
		Font:     th.Font(th.StripSize),
		At:       ir.Point{X: (box.Min.X + box.Max.X) / 2, Y: (box.Min.Y + box.Max.Y) / 2},
		H:        ir.AlignCenter,
		V:        ir.AlignMiddle,
		Rotation: rotation,
		Color:    th.StripColor,
	})
}

func drawGrid(b ir.Backend, th theme.Theme, c Panel, fur *coord.Furniture, xTicks, yTicks []scale.Tick) {
	stroke := ir.Stroke{Color: th.GridColor, Width: th.GridWidth, Dash: th.GridDash}
	if !stroke.Visible() {
		return
	}
	// Minor ticks get a tick mark but no grid line. A log axis emits eight of
	// them per decade; drawing a grid line for each would turn the plot area
	// into a hatch and bury the data it is there to support.
	if th.ShowGridX && !banded(c.X) {
		for i := range xTicks {
			strokeShape(b, shapeAt(fur.GridX, i), stroke)
		}
	}
	if th.ShowGridY && !banded(c.Y) {
		for i := range yTicks {
			strokeShape(b, shapeAt(fur.GridY, i), stroke)
		}
	}
}

// strokeShape draws one piece of furniture: as a polyline where the coord
// reported a straight run, and as a path where it reported a curve.
//
// The polyline is not a shortcut. A Cartesian grid line has reached the
// backend as a two-point Polyline since v0.1, and the golden files, the damage
// rectangles and the SVG in the documentation are all written in those terms.
func strokeShape(b ir.Backend, s *coord.Shape, stroke ir.Stroke) {
	if s == nil {
		return
	}
	if len(s.Pts) >= 2 {
		b.Polyline(s.Pts, stroke)
		return
	}
	if !s.Path.Empty() {
		b.StrokePath(&s.Path, stroke)
	}
}

// shapeAt is the i'th shape of a per-tick list, or nil where the coord had
// nothing to report for that tick.
func shapeAt(shapes []coord.Shape, i int) *coord.Shape {
	if i >= len(shapes) {
		return nil
	}
	return &shapes[i]
}

// banded reports whether an axis positions categories in slots.
//
// A band scale's ticks sit at the centre of each slot, which is where the mark
// is — so a grid line there is drawn straight through the bar or box it is
// supposed to help read. The grid is a reference for a continuous quantity;
// a categorical axis has no continuous quantity to reference.
func banded(s scale.Scale) bool {
	_, ok := s.(scale.Band)
	return ok
}

// minorTickScale is how long a minor tick is relative to a major one. A minor
// tick is drawn shorter so that the labelled ticks stay the ones the eye lands
// on; it travels to the coord as part of [coord.Metrics], because the coord is
// what decides where a tick mark goes and a coord must not know what a theme
// is.
const minorTickScale = 0.55

func drawAxes(b ir.Backend, th theme.Theme, p Panel, fur *coord.Furniture, xTicks, yTicks []scale.Tick) {
	axis := ir.Stroke{Color: th.AxisColor, Width: th.AxisWidth, Cap: ir.CapButt}
	tickFont := th.Font(th.TickSize)

	if th.ShowAxisLineX {
		strokeShape(b, &fur.AxisX, axis)
	}
	if th.ShowAxisLineY {
		strokeShape(b, &fur.AxisY, axis)
	}

	// X ticks. Where the labels sit along one line they will collide on a
	// dense axis; drop the ones that would overlap rather than let them run
	// together. Labels arranged around a ring share no line and are all kept.
	keep := selectXLabels(b, fur, xTicks, tickFont, th.TickLabelPad)
	for i, t := range xTicks {
		if !inFurniture(fur.InX, i) || !th.ShowTicksX {
			continue
		}
		if axis.Visible() {
			strokeShape(b, shapeAt(fur.TickX, i), axis)
		}
		if t.Label == "" || !keep[i] || !p.ShowX {
			continue
		}
		b.Text(labelRun(t.Label, tickFont, fur.LabelX[i], th.TickColor))
	}

	// Y ticks. On a Cartesian axis these stack vertically and are right-
	// aligned against the axis, so they collide far less often; the theme's
	// tick count hint is enough.
	for i, t := range yTicks {
		if !inFurniture(fur.InY, i) || !th.ShowTicksY {
			continue
		}
		if axis.Visible() {
			strokeShape(b, shapeAt(fur.TickY, i), axis)
		}
		if t.Label == "" || !p.ShowY {
			continue
		}
		b.Text(labelRun(t.Label, tickFont, fur.LabelY[i], th.TickColor))
	}
}

// inFurniture reports whether tick i falls inside the panel. A coord that
// reported no answer for it has nothing to draw.
func inFurniture(in []bool, i int) bool { return i < len(in) && in[i] }

// labelRun is one tick label, placed where the coord put it.
func labelRun(text string, font ir.FontRef, at coord.Label, col ir.Color) ir.TextRun {
	return ir.TextRun{
		Text:     text,
		Font:     font,
		At:       at.At,
		H:        at.H,
		V:        at.V,
		Rotation: at.Rotation,
		Color:    col,
	}
}

// selectXLabels greedily keeps every label that clears the previous kept one.
// Greedy left-to-right is the right policy here: it always keeps the first and
// preserves even spacing on a regular axis, which is what a reader expects.
func selectXLabels(m layout.Measurer, fur *coord.Furniture, ticks []scale.Tick, font ir.FontRef, pad float32) []bool {
	keep := make([]bool, len(ticks))
	if !fur.XLabelsShareARow {
		// Two labels on opposite sides of a ring can share an x and still be a
		// finger apart, so the overlap test does not apply and every label is
		// kept.
		for i := range keep {
			keep[i] = true
		}
		return keep
	}
	prevRight := float32(-1e30)
	for i, t := range ticks {
		if t.Label == "" || !inFurniture(fur.InX, i) || i >= len(fur.LabelX) {
			continue
		}
		w := m.Measure(ir.TextRun{Text: t.Label, Font: font}).Advance
		left := fur.LabelX[i].At.X - w/2
		if left < prevRight+pad {
			continue
		}
		keep[i] = true
		prevRight = fur.LabelX[i].At.X + w/2
	}
	return keep
}

func drawTitles(b ir.Backend, lay layout.GridResult, th theme.Theme, c Chart) {
	if c.Title != "" {
		b.Text(ir.TextRun{
			Text:  c.Title,
			Font:  th.Font(th.TitleSize),
			At:    lay.Title,
			H:     ir.AlignCenter,
			Color: th.TitleColor,
		})
	}
	labelFont := th.Font(th.LabelSize)
	if c.XTitle != "" {
		b.Text(ir.TextRun{
			Text:  c.XTitle,
			Font:  labelFont,
			At:    lay.XTitle,
			H:     ir.AlignCenter,
			Color: th.LabelColor,
		})
	}
	if c.YTitle != "" {
		b.Text(ir.TextRun{
			Text:     c.YTitle,
			Font:     labelFont,
			At:       lay.YTitle,
			H:        ir.AlignCenter,
			Rotation: -halfPi,
			Color:    th.LabelColor,
		})
	}
}

// halfPi is a quarter turn. The Y axis title reads bottom-to-top, which is a
// rotation of -90 degrees in screen coordinates.
const halfPi = 1.5707963267948966

func drawLayers(b ir.Backend, p Panel, plot ir.Rect, th theme.Theme, obs Observer, rows geom.Rows, cd coord.Coord) error {
	if plot.Empty() || len(p.Layers) == 0 {
		return nil
	}
	// What a panel clips to is the coord's answer: the rectangle, or the disc
	// inscribed in it.
	var clip ir.Path
	cd.Clip(&clip, plot)
	b.Push(&clip, ir.Identity)
	defer b.Pop()

	for i, g := range p.Layers {
		f := geom.Frame{Area: plot, X: p.X, Y: p.Y, Coord: cd, Theme: th, Index: i, Rows: rows}
		if obs != nil {
			obs.Layer(i, layerLabel(g, f))
		}
		if err := g.Build(b, f); err != nil {
			return err
		}
	}
	return nil
}

// layerLabel is what the layer calls itself: its legend entry, or failing
// that what it was configured with.
//
// The fallback is not redundant. A layer coloured from a continuous scale
// contributes a colourbar rather than a legend entry, so it has no legend
// label at all — and it is exactly the layer a reader is most likely to point
// at. An unlabelled annotation still has no name, and gets none.
func layerLabel(g geom.Geom, f geom.Frame) string {
	if e, ok := g.Legend(f); ok && e.Label != "" {
		return e.Label
	}
	d, ok := geom.Describe(g)
	if !ok {
		return ""
	}
	if d.Label != "" {
		return d.Label
	}
	return d.Y
}

func drawLegend(b ir.Backend, box ir.Rect, th theme.Theme, entries []geom.LegendEntry) {
	if len(entries) == 0 {
		return
	}
	if th.LegendBG.A != 0 || th.LegendBorder.A != 0 {
		var p ir.Path
		p.Rect(box)
		if th.LegendBG.A != 0 {
			b.FillPath(&p, ir.Solid(th.LegendBG), ir.NonZero)
		}
		if th.LegendBorder.A != 0 {
			b.StrokePath(&p, ir.Stroke{Color: th.LegendBorder, Width: 1})
		}
	}

	labelFont := th.Font(th.LabelSize)
	mm := b.Measure(ir.TextRun{Text: "Hg", Font: labelFont})
	entryH := mm.Height()

	x := box.Min.X + th.LegendPadding
	y := box.Min.Y + th.LegendPadding
	for _, e := range entries {
		cy := y + entryH/2
		drawSwatch(b, e, th, x, cy)
		b.Text(ir.TextRun{
			Text:  e.Label,
			Font:  labelFont,
			At:    ir.Point{X: x + th.LegendSwatch + th.LegendGap, Y: cy},
			V:     ir.AlignMiddle,
			Color: th.LegendColor,
		})
		y += entryH + th.LegendGap
	}
}

func drawSwatch(b ir.Backend, e geom.LegendEntry, th theme.Theme, x, cy float32) {
	w := th.LegendSwatch
	switch e.Kind {
	case geom.SwatchMarker:
		b.Markers(e.Marker, []ir.Point{{X: x + w/2, Y: cy}}, ir.MarkerStyle{
			Size: th.MarkerSize,
			Fill: e.Color,
		})
	case geom.SwatchBox:
		var p ir.Path
		p.Rect(ir.R(x, cy-w/2, x+w, cy+w/2))
		b.FillPath(&p, ir.Solid(e.Color), ir.NonZero)
	default: // geom.SwatchLine
		width := e.Width
		if width <= 0 {
			width = th.LineWidth
		}
		b.Polyline([]ir.Point{{X: x, Y: cy}, {X: x + w, Y: cy}}, ir.Stroke{
			Color: e.Color,
			Width: width,
			Cap:   ir.CapRound,
			Dash:  e.Dash,
		})
	}
}
