// Package markers builds the outline of a scatter marker.
//
// Every backend has to draw the same diamond. A marker that is a slightly
// different shape in PDF than in SVG is the kind of difference nobody notices
// until it matters, so the outline is defined once and the built-in backends
// share it. The gg backend is a separate module and cannot import an internal
// package; it carries its own copy, and says so.
package markers

import "github.com/timzifer/refract/ir"

// kappa is the cubic Bézier constant that approximates a quarter circle.
const kappa = 0.5522847498307936

// Path appends a marker shape centred on the origin, sized so that its nominal
// extent is size.
func Path(p *ir.Path, m ir.Marker, size float32) {
	r := size / 2
	switch m {
	case ir.MarkerSquare:
		p.Rect(ir.R(-r, -r, r, r))
	case ir.MarkerDiamond:
		p.MoveTo(0, -r).LineTo(r, 0).LineTo(0, r).LineTo(-r, 0).Close()
	case ir.MarkerTriangle:
		// Centroid-centred equilateral triangle pointing up.
		h := r * 1.5
		w := r * 1.2990381 // r * 1.5 * sqrt(3)/2
		p.MoveTo(0, -h*2/3).LineTo(w, h/3).LineTo(-w, h/3).Close()
	case ir.MarkerCross:
		arm := r * 0.28
		d := (r - arm) * 0.7071068
		p.MoveTo(-d-arm, -d).LineTo(-d, -d-arm).LineTo(0, -arm*1.4142136).
			LineTo(d, -d-arm).LineTo(d+arm, -d).LineTo(arm*1.4142136, 0).
			LineTo(d+arm, d).LineTo(d, d+arm).LineTo(0, arm*1.4142136).
			LineTo(-d, d+arm).LineTo(-d-arm, d).LineTo(-arm*1.4142136, 0).Close()
	case ir.MarkerPlus:
		arm := r * 0.28
		p.MoveTo(-arm, -r).LineTo(arm, -r).LineTo(arm, -arm).LineTo(r, -arm).
			LineTo(r, arm).LineTo(arm, arm).LineTo(arm, r).LineTo(-arm, r).
			LineTo(-arm, arm).LineTo(-r, arm).LineTo(-r, -arm).LineTo(-arm, -arm).Close()
	default: // ir.MarkerCircle
		k := r * kappa
		p.MoveTo(r, 0).
			CubicTo(r, k, k, r, 0, r).
			CubicTo(-k, r, -r, k, -r, 0).
			CubicTo(-r, -k, -k, -r, 0, -r).
			CubicTo(k, -r, r, -k, r, 0).
			Close()
	}
}
