package geom

import (
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Rect draws one rectangle per row, occupying an arbitrary box in data space.
//
// It is the mark [Bar] is not: a bar grows from a shared baseline to a value
// and therefore always touches the axis, while a rect is bounded on both axes
// by the row itself. That one difference is what a heatmap, a gantt chart, a
// candle, a waterfall step and a waffle cell all need, and it is why they are
// recipes over this mark rather than five more geoms — see docs/chart-types.md.
//
// An edge the row does not name is the slot the axis implies: a band scale's
// own bandwidth, or the closest spacing in the data narrowed by [BarWidth].
// So a heatmap over two categorical axes needs two columns and a colour, and
// nothing else:
//
//	geom.Rect(src, geom.X("day"), geom.Y("hour"),
//	    geom.ColorBy("calls", scale.Sequential(palette.Viridis)))
//
// while a gantt bar, which knows where it starts and stops, names both:
//
//	geom.Rect(src, geom.X("start"), geom.X2("end"), geom.Y("task"))
//
// [X2] gives the far edge on the horizontal axis and [Y2] on the vertical one.
// A [ColorBy] column paints each cell separately, through a ramp or through a
// qualitative palette; without one the whole layer is one colour.
//
// Cells that share an edge are drawn as separate shapes and antialiased
// separately, so a shared edge can show as a hairline of the background. That
// is compositing rather than a gap in the data — the cells are exactly
// contiguous, and the golden files say so — and removing it would need a mesh
// primitive in the IR, which is the thing ADR 0007 exists to refuse. Use
// [BarWidth] or the axis's own padding for a gap that is meant to be seen.
func Rect(src data.Source, opts ...Option) Geom {
	return &rectGeom{src: src, cfg: newConfig(opts)}
}

type rectGeom struct {
	src data.Source
	cfg config
	s   series
	x2  []float64
	err error
}

func (g *rectGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if g.cfg.x2col != "" {
		g.x2, g.err = column(g.src, g.cfg.x2col, x)
		if g.err != nil {
			return g.err
		}
		if len(g.x2) != len(g.s.x) {
			g.err = errLength(g.cfg.xcol, g.cfg.x2col, len(g.s.x), len(g.x2))
			return g.err
		}
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	trainColumn(x, g.s.x)
	trainColumn(y, g.s.y)
	if g.x2 != nil {
		trainColumn(x, g.x2)
	}
	if g.s.y2 != nil {
		trainColumn(y, g.s.y2)
	}
	g.cfg.trainColors(g.s)

	// An edge that comes from the slot rather than from a column has width the
	// domain does not know about, so the outermost cells would be clipped in
	// half. A band scale already reserves a slot per category and needs no
	// widening; a continuous one is widened by half a slot at each end, exactly
	// as a bar's axis is.
	if g.x2 == nil {
		widen(x, g.s.x, g.halfWidth(g.s.x))
	}
	if g.s.y2 == nil {
		widen(y, g.s.y, g.halfWidth(g.s.y))
	}
	return nil
}

// halfWidth is how far a slot-sized edge reaches on each side of its value, in
// data units.
func (g *rectGeom) halfWidth(vs []float64) float64 {
	return smallestGap(vs) * g.widthFraction() / 2
}

func (g *rectGeom) widthFraction() float64 {
	if g.cfg.barWidth <= 0 || g.cfg.barWidth > 1 {
		// A cell fills its slot: a heatmap with gaps between its tiles reads as
		// a grid of marks rather than as a field, which is the opposite of what
		// it is for. A bar's 0.8 is a different mark answering a different
		// question.
		return 1
	}
	return g.cfg.barWidth
}

func (g *rectGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	fill := g.cfg.colorFor(f)
	if g.cfg.fill != nil {
		fill = *g.cfg.fill
	}

	sc := acquire(f)
	defer sc.release()

	ok := sc.plottable(g.s, f.X, f.Y)
	halfX, halfY := g.halfWidth(g.s.x), g.halfWidth(g.s.y)

	// A rect already aggregates whatever it counted — one cell is one row of a
	// summary — so there is nothing here to reduce: dropping a cell would drop
	// a category rather than a pixel.
	rects := sc.rects[:0]
	rows := sc.rows[:0]
	for i := range g.s.x {
		if !ok[i] || (g.x2 != nil && !defined(f.X, g.x2[i])) {
			continue
		}
		x0, x1 := spanOn(f.X, g.s.x, g.x2, i, halfX, true)
		y0, y1 := spanOn(f.Y, g.s.y, g.s.y2, i, halfY, false)
		rects = append(rects, ir.R(x0, y0, x1, y1))
		rows = append(rows, i)
	}
	sc.rects, sc.rows = rects, rows
	if len(rects) == 0 {
		return nil
	}
	// A cell's row is at its middle: a rect is bounded on both axes, so unlike
	// a bar there is no end that means more than the other.
	if f.tracking() {
		sc.pts = grow(sc.pts, len(rects))
		for i, r := range rects {
			sc.pts[i] = ir.Point{X: (r.Min.X + r.Max.X) / 2, Y: (r.Min.Y + r.Max.Y) / 2}
		}
		f.Marks(sc.pts, sc.sourceRows(g.s, rows))
	}

	stroke := ir.Stroke{Color: g.cfg.colorFor(f), Width: pick(g.cfg.width, 1)}
	outline := g.cfg.fill != nil && g.cfg.color != nil

	// One subpath per cell, whether they are batched by colour or drawn in one
	// call, so that a pointer lands on the cell rather than on the sheet — see
	// docs/adr/0015-hit-testing.md.
	if cols := sc.colorsFor(g.cfg, g.s, rows); cols != nil {
		for _, run := range sc.groupByRect(rects, cols) {
			if run.color.A == 0 {
				continue
			}
			sc.fill.Reset()
			for _, r := range run.rects {
				sc.fill.Rect(r)
			}
			b.FillPath(&sc.fill, ir.Solid(run.color), ir.NonZero)
		}
		return nil
	}
	if fill.A == 0 {
		return nil
	}
	sc.fill.Reset()
	for _, r := range rects {
		sc.fill.Rect(r)
	}
	b.FillPath(&sc.fill, ir.Solid(fill), ir.NonZero)
	if outline && stroke.Visible() {
		b.StrokePath(&sc.fill, stroke)
	}
	return nil
}

// spanOn returns the device-space edges of one cell on one axis: the pair of
// columns where the row named both, and the slot around the single value where
// it did not.
func spanOn(s scale.Scale, v, v2 []float64, i int, half float64, horizontal bool) (float32, float32) {
	if v2 != nil {
		a, b := s.Map(v[i]), s.Map(v2[i])
		if b < a {
			a, b = b, a
		}
		return a, b
	}
	if band, ok := s.(scale.Band); ok {
		c, w := s.Map(v[i]), band.Bandwidth()
		return c - w/2, c + w/2
	}
	a, b := s.Map(v[i]-half), s.Map(v[i]+half)
	if b < a {
		a, b = b, a
	}
	return a, b
}

// widen extends a continuous domain by half a slot at each end, so that a mark
// with width is not clipped in half at the edge of the plot.
func widen(s scale.Scale, vs []float64, half float64) {
	if _, band := s.(scale.Band); band || half <= 0 {
		return
	}
	lo, hi, ok := extent(vs)
	if ok {
		s.Train(lo-half, hi+half)
	}
}

func (g *rectGeom) ColorGuide() (ColorGuide, bool) {
	return g.cfg.colorGuide(g.s, g.err)
}

func (g *rectGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.legends(f, nil, g.s, SwatchBox))
}

func (g *rectGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil || g.cfg.varying(g.s) {
		return LegendEntry{}, false
	}
	col := g.cfg.colorFor(f)
	if g.cfg.fill != nil {
		col = *g.cfg.fill
	}
	return LegendEntry{Label: g.cfg.labelFor(), Color: col, Kind: SwatchBox}, true
}

func (g *rectGeom) Source() data.Source { return g.src }
func (g *rectGeom) Subset(rows []int) Geom {
	return &rectGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *rectGeom) Describe() Desc {
	d := g.cfg.describe(MarkRect)
	d.Source = g.src
	return d
}

var (
	_ Describer = (*rectGeom)(nil)
	_ Faceter   = (*rectGeom)(nil)
	_ Guided    = (*rectGeom)(nil)
	_ Legender  = (*rectGeom)(nil)
)
