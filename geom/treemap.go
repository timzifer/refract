package geom

import (
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// Treemap draws a hierarchy as nested rectangles, each with an area
// proportional to its value.
//
//	geom.Treemap(src, geom.ID("path"), geom.Parent("under"), geom.Value("bytes"))
//
// The table is one row per node — the name it is known by, the name of the node
// above it, and its magnitude. Usually only the leaves carry a number, and an
// internal node's size is what is under it; see [Value].
//
// # What is drawn
//
// The leaves, and only the leaves. An internal node's rectangle is exactly the
// union of its children's, so painting it would be painting underneath paint;
// what makes the nesting visible is [Padding], which insets a node's box before
// its children are packed into it. A node carrying a value of its own beyond
// its children's keeps the remainder as empty room inside its box, which is
// what makes an unaccounted-for share something a reader can see.
//
// # Squarified, and against the panel
//
// The packing is [stat.Squarify]: tiles are gathered into rows for as long as
// adding one improves the row's worst aspect ratio. It runs against the plot
// rectangle rather than against the unit square, because what it optimises is a
// shape *on screen* — packing a square and then stretching it into a wide panel
// would defeat the whole algorithm. That makes it the third stat to run in
// Build rather than in Train, beside [Hexbin]'s lattice and [Beeswarm]'s
// offsets, and for the same reason: its answer is a length the reader sees
// rather than a number the axis has to describe. See
// docs/adr/0028-distribution-stats.md.
//
// Siblings are packed in the order their rows appear. [Order] with
// [OrderValue] is how to ask for the largest first, which gives the squarest
// tiles; source order is what keeps the picture stable as the numbers move.
//
// Both axes describe the unit square and there is nothing in that for a reader
// to see: give the chart a theme with no grid, no axis lines and no ticks.
func Treemap(src data.Source, opts ...Option) Geom {
	return &treemapGeom{src: src, cfg: newConfig(opts)}
}

type treemapGeom struct {
	src   data.Source
	cfg   config
	t     tree
	kids  []int // every node's children, gathered so that a parent's are contiguous
	first []int // where each parent's run of children starts in kids
	depth int
	err   error
}

func (g *treemapGeom) Train(x, y scale.Scale) error {
	if g.err = g.t.reset(g.src, g.cfg); g.err != nil {
		return g.err
	}
	if g.err = trainUnit(x, y); g.err != nil {
		return g.err
	}
	g.t.depth = stat.AppendDepth(g.t.depth, g.t.parent)
	g.t.total = stat.AppendRollup(g.t.total, g.t.val, g.t.parent, g.t.depth)
	g.gather()
	g.cfg.trainColors(g.t.s)
	return nil
}

// gather sorts every node under its parent, so that one sibling group is a
// contiguous run and the packing below needs no search.
//
// It is a counting sort over the parent list, which is a pass to count, a pass
// to total and a pass to place — and it keeps the siblings in the order their
// rows appeared, because a counting sort is stable and that order is what the
// layout is entitled to (docs/adr/0012-parallel-panels.md). Bucket zero is the
// roots; bucket p+1 is node p's children.
func (g *treemapGeom) gather() {
	n := len(g.t.parent)
	g.first = grow(g.first, n+2)
	for i := range g.first {
		g.first[i] = 0
	}
	for _, p := range g.t.parent {
		g.first[p+2]++
	}
	for i := 1; i < len(g.first); i++ {
		g.first[i] += g.first[i-1]
	}
	g.kids = grow(g.kids, n)
	// Place each node at the front of its bucket's unfilled part, walking the
	// offset forwards as it goes.
	for i := range n {
		b := g.t.parent[i] + 1
		g.kids[g.first[b]] = i
		g.first[b]++
	}
	// first[b] now points one past bucket b's run, which is where bucket b+1
	// starts — so shifting it back by one gives the starts again.
	for b := len(g.first) - 1; b > 0; b-- {
		g.first[b] = g.first[b-1]
	}
	g.first[0] = 0

	g.depth = 0
	for _, d := range g.t.depth {
		if d > g.depth {
			g.depth = d
		}
	}
}

// childrenOf is node i's children, or the roots for i == -1.
func (g *treemapGeom) childrenOf(i int) []int {
	b := i + 1
	if b+1 >= len(g.first) {
		return nil
	}
	return g.kids[g.first[b]:g.first[b+1]]
}

func (g *treemapGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	sc := acquire(f)
	defer sc.release()
	cd := f.Coords()

	n := len(g.t.parent)
	sc.tiles = grow(sc.tiles, n)

	// The root box is the unit square as the scales map it, so a zoomed chart
	// packs into what is on screen rather than into what used to be.
	x0, x1 := float64(f.X.Map(0)), float64(f.X.Map(1))
	y0, y1 := float64(f.Y.Map(0)), float64(f.Y.Map(1))
	gapX, gapY := g.cfg.padding*abs(x1-x0), g.cfg.padding*abs(y1-y0)

	g.packInto(sc, -1, x0, y0, x1, y1, gapX, gapY)
	for d := range g.depth {
		for i := range n {
			if g.t.depth[i] != d {
				continue
			}
			t := sc.tiles[i]
			g.packInto(sc, i, t.X0, t.Y0, t.X1, t.Y1, gapX, gapY)
		}
	}

	rects := sc.rects[:0]
	rows := sc.rows[:0]
	for i := range n {
		if g.t.depth[i] < 0 || len(g.childrenOf(i)) > 0 {
			continue
		}
		t := sc.tiles[i]
		cx0, cy0, cx1, cy1 := padBox(t, gapX, gapY)
		if !(cx1 > cx0) || !(cy1 > cy0) {
			continue
		}
		rects = append(rects, ir.R(float32(cx0), float32(cy0), float32(cx1), float32(cy1)))
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

// packInto squarifies one node's children into the box it was given.
func (g *treemapGeom) packInto(sc *scratch, node int, x0, y0, x1, y1, gapX, gapY float64) {
	kids := g.childrenOf(node)
	if len(kids) == 0 {
		return
	}
	if node >= 0 {
		// A group is packed inside its parent's box less the gap, which is what
		// makes the nesting visible without drawing the parent.
		x0, x1 = shrink(x0, x1, gapX)
		y0, y1 = shrink(y0, y1, gapY)
	}
	sc.gx = grow(sc.gx, len(kids))
	for k, i := range kids {
		sc.gx[k] = g.t.total[i]
	}
	sc.pack = stat.AppendSquarify(sc.pack, sc.gx, x0, y0, x1, y1)
	for k, i := range kids {
		sc.tiles[i] = sc.pack[k]
	}
}

func padBox(t stat.Tile, gapX, gapY float64) (float64, float64, float64, float64) {
	x0, x1 := shrink(t.X0, t.X1, gapX)
	y0, y1 := shrink(t.Y0, t.Y1, gapY)
	return x0, y0, x1, y1
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func (g *treemapGeom) ColorGuide() (ColorGuide, bool) { return g.cfg.colorGuide(g.t.s, g.err) }

func (g *treemapGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.nodeLegends(f, g.t.keys))
}

func (g *treemapGeom) Legend(f Frame) (LegendEntry, bool) {
	return oneNodeLegend(g.cfg, f, g.err)
}

func (g *treemapGeom) Source() data.Source { return g.src }

func (g *treemapGeom) Subset(rows []int) Geom {
	return &treemapGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *treemapGeom) Describe() Desc {
	d := g.cfg.describe(MarkTreemap)
	d.Source = g.src
	return d
}

var (
	_ Describer = (*treemapGeom)(nil)
	_ Faceter   = (*treemapGeom)(nil)
	_ Guided    = (*treemapGeom)(nil)
	_ Legender  = (*treemapGeom)(nil)
)
