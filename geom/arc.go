package geom

import (
	"math"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// Arc draws an edge list as nodes on a rail with ribbons between them: an arc
// diagram.
//
//	geom.Arc(src, geom.From("a"), geom.To("b"), geom.Value("calls"))
//
// The table is one row per edge — its two ends and its weight. Nodes are not
// declared anywhere: a node exists because a row mentioned it, and it takes a
// share of the rail proportional to the traffic through it. An edge naming no
// weight counts as one, so an unweighted graph draws without a [Value] column.
//
// # A chord diagram is this mark under a polar coord
//
// The layout is a position along the rail and a height off it, in the unit
// square, and the coordinate stage decides what that means. Left Cartesian the
// nodes sit on a rail and the ribbons rise off it. Wrapped round a circle, with
// the rail moved to the rim, the ribbons cross the middle — which is a chord
// diagram:
//
//	p := refract.New(refract.Theme(bare), refract.Coord(coord.Polar()))
//	p.X(scale.Linear())  // the rail, swept round the circle
//	p.Y(scale.Linear())  // the height off it, read as the radius
//	p.Add(geom.Arc(src, geom.From("a"), geom.To("b"), geom.Value("calls"),
//	    geom.Baseline(1)))
//
// [Baseline] is where the rail sits, and it is the whole difference between the
// two pictures: 0 — the default — puts it at the bottom of the plot with the
// ribbons above, and 1 puts it at the outer rim with them crossing the middle.
// It is [coord.Polar] and not [coord.Pie], which sweeps the wrong axis.
//
// [Thickness] is how deep the rail is, and [Padding] the gap between adjacent
// nodes on it. Both axes describe the unit square, so give the chart a theme
// with no grid, no axis lines and no ticks.
func Arc(src data.Source, opts ...Option) Geom {
	return &arcGeom{src: src, cfg: newConfig(opts)}
}

// arcRail is how deep the rail of nodes is when the layer did not say, as a
// fraction of the plot.
const arcRail = 0.06

// arcRibbonOpacity is what a ribbon is painted at when the layer named no
// opacity. Ribbons cross each other by design, and an opaque one hides every
// ribbon behind it.
const arcRibbonOpacity = 0.5

type arcGeom struct {
	src data.Source
	cfg config
	e   edges
	lay stat.Chord
	err error
}

func (g *arcGeom) Train(x, y scale.Scale) error {
	if g.err = g.e.reset(g.src, g.cfg); g.err != nil {
		return g.err
	}
	if g.err = trainUnit(x, y); g.err != nil {
		return g.err
	}
	g.lay.Reset(g.e.from, g.e.to, g.e.val, g.e.count(), g.cfg.padding)
	g.cfg.trainColors(g.e.s)
	return nil
}

// rail is where the nodes sit and which way the ribbons leave: the band's two
// edges, the one facing the ribbons, and the far side they curve towards.
func (g *arcGeom) rail() (lo, hi, inner, hub float64) {
	t := g.cfg.thickness
	if !(t > 0) {
		t = arcRail
	}
	t = min(t, 1)
	base := clamp01(g.cfg.baseline)
	if base >= 0.5 {
		// The rail is at the far end of the axis — the rim of a disc — and the
		// ribbons curve inwards, towards the middle.
		return base - t, base, base - t, 0
	}
	return base, base + t, base + t, 1
}

func (g *arcGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	sc := acquire(f)
	defer sc.release()
	cd := f.Coords()
	lo, hi, inner, hub := g.rail()

	// The ribbons first and the rail on top, so a pointer on a node reports the
	// node rather than whatever passes behind it — the later mark wins a tie.
	g.ribbons(b, sc, cd, f, inner, hub)
	return g.rails(b, sc, cd, f, lo, hi)
}

func (g *arcGeom) rails(b ir.Backend, sc *scratch, cd coord.Coord, f Frame, lo, hi float64) error {
	rects := sc.rects[:0]
	rows := sc.rows[:0]
	for i, a := range g.lay.Arcs {
		if !(a.Hi > a.Lo) {
			continue
		}
		rects = append(rects, ir.R(f.X.Map(a.Lo), f.Y.Map(lo), f.X.Map(a.Hi), f.Y.Map(hi)))
		rows = append(rows, i)
	}
	sc.rects, sc.rows = rects, rows
	if len(rects) == 0 {
		return nil
	}
	// A node is what several rows have in common rather than a row, so it
	// reports none; the ribbons are the rows and reported theirs.
	return fillBoxes(b, sc, cd, g.cfg, f, series{}, g.e.keys, rects, rows)
}

func (g *arcGeom) ribbons(b ir.Backend, sc *scratch, cd coord.Coord, f Frame, inner, hub float64) {
	opacity := arcRibbonOpacity
	if g.cfg.opacity >= 0 {
		opacity = g.cfg.opacity
	}
	cols := sc.colorsFor(g.cfg, g.e.s, indexes(sc, len(g.lay.Ribbons)))
	pts := sc.pts[:0]
	mrows := sc.mrows[:0]

	x := func(v float64) float32 { return f.X.Map(v) }
	y := func(v float64) float32 { return f.Y.Map(v) }
	reach := g.reach(cd, x, y, inner, hub)

	for e, r := range g.lay.Ribbons {
		if !(r.Src.Hi > r.Src.Lo) {
			continue
		}
		col := g.cfg.nodeColor(f, g.e.keys, g.e.from[e])
		if cols != nil {
			col = cols[e]
		}
		if col = g.cfg.fillOf(col, opacity); col.A == 0 {
			continue
		}
		sc.fill.Reset()
		chord(&sc.fill, cd, x, y, r, inner, hub, reach)
		b.FillPath(&sc.fill, ir.Solid(col), ir.NonZero)

		if sc.wantRows {
			pts = append(pts, cd.Point(x((r.Src.Lo+r.Dst.Hi)/2), y((inner+hub)/2)))
			mrows = append(mrows, g.e.s.rowAt(e))
		}
	}
	sc.pts, sc.mrows = pts, mrows
	f.Marks(pts, mrows)
}

// chord appends the closed ribbon between two spans of the rail: along one
// span, across to the other, along that, and back.
//
// The two crossings are cubics whose control points are pulled from the rail
// towards the hub — the far side of the axis. How far is what makes the shape
// read: a crossing that reaches right across the plot arcs the whole way over,
// and one between neighbours barely leaves the rail.
//
// The pull is one number for the whole ribbon rather than one per crossing, and
// that is what keeps the ribbon a ribbon: two boundaries pulled by different
// amounts converge, and a band that narrows to nothing halfway along says the
// quantity did too.
//
// It is measured in device space, as the distance between the two spans'
// middles, because that is the one measure that means the same thing under both
// coords: under a Cartesian one it is a width against a height, and under a
// polar one it is a chord against a diameter — so two nodes on opposite sides
// of a disc pull all the way to the centre and two beside each other hug the
// rim. One path, two pictures, and the coord decides which, which is the whole
// reason the layout is in the unit square.
func chord(p *ir.Path, cd coord.Coord, x, y func(float64) float32, r stat.Ribbon, inner, hub, reach float64) {
	at := func(v, h float64) ir.Point { return cd.Point(x(v), y(h)) }
	pull := 1.0
	if reach > 0 {
		pull = min(1, separation(cd, x, y, r, inner)/reach)
	}
	across := func(from, to float64) {
		a, b := at(from, inner), at(to, inner)
		c1 := between(a, at(from, hub), pull)
		c2 := between(b, at(to, hub), pull)
		p.CubicTo(c1.X, c1.Y, c2.X, c2.Y, b.X, b.Y)
	}

	start := at(r.Src.Lo, inner)
	p.MoveTo(start.X, start.Y)
	edgeAlong(p, cd, at, r.Src.Lo, r.Src.Hi, inner)
	across(r.Src.Hi, r.Dst.Lo)
	edgeAlong(p, cd, at, r.Dst.Lo, r.Dst.Hi, inner)
	across(r.Dst.Hi, r.Src.Lo)
	p.Close()
}

// separation is how far apart a ribbon's two ends are on the rail, in device
// units, measured between the middles of the two spans.
func separation(cd coord.Coord, x, y func(float64) float32, r stat.Ribbon, inner float64) float64 {
	a := cd.Point(x((r.Src.Lo+r.Src.Hi)/2), y(inner))
	b := cd.Point(x((r.Dst.Lo+r.Dst.Hi)/2), y(inner))
	return float64(gap(a, b))
}

// reach is what a ribbon's separation is measured against: the furthest apart
// any two of this layer's ends are, but never more than the width of the axis
// itself.
//
// The first half is what makes an arc diagram use the room it is given — the
// longest edge arcs the whole way over and every shorter one is a fraction of
// that — and the second is what stops a very wide panel from flattening every
// arc into a ripple. Under a polar coord the axis is the diameter, so two nodes
// on opposite sides of the disc still pull all the way to the centre.
func (g *arcGeom) reach(cd coord.Coord, x, y func(float64) float32, inner, hub float64) float64 {
	widest := 0.0
	for _, r := range g.lay.Ribbons {
		if !(r.Src.Hi > r.Src.Lo) {
			continue
		}
		widest = math.Max(widest, separation(cd, x, y, r, inner))
	}
	depth := float64(gap(cd.Point(x(0), y(inner)), cd.Point(x(0), y(hub))))
	return math.Min(widest, 2*depth)
}

// gap is the distance between two device points.
func gap(a, b ir.Point) float32 {
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	return float32(math.Hypot(dx, dy))
}

// between is t of the way from a to b.
func between(a, b ir.Point, t float64) ir.Point {
	return ir.Point{
		X: a.X + float32(t)*(b.X-a.X),
		Y: a.Y + float32(t)*(b.Y-a.Y),
	}
}

func (g *arcGeom) ColorGuide() (ColorGuide, bool) { return g.cfg.colorGuide(g.e.s, g.err) }

func (g *arcGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.nodeLegends(f, g.e.keys))
}

func (g *arcGeom) Legend(f Frame) (LegendEntry, bool) {
	return oneNodeLegend(g.cfg, f, g.err)
}

func (g *arcGeom) Source() data.Source { return g.src }

func (g *arcGeom) Subset(rows []int) Geom {
	return &arcGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *arcGeom) Describe() Desc {
	d := g.cfg.describe(MarkArc)
	d.Source = g.src
	return d
}

var (
	_ Describer = (*arcGeom)(nil)
	_ Faceter   = (*arcGeom)(nil)
	_ Guided    = (*arcGeom)(nil)
	_ Legender  = (*arcGeom)(nil)
)
