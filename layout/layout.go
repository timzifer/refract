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

	// LegendLabels are the series names. An empty slice suppresses the legend.
	LegendLabels []string

	// Colorbars are the continuous colour guides. Only the extents of their
	// titles and tick labels matter here; where the ticks fall is the colour
	// scale's business, and it needs the bar's rectangle to answer.
	Colorbars []Colorbar
}

// Colorbar is what layout needs to know about one continuous colour guide.
type Colorbar struct {
	// Title names the quantity the ramp encodes, or "" for none.
	Title string
	// Ticks are the labels written beside the bar.
	Ticks []string
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
	// Legend is the rectangle reserved for the legend, empty if there is none.
	Legend ir.Rect
	// Colorbars are the rectangles reserved for the colour guides, one per
	// entry in Chart.Colorbars and in the same order. Each covers the bar, its
	// tick labels and its title.
	Colorbars []ir.Rect
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
		Canvas:       c.Canvas,
		Theme:        c.Theme,
		Title:        c.Title,
		XTitle:       c.XTitle,
		YTitle:       c.YTitle,
		Rows:         1,
		Cols:         1,
		Panels:       []Panel{{XLabels: c.XLabels, YLabels: c.YLabels}},
		LegendLabels: c.LegendLabels,
		Colorbars:    c.Colorbars,
	}, m)

	return Result{
		Plot:         g.Areas[0],
		Title:        g.Title,
		XTitle:       g.XTitle,
		YTitle:       g.YTitle,
		Legend:       g.Legend,
		Colorbars:    g.Colorbars,
		TickLabelPad: g.TickLabelPad,
	}
}

// guide is one entry of the column beside the plot: a legend, or a colourbar
// with its title and labels.
type guide struct {
	w, h   float32
	legend bool
}

// measureGuides sizes every guide a chart carries, in the order they will be
// stacked. plotH is the height of the panel region, which is what a
// colourbar's length is a fraction of.
func measureGuides(th theme.Theme, legend []string, bars []Colorbar, m Measurer, plotH float32) []guide {
	labelFont := th.Font(th.LabelSize)
	var out []guide

	if n := len(legend); n > 0 {
		entryH := m.Measure(ir.TextRun{Text: "Hg", Font: labelFont}).Height()
		out = append(out, guide{
			legend: true,
			w:      th.LegendSwatch + th.LegendGap + maxAdvance(m, legend, labelFont) + 2*th.LegendPadding,
			h:      float32(n)*entryH + float32(n-1)*th.LegendGap + 2*th.LegendPadding,
		})
	}

	for _, cb := range bars {
		// The bar takes a fraction of the plot's height, but never less than
		// enough to read a ramp across: a bar shorter than a few multiples of
		// its own thickness is a swatch, which is the thing a colourbar exists
		// not to be.
		length := float32(th.ColorbarFraction) * plotH
		if floor := 4 * th.ColorbarThickness; length < floor {
			length = floor
		}
		if length > plotH {
			length = plotH
		}

		g := guide{h: length}
		g.w = th.ColorbarThickness + th.TickLength + th.TickLabelPad +
			maxAdvance(m, cb.Ticks, th.Font(th.TickSize))
		if cb.Title != "" {
			mm := m.Measure(ir.TextRun{Text: cb.Title, Font: labelFont})
			g.h += mm.Height() + th.TickLabelPad
			g.w = maxOf(g.w, mm.Advance)
		}
		out = append(out, g)
	}
	return out
}

// placeGuides stacks the guides in the column to the right of the plot,
// centred on it, and returns the legend's box and the colourbars' in order.
func placeGuides(guides []guide, plot ir.Rect, th theme.Theme) (legend ir.Rect, bars []ir.Rect) {
	if len(guides) == 0 {
		return ir.Rect{}, nil
	}
	var total float32
	for i, g := range guides {
		if i > 0 {
			total += th.GuideGap
		}
		total += g.h
	}
	top := (plot.Min.Y+plot.Max.Y)/2 - total/2
	if top < plot.Min.Y {
		top = plot.Min.Y
	}
	left := plot.Max.X + th.LegendPad

	for _, g := range guides {
		box := ir.Rect{
			Min: ir.Point{X: left, Y: top},
			Max: ir.Point{X: left + g.w, Y: top + g.h},
		}
		if g.legend {
			legend = box
		} else {
			bars = append(bars, box)
		}
		top += g.h + th.GuideGap
	}
	return legend, bars
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
