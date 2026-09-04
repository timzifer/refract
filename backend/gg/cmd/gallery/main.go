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
	"github.com/timzifer/refract/facet"
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
//
// A figure carrying an embedded raster has that raster compared as an image
// rather than as the base64 deflate stream it is written as; see embedded.go
// for why. Everything else — the vector half, and the <image> element's own
// position and size — is compared exactly, as before.
func compareSVG(path string, got []byte) string {
	want, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("%s: %v", path, err)
	}
	gotDoc, gotRasters := splitEmbedded(got)
	wantDoc, wantRasters := splitEmbedded(want)
	if msg := compareEmbedded(path, gotRasters, wantRasters); msg != "" {
		return msg
	}
	if ok, why := svgdiff.Equal(gotDoc, wantDoc, svgdiff.DefaultTolerance); !ok {
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
//
// Most are a single plot, described by build. A figure that documents subplots
// is a grid of plots instead, described by grid; exactly one of the two is set.
type figure struct {
	name  string
	width int
	high  int
	build func(*refract.Plot)
	grid  func(*refract.Grid)
	theme theme.Theme
	title string
}

// chart is anything that can be rendered into a target: a plot, or a grid of
// them. Both halves of every figure go through it, which is what keeps the SVG
// and the PNG two renders of one specification.
type chart interface {
	Render(refract.Target) error
}

func (f figure) chart() chart {
	if f.grid != nil {
		g := refract.NewGrid(2,
			refract.GridTheme(f.theme),
			refract.GridSize(f.width, f.high),
			refract.GridTitle(f.title),
		)
		f.grid(g)
		return g
	}
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
	if err := f.chart().Render(refract.SVGWriter(&sb)); err != nil {
		return nil, nil, err
	}
	var pb bytes.Buffer
	if err := f.chart().Render(ggbackend.Writer(&pb, ggbackend.FormatPNG)); err != nil {
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
				// Bin centres on a continuous axis. A histogram is the bar
				// chart that genuinely wants a numeric X: the gaps between
				// bins carry meaning, which is exactly what an ordinal axis
				// throws away.
				bins := ramp(5, 95, 10)
				counts := []float64{18, 47, 82, 96, 71, 44, 25, 13, 6, 2}
				src := refract.Float64Columns(map[string][]float64{"ms": bins, "count": counts})
				p.X(scale.Linear())
				p.Y(scale.Linear(scale.Nice(), scale.Zero()))
				p.Add(geom.Bar(src, geom.X("ms"), geom.Y("count"), geom.Color(palette.Green)))
			},
		},
		{
			name: "area", width: 700, high: 400, theme: theme.Light, title: "Estimate and interval",
			build: func(p *refract.Plot) {
				xs := ramp(0, 12, 120)
				src := refract.Float64Columns(map[string][]float64{
					"x":  xs,
					"y":  apply(xs, func(x float64) float64 { return math.Sin(x) + x/6 }),
					"lo": apply(xs, func(x float64) float64 { return math.Sin(x) + x/6 - 0.3 - x/20 }),
					"hi": apply(xs, func(x float64) float64 { return math.Sin(x) + x/6 + 0.3 + x/20 }),
				})
				p.X(scale.Linear(scale.Nice()))
				p.Y(scale.Linear(scale.Nice()))
				p.Add(
					geom.Area(src, geom.X("x"), geom.Y("hi"), geom.Y2("lo"),
						geom.Label("interval"), geom.Color(palette.SkyBlue), geom.Width(1)),
					geom.Line(src, geom.X("x"), geom.Y("y"),
						geom.Label("estimate"), geom.Color(palette.Blue)),
				)
			},
		},
		{
			name: "steps", width: 700, high: 380, theme: theme.Dark, title: "Replicas over the day",
			build: func(p *refract.Plot) {
				hours := ramp(0, 23, 24)
				replicas := []float64{2, 2, 2, 2, 2, 3, 5, 8, 12, 14, 14, 13, 13, 14, 15, 15, 13, 10, 7, 5, 4, 3, 2, 2}
				src := refract.Float64Columns(map[string][]float64{"hour": hours, "n": replicas})
				p.X(scale.Linear())
				p.Y(scale.Linear(scale.Nice(), scale.Zero()))
				p.Add(geom.Step(src, geom.X("hour"), geom.Y("n"),
					geom.Color(palette.Yellow), geom.Width(2)))
			},
		},
		{
			name: "categories", width: 700, high: 400, theme: theme.Light, title: "Sales by region",
			build: func(p *refract.Plot) {
				src := refract.NewTable().
					String("region", []string{"north", "south", "east", "west", "central", "overseas"}).
					Float64("sales", []float64{18, 42, 31, 25, 37, 12})
				p.X(scale.Ordinal())
				p.Y(scale.Linear(scale.Nice(), scale.Zero()))
				p.Add(geom.Bar(src, geom.X("region"), geom.Y("sales"),
					geom.ColorBy("sales", scale.Sequential(palette.Viridis))))
			},
		},
		{
			name: "boxplot", width: 700, high: 400, theme: theme.Light, title: "Latency by cohort",
			build: func(p *refract.Plot) {
				keys, vals := cohorts()
				src := refract.NewTable().String("cohort", keys).Float64("ms", vals)
				p.X(scale.Ordinal())
				p.Y(scale.Linear(scale.Nice()))
				p.Add(geom.Boxplot(src, geom.X("cohort"), geom.Y("ms"), geom.Color(palette.Blue)))
			},
		},
		{
			name: "logscale", width: 700, high: 400, theme: theme.Dark, title: "Requests per second",
			build: func(p *refract.Plot) {
				xs := ramp(0, 14, 140)
				src := refract.Float64Columns(map[string][]float64{
					"week":  xs,
					"rps":   apply(xs, func(x float64) float64 { return 5 * math.Exp(0.72*x) }),
					"floor": apply(xs, func(x float64) float64 { return 5 * math.Exp(0.45*x) }),
				})
				p.X(scale.Linear(scale.Nice()))
				p.Y(scale.Log(scale.LogNice()))
				p.Add(
					geom.Line(src, geom.X("week"), geom.Y("rps"),
						geom.Label("actual"), geom.Color(palette.Green)),
					geom.Line(src, geom.X("week"), geom.Y("floor"),
						geom.Label("plan"), geom.Color(palette.Orange), geom.Dash(6, 4)),
				)
			},
		},
		{
			name: "facets", width: 800, high: 460, theme: theme.Light, title: "Throughput by region",
			build: func(p *refract.Plot) {
				regions, hours, rps := fleet()
				src := refract.NewTable().
					String("region", regions).
					Float64("hour", hours).
					Float64("rps", rps)
				p.X(scale.Linear(scale.Nice()))
				p.Y(scale.Linear(scale.Nice(), scale.Zero()))
				p.Add(geom.Line(src, geom.X("hour"), geom.Y("rps"),
					geom.Color(palette.Blue), geom.Label("throughput")))
				p.Add(geom.HLine(60, geom.Label("target")))
				p.Facet(facet.Wrap("region", facet.Columns(3)))
			},
		},
		{
			name: "annotations", width: 760, high: 400, theme: theme.Light, title: "Latency against its budget",
			build: func(p *refract.Plot) {
				xs := ramp(0, 60, 180)
				src := refract.Float64Columns(map[string][]float64{
					"minute": xs,
					"p99": apply(xs, func(x float64) float64 {
						return 190 + 45*math.Sin(x/7) + 12*math.Sin(x/2.3)
					}),
				})
				p.X(scale.Linear(scale.Nice()))
				p.Y(scale.Linear(scale.Nice(), scale.Zero()))
				p.Add(
					geom.HBand(170, 210, geom.Label("tolerance")),
					geom.VBand(22, 26, geom.Fill(palette.Orange), geom.Opacity(0.18), geom.Label("deploy")),
					geom.Line(src, geom.X("minute"), geom.Y("p99"),
						geom.Color(palette.Blue), geom.Label("p99")),
					geom.HLine(250, geom.Label("SLO")),
					geom.Note(1, 246, "budget", geom.FontSize(11), geom.Align(ir.AlignStart, ir.AlignTop)),
				)
			},
		},
		{
			name: "decimation", width: 800, high: 400, theme: theme.Light,
			title: "A quarter of a million samples",
			build: func(p *refract.Plot) {
				xs, ys := trace(250_000)
				src := refract.Float64Columns(map[string][]float64{"i": xs, "v": ys})
				p.X(scale.Linear(scale.Nice()))
				p.Y(scale.Linear(scale.Nice()))
				// No option asks for the reduction: the layer sees a quarter of
				// a million rows against eight hundred pixel columns and picks
				// one. The spike is still full height.
				p.Add(geom.Line(src, geom.X("i"), geom.Y("v"), geom.Color(palette.Blue)))
			},
		},
		{
			name: "density", width: 760, high: 440, theme: theme.Light,
			title: "A million points",
			build: func(p *refract.Plot) {
				xs, ys := cloud(1_000_000)
				src := refract.Float64Columns(map[string][]float64{"x": xs, "y": ys})
				p.X(scale.Linear(scale.Nice()))
				p.Y(scale.Linear(scale.Nice()))
				// Markers this dense would bury each other and the picture would
				// be decided by row order, so the layer counts per cell and
				// draws the counts instead.
				p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.Color(palette.Blue)))
			},
		},
		{
			name: "subplots", width: 800, high: 480, theme: theme.Dark, title: "Fleet overview",
			grid: func(g *refract.Grid) {
				xs := ramp(0, 12, 120)
				add := func(title string, fn func(float64) float64, c int) {
					p := refract.New(refract.Title(title))
					p.X(scale.Linear(scale.Nice()))
					p.Y(scale.Linear(scale.Nice()))
					p.Add(geom.Line(refract.Float64Columns(map[string][]float64{
						"x": xs, "y": apply(xs, fn),
					}), geom.X("x"), geom.Y("y"), geom.Color(palette.OkabeIto.At(c))))
					g.Add(p)
				}
				add("latency", func(x float64) float64 { return 50 + 20*math.Sin(x) }, 0)
				add("throughput", func(x float64) float64 { return 900 * math.Exp(-x/9) }, 1)
				add("errors", func(x float64) float64 { return 3 * math.Sin(x/2) * math.Sin(x/2) }, 2)
				add("saturation", func(x float64) float64 { return 0.45 + 0.3*math.Sin(x/3) }, 3)
			},
		},
	}
}

// fleet builds five regions of hourly throughput, deterministically. A fixed
// formula rather than math/rand is what makes -check meaningful.
func fleet() (regions []string, hours, rps []float64) {
	for ri, name := range []string{"north", "south", "east", "west", "central"} {
		for h := range 24 {
			regions = append(regions, name)
			hours = append(hours, float64(h))
			base := 40 + 12*float64(ri)
			rps = append(rps, base+18*math.Sin(float64(h)/3.8+float64(ri)))
		}
	}
	return regions, hours, rps
}

// cohorts builds three deterministic latency distributions, each with a tail.
// A fixed recurrence rather than math/rand keeps the figure byte-stable across
// Go releases, which is what makes -check meaningful.
func cohorts() ([]string, []float64) {
	names := []string{"alpha", "beta", "gamma"}
	var keys []string
	var vals []float64
	for g, name := range names {
		for i := range 40 {
			t := float64(i) / 40
			v := 24 + float64(g)*7 + 9*math.Sin(9*t+float64(g)) + 3*math.Sin(37*t) + 2*math.Cos(61*t)
			keys, vals = append(keys, name), append(vals, v)
		}
		// One tail observation per cohort, so the outlier path is exercised by
		// a figure and not only by a test.
		keys, vals = append(keys, name), append(vals, 62+float64(g)*5)
	}
	return keys, vals
}

// trace builds a long, detailed signal with one spike in it: the shape that
// needs decimating, and the feature that catches a reduction which flattens.
// A fixed recurrence rather than math/rand keeps the figure byte-stable.
func trace(n int) (xs, ys []float64) {
	xs = make([]float64, n)
	ys = make([]float64, n)
	for i := range n {
		t := float64(i) / float64(n)
		xs[i] = float64(i)
		ys[i] = math.Sin(6*math.Pi*t) +
			0.35*math.Sin(211*math.Pi*t) +
			0.18*math.Sin(1013*math.Pi*t)
	}
	ys[n/3] = 2.6
	return xs, ys
}

// cloud builds a correlated point cloud: the overplotted scatter a density
// raster exists for.
//
// The generator is written out here rather than taken from math/rand so that
// the figure is byte-stable for -check no matter what any library does later.
// A low-discrepancy sequence would be shorter, but it lays the points on a
// lattice, and a picture of a lattice is not a picture of a point cloud.
func cloud(n int) (xs, ys []float64) {
	xs = make([]float64, n)
	ys = make([]float64, n)
	var seed uint64 = 0x9e3779b97f4a7c15
	next := func() float64 {
		// splitmix64, then the top 53 bits as a fraction of one.
		seed += 0x9e3779b97f4a7c15
		z := seed
		z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
		z = (z ^ (z >> 27)) * 0x94d049bb133111eb
		z ^= z >> 31
		return float64(z>>11) / (1 << 53)
	}
	for i := range n {
		u, v := next(), next()
		r := math.Sqrt(-2 * math.Log(u+1e-15))
		a, b := r*math.Cos(2*math.Pi*v), r*math.Sin(2*math.Pi*v)
		xs[i] = a
		ys[i] = 0.65*a + 0.76*b
	}
	return xs, ys
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
