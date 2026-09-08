package layout

import (
	"math"

	"github.com/timzifer/refract/ir"
)

// Labels places a panel's participating labels in drawing order. Its retained
// buffer is reused between frames; it stores geometry, never strings or rows.
// Reset makes it independent of whatever panel last borrowed it from the pool.
type Labels struct {
	area  ir.Rect
	boxes []ir.Rect
}

// Reset starts a panel, retaining capacity from the previous frame.
func (l *Labels) Reset(area ir.Rect) { l.area, l.boxes = area, l.boxes[:0] }

// Place tries a bounded, deterministic list of candidates. Earlier labels
// win, so ties cannot depend on scheduling or convergence of a simulation.
func (l *Labels) Place(run ir.TextRun, m ir.TextMetrics, move bool) (ir.Point, bool) {
	box := labelBounds(run, m)
	if !validLabelBox(box) || box.Empty() {
		return run.At, false
	}
	w, h := box.Max.X-box.Min.X+2, box.Max.Y-box.Min.Y+2
	offsets := [...]ir.Point{{}, {Y: -h}, {Y: h}, {X: w}, {X: -w}, {X: w, Y: -h}, {X: -w, Y: -h}, {X: w, Y: h}, {X: -w, Y: h}}
	n := 1
	if move {
		n = len(offsets)
	}
	for _, d := range offsets[:n] {
		b := ir.Rect{Min: ir.Point{X: box.Min.X + d.X, Y: box.Min.Y + d.Y}, Max: ir.Point{X: box.Max.X + d.X, Y: box.Max.Y + d.Y}}
		if b.Min.X < l.area.Min.X || b.Min.Y < l.area.Min.Y || b.Max.X > l.area.Max.X || b.Max.Y > l.area.Max.Y {
			continue
		}
		fits := true
		for _, old := range l.boxes {
			if b.Min.X < old.Max.X+1 && b.Max.X+1 > old.Min.X && b.Min.Y < old.Max.Y+1 && b.Max.Y+1 > old.Min.Y {
				fits = false
				break
			}
		}
		if fits {
			l.boxes = append(l.boxes, b)
			return ir.Point{X: run.At.X + d.X, Y: run.At.Y + d.Y}, true
		}
	}
	return run.At, false
}

func validLabelBox(b ir.Rect) bool {
	for _, v := range [...]float32{b.Min.X, b.Min.Y, b.Max.X, b.Max.Y} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return true
}

// Measure supplies baseline-relative ink and font metrics. Rotate all four
// corners after alignment, including italic overhang beyond the advance box.
func labelBounds(run ir.TextRun, m ir.TextMetrics) ir.Rect {
	x, y := float32(0), float32(0)
	switch run.H {
	case ir.AlignCenter:
		x = -m.Advance / 2
	case ir.AlignEnd:
		x = -m.Advance
	}
	switch run.V {
	case ir.AlignTop:
		y = m.Ascent
	case ir.AlignMiddle:
		y = (m.Ascent - m.Descent) / 2
	case ir.AlignBottom:
		y = -m.Descent
	}
	x0, x1 := x+min(float32(0), m.Ink.Min.X), x+max(m.Advance, m.Ink.Max.X)
	y0, y1 := y+min(-m.Ascent, m.Ink.Min.Y), y+max(m.Descent, m.Ink.Max.Y)
	s, c := math.Sincos(run.Rotation)
	var out ir.Rect
	for i, p := range [...]ir.Point{{X: x0, Y: y0}, {X: x1, Y: y0}, {X: x1, Y: y1}, {X: x0, Y: y1}} {
		q := ir.Point{X: run.At.X + float32(float64(p.X)*c-float64(p.Y)*s), Y: run.At.Y + float32(float64(p.X)*s+float64(p.Y)*c)}
		if i == 0 {
			out = ir.Rect{Min: q, Max: q}
		} else {
			out.Min.X, out.Min.Y = min(out.Min.X, q.X), min(out.Min.Y, q.Y)
			out.Max.X, out.Max.Y = max(out.Max.X, q.X), max(out.Max.Y, q.Y)
		}
	}
	return out
}
