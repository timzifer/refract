package refract_test

import (
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// The relational layouts, end to end. Six golden files out of four marks: the
// two recipes are pinned beside the marks they are made of, because "one layer,
// two coords" is the claim and a picture of only one of them proves nothing.

// bareRelational is the theme a layout wants, for the reason a pie wants one:
// both axes describe the unit square, and an axis reading 0 … 1 beside a
// treemap is a ladder of numbers that mean nothing.
func bareRelational() theme.Theme {
	return theme.Light.With(
		theme.Grid(false, false),
		theme.AxisLines(false, false),
		theme.Ticks(false, false),
	)
}

// filesystem is the hierarchy the treemap, icicle and sunburst are drawn from:
// a small tree where only the leaves carry a size.
func filesystem() refract.Source {
	return refract.NewTable().
		String("path", []string{
			"/", "src", "docs", "test",
			"geom", "scale", "coord",
			"guide", "adr",
			"unit", "golden",
		}).
		String("under", []string{
			"", "/", "/", "/",
			"src", "src", "src",
			"docs", "docs",
			"test", "test",
		}).
		Float64("kb", []float64{
			0, 0, 0, 0,
			420, 180, 260,
			150, 90,
			200, 110,
		})
}

// requests is the edge list the sankey, arc diagram and chord diagram are drawn
// from.
func requests() refract.Source {
	return refract.NewTable().
		String("from", []string{"web", "web", "mobile", "mobile", "api", "api", "api"}).
		String("to", []string{"api", "cdn", "api", "cdn", "cache", "db", "search"}).
		Float64("rps", []float64{620, 180, 340, 120, 500, 300, 160})
}

func relationalPlot(size float32, c coord.Coord, layer geom.Geom) *refract.Plot {
	p := refract.New(
		refract.Size(int(size), int(size)),
		refract.Theme(bareRelational()),
		refract.Coord(c),
	)
	// Neither scale is niced, for the reason a pie's is not: the layout runs
	// from zero to one exactly, and a domain rounded outwards would leave a
	// margin of nothing the mark never asked for.
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(layer)
	return p
}

func TestGoldenTreemap(t *testing.T) {
	p := refract.New(refract.Size(640, 400), refract.Theme(bareRelational()), refract.Title("Disk by directory"))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Treemap(filesystem(),
		geom.ID("path"), geom.Parent("under"), geom.Value("kb"),
		geom.Padding(0.008)))
	golden(t, "treemap", p)
}

func TestGoldenIcicleAndSunburst(t *testing.T) {
	layer := func() geom.Geom {
		return geom.Icicle(filesystem(),
			geom.ID("path"), geom.Parent("under"), geom.Value("kb"),
			geom.Padding(0.004))
	}
	t.Run("icicle", func(t *testing.T) {
		golden(t, "icicle", relationalPlot(420, coord.Cartesian(), layer()))
	})
	t.Run("sunburst", func(t *testing.T) {
		golden(t, "sunburst", relationalPlot(420, coord.Polar(coord.Hole(0.12)), layer()))
	})
}

func TestGoldenSankey(t *testing.T) {
	p := refract.New(refract.Size(640, 380), refract.Theme(bareRelational()), refract.Title("Requests per second"))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Sankey(requests(),
		geom.From("from"), geom.To("to"), geom.Value("rps"),
		geom.Padding(0.03)))
	golden(t, "sankey", p)
}

func TestGoldenArcAndChord(t *testing.T) {
	t.Run("arc", func(t *testing.T) {
		golden(t, "arc", relationalPlot(420, coord.Cartesian(), geom.Arc(requests(),
			geom.From("from"), geom.To("to"), geom.Value("rps"), geom.Padding(0.01))))
	})
	t.Run("chord", func(t *testing.T) {
		golden(t, "chord", relationalPlot(420, coord.Polar(), geom.Arc(requests(),
			geom.From("from"), geom.To("to"), geom.Value("rps"),
			geom.Baseline(1), geom.Padding(0.01))))
	})
}

// The claim the two recipes rest on: a sunburst and a chord diagram are not
// second implementations of anything. One layer object, two coords, and the
// only difference in what comes out is that the polar one draws curves.
func TestTheRecipesAreTheMarksTheyAreMadeOf(t *testing.T) {
	for _, tc := range []struct {
		name  string
		polar coord.Coord
		layer func() geom.Geom
	}{
		{"sunburst", coord.Polar(), func() geom.Geom {
			return geom.Icicle(filesystem(), geom.ID("path"), geom.Parent("under"), geom.Value("kb"))
		}},
		{"chord", coord.Polar(), func() geom.Geom {
			return geom.Arc(requests(), geom.From("from"), geom.To("to"), geom.Value("rps"), geom.Baseline(1))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flat := drawRelational(t, relationalPlot(400, coord.Cartesian(), tc.layer()))
			round := drawRelational(t, relationalPlot(400, tc.polar, tc.layer()))
			if len(flat) != len(round) {
				t.Fatalf("the two coords drew %d and %d shapes out of one layer", len(flat), len(round))
			}
			// The coord is the whole difference, and it shows as curvature:
			// what is a straight edge in the unit square is an arc once the
			// square is wrapped round a circle. An icicle's bands go from no
			// curves at all to none but curves; an arc diagram's ribbons are
			// cubics either way, and it is its rails that bend.
			if a, b := cubicsIn(flat), cubicsIn(round); b <= a {
				t.Errorf("the Cartesian reading drew %d curve segments and the polar one %d; "+
					"the polar reading of a %s bends what the flat one leaves straight", a, b, tc.name)
			}
		})
	}
}

func drawRelational(t *testing.T, p *refract.Plot) []irtest.Call {
	t.Helper()
	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}
	return rec.Filter("FillPath")
}

func cubicsIn(calls []irtest.Call) int {
	n := 0
	for _, c := range calls {
		if c.Path == nil {
			continue
		}
		c.Path.Walk(func(op ir.PathOp, _ []ir.Point) {
			if op == ir.OpCubicTo {
				n++
			}
		})
	}
	return n
}
