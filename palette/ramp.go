package palette

import (
	"math"

	"github.com/timzifer/refract/ir"
)

// Ramp is a continuous colour ramp: anchor colours spaced evenly over [0, 1]
// and interpolated between.
//
// Interpolation happens in linear light, not in sRGB byte space. Averaging two
// gamma-encoded bytes darkens the midpoint of a ramp by roughly 20% — a band
// that reads as a seam across an otherwise smooth gradient. gg composites in
// linear space for the same reason, so a ramp evaluated here and a gradient
// rasterised there agree.
type Ramp []ir.Color

// At returns the colour at position t, clamped to [0, 1]. An empty Ramp
// returns [ir.Transparent]; a one-colour Ramp is constant.
func (r Ramp) At(t float64) ir.Color {
	switch len(r) {
	case 0:
		return ir.Transparent
	case 1:
		return r[0]
	}
	switch {
	case math.IsNaN(t):
		return ir.Transparent
	case t <= 0:
		return r[0]
	case t >= 1:
		return r[len(r)-1]
	}
	x := t * float64(len(r)-1)
	i := int(x)
	// x < len(r)-1 holds because t < 1, but floating point rounding at the very
	// top of the range is worth guarding rather than trusting.
	if i >= len(r)-1 {
		return r[len(r)-1]
	}
	return Lerp(r[i], r[i+1], x-float64(i))
}

// Reverse returns the ramp running the other way. It allocates; a ramp is a
// handful of colours, and sharing the backing array would let one caller's
// reversal mutate another's palette.
func (r Ramp) Reverse() Ramp {
	out := make(Ramp, len(r))
	for i, c := range r {
		out[len(r)-1-i] = c
	}
	return out
}

// Lerp blends a into b by t in [0, 1], interpolating the colour channels in
// linear light and the alpha channel directly.
//
// Alpha is not gamma-encoded, so it interpolates as it is. The colours are
// non-premultiplied, which means blending two colours of different alpha
// interpolates the *stated* colour rather than the composited one — for a
// colour ramp, where the anchors are normally opaque, that is what a caller
// expects.
func Lerp(a, b ir.Color, t float64) ir.Color {
	switch {
	case t <= 0:
		return a
	case t >= 1:
		return b
	}
	return ir.Color{
		R: encodeSRGB(mix(decodeSRGB(a.R), decodeSRGB(b.R), t)),
		G: encodeSRGB(mix(decodeSRGB(a.G), decodeSRGB(b.G), t)),
		B: encodeSRGB(mix(decodeSRGB(a.B), decodeSRGB(b.B), t)),
		A: uint8(mix(float64(a.A), float64(b.A), t) + 0.5),
	}
}

func mix(a, b, t float64) float64 { return a + (b-a)*t }

// decodeSRGB converts one 8-bit sRGB channel to linear light, using the exact
// piecewise transfer function rather than a plain 2.2 power. The two differ
// most in the darks, which is precisely where a viridis ramp starts.
func decodeSRGB(v uint8) float64 {
	c := float64(v) / 255
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// encodeSRGB is the inverse of decodeSRGB, rounded to the nearest byte.
func encodeSRGB(c float64) uint8 {
	switch {
	case c <= 0:
		return 0
	case c >= 1:
		return 255
	}
	var s float64
	if c <= 0.0031308 {
		s = c * 12.92
	} else {
		s = 1.055*math.Pow(c, 1/2.4) - 0.055
	}
	return uint8(s*255 + 0.5)
}

// Viridis is the default sequential ramp: perceptually uniform, monotone in
// lightness — so it survives being printed in greyscale — and legible under
// every common form of colour vision deficiency.
//
// Smith & van der Walt, matplotlib (2015).
var Viridis = Ramp{
	ir.RGB(0x44, 0x01, 0x54),
	ir.RGB(0x48, 0x28, 0x78),
	ir.RGB(0x3E, 0x4A, 0x89),
	ir.RGB(0x31, 0x68, 0x8E),
	ir.RGB(0x26, 0x82, 0x8E),
	ir.RGB(0x1F, 0x9E, 0x89),
	ir.RGB(0x35, 0xB7, 0x79),
	ir.RGB(0x6D, 0xCD, 0x59),
	ir.RGB(0xB4, 0xDE, 0x2C),
	ir.RGB(0xFD, 0xE7, 0x25),
}

// Cividis is a sequential ramp optimised so that readers with and without
// deuteranomaly see the same ordering of colours, not merely a distinguishable
// one.
//
// Nuñez, Anderton & Renslow, "Optimizing colormaps with consideration for
// color vision deficiency" (PLOS ONE, 2018).
var Cividis = Ramp{
	ir.RGB(0x00, 0x20, 0x4D),
	ir.RGB(0x00, 0x30, 0x6F),
	ir.RGB(0x39, 0x48, 0x6B),
	ir.RGB(0x57, 0x5D, 0x6D),
	ir.RGB(0x70, 0x71, 0x73),
	ir.RGB(0x8A, 0x87, 0x79),
	ir.RGB(0xA6, 0x9D, 0x75),
	ir.RGB(0xC4, 0xB5, 0x6C),
	ir.RGB(0xE4, 0xCF, 0x5B),
	ir.RGB(0xFF, 0xEA, 0x46),
}

// Magma is a sequential ramp from black through purple and red to near-white.
// Like Viridis it is perceptually uniform and monotone in lightness.
var Magma = Ramp{
	ir.RGB(0x00, 0x00, 0x04),
	ir.RGB(0x18, 0x0F, 0x3D),
	ir.RGB(0x44, 0x0F, 0x76),
	ir.RGB(0x72, 0x1F, 0x81),
	ir.RGB(0x9E, 0x2F, 0x7F),
	ir.RGB(0xCD, 0x40, 0x71),
	ir.RGB(0xF1, 0x60, 0x5D),
	ir.RGB(0xFD, 0x96, 0x68),
	ir.RGB(0xFE, 0xC9, 0x8D),
	ir.RGB(0xFC, 0xFD, 0xBF),
}

// Blues is a single-hue sequential ramp, light to dark. Use it when the chart
// already carries a hue meaning and a second one would compete.
//
// ColorBrewer 2.0, Brewer & Harrower.
var Blues = Ramp{
	ir.RGB(0xF7, 0xFB, 0xFF),
	ir.RGB(0xDE, 0xEB, 0xF7),
	ir.RGB(0xC6, 0xDB, 0xEF),
	ir.RGB(0x9E, 0xCA, 0xE1),
	ir.RGB(0x6B, 0xAE, 0xD6),
	ir.RGB(0x42, 0x92, 0xC6),
	ir.RGB(0x21, 0x71, 0xB5),
	ir.RGB(0x08, 0x51, 0x9C),
	ir.RGB(0x08, 0x30, 0x6B),
}

// Greys is a neutral sequential ramp, light to dark.
var Greys = Ramp{
	ir.RGB(0xF7, 0xF7, 0xF7),
	ir.RGB(0xD9, 0xD9, 0xD9),
	ir.RGB(0xBD, 0xBD, 0xBD),
	ir.RGB(0x96, 0x96, 0x96),
	ir.RGB(0x73, 0x73, 0x73),
	ir.RGB(0x52, 0x52, 0x52),
	ir.RGB(0x25, 0x25, 0x25),
}

// BlueOrange is the default diverging ramp: blue at one end, vermilion at the
// other, a near-neutral light grey in the middle.
//
// Blue against orange is the one hue contrast that survives every common form
// of colour vision deficiency, which is why both ends are taken from
// Okabe-Ito rather than from the red/green pairing diverging ramps usually
// reach for.
var BlueOrange = Ramp{
	ir.RGB(0x05, 0x30, 0x61),
	Blue,
	ir.RGB(0x92, 0xC5, 0xDE),
	ir.RGB(0xF2, 0xF2, 0xF2),
	ir.RGB(0xF4, 0xA5, 0x82),
	Vermilion,
	ir.RGB(0x7F, 0x27, 0x04),
}

// PurpleGreen is a second diverging ramp, for when a chart already spends blue
// or orange on something else. Purple against green is the next safest pairing
// after blue against orange.
var PurpleGreen = Ramp{
	ir.RGB(0x40, 0x00, 0x4B),
	ir.RGB(0x9A, 0x70, 0xAB),
	ir.RGB(0xE7, 0xD4, 0xE8),
	ir.RGB(0xF7, 0xF7, 0xF7),
	ir.RGB(0xD9, 0xF0, 0xD3),
	ir.RGB(0x5A, 0xAE, 0x61),
	ir.RGB(0x00, 0x44, 0x1B),
}

// DefaultRamp is the sequential ramp a colour scale uses when none is given.
var DefaultRamp = Viridis
