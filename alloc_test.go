package refract_test

import (
	"math"
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

func benchmarkFrame(b *testing.B, rows int) {
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
