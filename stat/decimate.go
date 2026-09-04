package stat

import "math"

// LTTB reduces a series to at most threshold rows with the
// largest-triangle-three-buckets algorithm, returning the row numbers it kept
// in ascending order.
//
// LTTB splits the rows into equal-count buckets and keeps, from each, the row
// forming the largest triangle with the row kept before it and the mean of the
// bucket after it. Area is a proxy for "how much of the line's shape this row
// carries", which is why peaks and inflections survive a reduction that drops
// nine points in ten while a running average flattens them.
//
// It is lossy, and honestly so: the result is a subset of real rows, never an
// invented one, so every vertex the reader sees is a measurement that was
// taken. The first and last rows are always kept.
//
// Rows are assumed to be ordered along x. That is not an extra condition — a
// line geom already connects consecutive rows, so a series it can draw is a
// series this can bucket.
func LTTB[F Float](x, y []F, threshold int) []int {
	return AppendLTTB(nil, x, y, threshold)
}

// AppendLTTB is [LTTB] appending into dst.
func AppendLTTB[F Float](dst []int, x, y []F, threshold int) []int {
	n := min(len(x), len(y))
	// Fewer rows than the budget, or a budget too small to have an interior:
	// there is nothing to choose between, so keep everything.
	if threshold >= n || threshold < 3 {
		return appendAll(dst, n)
	}

	// The first and last rows are kept outright, so the buckets divide the
	// n-2 rows between them among the threshold-2 remaining slots.
	every := float64(n-2) / float64(threshold-2)

	dst = append(dst, 0)
	prev := 0
	for i := range threshold - 2 {
		lo, hi := bucket(i, every, n)
		if lo >= hi {
			continue
		}
		nlo, nhi := bucket(i+1, every, n)
		ax, ay := mean(x, y, nlo, nhi)
		px, py := float64(x[prev]), float64(y[prev])

		best, bestArea := lo, math.Inf(-1)
		for j := lo; j < hi; j++ {
			// Twice the area of the triangle (prev, candidate, next mean).
			// The factor of two is common to every candidate, so it never
			// changes which one wins and is not worth halving away.
			area := math.Abs((px-ax)*(float64(y[j])-py) - (px-float64(x[j]))*(ay-py))
			if area > bestArea {
				best, bestArea = j, area
			}
		}
		dst = append(dst, best)
		prev = best
	}
	return append(dst, n-1)
}

// bucket returns the half-open row range of bucket i, clamped to the interior
// rows [1, n-1) that LTTB is free to choose from.
func bucket(i int, every float64, n int) (lo, hi int) {
	lo = int(float64(i)*every) + 1
	hi = int(float64(i+1)*every) + 1
	return min(lo, n-1), min(hi, n-1)
}

// mean is the centre of gravity of a bucket, which stands in for the rows LTTB
// has not chosen yet. An empty bucket — which only happens at the end — falls
// back to the last row, the one that is kept regardless.
func mean[F Float](x, y []F, lo, hi int) (mx, my float64) {
	if lo >= hi {
		return float64(x[len(x)-1]), float64(y[len(y)-1])
	}
	for j := lo; j < hi; j++ {
		mx += float64(x[j])
		my += float64(y[j])
	}
	c := float64(hi - lo)
	return mx / c, my / c
}

// MinMax reduces a series to the rows that decide what each column of pixels
// looks like, returning the row numbers it kept in ascending order.
//
// Each column keeps four rows at most: the one that entered it, the smallest
// and the largest value in it, and the one that left. Keeping the extremes is
// what makes this reduction visually lossless for a signal — a spike one
// sample wide still reaches full height, where [LTTB] would weigh it against
// its neighbours and might drop it. Keeping the entry and the exit is what
// makes the segments between columns land where the data actually crosses
// them.
//
// ys is every value column the mark occupies: one for a line, two for a band,
// so that the kept rows bound the whole shape rather than one edge of it.
//
// Rows are assumed to be ordered along x, as in [LTTB].
func MinMax[F Float](x []F, columns int, ys ...[]F) []int {
	return AppendMinMax(nil, x, columns, ys...)
}

// AppendMinMax is [MinMax] appending into dst.
func AppendMinMax[F Float](dst []int, x []F, columns int, ys ...[]F) []int {
	n := len(x)
	for _, c := range ys {
		n = min(n, len(c))
	}
	if n == 0 || len(ys) == 0 {
		return dst
	}
	if columns < 1 {
		columns = 1
	}
	// Four rows per column is this reduction's own ceiling, so a series
	// already that small cannot be made smaller by running it.
	if n <= 4*columns {
		return appendAll(dst, n)
	}

	// Column index as a multiply rather than a divide: this runs once per
	// row, and a million rows is the case the whole reduction exists for.
	x0, x1 := extent(x[:n])
	perColumn := 0.0
	if span := float64(x1 - x0); span > 0 {
		perColumn = float64(columns) / span
	}
	col := func(i int) int {
		c := int(perColumn * (float64(x[i]) - float64(x0)))
		return min(max(c, 0), columns-1)
	}

	// base is where this call's own output starts, so that the de-duplication
	// below compares against rows this call appended and not against whatever
	// the caller already had in dst.
	base := len(dst)
	cur, first, lo, hi := col(0), 0, 0, 0
	vlo, vhi := extentAt(ys, 0)
	for i := 1; i < n; i++ {
		if c := col(i); c != cur {
			dst = emitColumn(dst, base, first, lo, hi, i-1)
			cur, first, lo, hi = c, i, i, i
			vlo, vhi = extentAt(ys, i)
			continue
		}
		a, b := extentAt(ys, i)
		if a < vlo {
			vlo, lo = a, i
		}
		if b > vhi {
			vhi, hi = b, i
		}
	}
	return emitColumn(dst, base, first, lo, hi, n-1)
}

// emitColumn appends one column's kept rows in ascending order.
//
// first and last bracket lo and hi by construction, so ordering them is a
// swap rather than a sort, and the four can only collide with their immediate
// neighbour — which is why comparing against the tail of dst is enough to keep
// the result strictly increasing.
func emitColumn(dst []int, base, first, lo, hi, last int) []int {
	if lo > hi {
		lo, hi = hi, lo
	}
	for _, i := range [4]int{first, lo, hi, last} {
		if len(dst) == base || dst[len(dst)-1] < i {
			dst = append(dst, i)
		}
	}
	return dst
}

// extentAt is how far row i reaches vertically: the span of every value column
// at that row, so that a band is bounded by both of its edges rather than by
// one.
func extentAt[F Float](ys [][]F, i int) (lo, hi float64) {
	lo = float64(ys[0][i])
	hi = lo
	for _, c := range ys[1:] {
		v := float64(c[i])
		lo, hi = min(lo, v), max(hi, v)
	}
	return lo, hi
}

func extent[F Float](vs []F) (lo, hi F) {
	lo, hi = vs[0], vs[0]
	for _, v := range vs[1:] {
		lo, hi = min(lo, v), max(hi, v)
	}
	return lo, hi
}

func appendAll(dst []int, n int) []int {
	for i := range n {
		dst = append(dst, i)
	}
	return dst
}
