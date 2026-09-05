package render

import (
	"fmt"

	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/layout"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// The guide column: a legend, the colourbars, the size keys. One list, one
// stacking rule, one drawing switch.
//
// It used to be two of everything — a []string of legend labels beside a
// []Colorbar, measured by two loops and placed into two fields. A third kind
// would have made that three of everything, so v0.9 generalised it once instead
// of extending it twice: [layout.Guide] carries what layout needs of any kind,
// and [guide] carries what render needs to draw one.

// guide is one entry of the chart's guide column.
//
// The three kinds are a union rather than three types because they are one
// ordered list: what decides where a guide goes is its position in the column,
// not what it shows.
type guide struct {
	kind layout.GuideKind

	// entries is a legend's rows.
	entries []geom.LegendEntry
	// color is a colourbar's guide, and size a size key's.
	color geom.ColorGuide
	size  geom.SizeGuide
	// samples are a size key's rows: the value each sample stands for and the
	// diameter it is drawn at.
	samples []sizeSample
}

// sizeSample is one row of a size key.
type sizeSample struct {
	label string
	size  float32
}

// chartGuides collects the whole guide column, in stacking order: the legend
// first, then the colourbars, then the size keys.
//
// The order is the one a reader scans in — what the series are, then what the
// colours mean, then what the sizes mean — and it is fixed here so that a chart
// with all three is laid out the same way every time.
func chartGuides(c Chart, panels []Panel, th theme.Theme, area ir.Rect) []guide {
	var out []guide
	if es := legendEntries(c, panels, area); len(es) > 0 {
		out = append(out, guide{kind: layout.GuideLegend, entries: es})
	}
	for _, cg := range colorGuides(layersOf(panels)) {
		out = append(out, guide{kind: layout.GuideColorbar, color: cg})
	}
	for _, sg := range sizeGuides(layersOf(panels), th) {
		out = append(out, guide{
			kind:    layout.GuideSize,
			size:    sg,
			samples: sizeSamples(sg, th),
		})
	}
	return out
}

// layersOf flattens every panel's layers. Faceted panels share their scales, so
// the merge inside each collector reduces them to the one guide that describes
// all of them.
func layersOf(panels []Panel) []geom.Geom {
	var all []geom.Geom
	for _, p := range panels {
		all = append(all, p.Layers...)
	}
	return all
}

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

// sizeGuides collects the size keys the layers contribute, merging duplicates
// the way colorGuides does.
//
// It also does the one thing that has to happen before anything is measured:
// it gives each size scale the diameters the theme asks for. A geom cannot do
// that in Build — panels are built on separate goroutines and a scale shared by
// two of them would be written from both — and it has no theme in Train. So the
// range is set here, once per scale, on the serial path, before the sizes are
// read for the key or for the marks.
func sizeGuides(layers []geom.Geom, th theme.Theme) []geom.SizeGuide {
	var out []geom.SizeGuide
	seen := map[string]bool{}
	for i, l := range layers {
		g, ok := l.(geom.Sized)
		if !ok {
			continue
		}
		sg, ok := g.SizeGuide()
		if !ok || sg.Scale == nil {
			continue
		}
		sg.Scale.SetRange(0, th.BubbleSize)
		if sg.Color.A == 0 {
			// The layer takes its colour from the palette, and which entry that
			// is depends on where it sits in the chart — which the layer cannot
			// know when it is asked.
			sg.Color = paletteAt(th, i)
		}
		k := sg.Key()
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, sg)
	}
	return out
}

func paletteAt(th theme.Theme, i int) ir.Color {
	pal := th.Palette
	if len(pal) == 0 {
		pal = theme.Light.Palette
	}
	return pal.At(i)
}

// sizeSamples picks the values a size key shows.
//
// They come from a linear scale's own tick generation, for the reason a
// colourbar's do: a key labelled 3.7, 7.4 and 11.1 is a key nobody can
// interpolate between, and round numbers at readable intervals is exactly what
// [scale.Scale.Ticks] already decides. The anchor itself is left out — a mark of
// no size is not a sample — and the largest is always kept, because it is what
// sets the reader's sense of the scale.
func sizeSamples(g geom.SizeGuide, th theme.Theme) []sizeSample {
	lo, hi := g.Scale.Domain()
	s := scale.Linear()
	s.Train(lo, hi)
	s.SetRange(0, 1)

	want := th.SizeKeyCount
	if want <= 0 {
		want = 3
	}

	var out []sizeSample
	for _, t := range s.Ticks(want) {
		if t.Minor || t.Label == "" || t.Value <= lo || t.Value > hi {
			continue
		}
		out = append(out, sizeSample{label: t.Label, size: g.Scale.Size(t.Value)})
	}
	if len(out) == 0 {
		// A domain no round number falls inside still has an extreme, and the
		// extreme is the sample that matters.
		return []sizeSample{{label: fmt.Sprintf("%g", hi), size: g.Scale.Size(hi)}}
	}
	// Thin from the small end, keeping the largest: the big samples are the
	// ones a reader measures against.
	for len(out) > want {
		out = out[1:]
	}
	return out
}

// layoutGuides is what the solver needs of each guide: its title and the text
// it writes, plus a size key's mark diameters.
func layoutGuides(gs []guide, th theme.Theme) []layout.Guide {
	if len(gs) == 0 {
		return nil
	}
	out := make([]layout.Guide, 0, len(gs))
	for _, g := range gs {
		switch g.kind {
		case layout.GuideColorbar:
			s := colorbarScale(g.color.Scale)
			s.SetRange(0, 1)
			out = append(out, layout.Guide{
				Kind:   layout.GuideColorbar,
				Title:  g.color.Label,
				Labels: labelsOf(s.Ticks(th.ColorbarTickCount)),
			})
		case layout.GuideSize:
			lg := layout.Guide{Kind: layout.GuideSize, Title: g.size.Label}
			for _, s := range g.samples {
				lg.Labels = append(lg.Labels, s.label)
				lg.Sizes = append(lg.Sizes, s.size)
			}
			out = append(out, lg)
		default:
			lg := layout.Guide{Kind: layout.GuideLegend}
			for _, e := range g.entries {
				lg.Labels = append(lg.Labels, e.Label)
			}
			out = append(out, lg)
		}
	}
	return out
}

// drawGuide paints one guide into the box the solver reserved for it.
func drawGuide(b ir.Backend, box ir.Rect, th theme.Theme, g guide) {
	if box.Empty() {
		return
	}
	switch g.kind {
	case layout.GuideColorbar:
		drawColorbar(b, box, th, g.color)
	case layout.GuideSize:
		drawSizeKey(b, box, th, g)
	default:
		drawLegend(b, box, th, g.entries)
	}
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
	tickFont := th.Font(th.TickSize)

	top := guideTitle(b, box, th, g.Label)

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

// drawSizeKey draws the ladder of sample marks a size channel is read off.
//
// The samples are drawn as the marks are — a filled circle with an outline, in
// the layer's own colour — because a key whose swatch is not the mark is a key
// that has to be translated before it can be used. They stack smallest first,
// which is the direction the colourbar beside them runs in.
func drawSizeKey(b ir.Backend, box ir.Rect, th theme.Theme, g guide) {
	if len(g.samples) == 0 {
		return
	}
	labelFont := th.Font(th.LabelSize)
	top := guideTitle(b, box, th, g.size.Label)

	var widest float32
	for _, s := range g.samples {
		widest = max(widest, s.size)
	}
	textH := b.Measure(ir.TextRun{Text: "Hg", Font: labelFont}).Height()

	fill := ir.Fade(g.size.Color, sizeKeyFillOpacity)
	stroke := ir.Stroke{Color: g.size.Color, Width: 1}

	x := box.Min.X + th.LegendPadding
	y := top + th.LegendPadding
	for _, s := range g.samples {
		h := max(s.size, textH)
		cy := y + h/2
		if s.size > 0 {
			var p ir.Path
			p.Circle(ir.Point{X: x + widest/2, Y: cy}, s.size/2)
			if fill.A != 0 {
				b.FillPath(&p, ir.Solid(fill), ir.NonZero)
			}
			b.StrokePath(&p, stroke)
		}
		b.Text(ir.TextRun{
			Text:  s.label,
			Font:  labelFont,
			At:    ir.Point{X: x + widest + th.LegendGap, Y: cy},
			V:     ir.AlignMiddle,
			Color: th.LegendColor,
		})
		y += h + th.LegendGap
	}
}

// sizeKeyFillOpacity matches what a bubble is drawn with, so the key reads as
// the same mark rather than as a darker relative of it.
const sizeKeyFillOpacity = 0.55

// guideTitle writes a guide's title at the top of its box and returns the Y the
// guide's body starts at. A guide with no title starts at the top of its box.
func guideTitle(b ir.Backend, box ir.Rect, th theme.Theme, title string) float32 {
	if title == "" {
		return box.Min.Y
	}
	labelFont := th.Font(th.LabelSize)
	mm := b.Measure(ir.TextRun{Text: title, Font: labelFont})
	b.Text(ir.TextRun{
		Text:  title,
		Font:  labelFont,
		At:    ir.Point{X: box.Min.X, Y: box.Min.Y},
		V:     ir.AlignTop,
		Color: th.LabelColor,
	})
	return box.Min.Y + mm.Height() + th.TickLabelPad
}
