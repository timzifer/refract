package scale

import "math"

// Extended Wilkinson tick labelling, after Talbot, Lin and Hanrahan,
// "An Extension of Wilkinson's Algorithm for Positioning Tick Labels on Axes"
// (InfoVis 2010).
//
// The algorithm searches over candidate tick sequences and scores each on four
// weighted criteria — simplicity (how "round" the step is), coverage (how
// tightly the labelled range hugs the data), density (how close the tick count
// is to the requested one) and legibility — returning the best-scoring
// sequence. It is what produces axis labels that look chosen rather than
// computed, and it is why refract does not use a 1-2-5 loop.

// niceSteps are the step multipliers the search considers, in decreasing order
// of preference. This is the paper's Q.
var niceSteps = [...]float64{1, 5, 2, 2.5, 4, 3}

// criteria weights: simplicity, coverage, density, legibility. The paper's w.
const (
	wSimplicity = 0.25
	wCoverage   = 0.2
	wDensity    = 0.5
	wLegibility = 0.05
)

// searchBound caps the inner loops. The paper's formulation lets k and z run to
// infinity, relying on the score bounds to break out; the caps are belt and
// braces against a pathological range producing an unbounded search.
const (
	maxSkipAmount = 4  // j
	maxTickCount  = 32 // k
	maxExponent   = 8  // z steps explored past the first candidate
)

// labelling is a candidate tick sequence.
type labelling struct {
	min, max, step float64
	count          int
}

// extendedWilkinson returns a tick sequence covering [dmin, dmax] with roughly
// want ticks. If loose is true the sequence is guaranteed to contain the data
// range; otherwise the search may crop to a tighter, rounder labelling.
//
// It falls back to a plain linear split when the range is degenerate or the
// search finds nothing, so it never returns an empty sequence for a valid
// range.
func extendedWilkinson(dmin, dmax float64, want int, loose bool) labelling {
	if want < 2 {
		want = 2
	}
	if !(dmin < dmax) || math.IsInf(dmin, 0) || math.IsInf(dmax, 0) ||
		math.IsNaN(dmin) || math.IsNaN(dmax) {
		return degenerate(dmin, dmax, want)
	}

	best := labelling{}
	bestScore := -2.0
	m := float64(want)

	for j := 1; j <= maxSkipAmount; j++ {
		fj := float64(j)
		for qi, q := range niceSteps {
			sm := simplicityMax(qi, fj)
			if wSimplicity*sm+wCoverage+wDensity+wLegibility < bestScore {
				// No q later in this j can beat the incumbent either.
				j = maxSkipAmount
				break
			}
			for k := 2; k <= maxTickCount; k++ {
				fk := float64(k)
				dm := densityMax(fk, m)
				if wSimplicity*sm+wCoverage+wDensity*dm+wLegibility < bestScore {
					break
				}
				delta := (dmax - dmin) / (fk + 1) / fj / q
				z := math.Ceil(math.Log10(delta))
				if math.IsInf(z, 0) || math.IsNaN(z) {
					continue
				}
				for e := 0; e <= maxExponent; e++ {
					step := fj * q * math.Pow(10, z+float64(e))
					if step <= 0 || math.IsInf(step, 0) {
						break
					}
					cm := coverageMax(dmin, dmax, step*(fk-1))
					if wSimplicity*sm+wCoverage*cm+wDensity*dm+wLegibility < bestScore {
						break
					}
					minStart := math.Floor(dmax/step)*fj - (fk-1)*fj
					maxStart := math.Ceil(dmin/step) * fj
					if minStart > maxStart {
						continue
					}
					for start := minStart; start <= maxStart; start++ {
						lmin := start * (step / fj)
						lmax := lmin + step*(fk-1)
						if loose && (lmin > dmin || lmax < dmax) {
							continue
						}
						score := wSimplicity*simplicity(qi, fj, lmin, lmax, step) +
							wCoverage*coverage(dmin, dmax, lmin, lmax) +
							wDensity*density(fk, m, dmin, dmax, lmin, lmax) +
							wLegibility*1.0
						if score > bestScore {
							bestScore = score
							best = labelling{min: lmin, max: lmax, step: step, count: k}
						}
					}
				}
			}
		}
	}

	if best.count == 0 {
		return degenerate(dmin, dmax, want)
	}
	return best
}

// simplicity scores how "round" the step is: earlier entries in niceSteps score
// higher, larger skip amounts score lower, and including zero earns a bonus.
func simplicity(qi int, j, lmin, lmax, lstep float64) float64 {
	const eps = 1e-10
	n := float64(len(niceSteps))
	i := float64(qi + 1)
	v := 0.0
	m := math.Mod(lmin, lstep)
	if (m < eps || lstep-m < eps) && lmin <= 0 && lmax >= 0 {
		v = 1
	}
	return 1 - (i-1)/(n-1) - j + v
}

func simplicityMax(qi int, j float64) float64 {
	n := float64(len(niceSteps))
	i := float64(qi + 1)
	return 1 - (i-1)/(n-1) - j + 1
}

// coverage penalises a labelled range that overshoots the data.
func coverage(dmin, dmax, l, h float64) float64 {
	r := dmax - dmin
	return 1 - 0.5*((dmax-h)*(dmax-h)+(dmin-l)*(dmin-l))/((0.1*r)*(0.1*r))
}

func coverageMax(dmin, dmax, span float64) float64 {
	r := dmax - dmin
	if span <= r {
		return 1
	}
	half := (span - r) / 2
	return 1 - 0.5*(half*half+half*half)/((0.1*r)*(0.1*r))
}

// density penalises tick counts far from the requested one.
func density(k, m, dmin, dmax, lmin, lmax float64) float64 {
	r := (k - 1) / (lmax - lmin)
	rt := (m - 1) / (math.Max(lmax, dmax) - math.Min(dmin, lmin))
	return 2 - math.Max(r/rt, rt/r)
}

func densityMax(k, m float64) float64 {
	if k >= m {
		return 2 - (k-1)/(m-1)
	}
	return 1
}

// degenerate handles ranges the search cannot work with: zero-width, reversed,
// or non-finite. It produces a usable sequence rather than failing, because an
// axis over constant data is a perfectly ordinary thing to plot.
func degenerate(dmin, dmax float64, want int) labelling {
	if math.IsNaN(dmin) || math.IsInf(dmin, 0) {
		dmin = 0
	}
	if math.IsNaN(dmax) || math.IsInf(dmax, 0) {
		dmax = dmin + 1
	}
	if dmax < dmin {
		dmin, dmax = dmax, dmin
	}
	if dmin == dmax {
		// Centre a unit-ish window on the value.
		pad := math.Abs(dmin) * 0.05
		if pad == 0 {
			pad = 0.5
		}
		dmin, dmax = dmin-pad, dmax+pad
	}
	step := (dmax - dmin) / float64(want-1)
	return labelling{min: dmin, max: dmax, step: step, count: want}
}

// values expands a labelling into its tick positions.
func (l labelling) values() []float64 {
	out := make([]float64, l.count)
	for i := range out {
		out[i] = l.min + float64(i)*l.step
	}
	return out
}
