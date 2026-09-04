// Package mathtext typesets mathematical notation for chart labels.
//
// An axis reading "energy (eV)" is a label. One reading "E = mc²" or "σ²/√n"
// is notation, and notation is not a string with some characters in it: a
// superscript is smaller and raised, a fraction is two things stacked with a
// rule between them, a radical has a bar over what it covers. Writing those in
// Unicode gets a few of them approximately right and the rest not at all.
//
// # Pluggable
//
// [Typesetter] is the seam. refract calls it with a label and gets back the
// runs and rules that draw the label; what happens in between is the
// typesetter's business. [TeX] is the one that ships — a small, deliberately
// bounded subset of TeX's notation — and a caller with a real typesetting
// engine, or with notation of their own, implements the interface instead of
// arguing with this one.
//
// Nothing here is on by default. A chart typesets its labels when it is given
// a typesetter, and draws them as plain text when it is not:
//
//	p := refract.New(refract.Math(mathtext.TeX()), refract.YTitle(`$\sigma^2$`))
//
// # Where the work happens
//
// A typesetter places; it does not shape. It measures through [Measurer],
// which is the one method of [ir.Backend] it is given, and returns positions —
// so the backend that will draw the label is the one that measured it, which is
// the same bargain the rest of refract's text handling makes (docs/adr/0003).
//
// A layout is relative to an origin on the baseline at the start of the
// expression, x rightwards and y downwards, which is the coordinate system
// everything else in refract uses. Placing that origin — anchoring, alignment,
// rotation — is the caller's job, and package render does it.
package mathtext

import "github.com/timzifer/refract/ir"

// Measurer is the part of [ir.Backend] a typesetter is given: the ability to
// ask the font stack that will draw a run how wide it is.
type Measurer interface {
	// Measure reports the metrics of a run. See [ir.Backend.Measure].
	Measure(run ir.TextRun) ir.TextMetrics
}

// Typesetter lays out a label that may contain notation.
//
// It is given the whole label, not the notation inside it, so that a typesetter
// decides for itself what its delimiters are — [TeX] reads TeX's dollar signs,
// and another may read something else. A label with nothing to typeset in it
// returns ok false, and the caller draws it as ordinary text: that is the
// common case, and it must stay free.
//
// A typesetter never fails a render. Notation it cannot parse comes back as
// ok false too, so the label is drawn exactly as it was written — which is what
// a reader needs in order to see what is wrong with it.
type Typesetter interface {
	Typeset(src string, font ir.FontRef, m Measurer) (l Layout, ok bool)
}

// Layout is a typeset label: the pieces of text to draw and the rules to fill,
// with the box they occupy.
//
// Positions are relative to an origin on the baseline of the outermost row.
// Width, Ascent and Descent describe the whole expression, so that a caller can
// centre it, right-align it or stack it exactly as it would a text run —
// [ir.TextMetrics] carries the same three, and means the same thing by them.
type Layout struct {
	// Runs are the pieces of text, each with its own font and position. There
	// is one per run of characters at one size; a superscript is a run of its
	// own because it is set smaller.
	Runs []Run
	// Rules are the filled rectangles notation is made of: a fraction bar, the
	// bar over a radical. They are rectangles rather than strokes because
	// that is what a rule is — a thin filled box — and it needs no line width,
	// cap or join to be decided somewhere else.
	Rules []ir.Rect

	Width           float32
	Ascent, Descent float32
}

// Empty reports whether l draws nothing.
func (l Layout) Empty() bool { return len(l.Runs) == 0 && len(l.Rules) == 0 }

// Metrics reports the layout as text metrics, so that a caller measuring a
// label does not care whether it turned out to be notation.
//
// Ink is the full box rather than a tight outline: an expression's extent is
// what it occupies, and the pieces that stick out of a font box — a raised
// superscript, a fraction's numerator — are exactly the ones a caller sizing a
// margin must not clip.
func (l Layout) Metrics() ir.TextMetrics {
	return ir.TextMetrics{
		Advance: l.Width,
		Ascent:  l.Ascent,
		Descent: l.Descent,
		Ink:     ir.R(0, -l.Ascent, l.Width, l.Descent),
	}
}

// Run is one piece of text in a layout: what to draw, in what font, with its
// baseline start at At.
type Run struct {
	Text string
	Font ir.FontRef
	At   ir.Point
}

// Draw emits a layout into a backend.
//
// at is the anchor — the point the label was positioned at, and the point a
// rotated one turns about. offset moves the layout's own origin relative to
// that anchor, which is where alignment lives: a centred label is offset by
// half its width, and only the caller knows it wanted one. color paints every
// piece, text and rule alike, and rotation turns the whole expression.
//
// It is the other half of [Typesetter]: a typesetter says where the pieces go
// and this puts them there, so that every caller draws notation the same way
// and no caller has to know that a fraction bar is a filled rectangle.
func Draw(b ir.Backend, l Layout, at, offset ir.Point, color ir.Color, rotation float64) {
	if l.Empty() {
		return
	}
	place := func(p ir.Point) ir.Point {
		return ir.Point{X: at.X + offset.X + p.X, Y: at.Y + offset.Y + p.Y}
	}
	if rotation != 0 {
		// A rotated expression is drawn in its own frame: the transform turns
		// about the anchor, and every piece is placed relative to it. Rotating
		// each run separately would turn each one about its own start instead,
		// which fans a fraction out across the canvas.
		b.Push(nil, ir.Translate(at.X, at.Y).Mul(ir.Rotate(rotation)))
		defer b.Pop()
		place = func(p ir.Point) ir.Point {
			return ir.Point{X: offset.X + p.X, Y: offset.Y + p.Y}
		}
	}
	for _, r := range l.Rules {
		var p ir.Path
		min, max := place(r.Min), place(r.Max)
		p.Rect(ir.Rect{Min: min, Max: max})
		b.FillPath(&p, ir.Solid(color), ir.NonZero)
	}
	for _, r := range l.Runs {
		b.Text(ir.TextRun{
			Text:  r.Text,
			Font:  r.Font,
			At:    place(r.At),
			H:     ir.AlignStart,
			V:     ir.AlignBaseline,
			Color: color,
		})
	}
}
