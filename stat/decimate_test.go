package stat_test

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/timzifer/refract/stat"
)

func ramp(n int) (x, y []float64) {
	x, y = make([]float64, n), make([]float64, n)
	for i := range n {
		x[i] = float64(i)
		y[i] = math.Sin(float64(i) / 50)
	}
	return x, y
}

func TestLTTBKeepsTheBudgetAndTheEnds(t *testing.T) {
	x, y := ramp(10000)
	for _, budget := range []int{3, 10, 100, 999} {
		got := stat.LTTB(x, y, budget)
		if len(got) != budget {
			t.Errorf("budget %d: kept %d rows, want exactly the budget", budget, len(got))
		}
		if got[0] != 0 || got[len(got)-1] != len(x)-1 {
			t.Errorf("budget %d: kept %d..%d, want the first and last row", budget, got[0], got[len(got)-1])
		}
		if !slices.IsSorted(got) {
			t.Errorf("budget %d: rows are out of order: %v", budget, got)
		}
	}
}

func TestLTTBKeepsEverythingBelowTheBudget(t *testing.T) {
	x, y := ramp(50)
	got := stat.LTTB(x, y, 200)
	if len(got) != 50 {
		t.Fatalf("kept %d of 50 rows, want all of them", len(got))
	}
	for i, row := range got {
		if row != i {
			t.Fatalf("row %d is %d, want the rows in order", i, row)
		}
	}
}

// A spike is what a reader opens a chart to find, so the reduction that claims
// to preserve shape has to keep it.
func TestLTTBKeepsAnIsolatedSpike(t *testing.T) {
	x, y := ramp(5000)
	const spike = 2317
	y[spike] = 100
	got := stat.LTTB(x, y, 200)
	if !slices.Contains(got, spike) {
		t.Errorf("the spike at row %d was dropped; kept %v...", spike, got[:10])
	}
}

func TestLTTBDegenerateBudgets(t *testing.T) {
	x, y := ramp(100)
	for _, budget := range []int{-1, 0, 1, 2} {
		if got := stat.LTTB(x, y, budget); len(got) != 100 {
			t.Errorf("budget %d: kept %d rows, want all 100 — a budget with no interior cannot choose", budget, len(got))
		}
	}
}

func TestAppendLTTBWritesIntoTheCallersSlice(t *testing.T) {
	x, y := ramp(4000)
	buf := make([]int, 0, 512)
	first := stat.AppendLTTB(buf, x, y, 300)
	second := stat.AppendLTTB(buf[:0], x, y, 300)
	if &first[0] != &second[0] {
		t.Error("AppendLTTB allocated a new array instead of filling the one it was given")
	}
	if !slices.Equal(first, second) {
		t.Error("two runs over the same data disagree")
	}
}

func TestMinMaxKeepsTheExtremesOfEveryColumn(t *testing.T) {
	// Ten rows per column, with the minimum and the maximum planted at known
	// rows inside each.
	const columns, per = 20, 10
	n := columns * per
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = float64(i)
		y[i] = 0
	}
	wantLo, wantHi := map[int]bool{}, map[int]bool{}
	for c := range columns {
		lo, hi := c*per+3, c*per+7
		y[lo], y[hi] = -float64(c+1), float64(c+1)
		wantLo[lo], wantHi[hi] = true, true
	}
	got := stat.MinMax(x, columns, y)
	if !slices.IsSorted(got) {
		t.Fatalf("rows are out of order: %v", got)
	}
	kept := map[int]bool{}
	for _, r := range got {
		kept[r] = true
	}
	for r := range wantLo {
		if !kept[r] {
			t.Errorf("row %d holds a column minimum and was dropped", r)
		}
	}
	for r := range wantHi {
		if !kept[r] {
			t.Errorf("row %d holds a column maximum and was dropped", r)
		}
	}
	if len(got) > 4*columns {
		t.Errorf("kept %d rows over %d columns, want at most four per column", len(got), columns)
	}
}

func TestMinMaxKeepsTheFirstAndLastRow(t *testing.T) {
	x, y := ramp(9000)
	got := stat.MinMax(x, 300, y)
	if got[0] != 0 || got[len(got)-1] != len(x)-1 {
		t.Errorf("kept %d..%d, want the first and last row", got[0], got[len(got)-1])
	}
}

// A band occupies two value columns, and the reduction has to bound both or it
// clips the shape it was asked to preserve.
func TestMinMaxBoundsBothEdgesOfABand(t *testing.T) {
	n := 4000
	x := make([]float64, n)
	lo := make([]float64, n)
	hi := make([]float64, n)
	for i := range n {
		x[i] = float64(i)
		lo[i], hi[i] = -1, 1
	}
	lo[1234], hi[2345] = -50, 50
	got := stat.MinMax(x, 100, lo, hi)
	if !slices.Contains(got, 1234) {
		t.Error("the lower edge's extreme was dropped")
	}
	if !slices.Contains(got, 2345) {
		t.Error("the upper edge's extreme was dropped")
	}
}

func TestMinMaxKeepsEverythingBelowItsOwnCeiling(t *testing.T) {
	x, y := ramp(40)
	if got := stat.MinMax(x, 20, y); len(got) != 40 {
		t.Errorf("kept %d of 40 rows over 20 columns, want all of them", len(got))
	}
}

func TestMinMaxWithoutValueColumns(t *testing.T) {
	x, _ := ramp(100)
	if got := stat.MinMax(x, 10); got != nil {
		t.Errorf("got %v, want nothing: there is no value to take an extreme of", got)
	}
}

func TestMinMaxOnConstantX(t *testing.T) {
	n := 500
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		y[i] = float64(i)
	}
	got := stat.MinMax(x, 50, y)
	if len(got) == 0 || !slices.IsSorted(got) {
		t.Fatalf("got %v, want an ordered non-empty result", got)
	}
	if got[len(got)-1] != n-1 {
		t.Errorf("the largest value, at row %d, was dropped", n-1)
	}
}

func TestDecimationOfFloat32Coordinates(t *testing.T) {
	// Device coordinates are float32; the same functions serve them, which is
	// the whole reason Float has two members.
	n := 5000
	x := make([]float32, n)
	y := make([]float32, n)
	r := rand.New(rand.NewPCG(1, 2))
	for i := range n {
		x[i] = float32(i)
		y[i] = float32(r.NormFloat64())
	}
	if got := stat.LTTB(x, y, 500); len(got) != 500 {
		t.Errorf("LTTB kept %d rows, want 500", len(got))
	}
	if got := stat.MinMax(x, 200, y); len(got) == 0 || !slices.IsSorted(got) {
		t.Errorf("MinMax returned %d rows, want an ordered non-empty result", len(got))
	}
}

func BenchmarkLTTB(b *testing.B) {
	x, y := ramp(1_000_000)
	dst := make([]int, 0, 2048)
	b.ReportAllocs()
	for b.Loop() {
		dst = stat.AppendLTTB(dst[:0], x, y, 1600)
	}
}

func BenchmarkMinMax(b *testing.B) {
	x, y := ramp(1_000_000)
	dst := make([]int, 0, 4096)
	b.ReportAllocs()
	for b.Loop() {
		dst = stat.AppendMinMax(dst[:0], x, 800, y)
	}
}

// The Append forms write after whatever the caller already had, and must not
// let the caller's last value decide which of their own rows to drop.
func TestAppendFormsDoNotReadTheCallersTail(t *testing.T) {
	x, y := ramp(5000)
	seed := []int{9_999_999}

	minmax := stat.AppendMinMax(append([]int(nil), seed...), x, 100, y)
	if got := minmax[1]; got != 0 {
		t.Errorf("MinMax appended %d first, want row 0: it compared against the caller's tail", got)
	}
	lttb := stat.AppendLTTB(append([]int(nil), seed...), x, y, 200)
	if got := lttb[1]; got != 0 {
		t.Errorf("LTTB appended %d first, want row 0", got)
	}
}

// LTTB decides by comparing areas, and a decision is where a last-bit
// difference stops being invisible: two candidates a hair apart swap places and
// a whole vertex moves. That is why the area is computed with each product
// rounded before the subtraction — Go is otherwise free to fuse them, and arm64
// does where amd64 does not.
//
// The fusion itself cannot be observed from a test on one architecture. What
// can be pinned is the other half of the same requirement: that the search is
// decided by the data and not by the order the bucket happens to be walked in.
func TestLTTBBreaksATieTowardsTheEarlierRow(t *testing.T) {
	// A flat line with two candidates that form exactly the same triangle,
	// mirrored about the bucket's centre.
	n := 400
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = float64(i)
	}
	y[100], y[300] = 5, 5

	got := stat.LTTB(x, y, 3)
	// Budget 3 is first, one interior row, last. Both spikes tie; the earlier
	// one has to win, every time, on every machine.
	if len(got) != 3 {
		t.Fatalf("kept %d rows, want 3", len(got))
	}
	if got[1] != 100 {
		t.Errorf("kept row %d, want 100: a tie must go to the earlier row", got[1])
	}
	for range 20 {
		if again := stat.LTTB(x, y, 3); again[1] != got[1] {
			t.Fatalf("two runs over the same data chose %d and %d", got[1], again[1])
		}
	}
}

// The reduction is what a documentation figure is rendered through, so it has
// to be a pure function of its input: same rows in, same rows out, whatever
// else the process has been doing.
func TestDecimationIsAPureFunctionOfItsInput(t *testing.T) {
	x, y := ramp(20_000)
	for i := range y {
		y[i] += 0.001 * float64((i*7919)%13)
	}
	first := stat.LTTB(x, y, 600)
	second := stat.LTTB(append([]float64(nil), x...), append([]float64(nil), y...), 600)
	if !slices.Equal(first, second) {
		t.Error("LTTB chose differently for the same values in different memory")
	}
	a := stat.MinMax(x, 200, y)
	b := stat.MinMax(append([]float64(nil), x...), 200, append([]float64(nil), y...))
	if !slices.Equal(a, b) {
		t.Error("MinMax chose differently for the same values in different memory")
	}
}
