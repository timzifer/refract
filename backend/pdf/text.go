package pdf

import "bytes"

// winAnsiHigh maps the code points WinAnsiEncoding puts in 0x80–0x9F, where
// Latin-1 has control characters instead. Everything else in the encoding is
// Latin-1, so those runes need no table.
var winAnsiHigh = map[rune]byte{
	'€': 0x80, // €
	'‚': 0x82, // ‚
	'ƒ': 0x83, // ƒ
	'„': 0x84, // „
	'…': 0x85, // …
	'†': 0x86, // †
	'‡': 0x87, // ‡
	'ˆ': 0x88, // ˆ
	'‰': 0x89, // ‰
	'Š': 0x8A, // Š
	'‹': 0x8B, // ‹
	'Œ': 0x8C, // Œ
	'Ž': 0x8E, // Ž
	'‘': 0x91, // ‘
	'’': 0x92, // ’
	'“': 0x93, // “
	'”': 0x94, // ”
	'•': 0x95, // •
	'–': 0x96, // –
	'—': 0x97, // —
	'˜': 0x98, // ˜
	'™': 0x99, // ™
	'š': 0x9A, // š
	'›': 0x9B, // ›
	'œ': 0x9C, // œ
	'ž': 0x9E, // ž
	'Ÿ': 0x9F, // Ÿ
}

// writeString writes s as a PDF literal string in WinAnsiEncoding.
//
// A rune the encoding cannot represent becomes "?". Dropping it instead would
// silently shorten a label that layout has already measured at its full width,
// and a visible substitute is the failure a reader can actually act on.
func writeString(w *bytes.Buffer, s string) {
	w.WriteByte('(')
	for _, r := range s {
		var b byte
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			// A text run is a single line by construction; a stray control
			// character would end up as a literal byte in the stream.
			b = ' '
		case r >= 0x20 && r <= 0x7E:
			b = byte(r)
		case r >= 0xA0 && r <= 0xFF:
			b = byte(r)
		default:
			if v, ok := winAnsiHigh[r]; ok {
				b = v
			} else {
				b = '?'
			}
		}
		switch b {
		case '(', ')', '\\':
			w.WriteByte('\\')
		}
		w.WriteByte(b)
	}
	w.WriteByte(')')
}

// baseFont names the base-14 face for a weight and slant. These four are the
// Helvetica family every conforming reader carries, so nothing is embedded.
func baseFont(weight int, italic bool) string {
	bold := weight >= 600
	switch {
	case bold && italic:
		return "Helvetica-BoldOblique"
	case bold:
		return "Helvetica-Bold"
	case italic:
		return "Helvetica-Oblique"
	}
	return "Helvetica"
}
