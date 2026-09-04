package ir

import "math"

// Point is a position in device space.
//
// The concept document calls this f32.Point; refract keeps it here rather than
// spending a package on two fields (see docs/adr/0002).
type Point struct {
	X, Y float32
}

// Rect is an axis-aligned rectangle in device space. Min is the top-left
// corner, Max the bottom-right; a Rect with Max below Min on either axis is
// empty.
type Rect struct {
	Min, Max Point
}

// R builds a Rect from its edges.
func R(x0, y0, x1, y1 float32) Rect {
	return Rect{Point{x0, y0}, Point{x1, y1}}
}

// Dx returns the width of r.
func (r Rect) Dx() float32 { return r.Max.X - r.Min.X }

// Dy returns the height of r.
func (r Rect) Dy() float32 { return r.Max.Y - r.Min.Y }

// Empty reports whether r encloses no area.
func (r Rect) Empty() bool { return r.Max.X <= r.Min.X || r.Max.Y <= r.Min.Y }

// Contains reports whether p lies inside r, edges included.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.Min.X && p.X <= r.Max.X && p.Y >= r.Min.Y && p.Y <= r.Max.Y
}

// Inset returns r shrunk by the given amounts on each side.
func (r Rect) Inset(left, top, right, bottom float32) Rect {
	return Rect{
		Point{r.Min.X + left, r.Min.Y + top},
		Point{r.Max.X - right, r.Max.Y - bottom},
	}
}

// Affine is a 2D affine transform in row-major order:
//
//	| A C E |
//	| B D F |
//	| 0 0 1 |
//
// which is the same element order SVG's matrix(a,b,c,d,e,f) uses.
type Affine struct {
	A, B, C, D, E, F float32
}

// Identity is the transform that changes nothing.
var Identity = Affine{A: 1, D: 1}

// IsIdentity reports whether a is the identity transform.
func (a Affine) IsIdentity() bool { return a == Identity }

// Translate returns a translation transform.
func Translate(dx, dy float32) Affine { return Affine{A: 1, D: 1, E: dx, F: dy} }

// Scale returns a scaling transform.
func Scale(sx, sy float32) Affine { return Affine{A: sx, D: sy} }

// Rotate returns a rotation transform by angle radians, clockwise in screen
// coordinates (Y down).
func Rotate(angle float64) Affine {
	s, c := math.Sincos(angle)
	return Affine{A: float32(c), B: float32(s), C: float32(-s), D: float32(c)}
}

// Mul returns a*b: the transform that applies b first, then a.
func (a Affine) Mul(b Affine) Affine {
	return Affine{
		A: a.A*b.A + a.C*b.B,
		B: a.B*b.A + a.D*b.B,
		C: a.A*b.C + a.C*b.D,
		D: a.B*b.C + a.D*b.D,
		E: a.A*b.E + a.C*b.F + a.E,
		F: a.B*b.E + a.D*b.F + a.F,
	}
}

// Apply transforms p by a.
func (a Affine) Apply(p Point) Point {
	return Point{
		X: a.A*p.X + a.C*p.Y + a.E,
		Y: a.B*p.X + a.D*p.Y + a.F,
	}
}
