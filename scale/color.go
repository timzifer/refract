package scale

import (
	"math"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
)

// ColorScale maps data values onto colours, the way a [Scale] maps them onto
// positions.
//
// It is a separate interface rather than a Scale because the two answer
// different questions and are trained from different columns: a chart commonly
// has two positional scales and one colour scale over a third column.
//
// Like [Scale], it is implemented outside this module and never gains a
// method; [DiscreteColorScale] is the optional interface beside it, and
// [RegisterColor] is how a kind this package does not define is read back.
type ColorScale interface {
	// Train extends the scale's domain to include vs, ignoring NaN and
	// infinities. Calling Train repeatedly accumulates.
	Train(vs ...float64)

	// Domain reports the current data domain.
	Domain() (min, max float64)

	// Color returns the colour for a value. Values outside the domain clamp to
	// its ends; NaN and infinities return the undefined colour.
	Color(v float64) ir.Color
}

// ColorOption configures a colour scale.
type ColorOption func(*colorScale)

// ColorDomain pins the domain explicitly, disabling training.
func ColorDomain(min, max float64) ColorOption {
	return func(c *colorScale) {
		c.fixed = true
		c.dmin, c.dmax, c.trained = min, max, true
	}
}

// ColorCenter pins the value that lands on the middle of a diverging ramp.
// The default is 0.
//
// It has no effect on a sequential scale, where there is no middle to pin.
func ColorCenter(v float64) ColorOption { return func(c *colorScale) { c.center = v } }

// ColorReverse runs the ramp the other way.
func ColorReverse() ColorOption { return func(c *colorScale) { c.reverse = true } }

// ColorUndefined sets the colour for a value the scale cannot place — NaN, an
// infinity, or a category outside a fixed set. The default is fully
// transparent, which draws nothing.
func ColorUndefined(col ir.Color) ColorOption { return func(c *colorScale) { c.undef = col } }

// Sequential returns a colour scale that runs a ramp across the domain from
// end to end. It is the scale for a quantity with a natural low and high —
// a count, a duration, a temperature.
//
// A nil ramp uses [palette.DefaultRamp].
func Sequential(ramp palette.Ramp, opts ...ColorOption) ColorScale {
	return newColorScale(ramp, false, opts)
}

// Diverging returns a colour scale that puts the middle of a ramp on a centre
// value and stretches both halves to the further end of the domain, so that
// equal deviations in either direction get equally strong colours.
//
// It is the scale for a quantity read against a reference: a residual, a
// change, an anomaly. Use [ColorCenter] to move the centre off zero.
//
// A nil ramp uses [palette.BlueOrange].
func Diverging(ramp palette.Ramp, opts ...ColorOption) ColorScale {
	if ramp == nil {
		ramp = palette.BlueOrange
	}
	return newColorScale(ramp, true, opts)
}

func newColorScale(ramp palette.Ramp, diverging bool, opts []ColorOption) ColorScale {
	if ramp == nil {
		ramp = palette.DefaultRamp
	}
	c := &colorScale{ramp: ramp, diverging: diverging, undef: ir.Transparent}
	for _, o := range opts {
		o(c)
	}
	if c.reverse {
		c.ramp = c.ramp.Reverse()
	}
	return c
}

type colorScale struct {
	domainRange
	ramp      palette.Ramp
	diverging bool
	center    float64
	reverse   bool
	fixed     bool
	undef     ir.Color
}

func (c *colorScale) Train(vs ...float64) {
	if c.fixed {
		return
	}
	c.domainRange.Train(vs...)
}

func (c *colorScale) Domain() (float64, float64) {
	if !c.trained {
		return 0, 1
	}
	return c.dmin, c.dmax
}

func (c *colorScale) Color(v float64) ir.Color {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return c.undef
	}
	return c.ramp.At(c.position(v))
}

// position maps a value into [0, 1] along the ramp.
func (c *colorScale) position(v float64) float64 {
	lo, hi := c.Domain()
	if c.diverging {
		// Both halves share the larger deviation, so the centre stays at the
		// middle of the ramp. Scaling each half to its own extreme instead
		// would make a small positive deviation as red as a huge negative one
		// is blue, which is the whole failure mode a diverging ramp exists to
		// avoid.
		reach := math.Max(math.Abs(hi-c.center), math.Abs(lo-c.center))
		if reach == 0 {
			return 0.5
		}
		return clamp01f(0.5 + (v-c.center)/(2*reach))
	}
	if hi == lo {
		return 0.5
	}
	return clamp01f((v - lo) / (hi - lo))
}

func clamp01f(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	}
	return v
}
