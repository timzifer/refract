// Package render lowers a resolved chart into IR.
//
// This is the one place that knows the drawing order of a chart — background,
// grid, axes, data, guides — and the one place that turns a layout rectangle
// and a set of scales into actual primitives. Geoms emit their own marks;
// everything around them is built here.
package render

import (
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/layout"
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

	Layers []geom.Geom

	// ShowLegend requests a legend. Entries come from the layers.
	ShowLegend bool
}

// Draw lowers c into b. It does not call Flush: the caller owns the backend's
// lifecycle.
func Draw(b ir.Backend, c Chart) error {
	th := c.Theme
	canvas := ir.R(0, 0, float32(c.Width), float32(c.Height))

	// 1. Train the scales. Tick labels depend on the domain, and the layout
	//    depends on the tick labels, so this has to happen before anything is
	//    measured.
	for _, g := range c.Layers {
		if err := g.Train(c.X, c.Y); err != nil {
			return err
		}
	}

	// 2. Tick label *text* depends only on the domain, but Scale.Ticks also
	//    reports positions, which need a range. Give the scales a provisional
	//    unit range so the labels can be measured, then set the real range once
	//    the plot rectangle is known and ask again.
	c.X.SetRange(0, 1)
	c.Y.SetRange(1, 0)
	xTicks := c.X.Ticks(th.TickCountHintX)
	yTicks := c.Y.Ticks(th.TickCountHintY)

	legend := legendEntries(c, ir.Rect{})

	lay := layout.Compute(layout.Chart{
		Canvas:       canvas,
		Theme:        th,
		Title:        c.Title,
		XTitle:       c.XTitle,
		YTitle:       c.YTitle,
		XLabels:      labelsOf(xTicks),
		YLabels:      labelsOf(yTicks),
		LegendLabels: labelsOfEntries(legend),
	}, b)

	// 3. Real ranges. Y is inverted: larger values are higher on screen.
	c.X.SetRange(lay.Plot.Min.X, lay.Plot.Max.X)
	c.Y.SetRange(lay.Plot.Max.Y, lay.Plot.Min.Y)
	xTicks = c.X.Ticks(th.TickCountHintX)
	yTicks = c.Y.Ticks(th.TickCountHintY)

	// 4. Paint.
	drawBackground(b, canvas, lay.Plot, th)
	drawGrid(b, lay.Plot, th, xTicks, yTicks)
	drawAxes(b, lay, th, xTicks, yTicks)
	drawTitles(b, lay, th, c)

	if err := drawLayers(b, c, lay.Plot); err != nil {
		return err
	}

	if !lay.Legend.Empty() {
		drawLegend(b, lay.Legend, th, legendEntries(c, lay.Plot))
	}
	return nil
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

func labelsOfEntries(es []geom.LegendEntry) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Label)
	}
	return out
}

func legendEntries(c Chart, plot ir.Rect) []geom.LegendEntry {
	if !c.ShowLegend {
		return nil
	}
	out := make([]geom.LegendEntry, 0, len(c.Layers))
	for i, g := range c.Layers {
		e, ok := g.Legend(geom.Frame{Area: plot, X: c.X, Y: c.Y, Theme: c.Theme, Index: i})
		if ok && e.Label != "" {
			out = append(out, e)
		}
	}
	return out
}

func drawBackground(b ir.Backend, canvas, plot ir.Rect, th theme.Theme) {
	if th.Background.A != 0 {
		var p ir.Path
		p.Rect(canvas)
		b.FillPath(&p, ir.Solid(th.Background), ir.NonZero)
	}
	if th.PlotFill.A != 0 && !plot.Empty() {
		var p ir.Path
		p.Rect(plot)
		b.FillPath(&p, ir.Solid(th.PlotFill), ir.NonZero)
	}
}

func drawGrid(b ir.Backend, plot ir.Rect, th theme.Theme, xTicks, yTicks []scale.Tick) {
	if plot.Empty() {
		return
	}
	stroke := ir.Stroke{Color: th.GridColor, Width: th.GridWidth, Dash: th.GridDash}
	if !stroke.Visible() {
		return
	}
	if th.ShowGridX {
		for _, t := range xTicks {
			if !inRange(t.Pos, plot.Min.X, plot.Max.X) {
				continue
			}
			b.Polyline([]ir.Point{{X: t.Pos, Y: plot.Min.Y}, {X: t.Pos, Y: plot.Max.Y}}, stroke)
		}
	}
	if th.ShowGridY {
		for _, t := range yTicks {
			if !inRange(t.Pos, plot.Min.Y, plot.Max.Y) {
				continue
			}
			b.Polyline([]ir.Point{{X: plot.Min.X, Y: t.Pos}, {X: plot.Max.X, Y: t.Pos}}, stroke)
		}
	}
}

func drawAxes(b ir.Backend, lay layout.Result, th theme.Theme, xTicks, yTicks []scale.Tick) {
	plot := lay.Plot
	if plot.Empty() {
		return
	}
	axis := ir.Stroke{Color: th.AxisColor, Width: th.AxisWidth, Cap: ir.CapButt}
	tickFont := th.Font(th.TickSize)

	if th.ShowAxisLineX {
		b.Polyline([]ir.Point{{X: plot.Min.X, Y: plot.Max.Y}, {X: plot.Max.X, Y: plot.Max.Y}}, axis)
	}
	if th.ShowAxisLineY {
		b.Polyline([]ir.Point{{X: plot.Min.X, Y: plot.Min.Y}, {X: plot.Min.X, Y: plot.Max.Y}}, axis)
	}

	// X ticks. Labels are centred on the tick, so a dense axis will collide;
	// drop the labels that would overlap rather than let them run together.
	keep := selectXLabels(b, xTicks, tickFont, th.TickLabelPad)
	for i, t := range xTicks {
		if !inRange(t.Pos, plot.Min.X, plot.Max.X) {
			continue
		}
		if axis.Visible() && th.TickLength > 0 {
			b.Polyline([]ir.Point{
				{X: t.Pos, Y: plot.Max.Y},
				{X: t.Pos, Y: plot.Max.Y + th.TickLength},
			}, axis)
		}
		if t.Label == "" || !keep[i] {
			continue
		}
		b.Text(ir.TextRun{
			Text:  t.Label,
			Font:  tickFont,
			At:    ir.Point{X: t.Pos, Y: plot.Max.Y + th.TickLength + th.TickLabelPad},
			H:     ir.AlignCenter,
			V:     ir.AlignTop,
			Color: th.TickColor,
		})
	}

	// Y ticks. These stack vertically and are right-aligned against the axis,
	// so they collide far less often; the theme's tick count hint is enough.
	for _, t := range yTicks {
		if !inRange(t.Pos, plot.Min.Y, plot.Max.Y) {
			continue
		}
		if axis.Visible() && th.TickLength > 0 {
			b.Polyline([]ir.Point{
				{X: plot.Min.X - th.TickLength, Y: t.Pos},
				{X: plot.Min.X, Y: t.Pos},
			}, axis)
		}
		if t.Label == "" {
			continue
		}
		b.Text(ir.TextRun{
			Text:  t.Label,
			Font:  tickFont,
			At:    ir.Point{X: plot.Min.X - th.TickLength - th.TickLabelPad, Y: t.Pos},
			H:     ir.AlignEnd,
			V:     ir.AlignMiddle,
			Color: th.TickColor,
		})
	}
}

// selectXLabels greedily keeps every label that clears the previous kept one.
// Greedy left-to-right is the right policy here: it always keeps the first and
// preserves even spacing on a regular axis, which is what a reader expects.
func selectXLabels(m layout.Measurer, ticks []scale.Tick, font ir.FontRef, pad float32) []bool {
	keep := make([]bool, len(ticks))
	prevRight := float32(-1e30)
	for i, t := range ticks {
		if t.Label == "" {
			continue
		}
		w := m.Measure(ir.TextRun{Text: t.Label, Font: font}).Advance
		left := t.Pos - w/2
		if left < prevRight+pad {
			continue
		}
		keep[i] = true
		prevRight = t.Pos + w/2
	}
	return keep
}

func drawTitles(b ir.Backend, lay layout.Result, th theme.Theme, c Chart) {
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

func drawLayers(b ir.Backend, c Chart, plot ir.Rect) error {
	if plot.Empty() || len(c.Layers) == 0 {
		return nil
	}
	var clip ir.Path
	clip.Rect(plot)
	b.Push(&clip, ir.Identity)
	defer b.Pop()

	for i, g := range c.Layers {
		f := geom.Frame{Area: plot, X: c.X, Y: c.Y, Theme: c.Theme, Index: i}
		if err := g.Build(b, f); err != nil {
			return err
		}
	}
	return nil
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
