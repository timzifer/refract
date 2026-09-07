package sfnt

import (
	"encoding/binary"
	"fmt"
)

// cmap is the character-to-glyph mapping, in whichever of the two shapes
// worth supporting the font carries.
//
// Format 4 is the segmented map every Windows font has had since TrueType
// began and covers the Basic Multilingual Plane. Format 12 is the flat
// grouped map a font needs to reach past it, which is what an emoji or a rare
// CJK ideograph lives in. Nothing else is read: format 0 and format 6 are
// single-byte and single-range maps that a font with a Unicode subtable has
// not used since the nineties, and half-reading a format is worse than
// refusing it.
type cmap struct {
	// The format-4 arrays, parallel and in segment order.
	ends, starts, deltas, rangeOffsets []uint16
	glyphIDs                           []byte // the array the range offsets point into
	segX2                              int

	// The format-12 groups, three uint32s each, in the table's own order.
	groups []byte
}

// readCmap finds the best Unicode subtable, and tolerates a font with none.
//
// A font with no cmap is not a broken font: it is what this package's own
// [Font.Subset] produces, because a PDF CID font addresses glyphs by id
// through Identity-H and never asks a character map anything. Refusing to
// parse one would mean this package could not read what it writes, which is
// the round trip its tests rest on. A caller that needs to map runes asks
// [Font.HasUnicodeMap] first.
func (f *Font) readCmap() error {
	t, ok := f.tables["cmap"]
	if !ok || len(t) < 4 {
		return nil
	}
	n := int(binary.BigEndian.Uint16(t[2:]))

	// The best subtable wins, and "best" is an order rather than a search:
	// a full-repertoire Unicode map, then a BMP one, then anything a
	// Unicode platform offered. Taking the first acceptable subtable instead
	// would pick a symbol map on a font that has both.
	best, bestRank := -1, -1
	for i := range n {
		rec := 4 + 8*i
		if rec+8 > len(t) {
			break
		}
		platform := binary.BigEndian.Uint16(t[rec:])
		encoding := binary.BigEndian.Uint16(t[rec+2:])
		off := int(binary.BigEndian.Uint32(t[rec+4:]))
		if off < 0 || off+4 > len(t) {
			continue
		}
		rank := -1
		switch {
		case platform == 3 && encoding == 10:
			rank = 3
		case platform == 0 && binary.BigEndian.Uint16(t[off:]) == 12:
			rank = 3
		case platform == 3 && encoding == 1:
			rank = 2
		case platform == 0:
			rank = 1
		}
		if rank > bestRank {
			best, bestRank = off, rank
		}
	}
	if best < 0 {
		return nil
	}
	if err := f.unicode.read(t[best:]); err != nil {
		return err
	}
	f.hasUnicode = true
	return nil
}

func (c *cmap) read(t []byte) error {
	if len(t) < 4 {
		return fmt.Errorf("%w: cmap subtable is truncated", ErrFormat)
	}
	switch format := binary.BigEndian.Uint16(t); format {
	case 4:
		return c.read4(t)
	case 12:
		return c.read12(t)
	default:
		return fmt.Errorf("%w: cmap subtable format %d", ErrFormat, format)
	}
}

func (c *cmap) read4(t []byte) error {
	if len(t) < 14 {
		return fmt.Errorf("%w: cmap format 4 header is truncated", ErrFormat)
	}
	c.segX2 = int(binary.BigEndian.Uint16(t[6:]))
	segs := c.segX2 / 2
	if segs == 0 || len(t) < 16+4*c.segX2 {
		return fmt.Errorf("%w: cmap format 4 is shorter than its segment count", ErrFormat)
	}
	read := func(off int) []uint16 {
		out := make([]uint16, segs)
		for i := range out {
			out[i] = binary.BigEndian.Uint16(t[off+2*i:])
		}
		return out
	}
	c.ends = read(14)
	c.starts = read(16 + c.segX2)
	c.deltas = read(16 + 2*c.segX2)
	c.rangeOffsets = read(16 + 3*c.segX2)
	// Everything after the range-offset array is the glyph id array, and the
	// offsets are measured from inside that array rather than from the table
	// — which is why it is kept as bytes and indexed relative to its start.
	c.glyphIDs = t[16+3*c.segX2:]
	return nil
}

func (c *cmap) read12(t []byte) error {
	if len(t) < 16 {
		return fmt.Errorf("%w: cmap format 12 header is truncated", ErrFormat)
	}
	n := int(binary.BigEndian.Uint32(t[12:]))
	if 16+12*n > len(t) {
		return fmt.Errorf("%w: cmap format 12 is shorter than its group count", ErrFormat)
	}
	c.groups = t[16 : 16+12*n]
	return nil
}

// lookup maps a rune to a glyph, or 0 for one the font has no glyph for.
func (c *cmap) lookup(r rune) uint16 {
	if c.groups != nil {
		return c.lookup12(r)
	}
	if c.ends == nil || r < 0 || r > 0xFFFF {
		return 0
	}
	u := uint16(r)
	for i, end := range c.ends {
		if u > end {
			continue
		}
		if u < c.starts[i] {
			return 0
		}
		if c.rangeOffsets[i] == 0 {
			return u + c.deltas[i]
		}
		// The offset is from the range-offset entry's own address, which is
		// the format's one genuinely awkward rule: the entry sits at
		// 2*i bytes into its array and the array's end is where glyphIDs
		// begins, so the byte offset into glyphIDs is the stored offset plus
		// twice the distance from this entry to the end of the array.
		idx := int(c.rangeOffsets[i]) + 2*int(u-c.starts[i]) - 2*(len(c.ends)-i)
		if idx < 0 || idx+2 > len(c.glyphIDs) {
			return 0
		}
		g := binary.BigEndian.Uint16(c.glyphIDs[idx:])
		if g == 0 {
			return 0
		}
		return g + c.deltas[i]
	}
	return 0
}

func (c *cmap) lookup12(r rune) uint16 {
	u := uint32(r)
	// Binary search: a format 12 map can hold thousands of groups, and a scan
	// per rune over a CJK label is the shape that turns a chart into a
	// noticeable pause.
	lo, hi := 0, len(c.groups)/12
	for lo < hi {
		mid := (lo + hi) / 2
		g := c.groups[12*mid:]
		start, end := binary.BigEndian.Uint32(g), binary.BigEndian.Uint32(g[4:])
		switch {
		case u < start:
			hi = mid
		case u > end:
			lo = mid + 1
		default:
			return uint16(binary.BigEndian.Uint32(g[8:]) + (u - start))
		}
	}
	return 0
}
