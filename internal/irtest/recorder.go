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
	"image/draw"
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

	// Damaged is what each frame was told to repaint, one entry per call to
	// [Recorder.Damage], and Whole says which of those meant the whole frame.
	// Frames counts the frames flushed.
	Damaged [][]ir.Rect
	Whole   []bool
	Frames  int
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
	// Snapshot rather than keep the reference. A geom that rasterizes draws
	// into pooled pixels and hands them over for the length of the call, the
	// same as it hands over a point slice — so a recording that kept the
	// original would show the next frame's image, not this one's.
	r.add(Call{Op: "Image", Image: snapshot(img), Rect: dst})
}

func snapshot(img image.Image) image.Image {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	out := image.NewNRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)
	return out
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

// Trace renders the recording one line per call, with the geometry and the
// style spelled out.
//
// It is [Recorder.String] at the detail a comparison needs: two charts that
// produce the same trace are the same chart, which is how a spec round trip is
// checked without a golden file. Coordinates are printed to three decimals,
// the same precision the SVG emitter writes.
func (r *Recorder) Trace() []string {
	out := make([]string, 0, len(r.Calls))
	for _, c := range r.Calls {
		var b strings.Builder
		b.WriteString(c.Op)
		switch c.Op {
		case "Text":
			fmt.Fprintf(&b, " %q at %s font=%v/%.3f h=%d v=%d rot=%.4f %s",
				c.Text.Text, pointStr(c.Text.At), c.Text.Font.Family, c.Text.Font.Size,
				c.Text.H, c.Text.V, c.Text.Rotation, colorStr(c.Text.Color))
		case "Polyline":
			fmt.Fprintf(&b, " %s %s", pointsStr(c.Points), strokeStr(c.Stroke))
		case "Markers":
			fmt.Fprintf(&b, " shape=%d %s size=%.3f fill=%s %s",
				c.Marker, pointsStr(c.Points), c.Style.Size, colorStr(c.Style.Fill), strokeStr(c.Style.Stroke))
		case "StrokePath":
			fmt.Fprintf(&b, " %s %s", pathStr(c.Path), strokeStr(c.Stroke))
		case "FillPath":
			fmt.Fprintf(&b, " %s rule=%d %s", pathStr(c.Path), c.Rule, fillStr(c.Fill))
		case "Image":
			fmt.Fprintf(&b, " %s", rectStr(c.Rect))
		case "Push":
			fmt.Fprintf(&b, " %v clip=%v", c.Affine, c.HasClip)
		}
		out = append(out, b.String())
	}
	return out
}

func pointStr(p ir.Point) string { return fmt.Sprintf("(%.3f,%.3f)", p.X, p.Y) }

func rectStr(r ir.Rect) string { return pointStr(r.Min) + "-" + pointStr(r.Max) }

func pointsStr(pts []ir.Point) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%d]", len(pts))
	for _, p := range pts {
		b.WriteString(pointStr(p))
	}
	return b.String()
}

func pathStr(p *ir.Path) string {
	if p == nil {
		return "<nil>"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "ops=%v", p.Ops)
	b.WriteString(pointsStr(p.Pts))
	return b.String()
}

func colorStr(c ir.Color) string {
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

func strokeStr(s ir.Stroke) string {
	return fmt.Sprintf("stroke=%s w=%.3f cap=%d join=%d dash=%v/%0.3f",
		colorStr(s.Color), s.Width, s.Cap, s.Join, s.Dash, s.DashOffset)
}

func fillStr(f ir.Fill) string {
	return fmt.Sprintf("fill=%s stops=%v %s%s", colorStr(f.Color), f.Stops,
		pointStr(f.Start), pointStr(f.End))
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

// Reset clears the recording, keeping the memory. A test driving several
// frames through one Recorder calls it between them.
func (r *Recorder) Reset() {
	r.Calls = r.Calls[:0]
	r.Depth, r.MaxDepth = 0, 0
}

// Damage implements [ir.Partial]: it records what a frame was told to repaint
// rather than repainting anything.
//
// It is how a test checks damage tracking end to end — that a chart whose data
// moved repaints where it moved and not the whole canvas.
func (r *Recorder) Damage(rects []ir.Rect) {
	r.Damaged = append(r.Damaged, append([]ir.Rect(nil), rects...))
	r.Whole = append(r.Whole, rects == nil)
}

// Flushes counts the frames completed on this Recorder.
func (r *Recorder) Flush() error { r.Frames++; return nil }

// Target returns an [ir.Target] handing out r, so that a test can drive
// [github.com/timzifer/refract.Plot.Render] or Plot.Live through a recorder.
// Closing it does nothing: a recorder has nothing to finalise.
func (r *Recorder) Target() ir.Target { return recorderTarget{r} }

type recorderTarget struct{ r *Recorder }

func (t recorderTarget) Open(int, int, float64) (ir.Backend, error) { return t.r, nil }
func (recorderTarget) Close() error                                 { return nil }

var (
	_ ir.Backend = (*Recorder)(nil)
	_ ir.Partial = (*Recorder)(nil)
	_ ir.Target  = recorderTarget{}
)

// NullBackend returns an ir.Backend that draws nothing and remembers nothing,
// measuring with the same built-in table [Recorder] uses.
//
// It is what a benchmark renders into when the thing being measured is
// refract's own work: with a real emitter most of a frame's time and memory
// belongs to the emitter, and a gate on refract's allocations would be reading
// someone else's.
func NullBackend() ir.Backend { return null{} }

// NullTarget returns an ir.Target handing out [NullBackend].
func NullTarget() ir.Target { return nullTarget{} }

type null struct{}

func (null) Polyline([]ir.Point, ir.Stroke)                {}
func (null) StrokePath(*ir.Path, ir.Stroke)                {}
func (null) FillPath(*ir.Path, ir.Fill, ir.FillRule)       {}
func (null) Text(ir.TextRun)                               {}
func (null) Markers(ir.Marker, []ir.Point, ir.MarkerStyle) {}
func (null) Image(image.Image, ir.Rect)                    {}
func (null) Push(*ir.Path, ir.Affine)                      {}
func (null) Pop()                                          {}
func (null) Flush() error                                  { return nil }

func (null) Measure(run ir.TextRun) ir.TextMetrics { return (&Recorder{}).Measure(run) }

type nullTarget struct{}

func (nullTarget) Open(int, int, float64) (ir.Backend, error) { return null{}, nil }
func (nullTarget) Close() error                               { return nil }

var (
	_ ir.Backend = null{}
	_ ir.Target  = nullTarget{}
)
