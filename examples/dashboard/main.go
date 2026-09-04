// Command dashboard renders the v0.3 additions: small multiples, annotations,
// a colourbar, a grid of subplots, and PDF output.
//
// Like the other documented examples it is executed by a test, so it cannot
// quietly stop compiling or stop producing charts.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/backend/pdf"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func main() {
	facets := flag.String("facets", "regions.svg", "output path for the faceted chart")
	budget := flag.String("budget", "budget.svg", "output path for the annotated chart")
	overview := flag.String("overview", "overview.pdf", "output path for the subplot grid")
	flag.Parse()
	if err := run(*facets, *budget, *overview); err != nil {
		fmt.Fprintln(os.Stderr, "dashboard:", err)
		os.Exit(1)
	}
}

func run(facets, budget, overview string) error {
	if err := regions(facets); err != nil {
		return err
	}
	if err := latencyBudget(budget); err != nil {
		return err
	}
	return fleetOverview(overview)
}

// regions is one panel per region, all on the same axes.
//
// The threshold line has no data behind it, so it cannot be split — which is
// why it appears on every panel rather than on one.
func regions(out string) error {
	names, hours, rps := fleet()
	src := refract.NewTable().
		String("region", names).
		Float64("hour", hours).
		Float64("rps", rps)

	p := refract.New(
		refract.Size(900, 520),
		refract.Title("Throughput by region"),
		refract.XTitle("hour"),
		refract.YTitle("requests/s"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(
		geom.Line(src, geom.X("hour"), geom.Y("rps"),
			geom.Color(palette.Blue), geom.Label("throughput")),
		geom.HLine(60, geom.Label("target")),
	)
	// Shared scales are the default, and the reason small multiples work: the
	// only thing that differs between panels is the data.
	p.Facet(facet.Wrap("region", facet.Columns(3)))
	return p.Render(refract.SVG(out))
}

// latencyBudget reads a series against the thresholds it is judged by.
func latencyBudget(out string) error {
	minutes := ramp(0, 60, 180)
	src := refract.Float64Columns(map[string][]float64{
		"minute": minutes,
		"p99": apply(minutes, func(x float64) float64 {
			return 190 + 45*math.Sin(x/7) + 12*math.Sin(x/2.3)
		}),
	})

	p := refract.New(
		refract.Size(800, 420),
		refract.Title("Latency against its budget"),
		refract.XTitle("minute"),
		refract.YTitle("ms"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(
		// Annotations are ordinary layers, drawn in the order given — the
		// shaded ones first so the data sits on top of them.
		geom.HBand(170, 210, geom.Label("tolerance")),
		geom.VBand(22, 26, geom.Fill(palette.Orange), geom.Opacity(0.18), geom.Label("deploy")),
		geom.Line(src, geom.X("minute"), geom.Y("p99"),
			geom.Color(palette.Blue), geom.Label("p99")),
		geom.HLine(250, geom.Label("SLO")),
		geom.Note(1, 246, "budget", geom.FontSize(11), geom.Align(ir.AlignStart, ir.AlignTop)),
	)
	return p.Render(refract.SVG(out))
}

// fleetOverview puts four unrelated plots on one page, as PDF.
//
// Each keeps its own scales, so each writes its own axes; the grid supplies
// the canvas, the theme and the titles that are shared.
func fleetOverview(out string) error {
	xs := ramp(0, 12, 120)
	g := refract.NewGrid(2,
		refract.GridSize(900, 560),
		refract.GridTheme(theme.Dark),
		refract.GridTitle("Fleet overview"),
		refract.GridAxisTitles("minute", "value"),
	)
	for i, s := range []struct {
		title string
		fn    func(float64) float64
	}{
		{"latency", func(x float64) float64 { return 50 + 20*math.Sin(x) }},
		{"throughput", func(x float64) float64 { return 900 * math.Exp(-x/9) }},
		{"errors", func(x float64) float64 { return 3 * math.Sin(x/2) * math.Sin(x/2) }},
		{"saturation", func(x float64) float64 { return 0.45 + 0.3*math.Sin(x/3) }},
	} {
		p := refract.New(refract.Title(s.title))
		p.X(scale.Linear(scale.Nice()))
		p.Y(scale.Linear(scale.Nice()))
		p.Add(geom.Line(refract.Float64Columns(map[string][]float64{
			"x": xs, "y": apply(xs, s.fn),
		}), geom.X("x"), geom.Y("y"), geom.Color(palette.OkabeIto.At(i))))
		g.Add(p)
	}
	// The PDF emitter is in the core module and links nothing: the same
	// specification, a different target.
	return g.Render(refract.PDF(out, pdf.Title("Fleet overview")))
}

// fleet builds five regions of hourly throughput.
//
// A fixed formula rather than math/rand keeps the example's output stable,
// which is what lets a test assert on it.
func fleet() (regions []string, hours, rps []float64) {
	for ri, name := range []string{"north", "south", "east", "west", "central"} {
		for h := range 24 {
			regions = append(regions, name)
			hours = append(hours, float64(h))
			rps = append(rps, 40+12*float64(ri)+18*math.Sin(float64(h)/3.8+float64(ri)))
		}
	}
	return regions, hours, rps
}

func ramp(lo, hi float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = lo + (hi-lo)*float64(i)/float64(n-1)
	}
	return out
}

func apply(xs []float64, fn func(float64) float64) []float64 {
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = fn(x)
	}
	return out
}
