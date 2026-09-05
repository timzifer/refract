package spec_test

import (
	"math"
	"strings"
	"testing"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/spec"
	"github.com/timzifer/refract/theme"
)

// The v0.8 addition to the document: a top-level `coord`, and the one mark
// option that came with it.

func polarChart(c coord.Coord, layers ...geom.Geom) spec.Chart {
	return spec.Chart{
		Width: 400, Height: 400, DPR: 1, Theme: theme.Light,
		X: scale.Linear(), Y: scale.Linear(),
		Coord:  c,
		Layers: layers,
	}
}

func TestEveryCoordSurvivesTheRoundTrip(t *testing.T) {
	src := longTable()
	cases := []struct {
		name  string
		coord coord.Coord
	}{
		{"cartesian", coord.Cartesian()},
		{"polar", coord.Polar()},
		{"pie", coord.Polar(coord.Theta(coord.FromY))},
		{"donut", coord.Polar(coord.Theta(coord.FromY), coord.Hole(0.45))},
		{"radar", coord.Polar(coord.Chord())},
		{"gauge", coord.Polar(coord.Theta(coord.FromY), coord.Sweep(math.Pi),
			coord.Start(-math.Pi/2), coord.Hole(0.6))},
		{"widdershins", coord.Polar(coord.Counterclockwise(true), coord.Radius(0.7))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := polarChart(tc.coord,
				geom.Bar(src, geom.X("t"), geom.Y("v"), geom.GroupBy("series")))
			want, got := draw(t, c), draw(t, roundTrip(t, c))
			if strings.Join(want, "\n") != strings.Join(got, "\n") {
				s, _ := spec.Of(c)
				b, _ := s.Marshal()
				t.Errorf("the %s coord did not survive the round trip\n%s", tc.name, b)
			}
		})
	}
}

// A closed contour is a property of the connected marks, so it is written by
// those and not by the six that would ignore it.
func TestAClosedContourSurvivesTheRoundTrip(t *testing.T) {
	src := longTable()
	for _, tc := range []struct {
		name  string
		layer geom.Geom
	}{
		{"line", geom.Line(src, geom.X("t"), geom.Y("v"), geom.Closed(true))},
		{"area", geom.Area(src, geom.X("t"), geom.Y("v"), geom.Closed(true))},
		{"step", geom.Step(src, geom.X("t"), geom.Y("v"), geom.Closed(true))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := polarChart(coord.Polar(coord.Chord()), tc.layer)
			want, got := draw(t, c), draw(t, roundTrip(t, c))
			if strings.Join(want, "\n") != strings.Join(got, "\n") {
				s, _ := spec.Of(c)
				b, _ := s.Marshal()
				t.Errorf("a closed %s did not survive the round trip\n%s", tc.name, b)
			}
		})
	}
}

// The coord is named plainly rather than smuggled through a borrowed name: a
// pie is a bar in a polar coord, not an `arc` mark that would read back into
// something refract cannot rebuild. See docs/adr/0014-json-spec.md.
func TestTheDocumentNamesTheCoordPlainly(t *testing.T) {
	c := polarChart(coord.Polar(coord.Theta(coord.FromY), coord.Hole(0.4)),
		geom.Bar(longTable(), geom.X("t"), geom.Y("v"), geom.GroupBy("series")))
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"coord"`, `"type": "polar"`, `"theta": "y"`, `"hole": 0.4`, `"type": "bar"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the document is missing %s:\n%s", want, b)
		}
	}
	if strings.Contains(string(b), `"arc"`) {
		t.Errorf("a pie was written as an arc mark:\n%s", b)
	}
}

// A Cartesian chart writes no coord at all, so a document written before there
// was a coord and one written after are the same document.
func TestACartesianCoordIsNotWrittenDown(t *testing.T) {
	for _, c := range []spec.Chart{
		polarChart(nil, geom.Line(longTable(), geom.X("t"), geom.Y("v"))),
		polarChart(coord.Cartesian(), geom.Line(longTable(), geom.X("t"), geom.Y("v"))),
	} {
		s, err := spec.Of(c)
		if err != nil {
			t.Fatal(err)
		}
		if s.Coord != nil {
			t.Errorf("a Cartesian chart wrote a coord: %+v", s.Coord)
		}
	}
}

func TestAHandWrittenCoordReads(t *testing.T) {
	const doc = `{
	  "width": 400, "height": 400,
	  "data": {"values": [{"one": 0, "share": 60}, {"one": 0, "share": 40}],
	           "format": {"parse": {"one": "number", "share": "number"}}},
	  "encoding": {
	    "x": {"type": "quantitative", "scale": {"type": "linear"}},
	    "y": {"type": "quantitative", "scale": {"type": "linear"}}
	  },
	  "coord": {"type": "polar", "theta": "y", "hole": 0.5},
	  "layer": [{"mark": {"type": "bar"},
	             "encoding": {"x": {"field": "one"}, "y": {"field": "share"}}}]
	}`
	s, err := spec.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Chart()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := coord.Describe(c.Coord)
	if !ok {
		t.Fatal("the coord that was read back cannot describe itself")
	}
	if d.Type != coord.TypePolar || d.Theta != coord.FromY || d.Hole != 0.5 {
		t.Errorf("read %+v, want a polar coord with theta from Y and a half hole", d)
	}
	// A sweep the document did not name is a whole turn, not zero.
	if d.Sweep != coord.FullTurn {
		t.Errorf("sweep = %v, want a full turn", d.Sweep)
	}
}

func TestAnUnknownCoordFieldIsAnError(t *testing.T) {
	for _, doc := range []string{
		`{"coord": {"type": "hyperbolic"}}`,
		`{"coord": {"type": "polar", "theta": "z"}}`,
		`{"coord": {"type": "polar", "edge": "spline"}}`,
	} {
		s, err := spec.Parse([]byte(doc))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if _, err := s.Chart(); err == nil {
			t.Errorf("%s was accepted", doc)
		}
	}
}
