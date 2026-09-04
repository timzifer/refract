// Command categories renders the categorical chart from the README.
//
// It is the second documented example, and like the first it is executed by a
// test so that it cannot silently stop compiling or stop producing a chart. It
// exercises the v0.2 additions together: a categorical column read through an
// ordinal band scale, a continuous colour scale over a second column, and a
// boxplot summarising a distribution per category.
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
	bars := flag.String("bars", "sales.svg", "output path for the bar chart")
	boxes := flag.String("boxes", "latency.svg", "output path for the boxplot")
	flag.Parse()
	if err := run(*bars, *boxes); err != nil {
		fmt.Fprintln(os.Stderr, "categories:", err)
		os.Exit(1)
	}
}

func run(bars, boxes string) error {
	if err := salesByRegion(bars); err != nil {
		return err
	}
	return latencyByCohort(boxes)
}

// salesByRegion is one bar per category, coloured by its own value.
func salesByRegion(out string) error {
	src := refract.NewTable().
		String("region", []string{"north", "south", "east", "west", "central"}).
		Float64("sales", []float64{18, 42, 31, 25, 37})

	p := refract.New(
		refract.Size(700, 400),
		refract.Title("Sales by region"),
		refract.YTitle("k€"),
	)
	// An ordinal scale gives every category an equal slot and tells the bar how
	// wide to be, rather than the bar inferring a width from the data.
	p.X(scale.Ordinal())
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(geom.Bar(src,
		geom.X("region"), geom.Y("sales"),
		geom.ColorBy("sales", scale.Sequential(palette.Viridis)),
	))
	return p.Render(refract.SVG(out))
}

// latencyByCohort summarises a distribution per category.
func latencyByCohort(out string) error {
	cohorts, values := samples()
	src := refract.NewTable().String("cohort", cohorts).Float64("ms", values)

	p := refract.New(
		refract.Size(700, 400),
		refract.Title("Latency by cohort"),
		refract.YTitle("ms"),
	)
	p.X(scale.Ordinal())
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Boxplot(src,
		geom.X("cohort"), geom.Y("ms"),
		geom.Color(palette.Blue),
		geom.Whisker(1.5),
	))
	return p.Render(refract.SVG(out))
}

// samples generates three distributions, each with one tail observation.
//
// A fixed recurrence rather than math/rand keeps the example's output stable,
// which is what lets a test assert on it.
func samples() ([]string, []float64) {
	names := []string{"alpha", "beta", "gamma"}
	var cohorts []string
	var values []float64
	for g, name := range names {
		for i := range 40 {
			t := float64(i) / 40
			v := 24 + float64(g)*7 + 9*math.Sin(9*t+float64(g)) + 3*math.Sin(37*t)
			cohorts, values = append(cohorts, name), append(values, v)
		}
		cohorts, values = append(cohorts, name), append(values, 62+float64(g)*5)
	}
	return cohorts, values
}
