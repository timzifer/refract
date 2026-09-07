package geom_test

import (
	"errors"
	"math"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// measured is the table this mark exists for: a value and how well it is
// known, which every chart of a mean, a forecast or a tolerance has and which
// refract could not draw.
func measured() *data.Table {
	return data.NewTable().
		Float64("t", []float64{0, 1, 2}).
		Float64("mean", []float64{10, 12, 11}).
		Float64("sd", []float64{1, 2, 0.5}).
		Float64("lo", []float64{9, 10, 10.5}).
		Float64("hi", []float64{11, 14, 11.5})
}

// segmentsOf reads the endpoints of every stroked run out of a recording. An
// error bar is drawn as runs, so this is what its geometry is checked against.
func segmentsOf(rec *irtest.Recorder) [][2]ir.Point {
	var out [][2]ir.Point
	for _, c := range rec.Calls {
		switch c.Op {
		case "Polyline":
			if len(c.Points) == 2 {
				out = append(out, [2]ir.Point{c.Points[0], c.Points[1]})
			}
		case "StrokePath":
			if c.Path != nil && len(c.Path.Pts) == 2 {
				out = append(out, [2]ir.Point{c.Path.Pts[0], c.Path.Pts[1]})
			}
		}
	}
	return out
}

// The axis has to describe the interval, not the value: an interval whose top
// runs off the plot is exactly the reading the chart was opened for. That is
// why the bounds are derived in Train.
func TestAnErrorBarTrainsItsAxisOnTheInterval(t *testing.T) {
	g := geom.ErrorBar(measured(), geom.X("t"), geom.Y("mean"), geom.ErrorBy("sd"))
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatal(err)
	}
	lo, hi := y.Domain()
	if lo > 9 || hi < 14 {
		t.Errorf("the axis runs %v..%v, want it to reach 9 and 14 — the ends of the widest interval", lo, hi)
	}
}

// The two spellings describe the same interval, so they must draw the same
// picture. A caller with a mean and a standard deviation should not have to do
// arithmetic to get the chart of what they measured.
func TestTheTwoSpellingsDrawTheSameInterval(t *testing.T) {
	src := measured()
	spread := geom.ErrorBar(src, geom.X("t"), geom.Y("mean"), geom.ErrorBy("sd"), geom.Mid("mean"))
	bounds := geom.ErrorBar(src, geom.X("t"), geom.Y("lo"), geom.Y2("hi"), geom.Mid("mean"))

	a, fa := frameOn(t, spread, scale.Linear(scale.Domain(-1, 3)), scale.Linear(scale.Domain(8, 15)), 300, 200)
	if err := spread.Build(a, fa); err != nil {
		t.Fatal(err)
	}
	b, fb := frameOn(t, bounds, scale.Linear(scale.Domain(-1, 3)), scale.Linear(scale.Domain(8, 15)), 300, 200)
	if err := bounds.Build(b, fb); err != nil {
		t.Fatal(err)
	}
	// Row 0 is 10±1 and 9..11, row 2 is 11±0.5 and 10.5..11.5. Row 1 differs
	// on purpose — 12±2 is 10..14 and the columns say 10..14 too — so all
	// three agree and the comparison is the whole picture.
	as, bs := segmentsOf(a), segmentsOf(b)
	if len(as) != len(bs) {
		t.Fatalf("the spread spelling drew %d runs and the bounds spelling %d", len(as), len(bs))
	}
	for i := range as {
		if !samePoint(as[i][0], bs[i][0]) || !samePoint(as[i][1], bs[i][1]) {
			t.Errorf("run %d differs: %v vs %v", i, as[i], bs[i])
		}
	}
}

// Three runs per row: the rule and a cap at each end.
func TestAnErrorBarDrawsARuleAndTwoCaps(t *testing.T) {
	g := geom.ErrorBar(measured(), geom.X("t"), geom.Y("mean"), geom.ErrorBy("sd"))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 300, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if got := len(segmentsOf(rec)); got != 9 {
		t.Errorf("drew %d runs, want nine: a rule and two caps for each of three rows", got)
	}
}

// Turning the caps off is the point-range look, and it takes the caps away
// rather than shrinking them to nothing.
func TestCapsOffLeavesTheRule(t *testing.T) {
	g := geom.ErrorBar(measured(), geom.X("t"), geom.Y("mean"), geom.ErrorBy("sd"), geom.Caps(false))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 300, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if got := len(segmentsOf(rec)); got != 3 {
		t.Errorf("drew %d runs, want three: one rule per row and no caps", got)
	}
}

// The cap is half as wide as a bar of the same BarWidth, so an error bar drawn
// over a bar chart is narrower than the bar it annotates. That is what makes
// the two readable together.
func TestACapIsHalfABarWide(t *testing.T) {
	src := measured()
	bars := geom.Bar(src, geom.X("t"), geom.Y("mean"))
	errs := geom.ErrorBar(src, geom.X("t"), geom.Y("mean"), geom.ErrorBy("sd"))

	// A pinned domain, because the two marks widen a trained one by their own
	// half-widths and the comparison is about the widths rather than about
	// where each axis stopped.
	rb, fb := frameOn(t, bars, scale.Linear(scale.Domain(-1, 3)), scale.Linear(scale.Domain(0, 15)), 300, 200)
	if err := bars.Build(rb, fb); err != nil {
		t.Fatal(err)
	}
	re, fe := frameOn(t, errs, scale.Linear(scale.Domain(-1, 3)), scale.Linear(scale.Domain(0, 15)), 300, 200)
	if err := errs.Build(re, fe); err != nil {
		t.Fatal(err)
	}

	barW := rectsOf(t, rb.Filter("FillPath")[0])[0]
	var capW float32
	for _, seg := range segmentsOf(re) {
		if w := float32(math.Abs(float64(seg[1].X - seg[0].X))); w > capW {
			capW = w
		}
	}
	want := (barW.Max.X - barW.Min.X) / 2
	if math.Abs(float64(capW-want)) > 0.01 {
		t.Errorf("the cap is %v wide and the bar is %v; want the cap to be half the bar", capW, barW.Max.X-barW.Min.X)
	}
}

// Which axis the interval runs along follows from the encoding and nothing
// else — the rule a rect already follows about its edges, which is why there
// is no orientation option to forget.
func TestTheEncodingDecidesWhichWayItRuns(t *testing.T) {
	src := measured()
	h := geom.ErrorBar(src, geom.Y("t"), geom.X("mean"), geom.ErrorXBy("sd"))
	rec, f := frameOn(t, h, scale.Linear(), scale.Linear(), 300, 200)
	if err := h.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	segs := segmentsOf(rec)
	if len(segs) != 9 {
		t.Fatalf("drew %d runs, want nine", len(segs))
	}
	// The rule now varies in X and each cap in Y, which is the vertical case
	// turned a quarter turn.
	rules, caps := 0, 0
	for _, s := range segs {
		switch {
		case math.Abs(float64(s[1].X-s[0].X)) > 0.01:
			rules++
		case math.Abs(float64(s[1].Y-s[0].Y)) > 0.01:
			caps++
		}
	}
	if rules != 3 || caps != 6 {
		t.Errorf("got %d horizontal runs and %d vertical ones, want 3 and 6", rules, caps)
	}
}

// An interval on both axes is a box, not an interval. Guessing which the
// caller meant would draw a different chart depending on the order the options
// were written in.
func TestAnIntervalOnBothAxesIsRefused(t *testing.T) {
	g := geom.ErrorBar(measured(), geom.X("lo"), geom.X2("hi"), geom.Y("lo"), geom.Y2("hi"))
	err := g.Train(scale.Linear(), scale.Linear())
	if !errors.Is(err, geom.ErrBothAxes) {
		t.Errorf("error is %v, want ErrBothAxes", err)
	}
}

// An error bar with nothing to bound is a layer that named the wrong mark.
func TestAnErrorBarWithNoIntervalIsRefused(t *testing.T) {
	g := geom.ErrorBar(measured(), geom.X("t"), geom.Y("mean"))
	if err := g.Train(scale.Linear(), scale.Linear()); !errors.Is(err, geom.ErrNoInterval) {
		t.Errorf("error is %v, want ErrNoInterval", err)
	}
}

// The measurement is marked by the symmetric spelling, because with it there
// always is one; the bounds spelling marks nothing unless Mid names it. A
// minimum and a maximum are not evidence of a mean.
func TestOnlyAMeasurementIsMarked(t *testing.T) {
	markers := func(g geom.Geom) int {
		t.Helper()
		rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 300, 200)
		if err := g.Build(rec, f); err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, c := range rec.Filter("Markers") {
			n += len(c.Points)
		}
		return n
	}
	src := measured()
	if got := markers(geom.ErrorBar(src, geom.X("t"), geom.Y("mean"), geom.ErrorBy("sd"))); got != 3 {
		t.Errorf("the spread spelling drew %d markers, want one per row", got)
	}
	if got := markers(geom.ErrorBar(src, geom.X("t"), geom.Y("lo"), geom.Y2("hi"))); got != 0 {
		t.Errorf("the bounds spelling drew %d markers with no Mid; a min and a max are not evidence of a mean", got)
	}
	if got := markers(geom.ErrorBar(src, geom.X("t"), geom.Y("lo"), geom.Y2("hi"), geom.Mid("mean"))); got != 3 {
		t.Errorf("naming Mid drew %d markers, want one per row", got)
	}
}

// A negative half-width is the same interval read backwards, and a NaN one is
// a row with no interval — which is a hole like any other, so the row is not
// drawn and the ones around it are.
func TestAnAbsentIntervalIsAHole(t *testing.T) {
	src := data.NewTable().
		Float64("t", []float64{0, 1, 2}).
		Float64("v", []float64{10, 12, 11}).
		Float64("e", []float64{1, math.NaN(), -2})

	g := geom.ErrorBar(src, geom.X("t"), geom.Y("v"), geom.ErrorBy("e"))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 300, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if got := len(segmentsOf(rec)); got != 6 {
		t.Errorf("drew %d runs, want six: two rows with an interval and one without", got)
	}
	// The negative half-width drew the interval it describes, 9..13.
	y := f.Y
	var lowest float32
	for i, s := range segmentsOf(rec) {
		if i == 0 || s[0].Y > lowest {
			lowest = s[0].Y
		}
	}
	if lowest < y.Map(13) {
		t.Error("the negative half-width did not produce the interval it describes")
	}
}

// A dodged layer lines up with the dodged bars it annotates, which is the
// chart this mark is most often drawn on.
func TestADodgedErrorBarSitsInItsSeriesSlot(t *testing.T) {
	src := data.NewTable().
		Float64("t", []float64{0, 0, 1, 1}).
		Float64("v", []float64{10, 12, 11, 13}).
		Float64("e", []float64{1, 1, 1, 1}).
		String("g", []string{"a", "b", "a", "b"})

	g := geom.ErrorBar(src, geom.X("t"), geom.Y("v"), geom.ErrorBy("e"),
		geom.GroupBy("g"), geom.Dodge(0.1))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 400, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	// The two series at t=0 must not share a rule position, or the dodge did
	// nothing.
	var xs []float32
	for _, s := range segmentsOf(rec) {
		if math.Abs(float64(s[1].X-s[0].X)) < 0.01 { // a rule, not a cap
			xs = append(xs, s[0].X)
		}
	}
	if len(xs) != 4 {
		t.Fatalf("found %d rules, want four", len(xs))
	}
	if math.Abs(float64(xs[0]-xs[1])) < 0.01 {
		t.Errorf("the two series' rules are both at x=%v; the dodge did nothing", xs[0])
	}
}

// A grouped layer names its series in the legend, exactly as every other
// grouped mark does.
func TestAGroupedErrorBarNamesItsSeries(t *testing.T) {
	src := data.NewTable().
		Float64("t", []float64{0, 0}).
		Float64("v", []float64{10, 12}).
		Float64("e", []float64{1, 1}).
		String("g", []string{"a", "b"})

	g := geom.ErrorBar(src, geom.X("t"), geom.Y("v"), geom.ErrorBy("e"),
		geom.GroupBy("g"), geom.ColorBy("g", scale.Qualitative(nil)))
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatal(err)
	}
	entries := geom.Legends(g, geom.Frame{X: x, Y: y, Theme: theme.Light})
	if len(entries) != 2 {
		t.Fatalf("got %d legend entries, want one per series", len(entries))
	}
}

// A row's identity is at the measurement where there is one and at the middle
// of the interval where there is not — which is where a reader points when
// they mean "this interval".
func TestAnErrorBarReportsItsRowsAtTheMeasurement(t *testing.T) {
	g := geom.ErrorBar(measured(), geom.X("t"), geom.Y("mean"), geom.ErrorBy("sd"))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 300, 200)

	sink := &rowSink{}
	f.Rows = sink
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if len(sink.at) != 3 || len(sink.rows) != 3 {
		t.Fatalf("reported %d points for %d rows, want three of each", len(sink.at), len(sink.rows))
	}
	for i, p := range sink.at {
		if want := f.Y.Map([]float64{10, 12, 11}[i]); math.Abs(float64(p.Y-want)) > 0.01 {
			t.Errorf("row %d is reported at y=%v, want the measurement at %v", i, p.Y, want)
		}
	}
}
