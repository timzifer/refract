package theme_test

import (
	"testing"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/theme"
)

// The derivation has to be able to express the hand-written v0.1 theme, or it
// is telling you what to want rather than saving you typing. These are the
// numbers that were literals before there were tokens.
func TestBuildReproducesTheHandWrittenSizes(t *testing.T) {
	th := theme.Light
	for _, c := range []struct {
		name string
		got  float64
		want float64
	}{
		{"TitleSize", th.TitleSize, 16},
		{"LabelSize", th.LabelSize, 12},
		{"TickSize", th.TickSize, 11},
		{"TickLength", float64(th.TickLength), 5},
		{"TickLabelPad", float64(th.TickLabelPad), 4},
		{"AxisTitlePad", float64(th.AxisTitlePad), 6},
		{"LegendSwatch", float64(th.LegendSwatch), 12},
		{"LegendPad", float64(th.LegendPad), 8},
		{"LegendGap", float64(th.LegendGap), 6},
		{"LegendPadding", float64(th.LegendPadding), 6},
		{"Margin", float64(th.Margin), 12},
	} {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestBuildFillsInMissingTokens(t *testing.T) {
	th := theme.Build(theme.Tokens{Name: "bare"})
	if th.LabelSize != 12 || th.TickSize != 11 || th.TitleSize != 16 {
		t.Errorf("sizes = %v/%v/%v, want the defaults", th.TitleSize, th.LabelSize, th.TickSize)
	}
	if len(th.Palette) == 0 || len(th.Sequential) == 0 || len(th.Diverging) == 0 {
		t.Error("a theme built from empty tokens has no colours")
	}
}

func TestWithDoesNotMutateTheReceiver(t *testing.T) {
	before := theme.Light.FontFamily
	got := theme.Light.With(theme.FontFamily("Inter"))
	if got.FontFamily != "Inter" {
		t.Errorf("FontFamily = %q, want Inter", got.FontFamily)
	}
	if theme.Light.FontFamily != before {
		t.Error("With edited the package-level theme")
	}
}

// FontSize moves all three sizes together: a chart with a 16pt label and an
// 11pt tick is not a bigger chart, it is a broken hierarchy.
func TestFontSizeKeepsTheTypeScale(t *testing.T) {
	got := theme.Light.With(theme.FontSize(24))
	if got.LabelSize != 24 {
		t.Fatalf("LabelSize = %v, want 24", got.LabelSize)
	}
	if got.TitleSize != 32 {
		t.Errorf("TitleSize = %v, want 32", got.TitleSize)
	}
	if got.TickSize != 22 {
		t.Errorf("TickSize = %v, want 22", got.TickSize)
	}
}

func TestFontSizeIgnoresNonsense(t *testing.T) {
	got := theme.Light.With(theme.FontSize(0), theme.FontSize(-3))
	if got.LabelSize != theme.Light.LabelSize {
		t.Errorf("LabelSize = %v, want it unchanged", got.LabelSize)
	}
}

// Density is about spacing only. Shrinking the type as well would make a dense
// chart an unreadable one.
func TestDensityScalesSpacingAndNotText(t *testing.T) {
	got := theme.Light.With(theme.Density(0.5))
	if got.Margin != theme.Light.Margin/2 {
		t.Errorf("Margin = %v, want %v", got.Margin, theme.Light.Margin/2)
	}
	if got.PanelGap != theme.Light.PanelGap/2 {
		t.Errorf("PanelGap = %v, want %v", got.PanelGap, theme.Light.PanelGap/2)
	}
	if got.LabelSize != theme.Light.LabelSize {
		t.Errorf("LabelSize = %v, want it unchanged", got.LabelSize)
	}
}

func TestOptionsCoverTheCommonEdits(t *testing.T) {
	got := theme.Light.With(
		theme.Named("mine"),
		theme.Palette(palette.Qualitative{palette.Red}),
		theme.Ramps(palette.Magma, nil),
		theme.Grid(false, true),
		theme.AxisLines(false, false),
		theme.TickCounts(3, 0),
		theme.Background(palette.Black),
		theme.PlotFill(palette.White),
	)
	if got.Name != "mine" {
		t.Errorf("Name = %q", got.Name)
	}
	if len(got.Palette) != 1 || got.Palette.At(0) != palette.Red {
		t.Errorf("Palette = %v", got.Palette)
	}
	if got.SequentialRamp()[0] != palette.Magma[0] {
		t.Error("Ramps did not set the sequential ramp")
	}
	if got.DivergingRamp()[0] != theme.Light.Diverging[0] {
		t.Error("a nil ramp overwrote the diverging one")
	}
	if got.ShowGridX || !got.ShowGridY {
		t.Errorf("grid = %v/%v, want false/true", got.ShowGridX, got.ShowGridY)
	}
	if got.ShowAxisLineX || got.ShowAxisLineY {
		t.Error("axis lines are still on")
	}
	if got.TickCountHintX != 3 || got.TickCountHintY != theme.Light.TickCountHintY {
		t.Errorf("tick hints = %d/%d", got.TickCountHintX, got.TickCountHintY)
	}
	if got.Background != palette.Black || got.PlotFill != palette.White {
		t.Errorf("surfaces = %v/%v", got.Background, got.PlotFill)
	}
}

func TestZeroThemeStillHasRamps(t *testing.T) {
	var zero theme.Theme
	if len(zero.SequentialRamp()) == 0 || len(zero.DivergingRamp()) == 0 {
		t.Error("a zero theme has no fallback ramps")
	}
}

func TestRegistryHoldsTheBuiltInThemes(t *testing.T) {
	for _, name := range []string{"light", "dark"} {
		got, ok := theme.ByName(name)
		if !ok {
			t.Fatalf("theme %q is not registered", name)
		}
		if got.Name != name {
			t.Errorf("ByName(%q).Name = %q", name, got.Name)
		}
	}
	if _, ok := theme.ByName("nope"); ok {
		t.Error("an unregistered name resolved")
	}
}

func TestRegisterIgnoresAnUnnamedTheme(t *testing.T) {
	before := len(theme.Names())
	theme.Register(theme.Theme{})
	if got := len(theme.Names()); got != before {
		t.Errorf("Names went from %d to %d", before, got)
	}
}

func TestRegisterAndNames(t *testing.T) {
	theme.Register(theme.Light.With(theme.Named("test-registry")))
	got, ok := theme.ByName("test-registry")
	if !ok || got.Name != "test-registry" {
		t.Fatalf("ByName = %v, %v", got.Name, ok)
	}
	var found bool
	names := theme.Names()
	for i, n := range names {
		if n == "test-registry" {
			found = true
		}
		if i > 0 && names[i-1] > n {
			t.Errorf("Names is not sorted: %v", names)
		}
	}
	if !found {
		t.Errorf("Names = %v, missing the registered theme", names)
	}
}

// Tokens is not an inverse of Build, but round-tripping the built-in themes
// has to land back on the same colours, or "the dark theme in my typeface"
// silently produces a different dark theme.
func TestTokensRoundTripsTheBuiltInThemes(t *testing.T) {
	for _, want := range []theme.Theme{theme.Light, theme.Dark} {
		got := theme.Build(want.Tokens())
		for _, c := range []struct {
			name     string
			got, ref ir.Color
		}{
			{"Background", got.Background, want.Background},
			{"TitleColor", got.TitleColor, want.TitleColor},
			{"LabelColor", got.LabelColor, want.LabelColor},
			{"TickColor", got.TickColor, want.TickColor},
			{"AxisColor", got.AxisColor, want.AxisColor},
			{"GridColor", got.GridColor, want.GridColor},
			{"StripBG", got.StripBG, want.StripBG},
		} {
			if c.got != c.ref {
				t.Errorf("%s: %s = %v, want %v", want.Name, c.name, c.got, c.ref)
			}
		}
		if got.Margin != want.Margin || got.TitleSize != want.TitleSize {
			t.Errorf("%s: Margin/TitleSize = %v/%v, want %v/%v",
				want.Name, got.Margin, got.TitleSize, want.Margin, want.TitleSize)
		}
	}
}
