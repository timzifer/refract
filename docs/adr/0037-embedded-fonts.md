# 0037 — A PDF carries the font its labels need, subset to the glyphs it drew

**Status:** Accepted · **Date:** 2026-09-07

## Context

`backend/pdf` names the base-14 Helvetica and encodes text as WinAnsi. Both
were right calls and are still the right default: the document is a few
kilobytes, every reader on earth can open it, and
`internal/fontmetrics` carries Helvetica's own advance table — so this is the
one backend that measures with exactly the font it draws with rather than
approximating.

WinAnsi is Latin-1 plus typographic punctuation. Everything else becomes `?`.
Written out, "everything else" is: Greek, Cyrillic, Hebrew, Arabic, Thai,
Devanagari, every CJK script, the Vietnamese diacritics, the Turkish dotless
ı, the Polish ł, the Romanian ș. A chart with a Japanese category axis renders
as a column of question marks, and the file it renders into is the format
people send to customers.

This was known and recorded — `AGENTS.md` has said "a PDF is one page with no
embedded font: text outside WinAnsi becomes `?`, and fixing that means
embedding a font" since v0.3. It stayed open because embedding a font means
reading one, and reading one means an sfnt parser in a module that has no
dependencies and keeps none.

## Decision

**`pdf.WithFont(regular, bold, italic)` embeds caller-supplied faces, and the
document is written as a CID font addressed by glyph id.**

### The parser is refract's, and it is small on purpose

`internal/sfnt` reads a table directory, `head`, `hhea`, `hmtx`, `maxp`,
`cmap`, `loca`, `glyf`, `OS/2`, `post` and `name`. It does not rasterize,
shape, kern or hint. It exists to answer four questions — which glyph is this
rune, how wide is it, what does the descriptor say, and what is the smallest
file that still draws these glyphs — and reads no table it has no use for.

That is a few hundred lines of `encoding/binary`, which is the trade the core
module's one rule forces and, at this size, is the better side of it: a font
library dependency would be an order of magnitude more code than the part
refract uses, in a module whose whole positioning is that it has none.

**Two cmap formats: 4 and 12.** Format 4 is the segmented map every Windows
font has carried since TrueType began; format 12 is what a font needs to reach
past the Basic Multilingual Plane. Formats 0 and 6 are single-byte and
single-range maps that no font with a Unicode subtable has used since the
nineties, and half-reading a format is worse than refusing it.

### Glyphs are numbered on first use, and that is what makes it one pass

A content stream names glyphs by id. The ids of a *subset* depend on which
glyphs the whole document used, so a writer that waited to find out would have
to buffer every text run in the file until the last one was drawn.

Numbering on first use inverts it: the id is known the moment a rune is seen,
the content stream is written immediately, and the subset is built at the end
to match — `Font.Subset` takes the order and honours it. First-appearance order
is also the only order that is a pure function of the drawing, which is the
rule group order, colour batching and panel replay all already follow
([ADR 0012](0012-parallel-panels.md)).

### The subset is closed over composites, and their references are rewritten

A composite glyph — an "ä" built from an "a" and a diaeresis — refers to other
glyphs by id. So the subset pulls its components in, **appending** them so that
nothing the caller has already numbered moves, and rewrites the ids inside the
composite to the new numbering. Copying a composite without rewriting it is the
one way a subset can be more than a gather, and it fails by drawing whatever
glyph happened to land at the old number: a plausible-looking wrong letter,
which is the worst kind.

### CFF is embedded whole, and says so

An `.otf` usually carries CFF charstrings rather than `glyf` outlines. Cutting
those up is a second outline interpreter, and it is not here. Such a font is
embedded whole as a `FontFile3` of subtype `OpenType` inside a `CIDFontType0`,
which is a correct document and a larger one — and a `.ttf` build of the same
family avoids it. `Font.CanSubset` is how a caller finds out which they have.

What is testable about that path is refract's half of it: that the format is
recognised, that it is reported as unsubsettable, and that the wrapper written
around it is the right one. Whether the charstrings render is the font's
business. There is a test for each of the three.

### A `ToUnicode` map is always written

Without one, an embedded font's text is ink: nothing can be selected, copied,
searched, or read by a screen reader. That is a chart's labels turned back into
a picture — the exact failure [ADR 0024](0024-accessibility.md) exists to
avoid, reappearing one layer down. It would also have been the easiest thing in
this record to leave out, because nothing *looks* wrong without it.

### An embedded face measures with its own tables

The package's promise is that this backend measures with the font it draws
with, and that has to survive the font changing. `embeddedFace` answers
`Advance`, `Ascent` and `Descent` from the embedded font's `hmtx` and `hhea`,
so a margin is the width the label turns out to have. Measuring against
Helvetica and drawing in Noto Sans would size every margin from a typeface the
document does not contain — which is the approximation every *other* backend
makes and this one does not.

## Consequences

- **The default is unchanged.** A document that does not ask for a font names
  Helvetica, embeds nothing and encodes WinAnsi, byte for byte as before. The
  golden files say so.
- **There is no per-glyph fallback.** A document draws every label in the face
  it was given, and a rune that face has no glyph for is `.notdef` rather than
  a character borrowed from another font. A fallback chain is a font-matching
  policy, and a plotting library is the wrong place to hold one.
- **A missing bold or italic falls back to the regular face**, which is a
  better answer than a bold label drawn in a font the document does not carry.
- **The subset tag is derived from the face's index, not from a hash of the
  glyphs.** A hash would make the file depend on which labels the chart
  happened to draw, and the golden tests compare bytes.
- **`internal/sfnttest` builds a font byte by byte**, because a test that needs
  a real font needs either a binary nobody can review or a font whose every
  byte the test wrote. It is a package rather than a test file because the
  parser and the backend both need it, and two copies would be two fonts that
  drift.
