package scale_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/scale"
)

func TestSymLogIsSymmetricAboutZero(t *testing.T) {
	s := trained(scale.SymLog(), -1000, 1000)
	mid := s.Map(0)
	if math.Abs(float64(mid-50)) > 0.01 {
		t.Errorf("Map(0) = %v, want the middle of the range", mid)
	}
	for _, v := range []float64{0.5, 5, 50, 500} {
		lo, hi := s.Map(-v), s.Map(v)
		if math.Abs(float64((mid-lo)-(hi-mid))) > 0.01 {
			t.Errorf("±%v map to %v and %v, which are not equidistant from zero", v, lo, hi)
		}
	}
}

func TestSymLogIsMonotone(t *testing.T) {
	s := trained(scale.SymLog(scale.SymLogThreshold(0.1)), -500, 500)
	prev := s.Map(-500)
	for v := -499.0; v <= 500; v += 3.7 {
		got := s.Map(v)
		if got < prev {
			t.Fatalf("Map is not monotone: Map(%v) = %v after %v", v, got, prev)
		}
		prev = got
	}
}

func TestSymLogGivesEveryDecadeTheSameRoom(t *testing.T) {
	s := trained(scale.SymLog(), -1e6, 1e6)
	// Well away from the threshold the transform is a plain logarithm, so
	// successive decades must occupy the same width — that is the whole point
	// of the scale. Close to the threshold they do not, by design: the
	// transform is log(1 + |v|/t), which bends into the linear region rather
	// than kinking into it.
	a := s.Map(1e4) - s.Map(1e3)
	b := s.Map(1e5) - s.Map(1e4)
	if math.Abs(float64(a-b)) > 0.05 {
		t.Errorf("decade widths differ: 1e3..1e4 is %v, 1e4..1e5 is %v", a, b)
	}
}

func TestSymLogKeepsTheRegionAroundZeroVisible(t *testing.T) {
	// A plain log axis cannot show this data at all and a linear one gives the
	// whole ±1 region less than a thousandth of the axis.
	s := trained(scale.SymLog(scale.SymLogThreshold(1)), -1000, 1000)
	room := s.Map(1) - s.Map(-1)
	if room < 5 {
		t.Errorf("the ±1 region got %v of 100 units; symlog exists to keep it readable", room)
	}
}

func TestSymLogInvertRoundTrips(t *testing.T) {
	s := trained(scale.SymLog(), -1000, 1000)
	for _, v := range []float64{-999, -12, -0.3, 0, 0.3, 12, 999} {
		got := s.Invert(s.Map(v))
		if math.Abs(got-v) > 1e-2*math.Max(1, math.Abs(v)) {
			t.Errorf("Invert(Map(%v)) = %v", v, got)
		}
	}
}

func TestSymLogAlwaysLabelsZero(t *testing.T) {
	s := trained(scale.SymLog(), -300, 900)
	found := false
	for _, tk := range s.Ticks(6) {
		if tk.Value == 0 {
			found = true
			if tk.Label != "0" {
				t.Errorf("the zero tick is labelled %q", tk.Label)
			}
		}
	}
	if !found {
		t.Error("a symlog axis must mark zero — it is the one place the scale changes character")
	}
}

func TestSymLogTicksStayInsideTheDomainAndAscend(t *testing.T) {
	s := trained(scale.SymLog(), -50, 4000)
	ticks := s.Ticks(6)
	if len(ticks) == 0 {
		t.Fatal("no ticks")
	}
	for _, tk := range ticks {
		if tk.Value < -50 || tk.Value > 4000 {
			t.Errorf("tick at %v is outside the domain", tk.Value)
		}
	}
	if !ascending(ticks) {
		t.Error("Ticks must come out ordered by value")
	}
}

func TestSymLogHandlesEveryFiniteValue(t *testing.T) {
	s := trained(scale.SymLog(), -10, 10)
	if _, excludes := s.(scale.Definite); excludes {
		t.Error("a symlog scale places every finite value; it must not exclude any")
	}
	for _, v := range []float64{-10, -1, 0, 1, 10} {
		if math.IsNaN(float64(s.Map(v))) {
			t.Errorf("Map(%v) is NaN", v)
		}
	}
}
