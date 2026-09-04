package fontmetrics

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Parse reads the metric tables out of a TrueType or OpenType font.
//
// It reads exactly four tables — head for unitsPerEm, hhea for the vertical
// metrics and the hmtx entry count, hmtx for advances, cmap for the
// rune-to-glyph mapping — and ignores everything else in the file, including
// the outlines. Parsing is bounds-checked throughout: font files arrive from
// users and must not be able to panic a render.
func Parse(b []byte) (*Font, error) {
	tables, err := tableDirectory(b)
	if err != nil {
		return nil, err
	}

	head, ok := tables["head"]
	if !ok {
		return nil, errors.New("fontmetrics: missing head table")
	}
	if len(head) < 54 {
		return nil, errors.New("fontmetrics: short head table")
	}
	unitsPerEm := float64(binary.BigEndian.Uint16(head[18:20]))
	if unitsPerEm == 0 {
		return nil, errors.New("fontmetrics: head.unitsPerEm is zero")
	}

	hhea, ok := tables["hhea"]
	if !ok {
		return nil, errors.New("fontmetrics: missing hhea table")
	}
	if len(hhea) < 36 {
		return nil, errors.New("fontmetrics: short hhea table")
	}
	ascent := float64(int16(binary.BigEndian.Uint16(hhea[4:6])))
	descent := float64(int16(binary.BigEndian.Uint16(hhea[6:8])))
	numHMetrics := int(binary.BigEndian.Uint16(hhea[34:36]))

	hmtx, ok := tables["hmtx"]
	if !ok {
		return nil, errors.New("fontmetrics: missing hmtx table")
	}
	if numHMetrics*4 > len(hmtx) {
		numHMetrics = len(hmtx) / 4
	}
	advances := make([]uint16, numHMetrics)
	for i := range advances {
		advances[i] = binary.BigEndian.Uint16(hmtx[i*4 : i*4+2])
	}
	if len(advances) == 0 {
		return nil, errors.New("fontmetrics: hmtx has no entries")
	}

	cm, err := parseCmap(tables["cmap"])
	if err != nil {
		return nil, err
	}

	f := &Font{
		unitsPerEm: unitsPerEm,
		ascent:     ascent,
		descent:    -descent, // hhea stores descent as a negative number
		advances:   advances,
		cmap:       cm,
		fallback:   advances[0],
	}
	if f.descent < 0 {
		f.descent = -f.descent
	}
	// Some fonts leave hhea zeroed and put the real numbers in OS/2. Rather
	// than parse a fifth table, fall back to a conventional 80/20 split of the
	// em, which is close enough for laying out an axis.
	if f.ascent <= 0 {
		f.ascent = unitsPerEm * 0.8
	}
	if f.descent <= 0 {
		f.descent = unitsPerEm * 0.2
	}
	return f, nil
}

// tableDirectory reads the sfnt header and returns each table's bytes.
func tableDirectory(b []byte) (map[string][]byte, error) {
	if len(b) < 12 {
		return nil, errors.New("fontmetrics: not a font file")
	}
	switch tag := binary.BigEndian.Uint32(b[0:4]); tag {
	case 0x00010000, 0x74727565 /* 'true' */, 0x4F54544F /* 'OTTO' */ :
	case 0x74746366: // 'ttcf' — a collection; use the first font in it
		if len(b) < 16 {
			return nil, errors.New("fontmetrics: short font collection header")
		}
		off := binary.BigEndian.Uint32(b[12:16])
		if int(off) >= len(b) {
			return nil, errors.New("fontmetrics: bad font collection offset")
		}
		return tableDirectoryAt(b, int(off))
	default:
		return nil, fmt.Errorf("fontmetrics: unsupported font tag %#08x", tag)
	}
	return tableDirectoryAt(b, 0)
}

func tableDirectoryAt(b []byte, base int) (map[string][]byte, error) {
	if base+12 > len(b) {
		return nil, errors.New("fontmetrics: truncated table directory")
	}
	n := int(binary.BigEndian.Uint16(b[base+4 : base+6]))
	out := make(map[string][]byte, n)
	for i := range n {
		rec := base + 12 + i*16
		if rec+16 > len(b) {
			return nil, errors.New("fontmetrics: truncated table record")
		}
		tag := string(b[rec : rec+4])
		off := int(binary.BigEndian.Uint32(b[rec+8 : rec+12]))
		length := int(binary.BigEndian.Uint32(b[rec+12 : rec+16]))
		if off < 0 || length < 0 || off > len(b) {
			continue
		}
		end := off + length
		if end > len(b) {
			end = len(b)
		}
		out[tag] = b[off:end]
	}
	return out, nil
}

// parseCmap picks the best available Unicode subtable and decodes it.
//
// Formats 4 and 12 cover every Unicode-capable font in practice: 4 for the
// BMP, 12 for fonts with characters beyond it.
func parseCmap(b []byte) (map[rune]uint16, error) {
	if len(b) < 4 {
		return nil, errors.New("fontmetrics: missing cmap table")
	}
	n := int(binary.BigEndian.Uint16(b[2:4]))
	bestOff, bestScore := -1, -1
	for i := range n {
		rec := 4 + i*8
		if rec+8 > len(b) {
			break
		}
		platform := binary.BigEndian.Uint16(b[rec : rec+2])
		encoding := binary.BigEndian.Uint16(b[rec+2 : rec+4])
		off := int(binary.BigEndian.Uint32(b[rec+4 : rec+8]))
		if off+4 > len(b) {
			continue
		}
		score := -1
		switch {
		case platform == 3 && encoding == 10: // Windows, UCS-4
			score = 4
		case platform == 0 && encoding >= 4: // Unicode, full repertoire
			score = 3
		case platform == 3 && encoding == 1: // Windows, BMP
			score = 2
		case platform == 0: // Unicode, BMP
			score = 1
		}
		if score > bestScore {
			bestScore, bestOff = score, off
		}
	}
	if bestOff < 0 {
		return nil, errors.New("fontmetrics: no usable cmap subtable")
	}

	sub := b[bestOff:]
	switch format := binary.BigEndian.Uint16(sub[0:2]); format {
	case 4:
		return parseCmap4(sub)
	case 12:
		return parseCmap12(sub)
	default:
		return nil, fmt.Errorf("fontmetrics: unsupported cmap format %d", format)
	}
}

func parseCmap4(b []byte) (map[rune]uint16, error) {
	if len(b) < 14 {
		return nil, errors.New("fontmetrics: short cmap format 4")
	}
	segX2 := int(binary.BigEndian.Uint16(b[6:8]))
	if segX2%2 != 0 {
		return nil, errors.New("fontmetrics: odd segCountX2 in cmap format 4")
	}
	seg := segX2 / 2
	endOff := 14
	startOff := endOff + segX2 + 2 // +2 skips reservedPad
	deltaOff := startOff + segX2
	rangeOff := deltaOff + segX2
	if rangeOff+segX2 > len(b) {
		return nil, errors.New("fontmetrics: truncated cmap format 4")
	}

	out := make(map[rune]uint16, 256)
	u16 := func(off int) uint16 { return binary.BigEndian.Uint16(b[off : off+2]) }
	for i := range seg {
		end := u16(endOff + i*2)
		start := u16(startOff + i*2)
		delta := u16(deltaOff + i*2)
		ro := u16(rangeOff + i*2)
		if start > end {
			continue
		}
		for c := uint32(start); c <= uint32(end); c++ {
			if c == 0xFFFF {
				break
			}
			var gid uint16
			if ro == 0 {
				gid = uint16(c) + delta
			} else {
				idx := rangeOff + i*2 + int(ro) + 2*int(uint16(c)-start)
				if idx+2 > len(b) {
					continue
				}
				gid = binary.BigEndian.Uint16(b[idx : idx+2])
				if gid != 0 {
					gid += delta
				}
			}
			if gid != 0 {
				out[rune(c)] = gid
			}
		}
	}
	return out, nil
}

func parseCmap12(b []byte) (map[rune]uint16, error) {
	if len(b) < 16 {
		return nil, errors.New("fontmetrics: short cmap format 12")
	}
	n := int(binary.BigEndian.Uint32(b[12:16]))
	out := make(map[rune]uint16, 256)
	for i := range n {
		rec := 16 + i*12
		if rec+12 > len(b) {
			break
		}
		start := binary.BigEndian.Uint32(b[rec : rec+4])
		end := binary.BigEndian.Uint32(b[rec+4 : rec+8])
		gid := binary.BigEndian.Uint32(b[rec+8 : rec+12])
		if start > end || end-start > 0x10FFFF {
			continue
		}
		for c := start; c <= end; c++ {
			g := gid + (c - start)
			if g > 0xFFFF {
				break
			}
			out[rune(c)] = uint16(g)
		}
	}
	return out, nil
}
