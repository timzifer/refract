package stat_test

import (
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/timzifer/refract/stat"
)

// The v0.9 distribution stats. Every one of them is a pure function of its
// arguments, and every one of them therefore gets the same two tests: it says
// the right thing about a hand-checkable input, and it says the same thing
// twice. The second half is not ceremony — ADR 0012 requires a parallel render
// to be byte identical to a serial one, and a reduction that reached for a map
// iteration order or math/rand would end that silently.

func TestBinCountsEveryObservationOnce(t *testing.T) {
	vs := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	got := stat.Bin(vs, 0, 10, 5)
	if len(got) != 5 {
		t.Fatalf("got %d buckets, want 5", len(got))
	}
	total := 0
	for _, b := range got {
		total += b.Count
	}
	if total != len(vs) {
		t.Errorf("the buckets hold %d of %d observations", total, len(vs))
	}
	// Two per bucket, except the last, which is closed on the right and so
	// holds 8, 9 and 10.
	want := []int{2, 2, 2, 2, 3}
	for i, b := range got {
		if b.Count != want[i] {
			t.Errorf("bucket %d [%v, %v) holds %d, want %d", i, b.Lo, b.Hi, b.Count, want[i])
		}
	}
	if got[4].Hi != 10 {
		t.Errorf("the last bucket ends at %v, want exactly 10", got[4].Hi)
	}
}

func TestBinLeavesOutWhatIsOutsideItsInterval(t *testing.T) {
	vs := []float64{-5, 0.5, 1.5, 9, math.NaN(), math.Inf(1)}
	got := stat.Bin(vs, 0, 2, 2)
	total := 0
	for _, b := range got {
		total += b.Count
	}
	if total != 2 {
		t.Errorf("counted %d observations in [0, 2], want 2 — a histogram over a named range is a statement about that range", total)
	}
}

func TestBinOverOneRepeatedValueStillDrawsSomething(t *testing.T) {
	got := stat.Bin([]float64{3, 3, 3}, 0, 0, 0)
	if len(got) != 1 || got[0].Count != 3 {
		t.Fatalf("got %v, want one bucket holding three", got)
	}
	if !(got[0].Lo < 3 && got[0].Hi > 3) {
		t.Errorf("the bucket [%v, %v) does not contain the value it counted", got[0].Lo, got[0].Hi)
	}
}

func TestSturgesIsTheDefaultBinCount(t *testing.T) {
	vs := make([]float64, 100)
	for i := range vs {
		vs[i] = float64(i)
	}
	if got, want := len(stat.Bin(vs, 0, 0, 0)), stat.Sturges(100); got != want {
		t.Errorf("an unspecified bin count gave %d buckets, want Sturges's %d", got, want)
	}
	if got := stat.Sturges(100); got != 8 {
		t.Errorf("Sturges(100) = %d, want ceil(log2 100)+1 = 8", got)
	}
}

func TestFreedmanDiaconisDeclinesWhenItHasNoAnswer(t *testing.T) {
	if got := stat.FreedmanDiaconis([]float64{1, 1, 1, 1}); got != 0 {
		t.Errorf("got %d bins for a column with no spread, want 0 so the caller can fall back", got)
	}
	vs := make([]float64, 1000)
	for i := range vs {
		vs[i] = float64(i)
	}
	if got := stat.FreedmanDiaconis(vs); got < 5 || got > 200 {
		t.Errorf("got %d bins for a thousand-row ramp, which is not a plausible answer", got)
	}
}

func TestABinnedColumnBinsTheSameWayTwice(t *testing.T) {
	vs := scatterOf(500, 7)
	a := stat.Bin(vs, 0, 0, 0)
	b := stat.Bin(vs, 0, 0, 0)
	if !reflect.DeepEqual(a, b) {
		t.Error("two runs of Bin over one column disagree")
	}
}

func TestAppendBinReusesTheCallersSlice(t *testing.T) {
	vs := scatterOf(200, 3)
	buf := stat.AppendBin(nil, vs, 0, 0, 8)
	before := &buf[0]
	buf = stat.AppendBin(buf, vs, 0, 0, 8)
	if &buf[0] != before {
		t.Error("AppendBin allocated a new array rather than reusing the one it was given")
	}
}

func TestADensityIntegratesToOne(t *testing.T) {
	vs := scatterOf(400, 11)
	lo, hi := -6.0, 6.0
	pts := stat.KDE(vs, 0.5, lo, hi, 601)
	// The trapezium rule over an interval that covers the whole sample plus
	// several bandwidths either side.
	step := (hi - lo) / 600
	area := 0.0
	for i := 1; i < len(pts); i++ {
		area += (pts[i].Y + pts[i-1].Y) / 2 * step
	}
	if math.Abs(area-1) > 0.01 {
		t.Errorf("the density integrates to %v, want 1", area)
	}
}

func TestADensityIsSymmetricAboutASymmetricSample(t *testing.T) {
	pts := stat.KDE([]float64{-1, 1}, 0.5, -3, 3, 61)
	for i := range pts {
		j := len(pts) - 1 - i
		if math.Abs(pts[i].Y-pts[j].Y) > 1e-12 {
			t.Fatalf("density at %v is %v and at %v is %v; a symmetric sample has a symmetric estimate",
				pts[i].X, pts[i].Y, pts[j].X, pts[j].Y)
		}
	}
}

func TestADensityEstimatesTheSameCurveTwice(t *testing.T) {
	vs := scatterOf(300, 5)
	a := stat.KDE(vs, 0, 0, 0, 64)
	b := stat.KDE(vs, 0, 0, 0, 64)
	if !reflect.DeepEqual(a, b) {
		t.Error("two runs of KDE over one column disagree")
	}
}

func TestSilvermanTakesTheSmallerSpread(t *testing.T) {
	// A wide standard deviation with a narrow interquartile range is what one
	// outlier does to a column; the rule has to follow the IQR.
	wide := stat.Silverman(100, 0, 50)
	robust := stat.Silverman(100, 1, 50)
	if !(robust < wide) {
		t.Errorf("bandwidth with an IQR of 1 is %v and without one %v; the robust form must be the smaller", robust, wide)
	}
	if stat.Silverman(0, 0, 50) != 0 {
		t.Error("a column with no spread has no bandwidth, and the caller has to see that")
	}
}

func TestAnECDFRisesToOneInStepsOfOneOverN(t *testing.T) {
	got := stat.ECDF([]float64{1, 2, 2, 3})
	want := []stat.Point{{X: 1, Y: 0.25}, {X: 2, Y: 0.75}, {X: 3, Y: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v — one point per distinct value, at its cumulative fraction", got, want)
	}
}

func TestAnECDFIgnoresWhatIsNotANumber(t *testing.T) {
	got := stat.ECDF([]float64{1, 2, math.NaN()})
	if len(got) != 2 || got[len(got)-1].Y != 1 {
		t.Errorf("got %v; the fractions are taken against the usable rows", got)
	}
}

func TestLoessFollowsALineExactly(t *testing.T) {
	// A locally linear fit through data that is already a line is that line,
	// wherever the window happens to fall.
	xs, ys := make([]float64, 50), make([]float64, 50)
	for i := range xs {
		xs[i] = float64(i)
		ys[i] = 3*float64(i) + 7
	}
	for _, p := range stat.Loess(xs, ys, 0.4, 25) {
		if want := 3*p.X + 7; math.Abs(p.Y-want) > 1e-6 {
			t.Fatalf("fit at %v is %v, want %v", p.X, p.Y, want)
		}
	}
}

func TestLoessSmoothsWhatABareLineWouldNot(t *testing.T) {
	// A hump. A straight fit would report a slope of zero everywhere; loess has
	// to bend, and it has to be highest in the middle.
	n := 61
	xs, ys := make([]float64, n), make([]float64, n)
	for i := range xs {
		xs[i] = float64(i) / float64(n-1)
		ys[i] = math.Sin(math.Pi * xs[i])
	}
	pts := stat.Loess(xs, ys, 0.3, 41)
	peak := 0
	for i, p := range pts {
		if p.Y > pts[peak].Y {
			peak = i
		}
	}
	if math.Abs(pts[peak].X-0.5) > 0.1 {
		t.Errorf("the fit peaks at %v, want the middle", pts[peak].X)
	}
	if pts[peak].Y < 0.8 {
		t.Errorf("the fit peaks at %v, want it to reach most of the way to 1", pts[peak].Y)
	}
}

func TestLoessFitsTheSameCurveTwice(t *testing.T) {
	xs, ys := make([]float64, 200), make([]float64, 200)
	for i := range xs {
		xs[i] = float64(i)
		ys[i] = math.Sin(float64(i)/13) * 4
	}
	a := stat.Loess(xs, ys, 0, 0)
	b := stat.Loess(xs, ys, 0, 0)
	if !reflect.DeepEqual(a, b) {
		t.Error("two runs of Loess over one pair of columns disagree")
	}
}

func TestAHexLatticeCountsEveryPointOnce(t *testing.T) {
	var h stat.Hex
	h.Reset(4, 0, 0, 40, 40)
	xs, ys := scatterOf(300, 2), scatterOf(300, 9)
	for i := range xs {
		xs[i], ys[i] = 20+xs[i]*3, 20+ys[i]*3
	}
	stat.BinHex(&h, xs, ys)
	var total uint32
	for _, c := range h.Counts {
		total += c
	}
	if int(total) != h.N {
		t.Errorf("the cells hold %d rows and N says %d", total, h.N)
	}
	if h.N == 0 {
		t.Fatal("nothing was binned")
	}
}

func TestEveryHexPointLandsInItsNearestCell(t *testing.T) {
	var h stat.Hex
	h.Reset(5, 0, 0, 60, 60)
	// A hexagonal lattice's defining property: the cell a point is assigned to
	// is the one whose centre is nearest. If that ever stops holding, the
	// picture grows seams.
	for i := range 40 {
		for j := range 40 {
			x, y := float64(i)*1.5, float64(j)*1.5
			col, row, ok := h.Cell(x, y)
			if !ok {
				t.Fatalf("(%v, %v) is inside the rectangle and was not placed", x, y)
			}
			cx, cy := h.Center(col, row)
			best := math.Hypot(x-cx, y-cy)
			for dr := -2; dr <= 2; dr++ {
				for dc := -2; dc <= 2; dc++ {
					ox, oy := h.Center(col+dc, row+dr)
					if d := math.Hypot(x-ox, y-oy); d < best-1e-9 {
						t.Fatalf("(%v, %v) went to the cell at (%v, %v), %v away, but (%v, %v) is %v away",
							x, y, cx, cy, best, ox, oy, d)
					}
				}
			}
		}
	}
}

func TestHexCellsComeOutInAFixedOrder(t *testing.T) {
	var h stat.Hex
	h.Reset(6, 0, 0, 50, 50)
	xs, ys := scatterOf(200, 4), scatterOf(200, 6)
	for i := range xs {
		xs[i], ys[i] = 25+xs[i]*5, 25+ys[i]*5
	}
	stat.BinHex(&h, xs, ys)
	a := h.Cells(nil)
	b := h.Cells(nil)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("two listings of one lattice disagree")
	}
	if !slices.IsSortedFunc(a, func(p, q stat.Cell) int {
		if p.Row != q.Row {
			return p.Row - q.Row
		}
		return p.Col - q.Col
	}) {
		t.Error("the cells are not in row-major order, so a parallel render could differ from a serial one")
	}
}

func TestHexVerticesTileWithoutGaps(t *testing.T) {
	// Two cells one row apart share an edge, so the lower vertex of the upper
	// one coincides with a vertex of the lower one.
	var h stat.Hex
	h.Reset(10, 0, 0, 100, 100)
	x0, y0 := h.Center(2, 2)
	x1, y1 := h.Center(2, 3)
	a := stat.Vertices(nil, x0, y0, h.Radius)
	b := stat.Vertices(nil, x1, y1, h.Radius)
	near := func(p, q stat.Point) bool { return math.Hypot(p.X-q.X, p.Y-q.Y) < 1e-9 }
	shared := 0
	for _, p := range a {
		for _, q := range b {
			if near(p, q) {
				shared++
			}
		}
	}
	if shared != 2 {
		t.Errorf("the two cells share %d vertices, want the 2 that make an edge", shared)
	}
}

// ramp is a deterministic pseudo-sample: a fixed sequence that looks scattered
// and is the same on every machine and every run, which is what a determinism
// test needs and what math/rand would take away.
func scatterOf(n, seed int) []float64 {
	out := make([]float64, n)
	v := float64(seed)
	for i := range out {
		v = math.Mod(v*1103515245+12345, 2147483648)
		out[i] = v/2147483648*4 - 2
	}
	return out
}
