package spec_test

import (
	"strings"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/spec"
	"github.com/timzifer/refract/theme"
)

// The v0.7 additions to the document: the data-driven rect, the series channel
// and the position adjustments defined over it.

func longTable() *data.Table {
	return data.NewTable().
		Float64("t", []float64{0, 0, 1, 1, 2, 2}).
		Float64("v", []float64{1, 2, 3, 1, 2, 4}).
		Float64("w", []float64{1, 1, 2, 2, 3, 3}).
		Float64("v2", []float64{2, 3, 4, 2, 3, 5}).
		String("series", []string{"a", "b", "a", "b", "a", "b"})
}

func chartOf(layers ...geom.Geom) spec.Chart {
	return spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X: scale.Linear(scale.Nice()), Y: scale.Linear(scale.Nice()),
		Layers: layers,
	}
}

func TestTheV07MarksAndOptionsSurviveTheRoundTrip(t *testing.T) {
	src := longTable()
	cases := []struct {
		name  string
		layer geom.Geom
	}{
		{"rect", geom.Rect(src, geom.X("t"), geom.X2("w"), geom.Y("v"), geom.Y2("v2"))},
		{"heatmap", geom.Rect(src, geom.X("t"), geom.Y("v"),
			geom.ColorBy("v2", scale.Sequential(palette.Viridis)))},
		{"grouped-line", geom.Line(src, geom.X("t"), geom.Y("v"), geom.GroupBy("series"))},
		{"stacked-bar", geom.Bar(src, geom.X("t"), geom.Y("v"), geom.GroupBy("series"))},
		{"unstacked-bar", geom.Bar(src, geom.X("t"), geom.Y("v"),
			geom.GroupBy("series"), geom.Stack(geom.NoStack))},
		{"filled-bar", geom.Bar(src, geom.X("t"), geom.Y("v"),
			geom.GroupBy("series"), geom.Stack(geom.StackFill))},
		{"dodged-bar", geom.Bar(src, geom.X("t"), geom.Y("v"),
			geom.GroupBy("series"), geom.Dodge(0.25))},
		{"marimekko", geom.Bar(src, geom.X("t"), geom.Y("v"),
			geom.GroupBy("series"), geom.WidthBy("w"), geom.Stack(geom.StackFill))},
		{"streamgraph", geom.Area(src, geom.X("t"), geom.Y("v"),
			geom.GroupBy("series"), geom.Stack(geom.StackWiggle), geom.Order(geom.OrderInsideOut))},
		{"themeriver", geom.Area(src, geom.X("t"), geom.Y("v"),
			geom.GroupBy("series"), geom.Stack(geom.StackSilhouette))},
		{"ordered-area", geom.Area(src, geom.X("t"), geom.Y("v"),
			geom.GroupBy("series"), geom.Order(geom.OrderValue))},
		{"discrete-colour", geom.Scatter(src, geom.X("t"), geom.Y("v"),
			geom.ColorBy("series", scale.Qualitative(palette.OkabeIto)))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := chartOf(tc.layer)
			want, got := draw(t, c), draw(t, roundTrip(t, c))
			if strings.Join(want, "\n") != strings.Join(got, "\n") {
				s, _ := spec.Of(c)
				b, _ := s.Marshal()
				t.Errorf("the %s layer did not survive the round trip\n%s", tc.name, b)
			}
		})
	}
}

// The collision the catalogue named before this mark existed: `("rect", "")`
// is both a data-driven rect and the region annotation, and the mark object
// alone cannot tell them apart.
func TestARectAndARegionAreToldApartByTheirEncoding(t *testing.T) {
	src := longTable()
	c := chartOf(
		geom.Rect(src, geom.X("t"), geom.Y("v"), geom.X2("w"), geom.Y2("v2")),
		geom.Region(0.5, 1, 1.5, 3),
	)
	back := roundTrip(t, c)
	if len(back.Layers) != 2 {
		t.Fatalf("got %d layers back", len(back.Layers))
	}
	first, _ := geom.Describe(back.Layers[0])
	second, _ := geom.Describe(back.Layers[1])
	if first.Mark != geom.MarkRect {
		t.Errorf("the layer with columns read back as %q", first.Mark)
	}
	if second.Mark != geom.MarkRegion {
		t.Errorf("the layer with literals read back as %q", second.Mark)
	}
	want, got := draw(t, c), draw(t, back)
	if strings.Join(want, "\n") != strings.Join(got, "\n") {
		t.Error("a rect and a region in one chart do not survive together")
	}
}

func TestTheDocumentSpellsTheAdjustmentOut(t *testing.T) {
	src := longTable()
	s, err := spec.Of(chartOf(geom.Area(src, geom.X("t"), geom.Y("v"),
		geom.GroupBy("series"), geom.Stack(geom.StackWiggle), geom.Order(geom.OrderInsideOut))))
	if err != nil {
		t.Fatal(err)
	}
	enc := s.Layer[0].Encoding
	if enc.Detail == nil || enc.Detail.Field != "series" {
		t.Errorf("the series column is not in the detail channel: %+v", enc.Detail)
	}
	if enc.Y == nil || enc.Y.Stack != "wiggle" {
		t.Errorf("the stack is %+v, want \"wiggle\" on the Y channel", enc.Y)
	}
	if s.Layer[0].Mark.Order != "inside-out" {
		t.Errorf("the order is %q", s.Layer[0].Mark.Order)
	}
}

// A hand-written document that names a series column and no adjustment gets
// the mark's own default, which is what a Vega-Lite reader would expect of the
// same document.
func TestAGroupedBarWithNoStackNamedStacksAnyway(t *testing.T) {
	doc := `{
	  "data": {"values": [
	    {"t": 0, "v": 1, "s": "a"}, {"t": 0, "v": 2, "s": "b"},
	    {"t": 1, "v": 3, "s": "a"}, {"t": 1, "v": 1, "s": "b"}
	  ]},
	  "layer": [{"mark": {"type": "bar"}, "encoding": {
	    "x": {"field": "t"}, "y": {"field": "v"}, "detail": {"field": "s"}
	  }}]
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
		t.Fatal("the layer cannot describe itself")
	}
	if d.Group != "s" {
		t.Errorf("the series column is %q", d.Group)
	}
	if d.Stack != geom.StackZero {
		t.Errorf("the adjustment is %v, want a grouped bar's default", d.Stack)
	}
	if d.StackSet {
		t.Error("a document that named no stack read back as one that pinned it")
	}
}

func TestAQualitativeColourScaleIsNamedInTheDocument(t *testing.T) {
	src := longTable()
	s, err := spec.Of(chartOf(geom.Scatter(src, geom.X("t"), geom.Y("v"),
		geom.ColorBy("series", scale.Qualitative(palette.OkabeIto)))))
	if err != nil {
		t.Fatal(err)
	}
	ch := s.Layer[0].Encoding.Color
	if ch == nil {
		t.Fatal("the colour channel is missing")
	}
	if ch.Type != "nominal" {
		t.Errorf("the colour channel is %q, want a category", ch.Type)
	}
	if ch.Scale == nil || ch.Scale.Scheme != "okabeito" {
		t.Errorf("the palette is %+v, want it named", ch.Scale)
	}
	if len(ch.Scale.Domain) != 0 {
		t.Error("a discovered category set was written down as a domain")
	}
}
