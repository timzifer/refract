package stat

import "math"

// DefaultSpan is the fraction of the data one local fit sees when the caller
// names none. Three quarters is Cleveland's own default and is the right
// starting point for a trend line: enough neighbours that the fit is a trend
// rather than an interpolation, few enough that it still bends.
const DefaultSpan = 0.75

// DefaultLoessPoints is how many abscissae a fit is evaluated at when the
// caller names no resolution.
const DefaultLoessPoints = 64

// Loess fits a locally weighted linear regression of ys on xs and evaluates it
// at n equally spaced points across the data.
//
// It is the trend line that does not assume a shape. A straight fit answers
// "is this rising", which is a question about the whole column at once; loess
// answers "what is it doing here", by fitting a line through the neighbours of
// each abscissa and weighting them by how near they are. The span decides how
// many neighbours that is, as a fraction of the rows in (0, 1]: small spans
// follow the data, large ones flatten it, and the choice is the reader's rather
// than this function's.
//
// # What the columns must be
//
// xs ascending, both columns finite, and the same length — the shorter one wins
// if they are not. That is a narrower contract than the rest of this package
// takes and it is deliberate: sorting and hole-filling both need a buffer, a
// caller redrawing a chart every frame already keeps one, and doing it here
// would allocate a copy of the column per frame to save the caller a loop. See
// [github.com/timzifer/refract/geom.Trend], which is that caller.
//
// Pass span <= 0 for [DefaultSpan] and n <= 0 for [DefaultLoessPoints].
//
// The fit is locally *linear* — degree one, not degree two. A quadratic local
// fit tracks a peak more faithfully and overshoots at the ends of the data,
// where a trend line is read most confidently and is least supported; the
// linear form is the conservative half of that trade.
func Loess(xs, ys []float64, span float64, n int) []Point {
	return AppendLoess(nil, xs, ys, span, n)
}

// AppendLoess is [Loess] writing into a caller-owned slice. dst is truncated
// first, so a chart redrawn every frame fits into the same memory.
//
// The window is walked forward rather than searched for: the abscissae ascend,
// so the neighbourhood of the next one starts at or after this one's. That is
// what keeps a fit over a long column linear in it rather than quadratic.
func AppendLoess(dst []Point, xs, ys []float64, span float64, n int) []Point {
	dst = dst[:0]
	if span <= 0 || span > 1 {
		span = DefaultSpan
	}
	if n <= 0 {
		n = DefaultLoessPoints
	}

	m := min(len(xs), len(ys))
	if m == 0 {
		return dst
	}
	lo, hi := xs[0], xs[m-1]
	if m == 1 || !(lo < hi) {
		return append(dst, Point{X: lo, Y: ys[0]})
	}

	// How many neighbours one local fit sees. At least two, or the weighted
	// line through them is not determined.
	q := min(max(int(math.Ceil(span*float64(m))), 2), m)

	left := 0
	for i := range n {
		x := gridAt(lo, hi, i, n)
		// Slide the window while the row entering on the right is nearer to x
		// than the row leaving on the left. On an ascending column that is the
		// definition of the q nearest neighbours, and it never moves backwards
		// because x only increases.
		for left+q < m && math.Abs(xs[left+q]-x) < math.Abs(xs[left]-x) {
			left++
		}
		if p, ok := fitAt(xs[left:left+q], ys[left:left+q], x); ok {
			dst = append(dst, p)
		}
	}
	return dst
}

// fitAt evaluates the weighted least-squares line through one window at x.
//
// The weight is Cleveland's tricube, (1-|d/dmax|³)³, over the distance to the
// furthest neighbour in the window. A window whose furthest neighbour is at
// distance zero — every X in it is x — has no spread to weight, so the answer
// is the plain mean of its Y values, which is the limit of the fit there.
func fitAt(xs, ys []float64, x float64) (Point, bool) {
	if len(xs) == 0 {
		return Point{}, false
	}
	dmax := 0.0
	for _, v := range xs {
		dmax = math.Max(dmax, math.Abs(v-x))
	}

	var sw, swx, swy, swxx, swxy float64
	for i, v := range xs {
		w := 1.0
		if dmax > 0 {
			t := math.Abs(v-x) / dmax
			if t >= 1 {
				continue
			}
			u := 1 - t*t*t
			w = u * u * u
		}
		if w <= 0 {
			continue
		}
		sw += w
		swx += w * v
		swy += w * ys[i]
		swxx += w * v * v
		swxy += w * v * ys[i]
	}
	if sw == 0 {
		return Point{}, false
	}
	mx, my := swx/sw, swy/sw
	den := swxx - sw*mx*mx
	if den <= 0 {
		return Point{X: x, Y: my}, true
	}
	slope := (swxy - sw*mx*my) / den
	return Point{X: x, Y: my + slope*(x-mx)}, true
}
