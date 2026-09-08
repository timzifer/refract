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

// ColorTransform names the shape of a ramp's traversal of the domain.
//
// It is the colour channel's answer to the choice between [Linear], [Log] and
// [SymLog] on an axis, and it exists for the same reason: a quantity that
// spans orders of magnitude — a bin count, a hexbin density, a duration —
// has every value but the largest few rounded to one end of a ramp that runs
// linearly.
type ColorTransform string

// The colour transforms.
const (
	// TransformLinear runs the ramp evenly across the domain. It is the
	// default and the zero value.
	TransformLinear ColorTransform = ""
	// TransformLog runs it evenly across the logarithm of the domain. See
	// [ColorLog].
	TransformLog ColorTransform = "log"
	// TransformSymLog runs it evenly across a symmetric logarithm: linear
	// within a threshold of zero and logarithmic outside it. See
	// [ColorSymLog].
	TransformSymLog ColorTransform = "symlog"
)

// ColorLog runs the ramp logarithmically across the domain, so that each
// decade of the data gets an equal share of the ramp. A base at or below 1
// means the default, 10.
//
// The domain is strictly positive, exactly as [Log]'s is. Training ignores
// zero and negative values, and [ColorScale.Color] returns the undefined
// colour for one rather than clamping it to the low end of the ramp — a
// heatmap that painted an empty cell the colour of its rarest observation
// would be inventing a count. With the default transparent undefined colour
// that is what a reader expects anyway: the empty cells of a binned heatmap
// are the background.
//
// On a [Diverging] scale the transform runs on the signed deviation from the
// centre rather than on the value, so a logarithm there is a symmetric one —
// see [ColorSymLog]. A plain logarithm has nothing to say about a negative
// deviation, and nothing to say about zero, which is the centre itself.
func ColorLog(base float64) ColorOption {
	return func(c *colorScale) {
		c.xf.kind = TransformLog
		if base > 1 {
			c.xf.base = base
		}
	}
}

// ColorSymLog runs the ramp across a symmetric logarithm of the domain:
// linear within threshold of zero, logarithmic outside it, and defined for
// every finite value including negative ones. A base at or below 1 means the
// default, 10; a threshold at or below 0 means the default, 1.
//
// This is the transform for a quantity that spans orders of magnitude *and*
// crosses zero — a signed residual, a change, a log-fold ratio — which is
// also the quantity a [Diverging] ramp is for. Choose the threshold to match
// the smallest magnitude that carries meaning.
func ColorSymLog(base, threshold float64) ColorOption {
	return func(c *colorScale) {
		c.xf.kind = TransformSymLog
		if base > 1 {
			c.xf.base = base
		}
		if threshold > 0 {
			c.xf.thr = threshold
		}
	}
}

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

func newColorScale(ramp palette.Ramp, diverging bool, opts []ColorOption) *colorScale {
	if ramp == nil {
		ramp = palette.DefaultRamp
	}
	c := &colorScale{
		ramp: ramp, diverging: diverging, undef: ir.Transparent,
		xf: colorXform{base: 10, thr: 1},
	}
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
	xf        colorXform
}

// colorXform is the transform's configuration. The zero kind is linear, which
// is why every scale can carry one.
type colorXform struct {
	kind ColorTransform
	base float64
	thr  float64
}

// logOf is the transform's forward map on a positive value.
func (x colorXform) logOf(v float64) float64 { return math.Log(v) / math.Log(x.base) }

// symOf is the forward map of the symmetric logarithm, which is what a signed
// quantity is transformed by: sign(v)·log_base(1 + |v|/threshold). It is the
// same expression [SymLog] positions with.
func (x colorXform) symOf(v float64) float64 {
	return math.Copysign(x.logOf(1+math.Abs(v)/x.thr), v)
}

// symInv inverts symOf.
func (x colorXform) symInv(t float64) float64 {
	return math.Copysign((math.Pow(x.base, math.Abs(t))-1)*x.thr, t)
}

func (c *colorScale) Train(vs ...float64) {
	if c.fixed {
		return
	}
	if !c.logDomain() {
		c.domainRange.Train(vs...)
		return
	}
	// A log ramp has no colour for zero or a negative number, so training on
	// one would drag the domain somewhere the ramp cannot reach. This is what
	// [logScale.Train] does with the same values and for the same reason.
	for _, v := range vs {
		if v > 0 {
			c.domainRange.Train(v)
		}
	}
}

// logDomain reports whether the domain itself is logarithmic, which it is only
// for a sequential log scale: a diverging one transforms the signed deviation
// from its centre, and that is symmetric by construction.
func (c *colorScale) logDomain() bool { return c.xf.kind == TransformLog && !c.diverging }

func (c *colorScale) Domain() (float64, float64) {
	if c.logDomain() {
		// Mirrors [logScale.effective]: an untrained or degenerate log domain
		// still has to paint something, and a decade either side is the
		// log-space equivalent of the linear fallback.
		lo, hi := c.dmin, c.dmax
		if !c.trained || lo <= 0 || hi <= 0 {
			return 1, 10
		}
		if lo == hi {
			return lo / c.xf.base, hi * c.xf.base
		}
		return lo, hi
	}
	if !c.trained {
		return 0, 1
	}
	return c.dmin, c.dmax
}

func (c *colorScale) Color(v float64) ir.Color {
	if math.IsNaN(v) || math.IsInf(v, 0) || !c.defined(v) {
		return c.undef
	}
	return c.ramp.At(c.position(v))
}

// defined reports whether v has a place on the ramp. Only a log domain
// excludes a finite value, and it excludes the same ones [Log] does.
func (c *colorScale) defined(v float64) bool { return !c.logDomain() || v > 0 }

// ColorPosition implements [ColorTransformer]: where v sits along the ramp.
func (c *colorScale) ColorPosition(v float64) float64 { return c.position(v) }

// position maps a value into [0, 1] along the ramp.
func (c *colorScale) position(v float64) float64 {
	lo, hi := c.Domain()
	return c.positionIn(lo, hi, v)
}

// positionIn is position over a domain given rather than the scale's own. A
// classed scale reuses the arithmetic over a domain it has widened to cover
// its outermost breaks; see [Threshold].
func (c *colorScale) positionIn(lo, hi, v float64) float64 {
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
		d, span := v-c.center, reach
		if c.xf.kind != TransformLinear {
			// The transform runs on the deviation, so both halves are still
			// mirror images of each other and the centre is still the middle
			// of the ramp. A signed quantity transforms symmetrically, which
			// is why a log transform reads as a symlog here.
			d, span = c.xf.symOf(d), c.xf.symOf(reach)
		}
		if span == 0 {
			return 0.5
		}
		return clamp01f(0.5 + d/(2*span))
	}
	if hi == lo {
		return 0.5
	}
	switch c.xf.kind {
	case TransformLog:
		l, h := c.xf.logOf(lo), c.xf.logOf(hi)
		if h == l {
			return 0.5
		}
		return clamp01f((c.xf.logOf(v) - l) / (h - l))
	case TransformSymLog:
		l, h := c.xf.symOf(lo), c.xf.symOf(hi)
		if h == l {
			return 0.5
		}
		return clamp01f((c.xf.symOf(v) - l) / (h - l))
	}
	return clamp01f((v - lo) / (hi - lo))
}

// ColorValueAt implements [ColorTransformer]: the inverse of position.
func (c *colorScale) ColorValueAt(t float64) float64 {
	lo, hi := c.Domain()
	return c.valueIn(lo, hi, t)
}

// valueIn is ColorValueAt over a domain given rather than the scale's own.
func (c *colorScale) valueIn(lo, hi, t float64) float64 {
	if c.diverging {
		reach := math.Max(math.Abs(hi-c.center), math.Abs(lo-c.center))
		if c.xf.kind == TransformLinear {
			return c.center + (t-0.5)*2*reach
		}
		return c.center + c.xf.symInv((t-0.5)*2*c.xf.symOf(reach))
	}
	switch c.xf.kind {
	case TransformLog:
		l, h := c.xf.logOf(lo), c.xf.logOf(hi)
		return math.Pow(c.xf.base, l+t*(h-l))
	case TransformSymLog:
		l, h := c.xf.symOf(lo), c.xf.symOf(hi)
		return c.xf.symInv(l + t*(h-l))
	}
	return lo + t*(hi-lo)
}

// ColorAxis implements [ColorTransformer].
//
// A colourbar is an axis, and this is the axis it is: a scale over the same
// domain whose tick search picks numbers that read as round in whatever space
// the ramp runs in — decades for a log ramp, round numbers for a linear one.
// It is a fresh scale on every call, because the caller sets its range.
//
// A diverging scale reports a linear axis whatever its transform is: its
// ticks are values, and a symmetric logarithm centred on a value that is not
// zero has no round numbers of its own to offer. Where those ticks *sit* on
// the bar is not the axis's business — see [ColorPositionOf].
func (c *colorScale) ColorAxis() Scale {
	lo, hi := c.Domain()
	return c.axisOver(lo, hi)
}

// axisOver is ColorAxis over a domain given rather than the scale's own.
func (c *colorScale) axisOver(lo, hi float64) Scale {
	var s Scale
	switch {
	case c.diverging:
		s = Linear()
	case c.xf.kind == TransformLog:
		s = Log(LogBase(c.xf.base))
	case c.xf.kind == TransformSymLog:
		s = SymLog(SymLogBase(c.xf.base), SymLogThreshold(c.xf.thr))
	default:
		s = Linear()
	}
	s.Train(lo, hi)
	return s
}

// ColorTransform implements [ColorTransformer].
func (c *colorScale) ColorTransform() ColorTransform { return c.xf.kind }

// ColorTransformer is the optional interface a colour scale implements when
// the ramp does not run linearly across its domain.
//
// It is what a colourbar reads. Painting one means answering three questions —
// which colour is at this point of the bar, where on the bar does this value
// sit, and which values are worth labelling — and a scale that compresses its
// domain answers all three differently from one that does not. A scale that
// does not implement it is read as linear over [ColorScale.Domain], which is
// what every colour scale did before the interface existed.
type ColorTransformer interface {
	// ColorPosition returns where v sits along the ramp, in [0, 1]. It is the
	// same number [ColorScale.Color] paints from.
	ColorPosition(v float64) float64

	// ColorValueAt inverts ColorPosition: the value the ramp reaches at t.
	// It is how a gradient is sampled evenly along the bar rather than evenly
	// across the domain, which for a compressed ramp is not the same thing.
	ColorValueAt(t float64) float64

	// ColorAxis returns a positional scale over the domain whose ticks are
	// the values worth labelling. It is a fresh scale, and the caller owns it.
	ColorAxis() Scale

	// ColorTransform names the transform, so that a caller which supplies its
	// own compression — a density raster, a hexbin — can tell that the scale
	// is already doing the job.
	ColorTransform() ColorTransform
}

// ColorPositionOf returns where v sits along cs's ramp, in [0, 1], falling
// back to a linear reading of the domain for a scale that is not a
// [ColorTransformer].
func ColorPositionOf(cs ColorScale, v float64) float64 {
	if t, ok := cs.(ColorTransformer); ok {
		return t.ColorPosition(v)
	}
	lo, hi := cs.Domain()
	if hi == lo {
		return 0.5
	}
	return clamp01f((v - lo) / (hi - lo))
}

// ColorValueOf returns the value cs's ramp reaches at t, the inverse of
// [ColorPositionOf].
func ColorValueOf(cs ColorScale, t float64) float64 {
	if x, ok := cs.(ColorTransformer); ok {
		return x.ColorValueAt(t)
	}
	lo, hi := cs.Domain()
	return lo + t*(hi-lo)
}

// ColorAxisOf returns the axis a colourbar over cs is labelled by. It is
// never nil: a scale that is not a [ColorTransformer] gets a linear axis over
// its domain, which is what a colourbar has always drawn.
func ColorAxisOf(cs ColorScale) Scale {
	if x, ok := cs.(ColorTransformer); ok {
		return x.ColorAxis()
	}
	lo, hi := cs.Domain()
	s := Linear()
	s.Train(lo, hi)
	return s
}

// ColorTransformOf names cs's transform, reporting [TransformLinear] for a
// scale that is not a [ColorTransformer].
func ColorTransformOf(cs ColorScale) ColorTransform {
	if x, ok := cs.(ColorTransformer); ok {
		return x.ColorTransform()
	}
	return TransformLinear
}

var _ ColorTransformer = (*colorScale)(nil)

func clamp01f(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	}
	return v
}
