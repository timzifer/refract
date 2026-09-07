package pdf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"

	"github.com/timzifer/refract/internal/sfnt"
)

// embedded is one face the document carries its own copy of.
//
// The glyphs are numbered as they are first met rather than gathered at the
// end, and that is what makes writing a PDF a single pass: a content stream
// names glyphs by id, the ids of a *subset* depend on which glyphs the whole
// document used, and a writer that waited to find out would have to buffer
// every text run in the file. Numbering on first use inverts it — the id is
// known immediately and the subset is built at the end to match, in exactly
// that order.
//
// It is also why the order is first appearance rather than sorted: it is the
// one order that is a pure function of the drawing, which is the same rule
// group order, colour batching and panel replay all follow.
type embedded struct {
	font *sfnt.Font
	name string // the resource name in the content stream
	obj  int    // the Type0 font object, reserved early and filled at the end
	tag  string // the six-letter subset prefix

	glyphs []uint16          // cid → glyph in the original font
	cid    map[uint16]uint16 // glyph → cid
	text   map[uint16]rune   // cid → a rune that reaches it, for ToUnicode
}

func newEmbedded(f *sfnt.Font, name string, obj int, tag string) *embedded {
	// Cid 0 is glyph 0 is .notdef, in the subset exactly as in the font it
	// came from: every consumer assumes that, and a subset whose glyph 0 was
	// an "A" would draw an A wherever a glyph is missing.
	return &embedded{
		font: f, name: name, obj: obj, tag: tag,
		glyphs: []uint16{0},
		cid:    map[uint16]uint16{0: 0},
		text:   map[uint16]rune{},
	}
}

// encode writes s as the two-byte glyph ids an Identity-H font is addressed
// with, numbering glyphs it has not seen before.
func (e *embedded) encode(s string) []byte {
	out := make([]byte, 0, 2*len(s))
	for _, r := range s {
		g := e.font.GlyphIndex(r)
		cid, seen := e.cid[g]
		if !seen {
			cid = uint16(len(e.glyphs))
			e.glyphs = append(e.glyphs, g)
			e.cid[g] = cid
			e.text[cid] = r
		}
		out = binary.BigEndian.AppendUint16(out, cid)
	}
	return out
}

// advance is the width of s at the given size, in device units.
func (e *embedded) advance(s string, size float64) float64 {
	total := 0
	for _, r := range s {
		total += e.font.Advance(e.font.GlyphIndex(r))
	}
	return float64(total) * size / float64(e.font.UnitsPerEm())
}

func (e *embedded) ascent(size float64) float64 {
	return float64(e.font.Ascent()) * size / float64(e.font.UnitsPerEm())
}

// descent is positive, which is refract's convention and the opposite of the
// font's.
func (e *embedded) descent(size float64) float64 {
	return float64(-e.font.Descent()) * size / float64(e.font.UnitsPerEm())
}

// write emits the font's objects into the document.
//
// The shape is the one a CID font takes: a Type0 font with Identity-H
// encoding, a CIDFontType2 descendant that holds the metrics, a descriptor,
// and the font program itself. It is the only shape that reaches past 256
// glyphs, which is the whole point — a WinAnsi simple font has room for Latin-1
// and nothing else.
func (e *embedded) write(d *document, compress bool) error {
	prog, err := e.program()
	if err != nil {
		return err
	}
	f := e.font
	upem := float64(f.UnitsPerEm())
	// PDF measures a glyph in thousandths of the text size, so every font
	// number is scaled out of its own design grid. A font drawn on a 2048-unit
	// em and one drawn on 1000 then describe the same face.
	k := func(v int) int { return int(float64(v)*1000/upem + 0.5) }

	name := e.tag + "+" + e.baseName()
	dict := "/Subtype /CIDFontType2 /CIDToGIDMap /Identity"
	fileKey := "/FontFile2"
	if f.CFF() {
		// CFF outlines are embedded whole and described as OpenType, because
		// this package does not cut charstrings up — see internal/sfnt.
		dict = "/Subtype /CIDFontType0"
		fileKey = "/FontFile3 "
	}

	stream := fmt.Sprintf("/Length1 %d", len(prog))
	if f.CFF() {
		stream = "/Subtype /OpenType"
	}
	file := d.addStream(stream, prog, compress)

	xMin, yMin, xMax, yMax := f.BBox()
	desc := d.add([]byte(fmt.Sprintf(
		"<< /Type /FontDescriptor /FontName /%s /Flags 4 /FontBBox [%d %d %d %d] "+
			"/ItalicAngle %s /Ascent %d /Descent %d /CapHeight %d /StemV %d %s%d 0 R >>",
		name, k(xMin), k(yMin), k(xMax), k(yMax),
		fmtNum(f.ItalicAngle()), k(f.Ascent()), k(f.Descent()), k(f.CapHeight()),
		stemV(f.WeightClass()), fileKey, file)))

	descendant := d.add([]byte(fmt.Sprintf(
		"<< /Type /Font %s /BaseFont /%s "+
			"/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> "+
			"/FontDescriptor %d 0 R /DW 1000 /W %s >>",
		dict, name, desc, e.widths(k))))

	toUnicode := d.addStream("", e.toUnicode(), compress)
	d.set(e.obj, []byte(fmt.Sprintf(
		"<< /Type /Font /Subtype /Type0 /BaseFont /%s /Encoding /Identity-H "+
			"/DescendantFonts [%d 0 R] /ToUnicode %d 0 R >>",
		name, descendant, toUnicode)))
	return nil
}

// program is the bytes of the embedded font: a subset where one can be cut,
// and the whole file where it cannot.
func (e *embedded) program() ([]byte, error) {
	if !e.font.CanSubset() {
		return e.font.Raw(), nil
	}
	data, final, err := e.font.Subset(e.glyphs)
	if err != nil {
		return nil, fmt.Errorf("refract/backend/pdf: subsetting the embedded font: %w", err)
	}
	// The closure may have pulled in the components of a composite. Those get
	// cids too — they are in the file and the widths array describes every
	// glyph the file holds — but nothing addresses them, so no text changes.
	e.glyphs = final
	return data, nil
}

// widths is the /W array: runs of consecutive cids and their advances, in
// thousandths of the text size.
//
// It is written as runs rather than one entry per glyph because a subset of a
// CJK font is thousands of glyphs and the runs are what keeps the array a few
// lines rather than a few pages. Glyphs whose width is the default are left
// out entirely, which /DW is for.
func (e *embedded) widths(k func(int) int) string {
	var b bytes.Buffer
	b.WriteByte('[')
	i := 0
	for i < len(e.glyphs) {
		w := k(e.font.Advance(e.glyphs[i]))
		if w == 1000 {
			i++
			continue
		}
		j := i + 1
		for j < len(e.glyphs) && k(e.font.Advance(e.glyphs[j])) != 1000 {
			j++
		}
		if b.Len() > 1 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%d [", i)
		for n := i; n < j; n++ {
			if n > i {
				b.WriteByte(' ')
			}
			b.WriteString(strconv.Itoa(k(e.font.Advance(e.glyphs[n]))))
		}
		b.WriteByte(']')
		i = j
	}
	b.WriteByte(']')
	return b.String()
}

// toUnicode is the CMap that maps the document's glyph ids back to the text
// they were made from.
//
// Without it a PDF's glyphs are ink: nothing can be selected, copied, searched
// or read aloud, which for a chart means its labels are a picture again — the
// exact failure [ADR 0024](../../docs/adr/0024-accessibility.md) exists to
// avoid, reappearing one layer down.
func (e *embedded) toUnicode() []byte {
	cids := make([]uint16, 0, len(e.text))
	for cid := range e.text {
		cids = append(cids, cid)
	}
	// Sorted, because a map's order is not an order and this file is compared
	// byte for byte by the golden tests.
	sort.Slice(cids, func(i, j int) bool { return cids[i] < cids[j] })

	var b bytes.Buffer
	b.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n")
	b.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	b.WriteString("/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n")
	b.WriteString("1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")

	// A CMap allows at most a hundred entries per block, which is the format's
	// own limit rather than a style choice.
	for start := 0; start < len(cids); start += 100 {
		end := min(start+100, len(cids))
		fmt.Fprintf(&b, "%d beginbfchar\n", end-start)
		for _, cid := range cids[start:end] {
			fmt.Fprintf(&b, "<%04X> <%s>\n", cid, utf16Hex(e.text[cid]))
		}
		b.WriteString("endbfchar\n")
	}
	b.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return b.Bytes()
}

// utf16Hex writes a rune as the UTF-16BE hex a bfchar entry holds, surrogate
// pair and all — which is how a code point past the Basic Multilingual Plane
// survives into a reader's clipboard.
func utf16Hex(r rune) string {
	if r > 0xFFFF {
		r -= 0x10000
		return fmt.Sprintf("%04X%04X", 0xD800+(r>>10), 0xDC00+(r&0x3FF))
	}
	return fmt.Sprintf("%04X", r)
}

// baseName is the font's own PostScript name with the characters a PDF name
// cannot hold taken out.
func (e *embedded) baseName() string {
	name := e.font.PostScriptName()
	var out []byte
	for i := range len(name) {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return "EmbeddedFont"
	}
	return string(out)
}

// subsetTag is the six upper-case letters a subset's name is prefixed with.
//
// The specification asks for a tag unique to the subset so that two documents
// carrying different subsets of one font are not merged into one. It is
// derived from the face's index in this document rather than from a hash of
// the glyphs, because a hash would make the output depend on which labels the
// chart happened to draw and the golden files compare bytes.
func subsetTag(i int) string {
	var out [6]byte
	for j := range out {
		out[j] = byte('A' + (i+j*7)%26)
	}
	return string(out[:])
}

// stemV is the descriptor's required stem width, estimated from the weight
// class.
//
// PDF requires the field and no font carries it: it is a Type 1 notion that
// TrueType never had. Every embedder estimates, and the estimate only matters
// to a reader substituting a font it does not have — which cannot happen here,
// because the font is in the file.
func stemV(weightClass int) int {
	if weightClass <= 0 {
		weightClass = 400
	}
	return 50 + weightClass*weightClass/6400
}

// embeddedFace is an embedded font at one size, wearing the metric interface
// the rest of the backend measures through.
//
// It is the reason this backend keeps its promise about measurement with an
// embedded font as well as without one: the doc comment says refract measures
// PDF text with the font it draws with, and that has to stay true of a font
// the caller supplied — a label measured against Helvetica and drawn in Noto
// Sans would size its margin from the wrong typeface, which is exactly the
// approximation every other backend makes and this one does not.
type embeddedFace struct {
	e    *embedded
	size float64
}

func (f embeddedFace) Advance(s string) float64 { return f.e.advance(s, f.size) }
func (f embeddedFace) Ascent() float64          { return f.e.ascent(f.size) }
func (f embeddedFace) Descent() float64         { return f.e.descent(f.size) }
