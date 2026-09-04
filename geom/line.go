package geom

import (
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Line connects consecutive rows with a stroked path.
func Line(src data.Source, opts ...Option) Geom {
	return &lineGeom{src: src, cfg: newConfig(opts)}
}

type lineGeom struct {
	src data.Source
	cfg config
	s   series
	err error
}

func (g *lineGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	trainColumn(x, g.s.x)
	trainColumn(y, g.s.y)
	return nil
}

func (g *lineGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	stroke := ir.Stroke{
		Color: g.cfg.colorFor(f),
		Width: pick(g.cfg.width, f.Theme.LineWidth),
		Cap:   ir.CapRound,
		Join:  ir.JoinRound,
		Dash:  g.cfg.dashFor(f),
	}
	if !stroke.Visible() {
		return nil
	}

	sc := acquire(f)
	defer sc.release()

	mode, budget := g.cfg.reduction(shapePath, g.s, f)
	for _, seg := range sc.segments(g.s, sc.plottable(g.s, f.X, f.Y), g.cfg.missing) {
		x, y, _ := sc.project(seg, f)
		keep := sc.reduce(mode, budget, x, y, nil)
		pts := sc.marks(x, y, keep)
		if len(pts) < 2 {
			continue
		}
		// The vertices are the rows, whether the path drawn through them is
		// straight or a curve — which is why this is reported here rather than
		// left to be read off the drawing call.
		f.Marks(pts, sc.rowsOf(seg, keep, len(x)))
		if g.cfg.tension <= 0 {
			b.Polyline(pts, stroke)
			continue
		}
		sc.line.Reset()
		appendCurve(&sc.line, pts, float32(clamp01(g.cfg.tension)), true)
		b.StrokePath(&sc.line, stroke)
	}
	return nil
}

func (g *lineGeom) Legend(f Frame) (LegendEntry, bool) {
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

// segments splits a series at missing values according to the policy. Gap
// yields one segment per run of plottable rows; Interpolate yields a single
// segment with interior holes filled in; Error has already been rejected in
// Train.
//
// ok marks the rows that can be drawn — see [series.plottable]. It is passed
// in rather than recomputed so that every traversal of one series agrees on
// which rows are holes.
func (sc *scratch) segments(s series, ok []bool, m Missing) []series {
	out := sc.segs[:0]
	if m == Interpolate {
		sc.segs = append(out, sc.interpolate(s, ok))
		return sc.segs
	}
	start := -1
	for i := range s.x {
		switch {
		case ok[i] && start < 0:
			start = i
		case !ok[i] && start >= 0:
			out = append(out, s.slice(start, i))
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, s.slice(start, len(s.x)))
	}
	sc.segs = out
	return out
}

// slice returns the rows [lo, hi) as a series, borrowing rather than copying.
func (s series) slice(lo, hi int) series {
	out := series{x: s.x[lo:hi], y: s.y[lo:hi], off: s.off + lo, origin: s.origin}
	if s.y2 != nil {
		out.y2 = s.y2[lo:hi]
	}
	if s.c != nil {
		out.c = s.c[lo:hi]
	}
	if s.rows != nil {
		out.rows = s.rows[lo:hi]
	}
	return out
}

// interpolate fills interior holes by linear interpolation between the nearest
// plottable neighbours. Leading and trailing holes are dropped: there is
// nothing to interpolate between.
func (sc *scratch) interpolate(s series, ok []bool) series {
	n := len(s.x)
	first, last := -1, -1
	for i := 0; i < n; i++ {
		if ok[i] {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return series{}
	}
	out := series{
		x: grow(sc.fx, last-first+1)[:0],
		y: grow(sc.fy, last-first+1)[:0],
	}
	if s.y2 != nil {
		out.y2 = grow(sc.fz, last-first+1)[:0]
	}
	// An interpolated series is not a contiguous run of the source: some of
	// its elements are values that were never measured. It therefore carries a
	// row per element, with -1 for the invented ones — but only when someone
	// asked, because otherwise this is a per-row buffer nobody reads.
	if sc.wantRows {
		out.rows = grow(sc.irows, last-first+1)[:0]
	}
	// The interpolated columns live in the scratch, so the next frame fills
	// the same memory instead of asking for more.
	defer func() {
		sc.fx, sc.fy = out.x, out.y
		if out.y2 != nil {
			sc.fz = out.y2
		}
		if out.rows != nil {
			sc.irows = out.rows
		}
	}()
	prev := first
	for i := first; i <= last; i++ {
		if ok[i] {
			out.append(s, i)
			prev = i
			continue
		}
		// Find the next plottable row and interpolate across the hole.
		next := i + 1
		for next <= last && !ok[next] {
			next++
		}
		if next > last {
			break
		}
		t := float64(i-prev) / float64(next-prev)
		out.x = append(out.x, lerp(s.x[prev], s.x[next], t))
		out.y = append(out.y, lerp(s.y[prev], s.y[next], t))
		if s.y2 != nil {
			out.y2 = append(out.y2, lerp(s.y2[prev], s.y2[next], t))
		}
		if out.rows != nil {
			out.rows = append(out.rows, -1)
		}
	}
	return out
}

// append copies row i of src onto s.
func (s *series) append(src series, i int) {
	s.x = append(s.x, src.x[i])
	s.y = append(s.y, src.y[i])
	if s.y2 != nil {
		s.y2 = append(s.y2, src.y2[i])
	}
	if s.rows != nil {
		s.rows = append(s.rows, src.rowAt(i))
	}
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// appendCurve appends pts to p, straight when tension is zero and
// Catmull-Rom-smoothed otherwise, starting a new subpath when move is set.
//
// Continuing an existing subpath is what lets an area append its lower edge to
// its upper one and get a single closed shape rather than two.
func appendCurve(p *ir.Path, pts []ir.Point, tension float32, move bool) {
	if len(pts) == 0 {
		return
	}
	if move {
		p.MoveTo(pts[0].X, pts[0].Y)
	} else {
		p.LineTo(pts[0].X, pts[0].Y)
	}
	if tension <= 0 {
		for _, q := range pts[1:] {
			p.LineTo(q.X, q.Y)
		}
		return
	}
	catmullRom(p, pts, tension)
}

// catmullRom appends a Catmull-Rom spline through pts as cubic Béziers,
// continuing from p's current point — which must already be pts[0].
//
// tension in (0, 1] scales the tangents: 1 gives the classic uniform
// Catmull-Rom curve, smaller values pull the curve back towards the polyline.
// The curve passes through every data point, which matters — a smoothing that
// misses the data would be drawing something that was never measured.
func catmullRom(p *ir.Path, pts []ir.Point, tension float32) {
	n := len(pts)
	if n < 2 {
		return
	}
	if n == 2 {
		p.LineTo(pts[1].X, pts[1].Y)
		return
	}
	k := tension / 6
	for i := 0; i < n-1; i++ {
		p0 := pts[max(i-1, 0)]
		p1 := pts[i]
		p2 := pts[i+1]
		p3 := pts[min(i+2, n-1)]
		c1 := ir.Point{X: p1.X + (p2.X-p0.X)*k, Y: p1.Y + (p2.Y-p0.Y)*k}
		c2 := ir.Point{X: p2.X - (p3.X-p1.X)*k, Y: p2.Y - (p3.Y-p1.Y)*k}
		p.CubicTo(c1.X, c1.Y, c2.X, c2.Y, p2.X, p2.Y)
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// pick returns v if it was set to something positive, otherwise the fallback.
func pick(v, fallback float32) float32 {
	if v > 0 {
		return v
	}
	return fallback
}
