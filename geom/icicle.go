package geom

import (
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// Icicle draws a hierarchy as bands: one per node, as wide as its share of the
// whole and as far out as it is deep.
//
//	geom.Icicle(src, geom.ID("path"), geom.Parent("under"), geom.Value("bytes"))
//
// The table is one row per node — the name it is known by, the name of the node
// above it, and its magnitude. Usually only the leaves carry a number; an
// internal node's size is what is under it, which [geom.Value] explains.
//
// # A sunburst is this mark under a polar coord
//
// The layout is a span across and a depth out, in the unit square, and the
// coordinate stage decides what that looks like. Left Cartesian it is an
// icicle, growing upward from a root along the bottom — the flame-graph
// orientation. Wrapped round a circle it is a sunburst, with the root at the
// middle:
//
//	p := refract.New(refract.Theme(bare), refract.Coord(coord.Polar()))
//	p.X(scale.Linear())  // the span, swept round the circle
//	p.Y(scale.Linear())  // the depth, read as the radius
//	p.Add(geom.Icicle(src, geom.ID("path"), geom.Parent("under"), geom.Value("bytes")))
//
// It is [coord.Polar] and not [coord.Pie]: a pie sweeps the *Y* axis round, and
// this mark's Y is its depth. [coord.Hole] leaves the middle empty, which is
// what a sunburst that does not want to draw its root wants.
//
// Both axes describe the unit square, which is nothing a reader needs to see:
// give the chart a theme with no grid, no axis lines and no ticks, exactly as a
// pie does.
func Icicle(src data.Source, opts ...Option) Geom {
	return &icicleGeom{src: src, cfg: newConfig(opts)}
}

type icicleGeom struct {
	src   data.Source
	cfg   config
	t     tree
	rings int
	err   error
}

func (g *icicleGeom) Train(x, y scale.Scale) error {
	if g.err = g.t.reset(g.src, g.cfg); g.err != nil {
		return g.err
	}
	if g.err = trainUnit(x, y); g.err != nil {
		return g.err
	}
	g.t.depth = stat.AppendDepth(g.t.depth, g.t.parent)
	g.t.total = stat.AppendRollup(g.t.total, g.t.val, g.t.parent, g.t.depth)
	g.t.lo, g.t.hi = stat.AppendPartition(g.t.lo, g.t.hi, g.t.total, g.t.parent, g.t.depth)

	g.rings = 1
	for _, d := range g.t.depth {
		if d+1 > g.rings {
			g.rings = d + 1
		}
	}
	g.cfg.trainColors(g.t.s)
	return nil
}

func (g *icicleGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	sc := acquire(f)
	defer sc.release()
	cd := f.Coords()

	ring := 1 / float64(g.rings)
	rects := sc.rects[:0]
	rows := sc.rows[:0]
	for i, d := range g.t.depth {
		if d < 0 || !(g.t.hi[i] > g.t.lo[i]) {
			continue
		}
		x0, y0, x1, y1 := padded(g.t.lo[i], float64(d)*ring, g.t.hi[i], float64(d+1)*ring, g.cfg.padding)
		rects = append(rects, ir.R(f.X.Map(x0), f.Y.Map(y0), f.X.Map(x1), f.Y.Map(y1)))
		rows = append(rows, i)
	}
	sc.rects, sc.rows = rects, rows
	if len(rects) == 0 {
		return nil
	}
	if f.tracking() {
		reportBoxes(sc, f, rects, rows, g.t.s)
	}
	return fillBoxes(b, sc, cd, g.cfg, f, g.t.s, g.t.keys, rects, rows)
}

func (g *icicleGeom) ColorGuide() (ColorGuide, bool) { return g.cfg.colorGuide(g.t.s, g.err) }

func (g *icicleGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.nodeLegends(f, g.t.keys))
}

func (g *icicleGeom) Legend(f Frame) (LegendEntry, bool) {
	return oneNodeLegend(g.cfg, f, g.err)
}

func (g *icicleGeom) Source() data.Source { return g.src }

func (g *icicleGeom) Subset(rows []int) Geom {
	return &icicleGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *icicleGeom) Describe() Desc {
	d := g.cfg.describe(MarkIcicle)
	d.Source = g.src
	return d
}

var (
	_ Describer = (*icicleGeom)(nil)
	_ Faceter   = (*icicleGeom)(nil)
	_ Guided    = (*icicleGeom)(nil)
	_ Legender  = (*icicleGeom)(nil)
)
