package geom

import (
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Area fills the region between a series and a baseline, or — given [Y2] — the
// band between two series.
//
// The upper edge is stroked in the layer's full colour and the interior is
// filled with a faded version of it. A band drawn in one flat colour reads as
// a solid object; a band with a drawn edge reads as a series with uncertainty
// around it, which is what an area chart is for. Use [Opacity] to change the
// fill, [Fill] to set it outright.
//
// Given [GroupBy] it draws one band per series, stacked: a stacked area chart.
// [Stack] chooses the baseline they are stacked about — [StackFill] for a
// 100 % chart, [StackSilhouette] for a ThemeRiver, [StackWiggle] for a
// streamgraph — and the fill is solid rather than faded, because the bands of
// a stack are read against each other rather than through each other.
func Area(src data.Source, opts ...Option) Geom {
	return &areaGeom{src: src, cfg: newConfig(opts)}
}

// areaFillOpacity is how much of the layer's colour an inherited area fill
// keeps. It is low enough that a grid line still reads through the fill and
// high enough that two overlapping bands are still two bands.
const areaFillOpacity = 0.25

// stackedFillOpacity is the same for a band that is part of a stack. The bands
// do not overlap and there is nothing to read through them, so they are drawn
// solid: a stack of faded bands would show the grid through the data and would
// make two adjacent series harder to tell apart, not easier.
const stackedFillOpacity = 1

type areaGeom struct {
	src data.Source
	cfg config
	s   series
	gs  groups
	err error
}

func (g *areaGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	trainColumn(x, g.s.x)
	if g.err = g.gs.train(g.src, g.s, g.cfg, x, y, g.cfg.stackFor(StackZero)); g.err != nil {
		return g.err
	}
	if g.gs.stacked() {
		return nil
	}
	trainColumn(y, g.s.y)
	if g.s.y2 != nil {
		trainColumn(y, g.s.y2)
		return nil
	}
	// The baseline is part of the shape, so it has to be inside the domain or
	// the fill is clipped at an edge the reader cannot see.
	y.Train(g.cfg.baseline)
	return nil
}

func (g *areaGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	sc := acquire(f)
	defer sc.release()

	if g.gs.grouped() {
		return g.buildGroups(b, f, sc)
	}
	return g.build(b, f, sc, g.s, g.cfg.fillFor(f, areaFillOpacity), g.cfg.colorFor(f), g.cfg.dashFor(f))
}

// buildGroups draws one band per series.
//
// Each series is drawn as its own band between the bounds the adjustment gave
// it, which is the same shape a [Y2] band already is — so the drawing code is
// the same code, called once per group over that group's rows.
func (g *areaGeom) buildGroups(b ir.Backend, f Frame, sc *scratch) error {
	op := areaFillOpacity
	if g.gs.stacked() {
		op = stackedFillOpacity
	}
	return eachGroup(sc, &g.gs, g.s, func(seg series, grp int) error {
		col := g.cfg.groupColor(f, &g.gs, grp)
		return g.build(b, f, sc, seg, g.cfg.fillOf(col, op), col, g.cfg.groupDash(f, grp))
	})
}

// build draws one band: the shared body of an ungrouped area and of one series
// of a grouped one.
func (g *areaGeom) build(b ir.Backend, f Frame, sc *scratch, s series, fill, line ir.Color, dash []float32) error {
	stroke := ir.Stroke{
		Color: line,
		Width: pick(g.cfg.width, f.Theme.LineWidth),
		Cap:   ir.CapRound,
		Join:  ir.JoinRound,
		Dash:  dash,
	}
	tension := float32(clamp01(g.cfg.tension))
	base := baselinePos(f, g.cfg.baseline)

	// A band is bounded by both of its edges, so its reduction has to see both;
	// an area over a baseline is a line with a fill under it. See [Decimate].
	shape := shapePath
	if s.y2 != nil {
		shape = shapeBand
	}
	mode, budget := g.cfg.reduction(shape, s, f)

	cd := f.Coords()
	for _, seg := range sc.segments(s, sc.plottable(s, f.X, f.Y), g.cfg.missing) {
		x, y, z := sc.project(seg, f)
		keep := sc.reduce(mode, budget, x, y, z)
		top := sc.marks(cd, x, y, keep)
		if len(top) < 2 {
			continue
		}
		// The upper edge carries the rows. A band's lower edge is the same
		// rows in reverse, and reporting it as well would put two positions on
		// each row for no gain.
		f.Marks(top, sc.rowsOf(seg, keep, len(x)))
		if fill.A != 0 {
			sc.fill.Reset()
			appendCurve(&sc.fill, cd, top, tension, true)
			switch {
			case z != nil:
				appendCurve(&sc.fill, cd, sc.lowerEdge(cd, x, z, keep), tension, false)
			case g.cfg.closed:
				// A closed contour is its own boundary: a filled radar is the
				// polygon through the marks, not the polygon plus a detour to
				// the baseline.
				cd.Edge(&sc.fill, top[len(top)-1], top[0])
			default:
				// The floor of an area is the baseline run back under it,
				// which is a pair of mapped positions like any other: under a
				// polar coord it is the arc at the baseline radius, not a
				// chord across the middle of the chart.
				g.appendFloor(&sc.fill, cd, x, keep, base)
			}
			sc.fill.Close()
			b.FillPath(&sc.fill, ir.Solid(fill), ir.NonZero)
		}
		if !stroke.Visible() {
			continue
		}
		sc.line.Reset()
		appendCurve(&sc.line, cd, top, tension, true)
		if g.cfg.closed && z == nil {
			closeLoop(&sc.line, cd, top)
		}
		b.StrokePath(&sc.line, stroke)
		if z != nil {
			sc.line.Reset()
			appendCurve(&sc.line, cd, sc.lowerEdge(cd, x, z, keep), tension, true)
			b.StrokePath(&sc.line, stroke)
		}
	}
	return nil
}

// appendFloor closes an area over a baseline: back along the baseline from the
// last mark to the first.
//
// Under a Cartesian coord that is the two corners it has always been. Under
// one that bends its edges the baseline is a curve of its own, so the run is
// walked at the same resolution the top edge was drawn at.
func (g *areaGeom) appendFloor(p *ir.Path, cd coord.Coord, x []float32, keep []int, base float32) {
	first, last := x[0], x[len(x)-1]
	if keep != nil {
		first, last = x[keep[0]], x[keep[len(keep)-1]]
	}
	if cd.Straight() {
		end, start := cd.Point(last, base), cd.Point(first, base)
		p.LineTo(end.X, end.Y)
		p.LineTo(start.X, start.Y)
		return
	}
	end, start := cd.Point(last, base), cd.Point(first, base)
	p.LineTo(end.X, end.Y)
	cd.Edge(p, end, start)
}

func (g *areaGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.legends(f, &g.gs, g.s, SwatchBox))
}

func (g *areaGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil {
		return LegendEntry{}, false
	}
	return LegendEntry{
		Label: g.cfg.labelFor(),
		Color: g.cfg.colorFor(f),
		Kind:  SwatchBox,
	}, true
}
