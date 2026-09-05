package layout_test

import (
	"testing"

	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/layout"
	"github.com/timzifer/refract/theme"
)

func base() layout.Chart {
	return layout.Chart{
		Canvas: ir.R(0, 0, 800, 500),
		Theme:  theme.Light,
	}
}

func TestPlotAreaFitsInsideTheCanvasMargins(t *testing.T) {
	c := base()
	got := layout.Compute(c, irtest.New())

	m := theme.Light.Margin
	if got.Plot.Min.X < m || got.Plot.Min.Y < m {
		t.Errorf("plot area %v starts inside the margin %v", got.Plot, m)
	}
	if got.Plot.Max.X > 800-m || got.Plot.Max.Y > 500-m {
		t.Errorf("plot area %v runs past the margin %v", got.Plot, m)
	}
	if got.Plot.Empty() {
		t.Fatal("plot area is empty")
	}
}

func TestWiderTickLabelsWidenTheLeftMargin(t *testing.T) {
	narrow := base()
	narrow.YLabels = []string{"1", "2", "3"}

	wide := base()
	wide.YLabels = []string{"1000000", "2000000", "3000000"}

	m := irtest.New()
	a := layout.Compute(narrow, m)
	b := layout.Compute(wide, m)

	if b.Plot.Min.X <= a.Plot.Min.X {
		t.Fatalf("wide labels did not widen the margin: %v vs %v", b.Plot.Min.X, a.Plot.Min.X)
	}
}

func TestTitleTakesRoomFromTheTop(t *testing.T) {
	m := irtest.New()
	without := layout.Compute(base(), m)

	c := base()
	c.Title = "A title"
	with := layout.Compute(c, m)

	if with.Plot.Min.Y <= without.Plot.Min.Y {
		t.Fatal("a title must push the plot area down")
	}
	if with.Title.Y <= 0 || with.Title.Y >= with.Plot.Min.Y {
		t.Fatalf("title baseline %v is not between the canvas top and the plot area", with.Title.Y)
	}
	if want := (with.Plot.Min.X + with.Plot.Max.X) / 2; with.Title.X != want {
		t.Errorf("title x = %v, want the plot centre %v", with.Title.X, want)
	}
}

func TestAxisTitlesTakeRoom(t *testing.T) {
	m := irtest.New()
	plain := layout.Compute(base(), m)

	c := base()
	c.XTitle, c.YTitle = "time", "amplitude"
	titled := layout.Compute(c, m)

	if titled.Plot.Max.Y >= plain.Plot.Max.Y {
		t.Error("an X axis title must take room from the bottom")
	}
	if titled.Plot.Min.X <= plain.Plot.Min.X {
		t.Error("a Y axis title must take room from the left")
	}
	if titled.YTitle.X <= 0 || titled.YTitle.X >= titled.Plot.Min.X {
		t.Errorf("Y title anchor %v is not in the left gutter", titled.YTitle.X)
	}
}

func TestLegendReservesSpaceToTheRight(t *testing.T) {
	m := irtest.New()
	c := base()
	c.Guides = []layout.Guide{{Kind: layout.GuideLegend, Labels: []string{"alpha", "beta"}}}
	got := layout.Compute(c, m)

	if len(got.Guides) != 1 || got.Guides[0].Empty() {
		t.Fatal("legend rectangle is empty")
	}
	if got.Guides[0].Min.X < got.Plot.Max.X {
		t.Errorf("legend %v overlaps the plot area %v", got.Guides[0], got.Plot)
	}
	if got.Guides[0].Max.X > 800 {
		t.Errorf("legend %v runs off the canvas", got.Guides[0])
	}
}

func TestLongerLegendLabelsWidenTheReservation(t *testing.T) {
	m := irtest.New()
	short := base()
	short.Guides = []layout.Guide{{Kind: layout.GuideLegend, Labels: []string{"a"}}}
	long := base()
	long.Guides = []layout.Guide{{Kind: layout.GuideLegend, Labels: []string{"a very long series name indeed"}}}

	if layout.Compute(long, m).Plot.Max.X >= layout.Compute(short, m).Plot.Max.X {
		t.Fatal("a longer legend label must shrink the plot area")
	}
}

func TestLastXLabelDoesNotRunOffTheCanvas(t *testing.T) {
	m := irtest.New()
	c := base()
	c.XLabels = []string{"09:00", "2026-03-14 09:00:00"}
	got := layout.Compute(c, m)

	half := m.Measure(ir.TextRun{
		Text: "2026-03-14 09:00:00",
		Font: theme.Light.Font(theme.Light.TickSize),
	}).Advance / 2

	if got.Plot.Max.X+half > 800 {
		t.Fatalf("the centred last label would overflow: plot ends at %v, half-label %v", got.Plot.Max.X, half)
	}
}

func TestTinyCanvasDegradesWithoutInvertingThePlotArea(t *testing.T) {
	// A canvas smaller than its own furniture must not produce a rectangle with
	// Max < Min: everything downstream would map to nonsense coordinates.
	c := layout.Chart{
		Canvas:  ir.R(0, 0, 20, 20),
		Theme:   theme.Light,
		Title:   "A long title that cannot possibly fit",
		XTitle:  "x",
		YTitle:  "y",
		XLabels: []string{"100000"},
		YLabels: []string{"100000"},
	}
	got := layout.Compute(c, irtest.New())
	if got.Plot.Max.X < got.Plot.Min.X || got.Plot.Max.Y < got.Plot.Min.Y {
		t.Fatalf("plot area is inverted: %v", got.Plot)
	}
}
