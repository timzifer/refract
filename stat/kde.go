package stat

import "math"

// KDE evaluates a Gaussian kernel density estimate of vs at n equally spaced
// points across [lo, hi].
//
// A histogram answers "how many are here" and depends on where the bin edges
// happen to fall; a density estimate answers "how thick is the data here" and
// does not. That is the whole reason a violin is a violin rather than two
// histograms back to back: the shape must not change when the first bin edge
// moves half a bin.
//
// bw is the kernel bandwidth in the data's own units, and it is the one number
// that decides what the answer looks like — too small and the estimate is a
// comb of the observations, too large and every distribution is a hump. Pass
// bw <= 0 to have [Silverman] choose it. Pass lo >= hi to take the interval
// from the data, and n <= 0 for [DefaultKDEPoints].
//
// The result is normalised as a density: it integrates to 1 over the real line,
// not over [lo, hi], so two groups drawn on one axis are comparable in the way
// their areas are.
func KDE(vs []float64, bw, lo, hi float64, n int) []Point {
	return AppendKDE(nil, vs, bw, lo, hi, n)
}

// DefaultKDEPoints is how finely a density is evaluated when the caller names
// no resolution. It is well above the number of pixels a violin is drawn
// across, so the curve is limited by the bandwidth rather than by the sampling.
const DefaultKDEPoints = 128

// AppendKDE is [KDE] writing into a caller-owned slice. dst is truncated
// first, so a chart redrawn every frame estimates into the same memory.
//
// The cost is one exponential per observation per evaluation point. That is
// fine for the columns a violin or a ridgeline is drawn from — a group of a few
// thousand rows over a hundred points — and it is why nothing here reaches for
// a density estimate on a million-row column, where [Grid] is the honest
// answer.
func AppendKDE(dst []Point, vs []float64, bw, lo, hi float64, n int) []Point {
	dst = dst[:0]
	if lo >= hi {
		var ok bool
		lo, hi, ok = finiteExtent(vs)
		if !ok {
			return dst
		}
	}
	if n <= 0 {
		n = DefaultKDEPoints
	}
	if bw <= 0 {
		bw = Silverman(StdDev(vs), 0, countFinite(vs))
	}
	if !(bw > 0) || !finite(lo) || !finite(hi) {
		return dst
	}

	count := 0
	for _, v := range vs {
		if finite(v) {
			count++
		}
	}
	if count == 0 {
		return dst
	}

	// The normalisation is 1/(n·h·sqrt(2π)), applied once rather than per
	// kernel: the sum inside is a plain sum of exp(-t²/2).
	norm := 1 / (float64(count) * bw * math.Sqrt(2*math.Pi))
	for i := range n {
		x := gridAt(lo, hi, i, n)
		sum := 0.0
		for _, v := range vs {
			if !finite(v) {
				continue
			}
			t := (x - v) / bw
			sum += math.Exp(-0.5 * t * t)
		}
		dst = append(dst, Point{X: x, Y: sum * norm})
	}
	return dst
}

// Every kernel is summed, with no cutoff, and that is a correction rather than
// an oversight.
//
// This first skipped anything past four bandwidths, where a Gaussian is under a
// ten-thousandth of its peak — invisible in a chart, and worth an exp call. It
// is not invisible to a *decision*. A cutoff is a discontinuity: a value a hair
// inside it contributes 3.4e-4 and a value a hair outside contributes nothing,
// so the estimate depended on the last bit of its argument. That is the same
// trap scale.place documents, and it sprang the same way — a grid whose last
// point came out as 3 on amd64 and 3.0000000000000004 on arm64 put one sample
// on each side of the fence, and a density that was symmetric on one machine
// was not on the other.
//
// The cutoff was not even buying an order of growth: the loop visits every row
// either way, so it saved an exp and nothing else. Summing every kernel makes
// the estimate a continuous function of its argument, which is what a
// determinism claim across two architectures actually requires.

// Silverman returns the bandwidth Silverman's rule of thumb chooses:
// 0.9·min(σ, IQR/1.349)·n^(-1/5).
//
// Both spread measures are the caller's to supply, and iqr may be 0 to use the
// standard deviation alone. That is not a shortcut — computing an
// interquartile range means sorting, and a chart redrawn every frame sorts into
// a buffer it keeps rather than into one this package would allocate. A caller
// with a sorted column passes [Quantile] of it; one without passes 0 and gets
// the σ-only form, which is the rule as Silverman first wrote it.
//
// The robust minimum matters on real data: a single outlier inflates σ without
// moving the IQR, and a bandwidth taken from σ alone then smooths the whole
// distribution flat because one row was far away.
func Silverman(sd, iqr float64, n int) float64 {
	if n < 2 {
		return 0
	}
	spread := sd
	if iqr > 0 {
		if s := iqr / 1.349; spread <= 0 || s < spread {
			spread = s
		}
	}
	if !(spread > 0) {
		return 0
	}
	return 0.9 * spread * math.Pow(float64(n), -0.2)
}
