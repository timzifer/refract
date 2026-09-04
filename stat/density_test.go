package stat_test

import (
	"image/color"
	"math"
	"testing"

	"github.com/timzifer/refract/stat"
)

func TestGridCountsEveryPointOnce(t *testing.T) {
	var g stat.Grid
	g.Reset(4, 2, 0, 0, 8, 4)
	xs := []float64{0, 1, 2, 3, 4, 5, 6, 7}
	ys := []float64{0, 0, 0, 0, 3, 3, 3, 3}
	stat.Bin(&g, xs, ys)
	if g.N != 8 {
		t.Fatalf("binned %d of 8 points", g.N)
	}
	var total uint32
	for _, c := range g.Counts {
		total += c
	}
	if total != 8 {
		t.Errorf("counts sum to %d, want 8", total)
	}
	if g.At(0, 0) != 2 || g.At(2, 1) != 2 {
		t.Errorf("cell (0,0) holds %d and cell (2,1) %d, want two each", g.At(0, 0), g.At(2, 1))
	}
	if g.Max != 2 {
		t.Errorf("Max is %d, want 2", g.Max)
	}
}

func TestGridDropsPointsOutsideItsRectangle(t *testing.T) {
	var g stat.Grid
	g.Reset(10, 10, 0, 0, 1, 1)
	for _, p := range [][2]float64{{-0.1, 0.5}, {1.1, 0.5}, {0.5, -0.1}, {0.5, 1.1}, {math.NaN(), 0.5}, {0.5, math.Inf(1)}} {
		if g.Add(p[0], p[1]) {
			t.Errorf("(%v, %v) was counted, want it dropped", p[0], p[1])
		}
	}
	if g.N != 0 {
		t.Errorf("N is %d, want 0", g.N)
	}
}

// The far edge belongs to the last cell. Without that, the single point at the
// domain maximum — the one an axis is scaled to reach — falls out of the grid.
func TestGridPutsTheFarEdgeInTheLastCell(t *testing.T) {
	var g stat.Grid
	g.Reset(4, 4, 0, 0, 1, 1)
	if !g.Add(1, 1) {
		t.Fatal("the far corner was dropped")
	}
	if g.At(3, 3) != 1 {
		t.Errorf("the far corner landed somewhere other than the last cell")
	}
}

func TestGridHandlesAnInvertedAxis(t *testing.T) {
	// A device Y axis can run either way round; a grid over one has to bin the
	// same way round as the axis.
	var up, down stat.Grid
	up.Reset(1, 4, 0, 0, 1, 4)
	down.Reset(1, 4, 0, 4, 1, 0)
	up.Add(0.5, 0.5)
	down.Add(0.5, 0.5)
	if up.At(0, 0) != 1 {
		t.Error("on an ascending axis 0.5 belongs in the first row")
	}
	if down.At(0, 3) != 1 {
		t.Error("on a descending axis 0.5 belongs in the last row")
	}
}

func TestResetReusesTheBuffer(t *testing.T) {
	var g stat.Grid
	g.Reset(20, 20, 0, 0, 1, 1)
	g.Add(0.5, 0.5)
	first := &g.Counts[0]
	g.Reset(10, 10, 0, 0, 1, 1)
	if &g.Counts[0] != first {
		t.Error("Reset allocated a new buffer for a smaller grid")
	}
	for _, c := range g.Counts {
		if c != 0 {
			t.Fatal("Reset left old counts behind")
		}
	}
	if g.Max != 0 || g.N != 0 {
		t.Errorf("Reset left Max=%d N=%d behind", g.Max, g.N)
	}
}

func TestFractionScalings(t *testing.T) {
	var g stat.Grid
	g.Reset(2, 1, 0, 0, 2, 1)
	for range 100 {
		g.Add(0.5, 0.5)
	}
	g.Add(1.5, 0.5)

	if got := g.Fraction(0, stat.Log); got != 0 {
		t.Errorf("an empty cell has fraction %v, want 0", got)
	}
	if got := g.Fraction(100, stat.Log); got != 1 {
		t.Errorf("the busiest cell has fraction %v, want 1", got)
	}
	lin := g.Fraction(1, stat.Linear)
	log := g.Fraction(1, stat.Log)
	sqrt := g.Fraction(1, stat.Sqrt)
	// The point of a log scaling is that a lone row is still visible against a
	// cell holding a hundred.
	if !(lin < sqrt && sqrt < log) {
		t.Errorf("linear %v, sqrt %v, log %v: want each to lift a lone row further than the last", lin, sqrt, log)
	}
}

func TestRasterPaintsOnlyOccupiedCells(t *testing.T) {
	var g stat.Grid
	g.Reset(3, 2, 0, 0, 3, 2)
	g.Add(0.5, 0.5)
	g.Add(2.5, 1.5)

	img := g.Raster(nil, stat.Log, func(t float64) color.NRGBA {
		return color.NRGBA{R: 10, G: 20, B: 30, A: uint8(255 * t)}
	})
	if b := img.Bounds(); b.Dx() != 3 || b.Dy() != 2 {
		t.Fatalf("image is %v, want one pixel per cell", b)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a == 0 {
		t.Error("an occupied cell was left transparent")
	}
	if _, _, _, a := img.At(1, 0).RGBA(); a != 0 {
		t.Error("an empty cell was painted; the plot background must read through")
	}
	if _, _, _, a := img.At(2, 1).RGBA(); a == 0 {
		t.Error("the second occupied cell was left transparent")
	}
}

func TestRasterReusesTheImage(t *testing.T) {
	var g stat.Grid
	g.Reset(8, 8, 0, 0, 1, 1)
	g.Add(0.1, 0.1)
	paint := func(float64) color.NRGBA { return color.NRGBA{A: 255} }
	first := g.Raster(nil, stat.Log, paint)

	g.Reset(4, 4, 0, 0, 1, 1)
	g.Add(0.9, 0.9)
	second := g.Raster(first, stat.Log, paint)
	if &second.Pix[0] != &first.Pix[0] {
		t.Error("Raster allocated a new image for a smaller grid")
	}
	if b := second.Bounds(); b.Dx() != 4 || b.Dy() != 4 {
		t.Fatalf("reused image reports bounds %v, want 4x4", b)
	}
	if _, _, _, a := second.At(0, 0).RGBA(); a != 0 {
		t.Error("Raster left the previous frame's pixels behind")
	}
}

func BenchmarkBin(b *testing.B) {
	n := 1_000_000
	xs := make([]float32, n)
	ys := make([]float32, n)
	for i := range n {
		xs[i] = float32(i % 800)
		ys[i] = float32((i * 7) % 500)
	}
	var g stat.Grid
	b.ReportAllocs()
	for b.Loop() {
		g.Reset(800, 500, 0, 0, 800, 500)
		stat.Bin(&g, xs, ys)
	}
}
