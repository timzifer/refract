// Command bigdata renders two charts that could not be drawn mark for mark.
//
// One is a line over two million samples; the other is a scatter of a million
// points. Neither asks for a reduction — the layers see how many rows they hold
// against how wide the plot is and choose one. The point of the example is what
// is *not* in it.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

func main() {
	dir := flag.String("d", ".", "output directory")
	flag.Parse()
	if err := run(*dir); err != nil {
		fmt.Fprintln(os.Stderr, "bigdata:", err)
		os.Exit(1)
	}
}

func run(dir string) error {
	for _, step := range []struct {
		file string
		draw func(string) error
	}{
		{"bigdata-signal.svg", signal},
		{"bigdata-cloud.svg", cloud},
	} {
		path := filepath.Join(dir, step.file)
		took, err := elapsed(func() error { return step.draw(path) })
		if err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		fmt.Printf("%s: %v, %d bytes\n", step.file, took.Round(time.Millisecond), info.Size())
	}
	return nil
}

// signal draws two million samples of a noisy trace with one dropout and one
// spike in it.
//
// Both survive: the dropout is a NaN, which the missing-data policy turns into
// a gap, and the spike is a single row that min/max-per-pixel-column keeps
// because it is the extreme of its column. Ask for geom.LTTB instead and the
// trace still looks right, but a one-sample spike is no longer guaranteed —
// LTTB weighs it against its neighbours.
func signal(out string) error {
	const n = 2_000_000
	t := make([]float64, n)
	v := make([]float64, n)
	for i := range n {
		x := float64(i) / n
		t[i] = float64(i)
		v[i] = math.Sin(20*math.Pi*x) + 0.3*math.Sin(701*math.Pi*x)
	}
	v[n/3] = 4.2          // a spike one sample wide
	for i := range 5000 { // and a dropout
		v[n/2+i] = math.NaN()
	}

	src := refract.Float64Columns(map[string][]float64{"i": t, "v": v})

	p := refract.New(
		refract.Size(900, 420),
		refract.Title("Two million samples"),
		refract.XTitle("sample"),
		refract.YTitle("volts"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(src,
		geom.X("i"), geom.Y("v"),
		geom.Color(palette.Blue),
		geom.Decimate(geom.MinMax), // keep every extreme; the spike matters
	))
	return p.Render(refract.SVG(out))
}

// cloud draws a million correlated points.
//
// Nothing here asks for a raster either. A million markers at six device units
// across would cover the plot area more than twenty times over, so the layer
// counts them per cell and draws the counts — which is the only honest picture
// of a million overlapping points.
func cloud(out string) error {
	const n = 1_000_000
	xs := make([]float64, n)
	ys := make([]float64, n)
	r := lcg(1)
	for i := range n {
		a, b := gauss(r), gauss(r)
		xs[i] = a
		ys[i] = 0.65*a + 0.76*b
	}

	src := refract.Float64Columns(map[string][]float64{"x": xs, "y": ys})

	p := refract.New(
		refract.Size(760, 480),
		refract.Title("A million points"),
		refract.XTitle("x"),
		refract.YTitle("y"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.Color(palette.Blue)))
	return p.Render(refract.SVG(out))
}

// lcg is a tiny deterministic generator, so that running the example twice
// produces the same picture.
func lcg(seed uint64) func() float64 {
	state := seed
	return func() float64 {
		state = state*6364136223846793005 + 1442695040888963407
		return float64(state>>11) / (1 << 53)
	}
}

func gauss(r func() float64) float64 {
	u, v := r(), r()
	return math.Sqrt(-2*math.Log(u+1e-15)) * math.Cos(2*math.Pi*v)
}

// elapsed times a step, so the example can print how long it took and how large
// the result is — the part nobody believes until they see it.
func elapsed(fn func() error) (time.Duration, error) {
	start := time.Now()
	err := fn()
	return time.Since(start), err
}
