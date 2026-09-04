package refract_test

import (
	"bytes"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/svgdiff"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

// The v0.4 milestone, end to end: a chart over more rows than the output has
// pixels comes out the size a chart should be, and it comes out the same
// however many goroutines drew it.

// noisySignal is a million-row series with a spike in it — the shape big-data
// decimation exists for, and the one that catches a reduction that flattens.
func noisySignal(n int) refract.Source {
	r := rand.New(rand.NewPCG(7, 11))
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = float64(i)
		y[i] = math.Sin(float64(i)/float64(n)*20) + 0.05*r.NormFloat64()
	}
	y[n/3] = 4
	return refract.Float64Columns(map[string][]float64{"x": x, "y": y})
}

func TestABigSeriesRendersAsASmallFile(t *testing.T) {
	const rows = 1_000_000
	src := noisySignal(rows)

	render := func(opts ...geom.Option) int {
		t.Helper()
		p := refract.New(refract.Size(800, 500), refract.Title("A million rows"))
		p.Add(geom.Line(src, append([]geom.Option{geom.X("x"), geom.Y("y")}, opts...)...))
		var buf bytes.Buffer
		if err := p.Render(refract.SVGWriter(&buf)); err != nil {
			t.Fatalf("Render: %v", err)
		}
		return buf.Len()
	}

	reduced := render()
	whole := render(geom.Decimate(geom.NoDecimation))

	if reduced >= whole/10 {
		t.Errorf("the reduced chart is %d bytes against %d for every row: "+
			"a 800px-wide plot should be an order of magnitude smaller", reduced, whole)
	}
	// A megabyte of SVG for an 800-pixel chart is the failure this milestone is
	// about; the reduced one has to be small enough to serve.
	if reduced > 200_000 {
		t.Errorf("the reduced chart is %d bytes, want something a web page can carry", reduced)
	}
}

// A reduction that dropped the spike would be a reduction that hid the answer.
func TestASpikeSurvivesTheReduction(t *testing.T) {
	src := noisySignal(500_000)
	p := refract.New(refract.Size(800, 500))
	p.Y(scale.Linear())
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y"), geom.Decimate(geom.MinMax)))

	var buf bytes.Buffer
	if err := p.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	// The spike is the domain maximum, so the y axis is labelled to reach it.
	if !bytes.Contains(buf.Bytes(), []byte(">4<")) {
		t.Error("the axis does not reach 4; the spike was reduced away or the domain was taken from the subset")
	}
}

func TestGoldenDecimatedSignal(t *testing.T) {
	p := refract.New(
		refract.Size(640, 400),
		refract.Title("Decimated"),
		refract.XTitle("sample"),
		refract.YTitle("value"),
	)
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(noisySignal(250_000), geom.X("x"), geom.Y("y"), geom.Color(palette.Blue)))
	golden(t, "decimated", p)
}

// The density raster is checked structurally rather than against a golden SVG.
// Its pixels reach the file as a base64 PNG, and a golden file over that would
// be pinning the standard library's deflate output rather than anything
// refract decides. What refract decides is that there is one image, covering
// the plot area, and no markers — and that is what this asserts. The pixels
// themselves are covered in geom and stat, where they are pixels rather than
// an encoding of them.
func TestACrowdedScatterRendersAsAnImage(t *testing.T) {
	p := refract.New(
		refract.Size(640, 400),
		refract.Title("Density"),
		refract.XTitle("x"),
		refract.YTitle("y"),
	)
	p.Add(geom.Scatter(gaussianCloud(200_000), geom.X("x"), geom.Y("y"), geom.Color(palette.Blue)))

	var buf bytes.Buffer
	if err := p.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.Bytes()
	if n := bytes.Count(out, []byte("<image ")); n != 1 {
		t.Fatalf("the chart holds %d images, want exactly one density raster", n)
	}
	if bytes.Contains(out, []byte("<use ")) {
		t.Error("the chart drew markers as well as a raster")
	}
	if !bytes.Contains(out, []byte(`data:image/png;base64,`)) {
		t.Error("the raster was not embedded in the document")
	}
	// Two hundred thousand markers would run to megabytes; the raster is the
	// whole point.
	if len(out) > 400_000 {
		t.Errorf("the chart is %d bytes, want a raster rather than a heap of markers", len(out))
	}
}

func gaussianCloud(n int) refract.Source {
	r := rand.New(rand.NewPCG(3, 5))
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		a, b := r.NormFloat64(), r.NormFloat64()
		x[i] = a
		y[i] = 0.6*a + 0.8*b
	}
	return refract.Float64Columns(map[string][]float64{"x": x, "y": y})
}

// The property that lets one set of golden files cover both paths.
func TestAParallelRenderIsByteForByteTheSerialOne(t *testing.T) {
	build := func(parallel bool) []byte {
		t.Helper()
		p := facetedSignal(9, 20_000)
		if !parallel {
			p = facetedSignalWith(9, 20_000, false)
		}
		var buf bytes.Buffer
		if err := p.Render(refract.SVGWriter(&buf)); err != nil {
			t.Fatalf("Render: %v", err)
		}
		return buf.Bytes()
	}
	par, ser := build(true), build(false)
	if !bytes.Equal(par, ser) {
		if ok, why := svgdiff.Equal(par, ser, svgdiff.DefaultTolerance); !ok {
			t.Fatalf("the parallel and serial renders differ: %s", why)
		}
		t.Error("the parallel and serial renders differ in their numbers; they must be identical, not merely equivalent")
	}
}

// A grid of different plots is the other multi-panel shape, and it goes down
// the same path.
func TestAGridRendersInParallel(t *testing.T) {
	g := refract.NewGrid(2, refract.GridSize(800, 600), refract.GridTitle("Fleet"))
	for range 4 {
		p := refract.New(refract.Title("panel"))
		p.Add(geom.Line(noisySignal(50_000), geom.X("x"), geom.Y("y")))
		g.Add(p)
	}
	var par, ser bytes.Buffer
	if err := g.Render(refract.SVGWriter(&par)); err != nil {
		t.Fatalf("Render: %v", err)
	}

	h := refract.NewGrid(2, refract.GridSize(800, 600), refract.GridTitle("Fleet"), refract.GridParallel(false))
	for range 4 {
		p := refract.New(refract.Title("panel"))
		p.Add(geom.Line(noisySignal(50_000), geom.X("x"), geom.Y("y")))
		h.Add(p)
	}
	if err := h.Render(refract.SVGWriter(&ser)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Equal(par.Bytes(), ser.Bytes()) {
		t.Error("a grid drawn in parallel differs from the same grid drawn serially")
	}
}
