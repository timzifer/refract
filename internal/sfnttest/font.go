// Package sfnttest builds a font file byte by byte.
//
// A test that needs a real font needs either a binary nobody can review or a
// font whose every byte the test wrote itself, and this is the second. It is a
// package rather than a test file because two packages need it — the parser
// and subsetter in internal/sfnt, and the PDF backend that embeds what they
// produce — and a font builder copied into both would be two fonts that drift.
//
// Four glyphs, one of them a composite, and a cmap in whichever of the two
// supported formats the caller asks for. It is enough to exercise the table
// directory, the metric tail, the composite closure and the glyph
// renumbering, and small enough that a failure names a byte rather than a
// font.
package sfnttest

import (
	"encoding/binary"
	"sort"
)

// The design grid and the glyph count of the font this package builds.
const (
	UnitsPerEm = 1000
	NumGlyphs  = 4
)

// The glyphs: 0 is .notdef and empty, 1 and 2 are simple triangles, 3 is a
// composite of 1 and 2 — which is what makes the subset's closure and its
// renumbering testable at all. GlyphA is reached by 'A', GlyphB by 'B' and
// GlyphComp by 'Ä'.
const (
	GlyphNotdef = 0
	GlyphA      = 1
	GlyphB      = 2
	GlyphComp   = 3
)

// Advances is the advance width of each glyph, in font units.
var Advances = [NumGlyphs]uint16{500, 600, 700, 650}

// simpleGlyph is one contour of three points, which is the smallest outline
// that is not degenerate.
func simpleGlyph(x0, y0 int16) []byte {
	var b []byte
	u16 := func(v uint16) { b = binary.BigEndian.AppendUint16(b, v) }
	i16 := func(v int16) { u16(uint16(v)) }

	i16(1)                          // one contour
	i16(x0)                         // xMin
	i16(y0)                         // yMin
	i16(x0 + 300)                   // xMax
	i16(y0 + 400)                   // yMax
	u16(2)                          // the last point of contour 0
	u16(0)                          // no instructions
	b = append(b, 0x01, 0x01, 0x01) // three on-curve points, long coordinates
	i16(x0)
	i16(300)
	i16(-300)
	i16(y0)
	i16(400)
	i16(-400)
	return b
}

// compositeGlyph refers to two other glyphs, each with a two-byte offset,
// which is the record shape the subsetter has to walk to find them.
func compositeGlyph(a, c uint16) []byte {
	var b []byte
	u16 := func(v uint16) { b = binary.BigEndian.AppendUint16(b, v) }
	i16 := func(v int16) { u16(uint16(v)) }

	i16(-1) // composite
	i16(0)
	i16(0)
	i16(700)
	i16(800)

	const argsAreWords, argsAreXY, more = 0x0001, 0x0002, 0x0020
	u16(argsAreWords | argsAreXY | more)
	u16(a)
	i16(0)
	i16(0)
	u16(argsAreWords | argsAreXY)
	u16(c)
	i16(100)
	i16(50)
	return b
}

// Cmap4 maps 'A' and 'B' to glyphs 1 and 2 and 'Ä' to the composite, in the
// segmented format every Windows font carries.
func Cmap4() []byte {
	type seg struct {
		start, end uint16
		delta      int16
	}
	segs := []seg{
		{'A', 'B', GlyphA - 'A'},
		{0xC4, 0xC4, GlyphComp - 0xC4},
		{0xFFFF, 0xFFFF, 1},
	}
	n := len(segs)
	var b []byte
	u16 := func(v uint16) { b = binary.BigEndian.AppendUint16(b, v) }

	u16(4)
	u16(uint16(16 + 8*n)) // length
	u16(0)                // language
	u16(uint16(2 * n))
	u16(2) // the search hints, which nothing reads
	u16(1)
	u16(0)
	for _, s := range segs {
		u16(s.end)
	}
	u16(0) // the reserved pad
	for _, s := range segs {
		u16(s.start)
	}
	for _, s := range segs {
		u16(uint16(s.delta))
	}
	for range segs {
		u16(0) // no range offsets: every segment is a plain delta
	}
	return wrapCmap(3, 1, b)
}

// Cmap12 is the same mapping in the flat format a font needs to reach past the
// Basic Multilingual Plane, plus one code point that actually is past it.
func Cmap12() []byte {
	groups := [][3]uint32{
		{'A', 'B', GlyphA},
		{0xC4, 0xC4, GlyphComp},
		{0x1F600, 0x1F600, GlyphB},
	}
	var b []byte
	u16 := func(v uint16) { b = binary.BigEndian.AppendUint16(b, v) }
	u32 := func(v uint32) { b = binary.BigEndian.AppendUint32(b, v) }

	u16(12)
	u16(0)
	u32(uint32(16 + 12*len(groups)))
	u32(0) // language
	u32(uint32(len(groups)))
	for _, g := range groups {
		u32(g[0])
		u32(g[1])
		u32(g[2])
	}
	return wrapCmap(3, 10, b)
}

// wrapCmap puts one subtable in a cmap table of its own.
func wrapCmap(platform, encoding uint16, sub []byte) []byte {
	var b []byte
	b = binary.BigEndian.AppendUint16(b, 0) // version
	b = binary.BigEndian.AppendUint16(b, 1) // one subtable
	b = binary.BigEndian.AppendUint16(b, platform)
	b = binary.BigEndian.AppendUint16(b, encoding)
	b = binary.BigEndian.AppendUint32(b, 12)
	return append(b, sub...)
}

// Font assembles the whole file. numH is how many full horizontal metrics the
// hmtx table carries: fewer than the glyph count exercises the tail every
// reader has to expand, which is the one part of the format that is easy to
// read as though it were an array.
func Font(cmapTable []byte, numH int) []byte {
	glyphs := [NumGlyphs][]byte{
		GlyphNotdef: nil,
		GlyphA:      simpleGlyph(0, 0),
		GlyphB:      simpleGlyph(100, 100),
		GlyphComp:   compositeGlyph(GlyphA, GlyphB),
	}
	var glyf []byte
	loca := make([]uint32, NumGlyphs+1)
	for i, g := range glyphs {
		loca[i] = uint32(len(glyf))
		glyf = append(glyf, g...)
		for len(glyf)%4 != 0 {
			glyf = append(glyf, 0)
		}
	}
	loca[NumGlyphs] = uint32(len(glyf))

	locaTable := make([]byte, 4*len(loca))
	for i, v := range loca {
		binary.BigEndian.PutUint32(locaTable[4*i:], v)
	}

	head := make([]byte, 54)
	binary.BigEndian.PutUint32(head, 0x00010000)
	binary.BigEndian.PutUint16(head[18:], UnitsPerEm)
	binary.BigEndian.PutUint16(head[36:], u16of(-50))  // xMin
	binary.BigEndian.PutUint16(head[38:], u16of(-200)) // yMin
	binary.BigEndian.PutUint16(head[40:], 800)         // xMax
	binary.BigEndian.PutUint16(head[42:], 900)         // yMax
	binary.BigEndian.PutUint16(head[50:], 1)           // long loca

	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[4:], 800)         // ascender
	binary.BigEndian.PutUint16(hhea[6:], u16of(-200)) // descender
	binary.BigEndian.PutUint16(hhea[34:], uint16(numH))

	hmtx := make([]byte, 4*numH+2*(NumGlyphs-numH))
	for i := range numH {
		binary.BigEndian.PutUint16(hmtx[4*i:], Advances[i])
		binary.BigEndian.PutUint16(hmtx[4*i+2:], uint16(int16(10+i)))
	}
	for i := numH; i < NumGlyphs; i++ {
		binary.BigEndian.PutUint16(hmtx[4*numH+2*(i-numH):], uint16(int16(20+i)))
	}

	maxp := make([]byte, 32)
	binary.BigEndian.PutUint32(maxp, 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:], NumGlyphs)

	os2 := make([]byte, 96)
	binary.BigEndian.PutUint16(os2, 4)       // version 4, so sCapHeight is there
	binary.BigEndian.PutUint16(os2[4:], 700) // usWeightClass
	binary.BigEndian.PutUint16(os2[88:], 720)

	post := make([]byte, 32)
	binary.BigEndian.PutUint32(post, 0x00030000)
	binary.BigEndian.PutUint32(post[4:], u32of(-12*65536)) // italic angle

	return assemble(map[string][]byte{
		"head": head, "hhea": hhea, "hmtx": hmtx, "maxp": maxp,
		"loca": locaTable, "glyf": glyf, "cmap": cmapTable,
		"OS/2": os2, "post": post, "name": nameTable("Refract Test"),
	})
}

// u16of and u32of write a signed value as the unsigned field the format
// stores it in. They exist because a negative untyped constant cannot be
// converted to an unsigned type in a constant expression, and spelling the
// two's complement out in hex would hide what the number is.
func u16of(v int16) uint16 { return uint16(v) }
func u32of(v int32) uint32 { return uint32(v) }

// nameTable holds one record: the PostScript name, in the Windows Unicode
// encoding a real font uses for it.
func nameTable(name string) []byte {
	var utf16 []byte
	for _, r := range name {
		utf16 = binary.BigEndian.AppendUint16(utf16, uint16(r))
	}
	var b []byte
	u16 := func(v uint16) { b = binary.BigEndian.AppendUint16(b, v) }
	u16(0) // format
	u16(1) // one record
	u16(6 + 12)
	u16(3) // Windows
	u16(1) // Unicode BMP
	u16(0x0409)
	u16(6) // the PostScript name
	u16(uint16(len(utf16)))
	u16(0)
	return append(b, utf16...)
}

func assemble(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	n := len(tags)
	head := make([]byte, 12+16*n)
	binary.BigEndian.PutUint32(head, 0x00010000)
	binary.BigEndian.PutUint16(head[4:], uint16(n))

	var body []byte
	for i, tag := range tags {
		t := tables[tag]
		rec := head[12+16*i:]
		copy(rec[0:4], tag)
		binary.BigEndian.PutUint32(rec[8:], uint32(len(head)+len(body)))
		binary.BigEndian.PutUint32(rec[12:], uint32(len(t)))
		body = append(body, t...)
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
	}
	return append(head, body...)
}

// CFFFont builds a font whose outlines are CFF charstrings rather than glyf
// entries — an `.otf`, in the shape a reader meets one.
//
// The CFF table's contents are a placeholder, and deliberately: nothing in
// refract interprets charstrings. What such a font exercises is the half that
// is refract's own — that the format is recognised, that it is reported as
// unsubsettable, and that the PDF wrapper written around it is the CID font a
// CFF program belongs in rather than the one a glyf program does.
func CFFFont() []byte {
	full := Font(Cmap4(), NumGlyphs)
	tables := map[string][]byte{}
	n := int(binary.BigEndian.Uint16(full[4:]))
	for i := range n {
		rec := full[12+16*i:]
		off := int(binary.BigEndian.Uint32(rec[8:]))
		length := int(binary.BigEndian.Uint32(rec[12:]))
		tag := string(rec[0:4])
		switch tag {
		case "glyf", "loca":
			// A CFF font has neither, which is the whole difference.
		default:
			tables[tag] = full[off : off+length]
		}
	}
	tables["CFF "] = []byte("a CFF program would go here")

	b := assemble(tables)
	binary.BigEndian.PutUint32(b, 0x4F54544F) // 'OTTO'
	return b
}
