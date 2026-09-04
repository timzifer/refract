package pdf

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"

	"github.com/timzifer/refract/internal/fontmetrics"
	"github.com/timzifer/refract/internal/markers"
	"github.com/timzifer/refract/ir"
)

type backend struct {
	opts options
	w, h float64

	doc     document
	content bytes.Buffer

	// Interned resources. Each map keys the thing that made a resource
	// unique; the parallel slice keeps the order they were created in, so the
	// resource dictionary comes out the same on every run.
	fonts   map[string]string
	gstates map[string]string
	res     map[string][]resource

	faces map[faceKey]fontmetrics.Face

	desc ir.Description

	rootRef int
	infoRef int

	depth int
	err   error
}

// resource is one entry of a resource sub-dictionary: the name the content
// stream refers to, and the object it points at.
type resource struct {
	name string
	obj  int
}

type faceKey struct {
	size   float64
	weight int
	italic bool
}

func newBackend(w, h int, o options) *backend {
	return &backend{
		opts:    o,
		w:       float64(w),
		h:       float64(h),
		fonts:   map[string]string{},
		gstates: map[string]string{},
		res:     map[string][]resource{},
		faces:   map[faceKey]fontmetrics.Face{},
	}
}

// --- content-stream helpers ----------------------------------------------

func (b *backend) op(format string, args ...any) {
	fmt.Fprintf(&b.content, format, args...)
	b.content.WriteByte('\n')
}

// path emits a path's construction operators. It does not paint it.
func (b *backend) path(p *ir.Path) {
	p.Walk(func(op ir.PathOp, pts []ir.Point) {
		switch op {
		case ir.OpMoveTo:
			nums(&b.content, float64(pts[0].X), float64(pts[0].Y))
			b.content.WriteString(" m\n")
		case ir.OpLineTo:
			nums(&b.content, float64(pts[0].X), float64(pts[0].Y))
			b.content.WriteString(" l\n")
		case ir.OpCubicTo:
			nums(&b.content,
				float64(pts[0].X), float64(pts[0].Y),
				float64(pts[1].X), float64(pts[1].Y),
				float64(pts[2].X), float64(pts[2].Y))
			b.content.WriteString(" c\n")
		case ir.OpClose:
			b.content.WriteString("h\n")
		}
	})
}

// colorOp writes a colour-setting operator: "rg" for fill, "RG" for stroke.
func (b *backend) colorOp(c ir.Color, op string) {
	nums(&b.content, float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
	b.content.WriteByte(' ')
	b.content.WriteString(op)
	b.content.WriteByte('\n')
}

// alpha selects a graphics state for the given fill and stroke alphas, or does
// nothing when both are opaque.
//
// PDF has no per-operation alpha; constant alpha lives in an ExtGState
// resource. Interning one per distinct pair keeps a chart with one faded area
// to a single extra object.
func (b *backend) alpha(fill, stroke uint8) {
	if fill == 255 && stroke == 255 {
		return
	}
	key := fmt.Sprintf("%d/%d", fill, stroke)
	name, ok := b.gstates[key]
	if !ok {
		var d bytes.Buffer
		d.WriteString("<< /Type /ExtGState /ca ")
		num(&d, float64(fill)/255)
		d.WriteString(" /CA ")
		num(&d, float64(stroke)/255)
		d.WriteString(" >>")
		n := b.doc.add(d.Bytes())
		name = "G" + strconv.Itoa(n)
		b.gstates[key] = name
		b.intern("ExtGState", name, n)
	}
	b.op("/%s gs", name)
}

func (b *backend) strokeState(s ir.Stroke) {
	b.colorOp(s.Color, "RG")
	num(&b.content, float64(s.Width))
	b.content.WriteString(" w\n")
	b.op("%d J", capOf(s.Cap))
	b.op("%d j", joinOf(s.Join))
	limit := float64(s.MiterLimit)
	if limit <= 0 {
		limit = 4
	}
	num(&b.content, limit)
	b.content.WriteString(" M\n")
	b.content.WriteByte('[')
	for i, d := range s.Dash {
		if i > 0 {
			b.content.WriteByte(' ')
		}
		num(&b.content, float64(d))
	}
	b.content.WriteString("] ")
	num(&b.content, float64(s.DashOffset))
	b.content.WriteString(" d\n")
}

func capOf(c ir.LineCap) int {
	switch c {
	case ir.CapRound:
		return 1
	case ir.CapSquare:
		return 2
	default:
		return 0
	}
}

func joinOf(j ir.LineJoin) int {
	switch j {
	case ir.JoinRound:
		return 1
	case ir.JoinBevel:
		return 2
	default:
		return 0
	}
}

// --- ir.Backend ----------------------------------------------------------

func (b *backend) Polyline(pts []ir.Point, style ir.Stroke) {
	if !style.Visible() || len(pts) < 2 {
		return
	}
	b.op("q")
	b.alpha(255, style.Color.A)
	b.strokeState(style)
	nums(&b.content, float64(pts[0].X), float64(pts[0].Y))
	b.content.WriteString(" m\n")
	for _, p := range pts[1:] {
		nums(&b.content, float64(p.X), float64(p.Y))
		b.content.WriteString(" l\n")
	}
	b.op("S")
	b.op("Q")
}

func (b *backend) StrokePath(p *ir.Path, style ir.Stroke) {
	if !style.Visible() || p == nil || p.Empty() {
		return
	}
	b.op("q")
	b.alpha(255, style.Color.A)
	b.strokeState(style)
	b.path(p)
	b.op("S")
	b.op("Q")
}

func (b *backend) FillPath(p *ir.Path, fill ir.Fill, rule ir.FillRule) {
	if p == nil || p.Empty() || !fill.Visible() {
		return
	}
	if fill.IsGradient() {
		b.fillGradient(p, fill, rule)
		return
	}
	b.op("q")
	b.alpha(fill.Color.A, 255)
	b.colorOp(fill.Color, "rg")
	b.path(p)
	if rule == ir.EvenOdd {
		b.op("f*")
	} else {
		b.op("f")
	}
	b.op("Q")
}

// fillGradient paints an axial shading through the path used as a clip.
//
// PDF's `sh` operator paints a shading across the whole clip region, so the
// path becomes the clip rather than the thing being filled. That is the
// standard construction and it is why a gradient fill costs a q/Q pair.
func (b *backend) fillGradient(p *ir.Path, fill ir.Fill, rule ir.FillRule) {
	name := b.shading(fill)
	if name == "" {
		return
	}
	b.op("q")
	b.alpha(gradientAlpha(fill), 255)
	b.path(p)
	if rule == ir.EvenOdd {
		b.op("W* n")
	} else {
		b.op("W n")
	}
	b.op("/%s sh", name)
	b.op("Q")
}

// gradientAlpha reports the constant alpha to paint a gradient at.
//
// PDF expresses a shading's transparency through a soft mask, which is a whole
// transparency group. refract's gradients come from colour ramps whose stops
// share an alpha, so the constant case is the only one worth carrying: when
// the stops disagree, the strongest wins and the difference is documented
// rather than silently approximated per stop.
func gradientAlpha(f ir.Fill) uint8 {
	var a uint8
	for _, s := range f.Stops {
		if s.Color.A > a {
			a = s.Color.A
		}
	}
	return a
}

// shading interns an axial shading object and returns its resource name.
func (b *backend) shading(f ir.Fill) string {
	stops := normalizedStops(f.Stops)
	if len(stops) < 2 {
		return ""
	}

	// The colour function is a stitch of one linear ramp per adjacent pair of
	// stops. PDF has no notion of a multi-stop gradient; this is how one is
	// spelled.
	var fn bytes.Buffer
	if len(stops) == 2 {
		fn.WriteString(expFunction(stops[0].Color, stops[1].Color))
	} else {
		var subs, bounds, encode bytes.Buffer
		for i := 0; i+1 < len(stops); i++ {
			if i > 0 {
				subs.WriteByte(' ')
				encode.WriteByte(' ')
			}
			subs.WriteString(expFunction(stops[i].Color, stops[i+1].Color))
			encode.WriteString("0 1")
		}
		// Bounds are the interior stop offsets: one fewer than the
		// sub-functions, and strictly increasing.
		for i := 1; i+1 < len(stops); i++ {
			if i > 1 {
				bounds.WriteByte(' ')
			}
			num(&bounds, float64(stops[i].Offset))
		}
		fn.WriteString("<< /FunctionType 3 /Domain [0 1] /Functions [")
		fn.Write(subs.Bytes())
		fn.WriteString("] /Bounds [")
		fn.Write(bounds.Bytes())
		fn.WriteString("] /Encode [")
		fn.Write(encode.Bytes())
		fn.WriteString("] >>")
	}

	var d bytes.Buffer
	d.WriteString("<< /ShadingType 2 /ColorSpace /DeviceRGB /Coords [")
	nums(&d, float64(f.Start.X), float64(f.Start.Y), float64(f.End.X), float64(f.End.Y))
	d.WriteString("] /Extend [true true] /Function ")
	d.Write(fn.Bytes())
	d.WriteString(" >>")

	n := b.doc.add(d.Bytes())
	name := "Sh" + strconv.Itoa(n)
	b.intern("Shading", name, n)
	return name
}

// intern records a resource under a sub-dictionary of the page's /Resources.
func (b *backend) intern(kind, name string, obj int) {
	b.res[kind] = append(b.res[kind], resource{name: name, obj: obj})
}

func expFunction(a, c ir.Color) string {
	var b bytes.Buffer
	b.WriteString("<< /FunctionType 2 /Domain [0 1] /N 1 /C0 [")
	nums(&b, float64(a.R)/255, float64(a.G)/255, float64(a.B)/255)
	b.WriteString("] /C1 [")
	nums(&b, float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
	b.WriteString("] >>")
	return b.String()
}

// normalizedStops sorts the stops, clamps their offsets into [0, 1] and drops
// duplicates, which is what a stitching function's Bounds array requires:
// strictly increasing interior bounds.
func normalizedStops(in []ir.GradientStop) []ir.GradientStop {
	out := make([]ir.GradientStop, len(in))
	copy(out, in)
	for i := range out {
		switch {
		case out[i].Offset < 0 || math.IsNaN(float64(out[i].Offset)):
			out[i].Offset = 0
		case out[i].Offset > 1:
			out[i].Offset = 1
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Offset < out[j].Offset })
	kept := out[:0]
	for i, s := range out {
		// Keep the first and last unconditionally; drop an interior stop that
		// lands on the previous one, which would make a zero-width band.
		if i > 0 && i < len(out)-1 && s.Offset <= kept[len(kept)-1].Offset {
			continue
		}
		kept = append(kept, s)
	}
	if len(kept) >= 2 {
		kept[0].Offset, kept[len(kept)-1].Offset = 0, 1
	}
	return kept
}

func (b *backend) Text(run ir.TextRun) {
	if run.Text == "" || run.Color.A == 0 {
		return
	}
	size := run.Font.Size
	if size <= 0 {
		size = 12
	}
	name := b.font(baseFont(run.Font.Weight, run.Font.Italic))

	f := b.face(run.Font)
	dx, dy := textOffset(run, f)

	// The text matrix carries the anchor and undoes the page's Y flip for the
	// glyphs, so they read upright while every other coordinate stays in
	// refract's top-left space. Rotation composes into the same matrix.
	s, c := math.Sincos(run.Rotation)
	b.op("q")
	b.alpha(run.Color.A, 255)
	b.colorOp(run.Color, "rg")
	b.op("BT")
	b.content.WriteByte('/')
	b.content.WriteString(name)
	b.content.WriteByte(' ')
	num(&b.content, size)
	b.content.WriteString(" Tf\n")
	nums(&b.content, c, s, s, -c, float64(run.At.X), float64(run.At.Y))
	b.content.WriteString(" Tm\n")
	if dx != 0 || dy != 0 {
		nums(&b.content, dx, dy)
		b.content.WriteString(" Td\n")
	}
	writeString(&b.content, run.Text)
	b.content.WriteString(" Tj\n")
	b.op("ET")
	b.op("Q")
}

// textOffset converts refract's two-axis alignment into an offset in text
// space, where Y runs up.
//
// It anchors on the font box rather than on this run's ink, so a column of
// numbers lines up whether or not any of them has a descender — the same
// choice the gg backend makes for the same reason.
func textOffset(run ir.TextRun, f fontmetrics.Face) (dx, dy float64) {
	switch run.H {
	case ir.AlignCenter:
		dx = -f.Advance(run.Text) / 2
	case ir.AlignEnd:
		dx = -f.Advance(run.Text)
	}
	switch run.V {
	case ir.AlignTop:
		dy = -f.Ascent()
	case ir.AlignMiddle:
		dy = -(f.Ascent() - f.Descent()) / 2
	case ir.AlignBottom:
		dy = f.Descent()
	}
	return dx, dy
}

func (b *backend) font(base string) string {
	if name, ok := b.fonts[base]; ok {
		return name
	}
	n := b.doc.add([]byte(
		"<< /Type /Font /Subtype /Type1 /BaseFont /" + base + " /Encoding /WinAnsiEncoding >>"))
	name := "F" + strconv.Itoa(n)
	b.fonts[base] = name
	b.intern("Font", name, n)
	return name
}

func (b *backend) Markers(shape ir.Marker, at []ir.Point, style ir.MarkerStyle) {
	if len(at) == 0 || style.Size <= 0 {
		return
	}
	hasFill := style.Fill.A != 0
	hasStroke := style.Stroke.Visible()
	if !hasFill && !hasStroke {
		return
	}

	// One shape, translated per instance. PDF has form XObjects, but a marker
	// is a dozen operators and a Do costs a resource lookup either way.
	var proto ir.Path
	markers.Path(&proto, shape, style.Size)

	b.op("q")
	b.alpha(alphaOf(hasFill, style.Fill), alphaOf(hasStroke, style.Stroke.Color))
	if hasFill {
		b.colorOp(style.Fill, "rg")
	}
	if hasStroke {
		b.strokeState(style.Stroke)
	}

	var moved ir.Path
	for _, p := range at {
		moved.Reset()
		moved.Ops = append(moved.Ops, proto.Ops...)
		for _, q := range proto.Pts {
			moved.Pts = append(moved.Pts, ir.Point{X: q.X + p.X, Y: q.Y + p.Y})
		}
		b.path(&moved)
		switch {
		case hasFill && hasStroke:
			b.op("B")
		case hasFill:
			b.op("f")
		default:
			b.op("S")
		}
	}
	b.op("Q")
}

func alphaOf(used bool, c ir.Color) uint8 {
	if !used {
		return 255
	}
	return c.A
}

func (b *backend) Image(img image.Image, dst ir.Rect) {
	if img == nil || dst.Empty() {
		return
	}
	n, err := b.imageObject(img)
	if err != nil {
		b.fail(err)
		return
	}
	name := "Im" + strconv.Itoa(n)
	b.intern("XObject", name, n)

	// A PDF image fills the unit square with its first row at the top of that
	// square. The page is already Y-flipped, so the placement matrix flips
	// back, which puts row zero at the top of dst as the IR intends.
	b.op("q")
	nums(&b.content, float64(dst.Dx()), 0, 0, -float64(dst.Dy()),
		float64(dst.Min.X), float64(dst.Max.Y))
	b.content.WriteString(" cm\n")
	b.op("/%s Do", name)
	b.op("Q")
}

// imageObject writes the image as a Flate-compressed RGB XObject, with a soft
// mask when it is not fully opaque.
func (b *backend) imageObject(img image.Image) (int, error) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return 0, errors.New("refract/backend/pdf: empty image")
	}
	rgb := make([]byte, 0, w*h*3)
	alpha := make([]byte, 0, w*h)
	opaque := true
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			rgb = append(rgb, c.R, c.G, c.B)
			alpha = append(alpha, c.A)
			if c.A != 255 {
				opaque = false
			}
		}
	}

	dict := fmt.Sprintf(
		"/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8",
		w, h)
	if !opaque {
		mask := b.doc.addStream(fmt.Sprintf(
			"/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8",
			w, h), alpha, b.opts.compress)
		dict += fmt.Sprintf(" /SMask %d 0 R", mask)
	}
	return b.doc.addStream(dict, rgb, b.opts.compress), nil
}

func (b *backend) Push(clip *ir.Path, xform ir.Affine) {
	b.op("q")
	b.depth++
	if !xform.IsIdentity() {
		nums(&b.content, float64(xform.A), float64(xform.B),
			float64(xform.C), float64(xform.D), float64(xform.E), float64(xform.F))
		b.content.WriteString(" cm\n")
	}
	if clip != nil && !clip.Empty() {
		b.path(clip)
		b.op("W n")
	}
}

func (b *backend) Pop() {
	if b.depth == 0 {
		b.fail(errors.New("refract/backend/pdf: Pop without matching Push"))
		return
	}
	b.depth--
	b.op("Q")
}

// --- measurement ---------------------------------------------------------

func (b *backend) Measure(run ir.TextRun) ir.TextMetrics {
	f := b.face(run.Font)
	adv := f.Advance(run.Text)
	asc, desc := f.Ascent(), f.Descent()
	return ir.TextMetrics{
		Advance: float32(adv),
		Ascent:  float32(asc),
		Descent: float32(desc),
		// No outlines are available, so the font box stands in for the ink
		// box. Layout uses Ink only for collision padding, where erring large
		// is the safe direction.
		Ink: ir.R(0, float32(-asc), float32(adv), float32(desc)),
	}
}

func (b *backend) face(f ir.FontRef) fontmetrics.Face {
	size := f.Size
	if size <= 0 {
		size = 12
	}
	k := faceKey{size: size, weight: f.Weight, italic: f.Italic}
	if got, ok := b.faces[k]; ok {
		return got
	}
	face := fontmetrics.Builtin(size, f.Weight, f.Italic)
	b.faces[k] = face
	return face
}

// Describe records what the document says about itself, which a PDF carries in
// its information dictionary: the title names the file in a reader's window and
// its document properties, and the long reading goes in the subject, which is
// the only field a PDF has for a paragraph.
//
// A title given to the target with [WithTitle] wins: that one names the
// document, and this one describes the chart in it.
func (b *backend) Describe(d ir.Description) { b.desc = d }

// --- finishing -----------------------------------------------------------

func (b *backend) Flush() error {
	if b.err != nil {
		return b.err
	}
	if b.depth != 0 {
		return fmt.Errorf("refract/backend/pdf: %d unclosed graphics state(s) at Flush", b.depth)
	}

	pages := b.doc.reserve()
	page := b.doc.reserve()

	// The page opens by flipping Y, so refract's top-left coordinates are
	// written into the content stream unchanged.
	var body bytes.Buffer
	body.WriteString("1 0 0 -1 0 ")
	num(&body, b.h)
	body.WriteString(" cm\n")
	body.Write(b.content.Bytes())
	contents := b.doc.addStream("", body.Bytes(), b.opts.compress)

	b.doc.set(page, []byte(fmt.Sprintf(
		"<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources %s /Contents %d 0 R >>",
		pages, fmtNum(b.w), fmtNum(b.h), b.resources(), contents)))
	b.doc.set(pages, []byte(fmt.Sprintf(
		"<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", page)))

	b.rootRef = b.doc.add([]byte(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages)))
	b.infoRef = b.info()
	return nil
}

// info writes the document information dictionary, or returns 0 when no
// metadata was supplied.
//
// It carries no creation date on purpose: a timestamp would make two renders
// of the same chart differ, and the golden tests exist precisely to notice
// when output changes.
func (b *backend) info() int {
	o := b.opts
	if o.title == "" {
		o.title = b.desc.Title
	}
	if o.subject == "" {
		o.subject = b.desc.Detail
	}
	if o.title == "" && o.author == "" && o.subject == "" {
		return 0
	}
	var d bytes.Buffer
	d.WriteString("<< ")
	for _, e := range []struct{ key, val string }{
		{"Title", o.title},
		{"Author", o.author},
		{"Subject", o.subject},
		{"Producer", "refract"},
	} {
		if e.val == "" {
			continue
		}
		d.WriteString("/")
		d.WriteString(e.key)
		d.WriteByte(' ')
		writeString(&d, e.val)
		d.WriteByte(' ')
	}
	d.WriteString(">>")
	return b.doc.add(d.Bytes())
}

// resourceKinds is the order the resource sub-dictionaries are written in.
// Fixing it keeps the output byte-stable; a map range would not.
var resourceKinds = [...]string{"Font", "ExtGState", "Shading", "XObject"}

func (b *backend) resources() string {
	var r bytes.Buffer
	r.WriteString("<< /ProcSet [/PDF /Text /ImageC]")
	for _, kind := range resourceKinds {
		entries := b.res[kind]
		if len(entries) == 0 {
			continue
		}
		fmt.Fprintf(&r, " /%s << ", kind)
		for _, e := range entries {
			fmt.Fprintf(&r, "/%s %d 0 R ", e.name, e.obj)
		}
		r.WriteString(">>")
	}
	r.WriteString(" >>")
	return r.String()
}

func fmtNum(v float64) string {
	var b bytes.Buffer
	num(&b, v)
	return b.String()
}

func (b *backend) fail(err error) {
	if b.err == nil {
		b.err = err
	}
}
