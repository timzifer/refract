package spec_test

import (
	"strings"
	"testing"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/spec"
	"github.com/timzifer/refract/theme"
)

// The Smith coord and the pinned tick sequence it is spelled with, written
// down and read back.

func smithChartSpec(c coord.Coord, x, y scale.Scale, layers ...geom.Geom) spec.Chart {
	return spec.Chart{
		Width: 400, Height: 400, DPR: 1, Theme: theme.Light,
		X: x, Y: y, Coord: c, Layers: layers,
	}
}

func smithScales() (scale.Scale, scale.Scale) {
	return scale.Linear(scale.Domain(0, 20), scale.TickValues(0, 0.2, 0.5, 1, 2, 5)),
		scale.Linear(scale.Domain(-20, 20), scale.TickValues(-5, -2, -1, -0.5, -0.2, 0.2, 0.5, 1, 2, 5))
}

func TestEverySmithChartSurvivesTheRoundTrip(t *testing.T) {
	src := longTable()
	for _, tc := range []struct {
		name  string
		coord coord.Coord
	}{
		{"impedance", coord.Smith()},
		{"a swept locus", coord.Smith(coord.SmithArc())},
		{"admittance", coord.Smith(coord.SmithAdmittance(true))},
		{"a tight disc", coord.Smith(coord.SmithRadius(0.7), coord.SmithChord())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, y := smithScales()
			c := smithChartSpec(tc.coord, x, y, geom.Line(src, geom.X("t"), geom.Y("v")))
			want, got := draw(t, c), draw(t, roundTrip(t, c))
			if strings.Join(want, "\n") != strings.Join(got, "\n") {
				s, _ := spec.Of(c)
				b, _ := s.Marshal()
				t.Errorf("the %s chart did not survive the round trip\n%s", tc.name, b)
			}
		})
	}
}

// The absent edge is each coord's own default, and the two coords default
// opposite ways. A document that names a Smith chart and nothing else has to
// come back as the coord the constructor builds — a chord — rather than as the
// arc a polar coord would read the same silence as.
func TestASmithChartsDefaultEdgeIsAChordInJSONToo(t *testing.T) {
	s, err := spec.Parse([]byte(`{
		"width": 400, "height": 400,
		"encoding": {"x": {"field": "t"}, "y": {"field": "v"}},
		"mark": "line",
		"coord": {"type": "smith"}
	}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	doc, err := s.Chart()
	if err != nil {
		t.Fatalf("Chart: %v", err)
	}
	d, ok := coord.Describe(doc.Coord)
	if !ok {
		t.Fatalf("the decoded coord %T cannot describe itself", doc.Coord)
	}
	if d.Type != coord.TypeSmith {
		t.Fatalf("decoded as %q", d.Type)
	}
	if d.Arc {
		t.Error("a document that named no edge came back drawing arcs, not the chords the constructor draws")
	}
	if want, _ := coord.Describe(coord.Smith()); d != want {
		t.Errorf("decoded %+v, want the coord Smith() builds, %+v", d, want)
	}
}

// Theta and sweep are a polar coord's, and a Smith document must not carry
// them: `"theta":"x"` is not omitted by omitempty, because "x" is not empty.
func TestASmithDocumentCarriesNoPolarFields(t *testing.T) {
	x, y := smithScales()
	c := smithChartSpec(coord.Smith(coord.SmithAdmittance(true)), x, y,
		geom.Line(longTable(), geom.X("t"), geom.Y("v")))
	s, err := spec.Of(c)
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, field := range []string{`"theta"`, `"sweep"`, `"hole"`, `"counterclockwise"`} {
		if strings.Contains(string(b), field) {
			t.Errorf("a Smith document carries the polar field %s:\n%s", field, b)
		}
	}
	for _, field := range []string{`"type": "smith"`, `"admittance": true`, `"tickValues"`} {
		if !strings.Contains(string(b), field) {
			t.Errorf("a Smith document is missing %s:\n%s", field, b)
		}
	}
}

// A pinned tick sequence is part of what the axis is, so it has to survive the
// document — otherwise a Smith chart read back from JSON draws an unfamiliar
// grid on a familiar disc.
func TestPinnedTickValuesSurviveTheRoundTrip(t *testing.T) {
	x, y := smithScales()
	c := smithChartSpec(coord.Smith(), x, y, geom.Line(longTable(), geom.X("t"), geom.Y("v")))
	back := roundTrip(t, c)
	got := back.X.Ticks(6)
	want := []float64{0, 0.2, 0.5, 1, 2, 5}
	if len(got) != len(want) {
		t.Fatalf("the read-back axis has %d ticks, want %d", len(got), len(want))
	}
	for i, tk := range got {
		if tk.Value != want[i] {
			t.Errorf("tick %d came back as %v, want %v", i, tk.Value, want[i])
		}
	}
}
