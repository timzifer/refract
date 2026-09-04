package scale

import (
	"math"
	"strconv"
)

// LinearOption configures a linear scale.
type LinearOption func(*linear)

// Nice expands the domain outwards to the tick sequence's own bounds, so the
// axis starts and ends on a labelled tick rather than on the extreme data
// value. This is what most charts want and what CONCEPT.md §13 shows.
func Nice() LinearOption { return func(l *linear) { l.nice = true } }

// Zero forces the domain to include zero. Bar charts need this: a bar chart
// whose baseline is off-screen misleads.
func Zero() LinearOption { return func(l *linear) { l.zero = true } }

// Domain pins the data domain explicitly, disabling training.
func Domain(min, max float64) LinearOption {
	return func(l *linear) {
		l.fixed = true
		l.dmin, l.dmax, l.trained = min, max, true
	}
}

// Format overrides tick label formatting.
func Format(fn func(v float64) string) LinearOption {
	return func(l *linear) { l.format = fn }
}

// Linear returns a linear scale.
func Linear(opts ...LinearOption) Scale {
	l := &linear{}
	for _, o := range opts {
		o(l)
	}
	return l
}

type linear struct {
	domainRange
	nice   bool
	zero   bool
	fixed  bool
	pinned bool
	format func(float64) string

	// cached nicing, invalidated whenever the domain changes
	nicedFor [2]float64
	niced    labelling
	hasNiced bool
}

func (l *linear) Train(vs ...float64) {
	if l.fixed {
		return
	}
	l.domainRange.Train(vs...)
	l.hasNiced = false
}

func (l *linear) effective() (float64, float64) {
	// A pinned domain is the domain. Zero-forcing and nicing are choices about
	// how to frame data; a view someone dragged into place is not data, and an
	// axis that snapped to round numbers after every wheel notch would not
	// follow the pointer. See [Zoomer].
	if l.pinned {
		return l.dmin, l.dmax
	}
	lo, hi := l.span()
	if l.zero {
		lo, hi = math.Min(lo, 0), math.Max(hi, 0)
		if lo == hi {
			hi = lo + 1
		}
	}
	if !l.nice {
		return lo, hi
	}
	lab := l.labelling(lo, hi, defaultTickCount)
	return math.Min(lo, lab.min), math.Max(hi, lab.max)
}

// labelling memoises the tick search, which layout calls repeatedly while
// sizing margins.
func (l *linear) labelling(lo, hi float64, want int) labelling {
	if l.hasNiced && l.nicedFor == [2]float64{lo, hi} {
		return l.niced
	}
	lab := extendedWilkinson(lo, hi, want, false)
	l.nicedFor, l.niced, l.hasNiced = [2]float64{lo, hi}, lab, true
	return lab
}

// defaultTickCount is the tick count the nicing pass targets. Layout may ask
// for a different count later; nicing has to commit to one before it knows the
// axis length, and five is a reasonable axis at any size refract renders.
const defaultTickCount = 5

func (l *linear) Domain() (float64, float64) { return l.effective() }

func (l *linear) Map(v float64) float32 {
	lo, hi := l.effective()
	rlo, rhi := l.rangeOf()
	if hi == lo {
		return rlo
	}
	// Snap the endpoints. Without this, mapping the domain maximum lands a
	// float32 ulp short of the range end, and every "is this tick inside the
	// plot" test at the boundary answers no.
	switch v {
	case lo:
		return rlo
	case hi:
		return rhi
	}
	t := (v - lo) / (hi - lo)
	return place(rlo, rhi, t)
}

func (l *linear) Invert(pos float32) float64 {
	lo, hi := l.effective()
	rlo, rhi := l.rangeOf()
	if rhi == rlo {
		return lo
	}
	t := float64((pos - rlo) / (rhi - rlo))
	return lo + t*(hi-lo)
}

func (l *linear) Ticks(want int) []Tick {
	lo, hi := l.effective()
	lab := extendedWilkinson(lo, hi, want, false)
	vals := lab.values()
	fmtFn := l.format
	if fmtFn == nil {
		fmtFn = formatterFor(lab.step)
	}
	out := make([]Tick, 0, len(vals))
	for _, v := range vals {
		if v < lo-1e-9*math.Abs(hi-lo) || v > hi+1e-9*math.Abs(hi-lo) {
			continue
		}
		out = append(out, Tick{Value: v, Pos: l.Map(v), Label: fmtFn(v)})
	}
	return out
}

// formatterFor picks a label format from the tick step: enough decimals to
// distinguish adjacent ticks, and scientific notation once plain digits get
// unreadable. Deriving this from the step rather than from each value is what
// keeps a column of labels aligned and consistent.
func formatterFor(step float64) func(float64) string {
	step = math.Abs(step)
	if step == 0 || math.IsNaN(step) || math.IsInf(step, 0) {
		return func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
	}
	exp := int(math.Floor(math.Log10(step)))
	if exp < -6 || exp > 8 {
		return func(v float64) string { return strconv.FormatFloat(v, 'e', 2, 64) }
	}
	// Use as many decimals as the step itself needs, not as many as its
	// magnitude suggests: a step of 2.5 is a one-decimal step even though it is
	// larger than one, and rounding it to whole numbers would label the ticks
	// 0, 2, 5, 8, 10 — evenly spaced marks with unevenly spaced numbers.
	decimals := 0
	for scaled := step; decimals < maxDecimals; decimals++ {
		if math.Abs(scaled-math.Round(scaled)) < stepEps*math.Max(1, math.Abs(scaled)) {
			break
		}
		scaled *= 10
	}
	return func(v float64) string {
		// Snap values that are a hair off an exact tick, so 0.30000000000000004
		// prints as 0.3 and -0 prints as 0.
		s := strconv.FormatFloat(v, 'f', decimals, 64)
		if s == "-0" || (decimals > 0 && s == "-0."+zeros(decimals)) {
			s = s[1:]
		}
		return s
	}
}

// maxDecimals caps label precision. Beyond six decimals a linear axis is
// better served by scientific notation, which formatterFor already switches to
// by exponent.
const maxDecimals = 6

// stepEps is the relative tolerance for "this step is a whole number at this
// scale". It absorbs the float error in a step like 0.1 without ever accepting
// a genuinely fractional step as whole.
const stepEps = 1e-9

func zeros(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '0'
	}
	return string(b)
}
