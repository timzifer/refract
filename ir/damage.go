package ir

import (
	"image"
	"math"
)

// Partial is implemented by a backend that can repaint part of a frame rather
// than all of it.
//
// It is the seam damage tracking needs and the only one: refract works out
// *where* a frame changed, and a backend that can act on that says so by
// implementing this. A backend that cannot — a file emitter, which writes a
// whole document or none — simply does not, and gets a full repaint like
// always.
//
// The rectangles are in device space, the same coordinates a drawing call
// takes. A backend is free to widen them (to whole pixels, to tiles) and must
// not narrow them.
type Partial interface {
	// Damage limits the next frame to the given rectangles. An empty list
	// means nothing changed and the frame can be skipped entirely; the
	// limitation lasts until the next Flush.
	Damage(rects []Rect)
}

// maxDamageRects caps how finely a frame is described.
//
// Damage tracking pays off when a small part of a large frame changed. Past a
// couple of dozen rectangles the bookkeeping — clip, clear and repaint, once
// per rectangle — costs more than repainting the frame, so the list collapses
// to its own bounding box instead. A tracker that produces five hundred
// rectangles has found a full repaint the slow way.
const maxDamageRects = 24

// Damage reports where two recordings differ, in device space.
//
// ok is false when the two are not comparable call for call — a different
// number of drawing calls, a different kind at the same index, a transform or
// a clip that moved. That is a chart whose *structure* changed rather than its
// data, and the honest answer is a full repaint rather than a list of
// rectangles that describes half of it.
//
// ok is true with an empty list when the two recordings are identical, which
// is a frame a caller can skip.
//
// Rectangles are appended to dst, which may be nil. Reusing one across frames
// is what keeps a redraw allocation-free.
func Damage(prev, next *Recorder, dst []Rect) (rects []Rect, ok bool) {
	rects = dst[:0]
	if prev == nil || next == nil || len(prev.calls) != len(next.calls) {
		return rects, false
	}

	// The transform stack is replayed rather than remembered, because a
	// recorded call's points are in the space its Push established and the
	// answer has to be in the space the backend paints in.
	var stack []Affine
	at := Identity
	for i := range next.calls {
		a, b := prev.calls[i], next.calls[i]
		if a.kind != b.kind {
			return rects[:0], false
		}
		switch a.kind {
		case recPush:
			if a.xform != b.xform || a.clip != b.clip || !prev.samePath(next, a, b) {
				return rects[:0], false
			}
			stack = append(stack, at)
			at = at.Mul(a.xform)
			continue
		case recPop:
			if n := len(stack); n > 0 {
				at, stack = stack[n-1], stack[:n-1]
			}
			continue
		}
		if prev.sameCall(next, a, b) {
			continue
		}
		rects = append(rects, at.applyRect(prev.bounds(a)), at.applyRect(next.bounds(b)))
	}
	return merge(rects), true
}

// Bounds reports the rectangle enclosing everything a recording drew. It is
// what a caller repaints when [Damage] says the two frames are not comparable.
func (r *Recorder) Bounds() Rect {
	out := Rect{Min: Point{X: inf, Y: inf}, Max: Point{X: -inf, Y: -inf}}
	var stack []Affine
	at := Identity
	for _, c := range r.calls {
		switch c.kind {
		case recPush:
			stack = append(stack, at)
			at = at.Mul(c.xform)
			continue
		case recPop:
			if n := len(stack); n > 0 {
				at, stack = stack[n-1], stack[:n-1]
			}
			continue
		}
		out = union(out, at.applyRect(r.bounds(c)))
	}
	if out.Empty() {
		return Rect{}
	}
	return out
}

const inf = float32(math.MaxFloat32)

// sameCall compares two recorded drawing calls, including the arena-held data
// each points at. The arena indices themselves are deliberately not compared:
// two recordings of the same frame hold the same points at the same offsets
// only by accident.
func (r *Recorder) sameCall(other *Recorder, a, b recorded) bool {
	switch a.kind {
	case recPolyline:
		return a.stroke.same(b.stroke) && samePoints(r.pts[a.lo:a.hi], other.pts[b.lo:b.hi])
	case recStrokePath:
		return a.stroke.same(b.stroke) && r.samePath(other, a, b)
	case recFillPath:
		return a.rule == b.rule && a.fill.same(b.fill) && r.samePath(other, a, b)
	case recText:
		return a.text == b.text
	case recMarkers:
		return a.marker == b.marker && a.style.same(b.style) &&
			samePoints(r.pts[a.lo:a.hi], other.pts[b.lo:b.hi])
	case recImage:
		return a.rect == b.rect && sameImage(r.imgs[a.img], other.imgs[b.img])
	}
	return false
}

func (r *Recorder) samePath(other *Recorder, a, b recorded) bool {
	if a.ophi-a.oplo != b.ophi-b.oplo {
		return false
	}
	for i := range a.ophi - a.oplo {
		if r.ops[a.oplo+i] != other.ops[b.oplo+i] {
			return false
		}
	}
	return samePoints(r.ppts[a.ptlo:a.pthi], other.ppts[b.ptlo:b.pthi])
}

func samePoints(a, b []Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameImage(a, b *image.NRGBA) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Rect != b.Rect || len(a.Pix) != len(b.Pix) {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}

// same compares two strokes. A Stroke holds a dash slice, so it is not
// comparable with ==, and the compiler will not tell you: == on a struct
// containing a slice is a compile error, but a struct *literal* comparison
// that happens to be legal today stops being legal when a field is added.
func (s Stroke) same(t Stroke) bool {
	if s.Color != t.Color || s.Width != t.Width || s.Cap != t.Cap ||
		s.Join != t.Join || s.MiterLimit != t.MiterLimit || s.DashOffset != t.DashOffset {
		return false
	}
	return sameFloats(s.Dash, t.Dash)
}

func (s MarkerStyle) same(t MarkerStyle) bool {
	return s.Size == t.Size && s.Fill == t.Fill && s.Stroke.same(t.Stroke)
}

func (f Fill) same(g Fill) bool {
	if f.Color != g.Color || f.Start != g.Start || f.End != g.End || len(f.Stops) != len(g.Stops) {
		return false
	}
	for i := range f.Stops {
		if f.Stops[i] != g.Stops[i] {
			return false
		}
	}
	return true
}

func sameFloats(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// bounds reports the rectangle a recorded call paints into, in the space of
// the transform in force when it was recorded.
//
// Stroke width is added on every side, because a stroke straddles its path.
// Text is measured through the recording's own Measurer — the same one that
// will draw it — and falls back to the font box when there is none, which
// over-reports by a little and never under-reports.
func (r *Recorder) bounds(c recorded) Rect {
	switch c.kind {
	case recPolyline:
		return pointBounds(r.pts[c.lo:c.hi], c.stroke.Width/2)
	case recStrokePath:
		return pointBounds(r.ppts[c.ptlo:c.pthi], c.stroke.Width/2)
	case recFillPath:
		return pointBounds(r.ppts[c.ptlo:c.pthi], 0)
	case recMarkers:
		return pointBounds(r.pts[c.lo:c.hi], c.style.Size/2+c.style.Stroke.Width)
	case recImage:
		return c.rect
	case recText:
		return r.textBounds(c.text)
	}
	return Rect{}
}

// textBounds is a run's box in device space, generous rather than tight.
//
// A tight box would need the shaper's ink extents and a rotation-aware hull; a
// box that covers the run at any rotation is the advance and the font height
// taken as a radius about the anchor. Damage that is too large repaints
// something that did not change, which is invisible; damage that is too small
// leaves a ghost, which is not.
func (r *Recorder) textBounds(run TextRun) Rect {
	m := r.Measure(run)
	w, h := m.Advance, m.Height()
	if w == 0 {
		w = float32(run.Font.Size) * float32(len(run.Text)) * 0.6
	}
	if h == 0 {
		h = float32(run.Font.Size) * 1.4
	}
	if run.Rotation != 0 {
		d := float32(math.Hypot(float64(w), float64(h)))
		return Rect{
			Min: Point{X: run.At.X - d, Y: run.At.Y - d},
			Max: Point{X: run.At.X + d, Y: run.At.Y + d},
		}
	}
	x := run.At.X
	switch run.H {
	case AlignCenter:
		x -= w / 2
	case AlignEnd:
		x -= w
	}
	y := run.At.Y
	switch run.V {
	case AlignTop:
		y += 0
	case AlignMiddle:
		y -= h / 2
	case AlignBottom:
		y -= h
	default: // AlignBaseline
		y -= m.Ascent
		if m.Ascent == 0 {
			y -= h * 0.8
		}
	}
	return Rect{Min: Point{X: x, Y: y}, Max: Point{X: x + w, Y: y + h}}
}

func pointBounds(pts []Point, pad float32) Rect {
	if len(pts) == 0 {
		return Rect{}
	}
	out := Rect{Min: pts[0], Max: pts[0]}
	for _, p := range pts[1:] {
		out.Min.X = min(out.Min.X, p.X)
		out.Min.Y = min(out.Min.Y, p.Y)
		out.Max.X = max(out.Max.X, p.X)
		out.Max.Y = max(out.Max.Y, p.Y)
	}
	return out.Inset(-pad, -pad, -pad, -pad)
}

// applyRect transforms a rectangle and returns the axis-aligned box around the
// result. A rotation turns a rectangle into a quadrilateral; the box around it
// is the smallest rectangle that still covers everything.
func (a Affine) applyRect(r Rect) Rect {
	if a.IsIdentity() || r == (Rect{}) {
		return r
	}
	c := [4]Point{
		a.Apply(r.Min),
		a.Apply(Point{X: r.Max.X, Y: r.Min.Y}),
		a.Apply(r.Max),
		a.Apply(Point{X: r.Min.X, Y: r.Max.Y}),
	}
	out := Rect{Min: c[0], Max: c[0]}
	for _, p := range c[1:] {
		out.Min.X = min(out.Min.X, p.X)
		out.Min.Y = min(out.Min.Y, p.Y)
		out.Max.X = max(out.Max.X, p.X)
		out.Max.Y = max(out.Max.Y, p.Y)
	}
	return out
}

func union(a, b Rect) Rect {
	if b == (Rect{}) {
		return a
	}
	if a == (Rect{}) {
		return b
	}
	return Rect{
		Min: Point{X: min(a.Min.X, b.Min.X), Y: min(a.Min.Y, b.Min.Y)},
		Max: Point{X: max(a.Max.X, b.Max.X), Y: max(a.Max.Y, b.Max.Y)},
	}
}

func overlaps(a, b Rect) bool {
	return a.Min.X <= b.Max.X && b.Min.X <= a.Max.X &&
		a.Min.Y <= b.Max.Y && b.Min.Y <= a.Max.Y
}

// merge folds overlapping rectangles together, in place.
//
// Repainting one region twice is wasted work and, on a backend that composites
// rather than replaces, visibly wrong — a translucent mark drawn over itself is
// darker. So the list that comes out never overlaps itself.
func merge(rects []Rect) []Rect {
	out := rects[:0]
	for _, r := range rects {
		if r == (Rect{}) {
			continue
		}
		merged := false
		for i := range out {
			if overlaps(out[i], r) {
				out[i] = union(out[i], r)
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, r)
		}
	}
	// One pass leaves rectangles that only started overlapping once their
	// neighbours grew; a second settles it for every shape a chart produces.
	for again := true; again; {
		again = false
		for i := 0; i < len(out); i++ {
			for j := i + 1; j < len(out); j++ {
				if overlaps(out[i], out[j]) {
					out[i] = union(out[i], out[j])
					out = append(out[:j], out[j+1:]...)
					j--
					again = true
				}
			}
		}
	}
	if len(out) > maxDamageRects {
		all := out[0]
		for _, r := range out[1:] {
			all = union(all, r)
		}
		out = append(out[:0], all)
	}
	return out
}
