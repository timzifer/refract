package refract_test

// The secondary Y axis, end to end. What these are about is that one chart has
// two vertical scales rather than that two charts were drawn on top of each
// other: the layers are trained on the axis they read, the axis is written at
// the right-hand edge, the grid stays the primary axis's, and a pointer over a
// mark reports the value of the axis that mark was placed by.

import (
	"strings"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// twoUnits is the chart this exists for: money as bars against the left axis
// and a percentage as a line against the right one.
func twoUnits(opts ...refract.Option) (*refract.Plot, scale.Scale, scale.Scale) {
	src := refract.Float64Columns(map[string][]float64{
		"m":       {0, 1, 2, 3},
		"revenue": {1000, 1400, 1200, 1600},
		"margin":  {0.11, 0.14, 0.09, 0.17},
	})
	y := scale.Linear(scale.Nice(), scale.Zero())
	y2 := scale.Linear(scale.Nice(), scale.NumberFormat("#.0%"))

	p := refract.New(append([]refract.Option{
		refract.Size(600, 360), refract.YTitle("revenue"), refract.Y2Title("margin"),
	}, opts...)...)
	p.X(scale.Linear(scale.Nice()))
	p.Y(y)
	p.Y2(y2)
	p.Add(geom.Bar(src, geom.X("m"), geom.Y("revenue")))
	p.Add(geom.Line(src, geom.X("m"), geom.Y("margin"), geom.OnY2()))
	return p, y, y2
}

// The two axes describe their own layers and nothing else. A secondary axis
// trained on the primary layer's numbers would run to 1600 and squash the line
// it exists to make readable.
func TestEachAxisDescribesItsOwnLayers(t *testing.T) {
	p, y, y2 := twoUnits()
	svgOf(t, p)

	if lo, hi := y.Domain(); lo != 0 || hi < 1600 {
		t.Errorf("the primary axis runs %v..%v, want it to cover the revenue", lo, hi)
	}
	if _, hi := y2.Domain(); hi > 1 {
		t.Errorf("the secondary axis reaches %v; it was trained on the primary layer's numbers", hi)
	}
}

// The right-hand axis writes its labels, in its own format, outside the panel.
func TestTheSecondaryAxisIsWrittenOnTheRight(t *testing.T) {
	p, _, _ := twoUnits()
	doc := svgOf(t, p)
	if !strings.Contains(doc, "%") {
		t.Errorf("no percentage label was written:\n%s", firstLabels(doc))
	}
	if !strings.Contains(doc, ">margin<") {
		t.Error("the secondary axis has no title")
	}

	// Every percentage label sits to the right of every revenue label, which
	// is what "on the right" means when the assertion has to survive a
	// layout change.
	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}
	var leftMost, rightMost float32 = 1e9, -1e9
	for _, c := range rec.Calls {
		if c.Op != "Text" {
			continue
		}
		switch {
		case strings.HasSuffix(c.Text.Text, "%"):
			if c.Text.At.X < leftMost {
				leftMost = c.Text.At.X
			}
		case c.Text.Text == "1200" || c.Text.Text == "800":
			if c.Text.At.X > rightMost {
				rightMost = c.Text.At.X
			}
		}
	}
	if leftMost <= rightMost {
		t.Errorf("a percentage label at x=%v is left of a revenue label at x=%v", leftMost, rightMost)
	}
}

// The grid stays the primary axis's. Two ladders of horizontal rules at
// different values are a moiré rather than a reading, and which of the two a
// line belongs to is unanswerable by looking.
func TestTheSecondaryAxisDrawsNoGrid(t *testing.T) {
	one := refract.New(refract.Size(600, 360))
	one.X(scale.Linear(scale.Nice()))
	one.Y(scale.Linear(scale.Nice(), scale.Zero()))
	src := refract.Float64Columns(map[string][]float64{"m": {0, 1, 2, 3}, "revenue": {1000, 1400, 1200, 1600}})
	one.Add(geom.Bar(src, geom.X("m"), geom.Y("revenue")))

	two, _, _ := twoUnits()

	if got, want := gridLines(t, two), gridLines(t, one); got != want {
		t.Errorf("the chart with two axes drew %d grid lines and the one with one drew %d", got, want)
	}
}

// gridLines counts the horizontal rules a chart draws across its panel.
func gridLines(t *testing.T, p *refract.Plot) int {
	t.Helper()
	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range rec.Calls {
		if c.Op == "Polyline" && len(c.Points) == 2 && c.Points[0].Y == c.Points[1].Y &&
			c.Points[1].X-c.Points[0].X > 100 {
			n++
		}
	}
	return n
}

// A pointer over a mark reports the value of the axis that mark was placed by.
// Reading it through the panel's own Y would name 4200 on a chart whose right
// axis reads 12 %.
func TestAHitOnTheSecondaryAxisReportsThatAxisValue(t *testing.T) {
	p, _, _ := twoUnits()
	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()

	// The line's own vertices are what the index holds, so asking at one of
	// them is asking about the row it is.
	var at ir.Point
	found := false
	for _, c := range rec.Calls {
		if c.Op == "Polyline" && len(c.Points) == 4 {
			at, found = c.Points[1], true
		}
	}
	if !found {
		t.Fatal("the line layer drew no run of four points")
	}
	hit, ok := ix.At(at, 4)
	if !ok {
		t.Fatal("nothing was hit at the line's own vertex")
	}
	if hit.Y > 1 {
		t.Errorf("the hit reports y=%v, which is the primary axis reading the same pixel; want the margin near 0.14", hit.Y)
	}
	if hit.Y < 0.05 || hit.Y > 0.3 {
		t.Errorf("the hit reports y=%v, want a margin between 0.05 and 0.3", hit.Y)
	}
}

// A layer that asks for an axis the chart does not have draws against the
// primary one, silently — the reason a break-out under a Cartesian coord draws
// nothing rather than erroring.
func TestALayerOnAnAxisThatIsNotThereUsesThePrimary(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {1, 2}})
	p := refract.New(refract.Size(400, 300))
	p.X(scale.Linear(scale.Nice()))
	y := scale.Linear(scale.Nice())
	p.Y(y)
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y"), geom.OnY2()))

	svgOf(t, p)
	if lo, hi := y.Domain(); lo > 1 || hi < 2 {
		t.Errorf("the primary axis runs %v..%v; the orphaned layer did not train it", lo, hi)
	}
}

// A coord with no far side draws no second axis. A ring has one radial axis,
// and a second radius over the first would be two scales sharing one line.
func TestAPolarChartDrawsNoSecondAxis(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 0, 0}, "y": {1, 2, 3}})
	p := refract.New(refract.Size(400, 400), refract.Coord(coord.Pie()))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Y2(scale.Linear(scale.Domain(0, 100)))
	p.Add(geom.Bar(src, geom.X("x"), geom.Y("y")))

	// It renders, and it does not draw the second axis's labels — a pie has no
	// right-hand edge to write them against.
	if doc := svgOf(t, p); strings.Contains(doc, ">100<") {
		t.Errorf("a polar chart wrote a second axis:\n%s", firstLabels(doc))
	}
}

// A chart with no second axis is byte for byte the chart it was. This is the
// property every golden file rests on.
func TestAChartWithNoSecondAxisIsUnchanged(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1, 2}, "y": {1, 4, 2}})
	build := func(withY2 bool) string {
		p := refract.New(refract.Size(500, 300), refract.Title("Signal"))
		p.X(scale.Linear(scale.Nice()))
		p.Y(scale.Linear(scale.Nice()))
		if withY2 {
			p.Y2(scale.Linear(scale.Nice()))
		}
		p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
		return svgOf(t, p)
	}
	plain := build(false)
	if plain == build(true) {
		t.Error("adding a second axis changed nothing; it should at least draw its own line")
	}
	// And the same chart built twice with no second axis is the same chart.
	if plain != build(false) {
		t.Error("a chart with no second axis is not reproducible")
	}
}

// The whole thing survives the round trip through the document, including
// which layer is on which axis — a document that lost that would read back as
// a chart with a percentage plotted against a revenue axis.
func TestASecondAxisSurvivesTheRoundTrip(t *testing.T) {
	p, _, _ := twoUnits()
	s, err := p.Spec()
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"ySecondary"`) || !strings.Contains(string(b), `"axis": "y2"`) {
		t.Fatalf("the document is missing the axis or its binding:\n%s", b)
	}
	back, err := refract.FromSpec(s)
	if err != nil {
		t.Fatal(err)
	}
	if svgOf(t, back) != svgOf(t, p) {
		t.Error("the chart read back differently")
	}
}

// A facet writes the shared second axis at the right-hand edge of the grid,
// which is the mirror of where the first one is written.
func TestAFacetWritesTheSecondAxisAtItsRightEdge(t *testing.T) {
	src := refract.NewTable().
		Float64("x", []float64{0, 1, 0, 1}).
		Float64("y", []float64{10, 20, 30, 40}).
		Float64("p", []float64{0.1, 0.2, 0.3, 0.4}).
		String("g", []string{"a", "a", "b", "b"})

	p := refract.New(refract.Size(700, 320))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Y2(scale.Linear(scale.Domain(0, 1), scale.NumberFormat("#.0%")))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("p"), geom.OnY2()))
	p.Facet(facet.Wrap("g"))

	doc := svgOf(t, p)
	if !strings.Contains(doc, "%") {
		t.Errorf("the faceted chart wrote no secondary labels:\n%s", firstLabels(doc))
	}
	// Two panels, one shared axis: the percentage ladder is written once.
	if n := strings.Count(doc, "50%"); n != 1 {
		t.Errorf("the shared second axis was written %d times, want once", n)
	}
}

// A chart with two axes is one chart, so a zoom is one zoom. Moving the left
// axis and leaving the right one would slide the two series apart under the
// reader's hand.
func TestAZoomMovesBothVerticalAxes(t *testing.T) {
	p, y, y2 := twoUnits()
	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	before, before2 := span(y), span(y2)
	if err := live.Wheel(300, 180, 0.5); err != nil {
		t.Fatal(err)
	}
	after, after2 := span(y), span(y2)

	if after >= before {
		t.Fatalf("the primary axis did not zoom: %v then %v", before, after)
	}
	if after2 >= before2 {
		t.Fatalf("the secondary axis did not zoom: %v then %v; the two series have slid apart", before2, after2)
	}
	// Both by the same factor, which is what "one zoom" means.
	if r1, r2 := after/before, after2/before2; r1 < r2-0.01 || r1 > r2+0.01 {
		t.Errorf("the axes zoomed by %v and %v", r1, r2)
	}

	if err := live.Autoscale(); err != nil {
		t.Fatal(err)
	}
	if span(y2) != before2 {
		t.Errorf("the reset view left the secondary axis at %v, want %v", span(y2), before2)
	}
}

func span(s scale.Scale) float64 {
	lo, hi := s.Domain()
	return hi - lo
}
