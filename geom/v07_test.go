package geom_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// The v0.7 marks: a rectangle bounded by its own row, groups within one layer,
// the adjustments defined over them, and the legend that names them.

// longTable is the shape every grouped chart is drawn from: one row per
// (position, series) pair, with the series in a column of its own.
func longTable() *data.Table {
	return data.NewTable().
		Float64("t", []float64{0, 0, 0, 1, 1, 1, 2, 2, 2}).
		Float64("v", []float64{1, 2, 3, 2, 2, 2, 3, 1, 5}).
		String("series", []string{"a", "b", "c", "a", "b", "c", "a", "b", "c"})
}

// frameOn is [frame] with the scales chosen by the caller, for the layers that
// need a categorical axis or a fixed size.
func frameOn(t *testing.T, g geom.Geom, x, y scale.Scale, w, h float32) (*irtest.Recorder, geom.Frame) {
	t.Helper()
	if err := g.Train(x, y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	area := ir.R(0, 0, w, h)
	x.SetRange(area.Min.X, area.Max.X)
	y.SetRange(area.Max.Y, area.Min.Y)
	return irtest.New(), geom.Frame{Area: area, X: x, Y: y, Theme: theme.Light}
}

// rectsOf reads the cells out of a fill call: one subpath is one rectangle,
// which is also what makes each of them separately pointable.
func rectsOf(t *testing.T, c irtest.Call) []ir.Rect {
	t.Helper()
	if c.Path == nil {
		t.Fatal("a fill call carried no path")
	}
	var out []ir.Rect
	var cur []ir.Point
	flush := func() {
		if len(cur) == 0 {
			return
		}
		r := ir.Rect{Min: cur[0], Max: cur[0]}
		for _, p := range cur {
			r.Min.X, r.Min.Y = min(r.Min.X, p.X), min(r.Min.Y, p.Y)
			r.Max.X, r.Max.Y = max(r.Max.X, p.X), max(r.Max.Y, p.Y)
		}
		out = append(out, r)
		cur = nil
	}
	c.Path.Walk(func(op ir.PathOp, pts []ir.Point) {
		if op == ir.OpMoveTo {
			flush()
		}
		cur = append(cur, pts...)
	})
	flush()
	return out
}

func allRects(t *testing.T, rec *irtest.Recorder) []ir.Rect {
	t.Helper()
	var out []ir.Rect
	for _, c := range rec.Filter("FillPath") {
		out = append(out, rectsOf(t, c)...)
	}
	return out
}

func TestARectDrawsOneCellPerRow(t *testing.T) {
	src := data.NewTable().
		Float64("x0", []float64{0, 2, 4}).
		Float64("x1", []float64{1, 3, 6}).
		Float64("y0", []float64{0, 1, 2}).
		Float64("y1", []float64{1, 3, 4})
	g := geom.Rect(src, geom.X("x0"), geom.X2("x1"), geom.Y("y0"), geom.Y2("y1"))

	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	cells := allRects(t, rec)
	if len(cells) != 3 {
		t.Fatalf("got %d cells, want one per row", len(cells))
	}
	// The first cell runs x 0..1 against a domain of 0..6, so it occupies the
	// first sixth of the plot and is bounded by its own row rather than by a
	// baseline.
	if got := cells[0].Dx(); math.Abs(float64(got)-100.0/6) > 0.5 {
		t.Errorf("the first cell is %v wide, want a sixth of the plot", got)
	}
	if cells[0].Min.Y <= 0 && cells[0].Max.Y >= 100 {
		t.Error("the cell spans the whole plot vertically: it grew from a baseline")
	}
}

// The heatmap case: two categorical axes, no second column on either, and a
// cell that fills its slot on both.
func TestARectWithoutASecondColumnFillsItsSlot(t *testing.T) {
	src := data.NewTable().
		String("day", []string{"mon", "mon", "tue", "tue"}).
		String("hour", []string{"09", "10", "09", "10"}).
		Float64("calls", []float64{1, 5, 9, 3})
	g := geom.Rect(src, geom.X("day"), geom.Y("hour"),
		geom.ColorBy("calls", scale.Sequential(palette.Viridis)))

	x, y := scale.Ordinal(), scale.Ordinal()
	rec, f := frameOn(t, g, x, y, 200, 100)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	cells := allRects(t, rec)
	if len(cells) != 4 {
		t.Fatalf("got %d cells, want one per row", len(cells))
	}
	band := x.(scale.Band).Bandwidth()
	for i, c := range cells {
		if math.Abs(float64(c.Dx()-band)) > 0.01 {
			t.Fatalf("cell %d is %v wide, want the band's %v", i, c.Dx(), band)
		}
	}
	// Four counts, four colours: a heatmap is a rect and a ramp, and nothing
	// else was needed to draw one.
	if n := rec.Count("FillPath"); n != 4 {
		t.Errorf("%d fill calls, want one per distinct colour", n)
	}
}

// A layer painted from a ramp contributes a colourbar; one painted from a
// qualitative palette contributes a legend entry per category. Which one
// follows from the scale, which is the whole of ADR 0020's first half.
func TestTheGuideFollowsTheKindOfColourScale(t *testing.T) {
	src := longTable()
	ramp := geom.Rect(src, geom.X("t"), geom.Y("v"),
		geom.ColorBy("v", scale.Sequential(palette.Viridis)))
	discrete := geom.Rect(src, geom.X("t"), geom.Y("v"),
		geom.ColorBy("series", scale.Qualitative(palette.OkabeIto)))

	_, f := frame(t, ramp)
	if _, ok := ramp.(geom.Guided).ColorGuide(); !ok {
		t.Error("a layer painted from a ramp contributes no colourbar")
	}
	if es := geom.Legends(ramp, f); len(es) != 0 {
		t.Errorf("a layer painted from a ramp contributes %d legend entries, want none", len(es))
	}

	_, f = frame(t, discrete)
	if _, ok := discrete.(geom.Guided).ColorGuide(); ok {
		t.Error("a layer painted from a palette contributes a colourbar")
	}
	es := geom.Legends(discrete, f)
	if len(es) != 3 {
		t.Fatalf("got %d legend entries, want one per category", len(es))
	}
	if es[0].Label != "a" || es[2].Label != "c" {
		t.Errorf("entries are %q, %q, %q; want the categories in order of first sight",
			es[0].Label, es[1].Label, es[2].Label)
	}
}

func TestAGroupedLineDrawsOnePathPerSeries(t *testing.T) {
	g := geom.Line(longTable(), geom.X("t"), geom.Y("v"), geom.GroupBy("series"))
	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	lines := rec.Filter("Polyline")
	if len(lines) != 3 {
		t.Fatalf("got %d paths, want one per series", len(lines))
	}
	for i, l := range lines {
		if len(l.Points) != 3 {
			t.Fatalf("series %d has %d points, want its own three rows", i, len(l.Points))
		}
	}
	if lines[0].Stroke.Color == lines[1].Stroke.Color {
		t.Error("two series drew in the same colour")
	}
	es := geom.Legends(g, f)
	if len(es) != 3 || es[0].Label != "a" {
		t.Errorf("the legend names %d series (%v), want all three", len(es), es)
	}
}

// The DoD's first clause: the axis has to describe what will be drawn, which
// for a stack is the total rather than the tallest single value.
func TestAStackedBarReachesTheStackedTotal(t *testing.T) {
	g := geom.Bar(longTable(), geom.X("t"), geom.Y("v"), geom.GroupBy("series"))
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	_, hi := y.Domain()
	if hi < 9 {
		t.Errorf("the Y axis reaches %v; the tallest stack is 1+2+3=6, 2+2+2=6, 3+1+5=9", hi)
	}
	if hi > 9.001 {
		t.Errorf("the Y axis reaches %v, further than the tallest stack", hi)
	}
}

func TestAStackedBarStacksItsSegments(t *testing.T) {
	g := geom.Bar(longTable(), geom.X("t"), geom.Y("v"), geom.GroupBy("series"))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 300, 90)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if n := rec.Count("FillPath"); n != 3 {
		t.Fatalf("%d fill calls, want one per series", n)
	}
	// The three segments of the first slot: they touch, they do not overlap,
	// and together they span the whole stack.
	var col []ir.Rect
	for _, r := range allRects(t, rec) {
		if r.Min.X < 20 {
			col = append(col, r)
		}
	}
	if len(col) != 3 {
		t.Fatalf("the first slot holds %d segments, want three", len(col))
	}
	sortByTop(col)
	for i := 1; i < len(col); i++ {
		if math.Abs(float64(col[i].Min.Y-col[i-1].Max.Y)) > 0.01 {
			t.Errorf("segment %d ends at %v and the next starts at %v: the stack has a seam",
				i-1, col[i-1].Max.Y, col[i].Min.Y)
		}
	}
	// The stack is 1+2+3 = 6 of a Y axis that reaches 9 over 90 pixels, so it
	// is 60 pixels tall and its bottom is on the baseline.
	if h := col[len(col)-1].Max.Y - col[0].Min.Y; math.Abs(float64(h)-60) > 0.5 {
		t.Errorf("the stack is %v tall, want 60", h)
	}
	if bottom := col[len(col)-1].Max.Y; math.Abs(float64(bottom)-90) > 0.5 {
		t.Errorf("the stack sits at %v, want the baseline at 90", bottom)
	}
}

func sortByTop(rs []ir.Rect) {
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && rs[j].Min.Y < rs[j-1].Min.Y; j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
}

func TestFillingNormalisesEverySlotToOne(t *testing.T) {
	g := geom.Bar(longTable(), geom.X("t"), geom.Y("v"),
		geom.GroupBy("series"), geom.Stack(geom.StackFill))
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	if lo, hi := y.Domain(); lo != 0 || math.Abs(hi-1) > 1e-9 {
		t.Errorf("a 100%% chart's axis runs %v..%v, want 0..1", lo, hi)
	}
}

func TestDodgePutsTheSeriesSideBySide(t *testing.T) {
	g := geom.Bar(longTable(), geom.X("t"), geom.Y("v"),
		geom.GroupBy("series"), geom.Dodge(0))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 300, 100)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	cells := allRects(t, rec)
	if len(cells) != 9 {
		t.Fatalf("got %d bars, want one per row", len(cells))
	}
	// Dodged bars share a slot and do not overlap: no two of them cover the
	// same pixel column, and each still reaches the baseline.
	for i, a := range cells {
		if math.Abs(float64(a.Max.Y)-100) > 0.5 {
			t.Fatalf("bar %d does not reach the baseline: it was stacked as well as dodged", i)
		}
		for _, b := range cells[i+1:] {
			if a.Min.X < b.Max.X-0.01 && b.Min.X < a.Max.X-0.01 {
				t.Fatalf("bars %v and %v overlap", a, b)
			}
		}
	}
}

// ADR 0019's own consequence: a hole is a hole, per the layer's policy, and
// both the cumulative sum and the drawing traversal read the same answer.
func TestAHoleDoesNotShiftTheSegmentsAboveIt(t *testing.T) {
	with := data.NewTable().
		Float64("t", []float64{0, 0, 0}).
		Float64("v", []float64{1, math.NaN(), 3}).
		String("series", []string{"a", "b", "c"})
	without := data.NewTable().
		Float64("t", []float64{0, 0}).
		Float64("v", []float64{1, 3}).
		String("series", []string{"a", "c"})

	rects := func(src data.Source) []ir.Rect {
		g := geom.Bar(src, geom.X("t"), geom.Y("v"), geom.GroupBy("series"))
		x, y := scale.Linear(scale.Domain(-1, 1)), scale.Linear(scale.Domain(0, 4))
		rec, f := frameOn(t, g, x, y, 100, 100)
		if err := g.Build(rec, f); err != nil {
			t.Fatalf("Build: %v", err)
		}
		out := allRects(t, rec)
		sortByTop(out)
		return out
	}
	a, b := rects(with), rects(without)
	if len(a) != 2 || len(b) != 2 {
		t.Fatalf("got %d and %d segments, want two either way", len(a), len(b))
	}
	for i := range a {
		if math.Abs(float64(a[i].Min.Y-b[i].Min.Y)) > 0.01 {
			t.Errorf("segment %d is at %v with the hole and %v without it", i, a[i].Min.Y, b[i].Min.Y)
		}
	}
}

func TestOrderByValuePutsTheLargestSeriesAtTheBottom(t *testing.T) {
	g := geom.Area(longTable(), geom.X("t"), geom.Y("v"),
		geom.GroupBy("series"), geom.Order(geom.OrderValue))
	_, f := frame(t, g)
	es := geom.Legends(g, f)
	if len(es) != 3 {
		t.Fatalf("got %d entries", len(es))
	}
	// c totals 3+2+5 = 10, a totals 1+2+3 = 6, b totals 2+2+1 = 5.
	if es[0].Label != "c" || es[2].Label != "b" {
		t.Errorf("the order is %q, %q, %q; want the largest first",
			es[0].Label, es[1].Label, es[2].Label)
	}
}

func TestASharedColourScaleGivesASeriesOneColourEverywhere(t *testing.T) {
	cs := scale.Qualitative(palette.OkabeIto)
	first := longTable()
	// A second panel that has never seen series "a": without a shared scale
	// its "b" would take the first palette colour, which is the failure a
	// faceted chart runs into.
	second := data.NewTable().
		Float64("t", []float64{0, 1}).
		Float64("v", []float64{4, 5}).
		String("series", []string{"b", "b"})

	colorOf := func(src data.Source) ir.Color {
		g := geom.Line(src, geom.X("t"), geom.Y("v"),
			geom.GroupBy("series"), geom.ColorBy("series", cs))
		rec, f := frame(t, g)
		if err := g.Build(rec, f); err != nil {
			t.Fatalf("Build: %v", err)
		}
		for _, e := range geom.Legends(g, f) {
			if e.Label == "b" {
				return e.Color
			}
		}
		t.Fatal("series b was not in the legend")
		return ir.Color{}
	}
	if a, b := colorOf(first), colorOf(second); a != b {
		t.Errorf("series b is %v in one panel and %v in the other", a, b)
	}
}

func TestAGroupedLayerReportsTheRowBehindEverySegment(t *testing.T) {
	g := geom.Bar(longTable(), geom.X("t"), geom.Y("v"), geom.GroupBy("series"))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 300, 100)
	rows := &rowSink{}
	f.Rows = rows
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(rows.rows) != 9 {
		t.Fatalf("%d rows reported, want one per segment", len(rows.rows))
	}
	seen := map[int]bool{}
	for _, r := range rows.rows {
		if r < 0 || r > 8 || seen[r] {
			t.Fatalf("row %d is reported twice or is not a row of the table", r)
		}
		seen[r] = true
	}
}

// rowSink collects what a layer reports about its rows. The slices are lent
// for the call, so it copies.
type rowSink struct {
	at   []ir.Point
	rows []int
}

func (s *rowSink) Marks(at []ir.Point, rows []int) {
	s.at = append(s.at, at...)
	s.rows = append(s.rows, rows...)
}

func TestAnUngroupedLayerDrawsExactlyWhatItAlwaysDrew(t *testing.T) {
	src := src(map[string][]float64{"x": {0, 1, 2, 3}, "y": {1, 4, 2, 8}})
	plain := geom.Line(src, geom.X("x"), geom.Y("y"))
	rec, f := frame(t, plain)
	if err := plain.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if n := rec.Count("Polyline"); n != 1 {
		t.Errorf("%d polylines for a layer with no groups, want 1", n)
	}
	es := geom.Legends(plain, f)
	if len(es) != 1 || es[0].Label != "y" {
		t.Errorf("an ungrouped layer contributes %v, want its own single entry", es)
	}
}

func TestStackingWantsAGroupColumn(t *testing.T) {
	// Stacking one series on itself is the series, so it is a no-op rather
	// than an error — and the axis must not double.
	g := geom.Bar(src(map[string][]float64{"x": {0, 1}, "y": {2, 4}}),
		geom.X("x"), geom.Y("y"), geom.Stack(geom.StackZero))
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	if _, hi := y.Domain(); hi != 4 {
		t.Errorf("the axis reaches %v, want the tallest bar at 4", hi)
	}
}

func TestAMissingGroupColumnIsNamed(t *testing.T) {
	g := geom.Line(longTable(), geom.X("t"), geom.Y("v"), geom.GroupBy("nope"))
	err := g.Train(scale.Linear(), scale.Linear())
	if err == nil {
		t.Fatal("a group column that does not exist was accepted")
	}
	if !contains(err.Error(), "nope") {
		t.Errorf("the error does not name the column: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestABarTakesItsWidthFromAColumn(t *testing.T) {
	src := data.NewTable().
		Float64("x", []float64{1, 3}).
		Float64("y", []float64{1, 1}).
		Float64("w", []float64{2, 0.5})
	g := geom.Bar(src, geom.X("x"), geom.Y("y"), geom.WidthBy("w"))
	rec, f := frameOn(t, g, scale.Linear(scale.Domain(0, 4)), scale.Linear(scale.Domain(0, 1)), 400, 100)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	cells := allRects(t, rec)
	if len(cells) != 2 {
		t.Fatalf("got %d bars", len(cells))
	}
	// The axis runs 0..4 over 400 pixels, so a bar two units wide is 200.
	if math.Abs(float64(cells[0].Dx())-200) > 0.5 {
		t.Errorf("the first bar is %v wide, want 200", cells[0].Dx())
	}
	if math.Abs(float64(cells[1].Dx())-50) > 0.5 {
		t.Errorf("the second bar is %v wide, want 50", cells[1].Dx())
	}
}
