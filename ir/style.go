package ir

import "image/color"

// Color is refract's colour type: 8-bit non-premultiplied sRGBA.
//
// It is an alias for color.NRGBA rather than a new type so that colours pass
// straight into any stdlib or backend API taking a color.Color, with no
// conversion layer and no import of refract in code that only wants to name a
// colour.
type Color = color.NRGBA

// RGB returns an opaque colour.
func RGB(r, g, b uint8) Color { return Color{R: r, G: g, B: b, A: 255} }

// RGBA returns a colour with explicit alpha.
func RGBA(r, g, b, a uint8) Color { return Color{R: r, G: g, B: b, A: a} }

// Transparent is the fully transparent colour. A fill or stroke with A == 0 is
// skipped by every backend.
var Transparent = Color{}

// Fade returns c with its alpha scaled by f, clamped to [0, 1].
func Fade(c Color, f float64) Color {
	switch {
	case f <= 0:
		return Color{R: c.R, G: c.G, B: c.B}
	case f >= 1:
		return c
	}
	return Color{R: c.R, G: c.G, B: c.B, A: uint8(float64(c.A)*f + 0.5)}
}

// LineCap is how a stroke terminates.
type LineCap uint8

// The line caps.
const (
	CapButt LineCap = iota
	CapRound
	CapSquare
)

// LineJoin is how two stroke segments meet.
type LineJoin uint8

// The line joins.
const (
	JoinMiter LineJoin = iota
	JoinRound
	JoinBevel
)

// Stroke is the full state needed to stroke a polyline or path.
type Stroke struct {
	Color      Color
	Width      float32
	Cap        LineCap
	Join       LineJoin
	MiterLimit float32   // 0 means the backend default (4)
	Dash       []float32 // nil or empty means solid
	DashOffset float32
}

// Visible reports whether stroking with s would put any ink on the canvas.
func (s Stroke) Visible() bool { return s.Color.A != 0 && s.Width > 0 }

// FillRule selects how a path's interior is determined.
type FillRule uint8

// The fill rules.
const (
	NonZero FillRule = iota
	EvenOdd
)

// GradientStop is one colour stop of a linear gradient, at offset t in [0, 1]
// along the gradient axis.
type GradientStop struct {
	Offset float32
	Color  Color
}

// Fill describes how a path's interior is painted.
//
// A Fill with no Stops is a solid Color. With Stops, it is a linear gradient
// running from Start to End in the same coordinate space as the path; Color is
// then ignored. Radial and sweep gradients are not in the v0.1 IR — no v0.1
// geom needs them, and adding them later is additive.
type Fill struct {
	Color Color
	Start Point
	End   Point
	Stops []GradientStop
}

// Solid returns an opaque-rules solid fill of colour c.
func Solid(c Color) Fill { return Fill{Color: c} }

// IsGradient reports whether f paints a gradient rather than a solid colour.
func (f Fill) IsGradient() bool { return len(f.Stops) > 0 }

// Visible reports whether filling with f would put any ink on the canvas.
func (f Fill) Visible() bool {
	if f.IsGradient() {
		for _, s := range f.Stops {
			if s.Color.A != 0 {
				return true
			}
		}
		return false
	}
	return f.Color.A != 0
}

// Marker is a scatter-point shape, drawn centred on its position.
type Marker uint8

// The marker shapes.
const (
	MarkerCircle Marker = iota
	MarkerSquare
	MarkerDiamond
	MarkerTriangle
	MarkerCross
	MarkerPlus
)

// MarkerStyle is the paint applied to every instance of a Markers call.
type MarkerStyle struct {
	Size   float32 // nominal diameter in device units
	Fill   Color
	Stroke Stroke
}
