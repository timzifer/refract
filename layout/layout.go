// Package layout decides where the plot area, titles and legend go.
//
// v0.1 implements exactly one arrangement: a single Cartesian panel with axes
// on the left and bottom, an optional title above, and an optional legend to
// the right. It sizes everything by measuring the real text with the real
// backend, so an axis whose labels are wide gets a wide margin and nothing is
// ever clipped by a guessed constant.
//
// It is deliberately not a constraint solver. Faceting and multi-panel axis
// alignment are the v0.3 milestone; the seam this package presents — measure,
// then hand back rectangles — is what that solver will slot into, so callers
// will not have to change.
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
	// TickLabelPad is the distance from the axis to the near edge of a tick
	// label, copied from the theme so the renderer does not re-derive it.
	TickLabelPad float32
}

// Compute lays out a chart.
func Compute(c Chart, m Measurer) Result {
	th := c.Theme
	var r Result
	r.TickLabelPad = th.TickLabelPad

	tickFont := th.Font(th.TickSize)
	labelFont := th.Font(th.LabelSize)
	titleFont := th.Font(th.TitleSize)

	area := c.Canvas.Inset(th.Margin, th.Margin, th.Margin, th.Margin)

	// Top: the chart title.
	var titleH float32
	if c.Title != "" {
		mm := m.Measure(ir.TextRun{Text: c.Title, Font: titleFont})
		titleH = mm.Height() + th.AxisTitlePad
	}

	// Bottom: tick labels, then the axis title.
	xTickH := maxHeight(m, c.XLabels, tickFont)
	bottom := float32(0)
	if xTickH > 0 {
		bottom += th.TickLength + th.TickLabelPad + xTickH
	}
	var xTitleH float32
	if c.XTitle != "" {
		xTitleH = m.Measure(ir.TextRun{Text: c.XTitle, Font: labelFont}).Height()
		bottom += xTitleH + th.AxisTitlePad
	}

	// Left: tick labels, then the rotated axis title.
	yTickW := maxAdvance(m, c.YLabels, tickFont)
	left := float32(0)
	if yTickW > 0 {
		left += th.TickLength + th.TickLabelPad + yTickW
	}
	var yTitleH float32
	if c.YTitle != "" {
		yTitleH = m.Measure(ir.TextRun{Text: c.YTitle, Font: labelFont}).Height()
		left += yTitleH + th.AxisTitlePad
	}

	// Right: the legend, or just enough room that the last X tick label — which
	// is centred on the axis end — does not run off the canvas.
	right := float32(0)
	var legendW float32
	if len(c.LegendLabels) > 0 {
		legendW = th.LegendSwatch + th.LegendGap + maxAdvance(m, c.LegendLabels, labelFont) + 2*th.LegendPadding
		right = legendW + th.LegendPad
	} else if n := len(c.XLabels); n > 0 {
		right = m.Measure(ir.TextRun{Text: c.XLabels[n-1], Font: tickFont}).Advance / 2
	}

	plot := ir.Rect{
		Min: ir.Point{X: area.Min.X + left, Y: area.Min.Y + titleH},
		Max: ir.Point{X: area.Max.X - right, Y: area.Max.Y - bottom},
	}
	// A canvas too small for its furniture would otherwise produce an inverted
	// rectangle and geometry that maps to nonsense. Collapse to a degenerate
	// but well-ordered plot area instead.
	if plot.Max.X < plot.Min.X {
		plot.Max.X = plot.Min.X
	}
	if plot.Max.Y < plot.Min.Y {
		plot.Max.Y = plot.Min.Y
	}
	r.Plot = plot

	if c.Title != "" {
		mm := m.Measure(ir.TextRun{Text: c.Title, Font: titleFont})
		r.Title = ir.Point{
			X: (plot.Min.X + plot.Max.X) / 2,
			Y: area.Min.Y + mm.Ascent,
		}
	}
	if c.XTitle != "" {
		r.XTitle = ir.Point{
			X: (plot.Min.X + plot.Max.X) / 2,
			Y: area.Max.Y - m.Measure(ir.TextRun{Text: c.XTitle, Font: labelFont}).Descent,
		}
	}
	if c.YTitle != "" {
		mm := m.Measure(ir.TextRun{Text: c.YTitle, Font: labelFont})
		r.YTitle = ir.Point{
			X: area.Min.X + mm.Ascent,
			Y: (plot.Min.Y + plot.Max.Y) / 2,
		}
	}
	if legendW > 0 {
		entryH := m.Measure(ir.TextRun{Text: "Hg", Font: labelFont}).Height()
		h := float32(len(c.LegendLabels))*entryH +
			float32(len(c.LegendLabels)-1)*th.LegendGap +
			2*th.LegendPadding
		top := (plot.Min.Y+plot.Max.Y)/2 - h/2
		if top < plot.Min.Y {
			top = plot.Min.Y
		}
		r.Legend = ir.Rect{
			Min: ir.Point{X: plot.Max.X + th.LegendPad, Y: top},
			Max: ir.Point{X: plot.Max.X + th.LegendPad + legendW, Y: top + h},
		}
	}
	return r
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
