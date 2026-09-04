// Command gallery renders every figure used in the README and the docs.
//
// It lives in the gg module because it is the only place that can produce both
// halves of each figure: the SVG comes from refract's built-in emitter, the PNG
// from this backend. Rendering both from one specification is also the
// cross-backend check — if the two ever stop agreeing, a figure will visibly
// disagree with itself.
//
// CI runs this with -check, which regenerates every figure into a scratch
// directory and fails if anything differs from what is committed. So the images
// in the README can never drift away from the code that produced them.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/timzifer/refract"
	ggbackend "github.com/timzifer/refract/backend/gg"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/svgdiff"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func main() {
	dir := flag.String("dir", filepath.Join("docs", "images"), "output directory")
	check := flag.Bool("check", false, "verify the committed figures are up to date instead of writing them")
	flag.Parse()

	if err := run(*dir, *check); err != nil {
		fmt.Fprintln(os.Stderr, "gallery:", err)
		os.Exit(1)
	}
}

func run(dir string, check bool) error {
	if check {
		return verify(dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range figures() {
		svg, png, err := f.render()
		if err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, f.name+".svg"), svg, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, f.name+".png"), png, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s.svg (%d bytes) and %s.png (%d bytes)\n", f.name, len(svg), f.name, len(png))
	}
	return nil
}

// verify re-renders every figure and compares it to what is on disk.
//
// Both halves are compared with a tolerance, for the same underlying reason:
// floating-point results are not bit-identical across architectures. In the SVG
// half everything but the numbers must match exactly and coordinates may differ
// by a hundredth of a pixel (see internal/svgdiff); in the PNG half a small
// per-channel difference is allowed. Neither tolerance is wide enough to hide a
// real change to a chart.
func verify(dir string) error {
	var problems []string
	for _, f := range figures() {
		svg, png, err := f.render()
		if err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
		if msg := compareSVG(filepath.Join(dir, f.name+".svg"), svg); msg != "" {
			problems = append(problems, msg)
		}
		if msg := compareImage(filepath.Join(dir, f.name+".png"), png); msg != "" {
			problems = append(problems, msg)
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, " ", p)
		}
		return errors.New("figures are out of date; run `go run ./backend/gg/cmd/gallery` and commit the result")
	}
	fmt.Println("all figures up to date")
	return nil
}

// compareSVG checks a freshly rendered figure against the committed one.
// "got" is what the code produces now, "want" is what is on disk — the same
// convention the golden tests use, so the failure messages read the same way.
func compareSVG(path string, got []byte) string {
	want, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("%s: %v", path, err)
	}
	if ok, why := svgdiff.Equal(got, want, svgdiff.DefaultTolerance); !ok {
		return fmt.Sprintf("%s: %s", path, why)
	}
	return ""
}

func compareImage(path string, want []byte) string {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("%s: %v", path, err)
	}
	diff, err := pngDiff(got, want)
	if err != nil {
		return fmt.Sprintf("%s: %v", path, err)
	}
	if diff > pngTolerance {
		return fmt.Sprintf("%s: differs (%.4f%% of channels beyond tolerance)", path, diff*100)
	}
	return ""
}

// figure is one documented chart.
type figure struct {
	name  string
	width int
	high  int
	build func(*refract.Plot)
	theme theme.Theme
	title string
}

func (f figure) plot() *refract.Plot {
	p := refract.New(
		refract.Theme(f.theme),
		refract.Size(f.width, f.high),
		refract.Title(f.title),
	)
	f.build(p)
	return p
}

func (f figure) render() (svg, png []byte, err error) {
	var sb bytes.Buffer
	if err := f.plot().Render(refract.SVGWriter(&sb)); err != nil {
		return nil, nil, err
	}
	var pb bytes.Buffer
	if err := f.plot().Render(ggbackend.Writer(&pb, ggbackend.FormatPNG)); err != nil {
		return nil, nil, err
	}
	return sb.Bytes(), pb.Bytes(), nil
}

func figures() []figure {
	return []figure{
		{
			name: "signal", width: 800, high: 400, theme: theme.Dark, title: "Signal",
			build: func(p *refract.Plot) {
				times, values := damped()
				src := refract.NewTable().Time("t", times).Float64("y", values)
				p.X(scale.Time())
				p.Y(scale.Linear(scale.Nice()))
				p.Add(geom.Line(src,
					geom.X("t"), geom.Y("y"),
					geom.Color(palette.SkyBlue),
					geom.Tension(0.4),
				))
			},
		},
		{
			name: "series", width: 800, high: 400, theme: theme.Light, title: "Three series",
			build: func(p *refract.Plot) {
				xs := ramp(0, 10, 120)
				src := refract.Float64Columns(map[string][]float64{
					"x":      xs,
					"linear": apply(xs, func(x float64) float64 { return x }),
					"square": apply(xs, func(x float64) float64 { return x * x / 10 }),
					"root":   apply(xs, func(x float64) float64 { return 3 * math.Sqrt(x) }),
				})
				p.X(scale.Linear(scale.Nice()))
				p.Y(scale.Linear(scale.Nice(), scale.Zero()))
				p.Add(
					geom.Line(src, geom.X("x"), geom.Y("linear"), geom.Label("linear")),
					geom.Line(src, geom.X("x"), geom.Y("square"), geom.Label("square"), geom.Dash(6, 4)),
					geom.Line(src, geom.X("x"), geom.Y("root"), geom.Label("root"), geom.Dash(2, 3)),
				)
			},
		},
		{
			name: "scatter", width: 700, high: 420, theme: theme.Light, title: "Scatter",
			build: func(p *refract.Plot) {
				xs, a, b := clusters()
				src := refract.Float64Columns(map[string][]float64{"x": xs, "a": a, "b": b})
				p.X(scale.Linear(scale.Nice()))
				p.Y(scale.Linear(scale.Nice()))
				p.Add(
					geom.Scatter(src, geom.X("x"), geom.Y("a"), geom.Label("group A"),
						geom.Shape(ir.MarkerCircle), geom.Size(7)),
					geom.Scatter(src, geom.X("x"), geom.Y("b"), geom.Label("group B"),
						geom.Shape(ir.MarkerDiamond), geom.Size(7)),
				)
			},
		},
		{
			name: "bars", width: 700, high: 400, theme: theme.Light, title: "Response time distribution",
			build: func(p *refract.Plot) {
				// Bin centres on a continuous axis: v0.1 has no ordinal scale,
				// and a histogram is the bar chart that genuinely wants a
				// numeric X anyway.
				bins := ramp(5, 95, 10)
				counts := []float64{18, 47, 82, 96, 71, 44, 25, 13, 6, 2}
				src := refract.Float64Columns(map[string][]float64{"ms": bins, "count": counts})
				p.X(scale.Linear())
				p.Y(scale.Linear(scale.Nice(), scale.Zero()))
				p.Add(geom.Bar(src, geom.X("ms"), geom.Y("count"), geom.Color(palette.Green)))
			},
		},
	}
}

// --- sample data ---------------------------------------------------------

func damped() ([]time.Time, []float64) {
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

func ramp(lo, hi float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		if n == 1 {
			out[i] = lo
			continue
		}
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

// clusters builds two deterministic point clouds. A fixed recurrence rather
// than math/rand keeps the figures byte-stable across Go releases, which is
// what makes the -check mode meaningful.
func clusters() (xs, a, b []float64) {
	const n = 45
	xs = make([]float64, n)
	a = make([]float64, n)
	b = make([]float64, n)
	for i := range n {
		t := float64(i) / n
		xs[i] = t * 10
		a[i] = 4 + 3*math.Sin(7*t) + 0.8*math.Sin(31*t)
		b[i] = 9 - 2.5*math.Cos(5*t) + 0.6*math.Cos(23*t)
	}
	return xs, a, b
}
