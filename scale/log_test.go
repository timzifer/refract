package scale_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/scale"
)

func trained(s scale.Scale, vs ...float64) scale.Scale {
	s.Train(vs...)
	s.SetRange(0, 100)
	return s
}

func TestLogMapsDecadesToEqualDistances(t *testing.T) {
	s := trained(scale.Log(scale.LogNice()), 1, 1000)
	// Three decades over 100 units: each decade is a third of the axis.
	for i, want := range []float32{0, 100.0 / 3, 200.0 / 3, 100} {
		got := s.Map(math.Pow(10, float64(i)))
		if math.Abs(float64(got-want)) > 0.01 {
			t.Errorf("Map(1e%d) = %v, want %v — decades must be evenly spaced", i, got, want)
		}
	}
}

func TestLogSnapsItsEndpoints(t *testing.T) {
	s := trained(scale.Log(), 3, 700)
	lo, hi := s.Domain()
	if got := s.Map(lo); got != 0 {
		t.Errorf("Map(domain min) = %v, want exactly 0", got)
	}
	if got := s.Map(hi); got != 100 {
		t.Errorf("Map(domain max) = %v, want exactly 100", got)
	}
}

func TestLogRejectsNonPositiveValues(t *testing.T) {
	s := scale.Log()
	s.Train(-5, 0, 10, 100)
	s.SetRange(0, 100)
	lo, hi := s.Domain()
	if lo != 10 || hi != 100 {
		t.Errorf("Domain() = (%v, %v), want (10, 100) — training must ignore what it cannot place", lo, hi)
	}
	d, ok := s.(scale.Definite)
	if !ok {
		t.Fatal("a log scale must implement scale.Definite so geoms can treat zero as missing")
	}
	for _, v := range []float64{0, -1, math.Inf(1)} {
		if d.Defined(v) {
			t.Errorf("Defined(%v) = true, want false", v)
		}
	}
	if !math.IsNaN(float64(s.Map(0))) {
		t.Error("Map(0) must be NaN, so a geom can spot it rather than draw at the axis minimum")
	}
}

func TestLogNiceExpandsToWholeDecades(t *testing.T) {
	s := scale.Log(scale.LogNice())
	s.Train(3, 700)
	lo, hi := s.Domain()
	if lo != 1 || hi != 1000 {
		t.Errorf("Domain() = (%v, %v), want (1, 1000)", lo, hi)
	}
}

func TestLogTicksLabelDecadesAndSubdivideThem(t *testing.T) {
	s := trained(scale.Log(scale.LogNice()), 1, 100)
	ticks := s.Ticks(5)

	var labelled []string
	minor := 0
	for _, tk := range ticks {
		if tk.Minor {
			minor++
			if tk.Label != "" {
				t.Errorf("minor tick at %v carries the label %q; minor ticks are unlabelled", tk.Value, tk.Label)
			}
			continue
		}
		labelled = append(labelled, tk.Label)
	}
	want := []string{"1", "10", "100"}
	if len(labelled) != len(want) {
		t.Fatalf("labelled ticks = %v, want %v", labelled, want)
	}
	for i := range want {
		if labelled[i] != want[i] {
			t.Errorf("labelled ticks = %v, want %v", labelled, want)
			break
		}
	}
	// Eight subdivisions in each of the two full decades; the top decade
	// contributes none because 200 is past the domain.
	if minor != 16 {
		t.Errorf("got %d minor ticks, want 16", minor)
	}
	if !ascending(ticks) {
		t.Error("Ticks must come out ordered by value")
	}
}

func TestLogTicksThinToWholeDecadesOverAWideRange(t *testing.T) {
	s := trained(scale.Log(scale.LogNice()), 1, 1e12)
	ticks := s.Ticks(4)
	labelled, marked := 0, 0
	for _, tk := range ticks {
		if tk.Minor {
			marked++
			continue
		}
		labelled++
	}
	if labelled > 5 {
		t.Errorf("got %d labelled ticks for a 13-decade axis asked for 4, want at most 5", labelled)
	}
	// Every decade the labels skipped still gets an unlabelled mark, and
	// nothing subdivides a decade once the axis is this compressed.
	if labelled+marked != 13 {
		t.Errorf("got %d ticks in total, want one per decade (13)", labelled+marked)
	}
}

func TestLogInvertRoundTrips(t *testing.T) {
	s := trained(scale.Log(), 1, 1000)
	for _, v := range []float64{1, 7, 42, 999} {
		got := s.Invert(s.Map(v))
		if math.Abs(got-v)/v > 1e-4 {
			t.Errorf("Invert(Map(%v)) = %v", v, got)
		}
	}
}

func TestLogUntrainedStillRenders(t *testing.T) {
	s := scale.Log()
	s.SetRange(0, 100)
	if lo, hi := s.Domain(); !(lo > 0 && hi > lo) {
		t.Errorf("Domain() = (%v, %v); an untrained log scale still has to produce an axis", lo, hi)
	}
	if len(s.Ticks(5)) == 0 {
		t.Error("an untrained log scale produced no ticks")
	}
}

func TestLogConstantDataGetsADecadeEitherSide(t *testing.T) {
	s := trained(scale.Log(), 50, 50)
	lo, hi := s.Domain()
	if lo != 5 || hi != 500 {
		t.Errorf("Domain() = (%v, %v), want (5, 500)", lo, hi)
	}
}

func ascending(ts []scale.Tick) bool {
	for i := 1; i < len(ts); i++ {
		if ts[i].Value < ts[i-1].Value {
			return false
		}
	}
	return true
}
