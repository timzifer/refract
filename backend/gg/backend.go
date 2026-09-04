package gg

import (
	"image"

	gogg "github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
	"github.com/timzifer/refract/ir"
)

// backend adapts refract's IR onto a gg drawing context.
type backend struct {
	ctx   *gogg.Context
	fonts *fontSet
	depth int
	err   error
}

// newBackend wraps an existing gg context. It is exported through the targets
// in target.go; nothing outside this package constructs one directly, which
// keeps the gg types out of refract's API surface.
func newBackend(ctx *gogg.Context, fonts *fontSet) *backend {
	return &backend{ctx: ctx, fonts: fonts}
}

// --- paths ---------------------------------------------------------------

// buildPath replays an IR path into the context's current path.
func (b *backend) buildPath(p *ir.Path) {
	b.ctx.ClearPath()
	p.Walk(func(op ir.PathOp, pts []ir.Point) {
		switch op {
		case ir.OpMoveTo:
			b.ctx.MoveTo(float64(pts[0].X), float64(pts[0].Y))
		case ir.OpLineTo:
			b.ctx.LineTo(float64(pts[0].X), float64(pts[0].Y))
		case ir.OpCubicTo:
			b.ctx.CubicTo(
				float64(pts[0].X), float64(pts[0].Y),
				float64(pts[1].X), float64(pts[1].Y),
				float64(pts[2].X), float64(pts[2].Y),
			)
		case ir.OpClose:
			b.ctx.ClosePath()
		}
	})
}

func (b *backend) applyStroke(s ir.Stroke) {
	st := gogg.Stroke{
		Width:      float64(s.Width),
		Cap:        capOf(s.Cap),
		Join:       joinOf(s.Join),
		MiterLimit: 4,
	}
	if s.MiterLimit > 0 {
		st.MiterLimit = float64(s.MiterLimit)
	}
	if len(s.Dash) > 0 {
		lengths := make([]float64, len(s.Dash))
		for i, d := range s.Dash {
			lengths[i] = float64(d)
		}
		dash := gogg.NewDash(lengths...)
		dash.Offset = float64(s.DashOffset)
		st.Dash = dash
	}
	b.ctx.SetStroke(st)
	b.ctx.SetColor(s.Color)
}

func capOf(c ir.LineCap) gogg.LineCap {
	switch c {
	case ir.CapRound:
		return gogg.LineCapRound
	case ir.CapSquare:
		return gogg.LineCapSquare
	default:
		return gogg.LineCapButt
	}
}

func joinOf(j ir.LineJoin) gogg.LineJoin {
	switch j {
	case ir.JoinRound:
		return gogg.LineJoinRound
	case ir.JoinBevel:
		return gogg.LineJoinBevel
	default:
		return gogg.LineJoinMiter
	}
}

// --- ir.Backend ----------------------------------------------------------

func (b *backend) Polyline(pts []ir.Point, style ir.Stroke) {
	if !style.Visible() || len(pts) < 2 {
		return
	}
	b.ctx.ClearPath()
	b.ctx.MoveTo(float64(pts[0].X), float64(pts[0].Y))
	for _, p := range pts[1:] {
		b.ctx.LineTo(float64(p.X), float64(p.Y))
	}
	b.applyStroke(style)
	b.fail(b.ctx.Stroke())
}

func (b *backend) StrokePath(p *ir.Path, style ir.Stroke) {
	if !style.Visible() || p == nil || p.Empty() {
		return
	}
	b.buildPath(p)
	b.applyStroke(style)
	b.fail(b.ctx.Stroke())
}

func (b *backend) FillPath(p *ir.Path, fill ir.Fill, rule ir.FillRule) {
	if p == nil || p.Empty() || !fill.Visible() {
		return
	}
	b.buildPath(p)
	if rule == ir.EvenOdd {
		b.ctx.SetFillRule(gogg.FillRuleEvenOdd)
	} else {
		b.ctx.SetFillRule(gogg.FillRuleNonZero)
	}
	if fill.IsGradient() {
		b.ctx.SetFillBrush(gradientBrush(fill))
	} else {
		b.ctx.SetColor(fill.Color)
	}
	b.fail(b.ctx.Fill())
	b.ctx.SetFillRule(gogg.FillRuleNonZero)
}

func (b *backend) Text(run ir.TextRun) {
	if run.Text == "" || run.Color.A == 0 {
		return
	}
	face := b.fonts.apply(b.ctx, run.Font.Size, run.Font.Weight)
	x, y := anchor(face, run)

	b.ctx.SetColor(run.Color)
	if run.Rotation != 0 {
		b.ctx.Push()
		b.ctx.RotateAbout(run.Rotation, float64(run.At.X), float64(run.At.Y))
		b.ctx.DrawString(run.Text, x, y)
		b.ctx.Pop()
		return
	}
	b.ctx.DrawString(run.Text, x, y)
}

// anchor converts refract's two-axis alignment into the baseline-start
// position gg's DrawString expects.
//
// Doing this here rather than with gg's DrawStringAnchored is deliberate:
// DrawStringAnchored anchors on the run's bounding box, which shifts with the
// particular glyphs in the string, so a column of numbers would not line up.
// Anchoring on the font box keeps every run in a font on the same baseline.
func anchor(face text.Face, run ir.TextRun) (x, y float64) {
	x, y = float64(run.At.X), float64(run.At.Y)
	switch run.H {
	case ir.AlignCenter:
		x -= face.Advance(run.Text) / 2
	case ir.AlignEnd:
		x -= face.Advance(run.Text)
	}
	m := face.Metrics()
	switch run.V {
	case ir.AlignTop:
		y += m.Ascent
	case ir.AlignMiddle:
		y += (m.Ascent - m.Descent) / 2
	case ir.AlignBottom:
		y -= m.Descent
	}
	return x, y
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

	// One reusable path, translated per instance. gg has no instancing on the
	// CPU path, so this is the allocation-free equivalent: build the shape once
	// and move it, rather than constructing N paths.
	var proto ir.Path
	markerPath(&proto, shape, style.Size)

	var moved ir.Path
	for _, p := range at {
		moved.Reset()
		moved.Ops = append(moved.Ops, proto.Ops...)
		for _, q := range proto.Pts {
			moved.Pts = append(moved.Pts, ir.Point{X: q.X + p.X, Y: q.Y + p.Y})
		}
		if hasFill {
			b.buildPath(&moved)
			b.ctx.SetColor(style.Fill)
			if hasStroke {
				b.fail(b.ctx.FillPreserve())
			} else {
				b.fail(b.ctx.Fill())
			}
		}
		if hasStroke {
			if !hasFill {
				b.buildPath(&moved)
			}
			b.applyStroke(style.Stroke)
			b.fail(b.ctx.Stroke())
		}
	}
}

func (b *backend) Image(img image.Image, dst ir.Rect) {
	if img == nil || dst.Empty() {
		return
	}
	buf := gogg.ImageBufFromImage(img)
	if buf == nil {
		return
	}
	b.ctx.DrawImageEx(buf, gogg.DrawImageOptions{
		X:         float64(dst.Min.X),
		Y:         float64(dst.Min.Y),
		DstWidth:  float64(dst.Dx()),
		DstHeight: float64(dst.Dy()),
		Opacity:   1,
	})
}

func (b *backend) Push(clip *ir.Path, xform ir.Affine) {
	b.ctx.Push()
	b.depth++
	if !xform.IsIdentity() {
		b.ctx.Transform(matrixOf(xform))
	}
	if clip != nil && !clip.Empty() {
		b.buildPath(clip)
		b.ctx.Clip()
	}
}

func (b *backend) Pop() {
	if b.depth == 0 {
		return
	}
	b.depth--
	b.ctx.Pop()
}

// matrixOf converts refract's SVG-ordered affine into gg's row-major one.
func matrixOf(a ir.Affine) gogg.Matrix {
	return gogg.Matrix{
		A: float64(a.A), B: float64(a.C), C: float64(a.E),
		D: float64(a.B), E: float64(a.D), F: float64(a.F),
	}
}

func (b *backend) Measure(run ir.TextRun) ir.TextMetrics {
	face := b.fonts.face(run.Font.Size, run.Font.Weight)
	m := face.Metrics()
	adv := face.Advance(run.Text)
	return ir.TextMetrics{
		Advance: float32(adv),
		Ascent:  float32(m.Ascent),
		Descent: float32(m.Descent),
		Ink:     ir.R(0, float32(-m.Ascent), float32(adv), float32(m.Descent)),
	}
}

func (b *backend) Flush() error { return b.err }

func (b *backend) fail(err error) {
	if err != nil && b.err == nil {
		b.err = err
	}
}

// --- shapes --------------------------------------------------------------

// kappa is the cubic Bézier constant that approximates a quarter circle.
const kappa = 0.5522847498307936

// markerPath builds a marker centred on the origin.
//
// It mirrors the SVG backend's markerPath exactly, because "the same chart
// looks the same on every backend" has to hold for marker geometry too — a
// diamond that is a different diamond in PNG than in SVG would break the
// promise in a way nobody would notice until it mattered.
func markerPath(p *ir.Path, m ir.Marker, size float32) {
	r := size / 2
	switch m {
	case ir.MarkerSquare:
		p.Rect(ir.R(-r, -r, r, r))
	case ir.MarkerDiamond:
		p.MoveTo(0, -r).LineTo(r, 0).LineTo(0, r).LineTo(-r, 0).Close()
	case ir.MarkerTriangle:
		h := r * 1.5
		w := r * 1.2990381
		p.MoveTo(0, -h*2/3).LineTo(w, h/3).LineTo(-w, h/3).Close()
	case ir.MarkerCross:
		arm := r * 0.28
		d := (r - arm) * 0.7071068
		p.MoveTo(-d-arm, -d).LineTo(-d, -d-arm).LineTo(0, -arm*1.4142136).
			LineTo(d, -d-arm).LineTo(d+arm, -d).LineTo(arm*1.4142136, 0).
			LineTo(d+arm, d).LineTo(d, d+arm).LineTo(0, arm*1.4142136).
			LineTo(-d, d+arm).LineTo(-d-arm, d).LineTo(-arm*1.4142136, 0).Close()
	case ir.MarkerPlus:
		arm := r * 0.28
		p.MoveTo(-arm, -r).LineTo(arm, -r).LineTo(arm, -arm).LineTo(r, -arm).
			LineTo(r, arm).LineTo(arm, arm).LineTo(arm, r).LineTo(-arm, r).
			LineTo(-arm, arm).LineTo(-r, arm).LineTo(-r, -arm).LineTo(-arm, -arm).Close()
	default:
		k := r * kappa
		p.MoveTo(r, 0).
			CubicTo(r, k, k, r, 0, r).
			CubicTo(-k, r, -r, k, -r, 0).
			CubicTo(-r, -k, -k, -r, 0, -r).
			CubicTo(k, -r, r, -k, r, 0).
			Close()
	}
}

// gradientBrush converts an IR linear gradient into a gg brush.
func gradientBrush(f ir.Fill) gogg.Brush {
	g := gogg.NewLinearGradientBrush(
		float64(f.Start.X), float64(f.Start.Y),
		float64(f.End.X), float64(f.End.Y),
	)
	for _, s := range f.Stops {
		g.AddColorStop(float64(s.Offset), gogg.FromColor(s.Color))
	}
	return g
}
