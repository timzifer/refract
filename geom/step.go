package geom

import (
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Step connects rows with horizontal and vertical segments instead of a
// straight line.
//
// It is the honest shape for anything that holds a value and then changes it —
// a configuration, a queue depth, a price, a state machine. A plain line
// between two such samples draws a gradual transition that never happened. Use
// [Steps] to say where the change falls between the two rows.
func Step(src data.Source, opts ...Option) Geom {
	return &stepGeom{src: src, cfg: newConfig(opts)}
}

type stepGeom struct {
	src data.Source
	cfg config
	s   series
	gs  groups
	err error
}

func (g *stepGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	trainColumn(x, g.s.x)
	trainColumn(y, g.s.y)
	// A staircase is a reading held over time, and two of them do not add up
	// any more than two lines do.
	g.err = g.gs.train(g.src, g.s, g.cfg, x, y, NoStack)
	return g.err
}

func (g *stepGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	if g.gs.grouped() {
		return eachGroup(f, &g.gs, g.s, func(seg series, grp int) error {
			return g.build(b, f, seg, g.cfg.groupColor(f, &g.gs, grp), g.cfg.groupDash(f, grp))
		})
	}
	return g.build(b, f, g.s, g.cfg.colorFor(f), g.cfg.dashFor(f))
}

func (g *stepGeom) build(b ir.Backend, f Frame, s series, col ir.Color, dash []float32) error {
	stroke := ir.Stroke{
		Color: col,
		Width: pick(g.cfg.width, f.Theme.LineWidth),
		Cap:   ir.CapButt,
		Join:  ir.JoinMiter,
		Dash:  dash,
	}
	if !stroke.Visible() {
		return nil
	}
	sc := acquire(f)
	defer sc.release()

	// A staircase is its transitions, so the reduction that keeps its extremes
	// per column is the one that keeps the picture. See [Decimate].
	mode, budget := g.cfg.reduction(shapeStair, s, f)
	for _, seg := range sc.segments(s, sc.plottable(s, f.X, f.Y), g.cfg.missing) {
		x, y, _ := sc.project(seg, f)
		keep := sc.reduce(mode, budget, x, y, nil)
		marks := sc.marks(x, y, keep)
		// Reported before the staircase is expanded: a step draws two points
		// per row, and only one of them is where the row is.
		f.Marks(marks, sc.rowsOf(seg, keep, len(x)))
		pts := sc.stepPoints(marks, g.cfg.steps)
		if len(pts) < 2 {
			continue
		}
		b.Polyline(pts, stroke)
	}
	return nil
}

func (g *stepGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.legends(f, &g.gs, g.s, SwatchLine))
}

func (g *stepGeom) Legend(f Frame) (LegendEntry, bool) {
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

// stepPoints expands a polyline into a staircase.
//
// A right angle is a corner in the stroke, not a data point, so the geom
// strokes with a butt cap and a miter join: rounding the corners would round
// the very thing the step is drawn to show.
func (sc *scratch) stepPoints(pts []ir.Point, where StepPos) []ir.Point {
	if len(pts) < 2 {
		return pts
	}
	n := 2*len(pts) - 1
	if where == StepMid {
		n = 3*len(pts) - 2
	}
	out := grow(sc.edge, n)[:0]
	defer func() { sc.edge = out }()
	out = append(out, pts[0])
	for i := 1; i < len(pts); i++ {
		a, c := pts[i-1], pts[i]
		switch where {
		case StepPre:
			out = append(out, ir.Point{X: a.X, Y: c.Y})
		case StepMid:
			mid := (a.X + c.X) / 2
			out = append(out, ir.Point{X: mid, Y: a.Y}, ir.Point{X: mid, Y: c.Y})
		default: // StepPost
			out = append(out, ir.Point{X: c.X, Y: a.Y})
		}
		out = append(out, c)
	}
	return out
}
