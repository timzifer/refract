// Package irtest provides a recording ir.Backend for tests.
//
// It lets a test assert on what a geom or the renderer actually emitted —
// which primitives, in which order, with which style — instead of inspecting
// rendered pixels or scraped SVG. That keeps model-layer tests independent of
// any backend.
package irtest

import (
	"fmt"
	"image"
	"strings"

	"github.com/timzifer/refract/internal/fontmetrics"
	"github.com/timzifer/refract/ir"
)

// Call is one recorded drawing operation.
type Call struct {
	Op       string
	Points   []ir.Point
	Path     *ir.Path
	Stroke   ir.Stroke
	Fill     ir.Fill
	Rule     ir.FillRule
	Text     ir.TextRun
	Marker   ir.Marker
	Style    ir.MarkerStyle
	Image    image.Image
	Rect     ir.Rect
	Affine   ir.Affine
	HasClip  bool
	ClipRect ir.Rect
}

// Recorder is an ir.Backend that remembers every call.
//
// It measures with the same built-in metrics table the SVG backend falls back
// to, so layout in a test behaves like layout in a real stdlib-only render.
type Recorder struct {
	Calls []Call
	Depth int

	// MaxDepth is the deepest the Push/Pop stack ever got, which is how a test
	// checks that clipping was actually applied around the data layers.
	MaxDepth int
}

// New returns an empty Recorder.
func New() *Recorder { return &Recorder{} }

func (r *Recorder) add(c Call) { r.Calls = append(r.Calls, c) }

func (r *Recorder) Polyline(pts []ir.Point, style ir.Stroke) {
	r.add(Call{Op: "Polyline", Points: append([]ir.Point(nil), pts...), Stroke: style})
}

func (r *Recorder) StrokePath(p *ir.Path, style ir.Stroke) {
	r.add(Call{Op: "StrokePath", Path: clone(p), Stroke: style})
}

func (r *Recorder) FillPath(p *ir.Path, fill ir.Fill, rule ir.FillRule) {
	r.add(Call{Op: "FillPath", Path: clone(p), Fill: fill, Rule: rule})
}

func (r *Recorder) Text(run ir.TextRun) {
	r.add(Call{Op: "Text", Text: run})
}

func (r *Recorder) Markers(shape ir.Marker, at []ir.Point, style ir.MarkerStyle) {
	r.add(Call{Op: "Markers", Marker: shape, Points: append([]ir.Point(nil), at...), Style: style})
}

func (r *Recorder) Image(img image.Image, dst ir.Rect) {
	r.add(Call{Op: "Image", Image: img, Rect: dst})
}

func (r *Recorder) Push(clip *ir.Path, xform ir.Affine) {
	c := Call{Op: "Push", Affine: xform}
	if clip != nil && !clip.Empty() {
		c.HasClip = true
		c.ClipRect = clip.Bounds()
		c.Path = clone(clip)
	}
	r.add(c)
	r.Depth++
	if r.Depth > r.MaxDepth {
		r.MaxDepth = r.Depth
	}
}

func (r *Recorder) Pop() {
	r.add(Call{Op: "Pop"})
	r.Depth--
}

func (r *Recorder) Measure(run ir.TextRun) ir.TextMetrics {
	size := run.Font.Size
	if size <= 0 {
		size = 12
	}
	f := fontmetrics.Builtin(size, run.Font.Weight, run.Font.Italic)
	adv := float32(f.Advance(run.Text))
	asc, desc := float32(f.Ascent()), float32(f.Descent())
	return ir.TextMetrics{Advance: adv, Ascent: asc, Descent: desc, Ink: ir.R(0, -asc, adv, desc)}
}

func (r *Recorder) Flush() error { return nil }

// Ops returns the recorded operation names, for a quick order assertion.
func (r *Recorder) Ops() []string {
	out := make([]string, len(r.Calls))
	for i, c := range r.Calls {
		out[i] = c.Op
	}
	return out
}

// Count returns how many calls of the given op were recorded.
func (r *Recorder) Count(op string) int {
	n := 0
	for _, c := range r.Calls {
		if c.Op == op {
			n++
		}
	}
	return n
}

// Filter returns the recorded calls of the given op.
func (r *Recorder) Filter(op string) []Call {
	var out []Call
	for _, c := range r.Calls {
		if c.Op == op {
			out = append(out, c)
		}
	}
	return out
}

// Texts returns the strings of every recorded text run, in order.
func (r *Recorder) Texts() []string {
	var out []string
	for _, c := range r.Calls {
		if c.Op == "Text" {
			out = append(out, c.Text.Text)
		}
	}
	return out
}

// String renders the recording as a short, diffable trace.
func (r *Recorder) String() string {
	var b strings.Builder
	for _, c := range r.Calls {
		switch c.Op {
		case "Text":
			fmt.Fprintf(&b, "Text %q at (%.1f,%.1f)\n", c.Text.Text, c.Text.At.X, c.Text.At.Y)
		case "Polyline":
			fmt.Fprintf(&b, "Polyline %d points\n", len(c.Points))
		case "Markers":
			fmt.Fprintf(&b, "Markers shape=%d n=%d\n", c.Marker, len(c.Points))
		default:
			fmt.Fprintf(&b, "%s\n", c.Op)
		}
	}
	return b.String()
}

func clone(p *ir.Path) *ir.Path {
	if p == nil {
		return nil
	}
	return &ir.Path{
		Ops: append([]ir.PathOp(nil), p.Ops...),
		Pts: append([]ir.Point(nil), p.Pts...),
	}
}

var _ ir.Backend = (*Recorder)(nil)
