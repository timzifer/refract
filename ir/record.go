package ir

import (
	"image"
	"image/draw"
)

// Measurer is the part of a [Backend] that is needed before anything is drawn.
//
// Layout runs first and needs to know how wide a tick label will be, so text
// measurement is separable from drawing — and separating it is what lets a
// recorder stand in for a backend without being able to draw at all.
type Measurer interface {
	// Measure reports metrics for a run. See [Backend.Measure].
	Measure(run TextRun) TextMetrics
}

// Recorder is a [Backend] that stores what was drawn on it so that it can be
// replayed into another Backend later.
//
// It is how refract draws several panels at once. A Backend is not safe for
// concurrent use, so each goroutine draws into a Recorder of its own and the
// recordings are replayed into the real backend afterwards, in panel order.
// Replaying in a fixed order is what makes a parallel render produce exactly
// what a serial one does rather than merely something equivalent.
//
// Points and path data go into flat arenas rather than one slice per call, so
// a Recorder that is [Recorder.Reset] and reused costs nothing per frame after
// the first.
//
// A Recorder is not safe for concurrent use either — one per goroutine is the
// whole idea. Measure is forwarded to the Measurer it was built with, which
// therefore must be safe to call from every goroutine using a Recorder.
type Recorder struct {
	m     Measurer
	calls []recorded
	pts   []Point
	ops   []PathOp
	ppts  []Point
	imgs  []*image.NRGBA
	nimg  int
}

// NewRecorder returns a Recorder measuring through m. m may be nil, in which
// case Measure reports zero — which is correct for a recorder used only for a
// data pass, since layout has already run by then.
func NewRecorder(m Measurer) *Recorder { return &Recorder{m: m} }

type recKind uint8

const (
	recPolyline recKind = iota
	recStrokePath
	recFillPath
	recText
	recMarkers
	recImage
	recPush
	recPop
)

type recorded struct {
	kind       recKind
	lo, hi     int // into pts
	oplo, ophi int // into ops
	ptlo, pthi int // into ppts
	img        int // into imgs
	stroke     Stroke
	fill       Fill
	rule       FillRule
	text       TextRun
	marker     Marker
	style      MarkerStyle
	rect       Rect
	xform      Affine
	clip       bool
}

// Reset empties the recording, keeping the memory for the next one.
func (r *Recorder) Reset() {
	r.calls = r.calls[:0]
	r.pts = r.pts[:0]
	r.ops = r.ops[:0]
	r.ppts = r.ppts[:0]
	r.nimg = 0
}

// SetMeasurer points the Recorder at a different Measurer. It exists so that a
// pooled Recorder can serve one render after another, each with a backend of
// its own.
func (r *Recorder) SetMeasurer(m Measurer) { r.m = m }

// Empty reports whether nothing has been recorded.
func (r *Recorder) Empty() bool { return len(r.calls) == 0 }

// Calls reports how many drawing operations were recorded.
func (r *Recorder) Calls() int { return len(r.calls) }

func (r *Recorder) points(pts []Point) (lo, hi int) {
	lo = len(r.pts)
	r.pts = append(r.pts, pts...)
	return lo, len(r.pts)
}

func (r *Recorder) path(p *Path) (oplo, ophi, ptlo, pthi int) {
	if p == nil {
		return 0, 0, 0, 0
	}
	oplo, ptlo = len(r.ops), len(r.ppts)
	r.ops = append(r.ops, p.Ops...)
	r.ppts = append(r.ppts, p.Pts...)
	return oplo, len(r.ops), ptlo, len(r.ppts)
}

// keepImage copies img into a buffer the Recorder owns.
//
// The copy is not optional. A geom that rasterizes draws into pooled pixels
// and lends them for the length of the call, exactly as it lends a point
// slice; keeping the reference would replay whatever the next frame put there.
func (r *Recorder) keepImage(img image.Image) int {
	b := img.Bounds()
	if r.nimg == len(r.imgs) {
		r.imgs = append(r.imgs, image.NewNRGBA(b))
	}
	dst := r.imgs[r.nimg]
	if dst.Rect != b || cap(dst.Pix) < 4*b.Dx()*b.Dy() {
		dst = image.NewNRGBA(b)
		r.imgs[r.nimg] = dst
	}
	draw.Draw(dst, b, img, b.Min, draw.Src)
	r.nimg++
	return r.nimg - 1
}

// Polyline records a stroked polyline. See [Backend.Polyline].
func (r *Recorder) Polyline(pts []Point, style Stroke) {
	lo, hi := r.points(pts)
	r.calls = append(r.calls, recorded{kind: recPolyline, lo: lo, hi: hi, stroke: style})
}

// StrokePath records a stroked path. See [Backend.StrokePath].
func (r *Recorder) StrokePath(p *Path, style Stroke) {
	oplo, ophi, ptlo, pthi := r.path(p)
	r.calls = append(r.calls, recorded{
		kind: recStrokePath, oplo: oplo, ophi: ophi, ptlo: ptlo, pthi: pthi, stroke: style,
	})
}

// FillPath records a filled path. See [Backend.FillPath].
func (r *Recorder) FillPath(p *Path, fill Fill, rule FillRule) {
	oplo, ophi, ptlo, pthi := r.path(p)
	r.calls = append(r.calls, recorded{
		kind: recFillPath, oplo: oplo, ophi: ophi, ptlo: ptlo, pthi: pthi, fill: fill, rule: rule,
	})
}

// Text records a text run. See [Backend.Text].
func (r *Recorder) Text(run TextRun) {
	r.calls = append(r.calls, recorded{kind: recText, text: run})
}

// Markers records a set of markers. See [Backend.Markers].
func (r *Recorder) Markers(shape Marker, at []Point, style MarkerStyle) {
	lo, hi := r.points(at)
	r.calls = append(r.calls, recorded{kind: recMarkers, lo: lo, hi: hi, marker: shape, style: style})
}

// Image records an image, copying its pixels. See [Backend.Image].
func (r *Recorder) Image(img image.Image, dst Rect) {
	if img == nil {
		return
	}
	r.calls = append(r.calls, recorded{kind: recImage, img: r.keepImage(img), rect: dst})
}

// Push records a transform and clip. See [Backend.Push].
func (r *Recorder) Push(clip *Path, xform Affine) {
	oplo, ophi, ptlo, pthi := r.path(clip)
	r.calls = append(r.calls, recorded{
		kind: recPush, oplo: oplo, ophi: ophi, ptlo: ptlo, pthi: pthi,
		xform: xform, clip: clip != nil,
	})
}

// Pop records the end of a Push. See [Backend.Pop].
func (r *Recorder) Pop() { r.calls = append(r.calls, recorded{kind: recPop}) }

// Measure forwards to the Measurer the Recorder was built with.
func (r *Recorder) Measure(run TextRun) TextMetrics {
	if r.m == nil {
		return TextMetrics{}
	}
	return r.m.Measure(run)
}

// Flush completes nothing: a Recorder holds a recording rather than a frame.
// The recording is finished by [Recorder.Replay].
func (r *Recorder) Flush() error { return nil }

// Replay makes the recorded calls on b, in the order they were made.
func (r *Recorder) Replay(b Backend) {
	// One path header, re-pointed at each call's slice of the arenas: the
	// arenas already hold the data, so replaying allocates nothing.
	var p Path
	for _, c := range r.calls {
		p.Ops, p.Pts = r.ops[c.oplo:c.ophi], r.ppts[c.ptlo:c.pthi]
		switch c.kind {
		case recPolyline:
			b.Polyline(r.pts[c.lo:c.hi], c.stroke)
		case recStrokePath:
			b.StrokePath(&p, c.stroke)
		case recFillPath:
			b.FillPath(&p, c.fill, c.rule)
		case recText:
			b.Text(c.text)
		case recMarkers:
			b.Markers(c.marker, r.pts[c.lo:c.hi], c.style)
		case recImage:
			b.Image(r.imgs[c.img], c.rect)
		case recPush:
			if c.clip {
				b.Push(&p, c.xform)
			} else {
				b.Push(nil, c.xform)
			}
		case recPop:
			b.Pop()
		}
	}
}

var (
	_ Backend  = (*Recorder)(nil)
	_ Measurer = (*Recorder)(nil)
)
