package mathtext

import "strings"

// Plainer is implemented by a typesetter that can also write its notation as
// readable text.
//
// A description that will be read aloud cannot contain markup: a screen reader
// asked to announce `$\frac{\sigma^2}{\sqrt{n}}$` says "dollar backslash frac".
// So a chart being described asks its typesetter for the same expression in
// words-and-characters, and falls back to the label as written when the
// typesetter has no opinion.
//
// It is optional, like every other interface refract extends a type through: a
// typesetter that does not implement it is still a typesetter.
type Plainer interface {
	// Plain returns src with its notation written as readable text. ok is
	// false when there was no notation in it, in which case src is already
	// what to say.
	Plain(src string) (string, bool)
}

// Plain writes the notation in src as readable text: symbols as themselves,
// a superscript after a caret, a fraction as a division.
//
// It is a reading rather than a rendering. "σ²/√n" is what a person would say
// looking at the fraction, and it is what a screen reader can pronounce; the
// typeset form is what the eye gets, and the two are not interchangeable.
func (t *tex) Plain(src string) (string, bool) {
	if !strings.ContainsRune(src, '$') {
		return src, false
	}
	pieces, ok := split(src)
	if !ok {
		return src, false
	}
	var b strings.Builder
	for _, p := range pieces {
		if !p.math {
			b.WriteString(p.text)
			continue
		}
		atoms, err := parse(p.text)
		if err != nil {
			// The same policy the layout takes: notation that cannot be parsed
			// is reported as it was written, so the mistake is visible.
			b.WriteString(p.text)
			continue
		}
		b.WriteString(plainRow(atoms))
	}
	return b.String(), true
}

func plainRow(atoms []atom) string {
	var b strings.Builder
	for _, a := range atoms {
		b.WriteString(plainNode(a.base))
		if a.sup != nil {
			b.WriteString("^")
			b.WriteString(group(plainNode(a.sup)))
		}
		if a.sub != nil {
			b.WriteString("_")
			b.WriteString(group(plainNode(a.sub)))
		}
	}
	return b.String()
}

func plainNode(n *node) string {
	if n == nil {
		return ""
	}
	switch n.kind {
	case nText:
		return n.text
	case nRow:
		return plainRow(n.kids)
	case nSpace:
		return " "
	case nFrac:
		return group(plainNode(n.args[0])) + "/" + group(plainNode(n.args[1]))
	case nSqrt:
		return "√" + group(plainNode(n.args[0]))
	case nOver:
		// A combining macron, so that "x̄" reads as one character rather than
		// as a bar drawn somewhere near an x.
		return plainNode(n.args[0]) + "\u0304"
	}
	return ""
}

// group parenthesises a fragment that would otherwise read as more than it is.
// "x^2" needs no brackets and "x^(n+1)" does, and the rule that tells them
// apart is whether the fragment is a single character.
func group(s string) string {
	if len([]rune(s)) <= 1 {
		return s
	}
	return "(" + s + ")"
}

// PlainOf writes src as readable text using ts, falling back to src for a
// typesetter that cannot — including no typesetter at all, which is a chart
// whose labels were never notation.
func PlainOf(ts Typesetter, src string) string {
	p, ok := ts.(Plainer)
	if !ok {
		return src
	}
	out, ok := p.Plain(src)
	if !ok {
		return src
	}
	return out
}
