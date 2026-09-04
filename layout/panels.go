package layout

import (
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/theme"
)

// Grid is a set of Cartesian panels laid out together with their axes aligned.
//
// Alignment is the whole point and it is what makes this a constraint problem
// rather than a loop. Every panel in a column gets the same horizontal extent
// and every panel in a row the same vertical one, so a value at the same
// position means the same thing wherever the reader's eye lands. That is not
// achievable panel by panel: the width of the widest Y tick label in a column
// decides where every panel in that column starts, and the panels' common size
// then falls out of what is left.
type Grid struct {
	// Canvas is the full drawing surface.
	Canvas ir.Rect
	// Theme supplies sizes, paddings and fonts.
	Theme theme.Theme

	// Title is the chart title above the whole grid, or "" for none.
	Title string
	// XTitle and YTitle label the shared axes, once for the grid.
	XTitle, YTitle string

	// Rows and Cols are the shape of the grid.
	Rows, Cols int

	// Panels are the panels, in any order. A cell with no panel is a hole.
	Panels []Panel

	// LegendLabels and Colorbars are the guides for the grid as a whole.
	LegendLabels []string
	Colorbars    []Colorbar
}

// Panel is one Cartesian area within a grid.
type Panel struct {
	// Row and Col place the panel.
	Row, Col int

	// Strip is the label written in a band above the panel, or "" for none.
	// It is what a facet is named by.
	Strip string
	// RightStrip is a label written in a band down the panel's right side,
	// reading top to bottom. A two-way facet names its rows this way.
	RightStrip string

	// XLabels and YLabels are the tick labels this panel writes. A panel that
	// shares an axis with the panel beside it leaves them empty and takes the
	// space anyway, so that the panels stay the same size.
	XLabels, YLabels []string
}

// GridResult is where everything goes, in device space.
type GridResult struct {
	// Areas are the panel rectangles, parallel to Grid.Panels.
	Areas []ir.Rect
	// Strips are the label bands above each panel, parallel to Grid.Panels
	// and empty for a panel with no strip.
	Strips []ir.Rect
	// RightStrips are the label bands beside each panel, parallel to
	// Grid.Panels and empty for a panel with none.
	RightStrips []ir.Rect

	// Region is the rectangle the panels and their gutters occupy together.
	// The chart title is centred on it and the guides sit beside it.
	Region ir.Rect

	// Title, XTitle and YTitle are the baseline anchors for the grid's own
	// titles, zero when there is none. YTitle is drawn rotated a quarter turn
	// anticlockwise.
	Title, XTitle, YTitle ir.Point

	// Legend and Colorbars are the guide boxes, as in [Result].
	Legend    ir.Rect
	Colorbars []ir.Rect

	// TickLabelPad is copied from the theme so the renderer does not re-derive
	// it.
	TickLabelPad float32
}

// Panels lays out a grid.
//
// The order of decisions matters and is the reason this is not four
// independent calculations: the guides' height depends on how tall the panel
// region is, the panel region's width depends on how wide the guides are, and
// both depend on the tick labels — which are known before any of it, because
// a scale can name its ticks from its domain alone.
func Panels(g Grid, m Measurer) GridResult {
	th := g.Theme
	var r GridResult
	r.TickLabelPad = th.TickLabelPad
	if g.Rows <= 0 || g.Cols <= 0 {
		return r
	}
	r.Areas = make([]ir.Rect, len(g.Panels))
	r.Strips = make([]ir.Rect, len(g.Panels))
	r.RightStrips = make([]ir.Rect, len(g.Panels))

	tickFont := th.Font(th.TickSize)
	labelFont := th.Font(th.LabelSize)
	titleFont := th.Font(th.TitleSize)
	stripFont := th.Font(th.StripSize)

	area := g.Canvas.Inset(th.Margin, th.Margin, th.Margin, th.Margin)

	// The grid's own furniture: a title above, axis titles outside the panels.
	var titleH float32
	if g.Title != "" {
		titleH = m.Measure(ir.TextRun{Text: g.Title, Font: titleFont}).Height() + th.AxisTitlePad
	}
	var bottomTitleH, leftTitleH float32
	if g.XTitle != "" {
		bottomTitleH = m.Measure(ir.TextRun{Text: g.XTitle, Font: labelFont}).Height() + th.AxisTitlePad
	}
	if g.YTitle != "" {
		leftTitleH = m.Measure(ir.TextRun{Text: g.YTitle, Font: labelFont}).Height() + th.AxisTitlePad
	}

	// Per-column left gutters and per-row bottom gutters. A gutter is shared
	// by everything in its column or row, which is what keeps the axes lined
	// up when the panels have scales of their own.
	colGutter := make([]float32, g.Cols)
	rowGutter := make([]float32, g.Rows)
	stripH := make([]float32, g.Rows)
	rightStripW := make([]float32, g.Cols)

	bandH := m.Measure(ir.TextRun{Text: "Hg", Font: stripFont}).Height() + 2*th.StripPad
	for _, p := range g.Panels {
		if !inGrid(g, p) {
			continue
		}
		if w := maxAdvance(m, p.YLabels, tickFont); w > 0 {
			colGutter[p.Col] = maxOf(colGutter[p.Col], th.TickLength+th.TickLabelPad+w)
		}
		if h := maxHeight(m, p.XLabels, tickFont); h > 0 {
			rowGutter[p.Row] = maxOf(rowGutter[p.Row], th.TickLength+th.TickLabelPad+h)
		}
		if p.Strip != "" {
			stripH[p.Row] = bandH
		}
		if p.RightStrip != "" {
			rightStripW[p.Col] = bandH
		}
	}

	// The panel region's height is known before its width, because nothing on
	// the right can change it. That is what lets the guides — whose length is
	// a fraction of it — be measured before the width is decided.
	regionTop := area.Min.Y + titleH
	regionBottom := area.Max.Y - bottomTitleH
	usableH := regionBottom - regionTop
	panelH := (usableH - sum(rowGutter) - sum(stripH) - float32(g.Rows-1)*th.PanelGap) / float32(g.Rows)

	guides := measureGuides(th, g.LegendLabels, g.Colorbars, m, panelH*float32(g.Rows))

	var guideW float32
	for _, gd := range guides {
		guideW = maxOf(guideW, gd.w)
	}
	right := float32(0)
	if guideW > 0 {
		right = guideW + th.LegendPad
	} else if last := lastXLabel(g); last != "" {
		// Without a guide the rightmost tick label, which is centred on the
		// axis end, would otherwise run off the canvas.
		right = m.Measure(ir.TextRun{Text: last, Font: tickFont}).Advance / 2
	}

	regionLeft := area.Min.X + leftTitleH
	regionRight := area.Max.X - right
	usableW := regionRight - regionLeft
	panelW := (usableW - sum(colGutter) - sum(rightStripW) - float32(g.Cols-1)*th.PanelGap) / float32(g.Cols)

	// A canvas too small for its furniture would produce inverted rectangles
	// and geometry that maps to nonsense. Collapse to degenerate but
	// well-ordered panels instead.
	panelW = maxOf(panelW, 0)
	panelH = maxOf(panelH, 0)

	r.Region = ir.Rect{
		Min: ir.Point{X: regionLeft, Y: regionTop},
		Max: ir.Point{X: regionRight, Y: regionBottom},
	}

	// Column x origins and row y origins, accumulated across the gutters.
	colX := make([]float32, g.Cols)
	x := regionLeft
	for c := range g.Cols {
		if c > 0 {
			x += th.PanelGap
		}
		x += colGutter[c]
		colX[c] = x
		x += panelW + rightStripW[c]
	}
	rowY := make([]float32, g.Rows)
	y := regionTop
	for row := range g.Rows {
		if row > 0 {
			y += th.PanelGap
		}
		y += stripH[row]
		rowY[row] = y
		y += panelH + rowGutter[row]
	}

	for i, p := range g.Panels {
		if !inGrid(g, p) {
			continue
		}
		box := ir.Rect{
			Min: ir.Point{X: colX[p.Col], Y: rowY[p.Row]},
			Max: ir.Point{X: colX[p.Col] + panelW, Y: rowY[p.Row] + panelH},
		}
		r.Areas[i] = box
		if p.Strip != "" {
			r.Strips[i] = ir.Rect{
				Min: ir.Point{X: box.Min.X, Y: box.Min.Y - stripH[p.Row]},
				Max: ir.Point{X: box.Max.X, Y: box.Min.Y},
			}
		}
		if p.RightStrip != "" {
			r.RightStrips[i] = ir.Rect{
				Min: ir.Point{X: box.Max.X, Y: box.Min.Y},
				Max: ir.Point{X: box.Max.X + rightStripW[p.Col], Y: box.Max.Y},
			}
		}
	}

	// The panels' own extent, which the titles centre on. It is not the region
	// — the region includes the gutters, and a title centred on those sits off
	// to one side of the thing it names.
	span := ir.Rect{
		Min: ir.Point{X: colX[0], Y: rowY[0]},
		Max: ir.Point{X: colX[g.Cols-1] + panelW, Y: rowY[g.Rows-1] + panelH},
	}
	if g.Title != "" {
		mm := m.Measure(ir.TextRun{Text: g.Title, Font: titleFont})
		r.Title = ir.Point{X: (span.Min.X + span.Max.X) / 2, Y: area.Min.Y + mm.Ascent}
	}
	if g.XTitle != "" {
		mm := m.Measure(ir.TextRun{Text: g.XTitle, Font: labelFont})
		r.XTitle = ir.Point{X: (span.Min.X + span.Max.X) / 2, Y: area.Max.Y - mm.Descent}
	}
	if g.YTitle != "" {
		mm := m.Measure(ir.TextRun{Text: g.YTitle, Font: labelFont})
		r.YTitle = ir.Point{X: area.Min.X + mm.Ascent, Y: (span.Min.Y + span.Max.Y) / 2}
	}

	r.Legend, r.Colorbars = placeGuides(guides, span, th)
	return r
}

func inGrid(g Grid, p Panel) bool {
	return p.Row >= 0 && p.Row < g.Rows && p.Col >= 0 && p.Col < g.Cols
}

// lastXLabel returns the last tick label written by any panel in the bottom
// row, which is the one that can overhang the canvas.
func lastXLabel(g Grid) string {
	var out string
	for _, p := range g.Panels {
		if p.Row != g.Rows-1 || len(p.XLabels) == 0 {
			continue
		}
		out = p.XLabels[len(p.XLabels)-1]
	}
	return out
}

func sum(vs []float32) float32 {
	var t float32
	for _, v := range vs {
		t += v
	}
	return t
}
