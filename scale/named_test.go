package scale_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

func TestANamedScaleColoursACategoryByName(t *testing.T) {
	s := scale.Named(map[string]ir.Color{
		"RUN":   palette.Green,
		"FAULT": palette.Red,
	})
	// Whichever category the data shows first, the colours are the ones the
	// caller named. That is the whole point of the scale: a fault is red on
	// a window that begins with a fault and on one that never has one.
	if got := s.ColorOf("RUN"); got != palette.Green {
		t.Errorf("RUN is %v, want %v", got, palette.Green)
	}
	if got := s.ColorOf("FAULT"); got != palette.Red {
		t.Errorf("FAULT is %v, want %v", got, palette.Red)
	}
	if s.Color(s.Encode("FAULT")) != palette.Red {
		t.Error("Color and ColorOf disagree about the same category")
	}
}

func TestANamedScaleListsItsCategoriesInSortedOrder(t *testing.T) {
	s := scale.Named(map[string]ir.Color{
		"WARTUNG": palette.Orange,
		"RUN":     palette.Green,
		"FAULT":   palette.Red,
	})
	// Sorted rather than as-written: a Go map has no order to preserve, and
	// ADR 0012 does not allow one that depends on map iteration.
	want := []string{"FAULT", "RUN", "WARTUNG"}
	if got := s.Labels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Labels() = %v, want %v", got, want)
	}
	s.ColorOf("RUN")
	if got := s.Labels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Labels() = %v after use, want the same %v", got, want)
	}
}

func TestANamedScaleStillDrawsACategoryNobodyNamed(t *testing.T) {
	fallback := palette.Qualitative{palette.Blue, palette.Purple}
	s := scale.Named(map[string]ir.Color{"RUN": palette.Green}, scale.ColorFallback(fallback))
	// A status code the caller did not enumerate is a category, not a null:
	// leaving it undrawn would put a hole in the line and a hole reads as a
	// gap in the measurements.
	if got := s.ColorOf("E-STOP"); got != palette.Blue {
		t.Errorf("an unnamed category is %v, want the first fallback colour %v", got, palette.Blue)
	}
	if got := s.ColorOf("ANLAUF"); got != palette.Purple {
		t.Errorf("the second unnamed category is %v, want %v", got, palette.Purple)
	}
	if got := s.ColorOf("RUN"); got != palette.Green {
		t.Errorf("a named category is %v after two were discovered, want %v", got, palette.Green)
	}
	want := []string{"RUN", "E-STOP", "ANLAUF"}
	if got := s.Labels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Labels() = %v, want the named ones first: %v", got, want)
	}
}

func TestANamedScaleAnswersANullWithTheUndefinedColour(t *testing.T) {
	s := scale.Named(map[string]ir.Color{"RUN": palette.Green}, scale.ColorUndefined(palette.Gray))
	if got := s.Color(math.NaN()); got != palette.Gray {
		t.Errorf("a null is %v, want the undefined colour %v", got, palette.Gray)
	}
}

func TestANamedScaleSurvivesBeingWrittenDown(t *testing.T) {
	fallback := palette.Qualitative{palette.Blue, palette.Purple}
	s := scale.Named(map[string]ir.Color{
		"RUN":   palette.Green,
		"FAULT": palette.Red,
	}, scale.ColorFallback(fallback), scale.ColorUndefined(palette.Gray))
	// Only what the caller configured is written: a category discovered in
	// the data is data, and pinning it would stop the next render finding it.
	s.ColorOf("E-STOP")

	d, ok := scale.DescribeColor(s)
	if !ok {
		t.Fatal("a named scale cannot describe itself")
	}
	if d.Kind != scale.KindNamed {
		t.Fatalf("kind is %q, want %q", d.Kind, scale.KindNamed)
	}
	if want := []string{"FAULT", "RUN"}; !reflect.DeepEqual(d.Labels, want) {
		t.Fatalf("Labels = %v, want only the named ones %v", d.Labels, want)
	}

	back, err := scale.ColorFromDesc(d)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := scale.Discrete(back)
	for _, l := range []string{"RUN", "FAULT"} {
		if got.ColorOf(l) != s.ColorOf(l) {
			t.Errorf("%s reads back as %v, want %v", l, got.ColorOf(l), s.ColorOf(l))
		}
	}
	if got.ColorOf("E-STOP") != palette.Blue {
		t.Error("the fallback palette did not survive")
	}
	if got.Color(math.NaN()) != palette.Gray {
		t.Error("the undefined colour did not survive")
	}
}
