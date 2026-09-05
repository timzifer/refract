package geom

import (
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Bar draws a rectangle per row, from a baseline to the row's Y value.
//
// Given [GroupBy] it draws one rectangle per row per series, stacked from the
// baseline up: a long table with a series column is a stacked bar chart, and
// [Stack] and [Dodge] are how it becomes a 100 % chart or a grouped one
// instead. See docs/adr/0019-position-adjustments.md.
func Bar(src data.Source, opts ...Option) Geom {
	return &barGeom{src: src, cfg: newConfig(opts)}
}

type barGeom struct {
	src   data.Source
	cfg   config
	s     series
	width []float64
	gs    groups
	err   error
}

func (g *barGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	if g.cfg.widthCol != "" {
		g.width, g.err = column(g.src, g.cfg.widthCol, nil)
		if g.err != nil {
			return g.err
		}
		if len(g.width) != len(g.s.x) {
			g.err = errLength(g.cfg.xcol, g.cfg.widthCol, len(g.s.x), len(g.width))
			return g.err
		}
	}
	trainColumn(x, g.s.x)
	g.cfg.trainColors(g.s)

	// The adjustment is derived here, before anything is measured, because the
	// axis has to describe what will be drawn: a stacked bar reaches the
	// cumulative total, and an axis trained on the individual values would let
	// the tallest stack run off the top of it.
	if g.err = g.gs.train(g.src, g.s, g.cfg, x, y, g.cfg.stackFor(StackZero)); g.err != nil {
		return g.err
	}
	if !g.gs.stacked() {
		trainColumn(y, g.s.y)
		// A bar is read as the area between the baseline and the value, so the
		// baseline must be in the domain or the chart lies about magnitude.
		y.Train(g.cfg.baseline)
	}

	// A band scale already reserves a slot per bar, so widening its domain
	// would only add an empty category at each end.
	if _, band := x.(scale.Band); band {
		return nil
	}

	// On a continuous axis bars have width, so the outermost bars would be
	// clipped in half by a domain that stops at the data. Widen by half a slot
	// on each side.
	widen(x, g.s.x, g.slot()/2*g.widthFraction())
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

// halfWidth is how far row i's bar reaches on each side of its position, in
// data units. [WidthBy] answers it per row; otherwise every bar is the same
// fraction of the closest spacing in the data.
func (g *barGeom) halfWidth(i int) float64 {
	if g.width != nil && finite(g.width[i]) && g.width[i] > 0 {
		return g.width[i] / 2
	}
	return g.slot() * g.widthFraction() / 2
}

func (g *barGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	fill := g.cfg.colorFor(f)
	if g.cfg.fill != nil {
		fill = *g.cfg.fill
	}
	if fill.A == 0 && !g.gs.grouped() && !g.cfg.varying(g.s) {
		return nil
	}

	sc := acquire(f)
	defer sc.release()

	cd := f.Coords()
	base := baselinePos(f, g.cfg.baseline)
	// A stacked layer draws the bounds the adjustment gave it rather than the
	// column, and every traversal below — including which rows are holes —
	// then reads the adjusted series.
	s := g.gs.bounds(g.s)
	ok := sc.plottable(s, f.X, f.Y)

	// Collect the bars first, so that a layer coloured from a scale can batch
	// them by colour and one coloured uniformly can still emit a single path.
	//
	// A bar already aggregates whatever it counted, so there is no reduction to
	// apply here: dropping bars would drop categories rather than pixels.
	rects := sc.rects[:0]
	rows := sc.rows[:0]
	for i := range s.x {
		if !ok[i] {
			continue
		}
		x0, x1 := markSpan(f, s.x[i], g.halfWidth(i))
		if g.cfg.dodge {
			x0, x1 = dodgeSpan(x0, x1, g.gs.slotIndex(i), g.gs.count(), g.cfg.dodgePad)
		}
		y0, y1 := f.Y.Map(s.y[i]), base
		if s.y2 != nil {
			y1 = f.Y.Map(s.y2[i])
		}
		if y1 < y0 {
			y0, y1 = y1, y0
		}
		// The pair is the mark's extent in the space the scales map into. Under
		// a Cartesian coord that is the rectangle it draws; under a polar one
		// it is the angles and radii the coord turns into an annular sector.
		rects = append(rects, ir.R(x0, y0, x1, y1))
		rows = append(rows, i)
	}
	sc.rects, sc.rows = rects, rows
	if len(rects) == 0 {
		return nil
	}
	// A bar's row is at the middle of the end it grew to, which is where a
	// reader points when they mean "this bar" — not at a corner, and not at
	// the middle of a shape whose height is the value. A stacked segment is
	// bounded at both ends, so its row is in the middle of it: neither end is
	// the value.
	if f.tracking() {
		sc.pts = grow(sc.pts, len(rects))
		for i, r := range rects {
			at := barTop(r, base)
			if s.y2 != nil {
				at = (r.Min.Y + r.Max.Y) / 2
			}
			// The position is worked out in the space the scales map into and
			// placed by the coord, so a slice of a pie reports the middle of
			// its arc rather than a point the transform never visited.
			sc.pts[i] = cd.Point((r.Min.X+r.Max.X)/2, at)
		}
		f.Marks(sc.pts, sc.sourceRows(g.s, rows))
	}

	// A grouped layer is painted by series, and every segment is its own
	// subpath so that a pointer lands on the segment rather than on the stack.
	if g.gs.grouped() {
		for _, run := range sc.groupRuns(&g.gs, rects, rows) {
			col := g.cfg.groupColor(f, &g.gs, run.group)
			if col.A == 0 {
				continue
			}
			sc.fill.Reset()
			for _, r := range run.rects {
				area(&sc.fill, cd, r)
			}
			b.FillPath(&sc.fill, ir.Solid(col), ir.NonZero)
		}
		return nil
	}

	if cols := sc.colorsFor(g.cfg, g.s, rows); cols != nil {
		for i, r := range rects {
			if cols[i].A == 0 {
				continue
			}
			sc.fill.Reset()
			area(&sc.fill, cd, r)
			b.FillPath(&sc.fill, ir.Solid(cols[i]), ir.NonZero)
		}
		return nil
	}

	sc.fill.Reset()
	for _, r := range rects {
		area(&sc.fill, cd, r)
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

func (g *barGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.legends(f, &g.gs, g.s, SwatchBox))
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
