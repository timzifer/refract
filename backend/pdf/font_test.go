package pdf_test

import (
	"strings"
	"testing"

	"github.com/timzifer/refract/backend/pdf"
	"github.com/timzifer/refract/internal/sfnttest"
	"github.com/timzifer/refract/ir"
)

// The failure this closes, stated once: without an embedded font a PDF's text
// is WinAnsi, so anything outside Latin-1 becomes "?" — no Greek, no Cyrillic,
// no Hebrew, no Thai, no CJK. The test font maps 'A', 'B' and 'Ä', which is
// enough to tell "encoded through the font" from "encoded through WinAnsi"
// without a megabyte of Noto in the repository.

func testTTF() []byte { return sfnttest.Font(sfnttest.Cmap4(), sfnttest.NumGlyphs) }

func drawLabel(t *testing.T, text string, opts ...pdf.Option) string {
	t.Helper()
	return render(t, 200, 100, func(b ir.Backend) {
		b.Text(ir.TextRun{Text: text, Font: ir.FontRef{Size: 12}, At: ir.Point{X: 10, Y: 50}, Color: ir.RGB(0, 0, 0)})
	}, opts...)
}

// Without a font the document names Helvetica and nothing is embedded, which
// is the right default: a chart of Latin text is a few kilobytes and every
// reader can open it.
func TestWithoutAFontNothingIsEmbedded(t *testing.T) {
	doc := drawLabel(t, "Hello")
	if !strings.Contains(doc, "/BaseFont /Helvetica") {
		t.Error("the default document does not name the base-14 Helvetica")
	}
	if strings.Contains(doc, "/FontFile2") {
		t.Error("the default document embedded a font program")
	}
}

// With one, the document carries a CID font: the only shape that reaches past
// 256 glyphs, which is the whole point.
func TestAnEmbeddedFontIsACIDFont(t *testing.T) {
	doc := drawLabel(t, "AB", pdf.WithFont(testTTF(), nil, nil))
	for _, want := range []string{
		"/Subtype /Type0",
		"/Encoding /Identity-H",
		"/Subtype /CIDFontType2",
		"/CIDToGIDMap /Identity",
		"/FontFile2",
		"/Type /FontDescriptor",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("the document is missing %q", want)
		}
	}
	if strings.Contains(doc, "/BaseFont /Helvetica") {
		t.Error("a document with an embedded font still named Helvetica")
	}
}

// The glyphs are numbered as they are first met, which is what makes writing
// a PDF one pass: the content stream names an id before the subset that
// defines it exists.
func TestTextIsWrittenAsGlyphIds(t *testing.T) {
	doc := drawLabel(t, "AB", pdf.WithFont(testTTF(), nil, nil))
	c := content(t, doc)
	// 'A' is the first glyph met, so it is cid 1; 'B' is cid 2.
	if !strings.Contains(c, "<00010002> Tj") {
		t.Errorf("the content stream does not carry the glyph ids:\n%s", c)
	}
	if strings.Contains(c, "(AB)") {
		t.Error("the text was written as a literal string; an Identity-H font is addressed by glyph")
	}
}

// The encoding is the whole difference. Through WinAnsi a label is a byte
// string and anything outside Latin-1 is a question mark; through the font it
// is a glyph id, and the repertoire is the font's rather than a code page's.
func TestALabelIsEncodedThroughTheFontRatherThanACodePage(t *testing.T) {
	// The base-14 path, on a rune WinAnsi has no byte for.
	plain := content(t, drawLabel(t, "漢"))
	if !strings.Contains(plain, "(?)") {
		t.Errorf("a CJK rune through WinAnsi is %q, want the visible substitute", plain)
	}

	// The same document with a font that has the glyph. The test font maps
	// 'Ä' to its composite, which is the first glyph met and therefore cid 1.
	c := content(t, drawLabel(t, "Ä", pdf.WithFont(testTTF(), nil, nil)))
	if !strings.Contains(c, "<0001> Tj") {
		t.Errorf("the composite glyph was not reached:\n%s", c)
	}
	if strings.Contains(c, "(") {
		t.Error("the label was still written as a byte string")
	}
}

// Without a ToUnicode map an embedded font's text is ink: nothing can be
// selected, copied, searched or read aloud, which turns a chart's labels back
// into a picture — the failure ADR 0024 exists to avoid, one layer down.
func TestAnEmbeddedFontCarriesAToUnicodeMap(t *testing.T) {
	doc := drawLabel(t, "AÄ", pdf.WithFont(testTTF(), nil, nil))
	if !strings.Contains(doc, "/ToUnicode") {
		t.Fatal("no ToUnicode map")
	}
	if !strings.Contains(doc, "beginbfchar") {
		t.Fatal("the ToUnicode map has no character mappings")
	}
	// 'A' is U+0041 at cid 1 and 'Ä' is U+00C4 at cid 2.
	for _, want := range []string{"<0001> <0041>", "<0002> <00C4>"} {
		if !strings.Contains(doc, want) {
			t.Errorf("the ToUnicode map is missing %q", want)
		}
	}
}

// Only the glyphs the document drew are written out. A chart with twenty
// labels must not carry a twenty-megabyte font.
func TestOnlyTheGlyphsDrawnAreEmbedded(t *testing.T) {
	one := drawLabel(t, "A", pdf.WithFont(testTTF(), nil, nil))
	if !strings.Contains(one, "/Length1") {
		t.Fatal("the font program was not embedded with its unsubset length")
	}
	// The subset holds .notdef and 'A' and nothing else, which the widths
	// array says: two entries rather than the font's four.
	if !strings.Contains(one, "/W [0 [500 600]]") {
		t.Errorf("the widths array is not the two glyphs that were drawn:\n%s", widthsOf(t, one))
	}
}

// A composite glyph pulls its components into the subset, or it draws nothing.
func TestACompositeGlyphBringsItsComponents(t *testing.T) {
	doc := drawLabel(t, "Ä", pdf.WithFont(testTTF(), nil, nil))
	// .notdef, the composite, and the two glyphs it is made of.
	if !strings.Contains(doc, "/W [0 [500 650 600 700]]") {
		t.Errorf("the subset does not hold the composite's components:\n%s", widthsOf(t, doc))
	}
}

// The metrics are the embedded font's own, which is what keeps this backend's
// promise that it measures with the font it draws with.
func TestAnEmbeddedFontMeasuresWithItsOwnTables(t *testing.T) {
	var got, want float32
	render(t, 200, 100, func(b ir.Backend) {
		got = b.Measure(ir.TextRun{Text: "AB", Font: ir.FontRef{Size: 100}}).Advance
	}, pdf.WithFont(testTTF(), nil, nil))
	render(t, 200, 100, func(b ir.Backend) {
		want = b.Measure(ir.TextRun{Text: "AB", Font: ir.FontRef{Size: 100}}).Advance
	})
	// 'A' is 600 units and 'B' is 700 on a 1000-unit em, so at 100pt the pair
	// is 130pt. Helvetica's own answer for "AB" is a different number, and a
	// backend measuring with the wrong one would size every margin from a
	// typeface the document does not contain.
	if got != 130 {
		t.Errorf("the embedded font measures %v, want 130 from its own advances", got)
	}
	if got == want {
		t.Errorf("the embedded font measured exactly as Helvetica did (%v); one of the two is not being read", got)
	}
}

// The three faces are three programs, and a face the caller did not supply
// falls back to the regular one rather than to a font the document does not
// carry.
func TestBoldFallsBackToTheRegularFace(t *testing.T) {
	doc := render(t, 200, 100, func(b ir.Backend) {
		b.Text(ir.TextRun{Text: "A", Font: ir.FontRef{Size: 12}, At: ir.Point{X: 10, Y: 20}, Color: ir.RGB(0, 0, 0)})
		b.Text(ir.TextRun{Text: "B", Font: ir.FontRef{Size: 12, Weight: 700}, At: ir.Point{X: 10, Y: 50}, Color: ir.RGB(0, 0, 0)})
	}, pdf.WithFont(testTTF(), nil, nil))

	if n := strings.Count(doc, "/FontFile2"); n != 2 {
		t.Errorf("the document holds %d font programs, want one per face asked for", n)
	}
	if n := strings.Count(doc, "/Subtype /Type0"); n != 2 {
		t.Errorf("the document holds %d Type0 fonts, want two", n)
	}
}

// A subset gets a tag so that two documents carrying different subsets of one
// font are not merged into one by a tool that reads the name.
func TestASubsetIsTagged(t *testing.T) {
	doc := drawLabel(t, "A", pdf.WithFont(testTTF(), nil, nil))
	if !strings.Contains(doc, "+RefractTest") {
		t.Errorf("the embedded font is not tagged with a subset prefix:\n%s", baseFontsOf(doc))
	}
}

// The same chart written twice is the same file. A tag from a hash of the
// glyphs, or a map iterated in its own order, would break that quietly.
func TestAnEmbeddedDocumentIsReproducible(t *testing.T) {
	a := drawLabel(t, "AÄB", pdf.WithFont(testTTF(), nil, nil))
	b := drawLabel(t, "AÄB", pdf.WithFont(testTTF(), nil, nil))
	if a != b {
		t.Error("two renders of the same chart produced different documents")
	}
}

// A font this package cannot read is refused when the target opens, because an
// Option cannot return an error and a document with half a font in it is worse
// than one that never started.
func TestABadFontIsRefusedAtOpen(t *testing.T) {
	tg := pdf.Writer(&strings.Builder{}, pdf.WithFont([]byte("not a font"), nil, nil))
	if _, err := tg.Open(100, 100, 1); err == nil {
		t.Error("a font that is not a font was accepted")
	}
}

// widthsOf and baseFontsOf pull the lines a failure should show out of a
// document, so a message is a fact rather than a wall of PDF.
func widthsOf(t *testing.T, doc string) string {
	t.Helper()
	for _, line := range strings.Split(doc, "\n") {
		if strings.Contains(line, "/W [") {
			return line
		}
	}
	return "no /W array in the document"
}

func baseFontsOf(doc string) string {
	var out []string
	for _, part := range strings.Split(doc, "/BaseFont /")[1:] {
		if i := strings.IndexAny(part, " \n"); i >= 0 {
			out = append(out, part[:i])
		}
	}
	return strings.Join(out, ", ")
}

// A CFF font is embedded whole and wrapped as the CID font a charstring
// program belongs in. The document is correct and larger, which is the trade
// the package comment names — and a .ttf build of the same family avoids it.
func TestACFFFontIsEmbeddedWhole(t *testing.T) {
	raw := sfnttest.CFFFont()
	doc := drawLabel(t, "AB", pdf.WithFont(raw, nil, nil))
	for _, want := range []string{"/Subtype /CIDFontType0", "/FontFile3", "/Subtype /OpenType"} {
		if !strings.Contains(doc, want) {
			t.Errorf("the document is missing %q", want)
		}
	}
	if strings.Contains(doc, "/FontFile2") {
		t.Error("a CFF program was written as a TrueType font file")
	}
	if !strings.Contains(doc, "a CFF program would go here") {
		t.Error("the font program was not embedded verbatim")
	}
	// The text is still glyph ids, because the encoding is the wrapper's
	// business rather than the outline format's.
	if !strings.Contains(content(t, doc), "<00010002> Tj") {
		t.Error("a CFF font's text was not written as glyph ids")
	}
}
