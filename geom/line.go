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
	g.s, g.err = resolve(g.src, g.cfg)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg); err != nil {
		return err
	}
	trainFinite(x, g.s.x)
	trainFinite(y, g.s.y)
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
		Dash:  g.cfg.dash,
	}
	if !stroke.Visible() {
		return nil
	}

	for _, seg := range segments(g.s, g.cfg.missing) {
		pts := project(seg, f)
		if len(pts) < 2 {
			continue
		}
		if g.cfg.tension <= 0 {
			b.Polyline(pts, stroke)
			continue
		}
		var p ir.Path
		catmullRom(&p, pts, float32(clamp01(g.cfg.tension)))
		b.StrokePath(&p, stroke)
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
		Dash:  g.cfg.dash,
		Width: pick(g.cfg.width, f.Theme.LineWidth),
	}, true
}

// segments splits a series at missing values according to the policy. Gap
// yields one segment per run of plottable rows; Interpolate yields a single
// segment with interior holes filled in; Error has already been rejected in
// Train.
func segments(s series, m Missing) []series {
	if m == Interpolate {
		return []series{interpolate(s)}
	}
	var out []series
	start := -1
	for i := range s.x {
		ok := finite(s.x[i]) && finite(s.y[i])
		switch {
		case ok && start < 0:
			start = i
		case !ok && start >= 0:
			out = append(out, series{x: s.x[start:i], y: s.y[start:i]})
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, series{x: s.x[start:], y: s.y[start:]})
	}
	return out
}

// interpolate fills interior holes by linear interpolation between the nearest
// plottable neighbours. Leading and trailing holes are dropped: there is
// nothing to interpolate between.
func interpolate(s series) series {
	n := len(s.x)
	first, last := -1, -1
	for i := 0; i < n; i++ {
		if finite(s.x[i]) && finite(s.y[i]) {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return series{}
	}
	xs := make([]float64, 0, last-first+1)
	ys := make([]float64, 0, last-first+1)
	prev := first
	for i := first; i <= last; i++ {
		if finite(s.x[i]) && finite(s.y[i]) {
			xs = append(xs, s.x[i])
			ys = append(ys, s.y[i])
			prev = i
			continue
		}
		// Find the next plottable row and interpolate across the hole.
		next := i + 1
		for next <= last && !(finite(s.x[next]) && finite(s.y[next])) {
			next++
		}
		if next > last {
			break
		}
		t := float64(i-prev) / float64(next-prev)
		xs = append(xs, s.x[prev]+t*(s.x[next]-s.x[prev]))
		ys = append(ys, s.y[prev]+t*(s.y[next]-s.y[prev]))
	}
	return series{x: xs, y: ys}
}

// project maps a segment into device space.
func project(s series, f Frame) []ir.Point {
	pts := make([]ir.Point, 0, len(s.x))
	for i := range s.x {
		pts = append(pts, ir.Point{X: f.X.Map(s.x[i]), Y: f.Y.Map(s.y[i])})
	}
	return pts
}

// catmullRom appends a Catmull-Rom spline through pts as cubic Béziers.
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
	p.MoveTo(pts[0].X, pts[0].Y)
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
