package geom_test

import (
	"errors"
	"math"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// frameWith builds a trained frame over the given scales, the way render does.
func frameWith(t *testing.T, g geom.Geom, x, y scale.Scale) (*irtest.Recorder, geom.Frame) {
	t.Helper()
	if err := g.Train(x, y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	area := ir.R(0, 0, 100, 100)
	x.SetRange(area.Min.X, area.Max.X)
	y.SetRange(area.Max.Y, area.Min.Y)
	return irtest.New(), geom.Frame{Area: area, X: x, Y: y, Theme: theme.Light}
}

func build(t *testing.T, g geom.Geom, x, y scale.Scale) *irtest.Recorder {
	t.Helper()
	rec, f := frameWith(t, g, x, y)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return rec
}

// --- area ----------------------------------------------------------------

func TestAreaFillsToTheBaselineAndStrokesItsEdge(t *testing.T) {
	g := geom.Area(src(map[string][]float64{
		"x": {0, 1, 2},
		"y": {1, 3, 2},
	}), geom.X("x"), geom.Y("y"))

	rec := build(t, g, scale.Linear(), scale.Linear())
	fills := rec.Filter("FillPath")
	if len(fills) != 1 {
		t.Fatalf("got %d fills, want 1", len(fills))
	}
	// Three data points, then down to the baseline and back: five points.
	if n := len(fills[0].Path.Pts); n != 5 {
		t.Errorf("the filled path has %d points, want 5 (three samples plus two baseline corners)", n)
	}
	if fills[0].Fill.Color.A == 255 {
		t.Error("an inherited area fill must be faded so the marks in front of it stay readable")
	}
	if rec.Count("StrokePath") != 1 {
		t.Errorf("got %d strokes, want the upper edge drawn once", rec.Count("StrokePath"))
	}
}

func TestAreaWithY2DrawsABandAndBothEdges(t *testing.T) {
	g := geom.Area(src(map[string][]float64{
		"x":  {0, 1, 2},
		"lo": {0, 1, 0},
		"hi": {2, 4, 3},
	}), geom.X("x"), geom.Y("hi"), geom.Y2("lo"))

	rec := build(t, g, scale.Linear(), scale.Linear())
	fills := rec.Filter("FillPath")
	if len(fills) != 1 {
		t.Fatalf("got %d fills, want 1", len(fills))
	}
	if n := len(fills[0].Path.Pts); n != 6 {
		t.Errorf("the band path has %d points, want 6 (three up, three back)", n)
	}
	if rec.Count("StrokePath") != 2 {
		t.Errorf("got %d strokes, want both edges of the band", rec.Count("StrokePath"))
	}
}

func TestAreaTrainsTheBaselineIntoTheDomain(t *testing.T) {
	g := geom.Area(src(map[string][]float64{
		"x": {0, 1},
		"y": {40, 50},
	}), geom.X("x"), geom.Y("y"))

	y := scale.Linear()
	if err := g.Train(scale.Linear(), y); err != nil {
		t.Fatal(err)
	}
	if lo, _ := y.Domain(); lo != 0 {
		t.Errorf("domain minimum is %v; an area filled to zero must include zero or it is clipped", lo)
	}
}

func TestAreaBreaksAtAGap(t *testing.T) {
	nan := math.NaN()
	g := geom.Area(src(map[string][]float64{
		"x": {0, 1, 2, 3, 4},
		"y": {1, 2, nan, 4, 5},
	}), geom.X("x"), geom.Y("y"))

	rec := build(t, g, scale.Linear(), scale.Linear())
	if n := rec.Count("FillPath"); n != 2 {
		t.Errorf("got %d fills, want 2 — a hole in an area is a hole, not a bridge", n)
	}
}

func TestAreaOpacityIsHonoured(t *testing.T) {
	g := geom.Area(src(map[string][]float64{"x": {0, 1}, "y": {1, 2}}),
		geom.X("x"), geom.Y("y"), geom.Color(palette.Blue), geom.Opacity(0.5))

	rec := build(t, g, scale.Linear(), scale.Linear())
	fills := rec.Filter("FillPath")
	if len(fills) != 1 {
		t.Fatalf("got %d fills, want 1", len(fills))
	}
	if got := fills[0].Fill.Color.A; got != 128 {
		t.Errorf("fill alpha = %d, want 128", got)
	}
}

// --- step ----------------------------------------------------------------

func TestStepInsertsACornerPerInterval(t *testing.T) {
	cases := []struct {
		name  string
		where geom.StepPos
		want  []ir.Point // the first three device points
	}{
		{"post", geom.StepPost, []ir.Point{{X: 0, Y: 100}, {X: 50, Y: 100}, {X: 50, Y: 50}}},
		{"pre", geom.StepPre, []ir.Point{{X: 0, Y: 100}, {X: 0, Y: 50}, {X: 50, Y: 50}}},
		{"mid", geom.StepMid, []ir.Point{{X: 0, Y: 100}, {X: 25, Y: 100}, {X: 25, Y: 50}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := geom.Step(src(map[string][]float64{
				"x": {0, 1, 2},
				"y": {0, 1, 2},
			}), geom.X("x"), geom.Y("y"), geom.Steps(tc.where))

			rec := build(t, g, scale.Linear(), scale.Linear())
			lines := rec.Filter("Polyline")
			if len(lines) != 1 {
				t.Fatalf("got %d polylines, want 1", len(lines))
			}
			got := lines[0].Points
			for i, want := range tc.want {
				if math.Abs(float64(got[i].X-want.X)) > 0.01 || math.Abs(float64(got[i].Y-want.Y)) > 0.01 {
					t.Errorf("point %d = %v, want %v (full path %v)", i, got[i], want, got)
				}
			}
		})
	}
}

func TestStepUsesSquareCornersNotRoundedOnes(t *testing.T) {
	g := geom.Step(src(map[string][]float64{"x": {0, 1}, "y": {0, 1}}), geom.X("x"), geom.Y("y"))
	rec := build(t, g, scale.Linear(), scale.Linear())
	s := rec.Filter("Polyline")[0].Stroke
	if s.Join != ir.JoinMiter || s.Cap != ir.CapButt {
		t.Error("a step's corners are the data; rounding them rounds away what the geom is for")
	}
}

func TestStepBreaksAtAGap(t *testing.T) {
	nan := math.NaN()
	g := geom.Step(src(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {0, nan, 2, 3},
	}), geom.X("x"), geom.Y("y"))
	rec := build(t, g, scale.Linear(), scale.Linear())
	if n := rec.Count("Polyline"); n != 1 {
		t.Errorf("got %d polylines, want 1 — a lone point either side of the gap draws nothing", n)
	}
}

// --- boxplot -------------------------------------------------------------

// boxSource builds two groups whose quartiles are known by hand.
func boxSource() data.Source {
	// Group 1: 1..9 — median 5, Q1 3, Q3 7, no outliers.
	// Group 2: 1..9 plus 100 — the 100 is beyond Q3 + 1.5·IQR.
	xs := make([]float64, 0, 19)
	ys := make([]float64, 0, 19)
	for v := 1.0; v <= 9; v++ {
		xs, ys = append(xs, 1), append(ys, v)
	}
	for v := 1.0; v <= 9; v++ {
		xs, ys = append(xs, 2), append(ys, v)
	}
	xs, ys = append(xs, 2), append(ys, 100)
	return src(map[string][]float64{"g": xs, "v": ys})
}

func TestBoxplotSummarisesEachGroup(t *testing.T) {
	g := geom.Boxplot(boxSource(), geom.X("g"), geom.Y("v"))
	y := scale.Linear()
	rec, f := frameWith(t, g, scale.Linear(), y)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// One box per group, filled and stroked.
	if n := rec.Count("FillPath"); n != 2 {
		t.Errorf("got %d box fills, want one per group", n)
	}
	if n := rec.Count("StrokePath"); n != 2 {
		t.Errorf("got %d box outlines, want one per group", n)
	}
	// Per group: a median line, two whiskers and two caps.
	if n := rec.Count("Polyline"); n != 10 {
		t.Errorf("got %d line segments, want 10 (median, two whiskers and two caps per group)", n)
	}
	// Only the second group has an outlier.
	markers := rec.Filter("Markers")
	if len(markers) != 1 || len(markers[0].Points) != 1 {
		t.Errorf("got %v outlier batches, want exactly one point", markers)
	}
	// The domain has to reach the outlier, or it would be drawn outside the plot.
	if _, hi := y.Domain(); hi < 100 {
		t.Errorf("domain maximum is %v; it must include the outlier at 100", hi)
	}
}

func TestBoxplotQuartilesUseLinearInterpolation(t *testing.T) {
	// 1..9 has median 5, Q1 3 and Q3 7 under the type-7 definition.
	g := geom.Boxplot(src(map[string][]float64{
		"g": {1, 1, 1, 1, 1, 1, 1, 1, 1},
		"v": {1, 2, 3, 4, 5, 6, 7, 8, 9},
	}), geom.X("g"), geom.Y("v"))

	y := scale.Linear(scale.Domain(0, 10))
	rec := build(t, g, scale.Linear(), y)

	// The box spans Q1..Q3, which on a 0..10 domain over an inverted 100..0
	// range is 70..30.
	box := rec.Filter("StrokePath")[0].Path.Bounds()
	if math.Abs(float64(box.Min.Y-30)) > 0.01 || math.Abs(float64(box.Max.Y-70)) > 0.01 {
		t.Errorf("box spans Y %v..%v, want 30..70 (Q3 = 7, Q1 = 3)", box.Min.Y, box.Max.Y)
	}
	// The median line is drawn heavier than the box.
	med := rec.Filter("Polyline")[0]
	if math.Abs(float64(med.Points[0].Y-50)) > 0.01 {
		t.Errorf("median line at Y %v, want 50 (median = 5)", med.Points[0].Y)
	}
	if med.Stroke.Width <= rec.Filter("StrokePath")[0].Stroke.Width {
		t.Error("the median is the number a reader takes off a boxplot; it must be drawn heavier than the box")
	}
}

func TestBoxplotWhiskersStopAtAnObservation(t *testing.T) {
	// 1..9 with one point at 20: the fence is at 13, so the upper whisker must
	// stop at 9 rather than reaching out to the fence.
	g := geom.Boxplot(src(map[string][]float64{
		"g": {1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		"v": {1, 2, 3, 4, 5, 6, 7, 8, 9, 20},
	}), geom.X("g"), geom.Y("v"))

	y := scale.Linear(scale.Domain(0, 20))
	rec := build(t, g, scale.Linear(), y)

	top := float32(100)
	for _, c := range rec.Filter("Polyline") {
		for _, p := range c.Points {
			if p.Y < top {
				top = p.Y
			}
		}
	}
	// 9 on a 0..20 domain over an inverted 100..0 range is 55.
	if math.Abs(float64(top-55)) > 0.01 {
		t.Errorf("the whisker reaches Y %v, want 55 — whiskers stop at data, not at the fence", top)
	}
}

func TestBoxplotOutliersCanBeTurnedOff(t *testing.T) {
	g := geom.Boxplot(boxSource(), geom.X("g"), geom.Y("v"), geom.Outliers(false))
	rec := build(t, g, scale.Linear(), scale.Linear())
	if n := rec.Count("Markers"); n != 0 {
		t.Errorf("got %d marker batches, want none", n)
	}
}

func TestBoxplotTakesItsWidthFromABandScale(t *testing.T) {
	tbl := data.NewTable().
		String("team", []string{"a", "a", "a", "b", "b", "b"}).
		Float64("v", []float64{1, 2, 3, 4, 5, 6})

	g := geom.Boxplot(tbl, geom.X("team"), geom.Y("v"))
	x := scale.Ordinal()
	rec := build(t, g, x, scale.Linear())

	want := x.(scale.Band).Bandwidth()
	box := rec.Filter("StrokePath")[0].Path.Bounds()
	if math.Abs(float64(box.Max.X-box.Min.X-want)) > 0.01 {
		t.Errorf("box is %v wide, want the scale's bandwidth %v", box.Max.X-box.Min.X, want)
	}
}

// --- categorical columns -------------------------------------------------

func TestCategoricalColumnNeedsAnOrdinalScale(t *testing.T) {
	tbl := data.NewTable().
		String("region", []string{"north", "south"}).
		Float64("sales", []float64{3, 4})

	g := geom.Bar(tbl, geom.X("region"), geom.Y("sales"))
	err := g.Train(scale.Linear(), scale.Linear())
	if !errors.Is(err, geom.ErrCategorical) {
		t.Fatalf("err = %v, want ErrCategorical — a continuous axis has no position for a name", err)
	}
}

func TestCategoricalColumnEncodesThroughTheAxis(t *testing.T) {
	tbl := data.NewTable().
		String("region", []string{"north", "south", "east"}).
		Float64("sales", []float64{3, 4, 5})

	g := geom.Bar(tbl, geom.X("region"), geom.Y("sales"))
	x := scale.Ordinal()
	rec := build(t, g, x, scale.Linear())

	if got := x.(scale.Categorical).Labels(); len(got) != 3 || got[0] != "north" {
		t.Errorf("Labels() = %v, want the three regions in data order", got)
	}
	fills := rec.Filter("FillPath")
	if len(fills) != 1 {
		t.Fatalf("got %d fills, want one path holding all three bars", len(fills))
	}
	if n := len(fills[0].Path.Pts); n != 12 {
		t.Errorf("the bar path has %d points, want 12 (four corners each)", n)
	}
}

func TestNumericColumnOnAnOrdinalAxisBecomesCategories(t *testing.T) {
	g := geom.Bar(src(map[string][]float64{
		"x": {10, 20, 30},
		"y": {1, 2, 3},
	}), geom.X("x"), geom.Y("y"))

	x := scale.Ordinal()
	build(t, g, x, scale.Linear())
	got := x.(scale.Categorical).Labels()
	if len(got) != 3 || got[0] != "10" || got[2] != "30" {
		t.Errorf("Labels() = %v, want [10 20 30] — an ordinal axis over numbers gives equal slots", got)
	}
}

func TestBarsOnABandScaleUseItsBandwidth(t *testing.T) {
	tbl := data.NewTable().
		String("k", []string{"a", "b"}).
		Float64("v", []float64{1, 2})

	g := geom.Bar(tbl, geom.X("k"), geom.Y("v"))
	x := scale.Ordinal()
	rec := build(t, g, x, scale.Linear())

	pts := rec.Filter("FillPath")[0].Path.Pts
	if got, want := pts[1].X-pts[0].X, x.(scale.Band).Bandwidth(); math.Abs(float64(got-want)) > 0.01 {
		t.Errorf("bar is %v wide, want the scale's bandwidth %v", got, want)
	}
}

// --- log scales and missing data -----------------------------------------

func TestALogAxisTreatsANonPositiveValueAsMissing(t *testing.T) {
	g := geom.Line(src(map[string][]float64{
		"x": {1, 2, 3, 4, 5},
		"y": {1, 10, -5, 100, 1000},
	}), geom.X("x"), geom.Y("y"))

	rec := build(t, g, scale.Linear(), scale.Log())
	if n := rec.Count("Polyline"); n != 2 {
		t.Errorf("got %d polylines, want 2 — a value a log axis cannot place is a hole", n)
	}
	for _, c := range rec.Filter("Polyline") {
		for _, p := range c.Points {
			if math.IsNaN(float64(p.X)) || math.IsNaN(float64(p.Y)) {
				t.Fatal("a NaN coordinate reached the backend")
			}
		}
	}
}

func TestTheErrorPolicyCoversValuesTheScaleCannotPlace(t *testing.T) {
	g := geom.Line(src(map[string][]float64{
		"x": {1, 2, 3},
		"y": {1, 0, 100},
	}), geom.X("x"), geom.Y("y"), geom.OnMissing(geom.Error))

	if err := g.Train(scale.Linear(), scale.Log()); err == nil {
		t.Fatal("want an error: zero has no position on a log axis")
	}
}

func TestBarsOnALogAxisGrowFromTheAxisNotFromNaN(t *testing.T) {
	g := geom.Bar(src(map[string][]float64{
		"x": {1, 2, 3},
		"y": {10, 100, 1000},
	}), geom.X("x"), geom.Y("y"))

	rec := build(t, g, scale.Linear(), scale.Log(scale.LogNice()))
	for _, p := range rec.Filter("FillPath")[0].Path.Pts {
		if math.IsNaN(float64(p.Y)) {
			t.Fatal("a bar reached the backend with a NaN edge; the default baseline of zero is off a log axis")
		}
	}
}

// --- colour scales -------------------------------------------------------

func TestScatterColoursEachPointFromAColourScale(t *testing.T) {
	g := geom.Scatter(src(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {0, 1, 2, 3},
		"z": {0, 0, 100, 100},
	}), geom.X("x"), geom.Y("y"), geom.ColorBy("z", scale.Sequential(palette.Viridis)))

	rec := build(t, g, scale.Linear(), scale.Linear())
	batches := rec.Filter("Markers")
	if len(batches) != 2 {
		t.Fatalf("got %d marker batches, want 2 — points sharing a colour are drawn together", len(batches))
	}
	if len(batches[0].Points) != 2 || len(batches[1].Points) != 2 {
		t.Errorf("batches hold %d and %d points, want 2 and 2", len(batches[0].Points), len(batches[1].Points))
	}
	if batches[0].Style.Fill == batches[1].Style.Fill {
		t.Error("both batches came out the same colour")
	}
	if got, want := batches[0].Style.Fill, palette.Viridis[0]; got != want {
		t.Errorf("the low end is %v, want the bottom of the ramp %v", got, want)
	}
}

func TestALayerColouredByAScaleContributesNoLegendEntry(t *testing.T) {
	g := geom.Scatter(src(map[string][]float64{
		"x": {0, 1}, "y": {0, 1}, "z": {0, 1},
	}), geom.X("x"), geom.Y("y"), geom.Label("points"),
		geom.ColorBy("z", scale.Sequential(nil)))

	_, f := frameWith(t, g, scale.Linear(), scale.Linear())
	if _, ok := g.Legend(f); ok {
		t.Error("a continuous colour scale needs a colourbar, not a one-swatch legend entry that lies about it")
	}
}

func TestBarsColouredByAScaleGetOnePathEach(t *testing.T) {
	g := geom.Bar(src(map[string][]float64{
		"x": {1, 2, 3},
		"y": {1, 2, 3},
		"z": {1, 2, 3},
	}), geom.X("x"), geom.Y("y"), geom.ColorBy("z", scale.Sequential(palette.Blues)))

	rec := build(t, g, scale.Linear(), scale.Linear())
	if n := rec.Count("FillPath"); n != 3 {
		t.Errorf("got %d fills, want one per bar", n)
	}
}

func TestColorByTrainsItsScaleFromTheColumn(t *testing.T) {
	cs := scale.Sequential(palette.Viridis)
	g := geom.Scatter(src(map[string][]float64{
		"x": {0, 1}, "y": {0, 1}, "z": {-7, 42},
	}), geom.X("x"), geom.Y("y"), geom.ColorBy("z", cs))

	if err := g.Train(scale.Linear(), scale.Linear()); err != nil {
		t.Fatal(err)
	}
	if lo, hi := cs.Domain(); lo != -7 || hi != 42 {
		t.Errorf("colour domain = (%v, %v), want (-7, 42)", lo, hi)
	}
}

func TestColorByNamesAMissingColumn(t *testing.T) {
	g := geom.Scatter(src(map[string][]float64{"x": {0}, "y": {0}}),
		geom.X("x"), geom.Y("y"), geom.ColorBy("nope", scale.Sequential(nil)))
	if err := g.Train(scale.Linear(), scale.Linear()); !errors.Is(err, geom.ErrNoColumn) {
		t.Fatalf("err = %v, want ErrNoColumn", err)
	}
}

func TestColorByRejectsACategoricalColumn(t *testing.T) {
	tbl := data.NewTable().
		Float64("x", []float64{0, 1}).
		Float64("y", []float64{0, 1}).
		String("kind", []string{"a", "b"})

	g := geom.Scatter(tbl, geom.X("x"), geom.Y("y"),
		geom.ColorBy("kind", scale.Sequential(nil)))
	err := g.Train(scale.Linear(), scale.Linear())
	if !errors.Is(err, geom.ErrCategorical) {
		t.Fatalf("err = %v, want ErrCategorical", err)
	}
}
