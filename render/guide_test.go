package render_test

import (
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func colored(cs scale.ColorScale, label ...geom.Option) geom.Geom {
	src := data.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {0, 2, 1, 3},
		"v": {10, 20, 30, 40},
	})
	opts := append([]geom.Option{geom.X("x"), geom.Y("y"), geom.ColorBy("v", cs)}, label...)
	return geom.Scatter(src, opts...)
}

// gradients returns the fills that are gradients, which is what a colourbar
// paints and nothing else in a chart does.
func gradients(rec *irtest.Recorder) []irtest.Call {
	var out []irtest.Call
	for _, c := range rec.Filter("FillPath") {
		if c.Fill.IsGradient() {
			out = append(out, c)
		}
	}
	return out
}

func TestColorByLayerGetsAColorbar(t *testing.T) {
	rec := draw(t, chart(colored(scale.Sequential(palette.Viridis))))
	bars := gradients(rec)
	if len(bars) != 1 {
		t.Fatalf("drew %d colourbars, want 1", len(bars))
	}
	stops := bars[0].Fill.Stops
	if len(stops) < 8 {
		t.Errorf("the ramp was sampled into %d stops, too few to be smooth", len(stops))
	}
	if stops[0].Offset != 0 || stops[len(stops)-1].Offset != 1 {
		t.Errorf("stops run from %v to %v, want 0 to 1", stops[0].Offset, stops[len(stops)-1].Offset)
	}
	// The gradient runs bottom to top, so its start is below its end.
	if bars[0].Fill.Start.Y <= bars[0].Fill.End.Y {
		t.Error("the bar's low end is not at the bottom")
	}
	if stops[0].Color != palette.Viridis[0] {
		t.Errorf("the bar starts at %v, want the ramp's first colour", stops[0].Color)
	}
}

// A layer whose colour comes from a scale is not a series, so it must not also
// claim a legend row.
func TestColorByLayerContributesNoLegendEntry(t *testing.T) {
	c := chart(colored(scale.Sequential(palette.Viridis)), line())
	c.ShowLegend = true
	rec := draw(t, c)

	var labels []string
	for _, call := range rec.Filter("Text") {
		labels = append(labels, call.Text.Text)
	}
	seen := 0
	for _, l := range labels {
		if l == "series" {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("the line's legend row appears %d times", seen)
	}
	if len(gradients(rec)) != 1 {
		t.Error("the colourbar is missing when a legend is also present")
	}
}

func TestColorbarIsTitledWithTheColumn(t *testing.T) {
	rec := draw(t, chart(colored(scale.Sequential(palette.Viridis))))
	if !hasText(rec, "v") {
		t.Errorf("the bar is not titled with the coloured column: %v", texts(rec))
	}
}

func TestColorbarTitleCanBeNamed(t *testing.T) {
	rec := draw(t, chart(colored(scale.Sequential(palette.Viridis), geom.Label("throughput"))))
	if !hasText(rec, "throughput") {
		t.Errorf("Label did not title the bar: %v", texts(rec))
	}
}

// The bar is an axis, so it gets round numbers from the same tick machinery
// the Y axis uses.
func TestColorbarIsTicked(t *testing.T) {
	rec := draw(t, chart(colored(scale.Sequential(palette.Viridis))))
	for _, want := range []string{"10", "20", "30", "40"} {
		if !hasText(rec, want) {
			t.Errorf("the bar has no %q tick: %v", want, texts(rec))
		}
	}
}

// Two layers sharing one colour scale mean one quantity, so one bar.
func TestIdenticalColorGuidesAreMerged(t *testing.T) {
	cs := scale.Sequential(palette.Viridis)
	rec := draw(t, chart(colored(cs), colored(cs)))
	if got := len(gradients(rec)); got != 1 {
		t.Errorf("drew %d colourbars for one shared scale, want 1", got)
	}
}

func TestDistinctColorGuidesEachGetABar(t *testing.T) {
	a := colored(scale.Sequential(palette.Viridis), geom.Label("a"))
	b := colored(scale.Sequential(palette.Magma), geom.Label("b"))
	rec := draw(t, chart(a, b))
	if got := len(gradients(rec)); got != 2 {
		t.Errorf("drew %d colourbars for two different scales, want 2", got)
	}
}

func TestNoColorScaleMeansNoColorbar(t *testing.T) {
	rec := draw(t, chart(line()))
	if got := len(gradients(rec)); got != 0 {
		t.Errorf("drew %d colourbars for a chart with no colour scale", got)
	}
}

// The guide column has to come out of the plot's width, or the bar is drawn
// over the data.
func TestTheGuideColumnNarrowsThePlot(t *testing.T) {
	withBar := plotRight(t, chart(colored(scale.Sequential(palette.Viridis))))
	without := plotRight(t, chart(line()))
	if withBar >= without {
		t.Errorf("plot reaches x=%v with a colourbar and x=%v without: the bar took no room", withBar, without)
	}
}

// plotRight recovers the right edge of the plot area from the clip the layers
// are drawn inside.
func plotRight(t *testing.T, c render.Chart) float32 {
	t.Helper()
	rec := draw(t, c)
	for _, call := range rec.Calls {
		if call.Op == "Push" && call.HasClip {
			return call.ClipRect.Max.X
		}
	}
	t.Fatal("no clipped layer group was pushed")
	return 0
}

func TestColorbarSurvivesADegenerateDomain(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{
		"x": {0, 1}, "y": {0, 1}, "v": {7, 7},
	})
	g := geom.Scatter(src, geom.X("x"), geom.Y("y"),
		geom.ColorBy("v", scale.Sequential(palette.Viridis)))
	rec := draw(t, chart(g))
	if got := len(gradients(rec)); got != 1 {
		t.Errorf("drew %d colourbars over a constant column, want 1", got)
	}
}

func TestColorbarIsBorderedByTheTheme(t *testing.T) {
	c := chart(colored(scale.Sequential(palette.Viridis)))
	c.Theme = theme.Light.With(func(t *theme.Theme) { t.ColorbarBorder = ir.Transparent })
	rec := draw(t, c)
	for _, call := range rec.Filter("StrokePath") {
		if call.Stroke.Color == theme.Light.ColorbarBorder {
			t.Error("the bar was outlined despite a transparent border token")
		}
	}
}

func hasText(rec *irtest.Recorder, s string) bool {
	for _, t := range texts(rec) {
		if t == s {
			return true
		}
	}
	return false
}

func texts(rec *irtest.Recorder) []string { return rec.Texts() }
