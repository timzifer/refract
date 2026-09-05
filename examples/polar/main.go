// Command polar renders the charts a coordinate system unlocks.
//
// It is the v0.8 example, and like the others it is executed by a test so that
// it cannot silently stop compiling or stop producing a chart. Its whole point
// is what is *not* here: there is no pie geom, no donut geom, no radar geom and
// no gauge geom. Every chart below is a mark that shipped in an earlier
// milestone, drawn in coord.Polar — which is what a pluggable coordinate stage
// buys, and why the groups and the position adjustments had to land first.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func main() {
	donut := flag.String("donut", "browsers.svg", "output path for the donut chart")
	radar := flag.String("radar", "designs.svg", "output path for the radar chart")
	rose := flag.String("rose", "wind.svg", "output path for the wind rose")
	gauge := flag.String("gauge", "capacity.svg", "output path for the gauge")
	spend := flag.String("spend", "budgets.svg", "output path for the broken-out donut")
	flag.Parse()
	if err := run(*donut, *radar, *rose, *gauge, *spend); err != nil {
		fmt.Fprintln(os.Stderr, "polar:", err)
		os.Exit(1)
	}
}

func run(donut, radar, rose, gauge, spend string) error {
	for _, step := range []func() error{
		func() error { return browserShare(donut) },
		func() error { return designScores(radar) },
		func() error { return windRose(rose) },
		func() error { return capacity(gauge) },
		func() error { return spending(spend) },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// bare is the theme a chart with no axis worth labelling wants. A pie has no
// radial quantity and no angular one either: the slices are the reading, and
// the legend names them.
func bare(base theme.Theme) theme.Theme {
	return base.With(
		theme.Grid(false, false),
		theme.AxisLines(false, false),
		theme.Ticks(false, false),
	)
}

// browserShare is a donut, which is a stacked bar with theta taken from the Y
// axis.
//
// Three things are worth noticing. The X column is a constant, so every slice
// fills the same radial slot and the chart is a ring rather than a spiral.
// Neither scale is niced: the ring closes into a full circle because a stacked
// domain ends at the total, and a domain rounded up to the next round number
// would leave a wedge of nothing at twelve o'clock. And the hole is where the
// radial scale *starts*, so it is an annulus — nothing is drawn inside it, and
// a pointer in it hits nothing.
func browserShare(out string) error {
	names, share := browsers()
	src := refract.NewTable().
		Float64("all", make([]float64, len(names))).
		Float64("share", share).
		String("browser", names)

	p := refract.New(
		refract.Size(620, 400),
		refract.Title("Browser share"),
		refract.Theme(bare(theme.Light)),
		refract.Coord(coord.Polar(coord.Theta(coord.FromY), coord.Hole(0.45))),
	)
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Bar(src,
		geom.X("all"), geom.Y("share"),
		geom.GroupBy("browser"),
		geom.ColorBy("browser", scale.Qualitative(palette.OkabeIto)),
	))
	return p.Render(refract.SVG(out))
}

// designScores is a radar, which is an area over an ordinal angular axis.
//
// Two options make it read as one. coord.Chord draws an edge between two marks
// as the straight line between them rather than as the arc through data space —
// without it the sides bow outwards into something the data does not say — and
// geom.Closed joins the last axis back to the first, because five axes drawn as
// an open line leave a gap in a shape that has none.
//
// geom.Stack(geom.NoStack) is not decoration either: a grouped area stacks by
// default, and two designs compared on the same axes are two readings rather
// than a total.
func designScores(out string) error {
	axes, designs, scores := designs()
	src := refract.NewTable().
		String("axis", axes).
		String("design", designs).
		Float64("score", scores)

	p := refract.New(
		refract.Size(620, 440),
		refract.Title("Two designs"),
		refract.Theme(theme.Dark),
		refract.Coord(coord.Polar(coord.Chord())),
	)
	// No padding: a radar's axes are directions rather than slots, so the
	// spokes sit on the categories rather than in the middle of bands.
	p.X(scale.Ordinal(scale.OrdinalPadding(0)))
	p.Y(scale.Linear(scale.Domain(0, 10)))
	p.Add(geom.Area(src,
		geom.X("axis"), geom.Y("score"),
		geom.GroupBy("design"),
		geom.Stack(geom.NoStack),
		geom.Closed(true),
		geom.ColorBy("design", scale.Qualitative(palette.OkabeIto)),
	))
	return p.Render(refract.SVG(out))
}

// windRose is a bar chart with the direction on the angular axis and the count
// on the radial one — Nightingale's coxcomb, and the chart a meteorologist
// calls a wind rose.
//
// This is the default orientation: theta from X, radius from Y. A petal is an
// annular sector exactly as a slice is, and the only difference from a bar
// chart is what the pair of mapped positions means.
func windRose(out string) error {
	points, counts := wind()
	src := refract.NewTable().
		String("direction", points).
		Float64("hours", counts)

	p := refract.New(
		refract.Size(560, 520),
		refract.Title("Hours by wind direction"),
		refract.Coord(coord.Polar()),
	)
	p.X(scale.Ordinal(scale.OrdinalPadding(0)))
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(geom.Bar(src,
		geom.X("direction"), geom.Y("hours"),
		geom.Color(palette.Blue),
		geom.BarWidth(0.9),
	))
	return p.Render(refract.SVG(out))
}

// capacity is a gauge: one bar over a partial sweep, with a hole.
//
// A gauge is where coord.Sweep earns its place — half a turn rather than a
// whole one — and it is still geom.Bar. The annotation is an ordinary VLine,
// which under a polar coord spans the radius at one angle: a needle.
func capacity(out string) error {
	src := refract.Float64Columns(map[string][]float64{
		"one":  {0},
		"used": {68},
	})

	p := refract.New(
		refract.Size(520, 320),
		refract.Title("Capacity used"),
		refract.Theme(bare(theme.Light)),
		refract.Legend(false),
		refract.Coord(coord.Polar(
			coord.Theta(coord.FromY),
			coord.Hole(0.6),
			coord.Sweep(math.Pi),
			coord.Start(-math.Pi/2),
		)),
	)
	p.X(scale.Linear())
	p.Y(scale.Linear(scale.Domain(0, 100)))
	p.Add(
		geom.Bar(src, geom.X("one"), geom.Y("used"), geom.Fill(palette.Vermilion)),
		geom.HLine(90, geom.Label("limit")),
	)
	return p.Render(refract.SVG(out))
}

// spending is a donut carrying three numbers per slice instead of one, with
// the slice that matters broken out of the ring.
//
// It is the v0.8 sugar in one chart, and nothing in it is a new mark. How far
// round a slice goes is its share of the spend, exactly as in the donut above.
// How far *out* it goes is its team's budget used, because geom.X and geom.X2
// name the radial edges per row — so the ring is no longer a ring, and the
// second measure is read against the rim rather than against a legend. And the
// team that went over its budget is pulled out of the ring by geom.ExplodeBy,
// which moves the slice without changing anything it says: the gap is where it
// came from.
//
// coord.Pie rather than coord.Donut, because here the hole is a column: every
// slice starts at the same floor, and it would be a different column if they
// did not.
func spending(out string) error {
	teams, share, floor, used, pull := budgets()
	src := refract.NewTable().
		String("team", teams).
		Float64("share", share).
		Float64("floor", floor).
		Float64("used", used).
		Float64("pull", pull)

	p := refract.New(
		refract.Size(620, 440),
		refract.Title("Spend by team, against each team's own budget"),
		refract.Theme(bare(theme.Light)),
		refract.Coord(coord.Pie(coord.Radius(0.95))),
	)
	// The radial domain is fixed at [0, 1] so that the rim means "the whole
	// budget" rather than "the biggest of these five", and the angular one is
	// not niced so that the ring closes.
	p.X(scale.Linear(scale.Domain(0, 1)))
	p.Y(scale.Linear())
	p.Add(geom.Bar(src,
		geom.X("floor"), geom.X2("used"), geom.Y("share"),
		geom.GroupBy("team"),
		geom.ExplodeBy("pull"),
		geom.ColorBy("team", scale.Qualitative(palette.OkabeIto)),
	))
	return p.Render(refract.SVG(out))
}

// budgets is five teams: what each spent as a share of the total, where its
// slice starts, how much of its own budget it used, and how far it is pulled
// out of the ring. Growth is the one over its budget, and the one broken out.
func budgets() (teams []string, share, floor, used, pull []float64) {
	return []string{"platform", "data", "growth", "support", "design"},
		[]float64{32, 24, 18, 14, 12},
		[]float64{0.35, 0.35, 0.35, 0.35, 0.35},
		[]float64{0.92, 0.78, 1, 0.55, 0.66},
		[]float64{0, 0, 0.12, 0, 0}
}

// browsers is a market share that adds to a hundred, so the ring closes on
// itself rather than on a rounding error.
func browsers() (names []string, share []float64) {
	return []string{"chrome", "safari", "firefox", "edge", "other"},
		[]float64{46, 24, 14, 11, 5}
}

// designs is two designs rated on five axes: the long-table shape a grouped
// layer draws from, which is what makes a radar one layer rather than two.
func designs() (axes, names []string, scores []float64) {
	byName := map[string][]float64{
		"prism": {8, 6, 9, 4, 7},
		"lens":  {5, 9, 6, 8, 5},
	}
	for _, name := range []string{"prism", "lens"} {
		for i, axis := range []string{"speed", "clarity", "range", "cost", "weight"} {
			axes = append(axes, axis)
			names = append(names, name)
			scores = append(scores, byName[name][i])
		}
	}
	return axes, names, scores
}

// wind is a year of hours per compass point, deterministically: no math/rand,
// because an example whose picture changes between runs is an example nobody
// can check.
func wind() (points []string, hours []float64) {
	points = []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	for i := range points {
		t := float64(i) / float64(len(points)) * 2 * math.Pi
		hours = append(hours, math.Round(600+420*math.Cos(t-0.9)))
	}
	return points, hours
}
