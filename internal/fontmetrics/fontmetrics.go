// Package fontmetrics answers "how wide is this string" using nothing but the
// standard library.
//
// It exists for the built-in SVG backend. That backend emits <text> elements
// and lets the viewer shape them, but refract's layout still has to size
// margins, space ticks and detect label collisions before the SVG is written —
// and that needs metrics. So this package reads the advance widths out of a
// TrueType/OpenType file (head, hhea, hmtx, cmap) and nothing else: no
// shaping, no GSUB/GPOS, no rasterization.
//
// # Accuracy
//
// Advances are per-glyph sums with no kerning and no substitution. For Latin
// chart labels — which is what ticks, axis titles and legends are — that is
// within a fraction of a pixel of a shaped result. For scripts that require
// shaping it is an approximation, and refract accepts that on the SVG path:
// the viewer shapes the text anyway, so exact agreement was never available.
// The gg backend measures with the same shaper it draws with and is exact.
package fontmetrics

// Face measures text at a specific size.
type Face interface {
	// Advance returns the total advance width of s, in device units.
	Advance(s string) float64
	// Ascent returns the distance from the baseline to the top of the font
	// box, positive, in device units.
	Ascent() float64
	// Descent returns the distance from the baseline down to the bottom of the
	// font box, positive, in device units.
	Descent() float64
}

// Font is a parsed font's size-independent metrics.
type Font struct {
	unitsPerEm float64
	ascent     float64 // font units
	descent    float64 // font units, positive
	advances   []uint16
	cmap       map[rune]uint16
	fallback   uint16
}

// Face returns a Face for f at the given size in device units.
func (f *Font) Face(size float64) Face { return &face{font: f, size: size} }

type face struct {
	font *Font
	size float64
}

func (fc *face) scale() float64 { return fc.size / fc.font.unitsPerEm }

func (fc *face) Advance(s string) float64 {
	var total float64
	for _, r := range s {
		total += float64(fc.font.advance(r))
	}
	return total * fc.scale()
}

func (fc *face) Ascent() float64  { return fc.font.ascent * fc.scale() }
func (fc *face) Descent() float64 { return fc.font.descent * fc.scale() }

func (f *Font) advance(r rune) uint16 {
	gid, ok := f.cmap[r]
	if !ok {
		return f.fallback
	}
	if int(gid) < len(f.advances) {
		return f.advances[gid]
	}
	if len(f.advances) > 0 {
		// hmtx stores one advance for every glyph after the last entry in the
		// per-glyph array; monospaced tails are encoded this way.
		return f.advances[len(f.advances)-1]
	}
	return f.fallback
}
