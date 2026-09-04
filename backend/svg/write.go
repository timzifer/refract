package svg

import (
	"bytes"
	"math"
	"strconv"

	"github.com/timzifer/refract/ir"
)

// coordDecimals is the precision every coordinate is written with.
//
// Three decimals is well below a pixel at any sane device scale, and fixing it
// is what makes output byte-stable: without it, the shortest-representation
// formatting of a float32 varies with the last bit of a computation and golden
// files churn for no visible reason.
const coordDecimals = 3

// num writes a device coordinate.
func (b *backend) num(w *bytes.Buffer, v float32) { writeNum(w, float64(v), coordDecimals) }

// numF writes a non-coordinate float, such as a font size or an angle.
func (b *backend) numF(w *bytes.Buffer, v float64) { writeNum(w, v, coordDecimals) }

func writeNum(w *bytes.Buffer, v float64, decimals int) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		// A non-finite coordinate would make the whole document unparseable.
		// Zero is wrong but recoverable, and the geom layer has already
		// filtered these out on every path that carries data.
		w.WriteByte('0')
		return
	}
	start := w.Len()
	w.Write(strconv.AppendFloat(w.AvailableBuffer(), v, 'f', decimals, 64))
	trimTrailingZeros(w, start)
}

// trimTrailingZeros rewrites the number just written at or after start,
// dropping a trailing zero run and normalising "-0" to "0".
func trimTrailingZeros(w *bytes.Buffer, start int) {
	s := w.Bytes()[start:]
	if !bytes.ContainsRune(s, '.') {
		return
	}
	end := len(s)
	for end > 0 && s[end-1] == '0' {
		end--
	}
	if end > 0 && s[end-1] == '.' {
		end--
	}
	// Everything left of the decimal point may still be a signed zero. SVG
	// accepts "-0", but emitting it would make otherwise identical renders
	// differ on the sign bit of a rounding result.
	if isZeroText(s[:end]) {
		w.Truncate(start)
		w.WriteByte('0')
		return
	}
	w.Truncate(start + end)
}

// isZeroText reports whether s is a zero, with or without a sign.
func isZeroText(s []byte) bool {
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		s = s[1:]
	}
	if len(s) == 0 {
		return true
	}
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}

// pathData writes a path as an SVG "d" attribute value.
func (b *backend) pathData(w *bytes.Buffer, p *ir.Path) {
	first := true
	p.Walk(func(op ir.PathOp, pts []ir.Point) {
		if !first {
			w.WriteByte(' ')
		}
		first = false
		switch op {
		case ir.OpMoveTo:
			w.WriteByte('M')
			b.writePts(w, pts)
		case ir.OpLineTo:
			w.WriteByte('L')
			b.writePts(w, pts)
		case ir.OpCubicTo:
			w.WriteByte('C')
			b.writePts(w, pts)
		case ir.OpClose:
			w.WriteByte('Z')
		}
	})
}

func (b *backend) writePts(w *bytes.Buffer, pts []ir.Point) {
	for i, p := range pts {
		if i > 0 {
			w.WriteByte(' ')
		}
		b.num(w, p.X)
		w.WriteByte(',')
		b.num(w, p.Y)
	}
}

// --- paint ---------------------------------------------------------------

func (b *backend) strokeAttrs(w *bytes.Buffer, s ir.Stroke) {
	w.WriteString(` stroke="`)
	writeColor(w, s.Color)
	w.WriteByte('"')
	if op := opacityOf(s.Color); op != "" {
		w.WriteString(` stroke-opacity="`)
		w.WriteString(op)
		w.WriteByte('"')
	}
	if s.Width != 1 {
		w.WriteString(` stroke-width="`)
		b.num(w, s.Width)
		w.WriteByte('"')
	}
	switch s.Cap {
	case ir.CapRound:
		w.WriteString(` stroke-linecap="round"`)
	case ir.CapSquare:
		w.WriteString(` stroke-linecap="square"`)
	}
	switch s.Join {
	case ir.JoinRound:
		w.WriteString(` stroke-linejoin="round"`)
	case ir.JoinBevel:
		w.WriteString(` stroke-linejoin="bevel"`)
	}
	if s.Join == ir.JoinMiter && s.MiterLimit > 0 && s.MiterLimit != 4 {
		w.WriteString(` stroke-miterlimit="`)
		b.num(w, s.MiterLimit)
		w.WriteByte('"')
	}
	if len(s.Dash) > 0 {
		w.WriteString(` stroke-dasharray="`)
		for i, d := range s.Dash {
			if i > 0 {
				w.WriteByte(' ')
			}
			b.num(w, d)
		}
		w.WriteByte('"')
		if s.DashOffset != 0 {
			w.WriteString(` stroke-dashoffset="`)
			b.num(w, s.DashOffset)
			w.WriteByte('"')
		}
	}
}

func (b *backend) fillAttrs(w *bytes.Buffer, f ir.Fill) {
	if f.IsGradient() {
		id := b.gradient(f)
		w.WriteString(` fill="url(#`)
		w.WriteString(id)
		w.WriteString(`)"`)
		return
	}
	w.WriteString(` fill="`)
	writeColor(w, f.Color)
	w.WriteByte('"')
	if op := opacityOf(f.Color); op != "" {
		w.WriteString(` fill-opacity="`)
		w.WriteString(op)
		w.WriteByte('"')
	}
}

// gradient emits a <linearGradient> into <defs> and returns its id.
func (b *backend) gradient(f ir.Fill) string {
	id := b.id("g")
	b.defs.WriteString(`<linearGradient id="`)
	b.defs.WriteString(id)
	b.defs.WriteString(`" gradientUnits="userSpaceOnUse" x1="`)
	b.num(&b.defs, f.Start.X)
	b.defs.WriteString(`" y1="`)
	b.num(&b.defs, f.Start.Y)
	b.defs.WriteString(`" x2="`)
	b.num(&b.defs, f.End.X)
	b.defs.WriteString(`" y2="`)
	b.num(&b.defs, f.End.Y)
	b.defs.WriteString(`">`)
	for _, s := range f.Stops {
		b.defs.WriteString(`<stop offset="`)
		b.num(&b.defs, s.Offset)
		b.defs.WriteString(`" stop-color="`)
		writeColor(&b.defs, s.Color)
		b.defs.WriteByte('"')
		if op := opacityOf(s.Color); op != "" {
			b.defs.WriteString(` stop-opacity="`)
			b.defs.WriteString(op)
			b.defs.WriteByte('"')
		}
		b.defs.WriteString(`/>`)
	}
	b.defs.WriteString(`</linearGradient>`)
	b.nl(&b.defs)
	return id
}

const hexDigits = "0123456789abcdef"

// writeColor writes a colour as #rrggbb. Alpha is carried separately in a
// *-opacity attribute rather than as #rrggbbaa, because the eight-digit form
// is not understood by every SVG consumer that matters.
func writeColor(w *bytes.Buffer, c ir.Color) {
	w.WriteByte('#')
	for _, v := range [3]uint8{c.R, c.G, c.B} {
		w.WriteByte(hexDigits[v>>4])
		w.WriteByte(hexDigits[v&0x0F])
	}
}

// opacityOf returns the opacity attribute value for c, or "" if it is opaque.
func opacityOf(c ir.Color) string {
	if c.A == 255 {
		return ""
	}
	// Two decimals: the eye cannot resolve finer, and it keeps output stable.
	return strconv.FormatFloat(math.Round(float64(c.A)/255*100)/100, 'g', -1, 64)
}

func anchorOf(h ir.HAlign) string {
	switch h {
	case ir.AlignCenter:
		return "middle"
	case ir.AlignEnd:
		return "end"
	default:
		return ""
	}
}

func baselineOf(v ir.VAlign) string {
	switch v {
	case ir.AlignTop:
		return "hanging"
	case ir.AlignMiddle:
		return "central"
	case ir.AlignBottom:
		return "auto"
	default:
		return ""
	}
}

// xmlEscape writes s with the five XML metacharacters escaped.
//
// encoding/xml's own escaper is not used because it also escapes newlines and
// tabs as character references, which would make golden files noisy for no
// benefit in attribute values that never contain them.
func xmlEscape(w *bytes.Buffer, s string) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '&':
			w.WriteString("&amp;")
		case '<':
			w.WriteString("&lt;")
		case '>':
			w.WriteString("&gt;")
		case '"':
			w.WriteString("&quot;")
		case '\'':
			w.WriteString("&apos;")
		default:
			w.WriteByte(c)
		}
	}
}

// --- marker shapes -------------------------------------------------------

// kappa is the cubic Bézier constant that approximates a quarter circle to
// within about 0.02% of the true arc.
const kappa = 0.5522847498307936

// markerPath builds a marker shape centred on the origin, sized so that its
// nominal extent is size.
func markerPath(p *ir.Path, m ir.Marker, size float32) {
	r := size / 2
	switch m {
	case ir.MarkerSquare:
		p.Rect(ir.R(-r, -r, r, r))
	case ir.MarkerDiamond:
		p.MoveTo(0, -r).LineTo(r, 0).LineTo(0, r).LineTo(-r, 0).Close()
	case ir.MarkerTriangle:
		// Centroid-centred equilateral triangle pointing up.
		h := r * 1.5
		w := r * 1.2990381 // r * 1.5 * sqrt(3)/2
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
	default: // ir.MarkerCircle
		k := r * kappa
		p.MoveTo(r, 0).
			CubicTo(r, k, k, r, 0, r).
			CubicTo(-k, r, -r, k, -r, 0).
			CubicTo(-r, -k, -k, -r, 0, -r).
			CubicTo(k, -r, r, -k, r, 0).
			Close()
	}
}
