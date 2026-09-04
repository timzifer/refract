package scale

import (
	"math"
	"time"
)

// TimeOption configures a time scale.
type TimeOption func(*timeScale)

// In sets the location used to compute calendar ticks and format labels.
// The default is time.UTC, so that a chart renders identically wherever it is
// built — a server rendering in the client's local zone is a bug, not a
// feature.
func In(loc *time.Location) TimeOption {
	return func(t *timeScale) {
		if loc != nil {
			t.loc = loc
		}
	}
}

// Origin sets the instant the scale measures its domain from, so that a
// domain value is nanoseconds since t rather than nanoseconds since 1970.
//
// It is what makes a deep zoom exact. A float64 holds 53 bits of mantissa, and
// a Unix nanosecond count in this century needs 61 — so two instants a
// nanosecond apart become the *same* float64, and an axis zoomed to a
// microsecond window has nothing left to separate them with. Measured from an
// origin near the data, the same two instants are 1.0 apart, and stay whole
// numbers of nanoseconds for the hundred days either side of it that a
// float64 counts exactly.
//
// The origin is part of the scale's domain space: every value handed to
// [Scale.Map] or [Scale.Train] on this scale, and every value it returns from
// [Scale.Invert] or [Scale.Domain], is measured from it. Use [Temporal] —
// or the package helpers [ValueOf] and [InstantOf] — to convert, rather than
// [Nanos] and [FromNanos], which are the origin-free pair and stay so.
// A geom reading a time column does this for you.
//
// The default origin is the Unix epoch, which is exactly [Nanos].
func Origin(t time.Time) TimeOption {
	return func(s *timeScale) { s.origin = t.UnixNano() }
}

// TimeFormat overrides tick label formatting. The unit the tick sequence
// settled on is passed so a caller can vary detail with zoom level.
func TimeFormat(fn func(t time.Time, unit time.Duration) string) TimeOption {
	return func(t *timeScale) { t.format = fn }
}

// Time returns a time scale.
//
// Its domain is carried as float64 Unix nanoseconds, so it satisfies the same
// Scale interface as every other scale and geoms need no special case. Use
// [Nanos] and [FromNanos] to convert.
func Time(opts ...TimeOption) Scale {
	t := &timeScale{loc: time.UTC}
	for _, o := range opts {
		o(t)
	}
	return t
}

// Nanos converts a time to the float64 domain value a time scale uses.
func Nanos(t time.Time) float64 { return float64(t.UnixNano()) }

// FromNanos converts a time scale's domain value back to a time.
func FromNanos(v float64) time.Time { return time.Unix(0, int64(v)) }

// Temporal is implemented by scales whose domain is time.
//
// It is the seam between an exact timestamp and the float64 a Scale maps: a
// scale that measures from an [Origin] converts across it in int64 and loses
// nothing, and one that does not is exactly [Nanos] and [FromNanos]. Anything
// turning a timestamp into a domain value — a geom reading a time column, a
// document writing an axis down — goes through it rather than assuming the
// Unix epoch, which is what [ValueOf] and [InstantOf] are for.
//
// It is an optional interface, like every other in this package: a scale that
// does not implement it is not a time axis.
type Temporal interface {
	// Value converts an instant into the scale's domain space.
	Value(t time.Time) float64

	// Instant converts a domain value back into an instant. It is the inverse
	// of Value.
	Instant(v float64) time.Time
}

// ValueOf converts an instant into s's domain space, falling back to [Nanos]
// for a scale that is not [Temporal] — including a nil one, which is what a
// geom reading a column for a colour scale has.
func ValueOf(s Scale, t time.Time) float64 {
	if ts, ok := s.(Temporal); ok {
		return ts.Value(t)
	}
	return Nanos(t)
}

// InstantOf converts a domain value of s back into an instant, falling back to
// [FromNanos] for a scale that is not [Temporal].
func InstantOf(s Scale, v float64) time.Time {
	if ts, ok := s.(Temporal); ok {
		return ts.Instant(v)
	}
	return FromNanos(v)
}

type timeScale struct {
	domainRange
	loc    *time.Location
	fixed  bool
	origin int64 // the instant the domain is measured from, in Unix nanoseconds
	format func(time.Time, time.Duration) string
}

// Value converts an instant into this scale's domain space, exactly: the
// subtraction happens in int64, so no precision is lost before the float64.
func (s *timeScale) Value(t time.Time) float64 { return float64(t.UnixNano() - s.origin) }

// Instant converts a domain value back into an instant.
func (s *timeScale) Instant(v float64) time.Time { return time.Unix(0, s.origin+int64(v)) }

// Train ignores a pinned domain, so that a view set by [Zoomer.SetDomain]
// survives the next render's training pass.
func (s *timeScale) Train(vs ...float64) {
	if s.fixed {
		return
	}
	s.domainRange.Train(vs...)
}

func (s *timeScale) Domain() (float64, float64) { return s.span() }

func (s *timeScale) Map(v float64) float32 {
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
	return place(rlo, rhi, (v-lo)/(hi-lo))
}

func (s *timeScale) Invert(pos float32) float64 {
	lo, hi := s.span()
	rlo, rhi := s.rangeOf()
	if rhi == rlo {
		return lo
	}
	return lo + float64((pos-rlo)/(rhi-rlo))*(hi-lo)
}

// timeUnit is a candidate tick spacing, with the label format that suits it.
type timeUnit struct {
	every  time.Duration // 0 for calendar units handled specially
	months int           // >0 for month-stepped units
	years  int           // >0 for year-stepped units
	layout string
}

// timeUnits are the candidate spacings, ascending. The ladder is the usual
// one: subdivisions that a reader can count in their head, never an arbitrary
// "1.7 hours".
var timeUnits = []timeUnit{
	{every: time.Millisecond, layout: "05.000"},
	{every: 10 * time.Millisecond, layout: "05.00"},
	{every: 100 * time.Millisecond, layout: "05.0"},
	{every: time.Second, layout: "15:04:05"},
	{every: 5 * time.Second, layout: "15:04:05"},
	{every: 15 * time.Second, layout: "15:04:05"},
	{every: 30 * time.Second, layout: "15:04:05"},
	{every: time.Minute, layout: "15:04"},
	{every: 5 * time.Minute, layout: "15:04"},
	{every: 15 * time.Minute, layout: "15:04"},
	{every: 30 * time.Minute, layout: "15:04"},
	{every: time.Hour, layout: "15:04"},
	{every: 3 * time.Hour, layout: "Jan 2 15:04"},
	{every: 6 * time.Hour, layout: "Jan 2 15:04"},
	{every: 12 * time.Hour, layout: "Jan 2 15:04"},
	{every: 24 * time.Hour, layout: "Jan 2"},
	{every: 48 * time.Hour, layout: "Jan 2"},
	{every: 7 * 24 * time.Hour, layout: "Jan 2"},
	{months: 1, layout: "Jan 2006"},
	{months: 3, layout: "Jan 2006"},
	{months: 6, layout: "Jan 2006"},
	{years: 1, layout: "2006"},
	{years: 2, layout: "2006"},
	{years: 5, layout: "2006"},
	{years: 10, layout: "2006"},
	{years: 25, layout: "2006"},
	{years: 50, layout: "2006"},
	{years: 100, layout: "2006"},
}

func (s *timeScale) Ticks(want int) []Tick {
	if want < 2 {
		want = 2
	}
	lo, hi := s.span()
	start, end := s.Instant(lo).In(s.loc), s.Instant(hi).In(s.loc)
	span := end.Sub(start)
	if span <= 0 {
		return []Tick{{Value: lo, Pos: s.Map(lo), Label: s.label(start, timeUnits[3])}}
	}

	u := s.pick(span, want)
	times := s.walk(start, end, u)

	out := make([]Tick, 0, len(times))
	for _, t := range times {
		v := s.Value(t)
		if v < lo || v > hi {
			continue
		}
		out = append(out, Tick{Value: v, Pos: s.Map(v), Label: s.label(t, u)})
	}
	return out
}

// approx reports a unit's nominal duration, used only to tell a custom
// formatter how coarse the axis is.
func (u timeUnit) approx() time.Duration {
	switch {
	case u.years > 0:
		return time.Duration(u.years) * 365 * 24 * time.Hour
	case u.months > 0:
		return time.Duration(u.months) * 30 * 24 * time.Hour
	default:
		return u.every
	}
}

// pick chooses the unit whose resulting tick count is closest to want.
func (s *timeScale) pick(span time.Duration, want int) timeUnit {
	best := timeUnits[len(timeUnits)-1]
	bestErr := math.Inf(1)
	for _, u := range timeUnits {
		n := float64(span) / float64(u.approx())
		if n < 1 {
			continue
		}
		// Compare in log space so "half as many ticks as asked" and "twice as
		// many" are penalised equally.
		e := math.Abs(math.Log(n / float64(want-1)))
		if e < bestErr {
			bestErr, best = e, u
		}
	}
	return best
}

// walk enumerates tick times from the first aligned instant at or before start
// through end.
func (s *timeScale) walk(start, end time.Time, u timeUnit) []time.Time {
	var out []time.Time
	const guard = 4096 // never emit more ticks than any axis could show

	switch {
	case u.years > 0:
		y := start.Year() / u.years * u.years
		for t := time.Date(y, time.January, 1, 0, 0, 0, 0, s.loc); !t.After(end) && len(out) < guard; t = t.AddDate(u.years, 0, 0) {
			if !t.Before(start) {
				out = append(out, t)
			}
		}
	case u.months > 0:
		m := (int(start.Month()) - 1) / u.months * u.months
		for t := time.Date(start.Year(), time.Month(m+1), 1, 0, 0, 0, 0, s.loc); !t.After(end) && len(out) < guard; t = t.AddDate(0, u.months, 0) {
			if !t.Before(start) {
				out = append(out, t)
			}
		}
	default:
		// Align to the unit boundary in the scale's location, then step. Using
		// Truncate directly would align to the UTC epoch and put daily ticks at
		// the wrong hour in any zone with a non-zero offset.
		_, offset := start.Zone()
		off := time.Duration(offset) * time.Second
		first := start.Add(off).Truncate(u.every).Add(-off)
		for t := first; !t.After(end) && len(out) < guard; t = t.Add(u.every) {
			if !t.Before(start) {
				out = append(out, t)
			}
		}
	}
	return out
}

func (s *timeScale) label(t time.Time, u timeUnit) string {
	if s.format != nil {
		return s.format(t, u.approx())
	}
	return t.Format(u.layout)
}
