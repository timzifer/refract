package stat_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/timzifer/refract/stat"
)

func TestQQKeepsTiesAndUsesInteriorRanks(t *testing.T) {
	input := []float64{math.NaN(), math.Inf(-1), 2, 2, 7, math.Inf(1)}
	got := stat.QQ(input, func(p float64) float64 { return p })
	want := []stat.Point{{X: 1.0 / 6, Y: 2}, {X: 0.5, Y: 2}, {X: 5.0 / 6, Y: 7}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QQ = %v, want %v", got, want)
	}
	if !math.IsNaN(input[0]) || input[2] != 2 || input[4] != 7 {
		t.Fatal("edited input")
	}
	if !reflect.DeepEqual(got, stat.QQ(input, func(p float64) float64 { return p })) {
		t.Fatal("nondeterministic")
	}
}

func TestNormalQQHasKnownQuantilesAndHandlesEmptySamples(t *testing.T) {
	got := stat.QQ([]float64{1, 3}, nil)
	if len(got) != 2 || math.Abs(got[0].X+0.6744897501960817) > 1e-12 || math.Abs(got[1].X-0.6744897501960817) > 1e-12 {
		t.Fatalf("normal QQ: %v", got)
	}
	if one := stat.QQ([]float64{42}, nil); len(one) != 1 || one[0].X != 0 || one[0].Y != 42 {
		t.Fatalf("singleton: %v", one)
	}
	if len(stat.QQ([]float64{math.NaN(), math.Inf(1)}, nil)) != 0 {
		t.Fatal("nonfinite observations survived")
	}
	if len(stat.AppendQQ(got, nil, nil)) != 0 {
		t.Fatal("empty input did not truncate destination")
	}
	if len(stat.QQ([]float64{1}, func(float64) float64 { return math.Inf(1) })) != 0 {
		t.Fatal("nonfinite theoretical quantile survived")
	}
}

func BenchmarkQQ(b *testing.B) {
	sample := make([]float64, 1000)
	for i := range sample {
		sample[i] = float64(i)
	}
	dst := make([]stat.Point, 0, len(sample))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		dst = stat.AppendQQ(dst, sample, nil)
	}
}
