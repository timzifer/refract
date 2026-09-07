package refract_test

import (
	"math"
	"strings"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/spec"
)

// The Smith chart, end to end.
//
// It is the third coordinate system, and the point of the tests below is that
// it is only that: no mark knows it exists, render draws its grid with the same
// two loops it draws a Cartesian one with, and the layer plotted on it is a
// geom.Line that shipped in v0.1. See docs/adr/0033-smith-charts.md.

// smithGrid is the grid a paper Smith chart is printed with. The domains are
// pinned rather than trained: the chart's extent is the whole disc whatever the
// data does, and a near-open reflection is a resistance in the thousands that
// would otherwise drag every tick into the last pixel before the rim.
func smithGrid(p *refract.Plot) *refract.Plot {
	p.X(scale.Linear(scale.Domain(0, 50), scale.TickValues(0, 0.2, 0.5, 1, 2, 5)))
	p.Y(scale.Linear(scale.Domain(-50, 50),
		scale.TickValues(-5, -2, -1, -0.5, -0.2, 0.2, 0.5, 1, 2, 5)))
	return p
}

// seriesRL is the reflection of a series resistor and inductor, swept in
// frequency: the locus every antenna measurement starts as. It is given as an
// impedance, because that is what a Smith panel's two columns hold.
func seriesRL(r float64, n int) refract.Source {
	rs, xs := make([]float64, n), make([]float64, n)
	for i := range n {
		rs[i] = r
		xs[i] = -2 + 4*float64(i)/float64(n-1)
	}
	return refract.Float64Columns(map[string][]float64{"r": rs, "x": xs})
}

func smithPlot(opts ...refract.Option) *refract.Plot {
	p := refract.New(append([]refract.Option{
		refract.Size(480, 480),
		refract.Coord(coord.Smith()),
		refract.Legend(false),
	}, opts...)...)
	return smithGrid(p)
}

// The whole claim of the milestone: the same layer, in a different coordinate
// system, is a different chart — and every point of it lands inside the disc.
func TestASmithChartIsALineInADifferentCoordinateSystem(t *testing.T) {
	src := seriesRL(0.5, 40)

	cartesian := refract.New(refract.Size(480, 480), refract.Legend(false))
	cartesian.X(scale.Linear()).Y(scale.Linear())
	cartesian.Add(geom.Line(src, geom.X("r"), geom.Y("x")))

	smith := smithPlot()
	smith.Add(geom.Line(src, geom.X("r"), geom.Y("x")))

	var flat, curved strings.Builder
	if err := cartesian.Render(refract.SVGWriter(&flat)); err != nil {
		t.Fatal(err)
	}
	if err := smith.Render(refract.SVGWriter(&curved)); err != nil {
		t.Fatal(err)
	}
	if flat.String() == curved.String() {
		t.Fatal("the coord changed nothing")
	}
}

// Every mark a Smith panel draws is inside the disc, because every passive
// impedance is. The check is against the panel the render announced rather than
// against a rectangle guessed from the canvas.
func TestEveryMarkOfASmithChartIsInsideTheDisc(t *testing.T) {
	src := seriesRL(0.5, 40)
	p := smithPlot()
	p.Add(geom.Scatter(src, geom.X("r"), geom.Y("x")))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	pn := live.Index().Panels()[0]
	centre := pn.Coord.Point(pn.X.Map(1), pn.Y.Map(0))
	rim := pn.Coord.Point(pn.X.Map(0), pn.Y.Map(0))
	radius := float64(centre.X - rim.X)
	if radius <= 0 {
		t.Fatalf("the disc has radius %v", radius)
	}
	for i := range 40 {
		at := pn.Coord.Point(pn.X.Map(0.5), pn.Y.Map(-2+4*float64(i)/39))
		d := math.Hypot(float64(at.X-centre.X), float64(at.Y-centre.Y))
		if d > radius+0.5 {
			t.Fatalf("sample %d sits %v from the centre, outside a disc of %v", i, d, radius)
		}
	}
}

// A hit reports the impedance, not a pixel and not a reflection coefficient.
// It gets there through the coord and then through the scales — and under this
// coord the second step is the identity, which is exactly why the first one has
// to be honest about what the interval means.
func TestAHitOnASmithChartNamesTheImpedance(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{
		"r": {0.5, 1, 2},
		"x": {-1, 0, 1.5},
	})
	p := smithPlot(refract.Theme(bare()))
	p.Add(geom.Scatter(src, geom.X("r"), geom.Y("x")))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()
	pn := ix.Panels()[0]
	for _, want := range [][2]float64{{0.5, -1}, {1, 0}, {2, 1.5}} {
		at := pn.Coord.Point(pn.X.Map(want[0]), pn.Y.Map(want[1]))
		h, ok := ix.At(at, 4)
		if !ok {
			t.Fatalf("nothing under the pointer at z = %v%+vj", want[0], want[1])
		}
		if math.Abs(h.X-want[0]) > 0.02 || math.Abs(h.Y-want[1]) > 0.02 {
			t.Errorf("the hit reports z = %.3f%+.3fj, want %v%+vj", h.X, h.Y, want[0], want[1])
		}
	}
}

// The grid is circles and arcs, and it is drawn by render's own two loops from
// the panel's own two tick lists — one curve per tick, with the tick's own
// label. That is the property that kept render out of this change.
func TestSmithFurnitureIsCirclesAndArcs(t *testing.T) {
	p := smithPlot()
	p.Add(geom.Line(seriesRL(0.5, 8), geom.X("r"), geom.Y("x")))

	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}
	// Five resistance circles (r = 0 is the rim, which the reactance axis
	// draws), ten reactance arcs, and the rim itself. Every one of them reaches
	// the backend as a stroked path, because every one of them is cubics — a
	// Cartesian grid line reaches it as a two-point Polyline, and the real axis
	// still does.
	if curves, want := rec.Count("StrokePath"), 16; curves < want {
		t.Errorf("the panel stroked %d curves, want at least %d — the grid is missing", curves, want)
	}
	joined := strings.Join(rec.Texts(), " ")
	for _, want := range []string{"0.2", "0.5", "1.0", "2.0", "5.0", "-1.0"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the grid label %q was not written; the canvas has %q", want, joined)
		}
	}
}

// A Smith chart written down and read back is the same chart, coord and pinned
// grid and all.
func TestASmithChartSurvivesTheSpecRoundTrip(t *testing.T) {
	p := smithPlot()
	p.Add(geom.Line(seriesRL(0.5, 12), geom.X("r"), geom.Y("x")))

	s, err := p.Spec()
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := spec.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := back.Chart(); err != nil {
		t.Fatalf("the document did not read back: %v\n%s", err, b)
	}
	if !strings.Contains(string(b), `"type": "smith"`) {
		t.Errorf("the document does not name the coord:\n%s", b)
	}
}

// The admittance chart is the same disc turned through half a turn, and its two
// columns are an admittance rather than an impedance. The two changes cancel,
// so one load lands in one place whichever chart it is read on — which is the
// property that makes the Y chart a second reading rather than a second
// measurement.
func TestOneLoadLandsInOnePlaceOnEitherChart(t *testing.T) {
	src := seriesRL(0.5, 8)
	z := smithPlot()
	z.Add(geom.Scatter(src, geom.X("r"), geom.Y("x")))
	y := smithGrid(refract.New(refract.Size(480, 480),
		refract.Coord(coord.Smith(coord.SmithAdmittance(true))), refract.Legend(false)))
	y.Add(geom.Scatter(src, geom.X("r"), geom.Y("x")))

	// z = 0.5 − 1j, and the admittance that is the same load.
	const r, x = 0.5, -1.0
	d := r*r + x*x
	on, over := placedAt(t, z, r, x), placedAt(t, y, r/d, -x/d)
	if math.Abs(float64(on.X-over.X)) > 0.5 || math.Abs(float64(on.Y-over.Y)) > 0.5 {
		t.Errorf("the load is at %v on the Z chart and at %v on the Y chart", on, over)
	}
	// And the picture itself really is the half turn.
	mid := placedAt(t, z, 1, 0)
	mirror := placedAt(t, y, r, x)
	if math.Abs(float64(on.X+mirror.X-2*mid.X)) > 0.5 || math.Abs(float64(on.Y+mirror.Y-2*mid.Y)) > 0.5 {
		t.Errorf("the admittance placement %v is not the half turn of %v about %v", mirror, on, mid)
	}
}

// placedAt renders p and reports where the panel puts one impedance.
func placedAt(t *testing.T, p *refract.Plot, r, x float64) ir.Point {
	t.Helper()
	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	pn := live.Index().Panels()[0]
	return pn.Coord.Point(pn.X.Map(r), pn.Y.Map(x))
}

func TestGoldenSmithChart(t *testing.T) {
	p := smithPlot(
		refract.Size(560, 520),
		refract.Title("A series RL load, swept"),
	)
	p.Add(geom.Line(seriesRL(0.5, 60), geom.X("r"), geom.Y("x")))
	p.Add(geom.Scatter(seriesRL(0.5, 5), geom.X("r"), geom.Y("x")))
	golden(t, "smith", p)
}
