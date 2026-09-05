package spec_test

import (
	"strings"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/spec"
	"github.com/timzifer/refract/theme"
)

// The v0.9 additions to the document: seven distribution marks with the
// numbers that configure them, and the size channel.
//
// Vega-Lite reaches most of this through transforms — `bin`, `density`,
// `loess`, `regression` — which refract runs inside the layer. So the numbers
// travel with the mark and the difference is spelled out rather than disguised,
// which is what ADR 0014 asks for.

func distTable() *data.Table {
	var v, w []float64
	var cat, who []string
	for i := range 60 {
		v = append(v, float64(i%17)+float64(i)/40)
		w = append(w, float64((i*7)%13))
		cat = append(cat, []string{"a", "b", "c"}[i%3])
		who = append(who, []string{"x", "y"}[i%2])
	}
	return data.NewTable().
		Float64("v", v).Float64("w", w).
		String("cat", cat).String("who", who)
}

func TestEveryDistributionMarkSurvivesTheRoundTrip(t *testing.T) {
	src := distTable()
	cases := []struct {
		name  string
		layer geom.Geom
		x, y  scale.Scale
	}{
		{"histogram", geom.Histogram(src, geom.X("v"), geom.Bins(9), geom.BinRange(-1, 20)),
			scale.Linear(scale.Nice()), scale.Linear(scale.Nice())},
		{"violin", geom.Violin(src, geom.X("cat"), geom.Y("v"), geom.Bandwidth(0.7), geom.BarWidth(0.6)),
			scale.Ordinal(), scale.Linear(scale.Nice())},
		{"ridgeline", geom.Ridgeline(src, geom.X("v"), geom.Y("cat"), geom.Overlap(2.25)),
			scale.Linear(scale.Nice()), scale.Ordinal()},
		{"hexbin", geom.Hexbin(src, geom.X("v"), geom.Y("w"), geom.DensityCells(14)),
			scale.Linear(scale.Nice()), scale.Linear(scale.Nice())},
		{"beeswarm", geom.Beeswarm(src, geom.X("cat"), geom.Y("v"), geom.Size(5)),
			scale.Ordinal(), scale.Linear(scale.Nice())},
		{"ecdf", geom.ECDF(src, geom.X("v"), geom.GroupBy("who")),
			scale.Linear(scale.Nice()), scale.Linear(scale.Nice())},
		{"trend-loess", geom.Trend(src, geom.X("v"), geom.Y("w"), geom.Span(0.4)),
			scale.Linear(scale.Nice()), scale.Linear(scale.Nice())},
		{"trend-linear", geom.Trend(src, geom.X("v"), geom.Y("w"), geom.Smooth(geom.LinearFit)),
			scale.Linear(scale.Nice()), scale.Linear(scale.Nice())},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := spec.Chart{
				Width: 500, Height: 350, DPR: 1, Theme: theme.Light,
				X: tc.x, Y: tc.y, Layers: []geom.Geom{tc.layer},
			}
			want, got := draw(t, c), draw(t, roundTrip(t, c))
			if strings.Join(want, "\n") != strings.Join(got, "\n") {
				s, _ := spec.Of(c)
				b, _ := s.Marshal()
				t.Errorf("the %s mark did not survive the round trip\n%s", tc.name, b)
			}
		})
	}
}

// A mark writes the properties it uses and no others. A document listing a
// trend's bin count would read as though that meant something.
func TestADistributionMarkWritesOnlyItsOwnProperties(t *testing.T) {
	src := distTable()
	s, err := spec.Of(spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X: scale.Linear(), Y: scale.Linear(),
		Layers: []geom.Geom{
			geom.Histogram(src, geom.X("v"), geom.Bins(5)),
			geom.Trend(src, geom.X("v"), geom.Y("w"), geom.Span(0.5)),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	hist, trend := s.Layer[0].Mark, s.Layer[1].Mark
	if hist.Bins != 5 {
		t.Errorf("the histogram wrote bins = %d, want 5", hist.Bins)
	}
	if hist.Span != 0 || hist.Method != "" {
		t.Errorf("the histogram carries a trend's fit: %+v", hist)
	}
	if trend.Bins != 0 || trend.Bandwidth != 0 || trend.Overlap != 0 {
		t.Errorf("the trend carries a histogram's bins or a ridge's overlap: %+v", trend)
	}
	if trend.Span != 0.5 || trend.Method != "loess" {
		t.Errorf("the trend wrote span/method = %v/%q, want 0.5/loess", trend.Span, trend.Method)
	}
}

// A histogram that never pinned its interval writes no interval: the bins come
// from the data, and a document that spelled the data's extent out would pin it
// against a table that may have changed.
func TestAnUnpinnedBinRangeIsNotWrittenDown(t *testing.T) {
	s, err := spec.Of(spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X: scale.Linear(), Y: scale.Linear(),
		Layers: []geom.Geom{geom.Histogram(distTable(), geom.X("v"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := s.Layer[0].Mark
	if m.BinStart != nil || m.BinEnd != nil {
		t.Errorf("the interval was written as %v..%v, want nothing", m.BinStart, m.BinEnd)
	}
	if m.Bins != 0 {
		t.Errorf("bins = %d, want nothing: the layer chooses", m.Bins)
	}
}

// The size channel. It goes on `encoding.size`, where Vega-Lite puts it, and
// the scale says it is a size scale — because the mapping is by area, and a
// document that left the type off would read as a plain linear range.
func TestTheSizeChannelSurvivesTheRoundTrip(t *testing.T) {
	src := distTable()
	c := spec.Chart{
		Width: 500, Height: 350, DPR: 1, Theme: theme.Light,
		X: scale.Linear(scale.Nice()), Y: scale.Linear(scale.Nice()),
		Layers: []geom.Geom{geom.Scatter(src,
			geom.X("v"), geom.Y("w"),
			geom.SizeBy("w", scale.Size(scale.SizeRange(4, 36), scale.SizeZero(-1))))},
	}
	want, got := draw(t, c), draw(t, roundTrip(t, c))
	if strings.Join(want, "\n") != strings.Join(got, "\n") {
		s, _ := spec.Of(c)
		b, _ := s.Marshal()
		t.Errorf("the size channel did not survive the round trip\n%s", b)
	}

	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	ch := s.Layer[0].Encoding.Size
	if ch == nil || ch.Field != "w" {
		t.Fatalf("the size channel is %+v, want the column it reads", ch)
	}
	if ch.Scale == nil || ch.Scale.Type != "size" {
		t.Fatalf("the size scale is %+v, want its type spelled out", ch.Scale)
	}
	if len(ch.Scale.SizeRange) != 2 || ch.Scale.SizeRange[1] != 36 {
		t.Errorf("sizeRange = %v, want the pinned diameters", ch.Scale.SizeRange)
	}
	if ch.Scale.SizeZero == nil || *ch.Scale.SizeZero != -1 {
		t.Errorf("sizeZero = %v, want the anchor written out", ch.Scale.SizeZero)
	}
}

// A range the theme decided is not the chart's, so it is not written down: a
// spec drawn at another size has to be free to scale its bubbles.
func TestAThemeChosenSizeRangeIsNotWrittenDown(t *testing.T) {
	s, err := spec.Of(spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X: scale.Linear(), Y: scale.Linear(),
		Layers: []geom.Geom{geom.Scatter(distTable(),
			geom.X("v"), geom.Y("w"), geom.SizeBy("w", scale.Size()))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Layer[0].Encoding.Size.Scale.SizeRange; len(got) != 0 {
		t.Errorf("sizeRange = %v, want nothing: the theme set it", got)
	}
}

// A hand-written document reaches the new marks by name, with no scale types
// and no parse map — which is the shape a person types.
func TestAHandWrittenDistributionSpecReads(t *testing.T) {
	const doc = `{
	  "width": 400, "height": 300,
	  "data": {"values": [{"v": 1}, {"v": 2}, {"v": 2}, {"v": 5}]},
	  "encoding": {"x": {"type": "quantitative"}, "y": {"type": "quantitative"}},
	  "layer": [{"mark": {"type": "histogram", "bins": 3}, "encoding": {"x": {"field": "v"}}}]
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
	if d.Mark != geom.MarkHistogram || d.Bins != 3 {
		t.Errorf("read back as %q with %d bins", d.Mark, d.Bins)
	}
	draw(t, c)
}
