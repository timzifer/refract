package layout

import (
	"math"
	"testing"

	"github.com/timzifer/refract/ir"
)

func TestLabelsMoveWithoutOverlappingAndReset(t *testing.T) {
	var l Labels
	l.Reset(ir.R(0, 0, 200, 200))
	run := ir.TextRun{At: ir.Point{X: 100, Y: 100}, H: ir.AlignCenter, V: ir.AlignMiddle}
	m := ir.TextMetrics{Advance: 20, Ascent: 8, Descent: 2}
	for range 9 {
		if _, ok := l.Place(run, m, true); !ok {
			t.Fatal("candidate unexpectedly rejected")
		}
	}
	if _, ok := l.Place(run, m, true); ok {
		t.Fatal("accepted a tenth overlapping label")
	}
	for i, a := range l.boxes {
		for _, b := range l.boxes[i+1:] {
			if a.Min.X < b.Max.X && a.Max.X > b.Min.X && a.Min.Y < b.Max.Y && a.Max.Y > b.Min.Y {
				t.Fatalf("overlap: %v and %v", a, b)
			}
		}
	}
	l.Reset(ir.R(0, 0, 200, 200))
	if at, ok := l.Place(run, m, false); !ok || math.Abs(float64(at.X-100))+math.Abs(float64(at.Y-100)) > 0.01 {
		t.Fatalf("reset lost anchor: %v %v", at, ok)
	}
	if _, ok := l.Place(run, m, false); ok {
		t.Fatal("box label was moved")
	}
}

func TestLabelsAccountForRotationAlignmentAndPanelEdges(t *testing.T) {
	var l Labels
	l.Reset(ir.R(0, 0, 100, 100))
	m := ir.TextMetrics{Advance: 40, Ascent: 8, Descent: 2}
	r := ir.TextRun{At: ir.Point{X: 50, Y: 50}, H: ir.AlignEnd, V: ir.AlignTop, Rotation: math.Pi / 2}
	b := labelBounds(r, m)
	for _, pair := range [][2]float32{{b.Min.X, 40}, {b.Min.Y, 10}, {b.Max.X, 50}, {b.Max.Y, 50}} {
		if math.Abs(float64(pair[0]-pair[1])) > 0.01 {
			t.Fatalf("rotated bounds: %v", b)
		}
	}
	r.At = ir.Point{X: 0, Y: 0}
	if _, ok := l.Place(r, m, false); ok {
		t.Fatal("accepted clipped label")
	}
	r.At.X = float32(math.NaN())
	if _, ok := l.Place(r, m, true); ok {
		t.Fatal("accepted NaN")
	}
}
