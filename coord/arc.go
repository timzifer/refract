package coord

import (
	"math"

	"github.com/timzifer/refract/ir"
)

// The partial-arc construction, shared by every coord that draws one.
//
// It was [Polar]'s alone until [Smith] needed the same cubics, and it is a
// verbatim move rather than a rewrite: the expressions below are the ones the
// polar coord shipped in v0.8, in the same order, so every golden file with a
// pie, a donut, a rose or a radar in it is the proof that lifting them out
// changed nothing. See docs/adr/0018-coordinate-systems.md.
//
// A *whole* circle is not here, and the omission is deliberate. Polar closes
// this construction on itself, because that is what its rings and its rim have
// always been and what its golden files hold; Smith draws one with
// [ir.Path.Circle], the four cubics at the exact kappa that every round mark in
// the library is made of. The two agree to about three ulp, which is the
// tolerance ADR 0018 already records for them, and neither is worth moving onto
// the other.
//
// # The angle convention
//
// Angle zero is twelve o'clock and grows clockwise: the point at angle a and
// radius r is (cx + r·sin a, cy − r·cos a). That is the convention a pie is
// written in — the first slice starts straight up — and it is deliberately not
// the convention the Smith geometry is written in, which measures from the
// positive real axis and grows anticlockwise. A coord that thinks in the other
// one converts at its own call site rather than asking for a second convention
// here, because two conventions in one helper is how a sign error survives a
// code review.

// onCircleAt places an angle and a radius about a centre.
func onCircleAt(cx, cy float32, angle, r float64) ir.Point {
	s, c := math.Sincos(angle)
	return ir.Point{X: cx + float32(r*s), Y: cy - float32(r*c)}
}

// arcTo appends the cubics of a sweep from (a0, r0) to (a0+sweep, r1) about the
// centre (cx, cy), continuing from the path's current point.
//
// A sweep at a constant radius is a circular arc, and this is the exact
// construction for one: the control points sit a distance k = (4/3)·tan(φ/4)
// times the radius along the tangent, which at a quarter turn is the kappa in
// internal/markers. Longer sweeps are cut into quarter turns or less, because
// that identity degrades badly past one. A sweep whose two radii differ is the
// same construction with each end scaled by its own radius, which is the honest
// reading of an edge that is straight in data space when the radius is part of
// the data.
func arcTo(path *ir.Path, cx, cy float32, a0, r0, sweep, r1 float64) {
	if sweep == 0 {
		end := onCircleAt(cx, cy, a0, r1)
		path.LineTo(end.X, end.Y)
		return
	}
	n := int(math.Ceil(math.Abs(sweep)/(math.Pi/2) - 1e-9))
	if n < 1 {
		n = 1
	}
	phi := sweep / float64(n)
	k := 4.0 / 3.0 * math.Tan(phi/4)
	for i := range n {
		t0 := float64(i) / float64(n)
		t1 := float64(i+1) / float64(n)
		b0, b1 := a0+sweep*t0, a0+sweep*t1
		ra := r0 + (r1-r0)*t0
		rb := r0 + (r1-r0)*t1
		s0, c0 := math.Sincos(b0)
		s1, c1 := math.Sincos(b1)
		// The point at angle b is (cx + r·sin b, cy − r·cos b), so the tangent
		// in the direction of increasing b is (cos b, sin b).
		p0 := ir.Point{X: cx + float32(ra*s0), Y: cy - float32(ra*c0)}
		p3 := ir.Point{X: cx + float32(rb*s1), Y: cy - float32(rb*c1)}
		c1x := p0.X + float32(k*ra*c0)
		c1y := p0.Y + float32(k*ra*s0)
		c2x := p3.X - float32(k*rb*c1)
		c2y := p3.Y - float32(k*rb*s1)
		path.CubicTo(c1x, c1y, c2x, c2y, p3.X, p3.Y)
	}
}
