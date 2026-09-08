package scale

import (
	"math"
	"sort"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
)

// ClassedColorScale paints a finite set of classes rather than a continuum:
// the domain is cut at a handful of boundaries and everything inside one gets
// a single colour.
//
// It rides the [ColorScale] interface the way [DiscreteColorScale] does, and
// it is a third thing rather than a variation on either. Its input is a
// number, so a layer binds it through the same option and reads it through
// [ColorScale.Color] — but its output is a short list of colours, so a reader
// can name the class a mark is in instead of estimating a value from a ramp.
// That is what a choropleth is for, and what a heatmap over counts is often
// better read as: "between a hundred and a thousand" is a fact a reader can
// carry, and "about a fifth of the way up a green" is not.
//
// A classed scale still contributes a colourbar rather than legend entries,
// because its classes are intervals of a quantity rather than names: the bar
// is drawn in steps and labelled at the boundaries.
type ClassedColorScale interface {
	ColorScale

	// Breaks returns the interior class boundaries, ascending. A scale with n
	// classes has n-1 of them, and the outer edges of the outermost classes
	// are the ends of [ColorScale.Domain].
	//
	// The slice is freshly allocated, so a caller may keep it.
	Breaks() []float64

	// Classes reports how many classes the scale cuts its domain into.
	Classes() int
}

// Threshold returns a colour scale that cuts the domain at boundaries given
// explicitly. A value lands in the class above a boundary it is equal to, so
// breaks of 10 and 100 make three classes: below 10, 10 up to 100, and 100 and
// over.
//
// It is the scale for boundaries that come from outside the data — a legal
// limit, a service level, a clinical range. Boundaries derived *from* the
// data are [Quantize]'s and [Quantile]'s business.
//
// The breaks are sorted and de-duplicated, so the caller need not. Passing
// none makes a scale of one class, which is a solid colour: that is what
// "cut this nowhere" means, and it is more useful than a panic in a chart
// whose breaks came from a configuration file.
//
// A nil ramp uses [palette.DefaultRamp].
func Threshold(ramp palette.Ramp, breaks []float64, opts ...ColorOption) ClassedColorScale {
	return &classed{
		kind:  KindThreshold,
		base:  newColorScale(ramp, false, opts),
		given: sortedBreaks(breaks),
	}
}

// Quantize returns a colour scale that cuts the domain into equal classes.
//
// Equal in the space the ramp runs in: under [ColorLog] the classes are
// decades rather than equal spans, which is the point of asking for both. A
// count of zero or less means one class.
//
// It is the scale for a quantity a reader should read in bands without the
// bands claiming to say anything about the distribution. When the bands should
// say something about it — equally many observations in each — the scale is
// [Quantile].
//
// A nil ramp uses [palette.DefaultRamp].
func Quantize(ramp palette.Ramp, classes int, opts ...ColorOption) ClassedColorScale {
	if classes < 1 {
		classes = 1
	}
	return &classed{kind: KindQuantize, base: newColorScale(ramp, false, opts), n: classes}
}

// Quantile returns a colour scale that cuts the domain so that each class
// holds as near as possible the same number of observations.
//
// It is the scale for a quantity whose distribution is the thing worth seeing:
// a count, an income, a latency. Equal-width classes over such data put nearly
// every observation in one class and leave the rest to name a handful of
// outliers, which is [Quantize]'s honest failure and this scale's whole
// purpose. What it gives up in exchange is that a class no longer stands for a
// fixed span — two charts drawn from different rows cut at different values,
// so a quantile ramp is read against its own bar and never against another's.
//
// # Cost
//
// Quantiles cannot be computed from a running minimum and maximum, so this is
// the one colour scale that keeps what it is trained on: every finite value,
// held until the scale is discarded. That is a fixed cost per row rather than
// per scale, and it is the reason the boundaries are recomputed at the end of
// each [ColorScale.Train] rather than lazily — training is serial and drawing
// is not, so a scale that sorted itself the first time a mark asked for a
// colour would be sorting itself from several panels at once.
//
// A chart redrawn from a [github.com/timzifer/refract/data.Stream] should
// therefore hand each frame a scale of its own, or compute the boundaries once
// and pin them with [Threshold]. A scale trained on every frame of a live
// chart has kept every frame of it.
//
// A nil ramp uses [palette.DefaultRamp].
func Quantile(ramp palette.Ramp, classes int, opts ...ColorOption) ClassedColorScale {
	if classes < 1 {
		classes = 1
	}
	return &classed{kind: KindQuantile, base: newColorScale(ramp, false, opts), n: classes}
}

// classed is [Threshold], [Quantize] and [Quantile]. The three differ only in
// where the boundaries come from, so they are one type: everything else — the
// domain, the transform, the undefined colour, which colour a class gets — is
// shared, and writing it twice is how they drift apart.
type classed struct {
	kind  ColorKind
	base  *colorScale
	given []float64 // KindThreshold: the boundaries, ascending.
	n     int       // KindQuantize and KindQuantile: the class count.

	// sample is every value a quantile scale has been trained on, ascending,
	// and cuts the boundaries computed from it.
	sample []float64
	cuts   []float64
}

func (c *classed) Train(vs ...float64) {
	c.base.Train(vs...)
	if c.kind != KindQuantile {
		return
	}
	for _, v := range vs {
		// The same values the domain accepts, so that a log ramp's sample
		// does not hold values the ramp has no colour for.
		if !math.IsNaN(v) && !math.IsInf(v, 0) && c.base.defined(v) {
			c.sample = append(c.sample, v)
		}
	}
	sort.Float64s(c.sample)
	c.recut()
}

// recut recomputes a quantile scale's boundaries from its sample.
//
// It runs at the end of training rather than at the first read because
// training is serial and drawing is not: a scale that sorted itself when a
// mark first asked for a colour would be sorting itself from every panel at
// once. Training is called once per layer with a whole column, so the sort
// costs a handful of passes over the data rather than one per row.
func (c *classed) recut() {
	c.cuts = c.cuts[:0]
	if len(c.sample) == 0 {
		return
	}
	for i := 1; i < c.n; i++ {
		c.cuts = append(c.cuts, quantileOf(c.sample, float64(i)/float64(c.n)))
	}
}

// quantileOf returns the p-th quantile of an ascending sample, interpolating
// between the two order statistics it falls between. It is the definition
// NumPy and R's default use, so a boundary this scale draws is a boundary the
// analysis that chose the class count would have computed.
func quantileOf(sorted []float64, p float64) float64 {
	h := float64(len(sorted)-1) * p
	lo := int(math.Floor(h))
	if lo >= len(sorted)-1 {
		return sorted[len(sorted)-1]
	}
	return sorted[lo] + (h-float64(lo))*(sorted[lo+1]-sorted[lo])
}

// Domain reports the interval the bar covers.
//
// A threshold scale widens the trained domain to cover its outermost breaks,
// because a class nobody has data in is still a class the legend has to show —
// a reader who cannot see the "over the limit" band has no way to know the
// limit was not exceeded rather than not drawn.
func (c *classed) Domain() (float64, float64) {
	lo, hi := c.base.Domain()
	if c.kind != KindThreshold || len(c.given) == 0 {
		return lo, hi
	}
	if !c.base.trained {
		return outerEdges(c.given)
	}
	blo, bhi := c.given[0], c.given[len(c.given)-1]
	if c.base.logDomain() && blo <= 0 {
		// A log domain has no room below zero to widen into.
		blo = lo
	}
	return math.Min(lo, blo), math.Max(hi, bhi)
}

// outerEdges invents the two ends of an untrained threshold domain, one break
// gap outside the outermost breaks, so that every class has width on the bar.
// A single break has no gap to measure, and gets one unit either side.
func outerEdges(b []float64) (float64, float64) {
	if len(b) == 1 {
		return b[0] - 1, b[0] + 1
	}
	step := (b[len(b)-1] - b[0]) / float64(len(b)-1)
	return b[0] - step, b[len(b)-1] + step
}

func (c *classed) Classes() int {
	if c.kind == KindThreshold {
		return len(c.given) + 1
	}
	return c.n
}

func (c *classed) Breaks() []float64 {
	if b, ok := c.stored(); ok {
		return append([]float64(nil), b...)
	}
	lo, hi := c.Domain()
	out := make([]float64, 0, c.n-1)
	for i := 1; i < c.n; i++ {
		out = append(out, c.base.valueIn(lo, hi, float64(i)/float64(c.n)))
	}
	return out
}

// stored returns the boundaries the scale holds, and whether it holds them at
// all: a quantize scale derives its own from the domain on the spot, and an
// untrained quantile scale holds none, which is not the same thing. It
// allocates nothing, which is what lets [classed.classIndex] use it once per
// mark.
func (c *classed) stored() ([]float64, bool) {
	switch c.kind {
	case KindThreshold:
		return c.given, true
	case KindQuantile:
		return c.cuts, true
	}
	return nil, false
}

func (c *classed) Color(v float64) ir.Color {
	if math.IsNaN(v) || math.IsInf(v, 0) || !c.base.defined(v) {
		return c.base.undef
	}
	n := c.Classes()
	// The class is read at its middle rather than at its lower edge, so that
	// n classes are n colours spread across the whole ramp. Reading at the
	// edge would give the last class the second-to-last colour and leave the
	// end of the ramp unused.
	return c.base.ramp.At((float64(c.classIndex(v, n)) + 0.5) / float64(n))
}

// classIndex is which class v falls in. It allocates nothing: a geom calls it
// once per mark.
func (c *classed) classIndex(v float64, n int) int {
	if b, ok := c.stored(); ok {
		// The count of breaks at or below v, so a value equal to a break
		// lands in the class above it.
		return sort.Search(len(b), func(i int) bool { return b[i] > v })
	}
	lo, hi := c.Domain()
	i := int(c.base.positionIn(lo, hi, v) * float64(n))
	if i >= n {
		return n - 1
	}
	if i < 0 {
		return 0
	}
	return i
}

// The bar a classed scale is read off is still an axis: a value sits where its
// number puts it, and the classes are as wide on the bar as they are in the
// data. Equal-height class blocks would be easier to label and would lie about
// where the boundaries are, which for a quantile scale is the one thing the
// bar has to show — that the classes are not equally wide is what "equally
// many observations in each" looks like.

func (c *classed) ColorPosition(v float64) float64 {
	lo, hi := c.Domain()
	return c.base.positionIn(lo, hi, v)
}

func (c *classed) ColorValueAt(t float64) float64 {
	lo, hi := c.Domain()
	return c.base.valueIn(lo, hi, t)
}

func (c *classed) ColorAxis() Scale {
	lo, hi := c.Domain()
	return c.base.axisOver(lo, hi)
}

func (c *classed) ColorTransform() ColorTransform { return c.base.xf.kind }

// DescribeColor writes the scale down. A quantize scale carries its class
// count and a threshold scale its breaks; neither carries the other's, because
// a document that pinned derived boundaries would stop them being derived.
func (c *classed) DescribeColor() ColorDesc {
	d := c.base.DescribeColor()
	d.Kind = c.kind
	d.Center = 0
	switch c.kind {
	case KindThreshold:
		d.Breaks = append([]float64(nil), c.given...)
	default:
		// A quantile scale's boundaries are the data rather than the scale:
		// pinning them would stop them being recomputed the next time the
		// chart is drawn over different rows, which is the whole contract.
		d.Classes = c.n
	}
	return d
}

// sortedBreaks returns the boundaries ascending with duplicates and
// non-finite values removed. A repeated break is a class of zero width, which
// is a class no value can be in and a band no reader can see.
func sortedBreaks(b []float64) []float64 {
	out := make([]float64, 0, len(b))
	for _, v := range b {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			out = append(out, v)
		}
	}
	sort.Float64s(out)
	n := 0
	for i, v := range out {
		if i == 0 || v != out[n-1] {
			out[n] = v
			n++
		}
	}
	return out[:n]
}

// Classed reports whether a colour scale paints a finite set of classes, which
// is what decides whether its colourbar is a gradient or a ladder of steps.
func Classed(s ColorScale) (ClassedColorScale, bool) {
	c, ok := s.(ClassedColorScale)
	return c, ok
}

var (
	_ ClassedColorScale = (*classed)(nil)
	_ ColorTransformer  = (*classed)(nil)
	_ ColorDescriber    = (*classed)(nil)
)
