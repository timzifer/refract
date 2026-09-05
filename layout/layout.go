// Package layout decides where the plot area, titles and guides go.
//
// [Compute] lays out a single Cartesian panel: axes on the left and bottom, an
// optional title above, and a column of guides — a legend, colourbars — to the
// right. [Panels] lays out a grid of them with their axes aligned, which is
// what subplots and faceting are made of.
//
// Everything is sized by measuring the real text with the real backend, so an
// axis whose labels are wide gets a wide margin and nothing is ever clipped by
// a guessed constant.
package layout

import (
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/theme"
)

// Measurer is the text-measurement capability layout needs. Every backend
// provides it; taking the narrow interface rather than the whole backend keeps
// layout unable to draw anything by accident.
type Measurer interface {
	Measure(run ir.TextRun) ir.TextMetrics
}

// Chart is everything layout needs to know about what will be drawn.
type Chart struct {
	// Canvas is the full drawing surface.
	Canvas ir.Rect
	// Theme supplies sizes, paddings and fonts.
	Theme theme.Theme

	// Title is the chart title, or "" for none.
	Title string
	// XTitle and YTitle are axis titles, or "" for none.
	XTitle, YTitle string

	// XLabels and YLabels are the tick label texts. Only their measured
	// extents matter here; their positions are the scales' business.
	XLabels, YLabels []string

	// Guides are the keys beside the plot, in the order they will be stacked:
	// a legend, colourbars, size keys. An empty slice reserves no column.
	Guides []Guide
}

// GuideKind is what one guide in the column beside the plot shows.
//
// The kinds are here rather than in three fields because they are one column
// with one stacking rule, and because a fourth kind must not mean a fourth
// field on every struct between here and [github.com/timzifer/refract/render].
// That was the state before the size key: a legend was a []string, a colourbar
// a []Colorbar, and adding a third would have meant a third of each and a third
// branch in the placement.
type GuideKind uint8

// The guide kinds.
const (
	// GuideLegend is the swatch-and-label list naming a chart's series.
	GuideLegend GuideKind = iota
	// GuideColorbar is the ramp a continuous colour scale is read off.
	GuideColorbar
	// GuideSize is the ladder of sample marks a size channel is read off.
	GuideSize
)

// Guide is what layout needs to know about one entry of the guide column.
//
// Only extents matter here. Where a colourbar's ticks fall along its bar is the
// colour scale's business and needs the bar's rectangle to answer; what a
// legend swatch looks like is the theme's. Layout measures the text and
// reserves the room.
type Guide struct {
	// Kind decides how the guide is sized.
	Kind GuideKind
	// Title names the quantity the guide encodes, or "" for none. A legend has
	// none: its entries name themselves.
	Title string
	// Labels are the texts written in the guide — one per legend entry, one per
	// colourbar tick, one per size sample.
	Labels []string
	// Sizes are the mark diameters a [GuideSize] key shows, parallel to Labels
	// and empty for every other kind. A size key's rows are as tall as their
	// marks, which is the one thing about it that is not text.
	Sizes []float32
}

// Result is where everything goes, in device space.
type Result struct {
	// Plot is the data area. Scales map into it.
	Plot ir.Rect
	// Title is the baseline anchor for the chart title, horizontally centred
	// on the plot area. Zero if there is no title.
	Title ir.Point
	// XTitle is the baseline anchor for the X axis title, centred on the plot
	// area. Zero if there is none.
	XTitle ir.Point
	// YTitle is the anchor for the Y axis title, centred on the plot area and
	// meant to be drawn rotated a quarter turn anticlockwise. Zero if there is
	// none.
	YTitle ir.Point
	// Guides are the rectangles reserved for the guide column, one per entry in
	// Chart.Guides and in the same order. Each covers the whole guide: a
	// colourbar's bar, its tick labels and its title.
	Guides []ir.Rect
	// TickLabelPad is the distance from the axis to the near edge of a tick
	// label, copied from the theme so the renderer does not re-derive it.
	TickLabelPad float32
}

// Compute lays out a single-panel chart.
//
// It is [Panels] over a one-by-one grid, and it exists because that is the
// shape almost every chart has: one panel, its axes, a guide column. Going
// through the same solver is what keeps a lone chart and a facet of one from
// coming out differently.
func Compute(c Chart, m Measurer) Result {
	g := Panels(Grid{
		Canvas: c.Canvas,
		Theme:  c.Theme,
		Title:  c.Title,
		XTitle: c.XTitle,
		YTitle: c.YTitle,
		Rows:   1,
		Cols:   1,
		Panels: []Panel{{XLabels: c.XLabels, YLabels: c.YLabels}},
		Guides: c.Guides,
	}, m)

	return Result{
		Plot:         g.Areas[0],
		Title:        g.Title,
		XTitle:       g.XTitle,
		YTitle:       g.YTitle,
		Guides:       g.Guides,
		TickLabelPad: g.TickLabelPad,
	}
}

// extent is how much room one guide needs.
type extent struct{ w, h float32 }

// measureGuides sizes every guide a chart carries, in the order they will be
// stacked, one entry per guide. plotH is the height of the panel region, which
// is what a colourbar's length is a fraction of.
//
// One function over one list, rather than a loop per kind. That is the whole
// generalisation: the column has a stacking rule and a width rule, and a kind
// contributes only its own measurement to them.
func measureGuides(th theme.Theme, guides []Guide, m Measurer, plotH float32) []extent {
	if len(guides) == 0 {
		return nil
	}
	out := make([]extent, len(guides))
	for i, g := range guides {
		switch g.Kind {
		case GuideColorbar:
			out[i] = measureColorbar(th, g, m, plotH)
		case GuideSize:
			out[i] = measureSizeKey(th, g, m)
		default:
			out[i] = measureLegend(th, g, m)
		}
	}
	return out
}

func measureLegend(th theme.Theme, g Guide, m Measurer) extent {
	n := len(g.Labels)
	if n == 0 {
		return extent{}
	}
	labelFont := th.Font(th.LabelSize)
	entryH := m.Measure(ir.TextRun{Text: "Hg", Font: labelFont}).Height()
	return extent{
		w: th.LegendSwatch + th.LegendGap + maxAdvance(m, g.Labels, labelFont) + 2*th.LegendPadding,
		h: float32(n)*entryH + float32(n-1)*th.LegendGap + 2*th.LegendPadding,
	}
}

func measureColorbar(th theme.Theme, g Guide, m Measurer, plotH float32) extent {
	// The bar takes a fraction of the plot's height, but never less than
	// enough to read a ramp across: a bar shorter than a few multiples of its
	// own thickness is a swatch, which is the thing a colourbar exists not to
	// be.
	length := float32(th.ColorbarFraction) * plotH
	if floor := 4 * th.ColorbarThickness; length < floor {
		length = floor
	}
	if length > plotH {
		length = plotH
	}

	e := extent{h: length}
	e.w = th.ColorbarThickness + th.TickLength + th.TickLabelPad +
		maxAdvance(m, g.Labels, th.Font(th.TickSize))
	return withTitle(e, th, g.Title, m)
}

// measureSizeKey sizes the ladder of sample marks a size channel is read off.
//
// A row is as tall as its own mark, not as tall as its label: the samples run
// from the smallest value to the largest and the largest is the whole point of
// the key. The width is the widest sample plus the widest label, so the labels
// line up in a column of their own rather than stepping out with the circles.
func measureSizeKey(th theme.Theme, g Guide, m Measurer) extent {
	n := len(g.Labels)
	if n == 0 {
		return extent{}
	}
	labelFont := th.Font(th.LabelSize)
	textH := m.Measure(ir.TextRun{Text: "Hg", Font: labelFont}).Height()

	var widest, h float32
	for i := range n {
		var d float32
		if i < len(g.Sizes) {
			d = g.Sizes[i]
		}
		widest = maxOf(widest, d)
		h += maxOf(d, textH)
	}
	e := extent{
		w: widest + th.LegendGap + maxAdvance(m, g.Labels, labelFont) + 2*th.LegendPadding,
		h: h + float32(n-1)*th.LegendGap + 2*th.LegendPadding,
	}
	return withTitle(e, th, g.Title, m)
}

// withTitle adds room for a guide's title above it.
func withTitle(e extent, th theme.Theme, title string, m Measurer) extent {
	if title == "" {
		return e
	}
	mm := m.Measure(ir.TextRun{Text: title, Font: th.Font(th.LabelSize)})
	e.h += mm.Height() + th.TickLabelPad
	e.w = maxOf(e.w, mm.Advance)
	return e
}

// placeGuides stacks the guides in the column to the right of the plot,
// centred on it, and returns their boxes in the same order.
//
// One box per guide, whatever its kind. A guide with nothing to say — a legend
// with no entries — is measured at zero and still gets its box, so that the
// result stays index for index with what the caller asked for and nothing has
// to correlate two lists of different lengths.
func placeGuides(guides []extent, plot ir.Rect, th theme.Theme) []ir.Rect {
	if len(guides) == 0 {
		return nil
	}
	var total float32
	any := false
	for _, g := range guides {
		if g.h <= 0 || g.w <= 0 {
			continue
		}
		if any {
			total += th.GuideGap
		}
		total, any = total+g.h, true
	}
	top := (plot.Min.Y+plot.Max.Y)/2 - total/2
	if top < plot.Min.Y {
		top = plot.Min.Y
	}
	left := plot.Max.X + th.LegendPad

	out := make([]ir.Rect, len(guides))
	first := true
	for i, g := range guides {
		if g.h <= 0 || g.w <= 0 {
			continue
		}
		if !first {
			top += th.GuideGap
		}
		first = false
		out[i] = ir.Rect{
			Min: ir.Point{X: left, Y: top},
			Max: ir.Point{X: left + g.w, Y: top + g.h},
		}
		top += g.h
	}
	return out
}

func maxOf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func maxAdvance(m Measurer, labels []string, font ir.FontRef) float32 {
	var w float32
	for _, s := range labels {
		if s == "" {
			continue
		}
		if a := m.Measure(ir.TextRun{Text: s, Font: font}).Advance; a > w {
			w = a
		}
	}
	return w
}

func maxHeight(m Measurer, labels []string, font ir.FontRef) float32 {
	var h float32
	for _, s := range labels {
		if s == "" {
			continue
		}
		if v := m.Measure(ir.TextRun{Text: s, Font: font}).Height(); v > h {
			h = v
		}
	}
	return h
}
