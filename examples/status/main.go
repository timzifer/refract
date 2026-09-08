// Command status renders the two charts a path coloured from a column is for:
// a measurement read against a limit, and a series read against the state of
// the thing that produced it.
//
// Both bind a column with geom.ColorBy, the way a scatter always could. What
// differs is where the colour changes, and neither chart says — the scale
// does. A classed scale puts the corner on the threshold, interpolated between
// two readings, because when the limit was passed is the fact being reported.
// A named scale puts it on the row where the new state was first seen, because
// nothing was measured in between. See docs/adr/0049-paths-colour-in-classes.md.
//
// It is executed by a test so that it cannot silently stop compiling or stop
// producing charts.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

func main() {
	budget := flag.String("budget", "budget.svg", "output path for the threshold chart")
	state := flag.String("state", "state.svg", "output path for the state chart")
	flag.Parse()
	if err := run(*budget, *state); err != nil {
		fmt.Fprintln(os.Stderr, "status:", err)
		os.Exit(1)
	}
}

func run(budget, state string) error {
	if err := latency(budget); err != nil {
		return err
	}
	return machineState(state)
}

// latency is a response time against the limit it is judged by.
//
// The band says where the limit is and the line says when it was over it. The
// two are different statements and both are wanted: a band alone leaves the
// reader tracing the line by eye, and a coloured line alone leaves them
// guessing what the boundary was.
func latency(out string) error {
	src := trace()

	p := refract.New(
		refract.Size(760, 420),
		refract.Title("Checkout latency against its budget"),
		refract.XTitle("minute"),
		refract.YTitle("ms"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))

	// Three classes from two boundaries: within budget, over it, and past the
	// point where the page is abandoned. They come from the service level
	// rather than from the data, which is what makes this Threshold and not
	// Quantize.
	limits := scale.Threshold(
		palette.Ramp{palette.Blue, palette.Orange, palette.Red},
		[]float64{200, 300},
	)
	p.Add(
		geom.HBand(200, 300, geom.Extend(false)),
		geom.Line(src, geom.X("minute"), geom.Y("ms"), geom.ColorBy("ms", limits)),
	)
	return p.Render(refract.SVG(out))
}

// machineState is a production rate coloured by what the machine was doing.
//
// A step rather than a line because a state is held until it changes, and the
// rate is a reading held with it. The colours are named rather than handed out
// in order of first sight: a fault is red on a window that begins with one and
// on a window that never has one, which is the whole reason scale.Named
// exists.
func machineState(out string) error {
	p := refract.New(
		refract.Size(760, 420),
		refract.Title("Line 3, by machine state"),
		refract.XTitle("minute"),
		refract.YTitle("parts/min"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(geom.Step(states(), geom.X("minute"), geom.Y("rate"),
		geom.ColorBy("state", scale.Named(map[string]ir.Color{
			"running":     palette.Green,
			"fault":       palette.Red,
			"maintenance": palette.Orange,
		}))))
	return p.Render(refract.SVG(out))
}

// trace is an hour of latency with two excursions in it, one of them well past
// the second boundary.
func trace() *data.Table {
	const n = 120
	minutes := make([]float64, n)
	ms := make([]float64, n)
	for i := range n {
		t := float64(i)
		minutes[i] = t / 2
		ms[i] = 170 + 25*math.Sin(t/7)
		switch {
		case i >= 30 && i < 44:
			ms[i] += 70
		case i >= 78 && i < 88:
			ms[i] += 160
		}
	}
	return refract.NewTable().Float64("minute", minutes).Float64("ms", ms)
}

// states is what the machine was doing, one row per change. The rate is the
// reading that held with it — a step draws both from the same rows.
func states() *data.Table {
	return refract.NewTable().
		Float64("minute", []float64{0, 12, 18, 27, 34, 48, 60}).
		Float64("rate", []float64{82, 82, 0, 0, 46, 79, 79}).
		String("state", []string{
			"running", "running", "fault", "fault", "maintenance", "running", "running",
		})
}
