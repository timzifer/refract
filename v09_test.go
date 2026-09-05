package refract_test

// The v0.9 milestone, end to end: every stat is a pure function of its input
// under test (in package stat); violin and ridgeline compose with v0.7's
// groups; a bubble chart's size key sits beside a legend and a colourbar
// without overlap, and doubling a value multiplies the diameter by the square
// root of two.

import (
	"math"
	"strings"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

// nations is the bubble chart's table: four columns, three of them channels.
func nations() *refract.Plot {
	src := refract.NewTable().
		Float64("income", []float64{1500, 4200, 12800, 31000, 46000, 58000}).
		Float64("years", []float64{58, 66, 72, 78, 81, 82}).
		Float64("people", []float64{212, 1420, 274, 51, 68, 335}).
		Float64("co2", []float64{0.3, 7.4, 2.1, 5.6, 8.9, 14.7}).
		String("region", []string{"Africa", "Asia", "Asia", "Europe", "Europe", "Americas"})

	p := refract.New(refract.Size(900, 560), refract.Title("Nations"), refract.Legend(true))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(
		// A line layer, so the chart has something to put in a legend beside
		// the two scales' own guides.
		geom.Line(src, geom.X("income"), geom.Y("years"), geom.Label("trend")),
		// No Label: a layer that names itself names *both* of its guides, and a
		// colourbar and a size key titled the same thing say nothing. Left
		// alone each guide is titled by the column it reads.
		geom.Scatter(src,
			geom.X("income"), geom.Y("years"),
			geom.SizeBy("people", scale.Size()),
			geom.ColorBy("co2", scale.Sequential(palette.Viridis))),
	)
	return p
}

// The definition of done, in one test: three guides in one column, none of them
// on top of another, all of them beside the plot rather than in it.
func TestASizeKeySitsBesideALegendAndAColourbar(t *testing.T) {
	rec := irtest.New()
	if err := nations().Render(rec.Target()); err != nil {
		t.Fatal(err)
	}

	// The plot area is what the layers were clipped to.
	var plot ir.Rect
	for _, c := range rec.Calls {
		if c.Op == "Push" && c.HasClip {
			plot = c.ClipRect
			break
		}
	}
	if plot.Empty() {
		t.Fatal("nothing was clipped to a plot area")
	}

	legend := textAt(t, rec, "trend")
	title := textAt(t, rec, "people")
	var bar ir.Rect
	for _, c := range rec.Filter("FillPath") {
		if c.Fill.IsGradient() {
			bar = c.Path.Bounds()
		}
	}
	if bar.Empty() {
		t.Fatal("the colourbar was not drawn")
	}

	for name, x := range map[string]float32{"legend": legend.X, "colourbar": bar.Min.X, "size key": title.X} {
		if x < plot.Max.X {
			t.Errorf("the %s starts at x=%v and the plot ends at x=%v: the guide is inside the panel",
				name, x, plot.Max.X)
		}
	}
	// Stacked in collection order — legend, colourbar, size key — and each
	// clear of the one before it.
	if !(legend.Y < bar.Min.Y && bar.Max.Y < title.Y) {
		t.Errorf("the guides are at y=%v (legend), %v..%v (colourbar) and %v (size key); they overlap",
			legend.Y, bar.Min.Y, bar.Max.Y, title.Y)
	}
}

// Doubling a value multiplies the diameter by √2, measured on the ink a real
// render emitted rather than on the scale alone.
func TestDoublingAValueMultipliesABubblesDiameterByRootTwo(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{
		"x": {1, 2, 3},
		"y": {1, 1, 1},
		"n": {25, 50, 100},
	})
	p := refract.New(refract.Size(600, 400), refract.Legend(false))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.SizeBy("n", scale.Size())))

	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}

	// The cloud is the one filled path holding several circular subpaths; the
	// key's samples are one circle per call.
	var widths []float32
	for _, c := range rec.Filter("FillPath") {
		if c.Fill.IsGradient() {
			continue
		}
		if ws := circleWidths(c.Path); len(ws) > len(widths) {
			widths = ws
		}
	}
	if len(widths) != 3 {
		t.Fatalf("got %d bubbles, want three", len(widths))
	}
	// Drawn largest first, so the list descends: 100, 50, 25.
	for i := 1; i < len(widths); i++ {
		ratio := float64(widths[i-1] / widths[i])
		if math.Abs(ratio-math.Sqrt2) > 0.02 {
			t.Errorf("bubbles %v and %v across are in the ratio %v, want √2",
				widths[i-1], widths[i], ratio)
		}
	}
}

// A pointer over a bubble names its row. That is not free: it is what one
// subpath per mark buys, and it is why a sized layer draws shapes rather than
// markers.
func TestAPointerOverABubbleNamesItsRow(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{
		"x": {1, 2, 3},
		"y": {1, 2, 3},
		"n": {10, 40, 90},
	})
	p := refract.New(refract.Size(600, 400), refract.Legend(false))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.SizeBy("n", scale.Size())))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()
	pan := ix.Panels()[0]
	for row, v := range [][2]float64{{1, 1}, {2, 2}, {3, 3}} {
		at := ir.Point{X: pan.X.Map(v[0]), Y: pan.Y.Map(v[1])}
		h, ok := ix.At(at, 2)
		if !ok {
			t.Errorf("row %d: nothing under the pointer at %v", row, at)
			continue
		}
		if h.Row != row {
			t.Errorf("the pointer over row %d names row %d", row, h.Row)
		}
	}
}

// The distribution marks compose with v0.7's groups and with v0.3's facets: a
// violin per series per slot, in every panel, from one layer.
func TestAViolinComposesWithGroupsAndFacets(t *testing.T) {
	tbl := refract.NewTable()
	var vals []float64
	var svc, region, quarter []string
	for i := range 480 {
		vals = append(vals, 20+float64(i%23)+float64(i%5)*2)
		svc = append(svc, []string{"auth", "search"}[i%2])
		region = append(region, []string{"eu", "us"}[(i/2)%2])
		quarter = append(quarter, []string{"Q1", "Q2"}[(i/4)%2])
	}
	tbl.Float64("ms", vals).String("service", svc).String("region", region).String("q", quarter)

	p := refract.New(refract.Size(900, 400), refract.Legend(true))
	p.X(scale.Ordinal())
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Violin(tbl, geom.X("service"), geom.Y("ms"), geom.GroupBy("region")))
	p.Facet(facet.Wrap("q"))

	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}
	// Two panels × two services × two regions.
	shapes := 0
	for _, c := range rec.Filter("FillPath") {
		if !c.Fill.IsGradient() && len(c.Path.Ops) > 8 {
			shapes++
		}
	}
	if shapes != 8 {
		t.Errorf("got %d violins, want two panels of two services by two regions", shapes)
	}
	// The legend names the regions once for the whole grid rather than once per
	// panel.
	for _, want := range []string{"eu", "us"} {
		if n := textCount(rec, want); n != 1 {
			t.Errorf("the legend names %q %d times, want once", want, n)
		}
	}
}

// A ridgeline is the same estimate down a categorical axis, and it says so
// rather than drawing nonsense when the axis cannot name rows.
func TestARidgelineNeedsACategoricalAxis(t *testing.T) {
	tbl := refract.NewTable()
	var vals []float64
	var month []string
	for i := range 300 {
		vals = append(vals, float64(i%37)+float64(i/100)*5)
		month = append(month, []string{"jan", "feb", "mar"}[i/100])
	}
	tbl.Float64("v", vals).String("m", month)

	ok := refract.New(refract.Size(600, 400), refract.Legend(false))
	ok.X(scale.Linear(scale.Nice()))
	ok.Y(scale.Ordinal())
	ok.Add(geom.Ridgeline(tbl, geom.X("v"), geom.Y("m")))
	if err := ok.Render(irtest.New().Target()); err != nil {
		t.Fatalf("a ridgeline over an ordinal axis: %v", err)
	}

	bad := refract.New(refract.Size(600, 400), refract.Legend(false))
	bad.X(scale.Linear())
	bad.Y(scale.Linear())
	bad.Add(geom.Ridgeline(tbl, geom.X("v"), geom.Y("m")))
	err := bad.Render(irtest.New().Target())
	if err == nil {
		t.Fatal("a ridgeline drew itself onto a continuous axis")
	}
	if !strings.Contains(err.Error(), "scale.Ordinal") {
		t.Errorf("error = %v, want it to name the scale the mark needs", err)
	}
}

// ADR 0012 again, for the seven new marks: a parallel render has to be
// byte-identical to a serial one. It is what lets one set of golden files cover
// both paths, and every one of these marks has somewhere it could have gone
// wrong — a map iteration order in a hex lattice, a sort in a swarm or a bubble
// cloud, a group order in a violin.
func TestEveryDistributionMarkDrawsTheSameInParallelAndSerial(t *testing.T) {
	tbl := refract.NewTable()
	var v, w []float64
	var cat, panel []string
	for i := range 600 {
		v = append(v, float64(i%29)+float64(i%7))
		w = append(w, float64((i*13)%41))
		cat = append(cat, []string{"a", "b", "c"}[i%3])
		panel = append(panel, []string{"p", "q", "r"}[(i/7)%3])
	}
	tbl.Float64("v", v).Float64("w", w).String("cat", cat).String("p", panel)

	cases := []struct {
		name  string
		layer func() geom.Geom
		x, y  func() scale.Scale
	}{
		{"histogram", func() geom.Geom { return geom.Histogram(tbl, geom.X("v")) }, linear, linear},
		{"ecdf", func() geom.Geom { return geom.ECDF(tbl, geom.X("v"), geom.GroupBy("cat")) }, linear, linear},
		{"hexbin", func() geom.Geom { return geom.Hexbin(tbl, geom.X("v"), geom.Y("w")) }, linear, linear},
		{"trend", func() geom.Geom { return geom.Trend(tbl, geom.X("v"), geom.Y("w")) }, linear, linear},
		{"violin", func() geom.Geom {
			return geom.Violin(tbl, geom.X("cat"), geom.Y("v"), geom.GroupBy("cat"))
		}, ordinal, linear},
		{"ridgeline", func() geom.Geom { return geom.Ridgeline(tbl, geom.X("v"), geom.Y("cat")) }, linear, ordinal},
		{"beeswarm", func() geom.Geom { return geom.Beeswarm(tbl, geom.X("cat"), geom.Y("v")) }, ordinal, linear},
		{"bubbles", func() geom.Geom {
			return geom.Scatter(tbl, geom.X("v"), geom.Y("w"), geom.SizeBy("w", scale.Size()))
		}, linear, linear},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trace := func(parallel bool) []string {
				p := refract.New(refract.Size(700, 450), refract.Parallel(parallel))
				p.X(tc.x())
				p.Y(tc.y())
				p.Add(tc.layer())
				p.Facet(facet.Wrap("p"))
				rec := irtest.New()
				if err := p.Render(rec.Target()); err != nil {
					t.Fatal(err)
				}
				return rec.Trace()
			}
			a, b := trace(true), trace(false)
			if len(a) != len(b) {
				t.Fatalf("%d calls in parallel, %d serial", len(a), len(b))
			}
			for i := range a {
				if a[i] != b[i] {
					t.Fatalf("call %d differs\n parallel: %s\n   serial: %s", i, a[i], b[i])
				}
			}
		})
	}
}

func linear() scale.Scale  { return scale.Linear(scale.Nice()) }
func ordinal() scale.Scale { return scale.Ordinal() }

// The whole milestone through the SVG emitter, which is what a caller actually
// gets: the marks are there, and so are the guides.
func TestTheDistributionChartsRenderToSVG(t *testing.T) {
	var b strings.Builder
	if err := nations().Render(refract.SVGWriter(&b)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<svg", "Nations", "linearGradient", "people", "co2", "trend"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("the document is missing %q", want)
		}
	}
}

// textAt returns where a run of text was drawn.
func textAt(t *testing.T, rec *irtest.Recorder, s string) ir.Point {
	t.Helper()
	for _, c := range rec.Filter("Text") {
		if c.Text.Text == s {
			return c.Text.At
		}
	}
	t.Fatalf("%q was never drawn", s)
	return ir.Point{}
}

func textCount(rec *irtest.Recorder, s string) int {
	n := 0
	for _, c := range rec.Filter("Text") {
		if c.Text.Text == s {
			n++
		}
	}
	return n
}

// circleWidths measures each cubic subpath of a path — one bubble each.
func circleWidths(p *ir.Path) []float32 {
	var out []float32
	var lo, hi float32
	open, curved := false, false
	flush := func() {
		if open && curved {
			out = append(out, hi-lo)
		}
		open, curved = false, false
	}
	p.Walk(func(op ir.PathOp, pts []ir.Point) {
		if op == ir.OpMoveTo {
			flush()
			lo, hi, open = pts[0].X, pts[0].X, true
		}
		if op == ir.OpCubicTo {
			curved = true
		}
		for _, q := range pts {
			lo, hi = min(lo, q.X), max(hi, q.X)
		}
	})
	flush()
	return out
}
