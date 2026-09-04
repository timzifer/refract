// Package theme holds the visual tokens a chart is drawn with: colours,
// fonts, sizes and spacings.
//
// A theme is plain data. It is resolved once, before lowering, and every
// decision that is "how should this look" reads from it rather than hardcoding
// a value — which is what makes a second theme a data change instead of a code
// change.
//
// # Tokens
//
// The full [Theme] is wide, and most of it follows from a handful of choices.
// [Tokens] holds those choices and [Build] derives the rest, so a new theme is
// a dozen lines rather than fifty. [Theme.With] edits a built theme, and
// [Register] and [ByName] make a theme reachable by name — which is what a
// configuration file or a command-line flag needs.
package theme

import (
	"sort"
	"sync"

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

	// Guides. A chart can carry more than one — a legend for the layers that
	// have a colour each, a colourbar for a layer whose colour comes from a
	// continuous scale — and they stack in one column beside the plot.
	GuideGap float32 // vertical gap between stacked guides

	// Colourbar.
	ColorbarThickness float32 // across the bar
	ColorbarFraction  float64 // of the plot height, before clamping
	ColorbarBorder    ir.Color
	ColorbarTickCount int

	// Annotations: reference lines, bands and labels that are not data.
	AnnotationColor   ir.Color
	AnnotationWidth   float32
	AnnotationDash    []float32
	AnnotationOpacity float64 // fill alpha for a shaded band

	// Panels: subplots and facets.
	PanelGap    float32 // between adjacent panels
	StripBG     ir.Color
	StripColor  ir.Color
	StripSize   float64
	StripPad    float32
	StripBorder ir.Color

	// Spacing.
	Margin float32 // outer margin on all four sides

	// Series colours, and the ramps a colour scale falls back to.
	Palette    palette.Qualitative
	Sequential palette.Ramp
	Diverging  palette.Ramp

	// Default geometry weights.
	LineWidth  float32
	MarkerSize float32
}

// LightTokens are the choices the light theme makes: dark ink on white.
var LightTokens = Tokens{
	Name:       "light",
	Background: ir.RGB(0xFF, 0xFF, 0xFF),
	Panel:      ir.Transparent,
	Strip:      ir.RGB(0xEE, 0xEF, 0xF1),

	Ink:       ir.RGB(0x1A, 0x1A, 0x1A),
	InkMuted:  ir.RGB(0x33, 0x33, 0x33),
	InkSubtle: ir.RGB(0x55, 0x55, 0x55),

	Line:       ir.RGB(0x33, 0x33, 0x33),
	LineSubtle: ir.RGB(0xDD, 0xDD, 0xDD),

	FontSize: 12,
	Space:    4,

	Palette:    palette.OkabeIto,
	Sequential: palette.Viridis,
	Diverging:  palette.BlueOrange,
}

// DarkTokens are the choices the dark theme makes: light ink on near-black.
//
// The axis is deliberately not the same grey as the labels. On a dark surface
// a line at the label's lightness reads heavier than the same line does on
// white, because it is the brighter of the two against its background rather
// than the darker.
var DarkTokens = Tokens{
	Name:       "dark",
	Background: ir.RGB(0x14, 0x16, 0x1A),
	Panel:      ir.Transparent,
	Strip:      ir.RGB(0x22, 0x26, 0x2D),

	Ink:       ir.RGB(0xF2, 0xF3, 0xF5),
	InkMuted:  ir.RGB(0xC8, 0xCB, 0xD0),
	InkSubtle: ir.RGB(0xA0, 0xA4, 0xAB),

	Line:       ir.RGB(0x9A, 0x9E, 0xA6),
	LineSubtle: ir.RGB(0x2A, 0x2E, 0x35),

	FontSize: 12,
	Space:    4,

	Palette:    palette.OkabeIto,
	Sequential: palette.Viridis,
	Diverging:  palette.BlueOrange,
}

// Light is the default theme: dark ink on a white surface.
var Light = Build(LightTokens)

// Dark is the inverted theme: light ink on a near-black surface.
var Dark = Build(DarkTokens)

// Font builds a font reference from the theme's family at the given size.
func (t Theme) Font(size float64) ir.FontRef {
	return ir.FontRef{Family: t.FontFamily, Size: size}
}

// SequentialRamp returns the theme's sequential ramp, falling back to the
// package default so that a zero Theme still produces colours.
func (t Theme) SequentialRamp() palette.Ramp {
	if len(t.Sequential) == 0 {
		return palette.DefaultRamp
	}
	return t.Sequential
}

// DivergingRamp returns the theme's diverging ramp, falling back to the
// package default.
func (t Theme) DivergingRamp() palette.Ramp {
	if len(t.Diverging) == 0 {
		return palette.BlueOrange
	}
	return t.Diverging
}

// Option edits a theme. Options exist so that "the dark theme, in my
// typeface, without a grid" is a call rather than a copy of the struct with
// three fields changed and forty copied.
type Option func(*Theme)

// With returns a copy of t with the options applied. The receiver is not
// modified, so package-level themes stay what they are.
func (t Theme) With(opts ...Option) Theme {
	for _, o := range opts {
		o(&t)
	}
	return t
}

// Named sets the theme's name, which is what [Register] keys it under.
func Named(name string) Option { return func(t *Theme) { t.Name = name } }

// FontFamily sets the logical font family. It is a name, not a file: the
// backend resolves it, and the SVG backend passes it through to the viewer.
func FontFamily(name string) Option { return func(t *Theme) { t.FontFamily = name } }

// FontSize rescales all three text sizes so that the label size becomes size,
// keeping the ratios between title, label and tick.
func FontSize(size float64) Option {
	return func(t *Theme) {
		if size <= 0 || t.LabelSize <= 0 {
			return
		}
		f := size / t.LabelSize
		t.TitleSize *= f
		t.LabelSize = size
		t.TickSize *= f
		t.StripSize *= f
	}
}

// Palette sets the qualitative sequence layers take their colours from.
func Palette(p palette.Qualitative) Option { return func(t *Theme) { t.Palette = p } }

// Ramps sets the sequential and diverging ramps a colour scale falls back to.
// A nil argument leaves that ramp alone.
func Ramps(sequential, diverging palette.Ramp) Option {
	return func(t *Theme) {
		if sequential != nil {
			t.Sequential = sequential
		}
		if diverging != nil {
			t.Diverging = diverging
		}
	}
}

// Grid turns the horizontal and vertical grid lines on or off.
func Grid(x, y bool) Option {
	return func(t *Theme) { t.ShowGridX, t.ShowGridY = x, y }
}

// AxisLines turns the axis rules on or off.
func AxisLines(x, y bool) Option {
	return func(t *Theme) { t.ShowAxisLineX, t.ShowAxisLineY = x, y }
}

// TickCounts sets how many ticks each axis aims for. They are hints: a scale
// rounds to whatever produces readable labels.
func TickCounts(x, y int) Option {
	return func(t *Theme) {
		if x > 0 {
			t.TickCountHintX = x
		}
		if y > 0 {
			t.TickCountHintY = y
		}
	}
}

// Density scales every spacing by f, tightening or loosening the whole chart
// at once. Text sizes are left alone: a dense chart with unreadable labels is
// not denser, it is worse.
func Density(f float64) Option {
	return func(t *Theme) {
		if f <= 0 {
			return
		}
		s := float32(f)
		t.TickLength *= s
		t.TickLabelPad *= s
		t.AxisTitlePad *= s
		t.LegendSwatch *= s
		t.LegendPad *= s
		t.LegendGap *= s
		t.LegendPadding *= s
		t.GuideGap *= s
		t.ColorbarThickness *= s
		t.PanelGap *= s
		t.StripPad *= s
		t.Margin *= s
	}
}

// Background sets the canvas colour behind everything.
func Background(c ir.Color) Option { return func(t *Theme) { t.Background = c } }

// PlotFill sets the fill of the plot area itself, which is transparent by
// default so the background shows through.
func PlotFill(c ir.Color) Option { return func(t *Theme) { t.PlotFill = c } }

// The registry. A theme reachable by name is what a configuration file, a
// command-line flag or an HTTP query parameter needs; resolving it in a map
// here keeps that lookup out of every caller.
var (
	registryMu sync.RWMutex
	registry   = map[string]Theme{}
)

func init() {
	Register(Light)
	Register(Dark)
}

// Register adds t to the registry under its own name, replacing any theme
// already registered under it. A theme with an empty name is ignored.
func Register(t Theme) {
	if t.Name == "" {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[t.Name] = t
}

// ByName returns the registered theme with the given name.
func ByName(name string) (Theme, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	t, ok := registry[name]
	return t, ok
}

// Names lists the registered theme names in order.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
