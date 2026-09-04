package geom

import (
	"math"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Bar draws a rectangle per row, from a baseline to the row's Y value.
func Bar(src data.Source, opts ...Option) Geom {
	return &barGeom{src: src, cfg: newConfig(opts)}
}

type barGeom struct {
	src data.Source
	cfg config
	s   series
	err error
}

func (g *barGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	trainColumn(x, g.s.x)
	trainColumn(y, g.s.y)
	g.cfg.trainColors(g.s)
	// A bar is read as the area between the baseline and the value, so the
	// baseline must be in the domain or the chart lies about magnitude.
	y.Train(g.cfg.baseline)

	// A band scale already reserves a slot per bar, so widening its domain
	// would only add an empty category at each end.
	if _, band := x.(scale.Band); band {
		return nil
	}

	// On a continuous axis bars have width, so the outermost bars would be
	// clipped in half by a domain that stops at the data. Widen by half a slot
	// on each side.
	half := g.slot() / 2 * g.widthFraction()
	if half > 0 {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, v := range g.s.x {
			if finite(v) {
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
		}
		if !math.IsInf(lo, 0) {
			x.Train(lo-half, hi+half)
		}
	}
	return nil
}

func (g *barGeom) widthFraction() float64 {
	if g.cfg.barWidth <= 0 || g.cfg.barWidth > 1 {
		return 0.8
	}
	return g.cfg.barWidth
}

// slot is the spacing between adjacent bars in data units.
func (g *barGeom) slot() float64 { return smallestGap(g.s.x) }

func (g *barGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	fill := g.cfg.colorFor(f)
	if g.cfg.fill != nil {
		fill = *g.cfg.fill
	}
	if fill.A == 0 {
		return nil
	}

	sc := acquire(f)
	defer sc.release()

	base := baselinePos(f, g.cfg.baseline)
	ok := sc.plottable(g.s, f.X, f.Y)

	// Collect the bars first, so that a layer coloured from a scale can batch
	// them by colour and one coloured uniformly can still emit a single path.
	//
	// A bar already aggregates whatever it counted, so there is no reduction to
	// apply here: dropping bars would drop categories rather than pixels.
	rects := sc.rects[:0]
	rows := sc.rows[:0]
	for i := range g.s.x {
		if !ok[i] {
			continue
		}
		x0, x1 := markSpan(f, g.s.x[i], g.slot()*g.widthFraction()/2)
		y0, y1 := f.Y.Map(g.s.y[i]), base
		if y1 < y0 {
			y0, y1 = y1, y0
		}
		rects = append(rects, ir.R(x0, y0, x1, y1))
		rows = append(rows, i)
	}
	sc.rects, sc.rows = rects, rows
	if len(rects) == 0 {
		return nil
	}
	// A bar's row is at the middle of the end it grew to, which is where a
	// reader points when they mean "this bar" — not at a corner, and not at
	// the middle of a shape whose height is the value.
	if f.tracking() {
		sc.pts = grow(sc.pts, len(rects))
		for i, r := range rects {
			sc.pts[i] = ir.Point{X: (r.Min.X + r.Max.X) / 2, Y: barTop(r, base)}
		}
		f.Marks(sc.pts, sc.sourceRows(g.s, rows))
	}

	if cols := sc.colorsFor(g.cfg, g.s, rows); cols != nil {
		for i, r := range rects {
			if cols[i].A == 0 {
				continue
			}
			sc.fill.Reset()
			sc.fill.Rect(r)
			b.FillPath(&sc.fill, ir.Solid(cols[i]), ir.NonZero)
		}
		return nil
	}

	sc.fill.Reset()
	for _, r := range rects {
		sc.fill.Rect(r)
	}
	b.FillPath(&sc.fill, ir.Solid(fill), ir.NonZero)

	if g.cfg.fill != nil && g.cfg.color != nil {
		b.StrokePath(&sc.fill, ir.Stroke{Color: *g.cfg.color, Width: pick(g.cfg.width, 1)})
	}
	return nil
}

// barTop is the end of a bar that is not the baseline. A bar below the
// baseline grew downwards, so its value is at the bottom of the rectangle.
func barTop(r ir.Rect, base float32) float32 {
	if r.Min.Y >= base {
		return r.Max.Y
	}
	return r.Min.Y
}

func (g *barGeom) ColorGuide() (ColorGuide, bool) {
	return g.cfg.colorGuide(g.s, g.err)
}

func (g *barGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil || g.cfg.varying(g.s) {
		return LegendEntry{}, false
	}
	col := g.cfg.colorFor(f)
	if g.cfg.fill != nil {
		col = *g.cfg.fill
	}
	return LegendEntry{Label: g.cfg.labelFor(), Color: col, Kind: SwatchBox}, true
}
