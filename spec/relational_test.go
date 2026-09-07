package spec_test

import (
	"strings"
	"testing"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/spec"
	"github.com/timzifer/refract/theme"
)

// The relational layouts through the document. What these check is the same
// property every other mark is held to — a spec that reads back as a chart
// drawing the same calls is a spec that lost nothing — over the five channels
// and two properties they added.

// graph is the table the relational marks read: an edge list and a hierarchy in
// one, since a spec test only needs the columns to exist.
func graph() *data.Table {
	return data.NewTable().
		String("from", []string{"web", "web", "api"}).
		String("to", []string{"api", "cache", "db"}).
		String("node", []string{"/", "src", "docs"}).
		String("under", []string{"", "/", "/"}).
		Float64("n", []float64{6, 2, 5})
}

func TestEveryRelationalMarkSurvivesTheRoundTrip(t *testing.T) {
	src := graph()
	cases := []struct {
		name  string
		coord coord.Coord
		layer geom.Geom
	}{
		{"treemap", nil, geom.Treemap(src, geom.ID("node"), geom.Parent("under"), geom.Value("n"), geom.Padding(0.01))},
		{"icicle", nil, geom.Icicle(src, geom.ID("node"), geom.Parent("under"), geom.Value("n"))},
		// The recipe halves. A sunburst and a chord diagram are these two marks
		// under a polar coord, and the coord is written down beside them — so
		// the round trip has to carry both or the document reads back as the
		// other picture.
		{"sunburst", coord.Polar(), geom.Icicle(src, geom.ID("node"), geom.Parent("under"), geom.Value("n"), geom.Padding(0.005))},
		{"sankey", nil, geom.Sankey(src, geom.From("from"), geom.To("to"), geom.Value("n"), geom.Thickness(0.05), geom.Padding(0.02))},
		{"arc", nil, geom.Arc(src, geom.From("from"), geom.To("to"), geom.Value("n"))},
		{"chord", coord.Polar(), geom.Arc(src, geom.From("from"), geom.To("to"), geom.Value("n"), geom.Baseline(1), geom.Thickness(0.08))},
		// An unweighted graph names no value column at all, which is the case
		// that would break if the decoder demanded one.
		{"unweighted", nil, geom.Arc(src, geom.From("from"), geom.To("to"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := spec.Chart{
				Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
				X: scale.Linear(), Y: scale.Linear(),
				Coord:  tc.coord,
				Layers: []geom.Geom{tc.layer},
			}
			want := draw(t, c)
			got := draw(t, roundTrip(t, c))
			if len(want) != len(got) {
				t.Fatalf("%d calls after the round trip, %d before", len(got), len(want))
			}
			for i := range want {
				if want[i] != got[i] {
					t.Fatalf("call %d differs:\n before: %s\n  after: %s", i, want[i], got[i])
				}
			}
		})
	}
}

// The channels are refract's own words, and a document is where a reader meets
// them. This is what pins the spelling.
func TestARelationalLayerWritesItsOwnChannels(t *testing.T) {
	c := spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X: scale.Linear(), Y: scale.Linear(),
		Layers: []geom.Geom{
			geom.Sankey(graph(), geom.From("from"), geom.To("to"), geom.Value("n"), geom.Padding(0.02)),
		},
	}
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	for _, want := range []string{`"type": "sankey"`, `"from": {`, `"to": {`, `"value": {`, `"padding": `} {
		if !strings.Contains(doc, want) {
			t.Errorf("the document does not contain %s:\n%s", want, doc)
		}
	}
	// A sankey has no positional channels of its own, and writing empty ones
	// would read as though they meant something. The chart's scales are still
	// at the top level, where they belong — this is about the layer.
	enc := s.Layer[0].Encoding
	if enc.X != nil || enc.Y != nil {
		t.Errorf("the layer wrote positional channels a sankey does not read: x=%v y=%v", enc.X, enc.Y)
	}
}

// Vega-Lite's `arc` is a pie wedge. Borrowing the word would make a Vega-Lite
// document decode into a mark that draws something else entirely, which is the
// one thing docs/adr/0014-json-spec.md asks this vocabulary not to do.
func TestAnArcDiagramDoesNotStealVegaLitesArc(t *testing.T) {
	c := spec.Chart{
		Width: 200, Height: 200, DPR: 1, Theme: theme.Light,
		X: scale.Linear(), Y: scale.Linear(),
		Layers: []geom.Geom{geom.Arc(graph(), geom.From("from"), geom.To("to"))},
	}
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Layer[0].Mark.Type; got != "arc-diagram" {
		t.Errorf("mark type %q, want %q", got, "arc-diagram")
	}
	if _, err := (spec.Spec{Layer: []spec.Layer{{Mark: spec.Mark{Type: "arc"}}}}).Chart(); err == nil {
		t.Error("a Vega-Lite arc was accepted as an arc diagram")
	}
}

// A layer named only by an edge table still gets its data. Without the decoder
// asking about the relational channels, a sankey reads back with no source and
// geom.FromDesc refuses it — which is a failure a round-trip test over the
// built chart would never see, because the encoder never lost anything.
func TestAHandWrittenRelationalSpecReads(t *testing.T) {
	doc := `{
	  "width": 400, "height": 300,
	  "data": {"values": [
	    {"from": "a", "to": "b", "n": 3},
	    {"from": "b", "to": "c", "n": 2}
	  ]},
	  "encoding": {"x": {"type": "quantitative"}, "y": {"type": "quantitative"}},
	  "layer": [{
	    "mark": {"type": "sankey", "padding": 0.02},
	    "encoding": {
	      "from": {"field": "from"}, "to": {"field": "to"}, "value": {"field": "n"}
	    }
	  }]
	}`
	s, err := spec.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Chart()
	if err != nil {
		t.Fatalf("Chart: %v", err)
	}
	if len(c.Layers) != 1 {
		t.Fatalf("got %d layers", len(c.Layers))
	}
	d, ok := geom.Describe(c.Layers[0])
	if !ok {
		t.Fatal("the rebuilt layer does not describe itself")
	}
	if d.Mark != geom.MarkSankey || d.From != "from" || d.To != "to" || d.ValueCol != "n" {
		t.Errorf("read back as %+v", d)
	}
	if d.Source == nil {
		t.Error("the layer read back with no data source")
	}
}
