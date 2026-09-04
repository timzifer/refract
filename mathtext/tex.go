package mathtext

import (
	"strings"

	"github.com/timzifer/refract/ir"
)

// TeX returns a typesetter for a bounded subset of TeX's notation.
//
// It reads what a chart label actually contains, and stops there. What it
// knows:
//
//   - `$...$` delimits notation; everything outside is drawn as it was typed,
//     so "peak power $P_\mathrm{max}$ (W)" is one label with one expression in
//     it. A `$$` is a literal dollar sign.
//   - `x^2` and `x_i` raise and lower, `x^{n+1}` groups, and the two combine:
//     `x_i^2` sets both about one base.
//   - `\frac{a}{b}` stacks two expressions with a rule between them.
//   - `\sqrt{x}` sets a radical with a bar over its argument.
//   - `\alpha`, `\Omega`, `\times`, `\leq`, `\infty` and the rest of
//     [Symbols] name characters that are hard to type.
//   - `\mathrm{...}` and `\text{...}` set their argument upright, which is how
//     a unit or a word inside notation is kept from looking like a product of
//     variables.
//   - `\,` `\;` `\quad` and `~` are spaces of the usual widths.
//
// What it does not know is everything else: matrices, integrals with limits,
// alignment, \left\right delimiters that grow, and the hundreds of macros a
// document class defines. A label needing those wants a typesetting engine,
// and [Typesetter] is where one plugs in.
//
// # Italics
//
// A single letter is a variable and is set italic, the way TeX sets one; a run
// of letters is a name and is set upright, so that "max" does not read as m
// times a times x. `\mathrm` forces upright and `\mathit` forces italic. A
// backend with no italic face draws the upright one, which is a legible chart
// rather than a failed render.
func TeX(opts ...TeXOption) Typesetter {
	t := &tex{
		scriptScale: 0.7,
		supShift:    0.44,
		subShift:    0.20,
		italic:      true,
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// TeXOption configures the built-in typesetter.
type TeXOption func(*tex)

// Italic controls whether single-letter variables are set italic. It is on by
// default; turn it off for a chart whose font has no italic face and whose
// backend synthesises an unconvincing one.
func Italic(on bool) TeXOption { return func(t *tex) { t.italic = on } }

// ScriptScale sets how much smaller a superscript or subscript is than what it
// is attached to. The default is 0.7, which is TeX's own first script size.
func ScriptScale(f float64) TeXOption {
	return func(t *tex) {
		if f > 0 && f <= 1 {
			t.scriptScale = f
		}
	}
}

type tex struct {
	scriptScale float64
	supShift    float64
	subShift    float64
	italic      bool
}

// Typeset splits the label at its dollar signs and lays out what is between
// them. A label with no notation in it is refused rather than laid out, so that
// the ordinary case costs one scan and one Text call.
func (t *tex) Typeset(src string, font ir.FontRef, m Measurer) (Layout, bool) {
	if m == nil || !strings.ContainsRune(src, '$') {
		return Layout{}, false
	}
	pieces, ok := split(src)
	if !ok {
		return Layout{}, false
	}

	st := &state{tex: t, m: m}
	var row []box
	for _, p := range pieces {
		if !p.math {
			row = append(row, st.textBox(p.text, font, false))
			continue
		}
		atoms, err := parse(p.text)
		if err != nil {
			// Unparseable notation is drawn as it was written: the label is
			// the only place the mistake can be seen, and a render that failed
			// would hide it.
			return Layout{}, false
		}
		row = append(row, st.row(atoms, font))
	}

	out := hcat(row)
	return Layout{
		Runs:    out.runs,
		Rules:   out.rules,
		Width:   out.w,
		Ascent:  out.ascent,
		Descent: out.descent,
	}, true
}

// piece is one span of a label: text as typed, or notation to be parsed.
type piece struct {
	text string
	math bool
}

// split cuts a label at unescaped dollar signs. "$$" is a literal dollar, and
// an unclosed "$" means the label was not notation at all — a price, most
// likely — so the whole label goes back as text.
func split(src string) ([]piece, bool) {
	var (
		out  []piece
		buf  strings.Builder
		math bool
		any  bool
	)
	flush := func(isMath bool) {
		if buf.Len() > 0 {
			out = append(out, piece{text: buf.String(), math: isMath})
			buf.Reset()
		}
	}
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c != '$' {
			buf.WriteByte(c)
			continue
		}
		if i+1 < len(src) && src[i+1] == '$' && !math {
			buf.WriteByte('$')
			i++
			continue
		}
		flush(math)
		math = !math
		any = any || math
	}
	if math {
		return nil, false // an unclosed $ is a dollar sign, not an expression
	}
	flush(false)
	return out, any
}
