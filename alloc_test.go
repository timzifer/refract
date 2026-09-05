package refract_test

import (
	"math"
	"runtime"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

// signal is a plot of one line over n rows, ready to render repeatedly.
func signal(n int) *refract.Plot {
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = float64(i)
		y[i] = math.Sin(float64(i)/50) + 0.2*math.Sin(float64(i)/3)
	}
	src := refract.Float64Columns(map[string][]float64{"x": x, "y": y})
	p := refract.New(refract.Size(800, 500), refract.Title("Signal"))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	return p
}

func colouredCloud(n int) *refract.Plot {
	x := make([]float64, n)
	y := make([]float64, n)
	v := make([]float64, n)
	for i := range n {
		x[i] = float64(i%997) / 997
		y[i] = math.Sin(float64(i) / 31)
		// A handful of distinct colours, which is what a real ramp over real
		// data quantises to anyway.
		v[i] = float64(i % 16)
	}
	src := refract.Float64Columns(map[string][]float64{"x": x, "y": y, "v": v})
	p := refract.New(refract.Size(800, 500))
	p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y"),
		geom.ColorBy("v", scale.Sequential(palette.Viridis))))
	return p
}

// stackedSeries is a long table of n rows split into four series and stacked,
// which is the v0.7 data path: the groups are indexed and the adjustment is
// derived on every Train, and neither may cost anything per row.
//
// The series column is text, which is what a series column is. A numeric one
// is formatted into a label per row, and that is a cost of naming a category
// with a number rather than of stacking.
func stackedSeries(n int) *refract.Plot {
	names := [...]string{"north", "south", "east", "west"}
	var x, y []float64
	var series []string
	for i := range n {
		x = append(x, float64(i/len(names)))
		y = append(y, 1+math.Sin(float64(i)/97))
		series = append(series, names[i%len(names)])
	}
	src := refract.NewTable().Float64("x", x).Float64("y", y).String("s", series)
	p := refract.New(refract.Size(800, 500))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Area(src, geom.X("x"), geom.Y("y"), geom.GroupBy("s")))
	return p
}

// polarSignal is [signal] drawn in a polar coord: the fourth data path, and
// the one that puts an interface call between a mapped pair and a device
// point.
//
// A polar coord does not decimate — see docs/adr/0018 — so this draws every
// row, which is exactly why it is worth measuring: if the per-point call ever
// stops going through the batch form, this is where it shows.
func polarSignal(n int) *refract.Plot {
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = float64(i)
		y[i] = 2 + math.Sin(float64(i)/50)
	}
	src := refract.Float64Columns(map[string][]float64{"x": x, "y": y})
	p := refract.New(refract.Size(800, 500), refract.Coord(coord.Polar(coord.Chord())))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	return p
}

// brokenRing is the v0.8 sugar's data path: a donut whose slices name their
// own inner and outer radius and are broken out of the ring per row.
//
// It is the fifth thing worth measuring, because it is the one that collects
// something per mark that is not a mark — a displacement — and carries it
// through the batching. The buffer for it comes from the scratch pool like
// every other, and this is what says so.
func brokenRing(n int) *refract.Plot {
	names := [...]string{"north", "south", "east", "west"}
	var share, floor, reach, pull []float64
	var series []string
	for i := range n {
		share = append(share, 1+math.Sin(float64(i)/97))
		floor = append(floor, 0.4)
		reach = append(reach, 0.6+0.4*math.Abs(math.Sin(float64(i)/31)))
		pull = append(pull, 0.05*float64(i%3))
		series = append(series, names[i%len(names)])
	}
	src := refract.NewTable().
		Float64("share", share).Float64("floor", floor).
		Float64("reach", reach).Float64("pull", pull).
		String("s", series)
	p := refract.New(refract.Size(800, 500), refract.Coord(coord.Donut(0.3)))
	p.X(scale.Linear(scale.Domain(0, 1)))
	p.Y(scale.Linear())
	p.Add(geom.Bar(src, geom.X("floor"), geom.X2("reach"), geom.Y("share"),
		geom.GroupBy("s"), geom.ExplodeBy("pull")))
	return p
}

func facetedSignal(panels, rows int) *refract.Plot { return facetedSignalWith(panels, rows, true) }

func facetedSignalWith(panels, rows int, parallel bool) *refract.Plot {
	var group []string
	var x, y []float64
	for g := range panels {
		for i := range rows {
			group = append(group, string(rune('a'+g)))
			x = append(x, float64(i))
			y = append(y, math.Sin(float64(i)/50+float64(g)))
		}
	}
	src := refract.NewTable().String("g", group).Float64("x", x).Float64("y", y)
	p := refract.New(refract.Size(900, 600), refract.Parallel(parallel))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	p.Facet(facet.Wrap("g", facet.Columns(3)))
	return p
}

// onOnePGate pins a benchmark's measurement to a single processor, and is what
// makes an allocation count reproducible rather than merely typical.
//
// The noise it removes is sync.Pool's, and it is worth writing down because it
// is invisible in a profile. A pool keeps a private slot per P. A render Gets
// its scratch and Puts it back on the same goroutine — but a frame here takes
// milliseconds, the scheduler preempts asynchronously every ten of them, and a
// goroutine that comes back on a different P finds its own scratch stranded in
// the old P's private slot, which nothing can steal from. The frame then
// refills every buffer it needs: about sixty allocations that have nothing to
// do with the data, landing in one iteration out of a few dozen. Measured over
// ten iterations that is the difference between 54 and 68 allocs/op for the
// same code, which is exactly the flake that makes a gate ignorable.
//
// One P has one private slot and nothing to migrate to, so the count is the
// same on every run. This is not a thumb on the scale: a pool miss is a real
// cost that a real chart pays occasionally, and it is simply not the cost this
// gate measures — the gate asks whether a frame allocates *per row*.
// testing.AllocsPerRun does the same thing for the test half of the gate, which
// is why the tests were steady all along while the benchmarks were not.
//
// The parallel benchmarks below are deliberately not pinned: what they measure
// is panels drawn on several goroutines, and nothing gates their counts.
func onOnePGate(b *testing.B) {
	prev := runtime.GOMAXPROCS(1)
	b.Cleanup(func() { runtime.GOMAXPROCS(prev) })
}

func benchmarkFrame(b *testing.B, rows int) {
	onOnePGate(b)
	p := signal(rows)
	target := irtest.NullTarget()
	if err := p.Render(target); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := p.Render(target); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkStacked(b *testing.B, rows int) {
	onOnePGate(b)
	p := stackedSeries(rows)
	target := irtest.NullTarget()
	if err := p.Render(target); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := p.Render(target); err != nil {
			b.Fatal(err)
		}
	}
}

// The stacked path measured the same way as the plain one. The adjustment is
// derived on every Train — the axis has to describe the totals — so this is
// where a buffer that escaped the layer's own memory would show up.
func BenchmarkStacked1k(b *testing.B)   { benchmarkStacked(b, 1_000) }
func BenchmarkStacked100k(b *testing.B) { benchmarkStacked(b, 100_000) }

func benchmarkPolar(b *testing.B, rows int) {
	onOnePGate(b)
	p := polarSignal(rows)
	target := irtest.NullTarget()
	if err := p.Render(target); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := p.Render(target); err != nil {
			b.Fatal(err)
		}
	}
}

// The polar path measured the same way. Every row goes through
// coord.Coord.Points, and the batch form is the reason that costs nothing.
func BenchmarkPolar1k(b *testing.B)   { benchmarkPolar(b, 1_000) }
func BenchmarkPolar100k(b *testing.B) { benchmarkPolar(b, 100_000) }

func benchmarkBrokenRing(b *testing.B, rows int) {
	onOnePGate(b)
	p := brokenRing(rows)
	target := irtest.NullTarget()
	if err := p.Render(target); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := p.Render(target); err != nil {
			b.Fatal(err)
		}
	}
}

// The v0.8 sugar's path: a slice's radii come from two columns and its
// break-out from a third, and the displacement of every mark is collected and
// carried through the batching out of pooled memory.
func BenchmarkBrokenRing1k(b *testing.B)   { benchmarkBrokenRing(b, 1_000) }
func BenchmarkBrokenRing100k(b *testing.B) { benchmarkBrokenRing(b, 100_000) }

// bubbleCloud is the v0.9 data path: a scatter whose size comes from a column,
// so every mark is a circle collected into a path and the marks are ordered
// largest first before they are emitted.
//
// It is worth measuring because it is the one path that *sorts* per frame. The
// sort is over the scratch's own index list and in place, so a hundred times
// the rows must still cost what a thousand cost.
func bubbleCloud(n int) *refract.Plot {
	x := make([]float64, n)
	y := make([]float64, n)
	size := make([]float64, n)
	for i := range n {
		x[i] = float64(i%997) / 997
		y[i] = math.Sin(float64(i) / 31)
		size[i] = float64(i%64) + 1
	}
	src := refract.Float64Columns(map[string][]float64{"x": x, "y": y, "n": size})
	p := refract.New(refract.Size(800, 500))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y"),
		geom.SizeBy("n", scale.Size())))
	return p
}

func benchmarkBubbles(b *testing.B, rows int) {
	onOnePGate(b)
	p := bubbleCloud(rows)
	target := irtest.NullTarget()
	if err := p.Render(target); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := p.Render(target); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBubbles1k(b *testing.B)   { benchmarkBubbles(b, 1_000) }
func BenchmarkBubbles100k(b *testing.B) { benchmarkBubbles(b, 100_000) }

func BenchmarkFrame1k(b *testing.B)   { benchmarkFrame(b, 1_000) }
func BenchmarkFrame100k(b *testing.B) { benchmarkFrame(b, 100_000) }
func BenchmarkFrame1M(b *testing.B)   { benchmarkFrame(b, 1_000_000) }

func benchmarkFacet(b *testing.B, parallel bool) {
	p := facetedSignalWith(9, 50_000, parallel)
	target := irtest.NullTarget()
	if err := p.Render(target); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := p.Render(target); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFacetParallel(b *testing.B) { benchmarkFacet(b, true) }
func BenchmarkFacetSerial(b *testing.B)   { benchmarkFacet(b, false) }
