// Command relational renders the charts a layout in the unit square unlocks.
//
// It is the bucket-E example, and like the others it is executed by a test so
// that it cannot silently stop compiling or stop producing a chart. Its point
// is the same one examples/polar makes, one bucket later: there is no sunburst
// geom and no chord geom here. A sunburst is the icicle two charts above it,
// wrapped round a circle; a chord diagram is the arc diagram beside it with its
// rail moved to the rim. Four marks, six charts, and the coordinate stage is
// the difference — see docs/adr/0039-relational-layouts.md.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func main() {
	treemap := flag.String("treemap", "disk.svg", "output path for the treemap")
	icicle := flag.String("icicle", "icicle.svg", "output path for the icicle")
	sunburst := flag.String("sunburst", "sunburst.svg", "output path for the sunburst")
	sankey := flag.String("sankey", "flow.svg", "output path for the sankey")
	arc := flag.String("arc", "arcs.svg", "output path for the arc diagram")
	chord := flag.String("chord", "chord.svg", "output path for the chord diagram")
	flag.Parse()
	if err := run(*treemap, *icicle, *sunburst, *sankey, *arc, *chord); err != nil {
		fmt.Fprintln(os.Stderr, "relational:", err)
		os.Exit(1)
	}
}

func run(treemap, icicle, sunburst, sankey, arc, chord string) error {
	for _, step := range []func() error{
		func() error { return diskTreemap(treemap) },
		func() error { return diskIcicle(icicle) },
		func() error { return diskSunburst(sunburst) },
		func() error { return trafficSankey(sankey) },
		func() error { return trafficArcs(arc) },
		func() error { return trafficChord(chord) },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// bare is the theme a mark that places its own layout wants, for the reason a
// pie wants one: both its axes describe the unit square, and an axis reading
// 0 … 1 beside a treemap is a ladder of numbers that mean nothing.
func bare() theme.Theme {
	return theme.Light.With(
		theme.Grid(false, false),
		theme.AxisLines(false, false),
		theme.Ticks(false, false),
	)
}

// disk is a directory tree: one row per node, the name of the node above it,
// and the size of the leaves. An internal directory carries no number of its
// own — its size is what is under it, which is what geom.Value means by rolling
// a hierarchy up.
func disk() refract.Source {
	return refract.NewTable().
		String("path", []string{
			"repo", "src", "docs", "test",
			"geom", "scale", "coord", "render",
			"guide", "adr",
			"unit", "golden",
		}).
		String("under", []string{
			"", "repo", "repo", "repo",
			"src", "src", "src", "src",
			"docs", "docs",
			"test", "test",
		}).
		String("area", []string{
			"", "src", "docs", "test",
			"src", "src", "src", "src",
			"docs", "docs",
			"test", "test",
		}).
		Float64("kb", []float64{
			0, 0, 0, 0,
			420, 180, 260, 310,
			150, 90,
			200, 110,
		})
}

// traffic is where a service's requests go: one row per edge, its two ends and
// how many requests a second run along it. No node is declared anywhere — a
// node exists because a row mentioned it.
func traffic() refract.Source {
	return refract.NewTable().
		String("from", []string{"web", "web", "mobile", "mobile", "api", "api", "api"}).
		String("to", []string{"api", "cdn", "api", "cdn", "cache", "db", "search"}).
		Float64("rps", []float64{620, 180, 340, 120, 500, 300, 160})
}

// layout is the plot every chart below is drawn in: a bare theme, two linear
// scales, and whichever coord the chart wants.
func layout(title string, w, h int, c coord.Coord, opts ...refract.Option) *refract.Plot {
	base := []refract.Option{refract.Size(w, h), refract.Title(title), refract.Theme(bare())}
	if c != nil {
		base = append(base, refract.Coord(c))
	}
	p := refract.New(append(base, opts...)...)
	// Neither scale is niced, for the reason a pie's is not: the layout runs
	// from zero to one exactly, and a domain rounded outwards would leave a
	// margin of nothing the mark never asked for.
	p.X(scale.Linear())
	p.Y(scale.Linear())
	return p
}

// A treemap is areas. Each leaf's rectangle is its share of the whole, and the
// nesting shows as the gap Padding leaves round each subtree — a border drawn
// round a cell would be ink the reader has to discount from the area they are
// being asked to compare.
func diskTreemap(path string) error {
	p := layout("Disk by directory", 700, 420, nil, refract.Legend(false))
	p.Add(geom.Treemap(disk(),
		geom.ID("path"), geom.Parent("under"), geom.Value("kb"),
		geom.Padding(0.006),
		geom.ColorBy("area", scale.Qualitative(palette.OkabeIto))))
	return p.Render(refract.SVG(path))
}

// An icicle is the same hierarchy read as a span across and a depth out. Every
// node is drawn, root included, because the levels are the reading.
func diskIcicle(path string) error {
	p := layout("Disk by depth", 640, 380, nil, refract.Legend(false))
	p.Add(geom.Icicle(disk(),
		geom.ID("path"), geom.Parent("under"), geom.Value("kb"),
		geom.Padding(0.004)))
	return p.Render(refract.SVG(path))
}

// And the sunburst is that icicle under a polar coord — the same layer, the
// same options, one line different. It is coord.Polar and not coord.Pie: a pie
// sweeps the Y axis round the circle, and this chart's Y is its depth.
func diskSunburst(path string) error {
	p := layout("Disk by directory", 520, 460, coord.Polar(coord.Hole(0.12)), refract.Legend(false))
	p.Add(geom.Icicle(disk(),
		geom.ID("path"), geom.Parent("under"), geom.Value("kb"),
		geom.Padding(0.004)))
	return p.Render(refract.SVG(path))
}

// A sankey is an edge list as a flow. The bands are the rows; the nodes are
// what several rows have in common, and nothing declares them.
func trafficSankey(path string) error {
	p := layout("Requests per second", 700, 400, nil)
	p.Add(geom.Sankey(traffic(),
		geom.From("from"), geom.To("to"), geom.Value("rps"),
		geom.Padding(0.03)))
	return p.Render(refract.SVG(path))
}

// An arc diagram is the same edge list on a rail, with the ribbons rising off
// it. The longest edge arcs the whole way over and every shorter one is a
// fraction of that, so the height reads as reach.
func trafficArcs(path string) error {
	p := layout("Service traffic", 640, 420, nil)
	p.Add(geom.Arc(traffic(),
		geom.From("from"), geom.To("to"), geom.Value("rps"),
		geom.Padding(0.01)))
	return p.Render(refract.SVG(path))
}

// A chord diagram is that arc diagram with its rail at the rim. geom.Baseline
// is the whole difference; the coord does the rest, and the ribbons that
// reached across the plot now cross the disc.
func trafficChord(path string) error {
	p := layout("Service traffic", 520, 460, coord.Polar())
	p.Add(geom.Arc(traffic(),
		geom.From("from"), geom.To("to"), geom.Value("rps"),
		geom.Baseline(1), geom.Padding(0.01)))
	return p.Render(refract.SVG(path))
}
