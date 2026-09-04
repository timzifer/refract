package stat

import "math"

// StackMode is where the bottom of a stack sits.
//
// Stacking itself is a running sum and needs no help. What differs between a
// stacked bar chart, a 100 % chart and a streamgraph is only the *baseline*
// each column of the stack is measured from, which is what this package
// computes: numbers in, numbers out, no scales and no geometry.
type StackMode uint8

// The stack baselines.
const (
	// StackZero puts the bottom of every stack on zero. It is the ordinary
	// stacked bar or area.
	StackZero StackMode = iota

	// StackFill normalises each column to sum to one, so the stack fills the
	// axis and the chart reads as proportions. The baseline is still zero;
	// the caller scales the values.
	StackFill

	// StackSilhouette centres each column on zero, which is the symmetric
	// baseline a ThemeRiver is drawn about.
	StackSilhouette

	// StackWiggle minimises the total slope of the interior boundaries, which
	// is what makes a streamgraph readable: the eye follows a band by its
	// thickness, and a band that is also climbing steeply is hard to follow.
	// Byron & Wattenberg, "Stacked Graphs — Geometry & Aesthetics" (2008).
	StackWiggle
)

// StackOffsets returns the baseline of each column of a stack.
//
// series[g][p] is group g's value at position p; every group must have the
// same number of positions, and the positions must be in axis order, because
// [StackWiggle] reads each column against the one before it. The result has
// one baseline per position: the first group's segment runs from base[p] to
// base[p] + series[0][p], the second from there, and so on.
//
// It is a pure function of its input. Nothing here reaches for a map or for
// math/rand, so a parallel render stays byte-identical to a serial one — see
// docs/adr/0012-parallel-panels.md.
func StackOffsets(mode StackMode, series [][]float64) []float64 {
	return AppendStackOffsets(nil, mode, series)
}

// AppendStackOffsets is [StackOffsets] writing into dst, which it truncates
// and grows as needed. It is the form a geom calls, because a chart redrawn
// every frame should not allocate a baseline per frame.
func AppendStackOffsets(dst []float64, mode StackMode, series [][]float64) []float64 {
	m := 0
	if len(series) > 0 {
		m = len(series[0])
	}
	dst = dst[:0]
	for range m {
		dst = append(dst, 0)
	}
	if m == 0 || len(series) == 0 {
		return dst
	}

	switch mode {
	case StackSilhouette:
		for p := range m {
			dst[p] = -total(series, p) / 2
		}
	case StackWiggle:
		wiggle(dst, series)
	}
	return dst
}

// total sums one column, treating a missing value as zero. A NaN is a row that
// is not there rather than a row of unknown height, which is the same reading
// the drawing traversal takes.
func total(series [][]float64, p int) float64 {
	sum := 0.0
	for _, s := range series {
		if v := s[p]; !math.IsNaN(v) && !math.IsInf(v, 0) {
			sum += v
		}
	}
	return sum
}

func at(series [][]float64, g, p int) float64 {
	v := series[g][p]
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

// wiggle is the Byron–Wattenberg offset.
//
// The baseline moves by the weighted average of how much the bands below each
// group are climbing, weighted by that group's own thickness — so a thick band
// pulls the baseline more than a thin one, and the overall silhouette absorbs
// the slope that would otherwise show up inside the stack.
func wiggle(dst []float64, series [][]float64) {
	n, m := len(series), len(series[0])
	y := 0.0
	for p := 1; p < m; p++ {
		var thickness, weighted float64
		for g := range n {
			// How far this group's own lower boundary has moved since the last
			// position: half of its own change, plus all of the change below it.
			d := (at(series, g, p) - at(series, g, p-1)) / 2
			for k := range g {
				d += at(series, k, p) - at(series, k, p-1)
			}
			thickness += at(series, g, p)
			weighted += d * at(series, g, p)
		}
		dst[p-1] = y
		if thickness != 0 {
			y -= weighted / thickness
		}
	}
	dst[m-1] = y
}
