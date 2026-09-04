// Package theme holds the visual tokens a chart is drawn with: colours,
// fonts, sizes and spacings.
//
// A theme is plain data. It is resolved once, before lowering, and every
// decision that is "how should this look" reads from it rather than hardcoding
// a value — which is what makes a second theme a data change instead of a code
// change.
package theme

import (
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
)

// Theme is the full set of visual tokens for a chart.
type Theme struct {
	Name string

	// Surfaces.
	Background ir.Color // outside the plot area
	PlotFill   ir.Color // inside the plot area

	// Text.
	FontFamily string
	TitleSize  float64
	LabelSize  float64
	TickSize   float64
	TitleColor ir.Color
	LabelColor ir.Color
	TickColor  ir.Color

	// Axes and grid.
	AxisColor      ir.Color
	AxisWidth      float32
	GridColor      ir.Color
	GridWidth      float32
	GridDash       []float32
	TickLength     float32
	TickLabelPad   float32
	AxisTitlePad   float32
	ShowGridX      bool
	ShowGridY      bool
	ShowAxisLineX  bool
	ShowAxisLineY  bool
	TickCountHintX int
	TickCountHintY int

	// Legend.
	LegendSwatch  float32
	LegendPad     float32
	LegendGap     float32
	LegendColor   ir.Color
	LegendBG      ir.Color
	LegendBorder  ir.Color
	LegendPadding float32

	// Spacing.
	Margin float32 // outer margin on all four sides

	// Series colours.
	Palette palette.Qualitative

	// Default geometry weights.
	LineWidth  float32
	MarkerSize float32
}

// Light is the default theme: dark ink on a white surface.
var Light = Theme{
	Name:       "light",
	Background: ir.RGB(0xFF, 0xFF, 0xFF),
	PlotFill:   ir.Transparent,

	FontFamily: "",
	TitleSize:  16,
	LabelSize:  12,
	TickSize:   11,
	TitleColor: ir.RGB(0x1A, 0x1A, 0x1A),
	LabelColor: ir.RGB(0x33, 0x33, 0x33),
	TickColor:  ir.RGB(0x55, 0x55, 0x55),

	AxisColor:      ir.RGB(0x33, 0x33, 0x33),
	AxisWidth:      1,
	GridColor:      ir.RGB(0xDD, 0xDD, 0xDD),
	GridWidth:      1,
	TickLength:     5,
	TickLabelPad:   4,
	AxisTitlePad:   6,
	ShowGridX:      true,
	ShowGridY:      true,
	ShowAxisLineX:  true,
	ShowAxisLineY:  true,
	TickCountHintX: 6,
	TickCountHintY: 5,

	LegendSwatch:  12,
	LegendPad:     8,
	LegendGap:     6,
	LegendColor:   ir.RGB(0x33, 0x33, 0x33),
	LegendBG:      ir.Transparent,
	LegendBorder:  ir.Transparent,
	LegendPadding: 6,

	Margin: 12,

	Palette:    palette.OkabeIto,
	LineWidth:  1.75,
	MarkerSize: 6,
}

// Dark is the inverted theme: light ink on a near-black surface.
var Dark = Theme{
	Name:       "dark",
	Background: ir.RGB(0x14, 0x16, 0x1A),
	PlotFill:   ir.Transparent,

	FontFamily: "",
	TitleSize:  16,
	LabelSize:  12,
	TickSize:   11,
	TitleColor: ir.RGB(0xF2, 0xF3, 0xF5),
	LabelColor: ir.RGB(0xC8, 0xCB, 0xD0),
	TickColor:  ir.RGB(0xA0, 0xA4, 0xAB),

	AxisColor:      ir.RGB(0x9A, 0x9E, 0xA6),
	AxisWidth:      1,
	GridColor:      ir.RGB(0x2A, 0x2E, 0x35),
	GridWidth:      1,
	TickLength:     5,
	TickLabelPad:   4,
	AxisTitlePad:   6,
	ShowGridX:      true,
	ShowGridY:      true,
	ShowAxisLineX:  true,
	ShowAxisLineY:  true,
	TickCountHintX: 6,
	TickCountHintY: 5,

	LegendSwatch:  12,
	LegendPad:     8,
	LegendGap:     6,
	LegendColor:   ir.RGB(0xC8, 0xCB, 0xD0),
	LegendBG:      ir.Transparent,
	LegendBorder:  ir.Transparent,
	LegendPadding: 6,

	Margin: 12,

	Palette:    palette.OkabeIto,
	LineWidth:  1.75,
	MarkerSize: 6,
}

// Font builds a font reference from the theme's family at the given size.
func (t Theme) Font(size float64) ir.FontRef {
	return ir.FontRef{Family: t.FontFamily, Size: size}
}
