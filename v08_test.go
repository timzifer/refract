package refract_test

// The v0.8 milestone, end to end: the same geom.Bar layer draws a bar chart in
// coord.Cartesian and a pie in coord.Polar; every existing golden file is
// unchanged; a pointer over a slice names its category and its row rather than
// a pixel; a donut's hole is an explicit annulus; and the spec round-trips the
// coord.

import (
	"math"
	"strings"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// browsers is the table every pie in this file is drawn from: one row per
// slice, one constant X so that every slice fills the radius, and a category
// column that becomes the series.
func browsers() refract.Source {
	return refract.NewTable().
		Float64("all", []float64{0, 0, 0, 0}).
		Float64("share", []float64{45, 25, 20, 10}).
		String("browser", []string{"chrome", "safari", "firefox", "edge"})
}

// bare is the theme a pie wants: no grid, no axis lines and no ticks. A pie
// has no radial quantity to label and no angular one either — the slices are
// the reading.
func bare() theme.Theme {
	return theme.Light.With(
		theme.Grid(false, false),
		theme.AxisLines(false, false),
		theme.Ticks(false, false),
	)
}

func pie(opts ...coord.Option) *refract.Plot {
	p := refract.New(refract.Size(400, 400), refract.Theme(bare()),
		refract.Coord(coord.Polar(append([]coord.Option{coord.Theta(coord.FromY)}, opts...)...)),
		refract.Legend(false))
	// Neither scale is niced. A pie's ring closes because the stacked Y domain
	// ends at the total; a domain rounded up to the next round number would
	// leave a wedge of nothing at twelve o'clock.
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Bar(browsers(), geom.X("all"), geom.Y("share"), geom.GroupBy("browser")))
	return p
}

// The milestone in one test: one layer, two coords, two charts.
func TestOneBarLayerDrawsABarChartAndAPie(t *testing.T) {
	layer := func() geom.Geom {
		return geom.Bar(browsers(), geom.X("all"), geom.Y("share"), geom.GroupBy("browser"))
	}

	cart := refract.New(refract.Size(400, 400), refract.Legend(false))
	cart.X(scale.Linear())
	cart.Y(scale.Linear())
	cart.Add(layer())

	polar := refract.New(refract.Size(400, 400), refract.Theme(bare()), refract.Legend(false),
		refract.Coord(coord.Polar(coord.Theta(coord.FromY))))
	polar.X(scale.Linear())
	polar.Y(scale.Linear())
	polar.Add(layer())

	bars, slices := irtest.New(), irtest.New()
	if err := cart.Render(bars.Target()); err != nil {
		t.Fatal(err)
	}
	if err := polar.Render(slices.Target()); err != nil {
		t.Fatal(err)
	}

	// Four segments either way — the layer did not change, only what the pair
	// of mapped positions means. The count is of the fills inside the panel's
	// clip: the canvas background is a fill too, and it is not a mark.
	if got := marks(bars); got != 4 {
		t.Errorf("the bar chart drew %d fills, want one per series", got)
	}
	if got := marks(slices); got != 4 {
		t.Errorf("the pie drew %d fills, want one per slice", got)
	}
	// A bar is four corners and a slice is an arc, so the pie is cubics where
	// the bar chart is lines. The IR did not change to carry them: ADR 0002
	// froze the verb set on the claim that every curve a chart needs is
	// cubics, and this is the first chart that tests it.
	if cubics(bars) != 0 {
		t.Errorf("a Cartesian bar chart drew %d cubic segments, want none", cubics(bars))
	}
	if cubics(slices) == 0 {
		t.Error("the pie drew no cubics; a slice's arc has to be one")
	}
}

// A pie's ring closes. The last slice ends exactly where the first began,
// because the stacking adjustment accumulates a running total and the coord
// puts equal angles in equal places.
func TestAPiesRingCloses(t *testing.T) {
	rec := irtest.New()
	if err := pie().Render(rec.Target()); err != nil {
		t.Fatal(err)
	}
	paths := clipped(rec, "FillPath")
	if len(paths) != 4 {
		t.Fatalf("%d slices, want 4", len(paths))
	}
	first, last := paths[0], paths[len(paths)-1]
	// Each wedge runs start-of-arc, arc, centre, close — so the last wedge's
	// arc ends at the point the first wedge's arc starts from.
	arcEnd := last.Pts[len(last.Pts)-2]
	if d := dist(arcEnd, first.Pts[0]); d > 0.05 {
		t.Errorf("the ring has a seam %v wide: %v against %v", d, arcEnd, first.Pts[0])
	}
}

// A donut's hole is an annulus rather than a circle of background painted over
// the middle: the radial scale starts at the inner radius, so no mark reaches
// the hole and a pointer inside it hits nothing.
func TestADonutsHoleIsAnAnnulus(t *testing.T) {
	p := pie(coord.Hole(0.5))
	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	area := live.Index().Panels()[0].Area
	cx, cy := (area.Min.X+area.Max.X)/2, (area.Min.Y+area.Max.Y)/2

	for _, path := range clipped(rec, "FillPath") {
		for _, q := range path.Pts {
			if d := dist(q, ir.Point{X: cx, Y: cy}); d < 1 {
				t.Fatalf("a slice reaches the middle of the donut at %v", q)
			}
		}
	}
	if _, ok := live.Index().At(ir.Point{X: cx, Y: cy}, 2); ok {
		t.Error("a pointer in the hole hit a slice")
	}
}

// A pointer over a slice names the row behind it, which is what names the
// category: the slice is one row of the table, and the row is what a tooltip
// looks the browser's name up by.
func TestAPointerOverASliceNamesItsRow(t *testing.T) {
	rec := irtest.New()
	live, err := pie().Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()
	area := ix.Panels()[0].Area
	cx, cy := (area.Min.X+area.Max.X)/2, (area.Min.Y+area.Max.Y)/2

	// 45, 25, 20 and 10 per cent of a turn, clockwise from noon; probe the
	// middle of each slice, halfway out.
	names := []string{"chrome", "safari", "firefox", "edge"}
	for row, deg := range []float64{81, 207, 288, 342} {
		rad := deg * math.Pi / 180
		r := float64(area.Dx()) / 4
		at := ir.Point{
			X: cx + float32(r*math.Sin(rad)),
			Y: cy - float32(r*math.Cos(rad)),
		}
		h, ok := ix.At(at, 4)
		if !ok {
			t.Errorf("%s: nothing under the pointer at %.0f degrees", names[row], deg)
			continue
		}
		if h.Row != row {
			t.Errorf("%s: the pointer at %.0f degrees names row %d, want %d",
				names[row], deg, h.Row, row)
		}
	}
}

// The values a hit reports come back through the coord and then through the
// scales. Reading the device position straight through the scales — which is
// what happened before there was a coord — reports a pixel dressed as a value.
func TestAHitInvertsThroughTheCoord(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{
		"angle": {0, 1, 2, 3},
		"r":     {1, 2, 3, 4},
	})
	p := refract.New(refract.Size(400, 400), refract.Theme(bare()),
		refract.Coord(coord.Polar()), refract.Legend(false))
	p.X(scale.Linear(scale.Domain(0, 4)))
	p.Y(scale.Linear(scale.Domain(0, 4)))
	p.Add(geom.Scatter(src, geom.X("angle"), geom.Y("r")))

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
	// The row at angle 1 of 4 — a quarter turn — and radius 2 of 4.
	at := pn.Coord.Point(pn.X.Map(1), pn.Y.Map(2))
	h, ok := ix.At(at, 4)
	if !ok {
		t.Fatal("nothing under the pointer")
	}
	if math.Abs(h.X-1) > 0.05 || math.Abs(h.Y-2) > 0.05 {
		t.Errorf("the hit reports (%.3f, %.3f), want the values (1, 2)", h.X, h.Y)
	}
}

// A radar is a line over an ordinal angular axis, and two things make it one:
// its edges are chords rather than arcs, and its contour closes back to the
// first axis.
func TestARadarIsALineWithChordsThatCloses(t *testing.T) {
	src := refract.NewTable().
		String("axis", []string{"speed", "power", "range", "cost", "weight"}).
		Float64("v", []float64{7, 9, 4, 6, 8})

	p := refract.New(refract.Size(400, 400), refract.Coord(coord.Polar(coord.Chord())),
		refract.Legend(false))
	p.X(scale.Ordinal(scale.OrdinalPadding(0)))
	p.Y(scale.Linear(scale.Domain(0, 10)))
	p.Add(geom.Line(src, geom.X("axis"), geom.Y("v"), geom.Closed(true)))

	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}
	var contour *ir.Path
	for _, path := range pathsOf(rec, "StrokePath") {
		if len(path.Pts) == 6 {
			contour = path
			break
		}
	}
	if contour == nil {
		t.Fatalf("no five-sided contour was drawn:\n%s", strings.Join(rec.Trace(), "\n"))
	}
	if curved(contour) {
		t.Error("a chord contour has a cubic in it; a radar's sides are straight")
	}
	if contour.Ops[len(contour.Ops)-1] != ir.OpClose {
		t.Error("the contour does not close back to the first axis")
	}
	if d := dist(contour.Pts[0], contour.Pts[len(contour.Pts)-1]); d > 0.05 {
		t.Errorf("the closing edge misses the first vertex by %v", d)
	}
}

// Polar furniture is concentric rings where a Cartesian panel has horizontal
// grid lines, and labels round the ring where it has a row of them along an
// edge. The coord reports the geometry; render still strokes it, in the order
// it always did.
func TestPolarFurnitureIsRingsAndLabelsRoundTheRing(t *testing.T) {
	src := refract.NewTable().
		String("axis", []string{"n", "e", "s", "w"}).
		Float64("v", []float64{3, 6, 2, 8})

	p := refract.New(refract.Size(400, 400), refract.Coord(coord.Polar()), refract.Legend(false))
	p.X(scale.Ordinal(scale.OrdinalPadding(0)))
	p.Y(scale.Linear(scale.Domain(0, 10)))
	p.Add(geom.Line(src, geom.X("axis"), geom.Y("v"), geom.Closed(true)))

	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}
	// Every one of the four compass points is labelled: labels round a ring
	// share no row, so the collision filter that thins a dense Cartesian axis
	// must not thin them.
	texts := strings.Join(rec.Texts(), " ")
	for _, name := range []string{"n", "e", "s", "w"} {
		if !strings.Contains(texts, name) {
			t.Errorf("the ring is not labelled %q: %v", name, rec.Texts())
		}
	}
	// The grid is rings, so it is stroked as paths of cubics rather than as
	// the two-point polylines a Cartesian grid is.
	rings := 0
	for _, path := range pathsOf(rec, "StrokePath") {
		if curved(path) {
			rings++
		}
	}
	if rings < 2 {
		t.Errorf("%d curved pieces of furniture, want the concentric rings", rings)
	}
}

// The spec carries the coord as a field of its own rather than smuggling a pie
// through an `arc` mark refract could not rebuild. See ADR 0014.
func TestTheSpecRoundTripsTheCoord(t *testing.T) {
	// The stock theme, because a theme that is not registered does not survive
	// the trip and this test is about the coord — see docs/adr/0014-json-spec.md.
	p := refract.New(refract.Size(400, 400), refract.Legend(false),
		refract.Coord(coord.Polar(coord.Theta(coord.FromY), coord.Hole(0.4),
			coord.Start(math.Pi/8), coord.Counterclockwise(true))))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Bar(browsers(), geom.X("all"), geom.Y("share"), geom.GroupBy("browser")))

	b, err := p.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"coord"`) || !strings.Contains(string(b), `"polar"`) {
		t.Fatalf("the document does not name the coord:\n%s", b)
	}
	if strings.Contains(string(b), `"arc"`) {
		t.Error("a pie was written as an arc mark rather than as a bar in a polar coord")
	}

	q, err := refract.ParseJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	again, err := q.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(b) {
		t.Errorf("the round trip changed the document:\n got %s\nwant %s", again, b)
	}

	// And it draws the same chart.
	before, after := irtest.New(), irtest.New()
	if err := p.Render(before.Target()); err != nil {
		t.Fatal(err)
	}
	if err := q.Render(after.Target()); err != nil {
		t.Fatal(err)
	}
	if a, b := strings.Join(before.Trace(), "\n"), strings.Join(after.Trace(), "\n"); a != b {
		t.Errorf("the chart read back draws something else:\n got %s\nwant %s", b, a)
	}
}

// A Cartesian chart writes no coord at all. The absent coord and the explicit
// Cartesian one draw the same thing, and a field in every document refract has
// ever written would be noise in every one of them.
func TestACartesianChartWritesNoCoord(t *testing.T) {
	p := refract.New(refract.Size(300, 200))
	p.Add(geom.Line(refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {1, 2}}),
		geom.X("x"), geom.Y("y")))
	b, err := p.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"coord"`) {
		t.Errorf("a Cartesian chart wrote a coord:\n%s", b)
	}
}

// A coord belongs to a chart, so the panels of a facet share it — the panels
// of a facet are one plot over different rows, and one of them in another
// coordinate system would be a different chart.
func TestAFacetsPanelsShareTheCoord(t *testing.T) {
	var region []string
	var one, share []float64
	for _, r := range []string{"north", "south"} {
		for i := range 3 {
			region = append(region, r)
			one = append(one, 0)
			share = append(share, float64(10*(i+1)))
		}
	}
	src := refract.NewTable().String("region", region).Float64("all", one).Float64("share", share)

	p := refract.New(refract.Size(600, 320), refract.Theme(bare()), refract.Legend(false),
		refract.Coord(coord.Polar(coord.Theta(coord.FromY))))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Bar(src, geom.X("all"), geom.Y("share")))
	p.Facet(facet.Wrap("region", facet.Columns(2)))

	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}
	// Two panels, and both drew wedges: the clip of each is a disc rather than
	// a rectangle.
	discs := 0
	for _, path := range pathsOf(rec, "Push") {
		if curved(path) {
			discs++
		}
	}
	if discs != 2 {
		t.Errorf("%d panels clipped to a disc, want 2", discs)
	}
}

// A parallel render is byte-identical to a serial one, coord and all: Frame
// hands back the coord positioned in one panel rather than moving the chart's
// own, so two panels on two goroutines never share a centre. ADR 0012.
func TestAPolarFacetIsIdenticalInParallelAndSerial(t *testing.T) {
	build := func(parallel bool) string {
		var group []string
		var x, y []float64
		for g := range 4 {
			for i := range 8 {
				group = append(group, string(rune('a'+g)))
				x = append(x, float64(i))
				y = append(y, math.Sin(float64(i)/3+float64(g))+2)
			}
		}
		src := refract.NewTable().String("g", group).Float64("x", x).Float64("y", y)
		p := refract.New(refract.Size(700, 500), refract.Parallel(parallel),
			refract.Coord(coord.Polar()))
		p.Add(geom.Line(src, geom.X("x"), geom.Y("y"), geom.Closed(true)))
		p.Facet(facet.Wrap("g", facet.Columns(2)))
		var sb strings.Builder
		if err := p.Render(refract.SVGWriter(&sb)); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}
	if a, b := build(true), build(false); a != b {
		t.Error("a parallel polar render differs from a serial one")
	}
}

// A polar coord reports that it does not decimate, and the layer believes it:
// a bucket of equal angle is not a bucket of equal width, so a reduction
// defined over pixel columns would be measuring something it was not designed
// to measure. See ADR 0018, property 4.
func TestAPolarLayerDoesNotDecimate(t *testing.T) {
	const n = 20000
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = float64(i)
		y[i] = math.Sin(float64(i) / 91)
	}
	src := refract.Float64Columns(map[string][]float64{"x": x, "y": y})

	count := func(c coord.Coord) int {
		p := refract.New(refract.Size(400, 400), refract.Coord(c), refract.Legend(false))
		p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
		rec := irtest.New()
		if err := p.Render(rec.Target()); err != nil {
			t.Fatal(err)
		}
		return vertices(rec)
	}
	cart, polar := count(coord.Cartesian()), count(coord.Polar(coord.Chord()))
	if cart >= n {
		t.Errorf("a Cartesian line over %d rows drew %d vertices; it should have reduced", n, cart)
	}
	if polar < n {
		t.Errorf("a polar line drew %d vertices of %d; it should have drawn every row", polar, n)
	}
}

// dist is how far apart two device points are. Coordinates are never compared
// with == in this repository: everything downstream of a scale is ordinary
// float arithmetic the compiler may contract into a fused multiply-add on one
// architecture and not on another. See AGENTS.md.
func dist(a, b ir.Point) float64 {
	return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y))
}

// pathsOf returns the paths of every recorded call of one kind, in order.
func pathsOf(rec *irtest.Recorder, op string) []*ir.Path {
	var out []*ir.Path
	for _, c := range rec.Filter(op) {
		if c.Path != nil {
			out = append(out, c.Path)
		}
	}
	return out
}

// clipped returns the paths of one kind of call drawn inside a panel's clip,
// which is where the data is and nowhere else. The canvas background and the
// legend swatches are fills too, and neither is a mark.
func clipped(rec *irtest.Recorder, op string) []*ir.Path {
	var out []*ir.Path
	depth := 0
	for _, c := range rec.Calls {
		switch c.Op {
		case "Push":
			depth++
		case "Pop":
			depth--
		case op:
			if depth > 0 && c.Path != nil {
				out = append(out, c.Path)
			}
		}
	}
	return out
}

// marks counts the filled marks a recording drew.
func marks(rec *irtest.Recorder) int { return len(clipped(rec, "FillPath")) }

// cubics counts the cubic segments in everything a recording drew. It is how a
// test says "this chart has arcs in it" without knowing which call carried
// them.
func cubics(rec *irtest.Recorder) int {
	n := 0
	for _, c := range rec.Calls {
		if c.Path == nil {
			continue
		}
		for _, op := range c.Path.Ops {
			if op == ir.OpCubicTo {
				n++
			}
		}
	}
	return n
}

// curved reports whether a path bends.
func curved(p *ir.Path) bool {
	for _, op := range p.Ops {
		if op == ir.OpCubicTo {
			return true
		}
	}
	return false
}

// vertices counts the points a recording's polylines and stroked paths carry,
// which is how many marks survived a reduction.
func vertices(rec *irtest.Recorder) int {
	n := 0
	for _, c := range rec.Calls {
		switch c.Op {
		case "Polyline":
			n += len(c.Points)
		case "StrokePath":
			n += len(c.Path.Pts)
		}
	}
	return n
}

// Steering a chart goes back through the coord too. A wheel is a device
// position and a drag is a device delta, and neither means anything to a scale
// that maps into an angle: the pointer is inverted first, and the domain moves
// by what the *scales* saw change.
func TestZoomingAPolarChartMovesTheRadialDomain(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{
		"angle": {0, 1, 2, 3},
		"r":     {1, 2, 3, 4},
	})
	p := refract.New(refract.Size(400, 400), refract.Theme(bare()), refract.Legend(false),
		refract.Coord(coord.Polar()))
	p.X(scale.Linear(scale.Domain(0, 4)))
	p.Y(scale.Linear())
	p.Add(geom.Scatter(src, geom.X("angle"), geom.Y("r")))

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

	// A wheel at the rim, where the radial domain's far end is.
	at := pn.Coords().Point(pn.X.Map(0), pn.Y.Map(4))
	if err := live.Wheel(float64(at.X), float64(at.Y), 0.5); err != nil {
		t.Fatal(err)
	}
	lo, hi := pn.Y.Domain()
	if hi-lo >= 4 {
		t.Errorf("the radial domain is still %v wide after zooming in", hi-lo)
	}
	// Zooming about the rim keeps the rim where it was, which is the property
	// that makes a wheel feel attached to the pointer.
	if math.Abs(hi-4) > 0.2 {
		t.Errorf("the far end of the domain moved to %v, want it to stay at 4", hi)
	}
}
