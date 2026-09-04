package scale_test

import (
	"testing"
	"time"

	"github.com/timzifer/refract/scale"
)

func trainTime(s scale.Scale, from time.Time, span time.Duration) {
	s.Train(scale.Nanos(from), scale.Nanos(from.Add(span)))
	s.SetRange(0, 800)
}

func TestTimeTicksPickCalendarUnits(t *testing.T) {
	start := time.Date(2026, time.March, 14, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		span      time.Duration
		wantLabel string // the first tick's expected format shape
	}{
		{2 * time.Minute, "09:00:00"},
		{time.Hour, "09:00"},
		{48 * time.Hour, "Mar 14 00:00"},
		{40 * 24 * time.Hour, "Mar 15"},
		{3 * 365 * 24 * time.Hour, "Jan 2027"},
	}
	for _, c := range cases {
		s := scale.Time()
		trainTime(s, start, c.span)
		ticks := s.Ticks(6)
		if len(ticks) < 2 {
			t.Errorf("span %v produced %d ticks", c.span, len(ticks))
			continue
		}
		if len(ticks[0].Label) != len(c.wantLabel) {
			t.Errorf("span %v: first label %q does not have the shape of %q — wrong unit chosen",
				c.span, ticks[0].Label, c.wantLabel)
		}
	}
}

func TestTimeTicksAreAscendingAndInsideTheDomain(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, span := range []time.Duration{
		time.Second, time.Minute, time.Hour, 24 * time.Hour,
		30 * 24 * time.Hour, 400 * 24 * time.Hour, 40 * 365 * 24 * time.Hour,
	} {
		s := scale.Time()
		trainTime(s, start, span)
		lo, hi := s.Domain()
		ticks := s.Ticks(6)
		if len(ticks) == 0 {
			t.Errorf("span %v produced no ticks", span)
			continue
		}
		for i, tk := range ticks {
			if tk.Value < lo || tk.Value > hi {
				t.Errorf("span %v: tick %d lies outside the domain", span, i)
			}
			if i > 0 && tk.Value <= ticks[i-1].Value {
				t.Errorf("span %v: ticks are not strictly ascending at %d", span, i)
			}
		}
	}
}

func TestTimeTicksHonourTheLocation(t *testing.T) {
	// Daily ticks must land on local midnight, not on UTC midnight shifted into
	// the local day. A fixed zone keeps the test independent of the tzdata that
	// happens to be installed.
	loc := time.FixedZone("UTC+5", 5*60*60)
	start := time.Date(2026, time.June, 1, 0, 0, 0, 0, loc)

	s := scale.Time(scale.In(loc))
	trainTime(s, start, 5*24*time.Hour)

	for _, tk := range s.Ticks(5) {
		got := scale.FromNanos(tk.Value).In(loc)
		if got.Hour() != 0 || got.Minute() != 0 {
			t.Fatalf("daily tick at %v is not local midnight", got)
		}
	}
}

func TestTimeDefaultsToUTC(t *testing.T) {
	// Rendering the same chart on two machines must produce the same labels, so
	// the default location cannot be the machine's.
	start := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	s := scale.Time()
	trainTime(s, start, 6*time.Hour)
	ticks := s.Ticks(4)
	if len(ticks) == 0 {
		t.Fatal("no ticks")
	}
	if got := scale.FromNanos(ticks[0].Value).UTC(); got.Minute()%15 != 0 {
		t.Fatalf("first tick %v is not aligned in UTC", got)
	}
}

func TestTimeMapRoundTrip(t *testing.T) {
	start := time.Date(2026, time.March, 14, 9, 0, 0, 0, time.UTC)
	s := scale.Time()
	trainTime(s, start, time.Hour)

	mid := scale.Nanos(start.Add(30 * time.Minute))
	pos := s.Map(mid)
	if pos < 399 || pos > 401 {
		t.Fatalf("the midpoint of the domain mapped to %v, want about 400", pos)
	}
	back := scale.FromNanos(s.Invert(pos))
	if d := back.Sub(start.Add(30 * time.Minute)); d > time.Second || d < -time.Second {
		t.Fatalf("Invert lost %v", d)
	}
}

func TestTimeDegenerateDomain(t *testing.T) {
	// A single timestamp must not produce a division by zero or an empty axis.
	s := scale.Time()
	now := time.Date(2026, time.March, 14, 9, 0, 0, 0, time.UTC)
	s.Train(scale.Nanos(now))
	s.SetRange(0, 100)
	if got := s.Ticks(5); len(got) == 0 {
		t.Fatal("a single-instant domain produced no ticks")
	}
}
