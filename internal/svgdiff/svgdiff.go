// Package svgdiff compares two SVG documents as drawings rather than as bytes.
//
// # Why this exists
//
// refract's SVG emitter is deterministic: fixed attribute order, fixed number
// formatting, no maps iterated, no timestamps. Two runs on one machine produce
// identical bytes. Across machines they do not, and cannot be made to.
//
// Go permits an implementation to contract `a*b + c` into a fused
// multiply-add, and arm64 does while amd64 does not. An FMA keeps the full
// intermediate product instead of rounding it, so the two architectures can
// disagree in the last bit of a float32 — which is exactly the shape of the
// expressions that produce chart coordinates: Catmull-Rom control points, a
// scale's `lo + t*(hi-lo)`, a layout's accumulated margins. One bit of
// disagreement lands on a rounding boundary often enough to change the printed
// third decimal, and a byte comparison then fails on a chart that is visually
// identical.
//
// Chasing the contraction out of every arithmetic site — the language spec
// says an explicit float32 conversion forbids it — would work until the next
// expression someone writes. Comparing numerically instead is both robust and
// more honest about what the golden files are actually asserting: that the
// same drawing comes out, not that the same floating-point rounding happened.
//
// Everything that is not a number is still compared exactly. Element order,
// attribute order, ids, colours, path verbs and text content must all match
// byte for byte, so the tests keep their teeth: this tolerates the last bit of
// a coordinate and nothing else.
package svgdiff

import (
	"fmt"
	"math"
	"strconv"
)

// DefaultTolerance is the permitted difference between two coordinates, in the
// document's own units — device pixels, for refract's output.
//
// A hundredth of a pixel is two orders of magnitude below anything visible and
// four above the one-ULP disagreement this exists to absorb. Any real change to
// layout, scales or geometry moves things by far more.
const DefaultTolerance = 0.01

// Equal reports whether got and want describe the same drawing, allowing
// numbers to differ by at most tol. When they differ, the returned string
// explains where and how.
func Equal(got, want []byte, tol float64) (bool, string) {
	i, j := 0, 0
	for i < len(got) && j < len(want) {
		gi, gv, gok := number(got, i)
		wj, wv, wok := number(want, j)

		if gok && wok {
			if math.Abs(gv-wv) > tol {
				return false, fmt.Sprintf(
					"number differs beyond tolerance %g: got %s, want %s\n%s",
					tol, got[i:gi], want[j:wj], context(got, want, i, j))
			}
			i, j = gi, wj
			continue
		}
		if got[i] != want[j] {
			return false, fmt.Sprintf("content differs\n%s", context(got, want, i, j))
		}
		i++
		j++
	}

	switch {
	case i < len(got):
		return false, fmt.Sprintf("the rendered document is longer; it continues with %q", trunc(got[i:]))
	case j < len(want):
		return false, fmt.Sprintf("the golden document is longer; it continues with %q", trunc(want[j:]))
	}
	return true, ""
}

// number parses a numeric literal starting at s[i]. It returns the index just
// past the literal, its value, and whether one was found.
//
// A leading sign counts as part of the number only when a digit or a decimal
// point follows, so a stray '-' in text is not mistaken for one.
func number(s []byte, i int) (int, float64, bool) {
	start := i
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		i++
	}
	digits := false
	for i < len(s) && isDigit(s[i]) {
		i++
		digits = true
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDigit(s[i]) {
			i++
			digits = true
		}
	}
	if !digits {
		return start, 0, false
	}
	// An exponent only counts if it is complete; "3e" in text is a 3 followed
	// by a letter.
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		k := i + 1
		if k < len(s) && (s[k] == '-' || s[k] == '+') {
			k++
		}
		if k < len(s) && isDigit(s[k]) {
			for k < len(s) && isDigit(s[k]) {
				k++
			}
			i = k
		}
	}
	v, err := strconv.ParseFloat(string(s[start:i]), 64)
	if err != nil {
		return start, 0, false
	}
	return i, v, true
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func context(got, want []byte, i, j int) string {
	return fmt.Sprintf("got:  ...%s...\nwant: ...%s...", window(got, i), window(want, j))
}

func window(s []byte, at int) []byte {
	lo := max(at-60, 0)
	return s[lo:min(at+60, len(s))]
}

func trunc(s []byte) []byte {
	if len(s) > 80 {
		return s[:80]
	}
	return s
}
