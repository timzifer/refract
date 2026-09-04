// Command demo opens a chart in a native window.
//
//	go run ./cmd/demo
//
// Hover to see what is under the pointer, drag to pan, turn the wheel to zoom
// about it, and double click to go back to the whole signal. Resize the window
// and the chart is laid out again at the size it now has; because the plot is
// [refract.Responsive], its type and stroke weights follow.
//
// It is here rather than in the repository's examples directory because it
// needs this module: the core's examples are built by a module that depends on
// nothing, and a window is a dependency.
package main

import (
	"fmt"
	"log"
	"math"
	"os"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/backend/window"
	"github.com/timzifer/refract/backend/window/show"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func main() {
	p := plot()

	// A hover reports the values under the pointer, and the row behind the
	// mark once row tracking is on — which is what highlighting a table beside
	// the chart would need.
	p.On(refract.Hover, func(ev refract.Event) {
		if !ev.Found {
			return
		}
		fmt.Printf("\r%-40s", fmt.Sprintf("%s: x=%.3f y=%.3f", ev.Series(), ev.Hit.X, ev.Hit.Y))
	})

	if err := show.Plot(p,
		window.Title("refract — signal"),
		window.Size(900, 560),
	); err != nil {
		log.Println("window:", err)
		os.Exit(1)
	}
}

func plot() *refract.Plot {
	const n = 4000
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		t := float64(i) / 40
		x[i] = t
		y[i] = math.Sin(t) + 0.35*math.Sin(7.3*t) + 0.12*math.Sin(31*t)
	}
	src := refract.Float64Columns(map[string][]float64{"t": x, "signal": y})

	p := refract.New(
		refract.Size(900, 560),
		refract.Responsive(true),
		refract.Theme(theme.Dark),
		refract.Title("Signal"),
		refract.XTitle("seconds"),
		refract.YTitle("amplitude"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(src, geom.X("t"), geom.Y("signal")))
	return p
}
