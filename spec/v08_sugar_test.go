package spec_test

import (
	"strings"
	"testing"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/spec"
)

// The v0.8 sugar in the document: a slice's two radii are the x and x2
// channels the spec already had, and its break-out is refract's own — a mark
// property for the constant and a channel for the column.

func TestADonutsRadiiAndBreakOutSurviveTheRoundTrip(t *testing.T) {
	src := longTable()
	for _, tc := range []struct {
		name  string
		layer geom.Geom
	}{
		{"radii", geom.Bar(src, geom.X("t"), geom.X2("w"), geom.Y("v"), geom.GroupBy("series"))},
		{"broken out", geom.Bar(src, geom.X("t"), geom.Y("v"), geom.GroupBy("series"), geom.Explode(0.08))},
		{"broken out per row", geom.Bar(src, geom.X("t"), geom.Y("v"), geom.GroupBy("series"), geom.ExplodeBy("w"))},
		{"both", geom.Bar(src, geom.X("t"), geom.X2("w"), geom.Y("v"),
			geom.GroupBy("series"), geom.Explode(0.05), geom.ExplodeBy("w"))},
		{"a broken-out cell", geom.Rect(src, geom.X("t"), geom.X2("w"), geom.Y("v"), geom.Explode(0.05))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := polarChart(coord.Donut(0.45), tc.layer)
			want, got := draw(t, c), draw(t, roundTrip(t, c))
			if strings.Join(want, "\n") != strings.Join(got, "\n") {
				s, _ := spec.Of(c)
				b, _ := s.Marshal()
				t.Errorf("a donut with %s did not survive the round trip\n%s", tc.name, b)
			}
		})
	}
}

// The break-out is written plainly, for the same reason the coord is: nothing
// in Vega-Lite means it, so nothing in Vega-Lite is borrowed to say it.
func TestTheDocumentNamesTheBreakOutPlainly(t *testing.T) {
	c := polarChart(coord.Donut(0.45),
		geom.Bar(longTable(), geom.X("t"), geom.X2("w"), geom.Y("v"),
			geom.GroupBy("series"), geom.Explode(0.08), geom.ExplodeBy("w")))
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"explode": 0.08`, `"explode"`, `"x2"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the document is missing %s:\n%s", want, b)
		}
	}
}

// And a document somebody wrote by hand reads back as the chart it describes.
func TestAHandWrittenDonutReads(t *testing.T) {
	const doc = `{
	  "width": 400, "height": 400,
	  "data": {"values": [{"floor": 0.3, "reach": 1, "share": 60, "pull": 0.1},
	                      {"floor": 0.3, "reach": 0.7, "share": 40, "pull": 0}],
	           "format": {"parse": {"floor": "number", "reach": "number",
	                                "share": "number", "pull": "number"}}},
	  "encoding": {
	    "x": {"type": "quantitative", "scale": {"type": "linear"}},
	    "y": {"type": "quantitative", "scale": {"type": "linear"}}
	  },
	  "coord": {"type": "polar", "theta": "y", "hole": 0.4},
	  "layer": [{"mark": {"type": "bar"},
	             "encoding": {"x": {"field": "floor"}, "x2": {"field": "reach"},
	                          "y": {"field": "share"}, "explode": {"field": "pull"}}}]
	}`
	s, err := spec.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Chart()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := geom.Describe(c.Layers[0])
	if !ok {
		t.Fatal("the layer that was read back cannot describe itself")
	}
	if d.X != "floor" || d.X2 != "reach" || d.ExplodeCol != "pull" {
		t.Errorf("read %q/%q broken out by %q, want floor/reach by pull", d.X, d.X2, d.ExplodeCol)
	}
}
