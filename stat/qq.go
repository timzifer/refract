package stat

import "math"

// QQ compares ascending observations with a theoretical quantile function.
// X holds theoretical quantiles and Y holds observations, including ties.
// Plotting positions are (i+0.5)/n, strictly inside (0,1), so distributions
// with infinite endpoints still give finite tail positions. NaN and infinities
// in the sample are ignored. A nil quantile uses the standard normal.
//
// Input must already be sorted. The function never edits the caller's sample.
func QQ(sorted []float64, quantile func(float64) float64) []Point {
	return AppendQQ(nil, sorted, quantile)
}

// AppendQQ is QQ reusing dst, which is truncated before appending. A quantile
// function returning a non-finite value omits that point without changing the
// plotting positions of the other observations.
func AppendQQ(dst []Point, sorted []float64, quantile func(float64) float64) []Point {
	dst = dst[:0]
	n := countFinite(sorted)
	if n == 0 {
		return dst
	}
	if quantile == nil {
		quantile = NormalQuantile
	}
	i := 0
	for _, y := range sorted {
		if !finite(y) {
			continue
		}
		p := (float64(i) + 0.5) / float64(n)
		i++
		x := quantile(p)
		if finite(x) {
			dst = append(dst, Point{X: x, Y: y})
		}
	}
	return dst
}

// NormalQuantile is the inverse standard normal CDF. It returns infinities at
// 0 and 1, and NaN outside [0,1]. A QQ plot uses interior plotting positions.
func NormalQuantile(p float64) float64 {
	if p < 0 || p > 1 {
		return math.NaN()
	}
	return math.Sqrt2 * math.Erfinv(2*p-1)
}
