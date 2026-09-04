package geom_test

import (
	"errors"
	"math"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// frame builds a trained frame over the given source, the way render would.
func frame(t *testing.T, g geom.Geom) (*irtest.Recorder, geom.Frame) {
	t.Helper()
	x := scale.Linear()
	y := scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	area := ir.R(0, 0, 100, 100)
	x.SetRange(area.Min.X, area.Max.X)
	y.SetRange(area.Max.Y, area.Min.Y)
	return irtest.New(), geom.Frame{Area: area, X: x, Y: y, Theme: theme.Light}
}

func src(cols map[string][]float64) data.Source { return data.Float64Columns(cols) }

func TestLineEmitsOnePolylinePerRunOfData(t *testing.T) {
	nan := math.NaN()
	g := geom.Line(src(map[string][]float64{
		"x": {0, 1, 2, 3, 4, 5},
		"y": {0, 1, nan, 3, 4, 5},
	}), geom.X("x"), geom.Y("y"))

	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}

	lines := rec.Filter("Polyline")
	if len(lines) != 2 {
		t.Fatalf("got %d polylines, want 2 — a gap must break the line, not bridge it", len(lines))
	}
	if len(lines[0].Points) != 2 || len(lines[1].Points) != 3 {
		t.Fatalf("segments have %d and %d points, want 2 and 3",
			len(lines[0].Points), len(lines[1].Points))
	}
}

func TestLineInterpolateFillsTheHole(t *testing.T) {
	nan := math.NaN()
	g := geom.Line(src(map[string][]float64{
		"x": {0, 1, 2, 3, 4},
		"y": {0, 1, nan, 3, 4},
	}), geom.X("x"), geom.Y("y"), geom.OnMissing(geom.Interpolate))

	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	lines := rec.Filter("Polyline")
	if len(lines) != 1 {
		t.Fatalf("got %d polylines, want 1 continuous line", len(lines))
	}
	if len(lines[0].Points) != 5 {
		t.Fatalf("got %d points, want 5 — the hole should have been filled, not dropped", len(lines[0].Points))
	}
	// The interpolated point must sit on the straight line between its
	// neighbours, which for y = x means it equals the x position.
	mid := lines[0].Points[2]
	if math.Abs(float64(mid.Y-mid.X)) > 0.5 {
		t.Errorf("interpolated point %v is not on the line between its neighbours", mid)
	}
}

func TestLineErrorPolicyRejectsMissingData(t *testing.T) {
	nan := math.NaN()
	g := geom.Line(src(map[string][]float64{
		"x": {0, 1, 2},
		"y": {0, nan, 2},
	}), geom.X("x"), geom.Y("y"), geom.OnMissing(geom.Error))

	err := g.Train(scale.Linear(), scale.Linear())
	if err == nil {
		t.Fatal("OnMissing(Error) must reject a NaN")
	}
}

func TestLineTensionEmitsCurves(t *testing.T) {
	g := geom.Line(src(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {0, 2, 1, 3},
	}), geom.X("x"), geom.Y("y"), geom.Tension(0.5))

	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	paths := rec.Filter("StrokePath")
	if len(paths) != 1 {
		t.Fatalf("got %d stroked paths, want 1", len(paths))
	}

	// The curve must pass through every data point: a smoothing that misses
	// the data draws something that was never measured.
	var ends []ir.Point
	paths[0].Path.Walk(func(op ir.PathOp, pts []ir.Point) {
		switch op {
		case ir.OpMoveTo:
			ends = append(ends, pts[0])
		case ir.OpCubicTo:
			ends = append(ends, pts[2])
		}
	})
	if len(ends) != 4 {
		t.Fatalf("curve has %d on-curve points, want 4", len(ends))
	}
	for i, want := range []float32{0, 1, 2, 3} {
		if got := f.X.Map(float64(want)); math.Abs(float64(ends[i].X-got)) > 1e-3 {
			t.Errorf("on-curve point %d is at x=%v, want %v", i, ends[i].X, got)
		}
	}
}

func TestLineWithoutTensionEmitsAPolyline(t *testing.T) {
	g := geom.Line(src(map[string][]float64{"x": {0, 1}, "y": {0, 1}}), geom.X("x"), geom.Y("y"))
	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if rec.Count("Polyline") != 1 || rec.Count("StrokePath") != 0 {
		t.Fatalf("a tension-free line should take the Polyline fast path, got %v", rec.Ops())
	}
}

func TestScatterSkipsMissingPoints(t *testing.T) {
	nan := math.NaN()
	g := geom.Scatter(src(map[string][]float64{
		"x": {0, 1, 2},
		"y": {0, nan, 2},
	}), geom.X("x"), geom.Y("y"))

	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	ms := rec.Filter("Markers")
	if len(ms) != 1 {
		t.Fatalf("got %d marker calls, want 1 batched call", len(ms))
	}
	if len(ms[0].Points) != 2 {
		t.Fatalf("got %d markers, want 2 — the missing row must be skipped", len(ms[0].Points))
	}
}

func TestBarIncludesTheBaselineInTheDomain(t *testing.T) {
	g := geom.Bar(src(map[string][]float64{
		"x": {1, 2, 3},
		"y": {20, 30, 25},
	}), geom.X("x"), geom.Y("y"))

	y := scale.Linear()
	if err := g.Train(scale.Linear(), y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	lo, _ := y.Domain()
	if lo != 0 {
		t.Fatalf("bar domain starts at %v — a bar chart whose baseline is off-screen misstates magnitude", lo)
	}
}

func TestBarWidensTheDomainSoOuterBarsFit(t *testing.T) {
	g := geom.Bar(src(map[string][]float64{
		"x": {1, 2, 3},
		"y": {1, 2, 3},
	}), geom.X("x"), geom.Y("y"))

	x := scale.Linear()
	if err := g.Train(x, scale.Linear()); err != nil {
		t.Fatalf("Train: %v", err)
	}
	lo, hi := x.Domain()
	if lo >= 1 || hi <= 3 {
		t.Fatalf("domain %v..%v does not leave room for the outer bars' width", lo, hi)
	}
}

func TestBarEmitsOneFilledPathForAllBars(t *testing.T) {
	g := geom.Bar(src(map[string][]float64{
		"x": {1, 2, 3},
		"y": {1, 2, 3},
	}), geom.X("x"), geom.Y("y"))

	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	fills := rec.Filter("FillPath")
	if len(fills) != 1 {
		t.Fatalf("got %d fill calls, want 1 batched path", len(fills))
	}
	// Three rectangles: MoveTo + 3 LineTo + Close each.
	if got := len(fills[0].Path.Ops); got != 15 {
		t.Fatalf("path has %d ops, want 15 for three closed rectangles", got)
	}
}

func TestMissingColumnIsReportedNotPanicked(t *testing.T) {
	g := geom.Line(src(map[string][]float64{"x": {1}}), geom.X("x"), geom.Y("absent"))
	err := g.Train(scale.Linear(), scale.Linear())
	if !errors.Is(err, geom.ErrNoColumn) {
		t.Fatalf("err = %v, want ErrNoColumn", err)
	}
}

func TestSeriesTakesItsColourFromThePaletteByIndex(t *testing.T) {
	g := geom.Line(src(map[string][]float64{"x": {0, 1}, "y": {0, 1}}), geom.X("x"), geom.Y("y"))
	rec, f := frame(t, g)
	f.Index = 2
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := theme.Light.Palette.At(2)
	if got := rec.Filter("Polyline")[0].Stroke.Color; got != want {
		t.Fatalf("layer 2 drew in %v, want the palette's third colour %v", got, want)
	}
}
