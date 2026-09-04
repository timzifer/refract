package refract_test

// The v0.7 milestone, end to end: one layer over a long table draws N coloured
// series and the legend names all N; a stacked bar's axis reaches the stacked
// total and each segment is separately hittable and separately attributable to
// its row; and a heatmap and a gantt chart render from the public API.

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

// traffic is the long table every grouped chart in this file is drawn from:
// three regions measured at four times, one row per pair.
func traffic() refract.Source {
	var t, v []float64
	var region []string
	for i := range 4 {
		for r, name := range []string{"north", "south", "east"} {
			t = append(t, float64(i))
			v = append(v, float64(1+r)+float64(i))
			region = append(region, name)
		}
	}
	return refract.NewTable().Float64("t", t).Float64("v", v).String("region", region)
}

func TestOneLayerDrawsEverySeriesAndTheLegendNamesThem(t *testing.T) {
	p := refract.New(refract.Size(600, 400), refract.Title("Traffic"))
	p.Add(geom.Line(traffic(), geom.X("t"), geom.Y("v"), geom.GroupBy("region")))

	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if n := rec.Count("Polyline"); n < 3 {
		t.Fatalf("%d polylines drawn, want at least one per series", n)
	}
	// A single grouped layer is more than one thing to tell apart, so the
	// legend appears without being asked for.
	texts := strings.Join(rec.Texts(), " ")
	for _, name := range []string{"north", "south", "east"} {
		if !strings.Contains(texts, name) {
			t.Errorf("the legend does not name %q: %v", name, rec.Texts())
		}
	}
}

func TestAStackedBarSegmentIsHittableAndNamesItsRow(t *testing.T) {
	src := traffic()
	p := refract.New(refract.Size(600, 400))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Bar(src, geom.X("t"), geom.Y("v"), geom.GroupBy("region")))

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

	// The stack at t = 0 is 1 (north) + 2 (south) + 3 (east) = 6, so the axis
	// reaches the total rather than the tallest single value.
	panel := ix.Panels()[0]
	if _, hi := panel.Y.Domain(); hi < 6 {
		t.Fatalf("the Y axis reaches %v, and the tallest stack is 9", hi)
	}

	// Point at the middle of the middle segment of the first stack: south runs
	// from 1 to 3 there, so 2 is inside it.
	at := ir.Point{X: panel.X.Map(0), Y: panel.Y.Map(2)}
	hit, ok := ix.At(at, 6)
	if !ok {
		t.Fatal("nothing was found inside a stacked segment")
	}
	if hit.Row != 1 {
		t.Errorf("the segment reports row %d, want row 1 — south at t=0", hit.Row)
	}
	// And the segment above it is a different mark reporting a different row,
	// which is what "separately hittable" means.
	above, ok := ix.At(ir.Point{X: panel.X.Map(0), Y: panel.Y.Map(4.5)}, 6)
	if !ok {
		t.Fatal("nothing was found in the segment above")
	}
	if above.Row == hit.Row {
		t.Errorf("both segments report row %d: the stack is one shape", hit.Row)
	}
	if above.Row != 2 {
		t.Errorf("the top segment reports row %d, want row 2 — east at t=0", above.Row)
	}
}

func TestAHeatmapRendersFromThePublicAPI(t *testing.T) {
	var day, hour []string
	var calls []float64
	for d, name := range []string{"mon", "tue", "wed"} {
		for h := range 4 {
			day = append(day, name)
			hour = append(hour, time.Date(2026, 1, 1, 9+h, 0, 0, 0, time.UTC).Format("15:04"))
			calls = append(calls, float64(d*4+h))
		}
	}
	src := refract.NewTable().String("day", day).String("hour", hour).Float64("calls", calls)

	p := refract.New(refract.Size(500, 320), refract.Title("Calls"))
	p.X(scale.Ordinal())
	p.Y(scale.Ordinal())
	p.Add(geom.Rect(src, geom.X("day"), geom.Y("hour"),
		geom.ColorBy("calls", scale.Sequential(palette.Viridis))))

	var out strings.Builder
	if err := p.Render(refract.SVGWriter(&out)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	doc := out.String()
	// Twelve cells, and a colourbar beside them: a heatmap is a rect and a
	// ramp, and both halves were already here the day the rect was.
	if n := strings.Count(doc, "<path"); n < 12 {
		t.Errorf("%d paths in the document, want at least one per cell", n)
	}
	if !strings.Contains(doc, "linearGradient") {
		t.Error("no colourbar was drawn beside a layer painted from a ramp")
	}
}

func TestAGanttChartRendersFromThePublicAPI(t *testing.T) {
	start := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	src := refract.NewTable().
		Time("from", []time.Time{start, start.AddDate(0, 0, 3), start.AddDate(0, 0, 5)}).
		Time("to", []time.Time{start.AddDate(0, 0, 4), start.AddDate(0, 0, 9), start.AddDate(0, 0, 7)}).
		String("task", []string{"design", "build", "ship"})

	p := refract.New(refract.Size(600, 240), refract.Title("Plan"))
	p.X(scale.Time())
	p.Y(scale.Ordinal())
	p.Add(geom.Rect(src, geom.X("from"), geom.X2("to"), geom.Y("task")))

	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatalf("Render: %v", err)
	}
	fills := rec.Filter("FillPath")
	if len(fills) == 0 {
		t.Fatal("nothing was filled")
	}
	// The bars are one call with a subpath each. The third task is two days
	// long and the second is six, so the bars have the widths the dates say
	// rather than a width the slot says.
	bars := fills[len(fills)-1]
	var widths []float32
	var cur []ir.Point
	flush := func() {
		if len(cur) == 0 {
			return
		}
		lo, hi := cur[0].X, cur[0].X
		for _, p := range cur {
			lo, hi = min(lo, p.X), max(hi, p.X)
		}
		widths = append(widths, hi-lo)
		cur = nil
	}
	bars.Path.Walk(func(op ir.PathOp, pts []ir.Point) {
		if op == ir.OpMoveTo {
			flush()
		}
		cur = append(cur, pts...)
	})
	flush()
	if len(widths) != 3 {
		t.Fatalf("got %d bars, want one per task", len(widths))
	}
	// design: 4 days, build: 6 days, ship: 2 days.
	if math.Abs(float64(widths[1]/widths[2])-3) > 0.05 {
		t.Errorf("the six-day task is %v wide and the two-day task %v: the ratio should be 3",
			widths[1], widths[2])
	}
}

func TestAStreamgraphIsOneLayerOverALongTable(t *testing.T) {
	p := refract.New(refract.Size(600, 300))
	p.Add(geom.Area(traffic(), geom.X("t"), geom.Y("v"),
		geom.GroupBy("region"),
		geom.Stack(geom.StackWiggle),
		geom.Order(geom.OrderInsideOut)))

	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatalf("Render: %v", err)
	}
	// One band per series, each a closed shape between its two edges.
	if n := rec.Count("FillPath"); n < 3 {
		t.Errorf("%d filled bands, want one per series", n)
	}
	texts := strings.Join(rec.Texts(), " ")
	if !strings.Contains(texts, "north") {
		t.Errorf("the bands are not named: %v", rec.Texts())
	}
}

// The golden files for the marks this milestone added. They are what would
// catch a change to the geometry that no assertion above happens to look at.
func TestGoldenStackedAndHeatmap(t *testing.T) {
	t.Run("stacked", func(t *testing.T) {
		p := refract.New(refract.Size(480, 320), refract.Title("Stacked"))
		p.X(scale.Linear())
		p.Y(scale.Linear(scale.Nice(), scale.Zero()))
		p.Add(geom.Bar(traffic(), geom.X("t"), geom.Y("v"),
			geom.GroupBy("region"),
			geom.ColorBy("region", scale.Qualitative(palette.OkabeIto))))
		golden(t, "stacked", p)
	})

	t.Run("heatmap", func(t *testing.T) {
		var day, hour []string
		var calls []float64
		for d, name := range []string{"mon", "tue", "wed"} {
			for h := range 4 {
				day, hour = append(day, name), append(hour, []string{"09", "10", "11", "12"}[h])
				calls = append(calls, float64(d*3+h*h))
			}
		}
		src := refract.NewTable().String("day", day).String("hour", hour).Float64("calls", calls)
		p := refract.New(refract.Size(480, 320), refract.Title("Heatmap"))
		p.X(scale.Ordinal(scale.OrdinalPadding(0)))
		p.Y(scale.Ordinal(scale.OrdinalPadding(0)))
		p.Add(geom.Rect(src, geom.X("day"), geom.Y("hour"),
			geom.ColorBy("calls", scale.Sequential(palette.Viridis))))
		golden(t, "heatmap", p)
	})
}

// ADR 0012, over the one thing in v0.7 that could break it: the groups are
// indexed through a map, and a group order that depended on map iteration
// would be an order that depended on scheduling.
func TestAFacetedStackIsTheSameChartOnEveryGoroutine(t *testing.T) {
	src := traffic()
	build := func(parallel bool) []string {
		p := refract.New(refract.Size(700, 460), refract.Parallel(parallel))
		p.Add(geom.Area(src, geom.X("t"), geom.Y("v"),
			geom.GroupBy("region"), geom.Stack(geom.StackWiggle)))
		p.Facet(facet.Wrap("region", facet.Columns(2)))
		rec := irtest.New()
		if err := p.Render(rec.Target()); err != nil {
			t.Fatalf("Render: %v", err)
		}
		return rec.Trace()
	}
	// Ten runs, because a scheduling-dependent order is a flake rather than a
	// failure: it comes out right most of the time.
	want := build(false)
	for range 10 {
		if got := build(true); strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatal("a parallel render of a stacked facet differs from a serial one")
		}
	}
}

// A chart with groups still writes down and reads back as the same chart,
// which is the clause of the DoD the spec answers.
func TestAGroupedChartSurvivesBeingWrittenDown(t *testing.T) {
	src := traffic()
	p := refract.New(refract.Size(600, 400), refract.Title("Traffic"))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Bar(src, geom.X("t"), geom.Y("v"),
		geom.GroupBy("region"),
		geom.ColorBy("region", scale.Qualitative(palette.OkabeIto)),
		geom.Stack(geom.StackFill)))

	b, err := p.MarshalJSON()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	q, err := refract.ParseJSON(b)
	if err != nil {
		t.Fatalf("ParseJSON: %v\n%s", err, b)
	}
	before, after := irtest.New(), irtest.New()
	if err := p.Render(before.Target()); err != nil {
		t.Fatal(err)
	}
	if err := q.Render(after.Target()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(before.Trace(), "\n") != strings.Join(after.Trace(), "\n") {
		t.Errorf("the chart came back drawing something else\n%s", b)
	}
}
