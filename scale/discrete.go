package scale

import (
	"math"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
)

// DiscreteColorScale paints named categories rather than numbers: one colour
// per distinct label, taken from a qualitative palette.
//
// It rides the [ColorScale] interface the way [Categorical] rides [Scale], and
// for the same reason. A layer binds a column to a colour scale through one
// option — [github.com/timzifer/refract/geom.ColorBy] — and a geom that paints
// per mark already reads its colours through [ColorScale.Color]; a category is
// simply a value that had to be encoded before it was a number. Which *guide*
// the layer contributes then follows from which kind of scale it was handed: a
// ramp gets a colourbar, a qualitative scale gets one legend entry per label.
//
// The numeric domain is the category index, so `Color(2)` is the third label's
// colour and a scale trained on the encoded column has the domain [0, n-1].
type DiscreteColorScale interface {
	ColorScale

	// Encode returns the index of a label, registering it if the scale has not
	// seen it before. Registration is in order of first sight.
	Encode(label string) float64

	// ColorOf returns the colour of a label, registering it the same way.
	ColorOf(label string) ir.Color

	// Labels returns the categories in the order they were registered, which
	// is the order a legend lists them in.
	Labels() []string
}

// Qualitative returns a colour scale that gives each distinct label a colour
// from p, in order of first appearance in the data.
//
// First appearance rather than sorted order is the same convention faceting
// and the ordinal axis already use, and ADR 0012 is why it is not negotiable:
// a parallel render must be byte-identical to a serial one, and an order that
// depends on map iteration is an order that depends on scheduling.
//
// A domain larger than the palette wraps rather than fails — [palette.Qualitative.At]
// does — which is the behaviour a layer index already gets. A chart with more
// series than the palette has colours needs a second encoding channel, not a
// longer palette; see [github.com/timzifer/refract/theme.Redundant].
//
// A nil palette uses [palette.Default]. Of the colour options only
// [ColorUndefined] and [ColorReverse] mean anything here: there is no domain to
// pin and no middle to centre, so [ColorDomain] and [ColorCenter] are accepted
// and ignored, exactly as an option a geom has no use for is.
func Qualitative(p palette.Qualitative, opts ...ColorOption) DiscreteColorScale {
	if len(p) == 0 {
		p = palette.Default
	}
	// The shared options are read off a colour scale rather than duplicated,
	// so that ColorUndefined means one thing across both kinds.
	var cfg colorScale
	cfg.undef = ir.Transparent
	for _, o := range opts {
		o(&cfg)
	}
	q := &qualitative{palette: p, undef: cfg.undef, reverse: cfg.reverse, at: map[string]int{}}
	return q
}

type qualitative struct {
	palette palette.Qualitative
	undef   ir.Color
	reverse bool

	labels []string
	at     map[string]int
}

// Train is a no-op with a body: the domain of a discrete scale is the set of
// labels it has been shown, and a label arrives through Encode rather than
// through a number. Accepting the call keeps this a ColorScale, which is what
// lets one option bind either kind.
func (q *qualitative) Train(...float64) {}

// Domain reports the index range, which is what a scale trained on the encoded
// column would report. It is the honest answer for a categorical domain and it
// is what stops a caller drawing a colourbar over one.
func (q *qualitative) Domain() (float64, float64) {
	if len(q.labels) == 0 {
		return 0, 0
	}
	return 0, float64(len(q.labels) - 1)
}

func (q *qualitative) Color(v float64) ir.Color {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return q.undef
	}
	return q.colorAt(int(v))
}

func (q *qualitative) Encode(label string) float64 {
	return float64(q.index(label))
}

func (q *qualitative) ColorOf(label string) ir.Color { return q.colorAt(q.index(label)) }

// Labels returns the registered categories. The slice is the scale's own and
// must not be modified; it is not copied because a legend reads it once per
// frame and copying it would allocate per frame to protect against a caller
// who has no reason to write to it.
func (q *qualitative) Labels() []string { return q.labels }

func (q *qualitative) index(label string) int {
	if i, ok := q.at[label]; ok {
		return i
	}
	i := len(q.labels)
	q.labels = append(q.labels, label)
	q.at[label] = i
	return i
}

// colorAt walks the palette from either end, so that ColorReverse means what
// it means for a ramp: the same colours, handed out the other way round.
func (q *qualitative) colorAt(i int) ir.Color {
	if i < 0 {
		return q.undef
	}
	if q.reverse {
		return q.palette.At(len(q.palette) - 1 - i)
	}
	return q.palette.At(i)
}

// DescribeColor writes the scale down. The palette is named where it is
// registered and spelled out where it is not, exactly as a ramp is.
func (q *qualitative) DescribeColor() ColorDesc {
	d := ColorDesc{Kind: KindQualitative, Reverse: q.reverse, Undefined: q.undef}
	if name, ok := palette.QualitativeName(q.palette); ok {
		d.Ramp = name
	} else {
		d.Colors = append(palette.Ramp(nil), q.palette...)
	}
	return d
}

var (
	_ DiscreteColorScale = (*qualitative)(nil)
	_ ColorDescriber     = (*qualitative)(nil)
)

// Discrete reports whether a colour scale paints categories, which is what
// decides whether a layer using it contributes legend entries or a colourbar.
func Discrete(s ColorScale) (DiscreteColorScale, bool) {
	d, ok := s.(DiscreteColorScale)
	return d, ok
}
