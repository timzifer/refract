package geom

import (
	"fmt"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// Sankey draws an edge list as a flow: nodes in columns, and a band between
// them as thick as what it carries.
//
//	geom.Sankey(src, geom.From("stage"), geom.To("next"), geom.Value("units"))
//
// The table is one row per link — where it comes from, where it goes and how
// much. Nodes are not declared anywhere: a node exists because a row mentioned
// it, and the order they are first mentioned in is the order they stack in. See
// [From].
//
// # What it decides
//
// A node stands one column past the deepest source that reaches it, so a flow
// reads left to right and a stage never sits before something feeding it. Its
// thickness is the greater of what enters and what leaves — a node that loses
// some of what it is given is as thick as the larger side, and the shortfall
// shows as the part of its edge no band leaves from.
//
// Within a column, nodes keep the order their rows gave them and only their
// positions are relaxed, [stat.SankeySweeps] times, towards the middle of what
// they are joined to. Reordering them to reduce crossings would mean a sort per
// sweep, and a sort is where a layout stops being a pure function of its input
// and starts depending on how a tie was broken. [Order] is how a caller asks
// for a different one — by sorting its own rows.
//
// # Cycles
//
// An edge list that runs in a circle is refused with [ErrCyclic] rather than
// drawn: a flow that returns to where it came from has no column to stand in,
// and picking one would draw an ordering nobody wrote.
//
// Both axes describe the unit square, so give the chart a theme with no grid,
// no axis lines and no ticks.
func Sankey(src data.Source, opts ...Option) Geom {
	return &sankeyGeom{src: src, cfg: newConfig(opts)}
}

// sankeyNode is the fraction of the plot's width a column of nodes fills when
// the layer did not say. It is a landmark rather than a mark to be measured, so
// it is narrow — and it shrinks further once there are enough columns that a
// fixed width would crowd them.
const sankeyNode = 0.04

// sankeyLinkOpacity is what a band is painted at when the layer named no
// opacity: bands overlap constantly, and an opaque one hides the ones behind it.
const sankeyLinkOpacity = 0.45

type sankeyGeom struct {
	src data.Source
	cfg config
	e   edges
	lay stat.Sankey
	err error
}

func (g *sankeyGeom) Train(x, y scale.Scale) error {
	if g.err = g.e.reset(g.src, g.cfg); g.err != nil {
		return g.err
	}
	if g.err = trainUnit(x, y); g.err != nil {
		return g.err
	}
	g.lay.Reset(g.e.from, g.e.to, g.e.val, g.e.count(), g.cfg.padding)
	if g.lay.Cyclic {
		g.err = fmt.Errorf("%w: a sankey reads a flow, and this one returns to where it started", ErrCyclic)
		return g.err
	}
	g.cfg.trainColors(g.e.s)
	return nil
}

// width is how wide a column of nodes is, in the unit square.
func (g *sankeyGeom) width() float64 {
	w := g.cfg.thickness
	if !(w > 0) {
		w = sankeyNode
		if room := 0.5 / float64(max(g.lay.Layers, 1)); w > room {
			w = room
		}
	}
	return min(w, 1)
}

// columnAt is the near edge of column l, spaced so the first column starts at
// the left of the plot and the last one ends at its right.
func (g *sankeyGeom) columnAt(l int, w float64) float64 {
	if g.lay.Layers <= 1 {
		return (1 - w) / 2
	}
	return float64(l) * (1 - w) / float64(g.lay.Layers-1)
}

func (g *sankeyGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	sc := acquire(f)
	defer sc.release()
	cd := f.Coords()
	w := g.width()

	// The bands go down first and the nodes on top of them, which is both the
	// right picture — a node is a landmark and should not be hidden by what
	// passes it — and the right hit test: a pointer on a node reports the node,
	// because the later mark wins a tie. See docs/adr/0015-hit-testing.md.
	g.bands(b, sc, cd, f, w)
	return g.nodes(b, sc, cd, f, w)
}

func (g *sankeyGeom) nodes(b ir.Backend, sc *scratch, cd coord.Coord, f Frame, w float64) error {
	rects := sc.rects[:0]
	rows := sc.rows[:0]
	for i, n := range g.lay.Nodes {
		if !(n.Hi > n.Lo) {
			continue
		}
		x := g.columnAt(n.Layer, w)
		rects = append(rects, ir.R(f.X.Map(x), f.Y.Map(n.Lo), f.X.Map(x+w), f.Y.Map(n.Hi)))
		rows = append(rows, i)
	}
	sc.rects, sc.rows = rects, rows
	if len(rects) == 0 {
		return nil
	}
	// A node is not a row: it is what several rows have in common, so it
	// reports none. The bands are the rows, and they were reported below.
	return fillBoxes(b, sc, cd, g.cfg, f, series{}, g.e.keys, rects, rows)
}

func (g *sankeyGeom) bands(b ir.Backend, sc *scratch, cd coord.Coord, f Frame, w float64) {
	opacity := sankeyLinkOpacity
	if g.cfg.opacity >= 0 {
		opacity = g.cfg.opacity
	}
	cols := sc.colorsFor(g.cfg, g.e.s, indexes(sc, len(g.lay.Flows)))
	pts := sc.pts[:0]
	mrows := sc.mrows[:0]

	for e, fl := range g.lay.Flows {
		if !(fl.SrcHi > fl.SrcLo) {
			continue
		}
		src, dst := g.e.from[e], g.e.to[e]
		x0 := g.columnAt(g.lay.Nodes[src].Layer, w) + w
		x1 := g.columnAt(g.lay.Nodes[dst].Layer, w)

		col := g.cfg.nodeColor(f, g.e.keys, src)
		if cols != nil {
			col = cols[e]
		}
		col = g.cfg.fillOf(col, opacity)
		if col.A == 0 {
			continue
		}
		sc.fill.Reset()
		ribbon(&sc.fill, cd, f, x0, x1, fl.SrcLo, fl.SrcHi, fl.DstLo, fl.DstHi)
		b.FillPath(&sc.fill, ir.Solid(col), ir.NonZero)

		if sc.wantRows {
			pts = append(pts, cd.Point(f.X.Map((x0+x1)/2), f.Y.Map((fl.SrcLo+fl.SrcHi+fl.DstLo+fl.DstHi)/4)))
			mrows = append(mrows, g.e.s.rowAt(e))
		}
	}
	sc.pts, sc.mrows = pts, mrows
	f.Marks(pts, mrows)
}

// ribbon appends the closed band between two columns: an S-curve down one side,
// across the far node's edge, and an S-curve back.
//
// The two curves are cubics whose control points sit halfway between the
// columns, which is what makes the band leave and arrive horizontally — a band
// that met a node at an angle would read as though the quantity changed there.
func ribbon(p *ir.Path, cd coord.Coord, f Frame, x0, x1, srcLo, srcHi, dstLo, dstHi float64) {
	at := func(x, y float64) ir.Point { return cd.Point(f.X.Map(x), f.Y.Map(y)) }
	mid := (x0 + x1) / 2

	a := at(x0, srcLo)
	d := at(x1, dstLo)
	p.MoveTo(a.X, a.Y)
	c1, c2 := at(mid, srcLo), at(mid, dstLo)
	p.CubicTo(c1.X, c1.Y, c2.X, c2.Y, d.X, d.Y)

	e := at(x1, dstHi)
	cd.Edge(p, d, e)

	c1, c2 = at(mid, dstHi), at(mid, srcHi)
	h := at(x0, srcHi)
	p.CubicTo(c1.X, c1.Y, c2.X, c2.Y, h.X, h.Y)

	cd.Edge(p, h, a)
	p.Close()
}

// indexes is 0..n-1, which is what colorsFor wants when the elements are the
// rows themselves rather than a subset of them.
func indexes(sc *scratch, n int) []int {
	sc.keep = grow(sc.keep, n)
	for i := range n {
		sc.keep[i] = i
	}
	return sc.keep
}

func (g *sankeyGeom) ColorGuide() (ColorGuide, bool) { return g.cfg.colorGuide(g.e.s, g.err) }

func (g *sankeyGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.nodeLegends(f, g.e.keys))
}

func (g *sankeyGeom) Legend(f Frame) (LegendEntry, bool) {
	return oneNodeLegend(g.cfg, f, g.err)
}

func (g *sankeyGeom) Source() data.Source { return g.src }

func (g *sankeyGeom) Subset(rows []int) Geom {
	return &sankeyGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *sankeyGeom) Describe() Desc {
	d := g.cfg.describe(MarkSankey)
	d.Source = g.src
	return d
}

var (
	_ Describer = (*sankeyGeom)(nil)
	_ Faceter   = (*sankeyGeom)(nil)
	_ Guided    = (*sankeyGeom)(nil)
	_ Legender  = (*sankeyGeom)(nil)
)
