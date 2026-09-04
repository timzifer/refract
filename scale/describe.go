package scale

import (
	"fmt"
	"time"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
)

// Kind names a scale's type. It is the word a written-down chart carries in
// place of the constructor that built the scale.
type Kind string

// The scale kinds.
const (
	KindLinear  Kind = "linear"
	KindLog     Kind = "log"
	KindSymLog  Kind = "symlog"
	KindTime    Kind = "time"
	KindOrdinal Kind = "ordinal"
)

// Desc is a scale reduced to what configures it.
//
// It is the same bargain [github.com/timzifer/refract/geom.Desc] makes: a
// Scale is an interface over an unexported type, which is right for mapping
// values and useless for writing one down, so every scale here answers
// [Describer] and [FromDesc] builds one back.
//
// # What does not survive
//
// A tick formatter is a Go function. [Format], [LogFormat], [SymLogFormat] and
// [TimeFormat] therefore have no place in a Desc, and a scale carrying one
// says so through Formatted — a chart that is written down and read back
// labels its ticks the standard way. Nothing else about the scale is lost.
type Desc struct {
	// Kind is which scale this is.
	Kind Kind

	// Min and Max are the domain. They are meaningful only when Fixed is set;
	// a trained domain belongs to the data, not to the scale.
	Min, Max float64
	// Fixed reports a domain pinned at construction by [Domain], [LogDomain]
	// or [SymLogDomain], or afterwards by [Zoomer.SetDomain].
	Fixed bool

	// Nice and Zero are the linear and log framing options.
	Nice, Zero bool
	// Base is the log or symlog base, and Threshold the symlog linear region.
	Base, Threshold float64
	// MinorTicks reports the unlabelled subdivisions of a log or symlog axis.
	MinorTicks bool

	// Origin is a time scale's epoch, in Unix nanoseconds: the instant its
	// domain values are measured from. It is zero for every other kind, and
	// for a time scale left on the Unix epoch. It is not a formatting choice
	// like Location is — it decides what the numbers in Min and Max *mean* —
	// so a document that dropped it would read back a different axis. See
	// [Origin].
	Origin int64

	// Categories is an ordinal scale's fixed category set, empty when it
	// discovers its categories from the data. Padding is the fraction of each
	// slot left blank.
	Categories []string
	Padding    float64

	// Location is a time scale's zone, by IANA name.
	Location string

	// Formatted reports a scale carrying a formatter that a Desc cannot hold.
	Formatted bool
}

// Describer is implemented by a scale that can say what it is. It is optional:
// a third-party scale that does not implement it still draws, and is simply
// not serializable.
type Describer interface {
	// Describe returns the scale's configuration.
	Describe() Desc
}

// Describe reports s's configuration, or ok == false if s cannot describe
// itself.
func Describe(s Scale) (Desc, bool) {
	d, ok := s.(Describer)
	if !ok {
		return Desc{}, false
	}
	return d.Describe(), true
}

// ErrUnknownKind reports a Desc naming a scale this package does not have.
var ErrUnknownKind = fmt.Errorf("refract/scale: unknown scale kind")

// FromDesc builds the scale d describes.
func FromDesc(d Desc) (Scale, error) {
	switch d.Kind {
	case KindLinear, "":
		var opts []LinearOption
		if d.Nice {
			opts = append(opts, Nice())
		}
		if d.Zero {
			opts = append(opts, Zero())
		}
		if d.Fixed {
			opts = append(opts, Domain(d.Min, d.Max))
		}
		return Linear(opts...), nil

	case KindLog:
		opts := []LogOption{LogMinorTicks(d.MinorTicks)}
		if d.Base > 1 {
			opts = append(opts, LogBase(d.Base))
		}
		if d.Nice {
			opts = append(opts, LogNice())
		}
		if d.Fixed {
			opts = append(opts, LogDomain(d.Min, d.Max))
		}
		return Log(opts...), nil

	case KindSymLog:
		opts := []SymLogOption{SymLogMinorTicks(d.MinorTicks)}
		if d.Base > 1 {
			opts = append(opts, SymLogBase(d.Base))
		}
		if d.Threshold > 0 {
			opts = append(opts, SymLogThreshold(d.Threshold))
		}
		if d.Fixed {
			opts = append(opts, SymLogDomain(d.Min, d.Max))
		}
		return SymLog(opts...), nil

	case KindTime:
		var opts []TimeOption
		if d.Location != "" {
			loc, err := time.LoadLocation(d.Location)
			if err != nil {
				return nil, fmt.Errorf("refract/scale: time zone %q: %w", d.Location, err)
			}
			opts = append(opts, In(loc))
		}
		if d.Origin != 0 {
			opts = append(opts, Origin(time.Unix(0, d.Origin)))
		}
		s := Time(opts...)
		if d.Fixed {
			s.(Zoomer).SetDomain(d.Min, d.Max)
		}
		return s, nil

	case KindOrdinal:
		opts := []OrdinalOption{OrdinalPadding(d.Padding)}
		if len(d.Categories) > 0 {
			opts = append(opts, Categories(d.Categories...))
		}
		return Ordinal(opts...), nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownKind, d.Kind)
}

func (l *linear) Describe() Desc {
	d := Desc{Kind: KindLinear, Nice: l.nice, Zero: l.zero, Fixed: l.fixed, Formatted: l.format != nil}
	if l.fixed {
		d.Min, d.Max = l.dmin, l.dmax
	}
	return d
}

func (l *logScale) Describe() Desc {
	d := Desc{
		Kind: KindLog, Base: l.base, Nice: l.nice, Fixed: l.fixed,
		MinorTicks: l.minor, Formatted: l.format != nil,
	}
	if l.fixed {
		d.Min, d.Max = l.dmin, l.dmax
	}
	return d
}

func (s *symlogScale) Describe() Desc {
	d := Desc{
		Kind: KindSymLog, Base: s.base, Threshold: s.thr, Fixed: s.fixed,
		MinorTicks: s.minor, Formatted: s.format != nil,
	}
	if s.fixed {
		d.Min, d.Max = s.dmin, s.dmax
	}
	return d
}

func (s *timeScale) Describe() Desc {
	d := Desc{Kind: KindTime, Fixed: s.fixed, Origin: s.origin, Formatted: s.format != nil}
	if s.loc != nil {
		d.Location = s.loc.String()
	}
	if s.fixed {
		d.Min, d.Max = s.dmin, s.dmax
	}
	return d
}

// Describe on an ordinal scale reports a fixed category set and not a
// discovered one. The distinction is the same one [Cloner] draws: a fixed list
// is the axis, a discovered one is the data, and the data is written down
// separately.
func (o *ordinal) Describe() Desc {
	d := Desc{Kind: KindOrdinal, Padding: o.padding}
	if o.fixed {
		d.Categories = append([]string(nil), o.labels...)
	}
	return d
}

var (
	_ Describer = (*linear)(nil)
	_ Describer = (*logScale)(nil)
	_ Describer = (*symlogScale)(nil)
	_ Describer = (*timeScale)(nil)
	_ Describer = (*ordinal)(nil)

	_ Zoomer = (*linear)(nil)
	_ Zoomer = (*logScale)(nil)
	_ Zoomer = (*symlogScale)(nil)
	_ Zoomer = (*timeScale)(nil)
)

// ColorKind names a colour scale's type.
type ColorKind string

// The colour scale kinds.
const (
	KindSequential ColorKind = "sequential"
	KindDiverging  ColorKind = "diverging"
)

// ColorDesc is a colour scale reduced to what configures it.
//
// Ramp is a name from [palette.RampByName] rather than a list of colours: a
// registered ramp is a word, and a chart that named one should read back as
// having named it. A ramp nobody registered has no name, so Colors carries it
// literally instead — an unregistered ramp is still a ramp, and losing it
// would be worse than spelling it out.
type ColorDesc struct {
	Kind      ColorKind
	Ramp      string
	Colors    palette.Ramp
	Min, Max  float64
	Fixed     bool
	Center    float64
	Reverse   bool
	Undefined ir.Color
}

// ColorDescriber is implemented by a colour scale that can say what it is.
type ColorDescriber interface {
	// DescribeColor returns the scale's configuration.
	DescribeColor() ColorDesc
}

// DescribeColor reports s's configuration, or ok == false if s cannot describe
// itself.
func DescribeColor(s ColorScale) (ColorDesc, bool) {
	d, ok := s.(ColorDescriber)
	if !ok {
		return ColorDesc{}, false
	}
	return d.DescribeColor(), true
}

// ColorFromDesc builds the colour scale d describes.
func ColorFromDesc(d ColorDesc) (ColorScale, error) {
	ramp := d.Colors
	if d.Ramp != "" {
		r, ok := palette.RampByName(d.Ramp)
		if !ok {
			return nil, fmt.Errorf("refract/scale: unknown colour ramp %q", d.Ramp)
		}
		ramp = r
	}
	opts := []ColorOption{ColorUndefined(d.Undefined)}
	if d.Fixed {
		opts = append(opts, ColorDomain(d.Min, d.Max))
	}
	if d.Reverse {
		opts = append(opts, ColorReverse())
	}
	switch d.Kind {
	case KindDiverging:
		return Diverging(ramp, append(opts, ColorCenter(d.Center))...), nil
	case KindSequential, "":
		return Sequential(ramp, opts...), nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownKind, d.Kind)
}

// DescribeColor reports the ramp the scale was *given*, not the one it holds:
// a reversed scale reverses its ramp once at construction, so naming the ramp
// it ended up with would produce a spec that reverses it twice.
func (c *colorScale) DescribeColor() ColorDesc {
	d := ColorDesc{
		Kind: KindSequential, Center: c.center, Reverse: c.reverse,
		Fixed: c.fixed, Undefined: c.undef,
	}
	if c.diverging {
		d.Kind = KindDiverging
	}
	if c.fixed {
		d.Min, d.Max = c.dmin, c.dmax
	}
	ramp := c.ramp
	if c.reverse {
		ramp = ramp.Reverse()
	}
	if name, ok := palette.RampName(ramp); ok {
		d.Ramp = name
	} else {
		d.Colors = append(palette.Ramp(nil), ramp...)
	}
	return d
}

var _ ColorDescriber = (*colorScale)(nil)
