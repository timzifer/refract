package spec_test

// The key channel: a column that changes nothing drawn, and therefore has to
// be asserted on rather than looked at.

import (
	"strings"
	"testing"

	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/spec"
)

func TestTheKeyChannelSurvivesTheRoundTrip(t *testing.T) {
	src := longTable()
	cases := []struct {
		name  string
		layer geom.Geom
	}{
		{"scatter", geom.Scatter(src, geom.X("t"), geom.Y("v"), geom.KeyBy("series"))},
		{"line", geom.Line(src, geom.X("t"), geom.Y("v"), geom.KeyBy("series"))},
		{"bar", geom.Bar(src, geom.X("t"), geom.Y("v"), geom.KeyBy("series"))},
		{"sankey", geom.Sankey(src, geom.From("series"), geom.To("series"),
			geom.Value("v"), geom.KeyBy("series"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			back := roundTrip(t, chartOf(tc.layer))
			if len(back.Layers) != 1 {
				t.Fatalf("layers = %d", len(back.Layers))
			}
			// A key names no channel a mark draws with, so comparing the two
			// drawings would pass whether or not it came back. Assert on the
			// description instead.
			d, ok := geom.Describe(back.Layers[0])
			if !ok {
				t.Fatal("the layer cannot describe itself")
			}
			if d.Key != "series" {
				t.Errorf("Key = %q after the round trip, want %q", d.Key, "series")
			}
		})
	}
}

// The document says "key" rather than something invented, because Vega-Lite
// already spells this channel that way.
func TestTheKeyChannelIsSpelledKey(t *testing.T) {
	c := chartOf(geom.Scatter(longTable(), geom.X("t"), geom.Y("v"), geom.KeyBy("series")))
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"key"`) {
		t.Errorf("the encoding does not name a key channel:\n%s", b)
	}
}

// A layer that named no key writes none, so a document is not littered with
// empty channels and an old reader sees exactly what it saw before.
func TestNoKeyWritesNoChannel(t *testing.T) {
	c := chartOf(geom.Scatter(longTable(), geom.X("t"), geom.Y("v")))
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"key"`) {
		t.Errorf("a layer with no key wrote one:\n%s", b)
	}
}
