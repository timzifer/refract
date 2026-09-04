package refract_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract"
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
