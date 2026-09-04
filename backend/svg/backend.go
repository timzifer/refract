package svg

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"strconv"

	"github.com/timzifer/refract/internal/fontmetrics"
	"github.com/timzifer/refract/ir"
)

type backend struct {
	out  io.Writer
	opts options

	w, h int
	dpr  float64

	defs bytes.Buffer
	body bytes.Buffer

	depth   int // open <g> elements from Push
	nextID  int
	symbols map[string]string // marker style key -> symbol id
	faces   map[faceKey]fontmetrics.Face

	err error
}

type faceKey struct {
	size   float64
	weight int
	italic bool
}

func newBackend(out io.Writer, w, h int, dpr float64, o options) *backend {
	return &backend{
		out:     out,
		opts:    o,
		w:       w,
		h:       h,
		dpr:     dpr,
		symbols: map[string]string{},
		faces:   map[faceKey]fontmetrics.Face{},
	}
}

func (b *backend) id(prefix string) string {
	b.nextID++
	return prefix + strconv.Itoa(b.nextID)
}

// nl writes a newline in pretty mode and nothing otherwise, so the compact and
// pretty outputs differ only in whitespace.
func (b *backend) nl(w *bytes.Buffer) {
	if b.opts.pretty {
		w.WriteByte('\n')
	}
}

// --- drawing -------------------------------------------------------------

func (b *backend) Polyline(pts []ir.Point, style ir.Stroke) {
	if !style.Visible() || len(pts) < 2 {
		return
	}
	b.body.WriteString(`<polyline fill="none" points="`)
	for i, p := range pts {
		if i > 0 {
			b.body.WriteByte(' ')
		}
		b.num(&b.body, p.X)
		b.body.WriteByte(',')
		b.num(&b.body, p.Y)
	}
	b.body.WriteByte('"')
	b.strokeAttrs(&b.body, style)
	b.body.WriteString(`/>`)
	b.nl(&b.body)
}

func (b *backend) StrokePath(p *ir.Path, style ir.Stroke) {
	if !style.Visible() || p == nil || p.Empty() {
		return
	}
	b.body.WriteString(`<path fill="none" d="`)
	b.pathData(&b.body, p)
	b.body.WriteByte('"')
	b.strokeAttrs(&b.body, style)
	b.body.WriteString(`/>`)
	b.nl(&b.body)
}

func (b *backend) FillPath(p *ir.Path, fill ir.Fill, rule ir.FillRule) {
	if p == nil || p.Empty() || !fill.Visible() {
		return
	}
	b.body.WriteString(`<path d="`)
	b.pathData(&b.body, p)
	b.body.WriteByte('"')
	b.fillAttrs(&b.body, fill)
	if rule == ir.EvenOdd {
		b.body.WriteString(` fill-rule="evenodd"`)
	}
	b.body.WriteString(`/>`)
	b.nl(&b.body)
}

func (b *backend) Text(run ir.TextRun) {
	if run.Text == "" || run.Color.A == 0 {
		return
	}
	family := run.Font.Family
	if family == "" {
		family = b.opts.family
	}

	b.body.WriteString(`<text x="`)
	b.num(&b.body, run.At.X)
	b.body.WriteString(`" y="`)
	b.num(&b.body, run.At.Y)
	b.body.WriteString(`" font-family="`)
	xmlEscape(&b.body, family)
	b.body.WriteString(`" font-size="`)
	b.numF(&b.body, run.Font.Size)
	b.body.WriteByte('"')
	if run.Font.Weight != 0 && run.Font.Weight != 400 {
		b.body.WriteString(` font-weight="`)
		b.body.WriteString(strconv.Itoa(run.Font.Weight))
		b.body.WriteByte('"')
	}
	if run.Font.Italic {
		b.body.WriteString(` font-style="italic"`)
	}
	if a := anchorOf(run.H); a != "" {
		b.body.WriteString(` text-anchor="`)
		b.body.WriteString(a)
		b.body.WriteByte('"')
	}
	if a := baselineOf(run.V); a != "" {
		b.body.WriteString(` dominant-baseline="`)
		b.body.WriteString(a)
		b.body.WriteByte('"')
	}
	b.body.WriteString(` fill="`)
	writeColor(&b.body, run.Color)
	b.body.WriteByte('"')
	if op := opacityOf(run.Color); op != "" {
		b.body.WriteString(` fill-opacity="`)
		b.body.WriteString(op)
		b.body.WriteByte('"')
	}
	if run.Rotation != 0 {
		b.body.WriteString(` transform="rotate(`)
		b.numF(&b.body, run.Rotation*180/math.Pi)
		b.body.WriteByte(' ')
		b.num(&b.body, run.At.X)
		b.body.WriteByte(' ')
		b.num(&b.body, run.At.Y)
		b.body.WriteString(`)"`)
	}
	b.body.WriteByte('>')
	xmlEscape(&b.body, run.Text)
	b.body.WriteString(`</text>`)
	b.nl(&b.body)
}

func (b *backend) Markers(shape ir.Marker, at []ir.Point, style ir.MarkerStyle) {
	if len(at) == 0 || style.Size <= 0 {
		return
	}
	if style.Fill.A == 0 && !style.Stroke.Visible() {
		return
	}
	id := b.symbol(shape, style)
	for _, p := range at {
		b.body.WriteString(`<use href="#`)
		b.body.WriteString(id)
		b.body.WriteString(`" x="`)
		b.num(&b.body, p.X)
		b.body.WriteString(`" y="`)
		b.num(&b.body, p.Y)
		b.body.WriteString(`"/>`)
		b.nl(&b.body)
	}
}

// symbol interns one <path> per distinct marker style in <defs>, so a scatter
// of 100k points writes one shape definition and 100k short <use> elements
// rather than 100k full paths.
func (b *backend) symbol(shape ir.Marker, style ir.MarkerStyle) string {
	key := fmt.Sprintf("%d|%g|%v|%v|%g", shape, style.Size, style.Fill, style.Stroke.Color, style.Stroke.Width)
	if id, ok := b.symbols[key]; ok {
		return id
	}
	id := b.id("m")
	b.symbols[key] = id

	var p ir.Path
	markerPath(&p, shape, style.Size)

	b.defs.WriteString(`<path id="`)
	b.defs.WriteString(id)
	b.defs.WriteString(`" d="`)
	b.pathData(&b.defs, &p)
	b.defs.WriteByte('"')
	if style.Fill.A != 0 {
		b.fillAttrs(&b.defs, ir.Solid(style.Fill))
	} else {
		b.defs.WriteString(` fill="none"`)
	}
	if style.Stroke.Visible() {
		b.strokeAttrs(&b.defs, style.Stroke)
	}
	b.defs.WriteString(`/>`)
	b.nl(&b.defs)
	return id
}

func (b *backend) Image(img image.Image, dst ir.Rect) {
	if img == nil || dst.Empty() {
		return
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, img); err != nil {
		b.fail(err)
		return
	}
	b.body.WriteString(`<image x="`)
	b.num(&b.body, dst.Min.X)
	b.body.WriteString(`" y="`)
	b.num(&b.body, dst.Min.Y)
	b.body.WriteString(`" width="`)
	b.num(&b.body, dst.Dx())
	b.body.WriteString(`" height="`)
	b.num(&b.body, dst.Dy())
	b.body.WriteString(`" preserveAspectRatio="none" href="data:image/png;base64,`)
	enc := base64.NewEncoder(base64.StdEncoding, &b.body)
	_, _ = enc.Write(raw.Bytes())
	_ = enc.Close()
	b.body.WriteString(`"/>`)
	b.nl(&b.body)
}

func (b *backend) Push(clip *ir.Path, xform ir.Affine) {
	b.body.WriteString(`<g`)
	if clip != nil && !clip.Empty() {
		id := b.id("c")
		b.defs.WriteString(`<clipPath id="`)
		b.defs.WriteString(id)
		b.defs.WriteString(`"><path d="`)
		b.pathData(&b.defs, clip)
		b.defs.WriteString(`"/></clipPath>`)
		b.nl(&b.defs)
		b.body.WriteString(` clip-path="url(#`)
		b.body.WriteString(id)
		b.body.WriteString(`)"`)
	}
	if !xform.IsIdentity() {
		b.body.WriteString(` transform="matrix(`)
		for i, v := range [...]float32{xform.A, xform.B, xform.C, xform.D, xform.E, xform.F} {
			if i > 0 {
				b.body.WriteByte(' ')
			}
			b.num(&b.body, v)
		}
		b.body.WriteString(`)"`)
	}
	b.body.WriteByte('>')
	b.nl(&b.body)
	b.depth++
}

func (b *backend) Pop() {
	if b.depth == 0 {
		b.fail(errors.New("refract/backend/svg: Pop without matching Push"))
		return
	}
	b.depth--
	b.body.WriteString(`</g>`)
	b.nl(&b.body)
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
		// Without outlines there is no true ink box, so report the font box.
		// Layout uses Ink only for collision padding, where erring large is
		// the safe direction.
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
	var face fontmetrics.Face
	if b.opts.font != nil {
		face = b.opts.font.Face(size)
	} else {
		face = fontmetrics.Builtin(size, f.Weight, f.Italic)
	}
	b.faces[k] = face
	return face
}

// --- finishing -----------------------------------------------------------

func (b *backend) Flush() error {
	if b.err != nil {
		return b.err
	}
	if b.depth != 0 {
		return fmt.Errorf("refract/backend/svg: %d unclosed group(s) at Flush", b.depth)
	}

	var head bytes.Buffer
	head.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.nl(&head)
	head.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="`)
	head.WriteString(strconv.Itoa(b.w))
	head.WriteString(`" height="`)
	head.WriteString(strconv.Itoa(b.h))
	head.WriteString(`" viewBox="0 0 `)
	head.WriteString(strconv.Itoa(b.w))
	head.WriteByte(' ')
	head.WriteString(strconv.Itoa(b.h))
	head.WriteString(`">`)
	b.nl(&head)

	if _, err := b.out.Write(head.Bytes()); err != nil {
		return err
	}
	if b.defs.Len() > 0 {
		if _, err := io.WriteString(b.out, `<defs>`); err != nil {
			return err
		}
		b.nlTo(b.out)
		if _, err := b.out.Write(b.defs.Bytes()); err != nil {
			return err
		}
		if _, err := io.WriteString(b.out, `</defs>`); err != nil {
			return err
		}
		b.nlTo(b.out)
	}
	if _, err := b.out.Write(b.body.Bytes()); err != nil {
		return err
	}
	_, err := io.WriteString(b.out, `</svg>`)
	return err
}

func (b *backend) nlTo(w io.Writer) {
	if b.opts.pretty {
		_, _ = io.WriteString(w, "\n")
	}
}

func (b *backend) fail(err error) {
	if b.err == nil {
		b.err = err
	}
}
