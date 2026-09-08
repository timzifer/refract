package render_test

import (
	"testing"

	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

// solidFills returns the colours of every solid fill in a recording. A classed
// bar paints one per class, where a continuous bar paints one gradient.
func solidFills(rec *irtest.Recorder) map[ir.Color]bool {
	out := map[ir.Color]bool{}
	for _, c := range rec.Filter("FillPath") {
		if !c.Fill.IsGradient() {
			out[c.Fill.Color] = true
		}
	}
	return out
}

func TestAClassedScaleGetsASteppedBarRatherThanAGradient(t *testing.T) {
	cs := scale.Threshold(palette.Viridis, []float64{20, 30})
	rec := draw(t, chart(colored(cs)))
	if got := len(gradients(rec)); got != 0 {
		t.Errorf("a classed scale drew %d gradients, want none", got)
	}
	// Three classes, three colours, and each of them painted.
	cs.Train(10, 40)
	seen := solidFills(rec)
	for _, v := range []float64{10, 25, 35} {
		if !seen[cs.Color(v)] {
			t.Errorf("the bar has no band in the colour of %g", v)
		}
	}
}

// A classed bar is labelled at its boundaries and nowhere else: a round number
// between two of them would invite a reader to interpolate across a step that
// has no inside.
func TestAClassedBarIsLabelledAtItsBreaks(t *testing.T) {
	rec := draw(t, chart(colored(scale.Threshold(palette.Viridis, []float64{20, 30}))))
	for _, want := range []string{"20", "30"} {
		if !hasText(rec, want) {
			t.Errorf("the bar has no %q boundary label: %v", want, texts(rec))
		}
	}
	// 25 is a round number inside a class, and the bar must not claim it is a
	// boundary.
	if hasText(rec, "25") {
		t.Errorf("the bar labelled a value inside a class: %v", texts(rec))
	}
}

func TestAQuantizedBarIsLabelledAtItsDerivedBreaks(t *testing.T) {
	// The column runs 10..40, so three classes break at 20 and 30.
	rec := draw(t, chart(colored(scale.Quantize(palette.Viridis, 3))))
	for _, want := range []string{"20", "30"} {
		if !hasText(rec, want) {
			t.Errorf("the bar has no %q boundary label: %v", want, texts(rec))
		}
	}
}

// Two classed scales that differ only in where they cut are two different
// bars, and the guide key has to see the difference.
func TestClassedBarsWithDifferentBreaksAreNotMerged(t *testing.T) {
	a := colored(scale.Threshold(palette.Viridis, []float64{20}))
	b := colored(scale.Threshold(palette.Viridis, []float64{30}))
	rec := draw(t, chart(a, b))
	labels := 0
	for _, want := range []string{"20", "30"} {
		if hasText(rec, want) {
			labels++
		}
	}
	if labels != 2 {
		t.Errorf("two scales cutting at different values drew %d of the two boundaries: %v", labels, texts(rec))
	}
}
