package refract_test

// Row identity, end to end: a hit can name the source row behind the mark it
// landed on, on every geom that has one — through decimation, across a gap,
// and out of the cut faceting makes.

import (
	"math"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

func TestRowsDecimated(t *testing.T) {
	const n = 200_000
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = float64(i)
		y[i] = math.Sin(float64(i) / 500)
	}
	y[n/3] = 5
	src := refract.Float64Columns(map[string][]float64{"x": x, "y": y})

	p := refract.New(refract.Size(800, 400))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()
	t.Logf("rows tracked: %d of %d source rows", ix.RowCount(), n)

	panel := ix.Panels()[0]
	// The spike survives the reduction; the row it reports must be the source
	// row, not an index into whatever survived.
	at := ir.Point{X: panel.X.Map(float64(n / 3)), Y: panel.Y.Map(5)}
	hit, ok := ix.At(at, 4)
	if !ok {
		t.Fatal("no hit on the spike")
	}
	if hit.Row != n/3 {
		t.Errorf("the spike reports row %d, want the source row %d", hit.Row, n/3)
	}
}

func TestRowsWithGaps(t *testing.T) {
	nan := math.NaN()
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3, 4, 5},
		"y": {1, 2, nan, 4, 5, 6},
	})
	p := refract.New(refract.Size(400, 300))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()
	panel := ix.Panels()[0]
	// Row 4 is in the second segment, which starts at source row 3.
	hit, ok := ix.At(ir.Point{X: panel.X.Map(4), Y: panel.Y.Map(5)}, 4)
	if !ok {
		t.Fatal("no hit")
	}
	if hit.Row != 4 {
		t.Errorf("the mark after the gap reports row %d, want 4", hit.Row)
	}
}

func TestRowsInterpolated(t *testing.T) {
	nan := math.NaN()
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3, 4},
		"y": {1, 2, nan, 4, 5},
	})
	p := refract.New(refract.Size(400, 300))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y"), geom.OnMissing(geom.Interpolate)))
	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()
	panel := ix.Panels()[0]
	if hit, ok := ix.At(ir.Point{X: panel.X.Map(3), Y: panel.Y.Map(4)}, 4); !ok || hit.Row != 3 {
		t.Errorf("a measured row reports %d, want 3 (ok=%v)", hit.Row, ok)
	}
	// The invented point at x=2 has no row, and must not borrow a neighbour's.
	if hit, ok := ix.At(ir.Point{X: panel.X.Map(2), Y: panel.Y.Map(3)}, 3); ok && hit.Row != -1 {
		t.Errorf("the interpolated point reports row %d, want none", hit.Row)
	}
}

func TestRowsFaceted(t *testing.T) {
	tbl := refract.NewTable().
		Float64("x", []float64{0, 1, 2, 3}).
		Float64("y", []float64{10, 20, 30, 40}).
		String("g", []string{"a", "b", "a", "b"})

	p := refract.New(refract.Size(600, 300))
	p.Add(geom.Scatter(tbl, geom.X("x"), geom.Y("y")))
	p.Facet(facet.Wrap("g"))

	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()
	if len(ix.Panels()) != 2 {
		t.Fatalf("panels = %d", len(ix.Panels()))
	}
	// Panel "b" holds source rows 1 and 3. Its second point must report 3,
	// not 1 — the row of the table that was handed in, not of the cut refract
	// made to build the panel.
	p1 := ix.Panels()[1]
	hit, ok := ix.At(ir.Point{X: p1.X.Map(3), Y: p1.Y.Map(40)}, 4)
	if !ok {
		t.Fatal("no hit in the second panel")
	}
	if hit.Row != 3 {
		t.Errorf("the faceted mark reports row %d, want the table's row 3", hit.Row)
	}
}

func TestRowsOffByDefault(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {1, 2}})
	p := refract.New(refract.Size(400, 300))
	p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y")))
	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()
	if ix.RowCount() != 0 {
		t.Errorf("row tracking is on by default: %d rows recorded", ix.RowCount())
	}
	panel := ix.Panels()[0]
	hit, ok := ix.At(ir.Point{X: panel.X.Map(1), Y: panel.Y.Map(2)}, 0)
	if !ok {
		t.Fatal("no hit")
	}
	if hit.Row != -1 {
		t.Errorf("Row = %d with tracking off, want -1", hit.Row)
	}
}

func TestRowsBoxplotHasNone(t *testing.T) {
	tbl := refract.NewTable().
		String("g", []string{"a", "a", "a", "a", "b", "b", "b", "b"}).
		Float64("v", []float64{1, 2, 3, 40, 5, 6, 7, 8})
	p := refract.New(refract.Size(400, 300))
	p.X(scale.Ordinal())
	p.Add(geom.Boxplot(tbl, geom.X("g"), geom.Y("v")))
	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()
	if ix.RowCount() != 0 {
		t.Errorf("a boxplot reported %d rows; a box aggregates and has none", ix.RowCount())
	}
}

// benchmarkRows measures what row tracking costs a frame that is already being
// watched: the marks are indexed either way, and this is the position and row
// number kept on top of them.
func benchmarkRows(b *testing.B, track bool) {
	onOnePGate(b)

	const n = 100_000
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = float64(i)
		y[i] = math.Sin(float64(i) / 500)
	}
	src := refract.Float64Columns(map[string][]float64{"x": x, "y": y})

	p := refract.New(refract.Size(800, 400))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))

	live, err := p.Live(irtest.NullTarget())
	if err != nil {
		b.Fatal(err)
	}
	defer live.Close()
	live.TrackRows(track)
	if err := live.Draw(); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// Move a point, so that every frame is a frame rather than a skip.
		y[i%n] = float64(i%7) / 10
		if err := live.Draw(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWatchedFrame(b *testing.B)     { benchmarkRows(b, false) }
func BenchmarkWatchedFrameRows(b *testing.B) { benchmarkRows(b, true) }
