// Package palette provides colours and colour sequences for charts.
//
// The default qualitative sequence is Okabe-Ito, which is designed to remain
// distinguishable under the common forms of colour vision deficiency. Charts
// should also encode series redundantly (dash patterns, marker shapes) —
// colour alone is never sufficient — but starting from a safe palette costs
// nothing.
package palette

import "github.com/timzifer/refract/ir"

// Named colours used by the examples and the default theme.
var (
	Blue      = ir.RGB(0x00, 0x72, 0xB2)
	Orange    = ir.RGB(0xE6, 0x9F, 0x00)
	SkyBlue   = ir.RGB(0x56, 0xB4, 0xE9)
	Green     = ir.RGB(0x00, 0x9E, 0x73)
	Yellow    = ir.RGB(0xF0, 0xE4, 0x42)
	Vermilion = ir.RGB(0xD5, 0x5E, 0x00)
	Purple    = ir.RGB(0xCC, 0x79, 0xA7)
	Black     = ir.RGB(0x00, 0x00, 0x00)
	White     = ir.RGB(0xFF, 0xFF, 0xFF)
	Red       = ir.RGB(0xD5, 0x5E, 0x00)
	Gray      = ir.RGB(0x88, 0x88, 0x88)
)

// Qualitative is a sequence of categorical colours.
type Qualitative []ir.Color

// At returns the i'th colour, wrapping around. A chart with more series than
// the palette has colours is a chart that needs a different encoding, but
// wrapping is better than panicking.
func (q Qualitative) At(i int) ir.Color {
	if len(q) == 0 {
		return Blue
	}
	return q[((i%len(q))+len(q))%len(q)]
}

// OkabeIto is the default colourblind-safe qualitative palette.
//
// Okabe & Ito, "Color Universal Design" (2008).
var OkabeIto = Qualitative{
	Blue,
	Vermilion,
	Green,
	Yellow,
	SkyBlue,
	Orange,
	Purple,
	ir.RGB(0x00, 0x00, 0x00),
}

// Default is the palette refract uses when a theme does not override it.
var Default = OkabeIto
