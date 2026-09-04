//go:build js && wasm

package canvas

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math"
	"strconv"
	"syscall/js"

	"github.com/timzifer/refract/internal/markers"
	"github.com/timzifer/refract/ir"
)

// coordDecimals is how precisely a coordinate is written into a path string.
// Three decimals is well below a pixel at any device scale, and it keeps the
// strings short enough that building them is cheaper than the JS call they
// save.
const coordDecimals = 3

// Option configures a target.
type Option func(*options)

type options struct {
	clear bool
}

// Clear controls whether the canvas is cleared before each frame. It is on by
// default. Turn it off to draw a chart over something already on the canvas —
// and remember that a theme with a transparent background then composites
// rather than replaces.
func Clear(on bool) Option { return func(o *options) { o.clear = on } }

// Element returns a target drawing into a canvas element.
//
// The element is sized for the chart and for the device pixel ratio: its
// backing store becomes width×dpr by height×dpr pixels and its CSS size stays
// width by height, which is what makes a chart sharp on a retina display
// without any coordinate in refract changing.
func Element(el js.Value, opts ...Option) ir.Target {
	return &target{el: el, opts: build(opts)}
}

// Context returns a target drawing into an existing 2D context.
//
// Nothing is resized: the caller owns the canvas, its pixel ratio and its
// transform. This is the form to use for a canvas shared with other drawing,
// or for an OffscreenCanvas on a worker.
func Context(ctx js.Value, opts ...Option) ir.Target {
	return &target{ctx: ctx, opts: build(opts)}
}

func build(opts []Option) options {
	o := options{clear: true}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

type target struct {
	el   js.Value
	ctx  js.Value
	opts options
	b    *backend
}

func (t *target) Open(widthPx, heightPx int, dpr float64) (ir.Backend, error) {
	if dpr <= 0 {
		dpr = 1
	}
	ctx := t.ctx
	if !t.el.IsUndefined() && !t.el.IsNull() {
		t.el.Set("width", int(float64(widthPx)*dpr+0.5))
		t.el.Set("height", int(float64(heightPx)*dpr+0.5))
		if style := t.el.Get("style"); style.Truthy() {
			style.Set("width", strconv.Itoa(widthPx)+"px")
			style.Set("height", strconv.Itoa(heightPx)+"px")
		}
		ctx = t.el.Call("getContext", "2d")
	}
	if !ctx.Truthy() {
		return nil, errors.New("refract/backend/canvas: no 2D context")
	}
	t.b = &backend{
		el: t.el, ctx: ctx, w: widthPx, h: heightPx, dpr: dpr,
		clear: t.opts.clear, path2d: js.Global().Get("Path2D"),
	}
	if !t.b.path2d.Truthy() {
		return nil, errors.New("refract/backend/canvas: this environment has no Path2D")
	}
	t.b.begin()
	return t.b, nil
}

// Close leaves the canvas as it is. A canvas is a surface rather than a
// document: there is nothing to finalise and nothing to write.
func (t *target) Close() error { return nil }

type backend struct {
	el     js.Value // the canvas element, undefined for a bare context
	ctx    js.Value
	path2d js.Value
	w, h   int
	dpr    float64
	clear  bool

	buf   []byte   // the path string being built, reused between calls
	args  []any    // one-call argument scratch
	depth int      // Push/Pop nesting, so Flush can unwind a damage clip
	dirty bool     // a damage clip is in force
	tmp   js.Value // the scratch canvas an Image is scaled through
}

// Describe implements [ir.Semantics]: it names the canvas for a screen reader.
//
// A <canvas> is a hole in the accessibility tree — it has pixels and no
// structure — so the only thing that can be said about it is said in its
// attributes: role="img" makes it one graphic rather than an unlabelled
// element, and aria-label is the text read out. The long reading is appended
// to the title, because an element has one label and a canvas has no children
// to hang a description on.
//
// A target built with [Context] has no element and nothing to label; the
// caller owns that canvas and its attributes.
func (b *backend) Describe(d ir.Description) {
	if b.el.IsUndefined() || b.el.IsNull() || d.Empty() {
		return
	}
	label := d.Title
	if d.Detail != "" {
		if label != "" {
			label += ". "
		}
		label += d.Detail
	}
	b.el.Call("setAttribute", "role", "img")
	b.el.Call("setAttribute", "aria-label", label)
}

// Resize implements [ir.Resizer]: it resizes the canvas the chart is drawn
// into, so that a reflowed layout redraws at the size it now has rather than
// being stretched by the browser.
//
// The backing store and the CSS size are set the way [Element] sets them. A
// target built with [Context] leaves both alone — the caller owns that canvas
// — and only records the new logical size.
func (b *backend) Resize(widthPx, heightPx int, dpr float64) error {
	if widthPx <= 0 || heightPx <= 0 {
		return fmt.Errorf("refract/backend/canvas: size %dx%d is not positive", widthPx, heightPx)
	}
	if dpr > 0 {
		b.dpr = dpr
	}
	b.w, b.h = widthPx, heightPx
	if !b.el.IsUndefined() && !b.el.IsNull() {
		b.el.Set("width", int(float64(widthPx)*b.dpr+0.5))
		b.el.Set("height", int(float64(heightPx)*b.dpr+0.5))
		if style := b.el.Get("style"); style.Truthy() {
			style.Set("width", strconv.Itoa(widthPx)+"px")
			style.Set("height", strconv.Itoa(heightPx)+"px")
		}
	}
	b.begin()
	return nil
}

// begin puts the context into the state every frame starts in: the identity
// transform, scaled for the device pixel ratio.
func (b *backend) begin() {
	b.ctx.Call("setTransform", b.dpr, 0, 0, b.dpr, 0, 0)
}

// Damage implements [ir.Partial]: it clips the frame to the given rectangles
// and clears them, so that only the part of the chart that changed is
// repainted.
//
// A nil list means the whole frame, which is what a first frame and a
// structural change both ask for.
func (b *backend) Damage(rects []ir.Rect) {
	b.begin()
	if rects == nil {
		if b.clear {
			b.ctx.Call("clearRect", 0, 0, b.w, b.h)
		}
		return
	}
	b.ctx.Call("save")
	b.dirty = true

	b.buf = b.buf[:0]
	for _, r := range rects {
		b.buf = appendRect(b.buf, r)
	}
	b.ctx.Call("clip", b.newPath(string(b.buf)))
	if b.clear {
		for _, r := range rects {
			b.ctx.Call("clearRect", f(r.Min.X), f(r.Min.Y), f(r.Dx()), f(r.Dy()))
		}
	}
}

func (b *backend) newPath(d string) js.Value { return b.path2d.New(d) }

func (b *backend) Polyline(pts []ir.Point, style ir.Stroke) {
	if len(pts) < 2 || !style.Visible() {
		return
	}
	b.buf = appendPolyline(b.buf[:0], pts)
	b.applyStroke(style)
	b.ctx.Call("stroke", b.newPath(string(b.buf)))
}

func (b *backend) StrokePath(p *ir.Path, style ir.Stroke) {
	if p == nil || p.Empty() || !style.Visible() {
		return
	}
	b.buf = appendPath(b.buf[:0], p)
	b.applyStroke(style)
	b.ctx.Call("stroke", b.newPath(string(b.buf)))
}

func (b *backend) FillPath(p *ir.Path, fill ir.Fill, rule ir.FillRule) {
	if p == nil || p.Empty() || !fill.Visible() {
		return
	}
	b.buf = appendPath(b.buf[:0], p)
	b.applyFill(fill)
	b.fill(b.newPath(string(b.buf)), rule)
}

func (b *backend) fill(path js.Value, rule ir.FillRule) {
	if rule == ir.EvenOdd {
		b.ctx.Call("fill", path, "evenodd")
		return
	}
	b.ctx.Call("fill", path)
}

// Markers draws every instance as one path. A scatter is one JS call per
// style, not one per point: crossing the WebAssembly boundary is the expensive
// part of drawing in a browser, and a thousand-point cloud that made a
// thousand calls would be slower than the rasterizer it replaced.
func (b *backend) Markers(shape ir.Marker, at []ir.Point, style ir.MarkerStyle) {
	if len(at) == 0 {
		return
	}
	var outline ir.Path
	markers.Path(&outline, shape, style.Size)

	b.buf = b.buf[:0]
	for _, c := range at {
		b.buf = appendPathAt(b.buf, &outline, c)
	}
	path := b.newPath(string(b.buf))
	if style.Fill.A != 0 {
		b.setFillColor(style.Fill)
		b.fill(path, ir.NonZero)
	}
	if style.Stroke.Visible() {
		b.applyStroke(style.Stroke)
		b.ctx.Call("stroke", path)
	}
}

func (b *backend) Text(run ir.TextRun) {
	if run.Text == "" || run.Color.A == 0 {
		return
	}
	b.applyFont(run)
	b.setFillColor(run.Color)
	if run.Rotation == 0 {
		b.ctx.Call("fillText", run.Text, f(run.At.X), f(run.At.Y))
		return
	}
	// A rotated run turns about its anchor, which is what the IR promises and
	// what a Y-axis title needs.
	b.ctx.Call("save")
	b.ctx.Call("translate", f(run.At.X), f(run.At.Y))
	b.ctx.Call("rotate", run.Rotation)
	b.ctx.Call("fillText", run.Text, 0, 0)
	b.ctx.Call("restore")
}

func (b *backend) applyFont(run ir.TextRun) {
	family := run.Font.Family
	if family == "" {
		family = "system-ui, sans-serif"
	}
	b.ctx.Set("font", trimNum(run.Font.Size)+"px "+family)
	switch run.H {
	case ir.AlignCenter:
		b.ctx.Set("textAlign", "center")
	case ir.AlignEnd:
		b.ctx.Set("textAlign", "right")
	default:
		b.ctx.Set("textAlign", "left")
	}
	switch run.V {
	case ir.AlignTop:
		b.ctx.Set("textBaseline", "top")
	case ir.AlignMiddle:
		b.ctx.Set("textBaseline", "middle")
	case ir.AlignBottom:
		b.ctx.Set("textBaseline", "bottom")
	default:
		b.ctx.Set("textBaseline", "alphabetic")
	}
}

// Measure reports what the canvas will actually draw.
//
// This is the seam ADR 0003 describes: the backend shapes and refract places,
// and layout has to ask the very shaper that will draw. A browser's
// measureText is that shaper, so a chart's margins are sized by the font the
// reader has rather than by a table of what a font usually looks like.
func (b *backend) Measure(run ir.TextRun) ir.TextMetrics {
	b.applyFont(run)
	m := b.ctx.Call("measureText", run.Text)
	adv := float32(m.Get("width").Float())

	asc := metric(m, "fontBoundingBoxAscent")
	desc := metric(m, "fontBoundingBoxDescent")
	if asc == 0 && desc == 0 {
		// Safari did not report font box metrics for years, and a browser
		// engine somewhere still will not. The em box is the honest fallback:
		// it over-reports the ascent slightly, which spaces labels a little
		// generously rather than overlapping them.
		asc, desc = float32(run.Font.Size)*0.8, float32(run.Font.Size)*0.2
	}
	ink := ir.R(0, -metric(m, "actualBoundingBoxAscent"), adv, metric(m, "actualBoundingBoxDescent"))
	if ink.Empty() {
		ink = ir.R(0, -asc, adv, desc)
	}
	return ir.TextMetrics{Advance: adv, Ascent: asc, Descent: desc, Ink: ink}
}

func metric(m js.Value, name string) float32 {
	v := m.Get(name)
	if v.Type() != js.TypeNumber {
		return 0
	}
	return float32(v.Float())
}

// Image blits a raster through a scratch canvas, because putImageData does not
// scale and drawImage does.
func (b *backend) Image(img image.Image, dst ir.Rect) {
	if img == nil || dst.Empty() {
		return
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	nrgba, ok := img.(*image.NRGBA)
	if !ok || nrgba.Stride != 4*w {
		conv := image.NewNRGBA(image.Rect(0, 0, w, h))
		draw.Draw(conv, conv.Bounds(), img, bounds.Min, draw.Src)
		nrgba = conv
	}

	pix := js.Global().Get("Uint8ClampedArray").New(len(nrgba.Pix))
	js.CopyBytesToJS(pix, nrgba.Pix)
	data := js.Global().Get("ImageData").New(pix, w, h)

	tmp := b.scratch(w, h)
	if !tmp.Truthy() {
		return
	}
	tmp.Call("getContext", "2d").Call("putImageData", data, 0, 0)
	b.ctx.Call("drawImage", tmp, f(dst.Min.X), f(dst.Min.Y), f(dst.Dx()), f(dst.Dy()))
}

// scratch returns a canvas of the given size, reusing the last one when it
// already fits — a density raster is redrawn every frame and allocating a
// canvas per frame is how a live chart starts stuttering.
func (b *backend) scratch(w, h int) js.Value {
	if b.tmp.Truthy() && b.tmp.Get("width").Int() == w && b.tmp.Get("height").Int() == h {
		return b.tmp
	}
	if off := js.Global().Get("OffscreenCanvas"); off.Truthy() {
		b.tmp = off.New(w, h)
		return b.tmp
	}
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		return js.Undefined()
	}
	b.tmp = doc.Call("createElement", "canvas")
	b.tmp.Set("width", w)
	b.tmp.Set("height", h)
	return b.tmp
}

func (b *backend) Push(clip *ir.Path, xform ir.Affine) {
	b.ctx.Call("save")
	b.depth++
	if !xform.IsIdentity() {
		b.ctx.Call("transform", f(xform.A), f(xform.B), f(xform.C), f(xform.D), f(xform.E), f(xform.F))
	}
	if clip != nil && !clip.Empty() {
		b.buf = appendPath(b.buf[:0], clip)
		b.ctx.Call("clip", b.newPath(string(b.buf)))
	}
}

func (b *backend) Pop() {
	if b.depth == 0 {
		return
	}
	b.depth--
	b.ctx.Call("restore")
}

// Flush ends the frame. There is nothing to write — a canvas is already on
// screen — so this only unwinds the state the frame left behind, which is what
// makes the next frame start from a known one.
func (b *backend) Flush() error {
	for b.depth > 0 {
		b.Pop()
	}
	if b.dirty {
		b.ctx.Call("restore")
		b.dirty = false
	}
	return nil
}

func (b *backend) applyStroke(s ir.Stroke) {
	b.ctx.Set("strokeStyle", cssColor(s.Color))
	b.ctx.Set("lineWidth", s.Width)
	switch s.Cap {
	case ir.CapRound:
		b.ctx.Set("lineCap", "round")
	case ir.CapSquare:
		b.ctx.Set("lineCap", "square")
	default:
		b.ctx.Set("lineCap", "butt")
	}
	switch s.Join {
	case ir.JoinRound:
		b.ctx.Set("lineJoin", "round")
	case ir.JoinBevel:
		b.ctx.Set("lineJoin", "bevel")
	default:
		b.ctx.Set("lineJoin", "miter")
	}
	if s.MiterLimit > 0 {
		b.ctx.Set("miterLimit", s.MiterLimit)
	}
	b.args = b.args[:0]
	for _, d := range s.Dash {
		b.args = append(b.args, f(d))
	}
	b.ctx.Call("setLineDash", js.ValueOf(b.args))
	b.ctx.Set("lineDashOffset", f(s.DashOffset))
}

func (b *backend) applyFill(fill ir.Fill) {
	if !fill.IsGradient() {
		b.setFillColor(fill.Color)
		return
	}
	g := b.ctx.Call("createLinearGradient",
		f(fill.Start.X), f(fill.Start.Y), f(fill.End.X), f(fill.End.Y))
	for _, s := range fill.Stops {
		g.Call("addColorStop", f(s.Offset), cssColor(s.Color))
	}
	b.ctx.Set("fillStyle", g)
}

func (b *backend) setFillColor(c ir.Color) { b.ctx.Set("fillStyle", cssColor(c)) }

// cssColor writes a colour the way CSS wants it. Alpha is a fraction rather
// than a byte, which is the one place canvas and the IR disagree.
func cssColor(c ir.Color) string {
	if c.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return "rgba(" + strconv.Itoa(int(c.R)) + "," + strconv.Itoa(int(c.G)) + "," +
		strconv.Itoa(int(c.B)) + "," + trimNum(float64(c.A)/255) + ")"
}

// f converts a coordinate for a JS call. js.ValueOf handles float64 and not
// float32, and a silent boxing failure would be a blank chart.
func f(v float32) float64 {
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return 0
	}
	return float64(v)
}

// The path builders below produce SVG path data, which is what Path2D takes.
// One string per drawing call is one crossing of the WebAssembly boundary per
// drawing call, against one per point for the moveTo/lineTo form.

func appendPolyline(b []byte, pts []ir.Point) []byte {
	for i, p := range pts {
		if i == 0 {
			b = append(b, 'M')
		} else {
			b = append(b, 'L')
		}
		b = appendPoint(b, p)
	}
	return b
}

func appendPath(b []byte, p *ir.Path) []byte {
	return appendPathAt(b, p, ir.Point{})
}

// appendPathAt writes a path translated by at, which is how one marker outline
// becomes an instance at every position.
func appendPathAt(b []byte, p *ir.Path, at ir.Point) []byte {
	p.Walk(func(op ir.PathOp, pts []ir.Point) {
		switch op {
		case ir.OpMoveTo:
			b = append(b, 'M')
			b = appendPoint(b, offset(pts[0], at))
		case ir.OpLineTo:
			b = append(b, 'L')
			b = appendPoint(b, offset(pts[0], at))
		case ir.OpCubicTo:
			b = append(b, 'C')
			for i, q := range pts {
				if i > 0 {
					b = append(b, ' ')
				}
				b = appendPoint(b, offset(q, at))
			}
		case ir.OpClose:
			b = append(b, 'Z')
		}
	})
	return b
}

func appendRect(b []byte, r ir.Rect) []byte {
	b = append(b, 'M')
	b = appendPoint(b, r.Min)
	b = append(b, 'L')
	b = appendPoint(b, ir.Point{X: r.Max.X, Y: r.Min.Y})
	b = append(b, 'L')
	b = appendPoint(b, r.Max)
	b = append(b, 'L')
	b = appendPoint(b, ir.Point{X: r.Min.X, Y: r.Max.Y})
	return append(b, 'Z')
}

func offset(p, by ir.Point) ir.Point { return ir.Point{X: p.X + by.X, Y: p.Y + by.Y} }

func appendPoint(b []byte, p ir.Point) []byte {
	b = appendNum(b, float64(p.X))
	b = append(b, ' ')
	return appendNum(b, float64(p.Y))
}

func appendNum(b []byte, v float64) []byte {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return append(b, '0')
	}
	start := len(b)
	b = strconv.AppendFloat(b, v, 'f', coordDecimals, 64)
	return trimZeros(b, start)
}

// trimZeros drops a trailing zero run and normalises "-0" to "0", so that a
// path string is as short as it can be without losing a digit that matters.
func trimZeros(b []byte, start int) []byte {
	s := b[start:]
	dot := -1
	for i, c := range s {
		if c == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		return b
	}
	end := len(s)
	for end > dot && s[end-1] == '0' {
		end--
	}
	if end-1 == dot {
		end--
	}
	b = b[:start+end]
	if string(b[start:]) == "-0" {
		b = append(b[:start], '0')
	}
	return b
}

func trimNum(v float64) string { return string(appendNum(nil, v)) }

var (
	_ ir.Backend = (*backend)(nil)
	_ ir.Partial = (*backend)(nil)
	_ ir.Target  = (*target)(nil)
)
