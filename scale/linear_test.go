package scale_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/scale"
)

func TestLinearMapAndInvertRoundTrip(t *testing.T) {
	s := scale.Linear(scale.Domain(0, 100))
	s.SetRange(50, 250)

	for _, v := range []float64{0, 1, 37.5, 99, 100} {
		pos := s.Map(v)
		back := s.Invert(pos)
		if math.Abs(back-v) > 1e-3 {
			t.Errorf("Map/Invert round trip lost %v: got %v via pos %v", v, back, pos)
		}
	}
	if got := s.Map(0); got != 50 {
		t.Errorf("domain minimum mapped to %v, want the range start 50", got)
	}
	if got := s.Map(100); got != 250 {
		t.Errorf("domain maximum mapped to %v, want the range end 250", got)
	}
}

func TestLinearInvertedRange(t *testing.T) {
	// A Y axis is set up with the range reversed so that larger values sit
	// higher on screen. Nothing in the scale should special-case that.
	s := scale.Linear(scale.Domain(0, 10))
	s.SetRange(400, 0)
	if got := s.Map(0); got != 400 {
		t.Errorf("Map(0) = %v, want 400", got)
	}
	if got := s.Map(10); got != 0 {
		t.Errorf("Map(10) = %v, want 0", got)
	}
	if got := s.Map(5); math.Abs(float64(got)-200) > 1e-3 {
		t.Errorf("Map(5) = %v, want 200", got)
	}
}

func TestLinearNiceExpandsToWholeTicks(t *testing.T) {
	s := scale.Linear(scale.Nice())
	s.Train(0.3, 9.4)
	lo, hi := s.Domain()
	if lo > 0.3 || hi < 9.4 {
		t.Fatalf("Nice() must not crop the data: domain is %v..%v", lo, hi)
	}
	if lo != 0 || hi != 10 {
		t.Errorf("domain = %v..%v, want 0..10", lo, hi)
	}
}

func TestLinearZeroIncludesBaseline(t *testing.T) {
	s := scale.Linear(scale.Zero())
	s.Train(20, 35)
	lo, _ := s.Domain()
	if lo != 0 {
		t.Errorf("Zero() domain starts at %v, want 0", lo)
	}
}

func TestLinearConstantData(t *testing.T) {
	// One repeated value must still produce a usable axis rather than a
	// zero-width domain that maps everything to one pixel.
	s := scale.Linear()
	s.Train(7, 7, 7)
	lo, hi := s.Domain()
	if !(lo < 7 && hi > 7) {
		t.Fatalf("constant data gave domain %v..%v, want an interval around 7", lo, hi)
	}
	s.SetRange(0, 100)
	if got := s.Map(7); math.IsNaN(float64(got)) {
		t.Fatal("Map returned NaN for constant data")
	}
}

func TestLinearIgnoresNonFiniteTraining(t *testing.T) {
	s := scale.Linear()
	s.Train(1, math.NaN(), math.Inf(1), 5, math.Inf(-1))
	lo, hi := s.Domain()
	if lo != 1 || hi != 5 {
		t.Fatalf("domain = %v..%v, want 1..5 — NaN and Inf must not train a scale", lo, hi)
	}
}

func TestLinearTicksAreEvenlySpacedAndDistinctlyLabelled(t *testing.T) {
	cases := []struct{ lo, hi float64 }{
		{0, 1}, {0, 10}, {-1, 1}, {0.3, 9.4}, {1000, 100000}, {-0.0004, 0.0004},
	}
	for _, c := range cases {
		s := scale.Linear(scale.Domain(c.lo, c.hi))
		s.SetRange(0, 500)
		ticks := s.Ticks(5)
		if len(ticks) < 2 {
			t.Errorf("domain %v..%v produced %d ticks", c.lo, c.hi, len(ticks))
			continue
		}
		step := ticks[1].Value - ticks[0].Value
		for i := 2; i < len(ticks); i++ {
			if d := ticks[i].Value - ticks[i-1].Value; math.Abs(d-step) > math.Abs(step)*1e-6 {
				t.Errorf("domain %v..%v: ticks are not evenly spaced (%v vs %v)", c.lo, c.hi, d, step)
				break
			}
		}
		seen := map[string]bool{}
		for _, tk := range ticks {
			if seen[tk.Label] {
				t.Errorf("domain %v..%v: duplicate tick label %q — the label format has too few digits for the step",
					c.lo, c.hi, tk.Label)
				break
			}
			seen[tk.Label] = true
		}
	}
}

// TestLinearFractionalStepKeepsItsDecimal guards the specific bug that made an
// evenly spaced axis look uneven: a step of 2.5 rounded to whole-number labels
// reads 0, 2, 5, 8, 10.
func TestLinearFractionalStepKeepsItsDecimal(t *testing.T) {
	s := scale.Linear(scale.Domain(0, 10))
	s.SetRange(0, 400)
	for _, tk := range s.Ticks(5) {
		if tk.Value == 2.5 && tk.Label != "2.5" {
			t.Fatalf("tick 2.5 is labelled %q", tk.Label)
		}
	}
}

func TestLinearCustomFormat(t *testing.T) {
	s := scale.Linear(scale.Domain(0, 3), scale.Format(func(v float64) string { return "x" }))
	s.SetRange(0, 100)
	for _, tk := range s.Ticks(4) {
		if tk.Label != "x" {
			t.Fatalf("custom format not applied: %q", tk.Label)
		}
	}
}

func TestLinearTicksStayInsideTheDomain(t *testing.T) {
	s := scale.Linear(scale.Domain(1.3, 8.7))
	s.SetRange(0, 100)
	for _, tk := range s.Ticks(5) {
		if tk.Value < 1.3-1e-9 || tk.Value > 8.7+1e-9 {
			t.Errorf("tick %v lies outside the pinned domain 1.3..8.7", tk.Value)
		}
	}
}
