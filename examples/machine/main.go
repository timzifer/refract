// Command machine renders the chart tracks exist for: a shopfloor terminal's
// view of one machine's last hour.
//
// The speed and its target are lines on a linear axis. The machine's state and
// the work order running at the time are gantt strips on ordinal axes of their
// own. All four are on one time axis, because the question the chart answers is
// what the machine was doing *while* the speed dropped.
//
// It is executed by a test so that it cannot silently stop compiling or stop
// producing a chart.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

func main() {
	out := flag.String("o", "machine.svg", "output SVG path")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "machine:", err)
		os.Exit(1)
	}
}

func run(out string) error {
	times, speed := trace()
	measured := refract.NewTable().Time("t", times).Float64("speed", speed)

	p := refract.New(
		refract.Size(900, 520),
		refract.Title("Line 3 — last hour"),
		refract.YTitle("m/min"),
	)
	p.X(scale.Time())

	// The Y axis is the speed's alone. That is the difference a track makes:
	// with the strips drawn as negative values in this panel instead, Zero
	// would have to round around geometry rather than around data.
	p.Y(scale.Linear(scale.Zero(), scale.Nice()))
	p.Add(
		geom.Line(measured, geom.X("t"), geom.Y("speed"),
			geom.Color(palette.Blue), geom.Label("measured")),
		geom.HLine(120, geom.Label("target")),
	)

	// Two bands under the panel, stacked in the order they are added. Each has
	// an ordinal scale of its own and neither touches the speed's axis.
	p.Track(refract.Bottom, refract.TrackHeight(40)).
		Add(geom.Rect(states(times[0]),
			geom.X("start"), geom.X2("end"), geom.Y("state"),
			geom.ColorBy("state", scale.Qualitative(palette.Default))))

	p.Track(refract.Bottom, refract.TrackHeight(28)).
		Add(geom.Rect(orders(times[0]),
			geom.X("start"), geom.X2("end"), geom.Y("lane"),
			geom.ColorBy("order", scale.Qualitative(palette.Default))))

	return p.Render(refract.SVG(out))
}

// trace is an hour of line speed with a stoppage in the middle of it.
func trace() ([]time.Time, []float64) {
	const n = 240
	start := time.Date(2026, time.March, 14, 9, 0, 0, 0, time.UTC)
	times := make([]time.Time, n)
	values := make([]float64, n)
	for i := range n {
		times[i] = start.Add(time.Duration(i) * 15 * time.Second)
		switch {
		case i < 90:
			values[i] = 120 + 2*math.Sin(float64(i)/6)
		case i < 130:
			values[i] = 38 + math.Sin(float64(i)/3)
		default:
			values[i] = 119 + 2*math.Sin(float64(i)/6)
		}
	}
	return times, values
}

// states is what the machine was doing, as spans. They are the same spans the
// speed trace shows, named rather than inferred — which is the whole reason to
// draw both.
func states(start time.Time) *data.Table {
	at := func(step int) time.Time { return start.Add(time.Duration(step) * 15 * time.Second) }
	return refract.NewTable().
		Time("start", []time.Time{at(0), at(90), at(130)}).
		Time("end", []time.Time{at(90), at(130), at(240)}).
		String("state", []string{"running", "fault", "running"})
}

// orders is which work order was loaded. One lane, several spans: a track's
// vertical scale is its own, so a single-lane strip and a multi-lane one sit
// under the same chart without either knowing about the other.
func orders(start time.Time) *data.Table {
	at := func(step int) time.Time { return start.Add(time.Duration(step) * 15 * time.Second) }
	return refract.NewTable().
		Time("start", []time.Time{at(0), at(110), at(180)}).
		Time("end", []time.Time{at(110), at(180), at(240)}).
		String("lane", []string{"order", "order", "order"}).
		String("order", []string{"WO-4471", "WO-4472", "WO-4473"})
}
