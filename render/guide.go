package render

import (
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/layout"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// colorGuides collects the continuous colour guides the layers contribute,
// merging the ones that would be drawn identically.
//
// Merging matters more than it sounds: two layers sharing one colour scale is
// the normal way to draw points and their trend, and two identical colourbars
// would take a column of the chart to say one thing twice.
func colorGuides(layers []geom.Geom) []geom.ColorGuide {
	var out []geom.ColorGuide
	seen := map[string]bool{}
	for _, l := range layers {
		g, ok := l.(geom.Guided)
		if !ok {
			continue
		}
		cg, ok := g.ColorGuide()
		if !ok || cg.Scale == nil {
			continue
		}
		k := cg.Key()
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, cg)
	}
	return out
}

// colorbarScale returns a positional scale over a colour scale's domain.
//
// A colourbar is an axis: it needs round numbers at readable intervals, which
// is exactly what a linear scale's tick generation already decides. Nicing is
// deliberately off — the bar shows the domain the data actually covers, and
// rounding it outwards would paint colours no mark has.
func colorbarScale(cs scale.ColorScale) scale.Scale {
	lo, hi := cs.Domain()
	s := scale.Linear()
	s.Train(lo, hi)
	return s
}

// colorbarLabels measures what a colourbar will write, before it is known
// where any of it goes. The provisional range is the same trick the axes use.
func colorbarLabels(gs []geom.ColorGuide, th theme.Theme) []layout.Colorbar {
	if len(gs) == 0 {
		return nil
	}
	out := make([]layout.Colorbar, 0, len(gs))
	for _, g := range gs {
		s := colorbarScale(g.Scale)
		s.SetRange(0, 1)
		out = append(out, layout.Colorbar{
			Title: g.Label,
			Ticks: labelsOf(s.Ticks(th.ColorbarTickCount)),
		})
	}
	return out
}

// colorbarStops is how finely a ramp is sampled into gradient stops.
//
// A backend interpolates between stops in its own colour space, which is not
// the linear light [palette.Ramp] blends in, so the stops have to be close
// enough that the difference is below a rounding error. Thirty-two across a
// bar of a couple of hundred pixels puts one every few pixels, which is well
// past that.
const colorbarStops = 32

// drawColorbar draws one continuous colour guide into box.
//
// The bar runs bottom to top, low value at the bottom, because that is the
// direction the Y axis beside it runs and a reader should not have to change
// convention halfway across a chart.
func drawColorbar(b ir.Backend, box ir.Rect, th theme.Theme, g geom.ColorGuide) {
	if box.Empty() {
		return
	}
	labelFont := th.Font(th.LabelSize)
	tickFont := th.Font(th.TickSize)

	top := box.Min.Y
	if g.Label != "" {
		mm := b.Measure(ir.TextRun{Text: g.Label, Font: labelFont})
		b.Text(ir.TextRun{
			Text:  g.Label,
			Font:  labelFont,
			At:    ir.Point{X: box.Min.X, Y: box.Min.Y},
			V:     ir.AlignTop,
			Color: th.LabelColor,
		})
		top += mm.Height() + th.TickLabelPad
	}

	bar := ir.Rect{
		Min: ir.Point{X: box.Min.X, Y: top},
		Max: ir.Point{X: box.Min.X + th.ColorbarThickness, Y: box.Max.Y},
	}
	if bar.Empty() {
		return
	}

	lo, hi := g.Scale.Domain()
	stops := make([]ir.GradientStop, 0, colorbarStops+1)
	for i := 0; i <= colorbarStops; i++ {
		t := float64(i) / colorbarStops
		stops = append(stops, ir.GradientStop{
			// Offset zero is the gradient's start, which is the bottom of the
			// bar, which is the low end of the domain.
			Offset: float32(t),
			Color:  g.Scale.Color(lo + t*(hi-lo)),
		})
	}
	var p ir.Path
	p.Rect(bar)
	b.FillPath(&p, ir.Fill{
		Start: ir.Point{X: bar.Min.X, Y: bar.Max.Y},
		End:   ir.Point{X: bar.Min.X, Y: bar.Min.Y},
		Stops: stops,
	}, ir.NonZero)

	if th.ColorbarBorder.A != 0 {
		b.StrokePath(&p, ir.Stroke{Color: th.ColorbarBorder, Width: th.AxisWidth})
	}

	// Ticks. The scale maps the domain onto the bar, inverted the way a Y axis
	// is, so that the low value sits at the bottom.
	s := colorbarScale(g.Scale)
	s.SetRange(bar.Max.Y, bar.Min.Y)
	axis := ir.Stroke{Color: th.ColorbarBorder, Width: th.AxisWidth, Cap: ir.CapButt}
	for _, t := range s.Ticks(th.ColorbarTickCount) {
		if t.Minor || t.Label == "" || !inRange(t.Pos, bar.Min.Y, bar.Max.Y) {
			continue
		}
		if axis.Visible() {
			b.Polyline([]ir.Point{
				{X: bar.Max.X, Y: t.Pos},
				{X: bar.Max.X + th.TickLength, Y: t.Pos},
			}, axis)
		}
		b.Text(ir.TextRun{
			Text:  t.Label,
			Font:  tickFont,
			At:    ir.Point{X: bar.Max.X + th.TickLength + th.TickLabelPad, Y: t.Pos},
			V:     ir.AlignMiddle,
			Color: th.TickColor,
		})
	}
}
