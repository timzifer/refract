package mathtext

import (
	"strings"

	"github.com/timzifer/refract/ir"
)

// box is a laid-out piece of an expression: what to draw, and how much room it
// takes.
//
// Positions inside a box are relative to its own baseline origin. Composing two
// boxes side by side is then a translation of one of them, and stacking is a
// translation of both — which is the whole of the layout algorithm.
type box struct {
	runs  []Run
	rules []ir.Rect

	w               float32
	ascent, descent float32
}

// state carries what every layout step needs: the typesetter's parameters and
// the font stack to measure through.
type state struct {
	tex *tex
	m   Measurer
}

// row lays out a sequence of atoms left to right, with the spacing their
// classes call for.
//
// The spacing is what makes notation read as notation: "a+b" set with no gaps
// is a word, and "x=1" is a filename. TeX puts a medium space either side of a
// binary operator and a thick one either side of a relation, and this does the
// same with TeX's own widths — see [class].
func (s *state) row(atoms []atom, font ir.FontRef) box {
	var (
		parts   []box
		classes []class
	)
	for i := 0; i < len(atoms); i++ {
		a := atoms[i]
		// Adjacent ordinary characters at the same size and shape are one text
		// run, which is what keeps "max" a word the shaper can kern rather than
		// three runs the caller placed by hand. An operator is never merged
		// into its neighbours: it is a class of its own and gets spacing.
		if lit, ok := a.base.literal(); ok && a.sup == nil && a.sub == nil && classOf(lit) == classOrd {
			var sb strings.Builder
			sb.WriteString(lit)
			shape := shapeOf(a.base, lit)
			j := i + 1
			for ; j < len(atoms); j++ {
				next, ok := atoms[j].base.literal()
				if !ok || atoms[j].sup != nil || atoms[j].sub != nil {
					break
				}
				if shapeOf(atoms[j].base, next) != shape || classOf(next) != classOrd {
					break
				}
				sb.WriteString(next)
			}
			// A run of letters is a name and is upright, even though each of
			// its letters alone would have been a variable.
			text := sb.String()
			parts = append(parts, s.textBox(text, font, shape == shapeItalic && isSingleLetter(text)))
			classes = append(classes, classOrd)
			i = j - 1
			continue
		}
		parts = append(parts, s.atom(a, font))
		classes = append(classes, s.classOfAtom(a, len(parts) == 1, classes))
	}
	return s.hspace(parts, classes, font)
}

// class is what a piece of an expression is, for spacing purposes. TeX has
// eight; three are enough for what this typesetter sets.
type class uint8

const (
	classOrd class = iota // a variable, a number, a fraction, a group
	classBin              // a binary operator: plus, minus, times
	classRel              // a relation: equals, less than, approximately
)

// The characters each class is made of. They are single runes because that is
// what a macro expands to and what a keyboard produces; anything longer is a
// name and therefore ordinary.
var (
	binOperators = "+-−±∓×÷·∗⋆∪∩"
	relations    = "=<>≤≥≠≡∼≈∝≪≫∈∉⊂⊆→←↔⇒⇐⊥"
)

func classOf(text string) class {
	r := []rune(text)
	if len(r) != 1 {
		return classOrd
	}
	switch {
	case strings.ContainsRune(binOperators, r[0]):
		return classBin
	case strings.ContainsRune(relations, r[0]):
		return classRel
	}
	return classOrd
}

// classOfAtom classifies a laid-out atom.
//
// A binary operator with nothing to its left, or with another operator there,
// is not binary at all — it is a sign, as in "-1" or "x = -y" — and a sign
// takes no space after it. TeX makes the same correction, and without it every
// negative number in a chart is written with a gap in the middle.
func (s *state) classOfAtom(a atom, first bool, before []class) class {
	lit, ok := a.base.literal()
	if !ok || a.sup != nil || a.sub != nil {
		return classOrd
	}
	c := classOf(lit)
	if c == classBin && (first || before[len(before)-1] != classOrd) {
		return classOrd
	}
	return c
}

// hspace sets the parts side by side with the gap each junction calls for.
func (s *state) hspace(parts []box, classes []class, font ir.FontRef) box {
	var out box
	for i, p := range parts {
		if i > 0 {
			out.w += float32(gapBetween(classes[i-1], classes[i]) * font.Size)
		}
		out = merge(out, shift(p, out.w, 0))
		out.w += p.w
	}
	return out
}

// gapBetween is the space between two classes, as a fraction of the em. They
// are TeX's medmuskip and thickmuskip.
func gapBetween(left, right class) float64 {
	switch {
	case left == classRel || right == classRel:
		return thickSpace
	case left == classBin || right == classBin:
		return mediumSpace
	}
	return 0
}

// shape is how a text node is set.
type shape uint8

const (
	shapeUpright shape = iota
	shapeItalic
)

func shapeOf(n *node, text string) shape {
	switch {
	case n.italic:
		return shapeItalic
	case n.upright:
		return shapeUpright
	case isSingleLetter(text):
		return shapeItalic
	}
	return shapeUpright
}

// atom lays out one base with its scripts.
func (s *state) atom(a atom, font ir.FontRef) box {
	base := s.node(a.base, font)
	if a.sup == nil && a.sub == nil {
		return base
	}

	script := font
	script.Size = max(font.Size*s.tex.scriptScale, minFontSize)
	out := base

	// The shifts are fractions of the *base* size rather than of the script
	// size, so that a superscript sits at the same height whatever is under
	// it — which is what makes a row of them read as a row.
	if a.sup != nil {
		sup := s.node(a.sup, script)
		dy := -float32(font.Size * s.tex.supShift)
		out = overlay(out, sup, out.w, dy)
	}
	if a.sub != nil {
		sub := s.node(a.sub, script)
		dy := float32(font.Size * s.tex.subShift)
		// Both scripts start at the same x: TeX stacks them, it does not set
		// one after the other.
		out = overlay(out, sub, base.w, dy)
	}
	return out
}

// node lays out one node at the given size.
func (s *state) node(n *node, font ir.FontRef) box {
	if n == nil {
		return box{}
	}
	switch n.kind {
	case nText:
		return s.textBox(n.text, font, shapeOf(n, n.text) == shapeItalic)
	case nRow:
		return s.row(n.kids, font)
	case nSpace:
		return box{w: float32(n.width * font.Size)}
	case nFrac:
		return s.frac(n, font)
	case nSqrt:
		return s.sqrt(n, font)
	case nOver:
		return s.over(n, font)
	}
	return box{}
}

// textBox measures a literal string and makes a box of it.
func (s *state) textBox(text string, font ir.FontRef, italic bool) box {
	if text == "" {
		return box{}
	}
	f := font
	f.Italic = italic && s.tex.italic
	m := s.m.Measure(ir.TextRun{Text: text, Font: f})
	return box{
		runs:    []Run{{Text: text, Font: f, At: ir.Point{}}},
		w:       m.Advance,
		ascent:  m.Ascent,
		descent: m.Descent,
	}
}

// frac stacks a numerator over a denominator with a rule between them.
//
// The rule sits on the *axis* — the height a minus sign is drawn at — rather
// than on the baseline, so that a fraction is vertically centred against what
// is beside it. Everything else follows from that: the numerator's descent
// clears the rule by a gap, and the denominator's ascent clears it below.
func (s *state) frac(n *node, font ir.FontRef) box {
	num := s.node(n.args[0], font)
	den := s.node(n.args[1], font)

	thickness := max(float32(font.Size*ruleThickness), minRuleThickness)
	gap := float32(font.Size * fracGap)
	axis := float32(font.Size * axisHeight)

	// The rule's own box, centred on the axis above the baseline.
	ruleTop := -axis - thickness/2
	ruleBottom := ruleTop + thickness

	w := max(num.w, den.w) + 2*float32(font.Size*fracPad)
	out := box{w: w}

	// Each half is centred over the rule.
	out = merge(out, shift(num, (w-num.w)/2, ruleTop-gap-num.descent))
	out = merge(out, shift(den, (w-den.w)/2, ruleBottom+gap+den.ascent))
	out.rules = append(out.rules, ir.R(0, ruleTop, w, ruleBottom))
	out.w = w
	out.ascent = max(out.ascent, -ruleTop)
	out.descent = max(out.descent, ruleBottom)
	return out
}

// sqrt draws a radical sign and a bar over its argument.
//
// The sign is the font's own √ rather than a path: refract's IR draws text and
// shapes, and a radical hook made of line segments would need a curve, a
// thickness that matched the font's stroke and a hinting story of its own. The
// bar is a rule, which is what it is in metal type too.
func (s *state) sqrt(n *node, font ir.FontRef) box {
	body := s.node(n.args[0], font)
	sign := s.textBox("√", font, false)

	thickness := max(float32(font.Size*ruleThickness), minRuleThickness)
	gap := float32(font.Size * sqrtGap)

	// The bar sits above the argument, clear of its tallest ink, and the sign
	// is set so its own top meets it.
	top := -max(body.ascent, sign.ascent) - gap - thickness
	out := merge(box{}, sign)
	out = merge(out, shift(body, sign.w, 0))
	out.w = sign.w + body.w
	out.rules = append(out.rules, ir.R(sign.w, top, out.w, top+thickness))
	out.ascent = max(max(body.ascent, sign.ascent)+gap+thickness, out.ascent)
	out.descent = max(body.descent, sign.descent)
	return out
}

// over draws a bar across the top of its argument, which is what \bar and
// \overline set. It is [state.sqrt] without the sign — the same gap, the same
// rule thickness — so a mean and a root sit at the same height beside each
// other.
func (s *state) over(n *node, font ir.FontRef) box {
	body := s.node(n.args[0], font)
	thickness := max(float32(font.Size*ruleThickness), minRuleThickness)
	gap := float32(font.Size * sqrtGap)

	top := -body.ascent - gap - thickness
	out := merge(box{}, body)
	out.w = body.w
	out.rules = append(out.rules, ir.R(0, top, body.w, top+thickness))
	out.ascent = max(body.ascent+gap+thickness, out.ascent)
	return out
}

// hcat sets boxes side by side on one baseline.
func hcat(parts []box) box {
	var out box
	for _, p := range parts {
		out = merge(out, shift(p, out.w, 0))
		out.w += p.w
	}
	return out
}

// overlay places b at (dx, dy) inside a, widening a to cover it. It is what
// attaches a script: the base's width is not extended by a subscript that
// shares its x with a superscript.
func overlay(a, b box, dx, dy float32) box {
	out := merge(a, shift(b, dx, dy))
	out.w = max(a.w, dx+b.w)
	return out
}

// merge collects two boxes that are already positioned relative to one origin.
func merge(a, b box) box {
	a.runs = append(a.runs, b.runs...)
	a.rules = append(a.rules, b.rules...)
	a.ascent = max(a.ascent, b.ascent)
	a.descent = max(a.descent, b.descent)
	return a
}

// shift translates a box, including the extent it reports: a superscript
// raised above the baseline makes the whole expression taller by exactly how
// far it was raised.
func shift(b box, dx, dy float32) box {
	out := box{w: b.w, ascent: b.ascent - dy, descent: b.descent + dy}
	if len(b.runs) > 0 {
		out.runs = make([]Run, len(b.runs))
		for i, r := range b.runs {
			r.At.X += dx
			r.At.Y += dy
			out.runs[i] = r
		}
	}
	if len(b.rules) > 0 {
		out.rules = make([]ir.Rect, len(b.rules))
		for i, r := range b.rules {
			out.rules[i] = ir.Rect{
				Min: ir.Point{X: r.Min.X + dx, Y: r.Min.Y + dy},
				Max: ir.Point{X: r.Max.X + dx, Y: r.Max.Y + dy},
			}
		}
	}
	return out
}

// The layout constants, as fractions of the em. They are the numbers TeX's
// font parameters carry for a text-size font, rounded to two figures: there is
// no reason to invent different ones, and a reader who has seen mathematics
// before will notice if they are wrong.
const (
	ruleThickness = 0.05 // a fraction bar or a radical's bar
	fracGap       = 0.16 // between a bar and what it separates
	fracPad       = 0.06 // beside a fraction, so two of them do not touch
	axisHeight    = 0.25 // where a minus sign sits above the baseline
	sqrtGap       = 0.08 // between a radical's bar and its argument

	// minRuleThickness keeps a bar visible at a small type size, where a
	// twentieth of an em rounds to nothing.
	minRuleThickness = 0.5
	// minFontSize is the floor a script size is clamped to, so that a
	// superscript of a superscript is still text.
	minFontSize = 5
)
