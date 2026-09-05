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
	sc := acquire(f)
	defer sc.release()

	if g.gs.grouped() {
		return eachGroup(sc, &g.gs, g.s, func(seg series, grp int) error {
			return g.build(b, f, sc, seg, g.cfg.groupColor(f, &g.gs, grp), g.cfg.groupDash(f, grp))
		})
	}
	return g.build(b, f, sc, g.s, g.cfg.colorFor(f), g.cfg.dashFor(f))
}

func (g *stepGeom) build(b ir.Backend, f Frame, sc *scratch, s series, col ir.Color, dash []float32) error {
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

	// A staircase is its transitions, so the reduction that keeps its extremes
	// per column is the one that keeps the picture. See [Decimate].
	cd := f.Coords()
	mode, budget := g.cfg.reduction(shapeStair, s, f)
	for _, seg := range sc.segments(s, sc.plottable(s, f.X, f.Y), g.cfg.missing) {
		x, y, _ := sc.project(seg, f)
		keep := sc.reduce(mode, budget, x, y, nil)
		marks := sc.marks(cd, x, y, keep)
		// Reported before the staircase is expanded: a step draws two points
		// per row, and only one of them is where the row is.
		f.Marks(marks, sc.rowsOf(seg, keep, len(x)))
		sx, sy := sc.stepColumns(x, y, keep, g.cfg.steps)
		if len(sx) < 2 {
			continue
		}
		sc.edge = cd.Points(grow(sc.edge, len(sx))[:0], sx, sy)
		strokeRun(b, cd, &sc.line, sc.edge, stroke, g.cfg.closed)
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

// stepColumns expands a projected segment into a staircase, in the mapped
// space the scales speak rather than in device space.
//
// Where the corner goes is the whole content of a step, and it is a statement
// about the *data*: the value held until here and then changed. Turning the
// rows into points first and inserting corners between them afterwards would
// put the corner wherever the coord happened to place the midpoint of two
// device positions, which under anything but a Cartesian coord is not where
// the change happened.
//
// A right angle is a corner in the stroke, not a data point, so the geom
// strokes with a butt cap and a miter join: rounding the corners would round
// the very thing the step is drawn to show.
func (sc *scratch) stepColumns(x, y []float32, keep []int, where StepPos) (sx, sy []float32) {
	n := len(x)
	if keep != nil {
		n = len(keep)
	}
	if n < 2 {
		return nil, nil
	}
	at := func(i int) (float32, float32) {
		if keep != nil {
			return x[keep[i]], y[keep[i]]
		}
		return x[i], y[i]
	}
	m := 2*n - 1
	if where == StepMid {
		m = 3*n - 2
	}
	sx, sy = grow(sc.sx, m)[:0], grow(sc.sy, m)[:0]
	defer func() { sc.sx, sc.sy = sx, sy }()

	px, py := at(0)
	sx, sy = append(sx, px), append(sy, py)
	for i := 1; i < n; i++ {
		cx, cy := at(i)
		switch where {
		case StepPre:
			sx, sy = append(sx, px), append(sy, cy)
		case StepMid:
			mid := (px + cx) / 2
			sx, sy = append(sx, mid, mid), append(sy, py, cy)
		default: // StepPost
			sx, sy = append(sx, cx), append(sy, py)
		}
		sx, sy = append(sx, cx), append(sy, cy)
		px, py = cx, cy
	}
	return sx, sy
}
