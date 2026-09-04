package scale_test

import (
	"testing"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

func TestAQualitativeScaleRegistersInOrderOfFirstSight(t *testing.T) {
	s := scale.Qualitative(palette.OkabeIto)
	for _, l := range []string{"north", "south", "north", "east"} {
		s.ColorOf(l)
	}
	want := []string{"north", "south", "east"}
	got := s.Labels()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want the order they first appeared: %v", got, want)
		}
	}
	// The index is the label's place in that order, which is what makes the
	// scale usable through the numeric ColorScale interface as well.
	if s.Encode("south") != 1 {
		t.Errorf("south encodes to %v, want 1", s.Encode("south"))
	}
	if s.Color(1) != s.ColorOf("south") {
		t.Error("Color and ColorOf disagree about the same category")
	}
}

func TestAQualitativeScaleWrapsPastTheEndOfItsPalette(t *testing.T) {
	p := palette.Qualitative{palette.Blue, palette.Orange}
	s := scale.Qualitative(p)
	if s.ColorOf("a") != palette.Blue || s.ColorOf("b") != palette.Orange {
		t.Fatal("the first two categories do not take the first two colours")
	}
	if s.ColorOf("c") != palette.Blue {
		t.Error("a third category did not wrap: a palette that runs out should repeat, not fail")
	}
}

func TestAnUnknownValueTakesTheUndefinedColour(t *testing.T) {
	s := scale.Qualitative(nil, scale.ColorUndefined(palette.Gray))
	if got := s.Color(-1); got != palette.Gray {
		t.Errorf("Color(-1) = %v, want the undefined colour", got)
	}
}

func TestAQualitativeScaleNamesItsPalette(t *testing.T) {
	s := scale.Qualitative(palette.OkabeIto)
	d, ok := scale.DescribeColor(s)
	if !ok {
		t.Fatal("a qualitative scale cannot describe itself")
	}
	if d.Kind != scale.KindQualitative {
		t.Errorf("kind = %q, want %q", d.Kind, scale.KindQualitative)
	}
	if d.Ramp != "okabeito" {
		t.Errorf("palette = %q, want the registered name", d.Ramp)
	}
	if len(d.Colors) != 0 {
		t.Error("a registered palette was spelled out as well as named")
	}

	back, err := scale.ColorFromDesc(d)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := back.(scale.DiscreteColorScale).ColorOf("a"), s.ColorOf("a"); got != want {
		t.Errorf("after the round trip a category is %v, want %v", got, want)
	}
}

func TestAnUnregisteredPaletteIsSpelledOut(t *testing.T) {
	p := palette.Qualitative{ir.RGB(1, 2, 3), ir.RGB(4, 5, 6)}
	d, _ := scale.DescribeColor(scale.Qualitative(p))
	if d.Ramp != "" {
		t.Errorf("an unregistered palette was named %q", d.Ramp)
	}
	if len(d.Colors) != 2 || d.Colors[0] != p[0] {
		t.Errorf("colours = %v, want the palette written out", d.Colors)
	}
}

func TestReversingHandsOutThePaletteTheOtherWayRound(t *testing.T) {
	p := palette.Qualitative{palette.Blue, palette.Orange, palette.Green}
	s := scale.Qualitative(p, scale.ColorReverse())
	if got := s.ColorOf("first"); got != palette.Green {
		t.Errorf("the first category is %v, want the last colour of the palette", got)
	}
}

// A discrete scale is a ColorScale, which is what lets one option bind either
// kind — and what makes the guide follow from the scale rather than from a
// second option nobody would remember to set.
func TestADiscreteScaleIsAColourScale(t *testing.T) {
	var cs scale.ColorScale = scale.Qualitative(nil)
	if _, ok := scale.Discrete(cs); !ok {
		t.Error("a qualitative scale does not report itself as discrete")
	}
	if _, ok := scale.Discrete(scale.Sequential(palette.Viridis)); ok {
		t.Error("a ramp reports itself as discrete")
	}
}
