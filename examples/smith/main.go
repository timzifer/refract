// Command smith renders the charts a third coordinate system unlocks.
//
// Like the other examples it is executed by a test, so it cannot silently stop
// compiling or stop producing a chart. And like examples/polar, its whole point
// is what is *not* here: there is no Smith geom, no reflection-coefficient
// scale and no RF package. Every chart below is a geom.Line or a geom.Scatter
// that shipped in v0.1, drawn in coord.Smith — which is what a pluggable
// coordinate stage buys.
//
// The two columns a Smith panel reads are the normalised impedance: r = R/Z₀ on
// X and x = X/Z₀ on Y. An instrument reports a reflection coefficient instead,
// so the first chart converts one with coord.SmithZ, which is the one line
// between a sweep and a picture of it.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func main() {
	antenna := flag.String("antenna", "antenna.svg", "output path for the measured sweep")
	matching := flag.String("matching", "matching.svg", "output path for the matching locus")
	admittance := flag.String("admittance", "admittance.svg", "output path for the admittance chart")
	flag.Parse()
	if err := run(*antenna, *matching, *admittance); err != nil {
		fmt.Fprintln(os.Stderr, "smith:", err)
		os.Exit(1)
	}
}

func run(antenna, matching, admittance string) error {
	for _, step := range []func() error{
		func() error { return measuredSweep(antenna) },
		func() error { return matchingNetwork(matching) },
		func() error { return shuntOnTheYChart(admittance) },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// smithAxes gives a plot the grid a paper Smith chart is printed with.
//
// Two things here are the whole recipe. scale.TickValues pins the tick
// positions, because 0.2 / 0.5 / 1 / 2 / 5 is a convention rather than the
// answer to a tick search — those six values are what the constant-resistance
// circles are drawn at on every chart ever printed, and no algorithm that looks
// for round numbers produces them. And the domains are pinned rather than
// trained, because a Smith chart's extent is the whole disc whatever the data
// does: a near-open reflection is a resistance in the thousands, and an axis
// that autoscaled to it would crowd every tick into the last pixel before the
// rim.
func smithAxes(p *refract.Plot) *refract.Plot {
	p.X(scale.Linear(
		scale.Domain(0, 50),
		scale.TickValues(0, 0.2, 0.5, 1, 2, 5),
	))
	p.Y(scale.Linear(
		scale.Domain(-50, 50),
		scale.TickValues(-5, -2, -1, -0.5, -0.2, 0.2, 0.5, 1, 2, 5),
	))
	return p
}

// measuredSweep is what an instrument hands you: S₁₁ against frequency, as a
// real and an imaginary part.
//
// coord.SmithZ is the conversion into the pair the panel reads, and it is the
// only RF arithmetic in this file — everything after it is a line over two
// columns. The markers are the band edges and the centre, which is the reading
// the chart is for: how far from the middle the antenna sits across the band.
func measuredSweep(out string) error {
	re, im := s11Sweep(121)
	r, x := make([]float64, len(re)), make([]float64, len(re))
	for i := range re {
		r[i], x[i] = coord.SmithZ(re[i], im[i])
	}
	src := refract.NewTable().Float64("r", r).Float64("x", x)

	// The three points the sweep is read at: where it starts, where it comes
	// closest to the middle — the frequency the antenna is tuned to — and
	// where it ends.
	best := bestMatch(re, im)
	marks := refract.NewTable().
		Float64("r", []float64{r[0], r[best], r[len(r)-1]}).
		Float64("x", []float64{x[0], x[best], x[len(x)-1]})

	p := smithAxes(refract.New(
		refract.Size(620, 560),
		refract.Title("A patch antenna across its band"),
		refract.Theme(theme.Light),
		refract.Coord(coord.Smith()),
		refract.Legend(false),
	))
	p.Add(geom.Line(src, geom.X("r"), geom.Y("x"), geom.Color(palette.OkabeIto[1])))
	p.Add(geom.Scatter(marks, geom.X("r"), geom.Y("x"), geom.Color(palette.OkabeIto[0])))
	return p.Render(refract.SVG(out))
}

// matchingNetwork is the chart's oldest job: showing that a load has been
// walked to the middle, and by what.
//
// The load is 15 − j25 Ω against a 50 Ω system. A series inductor moves it
// along its own constant-resistance circle; a shunt capacitor then moves it
// along a constant-conductance circle to the centre, which is a matched load
// and no reflection at all.
//
// coord.SmithArc is what makes the two steps read as steps. Each is a straight
// line in impedance — a series reactance adds to x and leaves r alone — and the
// image of a straight line under this map is an arc, so eight points joined by
// arcs draw the true path where eight points joined by chords would draw an
// octagon across it. That is the same distinction coord.Chord makes for a
// radar, made the other way round: there the default is the arc and the radar
// asks for chords, here the default is the chord and a computed locus asks for
// arcs.
func matchingNetwork(out string) error {
	series, shunt := matchLocus(9)
	step1 := refract.NewTable().
		Float64("r", realParts(series)).Float64("x", imagParts(series))
	step2 := refract.NewTable().
		Float64("r", realParts(shunt)).Float64("x", imagParts(shunt))

	p := smithAxes(refract.New(
		refract.Size(620, 560),
		refract.Title("Matching 15 − j25 Ω to 50 Ω"),
		refract.Theme(theme.Light),
		refract.Coord(coord.Smith(coord.SmithArc())),
		refract.Legend(false),
	))
	p.Add(geom.Line(step1, geom.X("r"), geom.Y("x"), geom.Color(palette.OkabeIto[1])))
	p.Add(geom.Line(step2, geom.X("r"), geom.Y("x"), geom.Color(palette.OkabeIto[2])))
	p.Add(geom.Scatter(refract.NewTable().
		Float64("r", []float64{real(series[0]), real(shunt[len(shunt)-1])}).
		Float64("x", []float64{imag(series[0]), imag(shunt[len(shunt)-1])}),
		geom.X("r"), geom.Y("x"), geom.Color(palette.OkabeIto[0])))
	return p.Render(refract.SVG(out))
}

// shuntOnTheYChart is the same locus on the admittance chart, which is the same
// disc turned through half a turn: y = 1/z gives Γ_y = −Γ_z.
//
// The columns change with it — they hold the conductance and the susceptance
// now — and the two changes cancel, so the locus lands on exactly the points it
// landed on above. That is the whole trick of the Y chart: it is not a
// different measurement, it is the same reflection read against the other grid,
// and the second step, which curves across the impedance chart, is a plain
// slide along a conductance circle here.
//
// coord.SmithArc is deliberately *not* used, and the reason is worth having in
// view: the option claims that the line between two marks is straight in the
// panel's own data space, and the panel's data space here is admittance. The
// second step is straight in it — a shunt susceptance adds to b — but the first
// one is straight in impedance, and a line in z is a circle in y. Asserting the
// wrong straightness would draw a confident curve through the right endpoints
// and the wrong middle. A fine sample and the honest chord is the right answer
// for a locus that is not straight in what the panel holds.
func shuntOnTheYChart(out string) error {
	series, shunt := matchLocus(60)
	all := append(append([]complex128{}, series...), shunt...)
	for i, z := range all {
		all[i] = 1 / z
	}
	src := refract.NewTable().
		Float64("g", realParts(all)).Float64("b", imagParts(all))

	p := smithAxes(refract.New(
		refract.Size(620, 560),
		refract.Title("The same two steps, read as admittance"),
		refract.Theme(theme.Light),
		refract.Coord(coord.Smith(coord.SmithAdmittance(true))),
		refract.Legend(false),
	))
	p.Add(geom.Line(src, geom.X("g"), geom.Y("b"), geom.Color(palette.OkabeIto[2])))
	return p.Render(refract.SVG(out))
}

// --- the data -------------------------------------------------------------

// s11Sweep is a synthesised reflection measurement: the textbook model of a
// patch antenna — a parallel RLC resonance behind the inductance of its feed —
// swept across the band it is resonant in. It stands in for a Touchstone file,
// which is a data source rather than a chart.
//
// The loop it traces is the shape the chart is read for: how tightly the locus
// curls, and how close to the middle it passes.
func s11Sweep(n int) (re, im []float64) {
	const (
		z0 = 50.0     // the system impedance, in ohms
		r  = 50.0     // the resonance's shunt resistance
		l  = 0.663e-9 // henries
		c  = 6.63e-12 // farads
		lf = 1.0e-9   // the feed inductance, which tilts the loop
	)
	re, im = make([]float64, n), make([]float64, n)
	for i := range n {
		f := 2.0e9 + 0.8e9*float64(i)/float64(n-1)
		w := 2 * math.Pi * f
		y := complex(1/r, w*c-1/(w*l))
		z := (complex(0, w*lf) + 1/y) / z0
		g := (z - 1) / (z + 1)
		re[i], im[i] = real(g), imag(g)
	}
	return re, im
}

// bestMatch is the index of the sample closest to the middle of the chart,
// which is the frequency the antenna is actually tuned to. It is the one
// reading a Smith chart makes that a magnitude plot cannot: where the locus
// passes, not merely how near it gets.
func bestMatch(re, im []float64) int {
	best, at := math.Inf(1), 0
	for i := range re {
		if d := math.Hypot(re[i], im[i]); d < best {
			best, at = d, i
		}
	}
	return at
}

// matchLocus is the two-element L network that takes 15 − j25 Ω to the centre:
// the impedances a series inductance walks through, and then the ones a shunt
// capacitance walks through.
//
// Both are computed rather than drawn, so the chart is a reading of a network
// and not an illustration of one.
func matchLocus(n int) (series, shunt []complex128) {
	load := complex(15, -25) / 50

	// A series reactance leaves the resistance alone, so the first step ends
	// where the load's own resistance circle crosses the unit-conductance
	// circle — which is where a shunt element can finish the job.
	r := real(load)
	end := complex(r, math.Sqrt(r*(1-r)))
	series = make([]complex128, n)
	for i := range n {
		t := float64(i) / float64(n-1)
		series[i] = complex(r, imag(load)+t*(imag(end)-imag(load)))
	}

	// A shunt susceptance leaves the conductance alone, so the second step runs
	// along a constant-conductance circle from there to the centre.
	y0, y1 := 1/end, complex(1, 0)
	shunt = make([]complex128, n)
	for i := range n {
		t := float64(i) / float64(n-1)
		y := complex(real(y0), imag(y0)+t*(imag(y1)-imag(y0)))
		shunt[i] = 1 / y
	}
	return series, shunt
}

func realParts(vs []complex128) []float64 {
	out := make([]float64, len(vs))
	for i, v := range vs {
		out[i] = real(v)
	}
	return out
}

func imagParts(vs []complex128) []float64 {
	out := make([]float64, len(vs))
	for i, v := range vs {
		out[i] = imag(v)
	}
	return out
}
