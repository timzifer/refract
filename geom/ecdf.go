package geom

import (
	"sort"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// ECDF draws the empirical cumulative distribution of the X column: a staircase
// rising from 0 to 1, one step per distinct observation.
//
// It is the distribution plot with no parameter in it. A histogram picks bin
// edges and a violin picks a bandwidth, and both choices change the picture, so
// two of them drawn together compare two smoothing decisions as much as they
// compare two datasets. An ECDF has nothing to choose: the curve is the data.
// What it costs is the shape — a reader sees medians and tails clearly and
// bimodality hardly at all, which is why this and [Violin] are both here.
//
// The Y axis is the layer's own, running 0 to 1, so the Y column is not read.
// Given [GroupBy] it draws one staircase per series, which is what the mark is
// most useful for: several distributions on one pair of axes, none of them
// hiding another.
func ECDF(src data.Source, opts ...Option) Geom {
	return &ecdfGeom{src: src, cfg: newConfig(opts)}
}

type ecdfGeom struct {
	src    data.Source
	cfg    config
	s      series
	gs     groups
	curves [][]stat.Point
	groups []int     // which series each curve belongs to, parallel to curves
	vals   []float64 // the buffer one series' values are sorted in
	all    []int     // the identity row list, for an ungrouped layer
	err    error
}

func (g *ecdfGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolveOne(g.src, g.cfg, x)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, x); err != nil {
		return err
	}
	// The series are split on the X scale for both axes: what makes a row
	// usable here is that its *observation* has a position, and the Y axis
	// holds a fraction this layer computes rather than anything from the table.
	if g.err = g.gs.train(g.src, g.s, g.cfg, x, x, NoStack); g.err != nil {
		return g.err
	}
	g.accumulate(x)

	trainColumn(x, g.s.x)
	// The axis runs the whole way, whatever the sample: an ECDF that stopped at
	// 0.97 because no observation reached the top would read as a distribution
	// with something missing from it.
	y.Train(0, 1)
	return nil
}

func (g *ecdfGeom) accumulate(x scale.Scale) {
	n := max(len(g.gs.keys), 1)
	g.curves = growCurves(g.curves, n)[:0]
	g.groups = grow(g.groups, n)[:0]

	for _, grp := range g.seriesOf() {
		g.vals = g.vals[:0]
		for _, i := range g.rowsOfSeries(grp) {
			if defined(x, g.s.x[i]) {
				g.vals = append(g.vals, g.s.x[i])
			}
		}
		if len(g.vals) == 0 {
			continue
		}
		sort.Float64s(g.vals)

		var curve []stat.Point
		if i := len(g.curves); i < cap(g.curves) {
			curve = g.curves[:i+1][i]
		}
		g.curves = append(g.curves, stat.AppendECDF(curve, g.vals))
		g.groups = append(g.groups, grp)
	}
}

func (g *ecdfGeom) seriesOf() []int {
	if !g.gs.grouped() {
		return oneSeries
	}
	return g.gs.order
}

// rowsOfSeries lists the rows of one series, or every row for a layer with no
// groups. The ungrouped list is built lazily and kept, so a chart redrawn every
// frame does not allocate one.
func (g *ecdfGeom) rowsOfSeries(grp int) []int {
	if g.gs.grouped() {
		return g.gs.rows[grp]
	}
	g.all = grow(g.all, len(g.s.x))
	for i := range g.s.x {
		g.all[i] = i
	}
	return g.all
}

func (g *ecdfGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	sc := acquire(f)
	defer sc.release()

	cd := f.Coords()
	for i, curve := range g.curves {
		col := g.cfg.colorFor(f)
		dash := g.cfg.dashFor(f)
		if g.gs.grouped() {
			col, dash = g.cfg.groupColor(f, &g.gs, g.groups[i]), g.cfg.groupDash(f, g.groups[i])
		}
		g.staircase(b, f, sc, cd, curve, ir.Stroke{
			Color: col,
			Width: pick(g.cfg.width, f.Theme.LineWidth),
			Cap:   ir.CapButt,
			Join:  ir.JoinMiter,
			Dash:  dash,
		})
	}
	return nil
}

// staircase draws one curve as the step function it is.
//
// Two points per observation: along to the next value at the fraction so far,
// then up to the new fraction. Drawing it as a polyline through the (value,
// fraction) pairs instead would slope between the steps, which claims
// observations between two measurements that were never made — the same reason
// [Step] exists beside [Line].
func (g *ecdfGeom) staircase(b ir.Backend, f Frame, sc *scratch, cd coord.Coord, curve []stat.Point, stroke ir.Stroke) {
	if len(curve) == 0 || !stroke.Visible() {
		return
	}
	n := 2 * len(curve)
	sc.sx, sc.sy = grow(sc.sx, n)[:0], grow(sc.sy, n)[:0]
	prev := 0.0
	for _, p := range curve {
		at := f.X.Map(p.X)
		sc.sx = append(sc.sx, at, at)
		sc.sy = append(sc.sy, f.Y.Map(prev), f.Y.Map(p.Y))
		prev = p.Y
	}
	pts := cd.Points(grow(sc.pts, n)[:0], sc.sx, sc.sy)
	sc.pts = pts
	strokeRun(b, cd, &sc.line, pts, stroke, false)
}

func (g *ecdfGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.legends(f, &g.gs, g.s, SwatchLine))
}

func (g *ecdfGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil {
		return LegendEntry{}, false
	}
	return LegendEntry{
		Label: g.cfg.labelForX(),
		Color: g.cfg.colorFor(f),
		Kind:  SwatchLine,
		Dash:  g.cfg.dashFor(f),
		Width: pick(g.cfg.width, f.Theme.LineWidth),
	}, true
}

func (g *ecdfGeom) Source() data.Source { return g.src }
func (g *ecdfGeom) Subset(rows []int) Geom {
	return &ecdfGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *ecdfGeom) Describe() Desc {
	d := g.cfg.describe(MarkECDF)
	d.Source = g.src
	return d
}

// growCurves grows a slice of point lists to n entries, reusing every one it
// already has.
func growCurves(buf [][]stat.Point, n int) [][]stat.Point {
	if cap(buf) >= n {
		return buf[:n]
	}
	return append(buf[:cap(buf)], make([][]stat.Point, n-cap(buf))...)
}

var (
	_ Describer = (*ecdfGeom)(nil)
	_ Faceter   = (*ecdfGeom)(nil)
	_ Legender  = (*ecdfGeom)(nil)
)
