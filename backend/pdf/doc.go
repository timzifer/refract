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
// The output uses the base-14 Helvetica, which every PDF reader has and no
// document has to embed. That is also the metric set
// internal/fontmetrics carries, so this backend measures with exactly the font
// it draws with — margins, tick spacing and collision decisions are not
// approximations here.
//
// Text is encoded as WinAnsi, which covers Latin-1 plus the usual typographic
// punctuation. A rune outside it is written as "?" rather than silently
// dropped, because a missing label is harder to notice than a wrong one.
//
// # Not here
//
// One page per document, no embedded fonts, no tagging or accessibility
// structure, no transparency groups. Alpha is expressed as a graphics-state
// constant, which is what a faded area fill needs and is not the same thing as
// a full transparency model.
package pdf
