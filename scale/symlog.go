package scale

import "math"

// SymLogOption configures a symmetric log scale.
type SymLogOption func(*symlogScale)

// SymLogBase sets the base. The default is 10.
func SymLogBase(b float64) SymLogOption {
	return func(s *symlogScale) {
		if b > 1 {
			s.base = b
		}
	}
}

// SymLogThreshold sets the half-width of the linear region around zero. The
// default is 1.
//
// Choose it to match the smallest magnitude that carries meaning: below the
// threshold the scale is linear, above it logarithmic, and the join is smooth.
func SymLogThreshold(t float64) SymLogOption {
	return func(s *symlogScale) {
		if t > 0 {
			s.thr = t
		}
	}
}

// SymLogDomain pins the data domain explicitly, disabling training.
func SymLogDomain(min, max float64) SymLogOption {
	return func(s *symlogScale) {
		s.fixed = true
		s.dmin, s.dmax, s.trained = min, max, true
	}
}

// SymLogFormat overrides tick label formatting.
func SymLogFormat(fn func(v float64) string) SymLogOption {
	return func(s *symlogScale) { s.format = fn }
}

// SymLogMinorTicks turns the unlabelled subdivisions inside each decade on or
// off. They are on by default.
func SymLogMinorTicks(show bool) SymLogOption {
	return func(s *symlogScale) { s.minor = show }
}

// SymLog returns a symmetric log scale: linear within a threshold of zero,
// logarithmic outside it, and defined for every finite value including
// negative ones.
//
// This is the scale for data that spans orders of magnitude *and* crosses
// zero — a signed residual, a profit and loss, a delta that is sometimes tiny.
// A plain [Log] cannot show such data at all and a [Linear] one buries
// everything small.
//
// The transform is sign(v)·log_base(1 + |v|/threshold), which is smooth at the
// origin rather than merely continuous: there is no visible kink where the
// linear region hands over to the logarithmic one.
func SymLog(opts ...SymLogOption) Scale {
	s := &symlogScale{base: 10, thr: 1, minor: true}
	for _, o := range opts {
		o(s)
	}
	return s
}

type symlogScale struct {
	domainRange
	base   float64
	thr    float64
	fixed  bool
	minor  bool
	format func(float64) string
}

func (s *symlogScale) Train(vs ...float64) {
	if s.fixed {
		return
	}
	s.domainRange.Train(vs...)
}

// forward is the symlog transform.
func (s *symlogScale) forward(v float64) float64 {
	return math.Copysign(math.Log1p(math.Abs(v)/s.thr)/math.Log(s.base), v)
}

// inverse undoes forward.
func (s *symlogScale) inverse(u float64) float64 {
	return math.Copysign(s.thr*math.Expm1(math.Abs(u)*math.Log(s.base)), u)
}

func (s *symlogScale) Domain() (float64, float64) { return s.span() }

func (s *symlogScale) Map(v float64) float32 {
	lo, hi := s.span()
	rlo, rhi := s.rangeOf()
	if hi == lo {
		return rlo
	}
	switch v {
	case lo:
		return rlo
	case hi:
		return rhi
	}
	flo, fhi := s.forward(lo), s.forward(hi)
	if fhi == flo {
		return rlo
	}
	t := (s.forward(v) - flo) / (fhi - flo)
	return rlo + float32(t)*(rhi-rlo)
}

func (s *symlogScale) Invert(pos float32) float64 {
	lo, hi := s.span()
	rlo, rhi := s.rangeOf()
	if rhi == rlo {
		return lo
	}
	flo, fhi := s.forward(lo), s.forward(hi)
	t := float64((pos - rlo) / (rhi - rlo))
	return s.inverse(flo + t*(fhi-flo))
}

func (s *symlogScale) Ticks(want int) []Tick {
	if want < 2 {
		want = 2
	}
	lo, hi := s.span()
	fmtFn := s.format
	if fmtFn == nil {
		fmtFn = formatLog
	}

	// Candidate magnitudes: the threshold, then a decade at a time until the
	// domain is covered. Mirrored about zero, with zero itself always present
	// — on a scale built to show sign changes, the axis must say where the
	// sign changes.
	mags := []float64{s.thr}
	limit := math.Max(math.Abs(lo), math.Abs(hi))
	for m := s.thr * s.base; m <= limit*s.base; m *= s.base {
		mags = append(mags, m)
		if len(mags) > maxSymLogDecades {
			break
		}
	}

	// Two arms plus zero, so each arm gets about half the budget.
	step := 1
	if n := 2*len(mags) + 1; n > want {
		step = (n + want - 1) / want
	}

	out := make([]Tick, 0, 2*len(mags)+1)
	for i := len(mags) - 1; i >= 0; i-- {
		if i%step != 0 {
			continue
		}
		v := -mags[i]
		if t, ok := s.tickAt(v, fmtFn(v), false); ok {
			out = append(out, t)
		}
	}
	if t, ok := s.tickAt(0, fmtFn(0), false); ok {
		out = append(out, t)
	}
	for i := range mags {
		if i%step != 0 {
			continue
		}
		if t, ok := s.tickAt(mags[i], fmtFn(mags[i]), false); ok {
			out = append(out, t)
		}
	}
	if !s.minor || step != 1 {
		return out
	}
	return s.withMinor(out, mags)
}

// withMinor merges the unlabelled subdivisions of each decade into an already
// ordered major sequence.
func (s *symlogScale) withMinor(major []Tick, mags []float64) []Tick {
	minor := make([]Tick, 0, len(mags)*8)
	for _, m := range mags {
		for k := 2.0; k < s.base; k++ {
			if t, ok := s.tickAt(m*k, "", true); ok {
				minor = append(minor, t)
			}
			if t, ok := s.tickAt(-m*k, "", true); ok {
				minor = append(minor, t)
			}
		}
	}
	out := append(major, minor...)
	sortTicks(out)
	return out
}

func (s *symlogScale) tickAt(v float64, label string, minor bool) (Tick, bool) {
	lo, hi := s.span()
	if v < lo || v > hi {
		return Tick{}, false
	}
	return Tick{Value: v, Pos: s.Map(v), Label: label, Minor: minor}, true
}

// maxSymLogDecades caps the decade sweep. A domain spanning more than this
// many decades either side of the threshold is beyond what any axis can label,
// and the cap keeps a pathological range from generating an unbounded
// sequence.
const maxSymLogDecades = 40
