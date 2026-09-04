package stat_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/stat"
)

// stream is three bands that rise and fall at different times, which is the
// shape every offset here is defined against.
func stream() [][]float64 {
	return [][]float64{
		{1, 2, 4, 8, 4, 2, 1},
		{4, 4, 3, 2, 3, 4, 4},
		{0, 1, 3, 6, 9, 6, 2},
	}
}

func TestStackingFromZeroLeavesTheBaselineAlone(t *testing.T) {
	base := stat.StackOffsets(stat.StackZero, stream())
	for p, v := range base {
		if v != 0 {
			t.Fatalf("baseline at %d is %v, want a stack that sits on zero", p, v)
		}
	}
}

func TestASilhouetteCentresEveryColumn(t *testing.T) {
	s := stream()
	base := stat.StackOffsets(stat.StackSilhouette, s)
	for p := range base {
		total := 0.0
		for _, g := range s {
			total += g[p]
		}
		// The column runs from base to base+total, so a centred one has as
		// much below the axis as above it.
		if lo, hi := base[p], base[p]+total; math.Abs(lo+hi) > 1e-12 {
			t.Fatalf("column %d runs %v..%v, which is not centred on zero", p, lo, hi)
		}
	}
}

// The wiggle exists to make the interior boundaries flatter than stacking from
// zero leaves them. That is a measurable claim, so it is measured rather than
// asserted against a table of magic numbers.
func TestTheWiggleFlattensTheInteriorBoundaries(t *testing.T) {
	s := stream()
	zero := slope(s, stat.StackOffsets(stat.StackZero, s))
	wiggle := slope(s, stat.StackOffsets(stat.StackWiggle, s))
	if wiggle >= zero {
		t.Fatalf("the wiggle's interior slope is %v against %v for a stack on zero: "+
			"the offset is not doing the one thing it is for", wiggle, zero)
	}
}

// slope is the total absolute change of every interior boundary of a stack.
func slope(series [][]float64, base []float64) float64 {
	m := len(base)
	total := 0.0
	edge := make([]float64, m)
	copy(edge, base)
	for _, g := range series[:len(series)-1] {
		for p := range m {
			edge[p] += g[p]
		}
		for p := 1; p < m; p++ {
			total += math.Abs(edge[p] - edge[p-1])
		}
	}
	return total
}

// The determinism test every reduction in this package carries. A parallel
// render must be byte-identical to a serial one, which it cannot be if a
// reduction reaches for a map or for math/rand — see ADR 0012.
func TestStackOffsetsAreAPureFunctionOfTheirInput(t *testing.T) {
	for _, mode := range []stat.StackMode{
		stat.StackZero, stat.StackFill, stat.StackSilhouette, stat.StackWiggle,
	} {
		first := stat.StackOffsets(mode, stream())
		for range 8 {
			again := stat.StackOffsets(mode, stream())
			for p := range first {
				if first[p] != again[p] {
					t.Fatalf("mode %d moved at %d: %v then %v", mode, p, first[p], again[p])
				}
			}
		}
	}
}

func TestAppendStackOffsetsReusesItsBuffer(t *testing.T) {
	s := stream()
	buf := make([]float64, 0, len(s[0]))
	got := stat.AppendStackOffsets(buf, stat.StackSilhouette, s)
	if &got[0] != &buf[:1][0] {
		t.Error("AppendStackOffsets allocated instead of filling the buffer it was given")
	}
	if len(got) != len(s[0]) {
		t.Fatalf("got %d offsets for %d positions", len(got), len(s[0]))
	}
}

// A missing value is a row that is not there rather than a row of unknown
// height. Both the offset and the drawing traversal have to read it that way
// or every segment above it shifts.
func TestAHoleCountsAsNothing(t *testing.T) {
	with := [][]float64{{1, math.NaN(), 3}, {2, 2, 2}}
	without := [][]float64{{1, 0, 3}, {2, 2, 2}}
	a := stat.StackOffsets(stat.StackSilhouette, with)
	b := stat.StackOffsets(stat.StackSilhouette, without)
	for p := range a {
		if a[p] != b[p] {
			t.Fatalf("a NaN at %d moved the baseline: %v against %v", p, a[p], b[p])
		}
	}
}
