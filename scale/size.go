package scale

import "math"

// SizeScale maps data values onto mark sizes, the way a [ColorScale] maps them
// onto colours.
//
// It is a third interface rather than a [Scale] for the reason ColorScale is:
// it answers a different question, from a different column, and it does not
// place anything on an axis. A chart commonly has two positional scales, one
// colour scale and one size scale over four columns.
//
// # Area, not radius
//
// A reader compares two circles by how much ink is in them, not by how far
// across they are. Mapping a value to a radius therefore exaggerates it by the
// square: a bubble for twice the value drawn at twice the radius has four times
// the ink and reads as four times the quantity. So this maps a value to an
// *area* and reports the diameter that has it, which is what makes doubling a
// value multiply the diameter by √2.
type SizeScale interface {
	// Train extends the scale's domain to include vs, ignoring NaN and
	// infinities. Calling Train repeatedly accumulates.
	Train(vs ...float64)

	// Domain reports the current data domain.
	Domain() (min, max float64)

	// SetRange sets the diameters the domain's ends map to, in device units.
	// It is the size channel's answer to [Scale.SetRange], and a geom calls it
	// with the sizes its theme asks for before it draws.
	SetRange(min, max float32)

	// Size returns the mark diameter for a value, in device units. A value the
	// scale cannot place — NaN, an infinity — has no size and reports 0, which
	// draws nothing.
	Size(v float64) float32
}

// SizeOption configures a size scale.
type SizeOption func(*sizeScale)

// SizeDomain pins the domain explicitly, disabling training.
func SizeDomain(min, max float64) SizeOption {
	return func(s *sizeScale) {
		s.fixed = true
		s.dmin, s.dmax, s.trained = min, max, true
	}
}

// SizeRange sets the diameters, in device units, that the ends of the domain
// map to. It is what [SizeScale.SetRange] sets, given at construction so that a
// caller can override what the theme would have chosen.
//
// The minimum is 0 by default, and that is the choice that makes the channel
// honest: a floor is a constant added to every area, so a value of zero draws a
// mark of some size and the ratio between two marks stops being the ratio
// between their values. Raise it when the smallest bubbles would otherwise
// vanish, knowing that is the trade being made.
//
// A range given here is *pinned*: [SizeScale.SetRange] no longer moves it. That
// is the same line [Zoomer.SetDomain] draws between a domain the data decides
// and one the caller did — a geom sets the range from its theme on every frame,
// and a caller who named the sizes has not asked for the theme's.
func SizeRange(min, max float32) SizeOption {
	return func(s *sizeScale) {
		s.setSizes(min, max)
		s.pinned = true
	}
}

// SizeZero anchors the area scale at a value other than zero: the value whose
// mark has the minimum size.
//
// Zero is the default and is what makes a bubble's ink proportional to its
// value. Anchoring at the smallest observation instead spreads the sizes over
// the data's own range, which shows small differences between large values —
// and stops the drawing being a proportion at all, so it is asked for rather
// than assumed.
func SizeZero(v float64) SizeOption { return func(s *sizeScale) { s.zero, s.zeroSet = v, true } }

// Size returns a scale that maps values onto mark diameters by area.
//
// The domain runs from zero to the largest value trained into it, and the range
// from a diameter of zero to whatever [SizeRange] or [SizeScale.SetRange]
// names. With those defaults the mapping is exactly d(v) = D·√(v/vmax): the
// area is proportional to the value, so twice the value is twice the ink and
// √2 times the diameter.
//
// Negative values have no area and no mark. A quantity that runs both ways is
// not a size channel — it is a colour channel over a diverging ramp, where a
// deviation of -3 and one of +3 are equally strong and visibly opposite.
func Size(opts ...SizeOption) SizeScale {
	s := &sizeScale{}
	for _, o := range opts {
		o(s)
	}
	return s
}

type sizeScale struct {
	domainRange
	fixed    bool
	zero     float64
	zeroSet  bool
	dlo, dhi float32
	dset     bool
	pinned   bool
}

func (s *sizeScale) Train(vs ...float64) {
	if s.fixed {
		return
	}
	s.domainRange.Train(vs...)
}

// Domain reports the interval the areas are spread over: from the anchor —
// zero unless [SizeZero] moved it — to the largest value seen.
//
// The lower bound is not the smallest observation. A size channel whose domain
// started at the data would draw the smallest bubble at the minimum size and
// the next one at nearly the maximum, which says something about the sample and
// nothing about the values.
func (s *sizeScale) Domain() (float64, float64) {
	if !s.trained {
		return s.anchor(), s.anchor() + 1
	}
	lo, hi := s.anchor(), s.dmax
	if s.fixed {
		lo = s.dmin
	}
	if hi <= lo {
		hi = lo + math.Abs(lo)*0.05 + 0.5
	}
	return lo, hi
}

func (s *sizeScale) anchor() float64 {
	if s.zeroSet {
		return s.zero
	}
	return 0
}

func (s *sizeScale) SetRange(min, max float32) {
	if s.pinned {
		return
	}
	s.setSizes(min, max)
}

func (s *sizeScale) setSizes(min, max float32) {
	if min < 0 {
		min = 0
	}
	if max < min {
		min, max = max, min
	}
	s.dlo, s.dhi, s.dset = min, max, true
}

// sizeRange is the diameters the scale maps into, falling back to an interval
// that draws something so that a scale nobody ranged still produces marks.
func (s *sizeScale) sizeRange() (float32, float32) {
	if !s.dset {
		return 0, DefaultMaxSize
	}
	return s.dlo, s.dhi
}

// DefaultMaxSize is the largest bubble a size scale draws when nothing has set
// its range. A geom overrides it from the theme before it draws; this is what a
// scale used on its own, in a test or by a caller driving one directly, answers
// with.
const DefaultMaxSize = 32

func (s *sizeScale) Size(v float64) float32 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	lo, hi := s.Domain()
	dlo, dhi := s.sizeRange()

	t := 0.0
	if hi > lo {
		t = (v - lo) / (hi - lo)
	}
	if t < 0 {
		return 0 // below the anchor there is no area to draw
	}
	if t > 1 {
		t = 1
	}
	// The interpolation is in area, and the diameter is what has that area.
	// Working in d² rather than in π·d²/4 is the same interpolation with the
	// constant divided out of both ends.
	a0, a1 := float64(dlo)*float64(dlo), float64(dhi)*float64(dhi)
	return float32(math.Sqrt(a0 + t*(a1-a0)))
}

// SizeDesc is a size scale reduced to what configures it, the way
// [ColorDesc] is for a colour scale.
type SizeDesc struct {
	// Min and Max are the domain, meaningful only when Fixed is set.
	Min, Max float64
	Fixed    bool

	// Zero is the value that maps to the minimum size, and ZeroSet reports
	// whether the scale chose it. The pair is [Desc]'s Fixed again and for the
	// same reason: zero is both the default anchor and an anchor somebody may
	// have asked for.
	Zero    float64
	ZeroSet bool

	// MinSize and MaxSize are the diameters the domain's ends map to, and
	// RangeSet reports whether the caller pinned them rather than leaving the
	// theme to say. A range the theme set is not written down: it is a property
	// of how the chart was drawn rather than of the chart.
	MinSize, MaxSize float32
	RangeSet         bool
}

// SizeDescriber is implemented by a size scale that can say what it is.
type SizeDescriber interface {
	// DescribeSize returns the scale's configuration.
	DescribeSize() SizeDesc
}

// DescribeSize reports s's configuration, or ok == false if s cannot describe
// itself.
func DescribeSize(s SizeScale) (SizeDesc, bool) {
	d, ok := s.(SizeDescriber)
	if !ok {
		return SizeDesc{}, false
	}
	return d.DescribeSize(), true
}

// SizeFromDesc builds the size scale d describes.
func SizeFromDesc(d SizeDesc) (SizeScale, error) {
	var opts []SizeOption
	if d.Fixed {
		opts = append(opts, SizeDomain(d.Min, d.Max))
	}
	if d.ZeroSet {
		opts = append(opts, SizeZero(d.Zero))
	}
	if d.RangeSet {
		opts = append(opts, SizeRange(d.MinSize, d.MaxSize))
	}
	return Size(opts...), nil
}

func (s *sizeScale) DescribeSize() SizeDesc {
	d := SizeDesc{Fixed: s.fixed, Zero: s.zero, ZeroSet: s.zeroSet, RangeSet: s.pinned}
	if s.fixed {
		d.Min, d.Max = s.dmin, s.dmax
	}
	if s.pinned {
		d.MinSize, d.MaxSize = s.dlo, s.dhi
	}
	return d
}

var _ SizeDescriber = (*sizeScale)(nil)
