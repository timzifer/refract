package ir_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/ir"
)

func TestPathWalkRoundTrip(t *testing.T) {
	var p ir.Path
	p.MoveTo(1, 2).LineTo(3, 4).CubicTo(5, 6, 7, 8, 9, 10).Close()

	var ops []ir.PathOp
	var counts []int
	p.Walk(func(op ir.PathOp, pts []ir.Point) {
		ops = append(ops, op)
		counts = append(counts, len(pts))
	})

	wantOps := []ir.PathOp{ir.OpMoveTo, ir.OpLineTo, ir.OpCubicTo, ir.OpClose}
	wantCounts := []int{1, 1, 3, 0}
	for i := range wantOps {
		if ops[i] != wantOps[i] || counts[i] != wantCounts[i] {
			t.Fatalf("op %d = %v with %d points, want %v with %d", i, ops[i], counts[i], wantOps[i], wantCounts[i])
		}
	}
}

func TestPathResetKeepsCapacity(t *testing.T) {
	var p ir.Path
	p.Polyline([]ir.Point{{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 2, Y: 2}})
	opCap, ptCap := cap(p.Ops), cap(p.Pts)

	p.Reset()
	if !p.Empty() {
		t.Fatal("Reset left the path non-empty")
	}
	if cap(p.Ops) != opCap || cap(p.Pts) != ptCap {
		t.Fatalf("Reset dropped capacity: ops %d->%d, pts %d->%d", opCap, cap(p.Ops), ptCap, cap(p.Pts))
	}
}

func TestPathPolylineNeedsTwoPoints(t *testing.T) {
	var p ir.Path
	p.Polyline([]ir.Point{{X: 1, Y: 1}})
	if !p.Empty() {
		t.Fatal("a one-point polyline should emit nothing")
	}
}

func TestPathBounds(t *testing.T) {
	var p ir.Path
	p.MoveTo(-2, 5).LineTo(4, -1).LineTo(0, 3)
	got := p.Bounds()
	want := ir.R(-2, -1, 4, 5)
	if got != want {
		t.Fatalf("Bounds() = %v, want %v", got, want)
	}
}

func TestAffineMulIsApplyOrder(t *testing.T) {
	// a.Mul(b) must apply b first, then a.
	scale := ir.Scale(2, 3)
	move := ir.Translate(10, 20)

	got := scale.Mul(move).Apply(ir.Point{X: 1, Y: 1})
	want := scale.Apply(move.Apply(ir.Point{X: 1, Y: 1}))
	if got != want {
		t.Fatalf("Mul composed in the wrong order: got %v, want %v", got, want)
	}
	if want != (ir.Point{X: 22, Y: 63}) {
		t.Fatalf("sanity check failed: %v", want)
	}
}

func TestAffineRotateMatchesSVGConvention(t *testing.T) {
	// A quarter turn must take +X to +Y, which is clockwise on screen.
	got := ir.Rotate(math.Pi / 2).Apply(ir.Point{X: 1, Y: 0})
	if math.Abs(float64(got.X)) > 1e-6 || math.Abs(float64(got.Y)-1) > 1e-6 {
		t.Fatalf("Rotate(pi/2) took (1,0) to %v, want (0,1)", got)
	}
}

func TestFade(t *testing.T) {
	c := ir.RGB(10, 20, 30)
	if got := ir.Fade(c, 0.5); got.A != 128 {
		t.Fatalf("Fade(.., 0.5).A = %d, want 128", got.A)
	}
	if got := ir.Fade(c, 2); got != c {
		t.Fatalf("Fade above 1 should clamp, got %v", got)
	}
	if got := ir.Fade(c, -1); got.A != 0 {
		t.Fatalf("Fade below 0 should clamp to transparent, got %v", got)
	}
}

func TestVisibility(t *testing.T) {
	if (ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 0}).Visible() {
		t.Error("a zero-width stroke is not visible")
	}
	if (ir.Stroke{Color: ir.Transparent, Width: 2}).Visible() {
		t.Error("a transparent stroke is not visible")
	}
	if !(ir.Fill{Stops: []ir.GradientStop{{Color: ir.RGB(1, 2, 3)}}}).Visible() {
		t.Error("a gradient with an opaque stop is visible")
	}
	if (ir.Fill{Stops: []ir.GradientStop{{Color: ir.Transparent}}}).Visible() {
		t.Error("a gradient whose every stop is transparent is not visible")
	}
}
