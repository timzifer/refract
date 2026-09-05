package geom

import (
	"math"
	"sort"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// Trend fits a smooth line through the X and Y columns and draws it.
//
// It is the layer that goes on top of a scatter: the cloud says what was
// measured, the trend says what it is doing. [Smooth] chooses how — [Loess],
// the default, fits a line through the neighbours of each abscissa and follows
// the data; [LinearFit] fits one straight line by least squares, which is the
// right mark when the claim being made is that the relationship *is* linear.
// [Span] sets how many neighbours a local fit sees.
//
// Given [GroupBy] it fits one line per series, which is the comparison the mark
// is usually added for: two clouds with two trends through them.
//
// # Where the fitting happens
//
// In Train, in data space — so the axis is trained on the fit as well as on the
// observations, and a curve that runs a little past the data is inside the plot
// rather than clipped at its edge. That is the opposite choice from decimation
// (ADR 0011), and for the same underlying reason: a reduction must not change
// what the axis says, and a *fit* is part of what the axis has to describe.
func Trend(src data.Source, opts ...Option) Geom {
	return &trendGeom{src: src, cfg: newConfig(opts)}
}

type trendGeom struct {
	src    data.Source
	cfg    config
	s      series
	gs     groups
	fits   [][]stat.Point
	groups []int // which series each fit belongs to, parallel to fits
	xs, ys []float64
	order  []int
	all    []int
	err    error
}

func (g *trendGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	if g.err = g.gs.train(g.src, g.s, g.cfg, x, y, NoStack); g.err != nil {
		return g.err
	}
	g.fit(x, y)

	trainColumn(x, g.s.x)
	trainColumn(y, g.s.y)
	for _, f := range g.fits {
		for _, p := range f {
			y.Train(p.Y)
		}
	}
	return nil
}

// fit builds one curve per series.
//
// The rows of a series are gathered and sorted by X first, because
// [stat.AppendLoess] takes an ascending, finite pair of columns — a narrower
// contract than the rest of stat, so that the sorting buffer belongs to the
// layer rather than being allocated inside the fit on every frame.
func (g *trendGeom) fit(x, y scale.Scale) {
	ok := g.s.plottable(x, y)
	n := max(len(g.gs.keys), 1)
	g.fits = growCurves(g.fits, n)[:0]
	g.groups = grow(g.groups, n)[:0]

	for _, grp := range g.seriesOf() {
		rows := g.rowsOfSeries(grp)
		g.order = grow(g.order, len(rows))[:0]
		for _, i := range rows {
			if ok[i] {
				g.order = append(g.order, i)
			}
		}
		if len(g.order) < 2 {
			continue
		}
		sort.SliceStable(g.order, func(a, b int) bool { return g.s.x[g.order[a]] < g.s.x[g.order[b]] })

		g.xs, g.ys = grow(g.xs, len(g.order))[:0], grow(g.ys, len(g.order))[:0]
		for _, i := range g.order {
			g.xs = append(g.xs, g.s.x[i])
			g.ys = append(g.ys, g.s.y[i])
		}

		var curve []stat.Point
		if i := len(g.fits); i < cap(g.fits) {
			curve = g.fits[:i+1][i]
		}
		g.fits = append(g.fits, g.fitOne(curve))
		g.groups = append(g.groups, grp)
	}
}

// fitOne runs the layer's chosen fit over the gathered columns.
func (g *trendGeom) fitOne(dst []stat.Point) []stat.Point {
	if g.cfg.smooth == LinearFit {
		return appendLinearFit(dst, g.xs, g.ys)
	}
	return stat.AppendLoess(dst, g.xs, g.ys, g.cfg.span, 0)
}

// appendLinearFit is ordinary least squares, as two points.
//
// Two points and not sixty-four: a straight line is exactly determined by its
// ends, and sampling it would emit sixty-two vertices that carry no information
// and that a reader could mistake for the fit having been evaluated somewhere.
func appendLinearFit(dst []stat.Point, xs, ys []float64) []stat.Point {
	dst = dst[:0]
	n := float64(len(xs))
	if n < 2 {
		return dst
	}
	var sx, sy, sxx, sxy float64
	for i, v := range xs {
		sx += v
		sy += ys[i]
		sxx += v * v
		sxy += v * ys[i]
	}
	den := n*sxx - sx*sx
	if den == 0 || math.IsNaN(den) {
		return dst
	}
	slope := (n*sxy - sx*sy) / den
	intercept := (sy - slope*sx) / n
	lo, hi := xs[0], xs[len(xs)-1]
	return append(dst,
		stat.Point{X: lo, Y: intercept + slope*lo},
		stat.Point{X: hi, Y: intercept + slope*hi})
}

func (g *trendGeom) seriesOf() []int {
	if !g.gs.grouped() {
		return oneSeries
	}
	return g.gs.order
}

func (g *trendGeom) rowsOfSeries(grp int) []int {
	if g.gs.grouped() {
		return g.gs.rows[grp]
	}
	g.all = grow(g.all, len(g.s.x))
	for i := range g.s.x {
		g.all[i] = i
	}
	return g.all
}

func (g *trendGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	sc := acquire(f)
	defer sc.release()

	cd := f.Coords()
	tension := float32(clamp01(g.cfg.tension))
	for i, curve := range g.fits {
		col, dash := g.cfg.colorFor(f), g.cfg.dashFor(f)
		if g.gs.grouped() {
			col, dash = g.cfg.groupColor(f, &g.gs, g.groups[i]), g.cfg.groupDash(f, g.groups[i])
		}
		stroke := ir.Stroke{
			Color: col,
			Width: pick(g.cfg.width, f.Theme.LineWidth),
			Cap:   ir.CapRound,
			Join:  ir.JoinRound,
			Dash:  dash,
		}
		if !stroke.Visible() || len(curve) < 2 {
			continue
		}
		sc.kx, sc.ky = grow(sc.kx, len(curve))[:0], grow(sc.ky, len(curve))[:0]
		for _, p := range curve {
			sc.kx = append(sc.kx, f.X.Map(p.X))
			sc.ky = append(sc.ky, f.Y.Map(p.Y))
		}
		pts := cd.Points(grow(sc.pts, len(curve))[:0], sc.kx, sc.ky)
		sc.pts = pts
		if tension > 0 {
			sc.line.Reset()
			appendCurve(&sc.line, cd, pts, tension, true)
			b.StrokePath(&sc.line, stroke)
			continue
		}
		strokeRun(b, cd, &sc.line, pts, stroke, false)
	}
	return nil
}

func (g *trendGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.legends(f, &g.gs, g.s, SwatchLine))
}

func (g *trendGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil {
		return LegendEntry{}, false
	}
	return LegendEntry{
		Label: g.cfg.labelFor(),
		Color: g.cfg.colorFor(f),
		Kind:  SwatchLine,
		Dash:  g.cfg.dashFor(f),
		Width: pick(g.cfg.width, f.Theme.LineWidth),
	}, true
}

func (g *trendGeom) Source() data.Source { return g.src }
func (g *trendGeom) Subset(rows []int) Geom {
	return &trendGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *trendGeom) Describe() Desc {
	d := g.cfg.describe(MarkTrend)
	d.Source = g.src
	return d
}

var (
	_ Describer = (*trendGeom)(nil)
	_ Faceter   = (*trendGeom)(nil)
	_ Legender  = (*trendGeom)(nil)
)
