package sfnt_test

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/timzifer/refract/internal/sfnt"
	"github.com/timzifer/refract/internal/sfnttest"
)

func parse(t *testing.T, b []byte) *sfnt.Font {
	t.Helper()
	f, err := sfnt.Parse(b)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return f
}

func TestParseReadsTheDescriptorsNumbers(t *testing.T) {
	f := parse(t, sfnttest.Font(sfnttest.Cmap4(), sfnttest.NumGlyphs))
	if f.UnitsPerEm() != sfnttest.UnitsPerEm {
		t.Errorf("sfnttest.UnitsPerEm is %d, want %d", f.UnitsPerEm(), sfnttest.UnitsPerEm)
	}
	if f.NumGlyphs() != sfnttest.NumGlyphs {
		t.Errorf("sfnttest.NumGlyphs is %d, want %d", f.NumGlyphs(), sfnttest.NumGlyphs)
	}
	if f.Ascent() != 800 || f.Descent() != -200 {
		t.Errorf("ascent/descent are %d/%d, want 800/-200", f.Ascent(), f.Descent())
	}
	if f.CapHeight() != 720 {
		t.Errorf("cap height is %d, want the OS/2 value 720", f.CapHeight())
	}
	if f.ItalicAngle() != -12 {
		t.Errorf("italic angle is %v, want -12: a face that leans right reports a negative angle", f.ItalicAngle())
	}
	if f.WeightClass() != 700 {
		t.Errorf("weight class is %d, want 700", f.WeightClass())
	}
	if x0, y0, x1, y1 := f.BBox(); x0 != -50 || y0 != -200 || x1 != 800 || y1 != 900 {
		t.Errorf("bbox is %d %d %d %d, want -50 -200 800 900", x0, y0, x1, y1)
	}
	if f.PostScriptName() != "Refract Test" {
		t.Errorf("name is %q, want the record in the name table", f.PostScriptName())
	}
	if !f.CanSubset() || f.CFF() {
		t.Error("a glyf font reported itself as unsubsettable or as CFF")
	}
}

// The hmtx tail repeats the last advance, and every reader has to undo that.
// It is the one part of the format that is easy to read as though it were an
// array, so it is tested from a font whose table really does have one.
func TestTheMetricTailRepeatsTheLastAdvance(t *testing.T) {
	f := parse(t, sfnttest.Font(sfnttest.Cmap4(), 2))
	if got := f.Advance(0); got != int(sfnttest.Advances[0]) {
		t.Errorf("glyph 0 advances %d, want %d", got, sfnttest.Advances[0])
	}
	if got := f.Advance(1); got != int(sfnttest.Advances[1]) {
		t.Errorf("glyph 1 advances %d, want %d", got, sfnttest.Advances[1])
	}
	for g := 2; g < sfnttest.NumGlyphs; g++ {
		if got := f.Advance(uint16(g)); got != int(sfnttest.Advances[1]) {
			t.Errorf("glyph %d advances %d, want the tail's %d", g, got, sfnttest.Advances[1])
		}
	}
}

func TestTheCmapFormats(t *testing.T) {
	t.Run("format 4", func(t *testing.T) {
		f := parse(t, sfnttest.Font(sfnttest.Cmap4(), sfnttest.NumGlyphs))
		check(t, f, 'A', sfnttest.GlyphA)
		check(t, f, 'B', sfnttest.GlyphB)
		check(t, f, 'Ä', sfnttest.GlyphComp)
		check(t, f, 'Z', 0)
		check(t, f, '\U0001F600', 0)
	})
	t.Run("format 12", func(t *testing.T) {
		f := parse(t, sfnttest.Font(sfnttest.Cmap12(), sfnttest.NumGlyphs))
		check(t, f, 'A', sfnttest.GlyphA)
		check(t, f, 'Ä', sfnttest.GlyphComp)
		check(t, f, 'Z', 0)
		// The whole point of format 12: a code point past the BMP, which
		// format 4 cannot address at all.
		check(t, f, '\U0001F600', sfnttest.GlyphB)
	})
}

func check(t *testing.T, f *sfnt.Font, r rune, want uint16) {
	t.Helper()
	if got := f.GlyphIndex(r); got != want {
		t.Errorf("%q maps to glyph %d, want %d", r, got, want)
	}
}

// The subset keeps the order it was given, which is what lets a PDF writer
// hand out an id the moment it first sees a rune rather than buffering the
// document.
func TestASubsetKeepsTheOrderItWasGiven(t *testing.T) {
	f := parse(t, sfnttest.Font(sfnttest.Cmap4(), sfnttest.NumGlyphs))
	_, final, err := f.Subset([]uint16{0, sfnttest.GlyphB, sfnttest.GlyphA})
	if err != nil {
		t.Fatal(err)
	}
	if len(final) != 3 || final[0] != 0 || final[1] != sfnttest.GlyphB || final[2] != sfnttest.GlyphA {
		t.Errorf("the subset came out as %v, want the order it was given", final)
	}
}

// A composite refers to other glyphs, so the subset is closed over them — and
// the components are appended rather than inserted, or the ids the caller
// already handed out would move.
func TestASubsetPullsInTheGlyphsACompositeNeeds(t *testing.T) {
	f := parse(t, sfnttest.Font(sfnttest.Cmap4(), sfnttest.NumGlyphs))
	data, final, err := f.Subset([]uint16{0, sfnttest.GlyphComp})
	if err != nil {
		t.Fatal(err)
	}
	if len(final) != 4 {
		t.Fatalf("the subset holds %v, want the composite and the two glyphs it is made of", final)
	}
	if final[0] != 0 || final[1] != sfnttest.GlyphComp {
		t.Errorf("the subset moved a glyph the caller had already numbered: %v", final)
	}
	if final[2] != sfnttest.GlyphA || final[3] != sfnttest.GlyphB {
		t.Errorf("the subset pulled in %v, want the composite's two components appended", final[2:])
	}

	sub := parse(t, data)
	if sub.NumGlyphs() != 4 {
		t.Errorf("the subset font says it has %d glyphs, want 4", sub.NumGlyphs())
	}
	if sub.HasUnicodeMap() {
		t.Error("the subset carries a character map; a CID font addresses glyphs by id and never asks")
	}
}

// The one place a subset is more than a gather: the glyph ids inside a
// composite are references into the same font, and leaving them alone makes an
// "Ä" draw whatever landed at the diaeresis's old number.
func TestACompositesComponentsAreRenumbered(t *testing.T) {
	f := parse(t, sfnttest.Font(sfnttest.Cmap4(), sfnttest.NumGlyphs))
	// Ask for the composite at new id 1 and its components after it, so that
	// every reference has to move: glyph 1 stays 1, glyph 2 becomes 3.
	data, final, err := f.Subset([]uint16{0, sfnttest.GlyphComp, sfnttest.GlyphA, sfnttest.GlyphB})
	if err != nil {
		t.Fatal(err)
	}
	if len(final) != 4 {
		t.Fatalf("the subset holds %v, want all four glyphs", final)
	}

	refs := componentsOf(t, data, 1)
	if len(refs) != 2 {
		t.Fatalf("the composite refers to %d glyphs, want two", len(refs))
	}
	// sfnttest.GlyphA is at new id 2 and sfnttest.GlyphB at new id 3.
	if refs[0] != 2 || refs[1] != 3 {
		t.Errorf("the composite refers to glyphs %v, want the new ids 2 and 3", refs)
	}
}

// componentsOf reads the glyph ids a composite refers to straight out of a
// font file, so the assertion is about the bytes rather than about the
// package's own reading of them.
func componentsOf(t *testing.T, font []byte, glyph int) []uint16 {
	t.Helper()
	tables := map[string][]byte{}
	n := int(binary.BigEndian.Uint16(font[4:]))
	for i := range n {
		rec := font[12+16*i:]
		off := int(binary.BigEndian.Uint32(rec[8:]))
		length := int(binary.BigEndian.Uint32(rec[12:]))
		tables[string(rec[0:4])] = font[off : off+length]
	}
	head, loca, glyf := tables["head"], tables["loca"], tables["glyf"]
	long := int16(binary.BigEndian.Uint16(head[50:])) != 0
	at := func(i int) uint32 {
		if long {
			return binary.BigEndian.Uint32(loca[4*i:])
		}
		return uint32(binary.BigEndian.Uint16(loca[2*i:])) * 2
	}
	d := glyf[at(glyph):at(glyph+1)]
	if int16(binary.BigEndian.Uint16(d)) >= 0 {
		t.Fatalf("glyph %d is not a composite", glyph)
	}

	var out []uint16
	for p := 10; ; {
		flags := binary.BigEndian.Uint16(d[p:])
		out = append(out, binary.BigEndian.Uint16(d[p+2:]))
		p += 4
		if flags&0x0001 != 0 {
			p += 4
		} else {
			p += 2
		}
		switch {
		case flags&0x0008 != 0:
			p += 2
		case flags&0x0040 != 0:
			p += 4
		case flags&0x0080 != 0:
			p += 8
		}
		if flags&0x0020 == 0 {
			return out
		}
	}
}

// A subset carries one full metric per glyph, so the tail every reader has to
// expand is gone and the advances still say what they said.
func TestASubsetKeepsTheAdvances(t *testing.T) {
	f := parse(t, sfnttest.Font(sfnttest.Cmap4(), 2))
	order := []uint16{0, sfnttest.GlyphComp, sfnttest.GlyphA}
	data, final, err := f.Subset(order)
	if err != nil {
		t.Fatal(err)
	}
	sub := parse(t, data)
	for i, g := range final {
		if got, want := sub.Advance(uint16(i)), f.Advance(g); got != want {
			t.Errorf("subset glyph %d (was %d) advances %d, want %d", i, g, got, want)
		}
	}
}

// The output is a pure function of its input, which is the rule everything
// else in this repository that could have reached for a map follows.
func TestASubsetIsReproducible(t *testing.T) {
	f := parse(t, sfnttest.Font(sfnttest.Cmap4(), sfnttest.NumGlyphs))
	a, _, err := f.Subset([]uint16{0, sfnttest.GlyphComp, sfnttest.GlyphA})
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := f.Subset([]uint16{0, sfnttest.GlyphComp, sfnttest.GlyphA})
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Error("two subsets of the same glyphs came out different")
	}
}

// A subset has to start at .notdef: every consumer assumes glyph 0 is it, and
// a font whose glyph 0 is an "A" draws an A wherever a glyph is missing.
func TestASubsetMustStartAtNotdef(t *testing.T) {
	f := parse(t, sfnttest.Font(sfnttest.Cmap4(), sfnttest.NumGlyphs))
	if _, _, err := f.Subset([]uint16{sfnttest.GlyphA}); !errors.Is(err, sfnt.ErrFormat) {
		t.Errorf("error is %v, want ErrFormat", err)
	}
	if _, _, err := f.Subset(nil); err == nil {
		t.Error("an empty subset was accepted")
	}
}

// The failures are refusals rather than half-reads, because a font this
// package half-read is a document nobody can open.
func TestParseRefusesWhatItCannotRead(t *testing.T) {
	cases := map[string][]byte{
		"empty":      nil,
		"not a font": []byte("this is not a font at all, not even close"),
		"a collection": func() []byte {
			b := append([]byte(nil), sfnttest.Font(sfnttest.Cmap4(), sfnttest.NumGlyphs)...)
			binary.BigEndian.PutUint32(b, 0x74746366)
			return b
		}(),
		"truncated table": func() []byte {
			b := append([]byte(nil), sfnttest.Font(sfnttest.Cmap4(), sfnttest.NumGlyphs)...)
			// Claim the first table is enormous.
			binary.BigEndian.PutUint32(b[12+12:], 1<<30)
			return b
		}(),
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := sfnt.Parse(b); !errors.Is(err, sfnt.ErrFormat) {
				t.Errorf("error is %v, want ErrFormat", err)
			}
		})
	}
}

// A CFF font is recognised and reported as what it is: the outlines are
// charstrings, which this package does not cut up, so a caller embedding one
// writes the whole file.
func TestACFFFontIsRecognisedAndNotSubset(t *testing.T) {
	raw := sfnttest.CFFFont()
	f := parse(t, raw)
	if !f.CFF() || f.CanSubset() {
		t.Fatalf("CFF()=%v CanSubset()=%v, want true and false", f.CFF(), f.CanSubset())
	}
	// Everything that is not the outlines still reads: the metrics and the
	// character map are the same tables in either flavour, which is what lets
	// one code path measure both.
	if f.Advance(sfnttest.GlyphA) != int(sfnttest.Advances[sfnttest.GlyphA]) {
		t.Error("a CFF font's advances did not read back")
	}
	if f.GlyphIndex('A') != sfnttest.GlyphA {
		t.Error("a CFF font's character map did not read back")
	}
	if string(f.Raw()) != string(raw) {
		t.Error("Raw did not hand back the file it was parsed from")
	}
	if _, _, err := f.Subset([]uint16{0, sfnttest.GlyphA}); !errors.Is(err, sfnt.ErrFormat) {
		t.Errorf("subsetting a CFF font gave %v, want ErrFormat", err)
	}
}
