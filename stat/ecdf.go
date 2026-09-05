package stat

// ECDF returns the empirical cumulative distribution of an ascending column:
// one point per distinct value, whose Y is the fraction of observations at or
// below it.
//
// It is the distribution plot that invents nothing. A histogram picks bin edges
// and a density picks a bandwidth, and both choices change the picture; an ECDF
// has no parameter at all, so two of them drawn together are two datasets
// compared rather than two smoothing choices compared. That is what makes it
// worth having beside [Bin] and [KDE] rather than instead of either.
//
// The column must be sorted ascending, for the reason [Quantile] is: sorting it
// here would mean either mutating the caller's data or allocating a copy of it
// on every frame. NaN and infinities are ignored, which is why the fractions
// are taken against the number of usable values rather than against len.
func ECDF(sorted []float64) []Point { return AppendECDF(nil, sorted) }

// AppendECDF is [ECDF] writing into a caller-owned slice. dst is truncated
// first.
//
// One point per *distinct* value, not per row: a column with a thousand copies
// of one number is one step of height 1000/n, and emitting a thousand points at
// one X would draw a thousand identical vertices and report a thousand marks to
// a hit test.
func AppendECDF(dst []Point, sorted []float64) []Point {
	dst = dst[:0]
	n := countFinite(sorted)
	if n == 0 {
		return dst
	}

	seen := 0
	for i := 0; i < len(sorted); i++ {
		v := sorted[i]
		if !finite(v) {
			continue
		}
		seen++
		// Look ahead over the ties so the step is emitted once, at its full
		// height. The scan is linear overall: each row is visited by exactly one
		// of the two loops.
		for i+1 < len(sorted) && sorted[i+1] == v {
			i++
			seen++
		}
		dst = append(dst, Point{X: v, Y: float64(seen) / float64(n)})
	}
	return dst
}
