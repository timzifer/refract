// Package sfnt reads the parts of a TrueType or OpenType font that embedding
// one in a PDF needs, and cuts a subset of it down to the glyphs a document
// actually uses.
//
// It is deliberately not a font library. It does not rasterize, it does not
// shape, it does not kern, and it reads no table it has no use for. What it
// answers is the four questions [github.com/timzifer/refract/backend/pdf] has
// to ask before it can write a font into a document: which glyph is this rune,
// how wide is that glyph, what does the font descriptor say about the face,
// and what is the smallest file that still draws these glyphs.
//
// It lives in the core module because the core module has no dependencies and
// keeps none — see AGENTS.md — and because parsing a table-directory format is
// a few hundred lines of `binary.BigEndian` and nothing else.
//
// # What it supports
//
// TrueType outlines — a `glyf` and `loca` pair — are parsed and can be
// subset. CFF outlines, which is what an `.otf` file usually carries, are
// recognised and *not* subset: the charstring format is a second interpreter
// and cutting one up correctly is a different project. Such a font is embedded
// whole, which is a correct document and a larger one; [Font.CanSubset] is how
// a caller finds out which it has.
//
// Bitmap-only fonts, `ttc` collections and variable-font instancing are out of
// scope and are refused rather than half-read.
package sfnt

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrFormat reports a file this package cannot read.
var ErrFormat = errors.New("refract/internal/sfnt: unsupported font file")

// Font is a parsed font.
//
// It borrows the bytes it was parsed from: the tables are slices into the
// caller's buffer, so the buffer must outlive the Font. That is the same
// bargain [github.com/timzifer/refract/data.Float64Columns] makes about a
// column, for the same reason — a font file is a megabyte and copying it to
// read four tables would be the only allocation in the package worth naming.
type Font struct {
	raw    []byte
	tables map[string][]byte

	unitsPerEm int
	numGlyphs  int
	longLoca   bool

	// The horizontal metrics, expanded: one advance per glyph, in font units.
	// The table stores a run of pairs and then a tail that repeats the last
	// advance, and every reader has to undo that — doing it once here is
	// cheaper than doing it per glyph and much easier to get right.
	advances []uint16

	loca []uint32 // numGlyphs+1 offsets into glyf, absolute within the table
	glyf []byte

	// The descriptor's numbers, in font units.
	ascent, descent      int
	capHeight            int
	xMin, yMin           int
	xMax, yMax           int
	italicAngle          float64
	weightClass          int
	postScriptName       string
	unicode              cmap
	hasOutlines, hasGlyf bool
	hasUnicode           bool
}

// Parse reads a font file.
func Parse(b []byte) (*Font, error) {
	if len(b) < 12 {
		return nil, fmt.Errorf("%w: too short to hold a table directory", ErrFormat)
	}
	switch tag := binary.BigEndian.Uint32(b); tag {
	case 0x00010000, 0x74727565: // 1.0, and Apple's 'true'
	case 0x4F54544F: // 'OTTO', CFF outlines
	case 0x74746366: // 'ttcf'
		return nil, fmt.Errorf("%w: a font collection holds several fonts; extract one first", ErrFormat)
	default:
		return nil, fmt.Errorf("%w: unrecognised sfnt version %#08x", ErrFormat, tag)
	}

	n := int(binary.BigEndian.Uint16(b[4:]))
	if len(b) < 12+16*n {
		return nil, fmt.Errorf("%w: table directory runs past the end of the file", ErrFormat)
	}
	f := &Font{raw: b, tables: make(map[string][]byte, n)}
	for i := range n {
		rec := b[12+16*i:]
		tag := string(rec[0:4])
		off, length := int(binary.BigEndian.Uint32(rec[8:])), int(binary.BigEndian.Uint32(rec[12:]))
		if off < 0 || length < 0 || off > len(b) || off+length > len(b) {
			// A table that runs past the end is a truncated file, and a font
			// with a truncated table is not one this package will half-read.
			return nil, fmt.Errorf("%w: table %q runs past the end of the file", ErrFormat, tag)
		}
		f.tables[tag] = b[off : off+length]
	}

	if err := f.readHead(); err != nil {
		return nil, err
	}
	if err := f.readMetrics(); err != nil {
		return nil, err
	}
	if err := f.readOutlines(); err != nil {
		return nil, err
	}
	f.readDescriptor()
	if err := f.readCmap(); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *Font) readHead() error {
	head, ok := f.tables["head"]
	if !ok || len(head) < 54 {
		return fmt.Errorf("%w: no usable head table", ErrFormat)
	}
	f.unitsPerEm = int(binary.BigEndian.Uint16(head[18:]))
	if f.unitsPerEm == 0 {
		return fmt.Errorf("%w: head says the em is zero units wide", ErrFormat)
	}
	f.xMin = int(int16(binary.BigEndian.Uint16(head[36:])))
	f.yMin = int(int16(binary.BigEndian.Uint16(head[38:])))
	f.xMax = int(int16(binary.BigEndian.Uint16(head[40:])))
	f.yMax = int(int16(binary.BigEndian.Uint16(head[42:])))
	f.longLoca = int16(binary.BigEndian.Uint16(head[50:])) != 0

	maxp, ok := f.tables["maxp"]
	if !ok || len(maxp) < 6 {
		return fmt.Errorf("%w: no usable maxp table", ErrFormat)
	}
	f.numGlyphs = int(binary.BigEndian.Uint16(maxp[4:]))
	if f.numGlyphs == 0 {
		return fmt.Errorf("%w: maxp says the font has no glyphs", ErrFormat)
	}
	return nil
}

func (f *Font) readMetrics() error {
	hhea, ok := f.tables["hhea"]
	if !ok || len(hhea) < 36 {
		return fmt.Errorf("%w: no usable hhea table", ErrFormat)
	}
	f.ascent = int(int16(binary.BigEndian.Uint16(hhea[4:])))
	f.descent = int(int16(binary.BigEndian.Uint16(hhea[6:])))
	numH := int(binary.BigEndian.Uint16(hhea[34:]))
	if numH == 0 {
		return fmt.Errorf("%w: hhea says the font has no horizontal metrics", ErrFormat)
	}

	hmtx, ok := f.tables["hmtx"]
	if !ok {
		return fmt.Errorf("%w: no hmtx table", ErrFormat)
	}
	// The table is numH pairs and then a tail of left side bearings that all
	// share the last advance. Expanding it here is what lets every reader
	// below index by glyph without repeating that rule.
	f.advances = make([]uint16, f.numGlyphs)
	last := uint16(0)
	for i := range f.numGlyphs {
		if i < numH {
			if 4*i+2 > len(hmtx) {
				return fmt.Errorf("%w: hmtx is shorter than hhea says", ErrFormat)
			}
			last = binary.BigEndian.Uint16(hmtx[4*i:])
		}
		f.advances[i] = last
	}
	return nil
}

func (f *Font) readOutlines() error {
	if _, cff := f.tables["CFF "]; cff {
		f.hasOutlines = true
		return nil
	}
	loca, hasLoca := f.tables["loca"]
	glyf, hasGlyf := f.tables["glyf"]
	if !hasLoca || !hasGlyf {
		return fmt.Errorf("%w: no outlines — neither glyf/loca nor CFF", ErrFormat)
	}
	f.hasOutlines, f.hasGlyf, f.glyf = true, true, glyf

	f.loca = make([]uint32, f.numGlyphs+1)
	if f.longLoca {
		if len(loca) < 4*(f.numGlyphs+1) {
			return fmt.Errorf("%w: loca is shorter than maxp says", ErrFormat)
		}
		for i := range f.loca {
			f.loca[i] = binary.BigEndian.Uint32(loca[4*i:])
		}
	} else {
		if len(loca) < 2*(f.numGlyphs+1) {
			return fmt.Errorf("%w: loca is shorter than maxp says", ErrFormat)
		}
		for i := range f.loca {
			// The short form stores half-offsets, which is what makes a
			// 64 kB glyf the format's limit.
			f.loca[i] = uint32(binary.BigEndian.Uint16(loca[2*i:])) * 2
		}
	}
	for i := 1; i < len(f.loca); i++ {
		if f.loca[i] < f.loca[i-1] || int(f.loca[i]) > len(f.glyf) {
			return fmt.Errorf("%w: loca is not a rising sequence inside glyf", ErrFormat)
		}
	}
	return nil
}

// readDescriptor fills in what the PDF font descriptor asks for, tolerating a
// font that carries none of it: every field here has a defensible fallback,
// and refusing to embed a font because it has no OS/2 table would be refusing
// a font that draws perfectly well.
func (f *Font) readDescriptor() {
	f.capHeight = f.ascent // a fallback, replaced below when OS/2 says
	if os2 := f.tables["OS/2"]; len(os2) >= 8 {
		f.weightClass = int(binary.BigEndian.Uint16(os2[4:]))
		version := binary.BigEndian.Uint16(os2)
		if version >= 2 && len(os2) >= 90 {
			if h := int(int16(binary.BigEndian.Uint16(os2[88:]))); h > 0 {
				f.capHeight = h
			}
		}
	}
	if post := f.tables["post"]; len(post) >= 8 {
		// A Fixed 16.16, and the sign is the whole point: an italic face
		// leans right and reports a negative angle.
		f.italicAngle = float64(int32(binary.BigEndian.Uint32(post[4:]))) / 65536
	}
	f.postScriptName = f.readName()
}

// readName finds the PostScript name (name ID 6), preferring a Windows
// Unicode record and falling back to a Macintosh Roman one.
//
// A font with no usable name record gets none here and the caller substitutes;
// a document whose font is called "Font" still draws, and refusing to embed
// over a missing string would be refusing over nothing.
func (f *Font) readName() string {
	name := f.tables["name"]
	if len(name) < 6 {
		return ""
	}
	count := int(binary.BigEndian.Uint16(name[2:]))
	storage := int(binary.BigEndian.Uint16(name[4:]))
	var mac string
	for i := range count {
		rec := 6 + 12*i
		if rec+12 > len(name) {
			break
		}
		platform := binary.BigEndian.Uint16(name[rec:])
		nameID := binary.BigEndian.Uint16(name[rec+6:])
		if nameID != 6 {
			continue
		}
		length := int(binary.BigEndian.Uint16(name[rec+8:]))
		off := storage + int(binary.BigEndian.Uint16(name[rec+10:]))
		if off < 0 || off+length > len(name) {
			continue
		}
		s := name[off : off+length]
		switch platform {
		case 3: // Windows: UTF-16BE, and a PostScript name is ASCII in it
			var out []byte
			for j := 0; j+1 < len(s); j += 2 {
				if s[j] == 0 {
					out = append(out, s[j+1])
				}
			}
			if len(out) > 0 {
				return string(out)
			}
		case 1: // Macintosh Roman
			mac = string(s)
		}
	}
	return mac
}

// UnitsPerEm is the font's design grid.
func (f *Font) UnitsPerEm() int { return f.unitsPerEm }

// NumGlyphs is how many glyphs the font has.
func (f *Font) NumGlyphs() int { return f.numGlyphs }

// PostScriptName is the font's own name, or "" when it carries none.
func (f *Font) PostScriptName() string { return f.postScriptName }

// CanSubset reports whether this font's outlines can be cut down. It is true
// for TrueType outlines and false for CFF ones — see the package comment.
func (f *Font) CanSubset() bool { return f.hasGlyf }

// CFF reports whether the outlines are CFF charstrings, which decides which
// PDF font subtype the caller has to write.
func (f *Font) CFF() bool { return f.hasOutlines && !f.hasGlyf }

// Advance is glyph g's advance width in font units.
func (f *Font) Advance(g uint16) int {
	if int(g) >= len(f.advances) {
		return 0
	}
	return int(f.advances[g])
}

// Ascent, Descent, CapHeight and the bounding box are the descriptor's
// numbers, in font units. Descent is negative, as the font stores it.
func (f *Font) Ascent() int    { return f.ascent }
func (f *Font) Descent() int   { return f.descent }
func (f *Font) CapHeight() int { return f.capHeight }
func (f *Font) BBox() (xMin, yMin, xMax, yMax int) {
	return f.xMin, f.yMin, f.xMax, f.yMax
}

// ItalicAngle is degrees anticlockwise from vertical, negative for a face that
// leans right.
func (f *Font) ItalicAngle() float64 { return f.italicAngle }

// WeightClass is the OS/2 usWeightClass, or 0 for a font that carries none.
func (f *Font) WeightClass() int { return f.weightClass }

// HasUnicodeMap reports whether the font carries a character map this package
// can read. A font without one — which includes every subset [Font.Subset]
// writes — answers 0 for every rune, because a CID font addresses glyphs by
// id and never asks.
func (f *Font) HasUnicodeMap() bool { return f.hasUnicode }

// GlyphIndex maps a rune to a glyph, returning 0 — .notdef — for a rune the
// font has no glyph for.
func (f *Font) GlyphIndex(r rune) uint16 { return f.unicode.lookup(r) }

// Raw returns the bytes the font was parsed from, which is what a caller
// embedding a font it cannot subset writes out.
func (f *Font) Raw() []byte { return f.raw }
