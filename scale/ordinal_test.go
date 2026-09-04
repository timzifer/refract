package scale_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/scale"
)

func TestOrdinalEncodesInFirstAppearanceOrder(t *testing.T) {
	s := scale.Ordinal()
	cat, ok := s.(scale.Categorical)
	if !ok {
		t.Fatal("an ordinal scale must implement scale.Categorical")
	}
	if got := cat.Encode("beta"); got != 0 {
		t.Errorf("Encode(beta) = %v, want 0", got)
	}
	if got := cat.Encode("alpha"); got != 1 {
		t.Errorf("Encode(alpha) = %v, want 1", got)
	}
	if got := cat.Encode("beta"); got != 0 {
		t.Errorf("Encode(beta) the second time = %v, want 0 — a category keeps its slot", got)
	}
	want := []string{"beta", "alpha"}
	got := cat.Labels()
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Labels() = %v, want %v", got, want)
	}
}

func TestOrdinalCategoriesFixTheOrderAndRejectStrangers(t *testing.T) {
	s := scale.Ordinal(scale.Categories("small", "medium", "large"))
	cat := s.(scale.Categorical)
	if got := cat.Encode("large"); got != 2 {
		t.Errorf("Encode(large) = %v, want 2 — a fixed set keeps the order it was given", got)
	}
	if got := cat.Encode("enormous"); !math.IsNaN(got) {
		t.Errorf("Encode(enormous) = %v, want NaN — a label outside a fixed set has no slot", got)
	}
	if n := len(cat.Labels()); n != 3 {
		t.Errorf("a fixed set grew to %d categories", n)
	}
}

func TestOrdinalCentresCategoriesInEqualSlots(t *testing.T) {
	s := scale.Ordinal()
	cat := s.(scale.Categorical)
	for _, l := range []string{"a", "b", "c", "d"} {
		cat.Encode(l)
	}
	s.SetRange(0, 100)
	for i, want := range []float32{12.5, 37.5, 62.5, 87.5} {
		if got := s.Map(float64(i)); math.Abs(float64(got-want)) > 0.01 {
			t.Errorf("Map(%d) = %v, want %v", i, got, want)
		}
	}
	band, ok := s.(scale.Band)
	if !ok {
		t.Fatal("an ordinal scale must implement scale.Band so bars can size themselves")
	}
	// Four slots of 25, less the default padding of 0.2.
	if got := band.Bandwidth(); math.Abs(float64(got-20)) > 0.01 {
		t.Errorf("Bandwidth() = %v, want 20", got)
	}
}

func TestOrdinalPaddingNarrowsTheBandNotTheSlot(t *testing.T) {
	s := scale.Ordinal(scale.OrdinalPadding(0.5))
	cat := s.(scale.Categorical)
	cat.Encode("a")
	cat.Encode("b")
	s.SetRange(0, 100)
	if got := s.Map(0); math.Abs(float64(got-25)) > 0.01 {
		t.Errorf("Map(0) = %v, want 25 — padding must not move the centres", got)
	}
	if got := s.(scale.Band).Bandwidth(); math.Abs(float64(got-25)) > 0.01 {
		t.Errorf("Bandwidth() = %v, want 25", got)
	}
}

func TestOrdinalTicksLabelEveryCategory(t *testing.T) {
	s := scale.Ordinal(scale.Categories("mon", "tue", "wed"))
	s.SetRange(0, 90)
	ticks := s.Ticks(2)
	if len(ticks) != 3 {
		t.Fatalf("got %d ticks for 3 categories asked for 2, want 3 — dropping one leaves an unlabelled slot", len(ticks))
	}
	for i, want := range []string{"mon", "tue", "wed"} {
		if ticks[i].Label != want {
			t.Errorf("tick %d is %q, want %q", i, ticks[i].Label, want)
		}
	}
	if got := ticks[0].Pos; math.Abs(float64(got-15)) > 0.01 {
		t.Errorf("the first tick is at %v, want 15 (the centre of its slot)", got)
	}
}

func TestOrdinalTrainNamesSlotsNothingElseNamed(t *testing.T) {
	s := scale.Ordinal()
	s.Train(0, 1, 2)
	cat := s.(scale.Categorical)
	if got := cat.Labels(); len(got) != 3 || got[2] != "2" {
		t.Errorf("Labels() = %v, want three slots labelled by index", got)
	}
}

func TestOrdinalRejectsPositionsOutsideItsSlots(t *testing.T) {
	s := scale.Ordinal(scale.Categories("a", "b"))
	d, ok := s.(scale.Definite)
	if !ok {
		t.Fatal("an ordinal scale must implement scale.Definite")
	}
	if !d.Defined(0) || !d.Defined(1) {
		t.Error("a registered category must be defined")
	}
	if d.Defined(math.NaN()) || d.Defined(7) {
		t.Error("a value with no slot must not be defined")
	}
}

func TestOrdinalSnapsItsEndpoints(t *testing.T) {
	s := scale.Ordinal(scale.Categories("a", "b", "c"))
	s.SetRange(10, 110)
	lo, hi := s.Domain()
	if got := s.Map(lo); got != 10 {
		t.Errorf("Map(domain min) = %v, want exactly 10", got)
	}
	if got := s.Map(hi); got != 110 {
		t.Errorf("Map(domain max) = %v, want exactly 110", got)
	}
}
