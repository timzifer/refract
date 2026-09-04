// Command groups renders the grouped charts from the README.
//
// It is the v0.7 example, and like the others it is executed by a test so that
// it cannot silently stop compiling or stop producing a chart. It exercises the
// milestone's four pieces together: one layer over a long table split into
// series, the adjustments defined over those series, the rectangle mark, and
// the legend that names what a single swatch could not.
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
)

func main() {
	stacked := flag.String("stacked", "revenue.svg", "output path for the stacked bar chart")
	stream := flag.String("stream", "traffic.svg", "output path for the streamgraph")
	heatmap := flag.String("heatmap", "calls.svg", "output path for the heatmap")
	gantt := flag.String("gantt", "plan.svg", "output path for the gantt chart")
	flag.Parse()
	if err := run(*stacked, *stream, *heatmap, *gantt); err != nil {
		fmt.Fprintln(os.Stderr, "groups:", err)
		os.Exit(1)
	}
}

func run(stacked, stream, heatmap, gantt string) error {
	for _, step := range []func() error{
		func() error { return revenueByProduct(stacked) },
		func() error { return trafficByChannel(stream) },
		func() error { return callsByHour(heatmap) },
		func() error { return plan(gantt) },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// revenueByProduct is one layer over a long table: three products, four
// quarters, twelve rows, one stacked bar per quarter.
func revenueByProduct(out string) error {
	quarter, product, revenue := revenue()
	src := refract.NewTable().
		String("quarter", quarter).
		String("product", product).
		Float64("revenue", revenue)

	p := refract.New(
		refract.Size(700, 420),
		refract.Title("Revenue by product"),
		refract.YTitle("k€"),
	)
	p.X(scale.Ordinal())
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	// A grouped bar stacks unless it is told otherwise, and the axis is trained
	// on the totals rather than on the individual values — so the tallest stack
	// fits. geom.Dodge(0.1) would put the three products side by side instead.
	p.Add(geom.Bar(src,
		geom.X("quarter"), geom.Y("revenue"),
		geom.GroupBy("product"),
		geom.ColorBy("product", scale.Qualitative(palette.OkabeIto)),
	))
	return p.Render(refract.SVG(out))
}

// trafficByChannel is the same shape of table drawn as a streamgraph: stacked
// areas about a wiggling baseline, with the series ordered so that the ones
// peaking earliest sit in the middle.
func trafficByChannel(out string) error {
	day, channel, visits := traffic()
	src := refract.NewTable().
		Float64("day", day).
		String("channel", channel).
		Float64("visits", visits)

	p := refract.New(
		refract.Size(760, 380),
		refract.Title("Traffic by channel"),
		refract.XTitle("day"),
	)
	p.X(scale.Linear())
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Area(src,
		geom.X("day"), geom.Y("visits"),
		geom.GroupBy("channel"),
		geom.Stack(geom.StackWiggle),
		geom.Order(geom.OrderInsideOut),
		geom.ColorBy("channel", scale.Qualitative(palette.OkabeIto)),
	))
	return p.Render(refract.SVG(out))
}

// callsByHour is a heatmap, which is a rect and a ramp and nothing else: the
// cells take their size from the two band scales, so no column says how wide a
// cell is.
func callsByHour(out string) error {
	day, hour, calls := calls()
	src := refract.NewTable().
		String("day", day).
		String("hour", hour).
		Float64("calls", calls)

	p := refract.New(
		refract.Size(720, 380),
		refract.Title("Calls per hour"),
	)
	p.X(scale.Ordinal(scale.OrdinalPadding(0)))
	p.Y(scale.Ordinal(scale.OrdinalPadding(0)))
	p.Add(geom.Rect(src,
		geom.X("day"), geom.Y("hour"),
		geom.ColorBy("calls", scale.Sequential(palette.Viridis)),
	))
	return p.Render(refract.SVG(out))
}

// plan is a gantt chart: a rect that names both of its horizontal edges, on a
// time axis against an ordinal one.
func plan(out string) error {
	start := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	day := func(n int) time.Time { return start.AddDate(0, 0, n) }
	src := refract.NewTable().
		String("task", []string{"design", "build", "review", "ship"}).
		Time("from", []time.Time{day(0), day(3), day(9), day(12)}).
		Time("to", []time.Time{day(4), day(10), day(12), day(14)})

	p := refract.New(
		refract.Size(760, 300),
		refract.Title("Plan"),
	)
	p.X(scale.Time())
	p.Y(scale.Ordinal())
	p.Add(geom.Rect(src,
		geom.X("from"), geom.X2("to"), geom.Y("task"),
		geom.Fill(palette.SkyBlue), geom.Color(palette.Blue),
	))
	return p.Render(refract.SVG(out))
}

// revenue is the long table the stacked bar is drawn from: one row per
// (quarter, product) pair, which is the shape a grouped layer wants.
func revenue() (quarter, product []string, value []float64) {
	quarters := []string{"Q1", "Q2", "Q3", "Q4"}
	products := []string{"prism", "lens", "filter"}
	for q, name := range quarters {
		for i, p := range products {
			quarter = append(quarter, name)
			product = append(product, p)
			value = append(value, 20+float64(q)*4+float64(i)*9-float64(q*i))
		}
	}
	return quarter, product, value
}

// traffic is four channels over eight weeks. A fixed recurrence rather than
// math/rand keeps the example's output stable, which is what lets a test
// assert on it.
func traffic() (day []float64, channel []string, visits []float64) {
	names := []string{"search", "social", "direct", "email"}
	for d := range 56 {
		for i, name := range names {
			t := float64(d) / 8
			v := 40 + 30*math.Sin(t/2+float64(i)*1.7) + 8*math.Sin(t*3+float64(i))
			day = append(day, float64(d))
			channel = append(channel, name)
			visits = append(visits, math.Max(v, 1))
		}
	}
	return day, channel, visits
}

// calls is one row per cell of the heatmap.
func calls() (day, hour []string, count []float64) {
	days := []string{"mon", "tue", "wed", "thu", "fri"}
	for d, name := range days {
		for h := 8; h < 18; h++ {
			day = append(day, name)
			hour = append(hour, fmt.Sprintf("%02d:00", h))
			// Two peaks, one mid-morning and one mid-afternoon, tailing off
			// towards the end of the week.
			v := 60*math.Exp(-math.Pow(float64(h)-10, 2)/6) +
				45*math.Exp(-math.Pow(float64(h)-15, 2)/8)
			count = append(count, v*(1-float64(d)*0.12))
		}
	}
	return day, hour, count
}
