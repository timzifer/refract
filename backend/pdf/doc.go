// Package pdf renders a chart to PDF using nothing but the standard library.
//
// It is the second built-in emitter, and it exists for the same reason as
// backend/svg: PDF is a text format with a simple imaging model, and writing
// it directly costs a few hundred lines and no dependency at all. A report
// generator that wants a vector chart in a document links the same stdlib-only
// core it would have linked for SVG.
//
//	p := refract.New(refract.Title("Signal"))
//	p.Add(geom.Line(src, geom.X("t"), geom.Y("y")))
//	err := p.Render(pdf.File("signal.pdf"))
//
// # Coordinates
//
// PDF puts the origin at the bottom left with Y running up; refract's IR puts
// it at the top left with Y running down. The page's content stream opens with
// a flip, so every coordinate refract emits is written unchanged and the two
// backends' geometry agrees exactly. Text is placed with its own matrix, which
// undoes the flip for the glyphs alone so they read the right way up.
//
// # Text
//
// By default the output uses the base-14 Helvetica, which every PDF reader has
// and no document has to embed. That is also the metric set
// internal/fontmetrics carries, so this backend measures with exactly the font
// it draws with — margins, tick spacing and collision decisions are not
// approximations here.
//
// Text is then encoded as WinAnsi, which covers Latin-1 plus the usual
// typographic punctuation. A rune outside it is written as "?" rather than
// silently dropped, because a missing label is harder to notice than a wrong
// one — and that is a code page rather than a policy: Greek, Cyrillic, Hebrew,
// Thai and every CJK script are outside it.
//
// [WithFont] is the answer to that. Given a TrueType or OpenType face the
// document carries its own copy, text is written as glyph ids through an
// Identity-H encoding, and the repertoire is the font's rather than a code
// page's. A TrueType font is subset to the glyphs the document actually draws;
// a CFF-flavoured OpenType font is embedded whole, because cutting charstrings
// up is a second outline interpreter this package does not have. A ToUnicode
// map is always written, so an embedded document's text can still be selected,
// copied, searched and read aloud — a picture of a label is what
// [ADR 0024](../../docs/adr/0024-accessibility.md) exists to avoid, and an
// embedded font without one would be exactly that.
//
// An embedded face measures with its own tables, so the promise above holds
// either way round: this backend still measures with the font it draws with.
//
// # Not here
//
// One page per document, no tagging or accessibility structure, no
// transparency groups. Alpha is expressed as a graphics-state constant, which
// is what a faded area fill needs and is not the same thing as a full
// transparency model. Font embedding does not fall back per glyph: a document
// draws every label in the face it was given, and a rune that face has no
// glyph for is .notdef rather than a character borrowed from somewhere else.
package pdf
