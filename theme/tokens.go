package theme

import (
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
)

// Tokens is the small set of decisions a theme actually makes.
//
// A [Theme] has around fifty fields, and almost all of them are consequences
// rather than choices: a tick label is the same colour as the other faint ink,
// the gap under an axis title is the same rhythm as the gap beside a legend.
// Writing all fifty out by hand is how two themes drift apart in ways nobody
// intended. Tokens holds the choices — four inks, two line weights, one type
// size, one spacing unit, the colours — and [Build] derives the rest.
//
// Anything derived can still be overridden afterwards: Build returns a plain
// struct, and [Theme.With] edits it. Tokens is the starting point, not a cage.
type Tokens struct {
	// Name identifies the theme, and is the key it registers under.
	Name string

	// Background is the canvas outside the plot area; Panel is the fill
	// inside it, normally transparent so the background shows through; Strip
	// is the band a facet's label sits in.
	Background ir.Color
	Panel      ir.Color
	Strip      ir.Color

	// Ink runs from the strongest text to the faintest: a title, a label, a
	// tick. Three levels is enough to build a hierarchy and few enough that a
	// second theme has to answer only three questions.
	Ink       ir.Color
	InkMuted  ir.Color
	InkSubtle ir.Color

	// Line is the axis; LineSubtle is the grid behind the data.
	Line       ir.Color
	LineSubtle ir.Color

	// FontFamily is the logical family; "" means the backend's default sans.
	FontFamily string

	// FontSize is the label size in device units. The title and tick sizes
	// derive from it — see [Build].
	FontSize float64

	// Space is the spacing unit every padding and gap is a multiple of.
	Space float64

	// Palette colours the layers of a chart; Sequential and Diverging are the
	// ramps a colour scale falls back to when it is given none.
	Palette    palette.Qualitative
	Sequential palette.Ramp
	Diverging  palette.Ramp
}

// Build derives a full theme from its tokens.
//
// The type scale is a perfect fourth up for the title and one step down for
// ticks — 12 gives 16 and 11, which is where the hand-written v0.1 themes
// already were. The ratios are written as exact fractions rather than decimals
// so that a round base size produces round derived sizes rather than 15.999.
//
// Spacings are multiples of Tokens.Space. With the default unit of 4 that
// reproduces the v0.1 numbers exactly, which is the point: a derived theme has
// to be able to express what was there before it, or the derivation is telling
// you what to want.
func Build(t Tokens) Theme {
	if t.FontSize <= 0 {
		t.FontSize = 12
	}
	if t.Space <= 0 {
		t.Space = 4
	}
	if len(t.Palette) == 0 {
		t.Palette = palette.Default
	}
	if t.Sequential == nil {
		t.Sequential = palette.DefaultRamp
	}
	if t.Diverging == nil {
		t.Diverging = palette.BlueOrange
	}

	u := float32(t.Space)
	label := t.FontSize
	return Theme{
		Name:       t.Name,
		Background: t.Background,
		PlotFill:   t.Panel,

		FontFamily: t.FontFamily,
		TitleSize:  label * 4 / 3,
		LabelSize:  label,
		TickSize:   label * 11 / 12,
		TitleColor: t.Ink,
		LabelColor: t.InkMuted,
		TickColor:  t.InkSubtle,

		AxisColor:      t.Line,
		AxisWidth:      1,
		GridColor:      t.LineSubtle,
		GridWidth:      1,
		TickLength:     1.25 * u,
		TickLabelPad:   u,
		AxisTitlePad:   1.5 * u,
		ShowGridX:      true,
		ShowGridY:      true,
		ShowAxisLineX:  true,
		ShowAxisLineY:  true,
		ShowTicksX:     true,
		ShowTicksY:     true,
		TickCountHintX: 6,
		TickCountHintY: 5,

		LegendSwatch:  3 * u,
		LegendPad:     2 * u,
		LegendGap:     1.5 * u,
		LegendColor:   t.InkMuted,
		LegendBG:      ir.Transparent,
		LegendBorder:  ir.Transparent,
		LegendPadding: 1.5 * u,

		GuideGap:          3 * u,
		ColorbarThickness: 3.5 * u,
		ColorbarFraction:  0.6,
		ColorbarBorder:    t.Line,
		ColorbarTickCount: 5,

		// A bubble the size of eight spacing units is large enough that the
		// biggest value in a cloud is unmistakable and small enough that it
		// does not swallow its neighbours; three samples are what a reader can
		// interpolate between without the key becoming a chart of its own.
		BubbleSize:   8 * u,
		SizeKeyCount: 3,

		AnnotationColor:   t.InkSubtle,
		AnnotationWidth:   1,
		AnnotationDash:    []float32{4, 3},
		AnnotationOpacity: 0.15,

		PanelGap:    3 * u,
		StripBG:     t.Strip,
		StripColor:  t.InkMuted,
		StripSize:   label * 11 / 12,
		StripPad:    u,
		StripBorder: t.LineSubtle,

		Margin: 3 * u,

		Palette:    t.Palette,
		Sequential: t.Sequential,
		Diverging:  t.Diverging,

		LineWidth:  1.75,
		MarkerSize: 6,
	}
}

// Tokens returns the tokens a theme was built from, as far as they can be read
// back off it.
//
// It is not an inverse of [Build] — a theme that has been edited afterwards
// may no longer be derivable from any tokens at all. It is what makes
// "the dark theme, but in my brand's typeface" one call rather than a rebuild
// from scratch.
func (t Theme) Tokens() Tokens {
	return Tokens{
		Name:       t.Name,
		Background: t.Background,
		Panel:      t.PlotFill,
		Strip:      t.StripBG,
		Ink:        t.TitleColor,
		InkMuted:   t.LabelColor,
		InkSubtle:  t.TickColor,
		Line:       t.AxisColor,
		LineSubtle: t.GridColor,
		FontFamily: t.FontFamily,
		FontSize:   t.LabelSize,
		Space:      float64(t.TickLabelPad),
		Palette:    t.Palette,
		Sequential: t.Sequential,
		Diverging:  t.Diverging,
	}
}
