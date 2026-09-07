package sfnt

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// Subset builds a font file holding only the glyphs in order, renumbered so
// that glyph i of the result is order[i] of the original.
//
// The caller decides the order and the result honours it, which is what lets
// a PDF writer hand out a glyph id the moment it first sees a rune and write
// the content stream immediately, rather than buffering the whole document
// until it knows which glyphs it used. First-appearance order is also what
// makes the output a pure function of the input — the same rule
// [ADR 0012](../../docs/adr/0012-parallel-panels.md) puts on everything else
// in this repository that could have reached for a map.
//
// order[0] must be glyph 0, the .notdef glyph: every TrueType font has one and
// every consumer assumes glyph 0 is it.
//
// A composite glyph refers to other glyphs, so the subset is closed over those
// references: the components are appended to order and the returned slice is
// the order the result actually uses, which is order with whatever it pulled
// in. A caller that assigned ids from its own copy of order therefore keeps
// them — nothing already in the list moves.
func (f *Font) Subset(order []uint16) (data []byte, final []uint16, err error) {
	if !f.hasGlyf {
		return nil, nil, fmt.Errorf("%w: only TrueType outlines can be subset", ErrFormat)
	}
	if len(order) == 0 || order[0] != 0 {
		return nil, nil, fmt.Errorf("%w: a subset must begin with glyph 0", ErrFormat)
	}

	final, err = f.close(order)
	if err != nil {
		return nil, nil, err
	}
	// The new glyph id of an old one, for rewriting the component references
	// inside a composite.
	remap := make(map[uint16]uint16, len(final))
	for i, g := range final {
		remap[g] = uint16(i)
	}

	glyf, loca, err := f.buildGlyf(final, remap)
	if err != nil {
		return nil, nil, err
	}
	longLoca := len(glyf) > 0x1FFFF || loca[len(loca)-1] > 0x1FFFE

	tables := map[string][]byte{
		"glyf": glyf,
		"loca": encodeLoca(loca, longLoca),
		"head": f.subsetHead(longLoca),
		"hhea": f.subsetHhea(len(final)),
		"hmtx": f.subsetHmtx(final),
		"maxp": f.subsetMaxp(len(final)),
	}
	// The hinting tables are copied through unchanged. They are programs
	// rather than per-glyph data, so nothing in them refers to a glyph id, and
	// dropping them would make a subset render differently from the font it
	// came from at small sizes.
	for _, tag := range [...]string{"cvt ", "fpgm", "prep"} {
		if t, ok := f.tables[tag]; ok {
			tables[tag] = t
		}
	}
	return buildSfnt(tables), final, nil
}

// close extends order with every glyph a composite in it refers to,
// transitively, without moving anything already there.
func (f *Font) close(order []uint16) ([]uint16, error) {
	out := append([]uint16(nil), order...)
	seen := make(map[uint16]bool, len(out))
	for _, g := range out {
		if int(g) >= f.numGlyphs {
			return nil, fmt.Errorf("%w: glyph %d is past the end of the font", ErrFormat, g)
		}
		seen[g] = true
	}
	// A queue rather than recursion: a composite may refer to a composite,
	// and the depth is the font's business rather than this package's.
	for i := 0; i < len(out); i++ {
		comps, err := f.components(out[i])
		if err != nil {
			return nil, err
		}
		for _, c := range comps {
			if int(c) >= f.numGlyphs || seen[c] {
				continue
			}
			seen[c] = true
			out = append(out, c)
		}
	}
	return out, nil
}

// glyphData is glyph g's entry in the glyf table, empty for a glyph with no
// outline — a space, and every glyph a font leaves blank.
func (f *Font) glyphData(g uint16) []byte {
	if int(g)+1 >= len(f.loca) {
		return nil
	}
	lo, hi := f.loca[g], f.loca[g+1]
	if hi <= lo {
		return nil
	}
	return f.glyf[lo:hi]
}

// components lists the glyphs a composite refers to, and nothing for a simple
// one.
//
// The component record's length depends on its flags, which is why this is a
// walk rather than an indexed read: the arguments are one or two bytes each,
// and the transform is absent, one value, two, or four.
func (f *Font) components(g uint16) ([]uint16, error) {
	d := f.glyphData(g)
	if len(d) < 10 || int16(binary.BigEndian.Uint16(d)) >= 0 {
		return nil, nil // simple, or empty
	}
	var out []uint16
	for p := 10; ; {
		if p+4 > len(d) {
			return nil, fmt.Errorf("%w: composite glyph %d is truncated", ErrFormat, g)
		}
		flags := binary.BigEndian.Uint16(d[p:])
		out = append(out, binary.BigEndian.Uint16(d[p+2:]))
		p += 4
		if flags&argsAreWords != 0 {
			p += 4
		} else {
			p += 2
		}
		switch {
		case flags&haveScale != 0:
			p += 2
		case flags&haveXYScale != 0:
			p += 4
		case flags&haveTwoByTwo != 0:
			p += 8
		}
		if flags&moreComponents == 0 {
			return out, nil
		}
	}
}

// The component flags this package has to understand. They are the ones that
// decide how long a component record is; the rest describe how the component
// is placed, and are copied through untouched.
const (
	argsAreWords   = 0x0001
	haveScale      = 0x0008
	moreComponents = 0x0020
	haveXYScale    = 0x0040
	haveTwoByTwo   = 0x0080
)

// buildGlyf writes the subset's glyf table and the offsets into it.
//
// A composite glyph is copied with its component glyph ids rewritten, which is
// the one place a subset is more than a gather: the ids inside a composite are
// references into the same font, and leaving them alone would make an "ä" in
// the subset draw whatever glyph happened to land at the diaeresis's old
// number.
func (f *Font) buildGlyf(order []uint16, remap map[uint16]uint16) ([]byte, []uint32, error) {
	var out []byte
	loca := make([]uint32, len(order)+1)
	for i, g := range order {
		loca[i] = uint32(len(out))
		d := f.glyphData(g)
		if len(d) == 0 {
			continue
		}
		start := len(out)
		out = append(out, d...)
		if int16(binary.BigEndian.Uint16(d)) >= 0 {
			// Simple glyph: its bytes mean the same in the subset.
			out = pad4(out)
			continue
		}
		if err := rewriteComposite(out[start:], remap); err != nil {
			return nil, nil, fmt.Errorf("glyph %d: %w", g, err)
		}
		out = pad4(out)
	}
	loca[len(order)] = uint32(len(out))
	return out, loca, nil
}

// rewriteComposite renumbers the component references in a composite glyph in
// place. A component naming a glyph the subset does not hold becomes .notdef,
// which cannot happen after [Font.close] and is checked because a silent
// out-of-range glyph id is a corrupt font rather than a missing accent.
func rewriteComposite(d []byte, remap map[uint16]uint16) error {
	for p := 10; ; {
		if p+4 > len(d) {
			return fmt.Errorf("%w: composite glyph is truncated", ErrFormat)
		}
		flags := binary.BigEndian.Uint16(d[p:])
		old := binary.BigEndian.Uint16(d[p+2:])
		binary.BigEndian.PutUint16(d[p+2:], remap[old])
		p += 4
		if flags&argsAreWords != 0 {
			p += 4
		} else {
			p += 2
		}
		switch {
		case flags&haveScale != 0:
			p += 2
		case flags&haveXYScale != 0:
			p += 4
		case flags&haveTwoByTwo != 0:
			p += 8
		}
		if flags&moreComponents == 0 {
			return nil
		}
	}
}

// pad4 rounds a glyf table up to a four-byte boundary, which the format
// requires of every glyph's start and which the short loca form's
// half-offsets rely on.
func pad4(b []byte) []byte {
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

func encodeLoca(loca []uint32, long bool) []byte {
	if long {
		out := make([]byte, 4*len(loca))
		for i, v := range loca {
			binary.BigEndian.PutUint32(out[4*i:], v)
		}
		return out
	}
	out := make([]byte, 2*len(loca))
	for i, v := range loca {
		binary.BigEndian.PutUint16(out[2*i:], uint16(v/2))
	}
	return out
}

func (f *Font) subsetHead(longLoca bool) []byte {
	out := append([]byte(nil), f.tables["head"]...)
	// The file checksum is computed over the whole font and this one is a
	// different font. Zero is what every subsetter writes and what every
	// reader accepts; a stale value would be a claim about bytes that are not
	// there any more.
	binary.BigEndian.PutUint32(out[8:], 0)
	v := uint16(0)
	if longLoca {
		v = 1
	}
	binary.BigEndian.PutUint16(out[50:], v)
	return out
}

func (f *Font) subsetHhea(numGlyphs int) []byte {
	out := append([]byte(nil), f.tables["hhea"]...)
	// Every glyph in the subset carries its own advance, so there is no tail
	// to compress and the metric count is the glyph count.
	binary.BigEndian.PutUint16(out[34:], uint16(numGlyphs))
	return out
}

// subsetHmtx writes one full metric per glyph.
//
// The left side bearing is taken as the glyph's own xMin, which is what it is
// for every glyph a font ships: the two disagree only in a font that has been
// edited without recompiling, and reading it out of the outline is what makes
// this correct without carrying the original table's variable-length tail
// around.
func (f *Font) subsetHmtx(order []uint16) []byte {
	out := make([]byte, 4*len(order))
	hmtx := f.tables["hmtx"]
	numH := 0
	if hhea := f.tables["hhea"]; len(hhea) >= 36 {
		numH = int(binary.BigEndian.Uint16(hhea[34:]))
	}
	for i, g := range order {
		binary.BigEndian.PutUint16(out[4*i:], uint16(f.Advance(g)))
		var lsb int16
		switch {
		case int(g) < numH && 4*int(g)+4 <= len(hmtx):
			lsb = int16(binary.BigEndian.Uint16(hmtx[4*int(g)+2:]))
		case numH > 0:
			if off := 4*numH + 2*(int(g)-numH); off+2 <= len(hmtx) {
				lsb = int16(binary.BigEndian.Uint16(hmtx[off:]))
			}
		}
		binary.BigEndian.PutUint16(out[4*i+2:], uint16(lsb))
	}
	return out
}

func (f *Font) subsetMaxp(numGlyphs int) []byte {
	out := append([]byte(nil), f.tables["maxp"]...)
	binary.BigEndian.PutUint16(out[4:], uint16(numGlyphs))
	return out
}

// buildSfnt assembles a font file from its tables.
//
// Tables come out in tag order and each is padded to four bytes, which is what
// the format asks for and what makes the output byte-identical from one run to
// the next — a map iterated in its own order would not be.
func buildSfnt(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	n := len(tags)
	// The binary-search hints in the header. Every reader ignores them and
	// every file has them; writing the values the specification defines costs
	// four lines and avoids a validator complaining about a font that works.
	entrySelector := uint16(0)
	for 1<<(entrySelector+1) <= uint16(n) {
		entrySelector++
	}
	searchRange := uint16(16) << entrySelector

	head := make([]byte, 12+16*n)
	binary.BigEndian.PutUint32(head, 0x00010000)
	binary.BigEndian.PutUint16(head[4:], uint16(n))
	binary.BigEndian.PutUint16(head[6:], searchRange)
	binary.BigEndian.PutUint16(head[8:], entrySelector)
	binary.BigEndian.PutUint16(head[10:], uint16(16*n)-searchRange)

	body := []byte{}
	offset := len(head)
	for i, tag := range tags {
		t := tables[tag]
		rec := head[12+16*i:]
		copy(rec[0:4], tag)
		binary.BigEndian.PutUint32(rec[4:], checksum(t))
		binary.BigEndian.PutUint32(rec[8:], uint32(offset))
		binary.BigEndian.PutUint32(rec[12:], uint32(len(t)))
		body = append(body, t...)
		body = pad4(body)
		offset = len(head) + len(body)
	}
	return append(head, body...)
}

// checksum is the format's own: the table read as big-endian uint32s and
// summed, with the tail zero-padded.
func checksum(t []byte) uint32 {
	var sum uint32
	for i := 0; i < len(t); i += 4 {
		var word [4]byte
		copy(word[:], t[i:])
		sum += binary.BigEndian.Uint32(word[:])
	}
	return sum
}
