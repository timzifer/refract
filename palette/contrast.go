package palette

import "github.com/timzifer/refract/ir"

// Luminance is the relative luminance of c, from 0 for black to 1 for white.
//
// It is WCAG's definition, which is the one a contrast decision is made
// against: the channels are decoded to linear light and weighted for the eye's
// sensitivity to each, so that a saturated yellow reads as light and a
// saturated blue as dark, which is what a reader sees and what averaging the
// encoded bytes would get wrong. It is the same decode [Lerp] blends through,
// for the same reason.
//
// Alpha is ignored: a translucent fill is composited over something this
// package cannot see, so its luminance is the luminance of the colour it
// states. A caller who knows what is behind it can blend the two with [Lerp]
// first.
func Luminance(c ir.Color) float64 {
	return 0.2126*decodeSRGB(c.R) + 0.7152*decodeSRGB(c.G) + 0.0722*decodeSRGB(c.B)
}
