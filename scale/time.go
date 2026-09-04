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

type timeScale struct {
	domainRange
	loc    *time.Location
	fixed  bool
	format func(time.Time, time.Duration) string
}

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
	start, end := FromNanos(lo).In(s.loc), FromNanos(hi).In(s.loc)
	span := end.Sub(start)
	if span <= 0 {
		return []Tick{{Value: lo, Pos: s.Map(lo), Label: s.label(start, timeUnits[3])}}
	}

	u := s.pick(span, want)
	times := s.walk(start, end, u)

	out := make([]Tick, 0, len(times))
	for _, t := range times {
		v := Nanos(t)
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
