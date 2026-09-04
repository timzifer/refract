package refract_test

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/backend/pdf"
	"github.com/timzifer/refract/backend/svg"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/svgdiff"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// goldenBytes is the golden comparison for anything that is not a single Plot.
func goldenBytes(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".svg")
	if *update {
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

// sin is math.Sin, named locally so the chart definitions above read as
// formulas rather than as calls.
func sin(v float64) float64 { return math.Sin(v) }

func facetSource() refract.Source {
	var region []string
	var x, y []float64
	for ri, r := range []string{"north", "south", "east"} {
		for i := range 12 {
			region = append(region, r)
			x = append(x, float64(i))
			y = append(y, float64(4+ri)+float64(i%5))
		}
	}
	return refract.NewTable().String("region", region).Float64("t", x).Float64("v", y)
}

func facetPlot() *refract.Plot {
	p := refract.New(
		refract.Size(760, 420),
		refract.Title("Throughput by region"),
		refract.XTitle("hour"),
		refract.YTitle("requests/s"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(geom.Line(facetSource(), geom.X("t"), geom.Y("v"), geom.Label("throughput")))
	p.Add(geom.HLine(7, geom.Label("target")))
	return p
}

func TestGoldenFacetWrap(t *testing.T) {
	golden(t, "facet-wrap", facetPlot().Facet(facet.Wrap("region", facet.Columns(2))))
}

func TestGoldenAnnotations(t *testing.T) {
	xs := ramp(0, 10, 60)
	p := refract.New(
		refract.Size(720, 400),
		refract.Title("Latency against its budget"),
		refract.XTitle("minute"),
		refract.YTitle("ms"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(geom.HBand(180, 220, geom.Label("tolerance")))
	p.Add(geom.VBand(3, 4, geom.Fill(palette.Orange), geom.Opacity(0.18), geom.Label("deploy")))
	p.Add(geom.Line(refract.Float64Columns(map[string][]float64{
		"t": xs,
		"y": apply(xs, func(v float64) float64 { return 200 + 40*sin(v) }),
	}), geom.X("t"), geom.Y("y"), geom.Color(palette.Blue), geom.Label("p99")))
	p.Add(geom.HLine(250, geom.Label("SLO")))
	p.Add(geom.Note(0.2, 244, "budget", geom.FontSize(11), geom.Align(ir.AlignStart, ir.AlignTop)))
	golden(t, "annotations", p)
}

func TestGoldenColorbar(t *testing.T) {
	src := refract.NewTable().
		String("region", []string{"north", "south", "east", "west"}).
		Float64("sales", []float64{18, 42, 31, 25})

	p := refract.New(refract.Size(620, 380), refract.Title("Sales by region"))
	p.X(scale.Ordinal())
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(geom.Bar(src, geom.X("region"), geom.Y("sales"),
		geom.ColorBy("sales", scale.Sequential(palette.Viridis))))
	golden(t, "colorbar", p)
}

func TestGoldenSubplotGrid(t *testing.T) {
	xs := ramp(0, 10, 40)
	panel := func(title string, fn func(float64) float64, c int) *refract.Plot {
		p := refract.New(refract.Title(title))
		p.X(scale.Linear(scale.Nice()))
		p.Y(scale.Linear(scale.Nice()))
		p.Add(geom.Line(refract.Float64Columns(map[string][]float64{
			"x": xs, "y": apply(xs, fn),
		}), geom.X("x"), geom.Y("y"), geom.Color(palette.OkabeIto.At(c)), geom.Label(title)))
		return p
	}
	g := refract.NewGrid(2,
		refract.GridSize(760, 460),
		refract.GridTheme(theme.Dark),
		refract.GridTitle("Fleet overview"),
		refract.GridAxisTitles("minute", "value"),
	)
	g.Add(
		panel("latency", func(v float64) float64 { return 50 + 20*sin(v) }, 0),
		panel("throughput", func(v float64) float64 { return 900 - 60*v }, 1),
		panel("errors", func(v float64) float64 { return 3 * sin(v/2) * sin(v/2) }, 2),
		panel("saturation", func(v float64) float64 { return 0.4 + 0.3*sin(v/3) }, 3),
	)

	var buf bytes.Buffer
	if err := g.Render(refract.SVGWriter(&buf, svg.Pretty())); err != nil {
		t.Fatalf("Render: %v", err)
	}
	goldenBytes(t, "subplots", buf.Bytes())
}

// --- behaviour -----------------------------------------------------------

func TestFacetDrawsOnePanelPerValue(t *testing.T) {
	got := renderString(t, facetPlot().Facet(facet.Wrap("region")))
	for _, want := range []string{"north", "south", "east"} {
		if !strings.Contains(got, ">"+want+"<") {
			t.Errorf("no panel labelled %q", want)
		}
	}
}

func TestFacetOnNilTurnsFacetingOff(t *testing.T) {
	p := facetPlot().Facet(facet.Wrap("region")).Facet(nil)
	if got := renderString(t, p); strings.Contains(got, ">north<") {
		t.Error("faceting was still applied after Facet(nil)")
	}
}

func TestFacetErrorsSurface(t *testing.T) {
	err := facetPlot().Facet(facet.Wrap("nope")).Render(refract.SVGWriter(&bytes.Buffer{}))
	if !errors.Is(err, facet.ErrNoColumn) {
		t.Errorf("err = %v, want ErrNoColumn", err)
	}
}

// A free axis needs a copy of the scale, and a scale that cannot be copied has
// to say so rather than quietly sharing one.
func TestAFreeAxisNeedsACloneableScale(t *testing.T) {
	p := facetPlot().Facet(facet.Wrap("region", facet.FreeY()))
	p.Y(uncloneable{scale.Linear()})
	err := p.Render(refract.SVGWriter(&bytes.Buffer{}))
	if err == nil || !strings.Contains(err.Error(), "Cloner") {
		t.Errorf("err = %v, want a complaint about scale.Cloner", err)
	}
}

type uncloneable struct{ scale.Scale }

func TestFreeScalesGiveEachPanelItsOwnDomain(t *testing.T) {
	shared := renderString(t, facetPlot().Facet(facet.Wrap("region")))
	free := renderString(t, facetPlot().Facet(facet.Wrap("region", facet.Free())))
	if shared == free {
		t.Error("free scales rendered identically to shared ones")
	}
}

func TestGridRendersEveryPlot(t *testing.T) {
	g := refract.NewGrid(2, refract.GridSize(600, 400))
	for _, name := range []string{"a", "b", "c"} {
		p := refract.New(refract.Title(name))
		p.Add(geom.Line(refract.Float64Columns(map[string][]float64{
			"x": {0, 1, 2}, "y": {0, 1, 0},
		}), geom.X("x"), geom.Y("y")))
		g.Add(p)
	}
	var buf bytes.Buffer
	if err := g.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c"} {
		if !strings.Contains(buf.String(), ">"+name+"<") {
			t.Errorf("panel %q is missing", name)
		}
	}
}

func TestGridAtPlacesAPlotAndLeavesHoles(t *testing.T) {
	p := refract.New(refract.Title("corner"))
	p.Add(geom.Line(refract.Float64Columns(map[string][]float64{
		"x": {0, 1}, "y": {0, 1},
	}), geom.X("x"), geom.Y("y")))

	g := refract.NewGrid(2, refract.GridSize(600, 400)).At(1, 1, p)
	var buf bytes.Buffer
	if err := g.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), ">corner<") {
		t.Error("the placed plot was not drawn")
	}
}

func TestGridAtReplacesACell(t *testing.T) {
	first := refract.New(refract.Title("first"))
	second := refract.New(refract.Title("second"))
	for _, p := range []*refract.Plot{first, second} {
		p.Add(geom.Line(refract.Float64Columns(map[string][]float64{
			"x": {0, 1}, "y": {0, 1},
		}), geom.X("x"), geom.Y("y")))
	}
	g := refract.NewGrid(1, refract.GridSize(400, 300)).Add(first).At(0, 0, second)
	var buf bytes.Buffer
	if err := g.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, ">first<") || !strings.Contains(got, ">second<") {
		t.Error("At did not replace the cell")
	}
}

func TestEmptyGridIsAnError(t *testing.T) {
	err := refract.NewGrid(2).Render(refract.SVGWriter(&bytes.Buffer{}))
	if !errors.Is(err, refract.ErrEmptyGrid) {
		t.Errorf("err = %v, want ErrEmptyGrid", err)
	}
	if err := refract.NewGrid(2).Render(nil); err == nil {
		t.Error("a nil target was accepted")
	}
}

// The grid's legend gathers every panel's series, so a reader does not have to
// find four legends in four corners.
func TestGridLegendGathersEveryPanel(t *testing.T) {
	g := refract.NewGrid(2, refract.GridSize(700, 500), refract.GridLegend(true))
	for _, name := range []string{"alpha", "beta"} {
		p := refract.New()
		p.Add(geom.Line(refract.Float64Columns(map[string][]float64{
			"x": {0, 1}, "y": {0, 1},
		}), geom.X("x"), geom.Y("y"), geom.Label(name)))
		g.Add(p)
	}
	var buf bytes.Buffer
	if err := g.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta"} {
		if !strings.Contains(buf.String(), ">"+name+"<") {
			t.Errorf("legend row %q is missing", name)
		}
	}
}

func TestPDFRendersTheSameChart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chart.pdf")
	if err := facetPlot().Render(refract.PDF(path, pdf.Title("Throughput"))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, []byte("%PDF-")) {
		t.Error("the file is not a PDF")
	}
	if len(got) < 500 {
		t.Errorf("the PDF is %d bytes, too small to hold a chart", len(got))
	}
}
