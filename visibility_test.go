package refract_test

// A clickable legend: what a hit on a row reports, and what turning a series
// off does to the chart.

import (
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

// twoSeries is a chart with a legend of two rows, which is the smallest chart
// where hiding one leaves something to look at.
func twoSeries(t *testing.T) (*refract.Plot, *refract.Live, *irtest.Recorder) {
	t.Helper()
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"a": {10, 20, 15, 25},
		"b": {5, 8, 6, 9},
	})
	p := refract.New(refract.Size(640, 320), refract.Legend(true))
	p.X(scale.Linear(scale.Domain(0, 3)))
	p.Y(scale.Linear(scale.Domain(0, 30)))
	p.Add(
		geom.Line(src, geom.X("x"), geom.Y("a"), geom.Label("a"), geom.Color(palette.Blue)),
		geom.Line(src, geom.X("x"), geom.Y("b"), geom.Label("b"), geom.Color(palette.Orange)),
	)
	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	return p, live, rec
}

// rowOf finds where a legend row was drawn, by asking the index the way a
// pointer would.
func rowOf(t *testing.T, l *refract.Live, label string) ir.Point {
	t.Helper()
	ix := l.Index()
	for x := float32(2); x < 640; x += 2 {
		for y := float32(2); y < 320; y += 2 {
			hit, ok := ix.At(ir.Point{X: x, Y: y}, 0)
			if ok && hit.Kind == refract.LegendRow && hit.Series == label {
				return ir.Point{X: x, Y: y}
			}
		}
	}
	t.Fatalf("no legend row for %q", label)
	return ir.Point{}
}

// A hit on a legend row says which series it stands for and which layer it
// toggles — and does not pretend to be a reading off an axis.
func TestALegendRowReportsItsSeries(t *testing.T) {
	_, live, _ := twoSeries(t)
	at := rowOf(t, live, "b")

	hit, ok := live.Index().At(at, 0)
	if !ok {
		t.Fatal("no hit")
	}
	if hit.Kind != refract.LegendRow {
		t.Errorf("kind = %v, want guide", hit.Kind)
	}
	if hit.Layer != 1 || hit.Series != "b" {
		t.Errorf("layer %d series %q, want 1 and %q", hit.Layer, hit.Series, "b")
	}
	if hit.Panel != -1 {
		t.Errorf("panel = %d, want -1 — a legend belongs to the chart", hit.Panel)
	}
	// A guide carries no value: inverting its position through a panel's
	// scales would report a number from a place no number was drawn.
	if hit.X != 0 || hit.Y != 0 {
		t.Errorf("the guide reports x=%v y=%v, want no value at all", hit.X, hit.Y)
	}
	if hit.Row != -1 {
		t.Errorf("row = %d, want -1", hit.Row)
	}
}

// A hover in the margins finds a legend row, which is the one thing out there
// worth finding — but still no marks, because a hover in the margin is over no
// data.
func TestAHoverInTheMarginFindsTheLegend(t *testing.T) {
	p, live, _ := twoSeries(t)
	var got refract.Event
	p.On(refract.Hover, func(ev refract.Event) { got = ev })
	p.On(refract.Leave, func(ev refract.Event) { got = ev })

	at := rowOf(t, live, "a")
	live.Move(float64(at.X), float64(at.Y))
	if !got.Found || got.Hit.Kind != refract.LegendRow {
		t.Fatalf("a hover over the legend found %v (found=%v)", got.Hit.Kind, got.Found)
	}
	if got.Panel != -1 {
		t.Errorf("panel = %d, want -1", got.Panel)
	}

	// Somewhere else in the margin finds nothing.
	live.Move(4, 4)
	if got.Found {
		t.Errorf("a hover in an empty margin found %v", got.Hit.Kind)
	}
}

// Hiding a layer stops it being drawn, and stops it being under the pointer.
func TestHidingALayerStopsItBeingDrawnAndHit(t *testing.T) {
	_, live, rec := twoSeries(t)
	before := rec.Count("Polyline")
	marks := live.Index().MarkCount()

	// The recorder accumulates across frames, so the next frame is measured on
	// its own.
	rec.Reset()
	if err := live.Hide(1, true); err != nil {
		t.Fatal(err)
	}
	if !live.IsHidden(1) {
		t.Fatal("the layer did not hide")
	}
	if got := rec.Count("Polyline"); got >= before {
		t.Errorf("polylines = %d after hiding a line, want fewer than %d", got, before)
	}
	if got := live.Index().MarkCount(); got >= marks {
		t.Errorf("marks = %d after hiding a layer, want fewer than %d — a hidden mark must not be hittable", got, marks)
	}

	// The other one is untouched.
	if live.IsHidden(0) {
		t.Error("hiding one layer hid another")
	}
}

// The legend keeps the row. A row that vanished would take with it the only
// way of getting the series back, and would move the rows under the pointer.
func TestAHiddenSeriesKeepsItsLegendRow(t *testing.T) {
	_, live, _ := twoSeries(t)
	if err := live.Hide(1, true); err != nil {
		t.Fatal(err)
	}
	at := rowOf(t, live, "b")
	hit, ok := live.Index().At(at, 0)
	if !ok || hit.Kind != refract.LegendRow {
		t.Fatal("the hidden series lost its legend row")
	}
	if !hit.Hidden {
		t.Error("the row does not report that its series is hidden")
	}
	if hit.Layer != 1 {
		t.Errorf("layer = %d, want 1", hit.Layer)
	}
}

// The axes do not move. A toggle is a reading aid, and an axis that rescaled
// under it would make the two readings incomparable.
func TestHidingALayerDoesNotMoveTheAxes(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1}, "a": {0, 100}, "b": {0, 5},
	})
	p := refract.New(refract.Size(640, 320), refract.Legend(true))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(
		geom.Line(src, geom.X("x"), geom.Y("a"), geom.Label("a")),
		geom.Line(src, geom.X("x"), geom.Y("b"), geom.Label("b")),
	)
	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	before, beforeHi := live.Index().Panels()[0].Y.Domain()

	if err := live.Hide(0, true); err != nil { // the tall one
		t.Fatal(err)
	}
	after, afterHi := live.Index().Panels()[0].Y.Domain()
	if before != after || beforeHi != afterHi {
		t.Errorf("the axis moved from [%v %v] to [%v %v] when a series was hidden",
			before, beforeHi, after, afterHi)
	}
}

// Toggle is the one a legend click calls, and it goes both ways.
func TestToggleGoesBothWays(t *testing.T) {
	_, live, rec := twoSeries(t)
	full := rec.Count("Polyline")

	if err := live.Toggle(1); err != nil {
		t.Fatal(err)
	}
	if !live.IsHidden(1) {
		t.Fatal("the first toggle did not hide")
	}
	rec.Reset()
	if err := live.Toggle(1); err != nil {
		t.Fatal(err)
	}
	if live.IsHidden(1) {
		t.Fatal("the second toggle did not show")
	}
	if got := rec.Count("Polyline"); got != full {
		t.Errorf("polylines = %d after hiding and showing, want %d back again", got, full)
	}
}

// The four-line recipe from Live.Toggle's doc, run end to end: a click on a
// legend row puts the series away.
func TestClickingALegendRowHidesItsSeries(t *testing.T) {
	p, live, _ := twoSeries(t)
	p.On(refract.Click, func(ev refract.Event) {
		if ev.Hit.Kind == refract.LegendRow {
			live.Toggle(ev.Hit.Layer)
		}
	})

	at := rowOf(t, live, "b")
	in := live.Input()
	if err := in.Down(float64(at.X), float64(at.Y)); err != nil {
		t.Fatal(err)
	}
	if err := in.Up(float64(at.X), float64(at.Y)); err != nil {
		t.Fatal(err)
	}
	if !live.IsHidden(1) {
		t.Error("clicking the legend row did not hide its series")
	}

	// And back. The row is still there to click, which is the point of
	// dimming it rather than dropping it.
	again := rowOf(t, live, "b")
	if err := in.Down(float64(again.X), float64(again.Y)); err != nil {
		t.Fatal(err)
	}
	if err := in.Up(float64(again.X), float64(again.Y)); err != nil {
		t.Fatal(err)
	}
	if live.IsHidden(1) {
		t.Error("clicking the dimmed row did not bring the series back")
	}
}

func TestShowAllBringsEverythingBack(t *testing.T) {
	_, live, _ := twoSeries(t)
	if err := live.Hide(0, true); err != nil {
		t.Fatal(err)
	}
	if err := live.Hide(1, true); err != nil {
		t.Fatal(err)
	}
	if err := live.ShowAll(); err != nil {
		t.Fatal(err)
	}
	if live.IsHidden(0) || live.IsHidden(1) {
		t.Error("ShowAll left something hidden")
	}
}

// A hidden series survives a rebuild: adding a layer is not a caller saying
// they want back the ones a reader put away.
func TestHiddenSurvivesARebuild(t *testing.T) {
	p, live, _ := twoSeries(t)
	if err := live.Hide(1, true); err != nil {
		t.Fatal(err)
	}
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {1, 2}})
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y"), geom.Label("c")))
	if err := live.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	if !live.IsHidden(1) {
		t.Error("the rebuild brought back a series the reader had put away")
	}
	if live.IsHidden(2) {
		t.Error("the layer that was just added arrived hidden")
	}
}

// A layer index the chart does not have changes nothing rather than panicking
// or growing the slice for ever.
func TestHidingANonexistentLayerIsANoOp(t *testing.T) {
	_, live, _ := twoSeries(t)
	if err := live.Hide(9, true); err != nil {
		t.Fatal(err)
	}
	if err := live.Hide(-1, true); err != nil {
		t.Fatal(err)
	}
	if live.IsHidden(9) || live.IsHidden(-1) {
		t.Error("a layer that does not exist reports as hidden")
	}
}

// A chart that hides nothing draws exactly what it drew before there was any
// such thing.
func TestNothingHiddenChangesNothing(t *testing.T) {
	_, a, ra := twoSeries(t)
	_, b, rb := twoSeries(t)
	_ = a
	if err := b.Hide(0, false); err != nil { // a no-op hide
		t.Fatal(err)
	}
	if len(ra.Trace()) != len(rb.Trace()) {
		t.Errorf("a chart hiding nothing drew %d calls, want %d", len(rb.Trace()), len(ra.Trace()))
	}
}

// The picture. What to look at in the diff is that the hidden series is gone
// from the plot and still present in the legend, dimmed — because a row that
// vanished would take with it the only way of getting the series back.
func TestHiddenSeriesGolden(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3, 4},
		"a": {10, 24, 15, 28, 19},
		"b": {4, 9, 6, 12, 8},
	})
	p := refract.New(
		refract.Size(640, 320),
		refract.Legend(true),
		refract.Title("One series put away"),
		refract.XTitle("x"),
		refract.YTitle("y"),
	)
	p.X(scale.Linear(scale.Domain(0, 4)))
	p.Y(scale.Linear(scale.Domain(0, 30)))
	p.Add(
		geom.Line(src, geom.X("x"), geom.Y("a"), geom.Label("a"), geom.Color(palette.Blue)),
		geom.Line(src, geom.X("x"), geom.Y("b"), geom.Label("b"), geom.Color(palette.Orange)),
	)
	p.HideLayer(1, true)
	golden(t, "hidden-series", p)
}

// A Live hides a series on its own surface. Two surfaces over one plot are two
// readers, and one of them putting a series away is not the other one doing it.
func TestHidingOnOneLiveDoesNotReachThePlot(t *testing.T) {
	p, one, _ := twoSeries(t)

	two, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer two.Close()
	if err := two.Draw(); err != nil {
		t.Fatal(err)
	}

	if err := one.Hide(1, true); err != nil {
		t.Fatal(err)
	}
	if two.IsHidden(1) {
		t.Error("hiding a series on one surface hid it on another")
	}

	three, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer three.Close()
	if three.IsHidden(1) {
		t.Error("a Live opened afterwards inherited the other one's hidden series")
	}
}
