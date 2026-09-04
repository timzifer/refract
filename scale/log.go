package scale

import (
	"math"
	"strconv"
)

// LogOption configures a log scale.
type LogOption func(*logScale)

// LogBase sets the base. The default is 10. Bases at or below 1 are ignored,
// because a logarithm to such a base is not a scale.
func LogBase(b float64) LogOption {
	return func(l *logScale) {
		if b > 1 {
			l.base = b
		}
	}
}

// LogNice expands the domain outwards to whole powers of the base, so the axis
// starts and ends on a labelled decade.
func LogNice() LogOption { return func(l *logScale) { l.nice = true } }

// LogDomain pins the data domain explicitly, disabling training. Both bounds
// must be positive; a non-positive bound is ignored, since a log scale has no
// position for it.
func LogDomain(min, max float64) LogOption {
	return func(l *logScale) {
		if min <= 0 || max <= 0 {
			return
		}
		l.fixed = true
		l.dmin, l.dmax, l.trained = min, max, true
	}
}

// LogFormat overrides tick label formatting.
func LogFormat(fn func(v float64) string) LogOption {
	return func(l *logScale) { l.format = fn }
}

// LogMinorTicks turns the unlabelled subdivisions inside each decade on or
// off. They are on by default: without them a reader has no way to judge where
// 3 sits between 1 and 10, which is the one thing a log axis is bad at.
func LogMinorTicks(show bool) LogOption {
	return func(l *logScale) { l.minor = show }
}

// Log returns a logarithmic scale.
//
// The domain is strictly positive. Training ignores zero and negative values
// the same way it ignores NaN, and [Scale.Map] returns NaN for them, which
// geoms treat as missing data under the layer's own policy — a log chart that
// silently clamped a negative reading to the axis minimum would be inventing
// a measurement. Use [SymLog] for data that genuinely crosses zero.
func Log(opts ...LogOption) Scale {
	l := &logScale{base: 10, minor: true}
	for _, o := range opts {
		o(l)
	}
	return l
}

type logScale struct {
	domainRange
	base   float64
	nice   bool
	fixed  bool
	minor  bool
	format func(float64) string
}

func (l *logScale) Train(vs ...float64) {
	if l.fixed {
		return
	}
	for _, v := range vs {
		if v > 0 {
			l.domainRange.Train(v)
		}
	}
}

// Defined implements [Definite]: only positive values have a position.
func (l *logScale) Defined(v float64) bool { return v > 0 && !math.IsInf(v, 0) }

// effective returns the domain actually mapped, guaranteed positive and
// non-degenerate.
func (l *logScale) effective() (float64, float64) {
	lo, hi := l.dmin, l.dmax
	if !l.trained || lo <= 0 || hi <= 0 {
		lo, hi = 1, 10
	}
	if lo == hi {
		// One repeated value still has to render. A decade either side is the
		// log-space equivalent of the linear scale's 5% padding.
		lo, hi = lo/l.base, hi*l.base
	}
	if !l.nice {
		return lo, hi
	}
	return math.Pow(l.base, math.Floor(l.logOf(lo))), math.Pow(l.base, math.Ceil(l.logOf(hi)))
}

func (l *logScale) logOf(v float64) float64 { return math.Log(v) / math.Log(l.base) }

func (l *logScale) Domain() (float64, float64) { return l.effective() }

func (l *logScale) Map(v float64) float32 {
	if !l.Defined(v) {
		return float32(math.NaN())
	}
	lo, hi := l.effective()
	rlo, rhi := l.rangeOf()
	if hi == lo {
		return rlo
	}
	// Snap the endpoints, for the same reason the linear scale does: a tick on
	// the plot edge must not land a float32 ulp outside it.
	switch v {
	case lo:
		return rlo
	case hi:
		return rhi
	}
	t := (math.Log(v) - math.Log(lo)) / (math.Log(hi) - math.Log(lo))
	return place(rlo, rhi, t)
}

func (l *logScale) Invert(pos float32) float64 {
	lo, hi := l.effective()
	rlo, rhi := l.rangeOf()
	if rhi == rlo {
		return lo
	}
	t := float64((pos - rlo) / (rhi - rlo))
	return math.Exp(math.Log(lo) + t*(math.Log(hi)-math.Log(lo)))
}

func (l *logScale) Ticks(want int) []Tick {
	if want < 2 {
		want = 2
	}
	lo, hi := l.effective()
	first := int(math.Floor(l.logOf(lo) + logEps))
	last := int(math.Ceil(l.logOf(hi) - logEps))

	// One decade per tick is the natural sequence; when the domain spans more
	// decades than the axis has room for, step whole decades rather than
	// thinning arbitrarily, so the labels stay a geometric sequence.
	step := 1
	if n := last - first + 1; n > want {
		step = (n + want - 1) / want
	}

	fmtFn := l.format
	if fmtFn == nil {
		fmtFn = formatLog
	}

	out := make([]Tick, 0, (last-first+1)*2)
	for e := first; e <= last; e++ {
		p := math.Pow(l.base, float64(e))
		if (e-first)%step == 0 {
			if t, ok := l.tickAt(p, fmtFn(p), false); ok {
				out = append(out, t)
			}
		} else if t, ok := l.tickAt(p, "", true); ok {
			// A decade the labels skipped still gets a mark. Without it a
			// reader has to measure to find out whether the gap between two
			// labels is one decade or three.
			out = append(out, t)
		}
		if !l.minor || step != 1 {
			continue
		}
		// Subdivide the decade. Only integer multiples up to the base are
		// meaningful, and only a base small enough to have them: base 2 has no
		// interior multiples at all.
		for m := 2.0; m < l.base; m++ {
			if t, ok := l.tickAt(p*m, "", true); ok {
				out = append(out, t)
			}
		}
	}
	return out
}

// tickAt builds a tick if the value falls inside the mapped domain. Decades
// are generated by flooring and ceiling the domain, so the outermost ones can
// sit outside it when the scale is not niced.
func (l *logScale) tickAt(v float64, label string, minor bool) (Tick, bool) {
	lo, hi := l.effective()
	if v < lo*(1-logEps) || v > hi*(1+logEps) {
		return Tick{}, false
	}
	return Tick{Value: v, Pos: l.Map(v), Label: label, Minor: minor}, true
}

// logEps absorbs the rounding in Log(base^k)/Log(base), which is not exactly k
// for every base and exponent. It is far smaller than any spacing a reader
// could perceive and far larger than the error it exists to hide.
const logEps = 1e-9

// formatLog labels a decade. Plain digits stay readable for about ten decades
// either side of one; past that they become a row of zeros nobody counts, and
// exponent notation is clearer.
func formatLog(v float64) string {
	a := math.Abs(v)
	if a != 0 && (a < 1e-5 || a >= 1e7) {
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
