package interact_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/interact"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func src() data.Source {
	return data.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3, 4},
		"y": {0, 4, 1, 9, 2},
	})
}

// indexed renders a chart through a watched recorder and returns the index.
func indexed(t *testing.T, layers ...geom.Geom) (*interact.Index, *irtest.Recorder) {
	t.Helper()
	ix := interact.New()
	rec := irtest.New()
	c := render.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X: scale.Linear(scale.Nice()), Y: scale.Linear(scale.Nice()),
		Layers: layers, Observer: ix,
	}
	if err := render.Draw(ix.Watch(rec), c); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	return ix, rec
}

func TestWatchingChangesNothing(t *testing.T) {
	layer := func() geom.Geom { return geom.Line(src(), geom.X("x"), geom.Y("y")) }

	plain := irtest.New()
	c := render.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X: scale.Linear(scale.Nice()), Y: scale.Linear(scale.Nice()),
		Layers: []geom.Geom{layer()},
	}
	if err := render.Draw(plain, c); err != nil {
		t.Fatal(err)
	}
	_, watched := indexed(t, layer())

	want, got := plain.Trace(), watched.Trace()
	if len(want) != len(got) {
		t.Fatalf("a watched render emitted %d calls, an unwatched one %d", len(got), len(want))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("call %d differs:\n plain: %s\n watch: %s", i, want[i], got[i])
		}
	}
}

func TestOnlyDataMarksAreIndexed(t *testing.T) {
	ix, rec := indexed(t, geom.Line(src(), geom.X("x"), geom.Y("y")))

	// The chart draws a background, a plot fill, a grid, two axes and a title
	// as well as the line; only the line is a mark a reader can point at.
	if got := ix.MarkCount(); got != 1 {
		t.Errorf("indexed %d marks, want 1 — the furniture must not be indexed", got)
	}
	if len(rec.Calls) < 10 {
		t.Fatalf("the chart drew only %d calls; the test is not exercising the furniture", len(rec.Calls))
	}
}

func TestAHitReportsTheRowUnderThePointer(t *testing.T) {
	ix, _ := indexed(t, geom.Scatter(src(), geom.X("x"), geom.Y("y"), geom.Label("series")))
	p := ix.Panels()[0]

	// Point at the third row, (2, 1).
	at := ir.Point{X: p.X.Map(2), Y: p.Y.Map(1)}
	hit, ok := ix.At(at, 0)
	if !ok {
		t.Fatal("no hit on a marker the pointer is exactly on")
	}
	if hit.Series != "series" {
		t.Errorf("series = %q, want the layer's label", hit.Series)
	}
	if hit.Layer != 0 || hit.Panel != 0 {
		t.Errorf("hit = panel %d layer %d, want 0/0", hit.Panel, hit.Layer)
	}
	if math.Abs(hit.X-2) > 0.01 || math.Abs(hit.Y-1) > 0.01 {
		t.Errorf("hit = (%v,%v), want (2,1)", hit.X, hit.Y)
	}
	if hit.Distance > 0.01 {
		t.Errorf("distance = %v, want 0 on the marker itself", hit.Distance)
	}
}

func TestAPointerNearNothingMisses(t *testing.T) {
	ix, _ := indexed(t, geom.Scatter(src(), geom.X("x"), geom.Y("y")))
	p := ix.Panels()[0]
	// Halfway between two rows, vertically far from either.
	at := ir.Point{X: p.X.Map(2.5), Y: p.Y.Map(6)}
	if hit, ok := ix.At(at, 4); ok {
		t.Errorf("hit %v at a point with no mark within four pixels", hit)
	}
}

func TestABarIsHitByBeingInsideIt(t *testing.T) {
	ix, _ := indexed(t, geom.Bar(src(), geom.X("x"), geom.Y("y")))
	p := ix.Panels()[0]

	// The middle of the fourth bar, which is tall: nowhere near a vertex.
	at := ir.Point{X: p.X.Map(3), Y: p.Y.Map(4)}
	hit, ok := ix.At(at, 1)
	if !ok {
		t.Fatal("no hit inside a bar")
	}
	if hit.Kind != interact.Area {
		t.Errorf("kind = %v, want Area", hit.Kind)
	}
	if hit.Distance != 0 {
		t.Errorf("distance = %v, want 0 inside the shape", hit.Distance)
	}
	// The reported position is a corner of the bar, which is where its value
	// is: the top of the bar, not the point the pointer happened to be at.
	if math.Abs(hit.Y-9) > 0.1 && math.Abs(hit.Y) > 0.1 {
		t.Errorf("hit.Y = %v, want the bar's top (9) or its base (0)", hit.Y)
	}
}

func TestTheTopmostMarkWins(t *testing.T) {
	under := data.Float64Columns(map[string][]float64{"x": {1}, "y": {1}})
	over := data.Float64Columns(map[string][]float64{"x": {1}, "y": {1}})
	ix, _ := indexed(t,
		geom.Scatter(under, geom.X("x"), geom.Y("y"), geom.Label("under")),
		geom.Scatter(over, geom.X("x"), geom.Y("y"), geom.Label("over")),
	)
	p := ix.Panels()[0]
	hit, ok := ix.At(ir.Point{X: p.X.Map(1), Y: p.Y.Map(1)}, 0)
	if !ok {
		t.Fatal("no hit")
	}
	if hit.Series != "over" {
		t.Errorf("series = %q, want the layer drawn last", hit.Series)
	}
}

func TestPanelAtFindsThePanel(t *testing.T) {
	ix, _ := indexed(t, geom.Line(src(), geom.X("x"), geom.Y("y")))
	p := ix.Panels()[0]
	mid := ir.Point{X: (p.Area.Min.X + p.Area.Max.X) / 2, Y: (p.Area.Min.Y + p.Area.Max.Y) / 2}
	if i, ok := ix.PanelAt(mid); !ok || i != 0 {
		t.Errorf("PanelAt(centre) = %d, %v", i, ok)
	}
	if _, ok := ix.PanelAt(ir.Point{X: 1, Y: 1}); ok {
		t.Error("the top-left corner of the canvas is in the margins, not in a panel")
	}
}

func TestResetEmptiesTheIndex(t *testing.T) {
	ix, _ := indexed(t, geom.Line(src(), geom.X("x"), geom.Y("y")))
	if ix.MarkCount() == 0 {
		t.Fatal("nothing indexed")
	}
	ix.Reset()
	if ix.MarkCount() != 0 || len(ix.Panels()) != 0 {
		t.Errorf("after Reset: %d marks, %d panels", ix.MarkCount(), len(ix.Panels()))
	}
	if _, ok := ix.At(ir.Point{X: 100, Y: 100}, 0); ok {
		t.Error("an empty index found something")
	}
}

func TestEventKindsAreNamed(t *testing.T) {
	for _, k := range []interact.EventKind{
		interact.Hover, interact.Leave, interact.Click, interact.Zoom, interact.Pan,
	} {
		if k.String() == "unknown" {
			t.Errorf("kind %d has no name", k)
		}
	}
}

func TestSeriesIsEmptyWithoutAHit(t *testing.T) {
	if got := (interact.Event{Kind: interact.Hover}).Series(); got != "" {
		t.Errorf("Series() = %q on an event with no hit", got)
	}
	ev := interact.Event{Kind: interact.Hover, Found: true, Hit: interact.Hit{Series: "s"}}
	if got := ev.Series(); got != "s" {
		t.Errorf("Series() = %q, want s", got)
	}
}

func TestADensityRasterIsIndexedAsAnArea(t *testing.T) {
	// A crowded scatter is drawn as one image rather than as markers, and a
	// pointer over it should still land on the layer that drew it.
	n := 20000
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = float64(i % 200)
		y[i] = float64((i * 7) % 200)
	}
	src := data.Float64Columns(map[string][]float64{"x": x, "y": y})

	ix, rec := indexed(t, geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.Label("cloud")))
	if rec.Count("Image") == 0 {
		t.Fatal("the crowded scatter was not rasterized; the test is not exercising Image")
	}
	p := ix.Panels()[0]
	mid := ir.Point{X: (p.Area.Min.X + p.Area.Max.X) / 2, Y: (p.Area.Min.Y + p.Area.Max.Y) / 2}
	hit, ok := ix.At(mid, 0)
	if !ok {
		t.Fatal("no hit inside the raster")
	}
	if hit.Kind != interact.Area || hit.Series != "cloud" {
		t.Errorf("hit = %+v, want an area belonging to the cloud", hit)
	}
}

func TestTheProbeForwardsEverything(t *testing.T) {
	// Every method has to reach the wrapped backend, including the two that do
	// nothing here — a Flush that stopped at the probe would leave a file
	// unwritten.
	ix := interact.New()
	rec := irtest.New()
	b := ix.Watch(rec)
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	if rec.Frames != 1 {
		t.Error("Flush did not reach the backend")
	}
	if got := b.Measure(ir.TextRun{Text: "x", Font: ir.FontRef{Size: 10}}); got.Advance == 0 {
		t.Error("Measure did not reach the backend")
	}
}

func TestRowIdentityIsOffUntilAskedFor(t *testing.T) {
	ix := interact.New()
	if ix.TrackingRows() {
		t.Error("a new index tracks rows")
	}
	if ix.TrackRows(true) != ix {
		t.Error("TrackRows does not return the index it was called on")
	}
	if !ix.TrackingRows() {
		t.Error("TrackRows(true) did not turn tracking on")
	}
	// Marks outside a layer are furniture and carry no row, tracking or not.
	ix.Marks([]ir.Point{{X: 1, Y: 2}}, []int{7})
	if ix.RowCount() != 0 {
		t.Errorf("a mark reported outside a layer was kept: %d", ix.RowCount())
	}
}

func TestMismatchedRowReportsAreDropped(t *testing.T) {
	// A geom handing over two slices of different lengths has a bug; taking
	// the shorter of the two would turn it into wrong rows rather than none.
	ix := interact.New().TrackRows(true)
	ix.Panel(0, ir.R(0, 0, 10, 10), scale.Linear(), scale.Linear(), coord.Cartesian())
	ix.Layer(0, "s")
	ix.Marks([]ir.Point{{X: 1}, {X: 2}}, []int{0})
	if ix.RowCount() != 0 {
		t.Errorf("a mismatched report was kept: %d rows", ix.RowCount())
	}
}

func TestResetForgetsRows(t *testing.T) {
	ix := interact.New().TrackRows(true)
	ix.Panel(0, ir.R(0, 0, 10, 10), scale.Linear(), scale.Linear(), coord.Cartesian())
	ix.Layer(0, "s")
	ix.Marks([]ir.Point{{X: 1, Y: 1}}, []int{4})
	if ix.RowCount() != 1 {
		t.Fatalf("rows = %d", ix.RowCount())
	}
	ix.Reset()
	if ix.RowCount() != 0 {
		t.Errorf("after Reset: %d rows", ix.RowCount())
	}
	if !ix.TrackingRows() {
		t.Error("Reset turned tracking off; it is a setting, not frame state")
	}
}

func TestARowIsNotTakenFromAnotherLayer(t *testing.T) {
	// Two series crossing at a point: the hit belongs to one of them, and so
	// must the row.
	a := data.Float64Columns(map[string][]float64{"x": {0, 1, 2}, "y": {0, 5, 0}})
	b := data.Float64Columns(map[string][]float64{"x": {0, 1, 2}, "y": {5, 5, 5}})

	ix := interact.New().TrackRows(true)
	rec := irtest.New()
	c := render.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X: scale.Linear(scale.Nice()), Y: scale.Linear(scale.Nice()),
		Layers: []geom.Geom{
			geom.Scatter(a, geom.X("x"), geom.Y("y"), geom.Label("a")),
			geom.Scatter(b, geom.X("x"), geom.Y("y"), geom.Label("b")),
		},
		Observer: ix, RowSink: ix,
	}
	if err := render.Draw(ix.Watch(rec), c); err != nil {
		t.Fatal(err)
	}
	p := ix.Panels()[0]
	hit, ok := ix.At(ir.Point{X: p.X.Map(1), Y: p.Y.Map(5)}, 0)
	if !ok {
		t.Fatal("no hit where the two series meet")
	}
	if hit.Series != "b" {
		t.Fatalf("the hit belongs to %q, want the layer drawn last", hit.Series)
	}
	if hit.Row != 1 {
		t.Errorf("row = %d, want 1", hit.Row)
	}
}
