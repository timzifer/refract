package fontmetrics

// The built-in fallback face.
//
// refract's core module is stdlib-only and embeds no font file, but the
// examples in the documentation must still render without a user supplying a
// TTF. So the core carries a table of advance widths for a generic sans —
// roughly 200 bytes, not 250 kB — and uses it whenever no font has been
// configured. SVG output names font-family="sans-serif", so a viewer picks a
// real font of its own anyway; the table only has to be close enough that
// margins and label-collision decisions come out right.
//
// The widths are the standard Helvetica advance widths per 1000 em, which is
// what "sans-serif" resolves to closely enough on every mainstream platform
// (Helvetica, Arial, Liberation Sans and Nimbus Sans all share this metric
// set).

const builtinUnitsPerEm = 1000

const (
	builtinAscent  = 718
	builtinDescent = 207
)

// builtinWidths holds advances for U+0020 through U+007E.
var builtinWidths = [95]uint16{
	278, 278, 355, 556, 556, 889, 667, 191, // ' ' ! " # $ % & '
	333, 333, 389, 584, 278, 333, 278, 278, // ( ) * + , - . /
	556, 556, 556, 556, 556, 556, 556, 556, // 0 1 2 3 4 5 6 7
	556, 556, 278, 278, 584, 584, 584, 556, // 8 9 : ; < = > ?
	1015, 667, 667, 722, 722, 667, 611, 778, // @ A B C D E F G
	722, 278, 500, 667, 556, 833, 722, 778, // H I J K L M N O
	667, 778, 722, 667, 611, 722, 667, 944, // P Q R S T U V W
	667, 667, 611, 278, 278, 278, 469, 556, // X Y Z [ \ ] ^ _
	333, 556, 556, 500, 556, 556, 278, 556, // ` a b c d e f g
	556, 222, 222, 500, 222, 833, 556, 556, // h i j k l m n o
	556, 556, 333, 500, 278, 556, 500, 722, // p q r s t u v w
	500, 500, 500, 334, 260, 334, 584, // x y z { | } ~
}

// builtinFallbackWidth is used for any rune outside the table. It is the
// advance of a lowercase 'o', which is a fair average for Latin text and
// deliberately not the widest glyph: overestimating every CJK label would
// reserve absurd margins.
const builtinFallbackWidth = 556

// Builtin returns a Face over the built-in metrics at the given size.
//
// weight and italic are accepted so callers do not have to special-case the
// fallback, but only weight is honoured, and crudely: bold text is treated as
// 4% wider, which is about right for the sans faces this table approximates.
// Synthetic obliquing does not change advances at all.
func Builtin(size float64, weight int, italic bool) Face {
	_ = italic
	f := 1.0
	if weight >= 600 {
		f = 1.04
	}
	return &builtinFace{size: size, bold: f}
}

type builtinFace struct {
	size float64
	bold float64
}

func (b *builtinFace) scale() float64 { return b.size / builtinUnitsPerEm * b.bold }

func (b *builtinFace) Advance(s string) float64 {
	var total float64
	for _, r := range s {
		if r >= 0x20 && r <= 0x7E {
			total += float64(builtinWidths[r-0x20])
			continue
		}
		total += builtinFallbackWidth
	}
	return total * b.scale()
}

func (b *builtinFace) Ascent() float64  { return builtinAscent * b.scale() }
func (b *builtinFace) Descent() float64 { return builtinDescent * b.scale() }
