// Command distributions renders the v0.9 charts from the README.
//
// Like the other examples it is executed by a test, so it cannot silently stop
// compiling or stop producing a chart. It exercises the milestone's two pieces
// together: the distribution marks, each of which decides one of its own axes,
// and the size channel, whose key is the third kind in the guide column.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

func main() {
	hist := flag.String("histogram", "latency.svg", "output path for the histogram and ECDF")
	violin := flag.String("violin", "services.svg", "output path for the violins")
	ridge := flag.String("ridgeline", "seasons.svg", "output path for the ridgeline")
	swarm := flag.String("beeswarm", "cohorts.svg", "output path for the beeswarm")
	hex := flag.String("hexbin", "cloud.svg", "output path for the hexbin and its trend")
	bubble := flag.String("bubble", "nations.svg", "output path for the bubble chart")
	flag.Parse()
	if err := run(*hist, *violin, *ridge, *swarm, *hex, *bubble); err != nil {
		fmt.Fprintln(os.Stderr, "distributions:", err)
		os.Exit(1)
	}
}

func run(hist, violin, ridge, swarm, hex, bubble string) error {
	for _, step := range []func() error{
		func() error { return latency(hist) },
		func() error { return services(violin) },
		func() error { return seasons(ridge) },
		func() error { return cohorts(swarm) },
		func() error { return cloud(hex) },
		func() error { return nations(bubble) },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// latency draws the same column twice: as a histogram, which picks bin edges,
// and as an ECDF, which picks nothing. Two readings of one distribution, and
// the pair is the argument for having both marks.
func latency(path string) error {
	vs := lognormal(2000, 3.6, 0.55, 11)

	p := refract.New(
		refract.Size(820, 320),
		refract.Title("Request latency"),
		refract.XTitle("milliseconds"),
		refract.YTitle("requests"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(geom.Histogram(refract.Float64Columns(map[string][]float64{"ms": vs}),
		geom.X("ms"), geom.Label("requests")))
	if err := p.Render(refract.SVG(path)); err != nil {
		return err
	}

	// The ECDF goes beside it, on an axis of its own, because a fraction and a
	// count are not the same quantity.
	q := refract.New(
		refract.Size(820, 320),
		refract.Title("Request latency, cumulative"),
		refract.XTitle("milliseconds"),
		refract.YTitle("fraction of requests"),
	)
	q.X(scale.Linear(scale.Nice()))
	q.Y(scale.Linear())
	q.Add(geom.ECDF(refract.Float64Columns(map[string][]float64{"ms": vs}),
		geom.X("ms"), geom.Label("cumulative")))
	return q.Render(refract.SVG(cumulativePath(path)))
}

// cumulativePath is the sibling file the ECDF is written to.
func cumulativePath(path string) string {
	return path[:len(path)-len(".svg")] + "-cumulative.svg"
}

// services is the chart a boxplot cannot draw: three distributions with the
// same quartiles and different shapes, split again by region.
func services(path string) error {
	tbl := refract.NewTable()
	var vals []float64
	var svc, region []string
	names := []string{"auth", "search", "checkout"}
	regions := []string{"eu", "us"}
	for i := range 900 {
		s := i % 3
		r := (i / 3) % 2
		v := lognormalAt(i, 3.2+0.35*float64(s)+0.2*float64(r), 0.4)
		if s == 2 && i%7 == 0 {
			v *= 3 // checkout has a second hump: a slow path some requests take
		}
		vals = append(vals, v)
		svc = append(svc, names[s])
		region = append(region, regions[r])
	}
	tbl.Float64("ms", vals).String("service", svc).String("region", region)

	p := refract.New(
		refract.Size(860, 420),
		refract.Title("Latency by service"),
		refract.YTitle("milliseconds"),
		refract.Legend(true),
	)
	p.X(scale.Ordinal())
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(geom.Violin(tbl,
		geom.X("service"), geom.Y("ms"),
		geom.GroupBy("region"),
		// Pinned, so the two regions are smoothed the same amount and a
		// difference in shape is a difference in distribution.
		geom.Bandwidth(3)))
	return p.Render(refract.SVG(path))
}

// seasons is twelve densities down one axis. Drawn as twelve panels it would be
// twelve comparisons a reader carries between them; drawn as ridges it is one
// picture of a year.
func seasons(path string) error {
	months := []string{
		"jan", "feb", "mar", "apr", "may", "jun",
		"jul", "aug", "sep", "oct", "nov", "dec",
	}
	tbl := refract.NewTable()
	var temps []float64
	var month []string
	for m, name := range months {
		// A sinusoid through the year, with the spread widening in winter.
		mid := 11 + 9*math.Sin(2*math.Pi*(float64(m)-3)/12)
		spread := 3.5 + 1.5*math.Cos(2*math.Pi*float64(m)/12)
		for i := range 220 {
			temps = append(temps, mid+spread*jitter(m*997+i))
			month = append(month, name)
		}
	}
	tbl.Float64("degrees", temps).String("month", month)

	p := refract.New(
		refract.Size(760, 560),
		refract.Title("Daily maximum, by month"),
		refract.XTitle("degrees"),
	)
	p.X(scale.Linear(scale.Nice()))
	// The rows are named on the Y axis, in the order they were given: a
	// ridgeline is read down its axis, so the categories are pinned rather
	// than discovered.
	p.Y(scale.Ordinal(scale.Categories(months...)))
	p.Add(geom.Ridgeline(tbl,
		geom.X("degrees"), geom.Y("month"),
		geom.Overlap(2.2),
		geom.Color(palette.Blue)))
	return p.Render(refract.SVG(path))
}

// cohorts is the chart that shows the rows themselves. A boxplot summarises and
// a violin smooths; a swarm is honest about a group of nine.
func cohorts(path string) error {
	tbl := refract.NewTable()
	var scores []float64
	var cohort []string
	names := []string{"control", "variant A", "variant B"}
	sizes := []int{60, 60, 9}
	for c, name := range names {
		for i := range sizes[c] {
			scores = append(scores, 50+float64(c)*6+9*jitter(c*613+i))
			cohort = append(cohort, name)
		}
	}
	tbl.Float64("score", scores).String("cohort", cohort)

	p := refract.New(
		refract.Size(720, 420),
		refract.Title("Scores by cohort"),
		refract.YTitle("score"),
	)
	p.X(scale.Ordinal())
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Beeswarm(tbl, geom.X("cohort"), geom.Y("score"), geom.Size(7)))
	return p.Render(refract.SVG(path))
}

// cloud is fifty thousand rows and a fit through them: the hexbin says how many
// are where, and the trend says what they are doing.
func cloud(path string) error {
	n := 50000
	xs, ys := make([]float64, n), make([]float64, n)
	for i := range n {
		x := 10 * float64(i) / float64(n-1)
		xs[i] = x
		ys[i] = math.Sin(x)*3 + x/3 + 1.2*jitter(i)
	}
	src := refract.Float64Columns(map[string][]float64{"x": xs, "y": ys})

	p := refract.New(
		refract.Size(820, 500),
		refract.Title("Fifty thousand observations"),
		refract.XTitle("x"),
		refract.YTitle("y"),
		refract.Legend(false),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(
		geom.Hexbin(src, geom.X("x"), geom.Y("y"),
			geom.DensityCells(7), geom.Color(palette.Blue)),
		geom.Trend(src, geom.X("x"), geom.Y("y"),
			geom.Span(0.15), geom.Color(palette.Orange), geom.Width(2.5)),
	)
	return p.Render(refract.SVG(path))
}

// nations is the bubble chart: four columns, three channels, and a size key
// beside the legend.
func nations(path string) error {
	tbl := refract.NewTable().
		Float64("income", []float64{1500, 4200, 12800, 31000, 46000, 58000, 9800, 24000}).
		Float64("years", []float64{58, 66, 72, 78, 81, 82, 70, 76}).
		Float64("people", []float64{212, 1420, 274, 51, 68, 335, 84, 38}).
		String("region", []string{
			"Africa", "Asia", "Asia", "Europe",
			"Europe", "Americas", "Africa", "Americas",
		})

	p := refract.New(
		refract.Size(860, 520),
		refract.Title("Income and life expectancy"),
		refract.XTitle("income per person"),
		refract.YTitle("years"),
		refract.Legend(true),
	)
	p.X(scale.Log(scale.LogNice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Scatter(tbl,
		geom.X("income"), geom.Y("years"),
		// Area, not radius: a country with twice the population is drawn with
		// twice the ink and √2 times the diameter.
		geom.SizeBy("people", scale.Size()),
		geom.ColorBy("region", scale.Qualitative(palette.OkabeIto)),
		geom.Label("population (millions)")))
	return p.Render(refract.SVG(path))
}

// The samples. Everything here is a fixed sequence rather than math/rand: an
// example that drew a different chart on every run would be an example nobody
// could check, and the same argument runs all the way down to stat's
// determinism tests.

func lognormal(n int, mu, sigma float64, seed int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = lognormalAt(i*seed+seed, mu, sigma)
	}
	return out
}

func lognormalAt(i int, mu, sigma float64) float64 {
	return math.Exp(mu + sigma*jitter(i))
}

// jitter is a deterministic pseudo-normal deviate: the sum of twelve values
// from a fixed linear congruential sequence, minus six, which is the classic
// Irwin–Hall approximation and is exactly reproducible.
func jitter(i int) float64 {
	v := uint64(i)*2862933555777941757 + 3037000493
	sum := 0.0
	for range 12 {
		v = v*6364136223846793005 + 1442695040888963407
		sum += float64(v>>11) / float64(uint64(1)<<53)
	}
	return sum - 6
}
