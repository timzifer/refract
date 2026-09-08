package refract_test

// Row identity that survives a frame: the key a layer names, the value a hit
// reports for it, and the reverse lookup that turns a row back into a place.

import (
	"math"
	"strconv"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
)

func keyTable() *data.Table {
	return refract.NewTable().
		Float64("x", []float64{0, 1, 2, 3}).
		Float64("y", []float64{10, 20, 30, 40}).
		String("id", []string{"w", "x", "y", "z"}).
		String("g", []string{"a", "b", "a", "b"})
}

// A hover reports the key of the row it landed on, and Hit.Row beside it.
func TestHoverReportsTheKey(t *testing.T) {
	var got refract.Event
	p := refract.New(refract.Size(600, 300))
	p.Add(geom.Scatter(keyTable(), geom.X("x"), geom.Y("y"), geom.KeyBy("id")))
	p.On(refract.Hover, func(ev refract.Event) { got = ev })

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	panel := live.Index().Panels()[0]
	at := ir.Point{X: panel.X.Map(2), Y: panel.Y.Map(30)}
	live.Move(float64(at.X), float64(at.Y))

	if !got.Found {
		t.Fatal("no hit under a point a mark was drawn at")
	}
	if got.Hit.Row != 2 {
		t.Errorf("row = %d, want 2", got.Hit.Row)
	}
	if got.Key != "y" {
		t.Errorf("key = %q, want %q", got.Key, "y")
	}
}

// A layer that named no key column has no key, and says so by being empty
// rather than by inventing the row number as one.
func TestNoKeyColumnNoKey(t *testing.T) {
	var got refract.Event
	p := refract.New(refract.Size(600, 300))
	p.Add(geom.Scatter(keyTable(), geom.X("x"), geom.Y("y")))
	p.On(refract.Hover, func(ev refract.Event) { got = ev })

	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	panel := live.Index().Panels()[0]
	at := ir.Point{X: panel.X.Map(2), Y: panel.Y.Map(30)}
	live.Move(float64(at.X), float64(at.Y))

	if !got.Found {
		t.Fatal("no hit")
	}
	if got.Key != "" {
		t.Errorf("key = %q, want empty for a layer that named no key column", got.Key)
	}
}

// Without row tracking there is no row, and therefore no key. A key resolved
// from a guessed row would be a confident wrong answer, which is the whole
// reason row identity is opt-in.
func TestKeyNeedsRowTracking(t *testing.T) {
	var got refract.Event
	p := refract.New(refract.Size(600, 300))
	p.Add(geom.Scatter(keyTable(), geom.X("x"), geom.Y("y"), geom.KeyBy("id")))
	p.On(refract.Hover, func(ev refract.Event) { got = ev })

	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	panel := live.Index().Panels()[0]
	at := ir.Point{X: panel.X.Map(2), Y: panel.Y.Map(30)}
	live.Move(float64(at.X), float64(at.Y))

	if !got.Found {
		t.Fatal("no hit")
	}
	if got.Hit.Row != -1 {
		t.Fatalf("row = %d with tracking off, want -1", got.Hit.Row)
	}
	if got.Key != "" {
		t.Errorf("key = %q with no row behind it, want empty", got.Key)
	}
}

// The trap this design can hide: on a faceted chart the panel's layer is a
// Subset copy over the cut, while Hit.Row is a row of the table that was
// handed in. Reading the key from the cut would index the wrong table and
// return a plausible, wrong answer rather than an error.
func TestKeyIsReadFromTheHandedInTable(t *testing.T) {
	var got refract.Event
	p := refract.New(refract.Size(600, 300))
	p.Add(geom.Scatter(keyTable(), geom.X("x"), geom.Y("y"), geom.KeyBy("id")))
	p.Facet(facet.Wrap("g"))
	p.On(refract.Hover, func(ev refract.Event) { got = ev })

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
	// Panel "b" holds source rows 1 and 3; its second point is the table's
	// row 3, whose key is "z". Read through the cut it would be the cut's
	// row 3, which does not exist — or, on a bigger table, somebody else's.
	p1 := ix.Panels()[1]
	at := ir.Point{X: p1.X.Map(3), Y: p1.Y.Map(40)}
	live.Move(float64(at.X), float64(at.Y))

	if !got.Found {
		t.Fatal("no hit in the second panel")
	}
	if got.Hit.Row != 3 {
		t.Fatalf("row = %d, want the handed-in table's row 3", got.Hit.Row)
	}
	if got.Key != "z" {
		t.Errorf("key = %q, want %q — the key was read from the facet's cut", got.Key, "z")
	}
}

// A numeric key column is spelled the way a facet panel key and a categorical
// tick are spelled, so the same number names the same thing everywhere.
func TestANumericKeyIsSpelledLikeACategory(t *testing.T) {
	var got refract.Event
	tbl := refract.NewTable().
		Float64("x", []float64{0, 1}).
		Float64("y", []float64{10, 20}).
		Float64("id", []float64{7, 8.5})

	p := refract.New(refract.Size(600, 300))
	p.Add(geom.Scatter(tbl, geom.X("x"), geom.Y("y"), geom.KeyBy("id")))
	p.On(refract.Hover, func(ev refract.Event) { got = ev })

	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	panel := live.Index().Panels()[0]
	at := ir.Point{X: panel.X.Map(1), Y: panel.Y.Map(20)}
	live.Move(float64(at.X), float64(at.Y))

	if got.Key != "8.5" {
		t.Errorf("key = %q, want %q", got.Key, "8.5")
	}
}

// Locate is the inverse of At: the point a hit reports for a row is the point
// the index hands back when asked where that row went.
func TestLocateIsTheInverseOfAt(t *testing.T) {
	p := refract.New(refract.Size(600, 300))
	p.Add(geom.Scatter(keyTable(), geom.X("x"), geom.Y("y"), geom.KeyBy("id")))

	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()

	for _, row := range []int{0, 1, 2, 3} {
		at, ok := ix.Locate(0, 0, row)
		if !ok {
			t.Fatalf("row %d was not located", row)
		}
		hit, ok := ix.At(at, 4)
		if !ok {
			t.Fatalf("nothing at the place row %d was located", row)
		}
		if hit.Row != row {
			t.Errorf("row %d located at %v, where At reports row %d", row, at, hit.Row)
		}
	}

	// A row that is not there is not somewhere.
	if _, ok := ix.Locate(0, 0, 99); ok {
		t.Error("a row the table does not have was located")
	}
	if _, ok := ix.Locate(0, 1, 0); ok {
		t.Error("a layer the chart does not have located a row")
	}
}

// Without row tracking there is nothing to locate. The index keeps no
// positions, so the answer is "no" rather than a nearby mark.
func TestLocateNeedsRowTracking(t *testing.T) {
	p := refract.New(refract.Size(600, 300))
	p.Add(geom.Scatter(keyTable(), geom.X("x"), geom.Y("y")))
	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	if _, ok := live.Index().Locate(0, 0, 1); ok {
		t.Error("a row was located with tracking off")
	}
}

// RowsOf reports one layer's rows and no other's, so two series crossing
// cannot take each other's.
func TestRowsOfIsConfinedToItsLayer(t *testing.T) {
	tbl := keyTable()
	p := refract.New(refract.Size(600, 300))
	p.Add(
		geom.Scatter(tbl, geom.X("x"), geom.Y("y")),
		geom.Line(tbl, geom.X("x"), geom.Y("y")),
	)
	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()

	var buf []refract.RowRef
	buf = ix.RowsOf(0, 0, buf[:0])
	if len(buf) != 4 {
		t.Fatalf("layer 0 reported %d rows, want 4", len(buf))
	}
	for _, r := range buf {
		if r.Layer != 0 {
			t.Errorf("RowsOf(0, 0) returned a row of layer %d", r.Layer)
		}
	}
	if got := len(ix.RowsOf(0, 1, nil)); got != 4 {
		t.Errorf("layer 1 reported %d rows, want 4", got)
	}
}

// RowsIn is what a brush reads: the rows whose reported position lies in the
// rectangle, and none outside it.
func TestRowsInSelectsByReportedPosition(t *testing.T) {
	p := refract.New(refract.Size(600, 300))
	p.Add(geom.Scatter(keyTable(), geom.X("x"), geom.Y("y"), geom.KeyBy("id")))
	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()

	// A box around rows 1 and 2 only, built from where they actually landed.
	a, _ := ix.Locate(0, 0, 1)
	b, _ := ix.Locate(0, 0, 2)
	box := ir.Rect{
		Min: ir.Point{X: min(a.X, b.X) - 2, Y: min(a.Y, b.Y) - 2},
		Max: ir.Point{X: max(a.X, b.X) + 2, Y: max(a.Y, b.Y) + 2},
	}

	got := map[int]bool{}
	for _, r := range ix.RowsIn(box, nil) {
		got[r.Row] = true
	}
	if !got[1] || !got[2] {
		t.Errorf("the brushed rows are %v, want 1 and 2 among them", got)
	}
	if got[0] || got[3] {
		t.Errorf("the brush took rows outside it: %v", got)
	}
}

// benchmarkHover measures what a pointer move costs: the hit test, and — with
// a key column named — the one column lookup that turns the row it found into
// an identity. The claim being pinned is that resolving a key is a small
// constant rather than proportional to the table, which is why it goes through
// data.Label and not data.Labels.
func benchmarkHover(b *testing.B, key bool) {
	onOnePGate(b)

	const n = 100_000
	x := make([]float64, n)
	y := make([]float64, n)
	id := make([]string, n)
	for i := range n {
		x[i] = float64(i)
		y[i] = math.Sin(float64(i) / 500)
		id[i] = "row-" + strconv.Itoa(i)
	}
	tbl := refract.NewTable().Float64("x", x).Float64("y", y).String("id", id)

	opts := []geom.Option{geom.X("x"), geom.Y("y")}
	if key {
		opts = append(opts, geom.KeyBy("id"))
	}
	p := refract.New(refract.Size(800, 400))
	p.Add(geom.Scatter(tbl, opts...))
	p.On(refract.Hover, func(refract.Event) {})

	live, err := p.Live(irtest.NullTarget())
	if err != nil {
		b.Fatal(err)
	}
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		b.Fatal(err)
	}
	panel := live.Index().Panels()[0]
	at := ir.Point{X: panel.X.Map(float64(n / 2)), Y: panel.Y.Map(y[n/2])}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		live.Move(float64(at.X), float64(at.Y))
	}
}

func BenchmarkHover(b *testing.B)      { benchmarkHover(b, false) }
func BenchmarkHoverKeyed(b *testing.B) { benchmarkHover(b, true) }
