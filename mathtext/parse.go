package mathtext

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

// atom is one item of a row: something to set, with whatever was raised or
// lowered against it.
//
// TeX's model, kept because it is the one that produces the right spacing: a
// script belongs to the atom before it rather than being an item in its own
// right, which is why x^2_i and x_i^2 set the same.
type atom struct {
	base *node
	sup  *node
	sub  *node
}

// nodeKind is what a node is.
type nodeKind uint8

const (
	nText  nodeKind = iota // a literal string
	nRow                   // a sequence of atoms
	nFrac                  // numerator over denominator
	nSqrt                  // a radical over its argument
	nOver                  // a bar over its argument
	nSpace                 // a fixed-width gap
)

// node is one piece of an expression.
type node struct {
	kind nodeKind
	text string
	// upright forces a text node upright — a name, a unit, a digit — rather
	// than leaving it to the single-letter rule.
	upright bool
	// italic forces it italic, which \mathit asks for.
	italic bool
	// width is a space's width, as a multiple of the em.
	width float64
	// kids holds a row's atoms, and args a fraction's two halves or a
	// radical's one.
	kids []atom
	args []*node
}

var errSyntax = errors.New("refract/mathtext: cannot parse")

// parse reads an expression into a row of atoms.
func parse(src string) ([]atom, error) {
	p := &parser{src: src}
	atoms, err := p.row(0)
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.src) {
		return nil, fmt.Errorf("%w: unexpected %q", errSyntax, p.src[p.pos:])
	}
	return atoms, nil
}

type parser struct {
	src string
	pos int
}

// row reads atoms until the end of the input or a closing brace. depth is the
// group nesting, which is what tells a closing brace from a stray one.
func (p *parser) row(depth int) ([]atom, error) {
	var out []atom
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case '}':
			if depth == 0 {
				return nil, fmt.Errorf("%w: unmatched }", errSyntax)
			}
			return out, nil
		case '^', '_':
			sup := p.src[p.pos] == '^'
			p.pos++
			arg, err := p.operand(depth)
			if err != nil {
				return nil, err
			}
			if len(out) == 0 {
				// A script with nothing to attach to gets an empty base,
				// which is what TeX does and is better than refusing: {}^2 is
				// legitimate notation.
				out = append(out, atom{base: &node{kind: nText}})
			}
			last := &out[len(out)-1]
			if sup {
				if last.sup != nil {
					return nil, fmt.Errorf("%w: two superscripts on one atom", errSyntax)
				}
				last.sup = arg
			} else {
				if last.sub != nil {
					return nil, fmt.Errorf("%w: two subscripts on one atom", errSyntax)
				}
				last.sub = arg
			}
		default:
			n, err := p.operand(depth)
			if err != nil {
				return nil, err
			}
			if n == nil {
				continue
			}
			out = append(out, atom{base: n})
		}
	}
	if depth > 0 {
		return nil, fmt.Errorf("%w: unclosed {", errSyntax)
	}
	return out, nil
}

// operand reads one thing: a group, a macro, or a single character.
func (p *parser) operand(depth int) (*node, error) {
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("%w: expression ends early", errSyntax)
	}
	switch c := p.src[p.pos]; {
	case c == '{':
		p.pos++
		kids, err := p.row(depth + 1)
		if err != nil {
			return nil, err
		}
		if p.pos >= len(p.src) || p.src[p.pos] != '}' {
			return nil, fmt.Errorf("%w: unclosed {", errSyntax)
		}
		p.pos++
		return &node{kind: nRow, kids: kids}, nil

	case c == '\\':
		return p.macro(depth)

	case c == ' ':
		// Space in notation is not a space: TeX ignores it, because the
		// spacing comes from what the symbols are. \, and friends are how a
		// gap is asked for.
		p.pos++
		return nil, nil

	case c == '}':
		return nil, fmt.Errorf("%w: unmatched }", errSyntax)

	default:
		r, size := utf8.DecodeRuneInString(p.src[p.pos:])
		p.pos += size
		// Digits and punctuation are upright; a letter's shape is decided at
		// layout, where its neighbours are known.
		return &node{kind: nText, text: string(r), upright: !unicode.IsLetter(r)}, nil
	}
}

// macro reads a backslash-introduced command.
func (p *parser) macro(depth int) (*node, error) {
	p.pos++ // the backslash
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("%w: trailing backslash", errSyntax)
	}

	// A one-character command: \, \; \% \$ \{ and so on. A name is letters.
	if !isLetter(p.src[p.pos]) {
		c := p.src[p.pos]
		p.pos++
		switch c {
		case ',':
			return &node{kind: nSpace, width: thinSpace}, nil
		case ';':
			return &node{kind: nSpace, width: thickSpace}, nil
		case ' ':
			return &node{kind: nSpace, width: interWordSpace}, nil
		case '\\':
			return nil, fmt.Errorf("%w: a line break has no place in a label", errSyntax)
		default:
			return &node{kind: nText, text: string(c), upright: true}, nil
		}
	}

	start := p.pos
	for p.pos < len(p.src) && isLetter(p.src[p.pos]) {
		p.pos++
	}
	name := p.src[start:p.pos]

	switch name {
	case "frac", "tfrac", "dfrac":
		num, err := p.argument(depth)
		if err != nil {
			return nil, err
		}
		den, err := p.argument(depth)
		if err != nil {
			return nil, err
		}
		return &node{kind: nFrac, args: []*node{num, den}}, nil

	case "sqrt":
		arg, err := p.argument(depth)
		if err != nil {
			return nil, err
		}
		return &node{kind: nSqrt, args: []*node{arg}}, nil

	case "bar", "overline":
		// An overbar is a radical without the sign, and it is the one accent a
		// chart label reaches for: x with a bar over it is a mean.
		arg, err := p.argument(depth)
		if err != nil {
			return nil, err
		}
		return &node{kind: nOver, args: []*node{arg}}, nil

	case "mathrm", "text", "textrm", "mathsf", "operatorname":
		arg, err := p.argument(depth)
		if err != nil {
			return nil, err
		}
		markText(arg, true, false)
		return arg, nil

	case "mathit":
		arg, err := p.argument(depth)
		if err != nil {
			return nil, err
		}
		markText(arg, false, true)
		return arg, nil

	case "quad":
		return &node{kind: nSpace, width: quadSpace}, nil
	case "qquad":
		return &node{kind: nSpace, width: 2 * quadSpace}, nil
	}

	if r, ok := Symbols[name]; ok {
		return &node{kind: nText, text: string(r), upright: true}, nil
	}
	return nil, fmt.Errorf("%w: unknown command \\%s", errSyntax, name)
}

// argument reads a macro's argument: a braced group, or the single item after
// it, which is what lets \sqrt2 mean \sqrt{2}.
func (p *parser) argument(depth int) (*node, error) {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("%w: a command is missing its argument", errSyntax)
	}
	n, err := p.operand(depth)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, fmt.Errorf("%w: a command is missing its argument", errSyntax)
	}
	return n, nil
}

func (p *parser) skipSpace() {
	for p.pos < len(p.src) && p.src[p.pos] == ' ' {
		p.pos++
	}
}

// markText walks a subtree and fixes the shape of every text node in it, which
// is how \mathrm reaches the letters inside a group.
func markText(n *node, upright, italic bool) {
	if n == nil {
		return
	}
	if n.kind == nText {
		n.upright, n.italic = upright, italic
	}
	for _, a := range n.kids {
		markText(a.base, upright, italic)
		markText(a.sup, upright, italic)
		markText(a.sub, upright, italic)
	}
	for _, arg := range n.args {
		markText(arg, upright, italic)
	}
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// The spaces, as multiples of the em. They are TeX's: a thin space is 3/18 of
// an em, a thick space 5/18, an inter-word space 6/18, a quad the em itself.
const (
	thinSpace      = 3.0 / 18
	mediumSpace    = 4.0 / 18
	thickSpace     = 5.0 / 18
	interWordSpace = 6.0 / 18
	quadSpace      = 1.0
)

// text returns the literal characters of a text node, for the layout pass to
// merge adjacent ones.
func (n *node) literal() (string, bool) {
	if n == nil || n.kind != nText {
		return "", false
	}
	return n.text, true
}

// isSingleLetter reports whether s is one letter, which is the rule that
// decides italics: a variable is a letter, a name is several.
func isSingleLetter(s string) bool {
	r, size := utf8.DecodeRuneInString(s)
	return size == len(s) && unicode.IsLetter(r) && !isGreekUpright(r)
}

// isGreekUpright reports the capital Greek letters, which TeX sets upright.
func isGreekUpright(r rune) bool { return r >= 'Α' && r <= 'Ω' }
