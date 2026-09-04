package render_test

import (
	"math"
	"slices"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func chart(layers ...geom.Geom) render.Chart {
	return render.Chart{
		Width:  600,
		Height: 400,
		DPR:    1,
		Theme:  theme.Light,
		X:      scale.Linear(scale.Nice()),
		Y:      scale.Linear(scale.Nice()),
		Layers: layers,
	}
}

func line() geom.Geom {
	src := data.Float64Columns(map[string][]float64{"x": {0, 1, 2, 3}, "y": {0, 2, 1, 3}})
	return geom.Line(src, geom.X("x"), geom.Y("y"), geom.Label("series"))
}

func draw(t *testing.T, c render.Chart) *irtest.Recorder {
	t.Helper()
	rec := irtest.New()
	if err := render.Draw(rec, c); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	return rec
}

func TestDrawPaintsBackgroundFirst(t *testing.T) {
	rec := draw(t, chart(line()))
	if len(rec.Calls) == 0 || rec.Calls[0].Op != "FillPath" {
		t.Fatalf("first call is %v, want the background fill", rec.Ops())
	}
	if got := rec.Calls[0].Fill.Color; got != theme.Light.Background {
		t.Errorf("background painted in %v, want %v", got, theme.Light.Background)
	}
}

func TestDataIsClipped(t *testing.T) {
	rec := draw(t, chart(line()))

	pushes := rec.Filter("Push")
	if len(pushes) != 1 || !pushes[0].HasClip {
		t.Fatalf("expected exactly one clipping Push around the data, got %v", rec.Ops())
	}
	if rec.Count("Pop") != 1 {
		t.Errorf("Push and Pop are unbalanced: %v", rec.Ops())
	}
	if rec.Depth != 0 {
		t.Errorf("the state stack was left at depth %d", rec.Depth)
	}
}

func TestDataIsDrawnInsideTheClip(t *testing.T) {
	rec := draw(t, chart(line()))

	pushAt := slices.Index(rec.Ops(), "Push")
	popAt := slices.Index(rec.Ops(), "Pop")
	dataAt := -1
	for i := pushAt; i < popAt; i++ {
		if rec.Calls[i].Op == "Polyline" && len(rec.Calls[i].Points) == 4 {
			dataAt = i
		}
	}
	if dataAt < 0 {
		t.Fatalf("the data polyline was not emitted between Push and Pop: %v", rec.Ops())
	}
}

func TestAxesAndTicksAreLabelled(t *testing.T) {
	c := chart(line())
	c.Title = "Title"
	c.XTitle = "x axis"
	c.YTitle = "y axis"
	rec := draw(t, c)

	texts := rec.Texts()
	for _, want := range []string{"Title", "x axis", "y axis"} {
		if !slices.Contains(texts, want) {
			t.Errorf("missing text %q; got %v", want, texts)
		}
	}
	// Tick labels for both axes plus the three titles.
	if len(texts) < 3+4 {
		t.Errorf("only %d text runs were emitted: %v", len(texts), texts)
	}
}

func TestYAxisTitleIsRotatedAQuarterTurn(t *testing.T) {
	c := chart(line())
	c.YTitle = "amplitude"
	rec := draw(t, c)

	for _, call := range rec.Filter("Text") {
		if call.Text.Text != "amplitude" {
			continue
		}
		if math.Abs(call.Text.Rotation+math.Pi/2) > 1e-9 {
			t.Fatalf("Y title rotation = %v, want -pi/2 so it reads bottom-to-top", call.Text.Rotation)
		}
		return
	}
	t.Fatal("the Y axis title was never drawn")
}

func TestGridCanBeTurnedOff(t *testing.T) {
	c := chart(line())
	withGrid := draw(t, c)

	c.Theme.ShowGridX = false
	c.Theme.ShowGridY = false
	without := draw(t, c)

	if without.Count("Polyline") >= withGrid.Count("Polyline") {
		t.Fatal("disabling the grid did not reduce the number of strokes")
	}
}

func TestLegendAppearsOnlyWhenAsked(t *testing.T) {
	c := chart(line())
	if got := draw(t, c).Texts(); slices.Contains(got, "series") {
		t.Errorf("legend drawn without ShowLegend: %v", got)
	}
	c.ShowLegend = true
	if got := draw(t, c).Texts(); !slices.Contains(got, "series") {
		t.Errorf("legend not drawn with ShowLegend: %v", got)
	}
}

func TestLegendSwatchMatchesTheSeriesColour(t *testing.T) {
	c := chart(line())
	c.ShowLegend = true
	rec := draw(t, c)

	// The last stroke outside the clip is the legend's line swatch.
	popAt := slices.Index(rec.Ops(), "Pop")
	var swatch *irtest.Call
	for i := popAt; i < len(rec.Calls); i++ {
		if rec.Calls[i].Op == "Polyline" {
			swatch = &rec.Calls[i]
		}
	}
	if swatch == nil {
		t.Fatal("no legend swatch was drawn")
	}
	want := theme.Light.Palette.At(0)
	if swatch.Stroke.Color != want {
		t.Errorf("swatch colour %v, want the series colour %v", swatch.Stroke.Color, want)
	}
}

func TestTickLabelsDoNotCollide(t *testing.T) {
	// A narrow chart with long time labels must drop labels rather than let
	// them overlap.
	c := chart(line())
	c.Width = 200
	c.Theme.TickCountHintX = 12
	rec := draw(t, c)

	var prevRight float32 = -1e30
	for _, call := range rec.Filter("Text") {
		if call.Text.V != ir.AlignTop || call.Text.H != ir.AlignCenter {
			continue // not an X tick label
		}
		w := rec.Measure(call.Text).Advance
		left := call.Text.At.X - w/2
		if left < prevRight {
			t.Fatalf("X tick label %q overlaps its predecessor", call.Text.Text)
		}
		prevRight = call.Text.At.X + w/2
	}
}

func TestGeomErrorsPropagate(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{"x": {1}})
	c := chart(geom.Line(src, geom.X("x"), geom.Y("missing")))
	if err := render.Draw(irtest.New(), c); err == nil {
		t.Fatal("a geom error must abort the render")
	}
}

func TestEmptyChartStillDrawsAxes(t *testing.T) {
	c := chart()
	rec := draw(t, c)
	if rec.Count("Polyline") == 0 {
		t.Fatal("a chart with no layers should still draw its axes")
	}
	if rec.Count("Push") != 0 {
		t.Error("with no layers there is nothing to clip")
	}
}

func TestScalesEndUpMappedToThePlotArea(t *testing.T) {
	c := chart(line())
	rec := draw(t, c)

	pushes := rec.Filter("Push")
	if len(pushes) == 0 {
		t.Fatal("no clip was pushed")
	}
	plot := pushes[0].ClipRect

	// After Draw, the scales must map the domain onto the plot rectangle, with
	// Y inverted.
	lo, hi := c.X.Domain()
	if got := c.X.Map(lo); math.Abs(float64(got-plot.Min.X)) > 0.01 {
		t.Errorf("X domain minimum maps to %v, want the plot left edge %v", got, plot.Min.X)
	}
	if got := c.X.Map(hi); math.Abs(float64(got-plot.Max.X)) > 0.01 {
		t.Errorf("X domain maximum maps to %v, want the plot right edge %v", got, plot.Max.X)
	}
	ylo, yhi := c.Y.Domain()
	if got := c.Y.Map(ylo); math.Abs(float64(got-plot.Max.Y)) > 0.01 {
		t.Errorf("Y domain minimum maps to %v, want the plot bottom %v", got, plot.Max.Y)
	}
	if got := c.Y.Map(yhi); math.Abs(float64(got-plot.Min.Y)) > 0.01 {
		t.Errorf("Y domain maximum maps to %v, want the plot top %v", got, plot.Min.Y)
	}
}

// --- v0.2 axis behaviour -------------------------------------------------

// gridLines counts the polylines drawn in the grid's own style. The grid and
// the axis are different colours in both shipped themes, which is what lets a
// test tell a grid line from a tick mark without measuring either.
func gridLines(rec *irtest.Recorder, th theme.Theme, horizontal bool) int {
	n := 0
	for _, c := range rec.Filter("Polyline") {
		if c.Stroke.Color != th.GridColor || len(c.Points) != 2 {
			continue
		}
		if (c.Points[0].Y == c.Points[1].Y) == horizontal {
			n++
		}
	}
	return n
}

func TestMinorTicksGetAMarkButNoGridLine(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{"x": {1, 2, 3}, "y": {1, 100, 10000}})
	c := chart(geom.Line(src, geom.X("x"), geom.Y("y")))
	c.Y = scale.Log(scale.LogNice())
	rec := draw(t, c)

	minors := 0
	for _, tk := range c.Y.Ticks(theme.Light.TickCountHintY) {
		if tk.Minor {
			minors++
		}
	}
	if minors == 0 {
		t.Fatal("the log axis produced no minor ticks, so this test proves nothing")
	}
	// Five decades, so every labelled Y tick contributes one horizontal grid
	// line; the subdivisions must contribute none.
	majors := len(c.Y.Ticks(theme.Light.TickCountHintY)) - minors
	horizontal := gridLines(rec, theme.Light, true)
	if horizontal != majors {
		t.Errorf("got %d horizontal grid lines for %d labelled ticks and %d subdivisions; "+
			"a subdivision must not draw one", horizontal, majors, minors)
	}
}

func TestMinorTickMarksAreShorterThanMajorOnes(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{"x": {1, 2, 3}, "y": {1, 5, 30}})
	c := chart(geom.Line(src, geom.X("x"), geom.Y("y")))
	c.Y = scale.Log(scale.LogNice())
	rec := draw(t, c)

	var lengths []float64
	for _, call := range rec.Filter("Polyline") {
		if call.Stroke.Color != theme.Light.AxisColor || len(call.Points) != 2 {
			continue
		}
		if call.Points[0].Y != call.Points[1].Y {
			continue
		}
		if d := math.Abs(float64(call.Points[1].X - call.Points[0].X)); d > 0 && d <= float64(theme.Light.TickLength) {
			lengths = append(lengths, d)
		}
	}
	if len(lengths) < 2 {
		t.Fatalf("expected both major and minor tick marks, got %v", lengths)
	}
	if slices.Max(lengths) == slices.Min(lengths) {
		t.Error("minor ticks are drawn the same length as major ones; the labelled ticks must stand out")
	}
}

func TestABandAxisDrawsNoGridLinesThroughItsMarks(t *testing.T) {
	tbl := data.NewTable().
		String("k", []string{"a", "b", "c"}).
		Float64("v", []float64{1, 2, 3})

	c := chart(geom.Bar(tbl, geom.X("k"), geom.Y("v")))
	c.X = scale.Ordinal()
	rec := draw(t, c)

	if n := gridLines(rec, theme.Light, false); n != 0 {
		t.Errorf("a categorical axis drew %d vertical grid lines; each one runs through a bar", n)
	}
	// The Y axis is still continuous, so its grid must survive — the rule is
	// about band scales, not about charts that happen to have categories.
	if n := gridLines(rec, theme.Light, true); n == 0 {
		t.Error("the continuous Y grid was suppressed too")
	}
	if n := gridLines(draw(t, chart(line())), theme.Light, false); n == 0 {
		t.Fatal("a continuous X axis drew no grid either, so this test proves nothing")
	}
}
