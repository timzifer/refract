// Command signal renders the chart from CONCEPT.md §13.
//
// It is the documented example, and it is executed by a test so that it cannot
// silently stop compiling or stop producing a chart.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func main() {
	out := flag.String("o", "signal.svg", "output SVG path")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "signal:", err)
		os.Exit(1)
	}
}

func run(out string) error {
	times, values := sample()

	src := refract.NewTable().Time("t", times).Float64("y", values)

	p := refract.New(
		refract.Theme(theme.Dark),
		refract.Size(800, 500),
		refract.Title("Signal"),
		refract.YTitle("amplitude"),
	)
	p.X(scale.Time())
	p.Y(scale.Linear(scale.Nice()))
	p.Add(
		geom.Line(src,
			geom.X("t"), geom.Y("y"),
			geom.Color(palette.Blue),
			geom.Tension(0.4),
			geom.OnMissing(geom.Gap),
		),
	)
	return p.Render(refract.SVG(out))
}

// sample generates a damped sine over an hour.
func sample() ([]time.Time, []float64) {
	const n = 240
	start := time.Date(2026, time.March, 14, 9, 0, 0, 0, time.UTC)
	times := make([]time.Time, n)
	values := make([]float64, n)
	for i := range n {
		times[i] = start.Add(time.Duration(i) * 15 * time.Second)
		x := float64(i) / n
		values[i] = math.Exp(-2*x) * math.Sin(12*math.Pi*x)
	}
	return times, values
}
