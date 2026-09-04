package ir_test

import (
	"image"
	"testing"

	"github.com/timzifer/refract/ir"
)

// recordInto builds a recording from a function that draws.
func recordInto(draw func(b ir.Backend)) *ir.Recorder {
	r := ir.NewRecorder(nil)
	draw(r)
	return r
}

func line(y float32) func(ir.Backend) {
	return func(b ir.Backend) {
		b.FillPath(rect(0, 0, 100, 100), ir.Solid(ir.RGB(255, 255, 255)), ir.NonZero)
		b.Polyline([]ir.Point{{X: 10, Y: y}, {X: 90, Y: y}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 2})
	}
}

func rect(x0, y0, x1, y1 float32) *ir.Path {
	var p ir.Path
	p.Rect(ir.R(x0, y0, x1, y1))
	return &p
}

func TestIdenticalRecordingsHaveNoDamage(t *testing.T) {
	a, b := recordInto(line(50)), recordInto(line(50))
	rects, ok := ir.Damage(a, b, nil)
	if !ok {
		t.Fatal("two identical recordings were reported as incomparable")
	}
	if len(rects) != 0 {
		t.Errorf("damage = %v, want none", rects)
	}
}

func TestDamageIsWhereTheDataMoved(t *testing.T) {
	a, b := recordInto(line(20)), recordInto(line(80))
	rects, ok := ir.Damage(a, b, nil)
	if !ok {
		t.Fatal("incomparable")
	}
	// Two rectangles: where the line was, and where it is. Not one box around
	// both — the sixty pixels between them did not change, and repainting them
	// is the work damage tracking exists to avoid.
	if len(rects) != 2 {
		t.Fatalf("damage = %v, want the old position and the new one", rects)
	}
	for _, r := range rects {
		if r.Min.X > 9 || r.Max.X < 91 {
			t.Errorf("damage %v does not span the line", r)
		}
		if r.Dy() > 4 {
			t.Errorf("damage %v is taller than a two-pixel stroke", r)
		}
	}
	if rects[0].Max.Y > 25 == (rects[1].Max.Y > 25) {
		t.Errorf("damage %v does not cover both positions", rects)
	}
}

func TestADifferentStructureIsNotComparable(t *testing.T) {
	a := recordInto(line(20))
	b := recordInto(func(bk ir.Backend) {
		line(20)(bk)
		bk.Polyline([]ir.Point{{X: 0, Y: 0}, {X: 10, Y: 10}}, ir.Stroke{Color: ir.RGB(1, 2, 3), Width: 1})
	})
	if _, ok := ir.Damage(a, b, nil); ok {
		t.Error("a recording with an extra call was reported as comparable")
	}

	// A different kind at the same index is not comparable either.
	c := recordInto(func(bk ir.Backend) {
		bk.FillPath(rect(0, 0, 100, 100), ir.Solid(ir.RGB(255, 255, 255)), ir.NonZero)
		bk.Markers(ir.MarkerCircle, []ir.Point{{X: 10, Y: 20}}, ir.MarkerStyle{Size: 4})
	})
	if _, ok := ir.Damage(a, c, nil); ok {
		t.Error("a recording with a different call kind was reported as comparable")
	}
}

func TestAMovedTransformIsNotComparable(t *testing.T) {
	a := recordInto(func(b ir.Backend) {
		b.Push(nil, ir.Translate(10, 0))
		b.Polyline([]ir.Point{{X: 0, Y: 0}, {X: 1, Y: 1}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})
		b.Pop()
	})
	c := recordInto(func(b ir.Backend) {
		b.Push(nil, ir.Translate(20, 0))
		b.Polyline([]ir.Point{{X: 0, Y: 0}, {X: 1, Y: 1}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})
		b.Pop()
	})
	if _, ok := ir.Damage(a, c, nil); ok {
		t.Error("a recording under a different transform was reported as comparable")
	}
}

func TestDamageIsInDeviceSpace(t *testing.T) {
	under := func(y float32) func(ir.Backend) {
		return func(b ir.Backend) {
			b.Push(nil, ir.Translate(200, 100))
			b.Polyline([]ir.Point{{X: 0, Y: y}, {X: 10, Y: y}}, ir.Stroke{Color: ir.RGB(0, 0, 0)})
			b.Pop()
		}
	}
	rects, ok := ir.Damage(recordInto(under(0)), recordInto(under(10)), nil)
	if !ok {
		t.Fatal("incomparable")
	}
	if len(rects) != 2 {
		t.Fatalf("damage = %v", rects)
	}
	for _, r := range rects {
		if r.Min.X < 199 || r.Min.Y < 99 {
			t.Errorf("damage %v was not transformed into device space", r)
		}
	}
}

func TestAStyleChangeIsDamage(t *testing.T) {
	a := recordInto(func(b ir.Backend) {
		b.Polyline([]ir.Point{{X: 0, Y: 0}, {X: 10, Y: 0}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})
	})
	c := recordInto(func(b ir.Backend) {
		b.Polyline([]ir.Point{{X: 0, Y: 0}, {X: 10, Y: 0}}, ir.Stroke{Color: ir.RGB(255, 0, 0), Width: 1})
	})
	rects, ok := ir.Damage(a, c, nil)
	if !ok || len(rects) != 1 {
		t.Errorf("recolouring a line produced %v, %v — a colour change is a repaint", rects, ok)
	}

	// A dash pattern is behind a slice, which is exactly the field a == would
	// have missed.
	d := recordInto(func(b ir.Backend) {
		b.Polyline([]ir.Point{{X: 0, Y: 0}, {X: 10, Y: 0}},
			ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1, Dash: []float32{2, 2}})
	})
	if rects, ok := ir.Damage(a, d, nil); !ok || len(rects) != 1 {
		t.Errorf("dashing a line produced %v, %v", rects, ok)
	}
}

func TestOverlappingDamageIsMerged(t *testing.T) {
	// Two lines that move into each other's boxes come back as one rectangle:
	// repainting a region twice composites it twice.
	wide := ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 8}
	a := recordInto(func(b ir.Backend) {
		b.Polyline([]ir.Point{{X: 0, Y: 0}, {X: 10, Y: 0}}, wide)
		b.Polyline([]ir.Point{{X: 5, Y: 5}, {X: 15, Y: 5}}, wide)
	})
	c := recordInto(func(b ir.Backend) {
		b.Polyline([]ir.Point{{X: 0, Y: 2}, {X: 10, Y: 2}}, wide)
		b.Polyline([]ir.Point{{X: 5, Y: 7}, {X: 15, Y: 7}}, wide)
	})
	rects, ok := ir.Damage(a, c, nil)
	if !ok {
		t.Fatal("incomparable")
	}
	if len(rects) != 1 {
		t.Errorf("damage = %v, want the two overlapping boxes merged into one", rects)
	}
}

func TestAnImageIsComparedByItsPixels(t *testing.T) {
	img := func(v uint8) *image.NRGBA {
		m := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		for i := range m.Pix {
			m.Pix[i] = v
		}
		return m
	}
	draw := func(v uint8) func(ir.Backend) {
		return func(b ir.Backend) { b.Image(img(v), ir.R(0, 0, 10, 10)) }
	}
	if rects, ok := ir.Damage(recordInto(draw(1)), recordInto(draw(1)), nil); !ok || len(rects) != 0 {
		t.Errorf("identical rasters produced %v, %v", rects, ok)
	}
	if rects, ok := ir.Damage(recordInto(draw(1)), recordInto(draw(2)), nil); !ok || len(rects) != 1 {
		t.Errorf("a changed raster produced %v, %v", rects, ok)
	}
}

func TestBoundsCoverEverythingDrawn(t *testing.T) {
	r := recordInto(func(b ir.Backend) {
		b.Polyline([]ir.Point{{X: 10, Y: 10}, {X: 90, Y: 60}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 4})
		b.Markers(ir.MarkerCircle, []ir.Point{{X: 5, Y: 5}}, ir.MarkerStyle{Size: 10})
	})
	got := r.Bounds()
	if got.Min.X > 0 || got.Min.Y > 0 {
		t.Errorf("bounds %v cut off the marker at (5,5) with a radius of five", got)
	}
	if got.Max.X < 92 || got.Max.Y < 62 {
		t.Errorf("bounds %v cut off the stroked line", got)
	}
	if (&ir.Recorder{}).Bounds() != (ir.Rect{}) {
		t.Error("an empty recording has bounds")
	}
}

func TestDamageAppendsToTheGivenSlice(t *testing.T) {
	buf := make([]ir.Rect, 0, 8)
	for range 3 {
		var ok bool
		buf, ok = ir.Damage(recordInto(line(20)), recordInto(line(80)), buf)
		if !ok || len(buf) != 2 {
			t.Fatalf("damage = %v, %v", buf, ok)
		}
	}
	if cap(buf) != 8 {
		t.Errorf("the buffer was reallocated: cap = %d, want the 8 it was given", cap(buf))
	}
}

func TestManyRectanglesCollapse(t *testing.T) {
	// Past a couple of dozen disjoint regions, describing the frame costs more
	// than repainting it, so the list collapses to one box.
	spread := func(dy float32) func(ir.Backend) {
		return func(b ir.Backend) {
			for i := range 60 {
				y := float32(i)*10 + dy
				b.Polyline([]ir.Point{{X: 0, Y: y}, {X: 5, Y: y}}, ir.Stroke{Color: ir.RGB(0, 0, 0)})
			}
		}
	}
	rects, ok := ir.Damage(recordInto(spread(0)), recordInto(spread(1)), nil)
	if !ok {
		t.Fatal("incomparable")
	}
	if len(rects) != 1 {
		t.Errorf("damage = %d rectangles, want them collapsed into one", len(rects))
	}
}

func TestTextIsCompared(t *testing.T) {
	run := func(s string) func(ir.Backend) {
		return func(b ir.Backend) {
			b.Text(ir.TextRun{Text: s, Font: ir.FontRef{Size: 12}, At: ir.Point{X: 50, Y: 20},
				Color: ir.RGB(0, 0, 0)})
		}
	}
	if rects, ok := ir.Damage(recordInto(run("12:00")), recordInto(run("12:00")), nil); !ok || len(rects) != 0 {
		t.Errorf("identical labels produced %v, %v", rects, ok)
	}
	rects, ok := ir.Damage(recordInto(run("12:00")), recordInto(run("12:05")), nil)
	if !ok || len(rects) != 1 {
		t.Fatalf("a relabelled tick produced %v, %v", rects, ok)
	}
	// The box has to cover the run rather than just its anchor, or the old
	// label is left on screen.
	if rects[0].Dx() < 10 || rects[0].Dy() < 5 {
		t.Errorf("the damaged box %v is too small to cover a label", rects[0])
	}
}

func TestARotatedRunGetsASquareBox(t *testing.T) {
	// A rotated run's ink is not axis-aligned, so the box is generous rather
	// than tight: damage that is too large repaints something that did not
	// change, which is invisible, while damage that is too small leaves a
	// ghost, which is not.
	run := func(s string) func(ir.Backend) {
		return func(b ir.Backend) {
			b.Text(ir.TextRun{Text: s, Font: ir.FontRef{Size: 12}, At: ir.Point{X: 50, Y: 50},
				Rotation: -1.5708, Color: ir.RGB(0, 0, 0)})
		}
	}
	rects, ok := ir.Damage(recordInto(run("amplitude")), recordInto(run("magnitude")), nil)
	if !ok || len(rects) != 1 {
		t.Fatalf("damage = %v, %v", rects, ok)
	}
	if rects[0].Dx() != rects[0].Dy() {
		t.Errorf("the box %v is not square", rects[0])
	}
	if !rects[0].Contains(ir.Point{X: 50, Y: 50}) {
		t.Errorf("the box %v does not contain the anchor", rects[0])
	}
}

func TestAPathIsComparedByItsVerbsAndPoints(t *testing.T) {
	bar := func(h float32) func(ir.Backend) {
		return func(b ir.Backend) {
			b.FillPath(rect(10, h, 20, 100), ir.Solid(ir.RGB(0, 0, 255)), ir.NonZero)
		}
	}
	if rects, ok := ir.Damage(recordInto(bar(50)), recordInto(bar(50)), nil); !ok || len(rects) != 0 {
		t.Errorf("an unchanged bar produced %v, %v", rects, ok)
	}
	if rects, ok := ir.Damage(recordInto(bar(50)), recordInto(bar(30)), nil); !ok || len(rects) != 1 {
		t.Errorf("a grown bar produced %v, %v", rects, ok)
	}

	// A different fill rule is a different shape.
	a := recordInto(func(b ir.Backend) {
		b.FillPath(rect(0, 0, 10, 10), ir.Solid(ir.RGB(1, 2, 3)), ir.NonZero)
	})
	c := recordInto(func(b ir.Backend) {
		b.FillPath(rect(0, 0, 10, 10), ir.Solid(ir.RGB(1, 2, 3)), ir.EvenOdd)
	})
	if rects, ok := ir.Damage(a, c, nil); !ok || len(rects) != 1 {
		t.Errorf("a changed fill rule produced %v, %v", rects, ok)
	}

	// A gradient is compared by its stops, which live behind a slice.
	grad := func(mid ir.Color) func(ir.Backend) {
		return func(b ir.Backend) {
			b.FillPath(rect(0, 0, 10, 10), ir.Fill{
				Start: ir.Point{}, End: ir.Point{X: 10},
				Stops: []ir.GradientStop{{Offset: 0, Color: ir.RGB(0, 0, 0)}, {Offset: 1, Color: mid}},
			}, ir.NonZero)
		}
	}
	if rects, ok := ir.Damage(recordInto(grad(ir.RGB(255, 255, 255))), recordInto(grad(ir.RGB(255, 0, 0))), nil); !ok || len(rects) != 1 {
		t.Errorf("a changed gradient produced %v, %v", rects, ok)
	}
}

func TestMarkersAreComparedByStyleAndPositions(t *testing.T) {
	dots := func(size float32, at float32) func(ir.Backend) {
		return func(b ir.Backend) {
			b.Markers(ir.MarkerCircle, []ir.Point{{X: at, Y: 10}, {X: at + 5, Y: 20}},
				ir.MarkerStyle{Size: size, Fill: ir.RGB(0, 0, 0)})
		}
	}
	if rects, ok := ir.Damage(recordInto(dots(6, 0)), recordInto(dots(6, 0)), nil); !ok || len(rects) != 0 {
		t.Errorf("identical markers produced %v, %v", rects, ok)
	}
	if rects, ok := ir.Damage(recordInto(dots(6, 0)), recordInto(dots(9, 0)), nil); !ok || len(rects) != 1 {
		t.Errorf("resized markers produced %v, %v", rects, ok)
	}
	if rects, ok := ir.Damage(recordInto(dots(6, 0)), recordInto(dots(6, 40)), nil); !ok || len(rects) != 2 {
		t.Errorf("moved markers produced %v, %v", rects, ok)
	}
}

func TestDamageOfNothingIsNotComparable(t *testing.T) {
	if _, ok := ir.Damage(nil, recordInto(line(10)), nil); ok {
		t.Error("a nil previous recording was reported as comparable")
	}
	if _, ok := ir.Damage(recordInto(line(10)), nil, nil); ok {
		t.Error("a nil next recording was reported as comparable")
	}
}
