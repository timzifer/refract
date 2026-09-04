package refract_test

import (
	"bytes"
	"errors"
	"flag"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/backend/svg"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/svgdiff"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

var update = flag.Bool("update", false, "rewrite the golden SVG files")

// golden compares an SVG render against testdata/golden/<name>.svg.
//
// Golden files are written in pretty mode, one element per line, so that a
// failure shows up as a handful of changed lines in a diff rather than as one
// enormous line.
//
// Everything except numbers is compared byte for byte; coordinates are allowed
// to differ by a hundredth of a pixel. That is not slack for sloppy rendering —
// it is the width of a float32 unit in the last place, which arm64 and amd64
// genuinely disagree about because Go contracts a*b+c into an FMA on one and
// not the other. See internal/svgdiff for the full reasoning.
func golden(t *testing.T, name string, p *refract.Plot) {
	t.Helper()

	var buf bytes.Buffer
	if err := p.Render(refract.SVGWriter(&buf, svg.Pretty())); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.Bytes()

	path := filepath.Join("testdata", "golden", name+".svg")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run `go test ./... -update` to create it)", err)
	}
	if ok, why := svgdiff.Equal(got, want, svgdiff.DefaultTolerance); !ok {
		t.Errorf("%s differs from the golden file: %s", name, why)
	}
}

// --- the charts ----------------------------------------------------------

func TestGoldenLineChart(t *testing.T) {
	xs := ramp(0, 10, 40)
	src := refract.Float64Columns(map[string][]float64{
		"x": xs,
		"y": apply(xs, func(v float64) float64 { return math.Sin(v) }),
	})
	p := refract.New(
		refract.Size(640, 400),
		refract.Title("Sine"),
		refract.XTitle("x"),
		refract.YTitle("sin x"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y"), geom.Color(palette.Blue)))
	golden(t, "line", p)
}

func TestGoldenMultiSeriesWithLegend(t *testing.T) {
	xs := ramp(0, 4, 25)
	src := refract.Float64Columns(map[string][]float64{
		"x": xs,
		"a": apply(xs, func(v float64) float64 { return v }),
		"b": apply(xs, func(v float64) float64 { return v * v }),
	})
	p := refract.New(refract.Size(640, 400), refract.Title("Two series"))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(
		geom.Line(src, geom.X("x"), geom.Y("a"), geom.Label("linear")),
		geom.Line(src, geom.X("x"), geom.Y("b"), geom.Label("square"), geom.Dash(5, 3)),
	)
	golden(t, "legend", p)
}

func TestGoldenScatterAndBars(t *testing.T) {
	t.Run("scatter", func(t *testing.T) {
		xs := ramp(0, 6, 20)
		src := refract.Float64Columns(map[string][]float64{
			"x": xs,
			"y": apply(xs, func(v float64) float64 { return math.Cos(v) }),
		})
		p := refract.New(refract.Size(480, 320), refract.Title("Scatter"))
		p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.Shape(ir.MarkerDiamond)))
		golden(t, "scatter", p)
	})

	t.Run("bars", func(t *testing.T) {
		src := refract.Float64Columns(map[string][]float64{
			"x": {1, 2, 3, 4, 5},
			"y": {3, 7, 4, 9, 6},
		})
		p := refract.New(refract.Size(480, 320), refract.Title("Bars"))
		p.Y(scale.Linear(scale.Nice(), scale.Zero()))
		p.Add(geom.Bar(src, geom.X("x"), geom.Y("y"), geom.Color(palette.Green)))
		golden(t, "bars", p)
	})
}

func TestGoldenTimeAxisDarkTheme(t *testing.T) {
	const n = 120
	start := time.Date(2026, time.March, 14, 9, 0, 0, 0, time.UTC)
	times := make([]time.Time, n)
	values := make([]float64, n)
	for i := range n {
		times[i] = start.Add(time.Duration(i) * 30 * time.Second)
		x := float64(i) / n
		values[i] = math.Exp(-2*x) * math.Sin(8*math.Pi*x)
	}
	src := refract.NewTable().Time("t", times).Float64("y", values)

	p := refract.New(
		refract.Theme(theme.Dark),
		refract.Size(720, 400),
		refract.Title("Signal"),
		refract.YTitle("amplitude"),
	)
	p.X(scale.Time())
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(src, geom.X("t"), geom.Y("y"), geom.Color(palette.SkyBlue), geom.Tension(0.4)))
	golden(t, "time-dark", p)
}

// --- behaviour -----------------------------------------------------------

func TestRenderIsRepeatable(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1, 2}, "y": {1, 3, 2}})
	p := refract.New(refract.Size(300, 200), refract.Title("Repeat"))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))

	var a, b bytes.Buffer
	if err := p.Render(refract.SVGWriter(&a)); err != nil {
		t.Fatal(err)
	}
	if err := p.Render(refract.SVGWriter(&b)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("rendering the same plot twice produced different output")
	}
}

func TestRenderToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chart.svg")
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {0, 1}})
	p := refract.New(refract.Size(200, 150))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))

	if err := p.Render(refract.SVG(path)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the file was not written: %v", err)
	}
	if !bytes.HasSuffix(b, []byte("</svg>")) {
		t.Fatal("the file on disk is not a complete document — Render must close the target")
	}
}

func TestLegendAppearsAutomaticallyForMultipleLayers(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "a": {0, 1}, "b": {1, 0}})

	one := refract.New(refract.Size(400, 300))
	one.Add(geom.Line(src, geom.X("x"), geom.Y("a")))

	two := refract.New(refract.Size(400, 300))
	two.Add(
		geom.Line(src, geom.X("x"), geom.Y("a")),
		geom.Line(src, geom.X("x"), geom.Y("b")),
	)

	if strings.Contains(renderString(t, one), ">a<") {
		t.Error("a single-layer plot should not draw a legend by default")
	}
	if !strings.Contains(renderString(t, two), ">a<") {
		t.Error("a multi-layer plot should draw a legend by default")
	}
}

func TestLegendCanBeForcedOff(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "a": {0, 1}, "b": {1, 0}})
	p := refract.New(refract.Size(400, 300), refract.Legend(false))
	p.Add(
		geom.Line(src, geom.X("x"), geom.Y("a")),
		geom.Line(src, geom.X("x"), geom.Y("b")),
	)
	if strings.Contains(renderString(t, p), ">a<") {
		t.Error("Legend(false) did not suppress the legend")
	}
}

func TestErrorsSurface(t *testing.T) {
	t.Run("nil target", func(t *testing.T) {
		if err := refract.New().Render(nil); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("empty plot", func(t *testing.T) {
		var buf bytes.Buffer
		if err := refract.New().Render(refract.SVGWriter(&buf)); !errors.Is(err, refract.ErrNoLayers) {
			t.Fatalf("err = %v, want ErrNoLayers", err)
		}
	})
	t.Run("bad column", func(t *testing.T) {
		src := refract.Float64Columns(map[string][]float64{"x": {1}})
		p := refract.New()
		p.Add(geom.Line(src, geom.X("x"), geom.Y("nope")))
		var buf bytes.Buffer
		if err := p.Render(refract.SVGWriter(&buf)); err == nil {
			t.Fatal("want an error naming the missing column")
		}
	})
	t.Run("unwritable file", func(t *testing.T) {
		src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {0, 1}})
		p := refract.New()
		p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
		// A directory that does not exist cannot be created implicitly.
		bad := filepath.Join(t.TempDir(), "no-such-dir", "chart.svg")
		if err := p.Render(refract.SVG(bad)); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestDefaultScalesAreLinear(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1, 2}, "y": {0, 5, 10}})
	p := refract.New(refract.Size(400, 300))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	// The step lands on 2.5, so every label in the column carries one decimal.
	if got := renderString(t, p); !strings.Contains(got, ">10.0<") {
		t.Errorf("expected a tick at 10 from the default linear scale:\n%s", got)
	}
}

// --- helpers -------------------------------------------------------------

func renderString(t *testing.T, p *refract.Plot) string {
	t.Helper()
	var buf bytes.Buffer
	if err := p.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

func ramp(lo, hi float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
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
