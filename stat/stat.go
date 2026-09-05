// Package stat aggregates data before it is drawn.
//
// Two families live here and they answer different questions.
//
// The first is reduction: a column has more rows than the plot has pixels, so
// which rows actually decide what the reader sees? The answers differ by mark.
// A line wants the rows that preserve its shape; a signal envelope wants the
// extremes of every pixel column, because a spike one sample wide is the reason
// someone opened the chart; a point cloud wants no rows at all but a count per
// cell, drawn as an image. See [LTTB], [MinMax] and [Grid].
//
// The second is distribution: the rows are not too many, they are the wrong
// shape. A histogram, a density estimate, an ECDF and a smoothing fit all
// replace the observations with a summary of where they are — which is a
// different reading of the same column rather than a cheaper one. See [Bin],
// [KDE], [ECDF], [Loess] and [Hex].
//
// Neither family changes a scale's domain by itself. A geom decides what its
// axis describes: a decimating layer trains on every row and reduces only when
// it draws, so the axis reports what the data holds rather than what survived;
// a histogram trains on the counts, because the counts are what it draws.
//
// # Purity and determinism
//
// Everything here is a pure function of its arguments. Nothing reads a clock,
// a map iteration order or math/rand: a parallel render has to be byte
// identical to a serial one (docs/adr/0012-parallel-panels.md), and a reduction
// that reached for a random number would quietly end that. Every function has a
// test that runs it twice and compares.
//
// The functions come in pairs: LTTB and AppendLTTB, Bin and AppendBin. The
// Append forms write into a caller-owned slice, which is how a chart redrawn
// every frame keeps its per-frame allocations flat.
package stat

import "math"

// Float is the coordinate type these functions accept.
//
// Both widths are here because both are real: a geom decimates in device
// space, where coordinates are float32 and a pixel is the unit that matters,
// while a caller aggregating before it ever reaches a chart has float64 data.
// Converting one to the other to cross this boundary would cost a copy of the
// whole column.
type Float interface{ ~float32 | ~float64 }

// Point is a position a distribution stat computed: a bin's centre and its
// count, a density's argument and its value, a fit's abscissa and its estimate.
//
// It is float64 and not ir.Point because none of it is a device coordinate —
// stat knows about numbers and nothing else, and a density evaluated in
// float32 would lose the tail of a narrow kernel.
type Point struct{ X, Y float64 }

// finite reports whether v is a number a summary can include. Written as two
// negations so that NaN falls out here rather than in every caller.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Quantile returns the p'th quantile of an ascending slice by linear
// interpolation between the two closest order statistics.
//
// This is the definition R calls type 7 and NumPy uses by default. Choosing it
// deliberately matters: the nine standard definitions disagree by a visible
// amount on the small samples a boxplot or a bandwidth rule is computed from,
// and a box that does not match the reader's own analysis is worse than no box.
//
// The slice must already be sorted ascending; sorting it here would mean either
// mutating the caller's column or allocating a copy of it per frame.
func Quantile(sorted []float64, p float64) float64 {
	n := len(sorted)
	switch n {
	case 0:
		return math.NaN()
	case 1:
		return sorted[0]
	}
	h := p * float64(n-1)
	i := int(math.Floor(h))
	if i >= n-1 {
		return sorted[n-1]
	}
	return sorted[i] + (h-float64(i))*(sorted[i+1]-sorted[i])
}

// StdDev returns the sample standard deviation of vs, ignoring NaN and
// infinities. It is 0 for fewer than two usable values.
//
// The two-pass form is deliberate. The textbook single-pass identity
// E[x²]-E[x]² cancels catastrophically on a column whose values are large and
// whose spread is small — a timestamp column in nanoseconds is exactly that —
// and a bandwidth rule fed a negative variance produces a density of NaN.
func StdDev(vs []float64) float64 {
	n, mean := 0, 0.0
	for _, v := range vs {
		if finite(v) {
			n++
			mean += v
		}
	}
	if n < 2 {
		return 0
	}
	mean /= float64(n)
	sum := 0.0
	for _, v := range vs {
		if finite(v) {
			d := v - mean
			sum += d * d
		}
	}
	return math.Sqrt(sum / float64(n-1))
}

// gridAt returns the i'th of n equally spaced points across [lo, hi].
//
// The ends are handed back exactly rather than accumulated, and that is
// load-bearing. `lo + float64(i)*step` is a multiply and an add, which Go may
// contract into a fused multiply-add "possibly across statements"; arm64 takes
// that contraction and amd64 does not, so a grid of sixty-one points over
// [-3, 3] ends at exactly 3 on one machine and at 3.0000000000000004 on the
// other. A grid whose last point is not its upper bound is wrong on both — the
// documentation says "across [lo, hi]" — and the fix is the one
// scale.place already carries for the same arithmetic: do not compute what is
// known.
func gridAt(lo, hi float64, i, n int) float64 {
	switch {
	case n <= 1 || i <= 0:
		return lo
	case i >= n-1:
		return hi
	}
	return lo + float64(i)*((hi-lo)/float64(n-1))
}

// finiteExtent is the range of the finite values in a column, and whether
// there were any. It is not [extent], which is the unguarded form the
// decimation family uses on columns it has already filtered.
func finiteExtent(vs []float64) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, v := range vs {
		if finite(v) {
			lo, hi, ok = math.Min(lo, v), math.Max(hi, v), true
		}
	}
	return lo, hi, ok
}
