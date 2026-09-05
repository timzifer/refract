package geom_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// v0.9: the distribution marks and the size channel.
//
// What these tests are about is the seam rather than the arithmetic — the
// statistics themselves are pure functions with their own tests in package
// stat. What a geom has to get right is which axis it decides, which buffers it
// keeps, and that a layer redrawn twice draws the same thing.

// A histogram's Y axis is the counts, which are nowhere in the data. If it
// trained on the column instead, the bars would run off the top of an axis
// describing observations.
func TestAHistogramTrainsItsAxisOnTheCounts(t *testing.T) {
	vs := make([]float64, 100)
	for i := range vs {
		vs[i] = float64(i % 10)
	}
	src := data.Float64Columns(map[string][]float64{"v": vs})
	g := geom.Histogram(src, geom.X("v"), geom.Bins(10))

	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatal(err)
	}
	if lo, hi := y.Domain(); lo != 0 || hi < 10 {
		t.Errorf("the count axis runs %v..%v; ten bins over a hundred rows put ten in each", lo, hi)
	}
	if lo, hi := x.Domain(); lo > 0 || hi < 9 {
		t.Errorf("the value axis runs %v..%v, want it to cover the data", lo, hi)
	}
}

// The bins are a division of the axis, so the bars touch. A gap between them
// would read as a bar chart of categories, which is a different claim.
func TestAHistogramsBarsTouch(t *testing.T) {
	vs := make([]float64, 40)
	for i := range vs {
		vs[i] = float64(i)
	}
	src := data.Float64Columns(map[string][]float64{"v": vs})
	g := geom.Histogram(src, geom.X("v"), geom.Bins(4))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 200, 100)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	bars := rectsOf(t, rec.Filter("FillPath")[0])
	if len(bars) != 4 {
		t.Fatalf("got %d bars, want one per bin", len(bars))
	}
	for i := 1; i < len(bars); i++ {
		if math.Abs(float64(bars[i].Min.X-bars[i-1].Max.X)) > 0.01 {
			t.Errorf("bar %d starts at %v and bar %d ended at %v", i, bars[i].Min.X, i-1, bars[i-1].Max.X)
		}
	}
}

// An ECDF's Y axis is a fraction, and it runs the whole way whatever the
// sample: a curve that stopped at 0.97 would read as a distribution with
// something missing.
func TestAnECDFRunsTheWholeAxis(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{"v": {3, 1, 2, 2}})
	g := geom.ECDF(src, geom.X("v"))
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatal(err)
	}
	if lo, hi := y.Domain(); lo != 0 || hi != 1 {
		t.Errorf("the fraction axis runs %v..%v, want 0..1", lo, hi)
	}
}

// One staircase per series, drawn from the series' own rows. This is the
// composition with v0.7's groups the milestone asks for.
func TestAGroupedECDFDrawsOneCurvePerSeries(t *testing.T) {
	tbl := data.NewTable()
	tbl.Float64("v", []float64{1, 2, 3, 10, 20, 30})
	tbl.String("who", []string{"a", "a", "a", "b", "b", "b"})

	g := geom.ECDF(tbl, geom.X("v"), geom.GroupBy("who"))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 300, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Filter("Polyline")); n != 2 {
		t.Fatalf("got %d staircases, want one per series", n)
	}
	if n := len(geom.Legends(g, f)); n != 2 {
		t.Errorf("the layer contributes %d legend entries, want one per series", n)
	}
}

// A violin is one closed shape per (slot, series), and the two flanks are one
// subpath: a pointer inside the shape has landed on this distribution.
func TestAViolinDrawsOneShapePerGroupPerSlot(t *testing.T) {
	tbl := data.NewTable()
	var vals []float64
	var cats, who []string
	for i := range 60 {
		vals = append(vals, math.Sin(float64(i))*3+float64(i%7))
		cats = append(cats, []string{"x", "y"}[i%2])
		who = append(who, []string{"a", "b"}[(i/2)%2])
	}
	tbl.Float64("v", vals)
	tbl.String("cat", cats)
	tbl.String("who", who)

	g := geom.Violin(tbl, geom.X("cat"), geom.Y("v"), geom.GroupBy("who"))
	rec, f := frameOn(t, g, scale.Ordinal(), scale.Linear(), 400, 300)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Filter("FillPath")); n != 4 {
		t.Fatalf("got %d violins, want two slots by two series", n)
	}
	for _, c := range rec.Filter("FillPath") {
		if got := len(subpaths(t, c.Path)); got != 1 {
			t.Errorf("a violin is %d subpaths, want one closed shape", got)
		}
	}
}

// Two violins of the same shape and different sample sizes are the same width:
// every estimate integrates to 1, and they are drawn against the widest of them.
func TestViolinsAreComparedByShapeNotBySampleSize(t *testing.T) {
	tbl := data.NewTable()
	var vals []float64
	var cats []string
	for i := range 20 {
		vals = append(vals, float64(i%10))
		cats = append(cats, "few")
	}
	for i := range 200 {
		vals = append(vals, float64(i%10))
		cats = append(cats, "many")
	}
	tbl.Float64("v", vals)
	tbl.String("cat", cats)

	g := geom.Violin(tbl, geom.X("cat"), geom.Y("v"), geom.Bandwidth(1))
	rec, f := frameOn(t, g, scale.Ordinal(), scale.Linear(), 400, 300)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	fills := rec.Filter("FillPath")
	if len(fills) != 2 {
		t.Fatalf("got %d violins, want two", len(fills))
	}
	a, b := fills[0].Path.Bounds(), fills[1].Path.Bounds()
	if math.Abs(float64(a.Dx()-b.Dx())) > 1 {
		t.Errorf("the twenty-row violin is %v wide and the two-hundred-row one %v; they hold the same distribution", a.Dx(), b.Dx())
	}
}

// A ridgeline needs somewhere to put its rows, and a continuous axis has no
// slots. Saying so is better than drawing every ridge on top of the last.
func TestARidgelineRefusesAContinuousAxis(t *testing.T) {
	tbl := data.NewTable()
	tbl.Float64("v", []float64{1, 2, 3})
	tbl.String("m", []string{"jan", "jan", "feb"})
	g := geom.Ridgeline(tbl, geom.X("v"), geom.Y("m"))
	if err := g.Train(scale.Linear(), scale.Linear()); err == nil {
		t.Error("a ridgeline was trained against a continuous Y axis")
	}
}

// The ridges rise out of their slots and overlap, which is the whole point of
// the chart — so the tallest one reaches past the slot it belongs to.
func TestRidgesRiseOutOfTheirSlots(t *testing.T) {
	tbl := data.NewTable()
	var vals []float64
	var months []string
	for i := range 120 {
		vals = append(vals, float64(i%20)+float64(i/40))
		months = append(months, []string{"jan", "feb", "mar"}[i/40])
	}
	tbl.Float64("v", vals)
	tbl.String("m", months)

	g := geom.Ridgeline(tbl, geom.X("v"), geom.Y("m"), geom.Overlap(2))
	y := scale.Ordinal()
	rec, f := frameOn(t, g, scale.Linear(), y, 400, 300)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	fills := rec.Filter("FillPath")
	if len(fills) != 3 {
		t.Fatalf("got %d ridges, want one per month", len(fills))
	}
	band, ok := y.(scale.Band)
	if !ok {
		t.Fatal("an ordinal scale is not a band scale")
	}
	tallest := float32(0)
	for _, c := range fills {
		tallest = max(tallest, c.Path.Bounds().Dy())
	}
	if tallest <= band.Bandwidth() {
		t.Errorf("the tallest ridge is %v and a slot is %v; Overlap(2) asked for twice the slot", tallest, band.Bandwidth())
	}
}

// A hexbin's cells are regular hexagons on screen, which is the whole reason
// the lattice is in device space.
func TestAHexbinDrawsHexagons(t *testing.T) {
	xs, ys := make([]float64, 500), make([]float64, 500)
	for i := range xs {
		xs[i] = math.Mod(float64(i)*7.13, 10)
		ys[i] = math.Mod(float64(i)*3.71, 10)
	}
	src := data.Float64Columns(map[string][]float64{"x": xs, "y": ys})
	g := geom.Hexbin(src, geom.X("x"), geom.Y("y"), geom.DensityCells(12))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 300, 300)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	calls := rec.Filter("FillPath")
	if len(calls) == 0 {
		t.Fatal("a hexbin over five hundred rows drew nothing")
	}
	total := 0
	for _, c := range calls {
		for _, op := range c.Path.Ops {
			if op == ir.OpMoveTo {
				total++
			}
		}
		// Six vertices and a close per cell.
		if len(c.Path.Ops)%7 != 0 {
			t.Errorf("a cell path has %d ops, which is not a whole number of hexagons", len(c.Path.Ops))
		}
	}
	if total < 10 {
		t.Errorf("got %d cells, want a lattice rather than a handful", total)
	}
}

// A swarm shows every row and hides none of them, and it does so without
// consulting a random number: two builds of one layer are the same picture.
func TestASwarmPlacesEveryRowAndRepeatsItself(t *testing.T) {
	tbl := data.NewTable()
	var vals []float64
	var cats []string
	for i := range 80 {
		vals = append(vals, float64(i%20))
		cats = append(cats, []string{"a", "b"}[i%2])
	}
	tbl.Float64("v", vals)
	tbl.String("cat", cats)

	g := geom.Beeswarm(tbl, geom.X("cat"), geom.Y("v"))
	rec, f := frameOn(t, g, scale.Ordinal(), scale.Linear(), 400, 300)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	first := rec.Filter("Markers")
	n := 0
	for _, c := range first {
		n += len(c.Points)
	}
	if n != 80 {
		t.Fatalf("drew %d marks for 80 rows", n)
	}

	again := irtest.New()
	if err := g.Build(again, f); err != nil {
		t.Fatal(err)
	}
	second := again.Filter("Markers")
	if len(second) != len(first) {
		t.Fatalf("the second build made %d calls and the first %d", len(second), len(first))
	}
	for i := range first {
		for j, p := range first[i].Points {
			if second[i].Points[j] != p {
				t.Fatalf("mark %d of call %d moved between builds: %v then %v", j, i, p, second[i].Points[j])
			}
		}
	}
}

// A swarm stays inside its slot. A mark in the neighbouring slot is a mark
// attributed to the wrong category.
func TestASwarmStaysInItsSlot(t *testing.T) {
	tbl := data.NewTable()
	var vals []float64
	var cats []string
	for i := range 200 {
		vals = append(vals, 5) // every row at one value: the worst case
		cats = append(cats, []string{"a", "b"}[i%2])
	}
	tbl.Float64("v", vals)
	tbl.String("cat", cats)

	x := scale.Ordinal()
	g := geom.Beeswarm(tbl, geom.X("cat"), geom.Y("v"))
	rec, f := frameOn(t, g, x, scale.Linear(), 400, 300)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	band := x.(scale.Band)
	half := band.Bandwidth() / 2
	for _, c := range rec.Filter("Markers") {
		for _, p := range c.Points {
			near := math.Min(math.Abs(float64(p.X-x.Map(0))), math.Abs(float64(p.X-x.Map(1))))
			if near > float64(half) {
				t.Fatalf("a mark at x=%v is %v from the nearer slot centre, and a slot is %v wide", p.X, near, band.Bandwidth())
			}
		}
	}
}

// A trend is fitted in data space, so the axis describes the curve as well as
// the observations and a fit that runs past them is inside the plot.
func TestATrendTrainsTheAxisOnItsFit(t *testing.T) {
	xs, ys := make([]float64, 50), make([]float64, 50)
	for i := range xs {
		xs[i] = float64(i)
		ys[i] = 2*float64(i) + 1
	}
	src := data.Float64Columns(map[string][]float64{"x": xs, "y": ys})
	g := geom.Trend(src, geom.X("x"), geom.Y("y"), geom.Smooth(geom.LinearFit))
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatal(err)
	}
	lo, hi := y.Domain()
	if lo > 1 || hi < 99 {
		t.Errorf("the Y axis runs %v..%v, want it to cover the fit through the data", lo, hi)
	}
}

// A straight fit is two points, because a straight line is determined by its
// ends and sixty-two more would suggest it had been evaluated somewhere.
func TestAStraightTrendIsTwoPoints(t *testing.T) {
	xs, ys := make([]float64, 30), make([]float64, 30)
	for i := range xs {
		xs[i] = float64(i)
		ys[i] = float64(i) * 3
	}
	src := data.Float64Columns(map[string][]float64{"x": xs, "y": ys})
	g := geom.Trend(src, geom.X("x"), geom.Y("y"), geom.Smooth(geom.LinearFit))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 300, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	lines := rec.Filter("Polyline")
	if len(lines) != 1 || len(lines[0].Points) != 2 {
		t.Fatalf("a straight fit drew %d calls, the first with %d points", len(lines), len(lines[0].Points))
	}
}

func TestAGroupedTrendFitsEachSeries(t *testing.T) {
	tbl := data.NewTable()
	var xs, ys []float64
	var who []string
	for i := range 60 {
		xs = append(xs, float64(i%30))
		ys = append(ys, float64(i%30)*float64(1+i/30))
		who = append(who, []string{"a", "b"}[i/30])
	}
	tbl.Float64("x", xs)
	tbl.Float64("y", ys)
	tbl.String("who", who)

	g := geom.Trend(tbl, geom.X("x"), geom.Y("y"), geom.GroupBy("who"), geom.Smooth(geom.LinearFit))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 400, 300)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Filter("Polyline")); n != 2 {
		t.Errorf("got %d fits, want one per series", n)
	}
}

// The size channel. A sized scatter draws circles rather than markers, because
// the IR carries one marker style per call — see geom.SizeBy.
func TestASizedScatterDrawsOneCirclePerRow(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {0, 1, 2, 3},
		"n": {1, 2, 4, 8},
	})
	g := geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.SizeBy("n", scale.Size()))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 300, 300)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.Filter("Markers")); n != 0 {
		t.Errorf("a sized layer emitted %d marker calls; a bubble is a shape", n)
	}
	fills := rec.Filter("FillPath")
	if len(fills) != 1 {
		t.Fatalf("got %d fill calls, want one for the whole cloud", len(fills))
	}
	if got := len(subpaths(t, fills[0].Path)); got != 4 {
		t.Errorf("the path holds %d subpaths, want one per row so a pointer lands on a bubble", got)
	}
}

// Doubling a value multiplies the diameter by √2, all the way through the geom:
// the scale says so, and nothing between it and the ink changes it.
func TestABubblesDiameterFollowsTheSquareRootOfItsValue(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{
		"x": {0, 1},
		"y": {0, 0},
		"n": {25, 50},
	})
	s := scale.Size(scale.SizeRange(0, 40))
	g := geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.SizeBy("n", s))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 400, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	widths := subpathWidths(t, rec.Filter("FillPath")[0])
	if len(widths) != 2 {
		t.Fatalf("got %d bubbles, want two", len(widths))
	}
	// Largest first, so the doubled value is the first bubble.
	ratio := float64(widths[0] / widths[1])
	if math.Abs(ratio-math.Sqrt2) > 0.02 {
		t.Errorf("50 draws %v across and 25 draws %v: the ratio is %v, want √2", widths[0], widths[1], ratio)
	}
}

// A layer with a size scale contributes the third guide kind, and it names the
// column it was read from.
func TestASizedLayerContributesASizeGuide(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {0, 1}, "pop": {10, 20}})
	g := geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.SizeBy("pop", scale.Size()))
	if err := g.Train(scale.Linear(), scale.Linear()); err != nil {
		t.Fatal(err)
	}
	sized, ok := g.(geom.Sized)
	if !ok {
		t.Fatal("a sized scatter does not implement geom.Sized")
	}
	sg, ok := sized.SizeGuide()
	if !ok {
		t.Fatal("a sized scatter contributes no size guide")
	}
	if sg.Label != "pop" {
		t.Errorf("the guide is titled %q, want the column's name", sg.Label)
	}
	if _, hi := sg.Scale.Domain(); hi != 20 {
		t.Errorf("the guide's scale reaches %v, want the largest value it was trained on", hi)
	}
}

// A layer with no size column contributes nothing, which is what keeps every
// ordinary chart's guide column what it was.
func TestAnUnsizedLayerContributesNoSizeGuide(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {0, 1}})
	g := geom.Scatter(src, geom.X("x"), geom.Y("y"))
	if err := g.Train(scale.Linear(), scale.Linear()); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.(geom.Sized).SizeGuide(); ok {
		t.Error("a plain scatter contributed a size guide")
	}
}

// Every v0.9 mark can say what it is and be built back from the answer. A layer
// that could not would be a chart that cannot be written down.
func TestTheDistributionMarksRoundTripThroughADesc(t *testing.T) {
	tbl := data.NewTable()
	tbl.Float64("v", []float64{1, 2, 3, 4})
	tbl.Float64("w", []float64{4, 3, 2, 1})
	tbl.String("cat", []string{"a", "a", "b", "b"})

	for _, g := range []geom.Geom{
		geom.Histogram(tbl, geom.X("v"), geom.Bins(7), geom.BinRange(0, 10)),
		geom.Violin(tbl, geom.X("cat"), geom.Y("v"), geom.Bandwidth(0.5)),
		geom.Ridgeline(tbl, geom.X("v"), geom.Y("cat"), geom.Overlap(2.5)),
		geom.Hexbin(tbl, geom.X("v"), geom.Y("w"), geom.DensityCells(9)),
		geom.Beeswarm(tbl, geom.X("cat"), geom.Y("v")),
		geom.ECDF(tbl, geom.X("v"), geom.GroupBy("cat")),
		geom.Trend(tbl, geom.X("v"), geom.Y("w"), geom.Span(0.5), geom.Smooth(geom.LinearFit)),
	} {
		d, ok := geom.Describe(g)
		if !ok {
			t.Fatalf("%T cannot describe itself", g)
		}
		back, err := geom.FromDesc(d)
		if err != nil {
			t.Fatalf("%s: %v", d.Mark, err)
		}
		got, _ := geom.Describe(back)
		if got.Mark != d.Mark || got.Bins != d.Bins || got.BinLo != d.BinLo || got.BinHi != d.BinHi ||
			got.Bandwidth != d.Bandwidth || got.Overlap != d.Overlap || got.Span != d.Span ||
			got.Smooth != d.Smooth || got.CellSize != d.CellSize || got.Group != d.Group {
			t.Errorf("%s round-tripped as %+v, want %+v", d.Mark, got, d)
		}
	}
}

// subpathWidths measures each subpath's bounding width, in order.
func subpathWidths(t *testing.T, c irtest.Call) []float32 {
	t.Helper()
	var out []float32
	for _, p := range subpaths(t, c.Path) {
		out = append(out, p.Bounds().Dx())
	}
	return out
}
