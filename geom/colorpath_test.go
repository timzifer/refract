package geom_test

import (
	"errors"
	"math"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

// polylines is the stroked runs of a drawn layer, in the order they were
// drawn.
func polylines(rec *irtest.Recorder) []irtest.Call {
	return rec.Filter("Polyline")
}

func TestALineSplitsOnTheThresholdItCrosses(t *testing.T) {
	// Two rows either side of 100: one at 90, one at 110. The crossing is
	// exactly half way along, and that is where the colour has to change —
	// the reader is being told when the limit was passed.
	s := src(map[string][]float64{
		"x": {0, 10},
		"y": {90, 110},
	})
	cs := scale.Threshold(palette.Ramp{palette.Blue, palette.Red}, []float64{100})
	g := geom.Line(s, geom.X("x"), geom.Y("y"), geom.ColorBy("y", cs))
	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}

	runs := polylines(rec)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want one per class:\n%s", len(runs), rec.Trace())
	}
	if runs[0].Stroke.Color == runs[1].Stroke.Color {
		t.Fatal("both runs are the same colour")
	}
	// The runs share the boundary vertex, so the line has no gap in it.
	last := runs[0].Points[len(runs[0].Points)-1]
	if last != runs[1].Points[0] {
		t.Fatalf("run 1 ends at %v and run 2 begins at %v; the edge between them is undrawn", last, runs[1].Points[0])
	}
	// Half way in x, and on the mapped position of 100 in y.
	if got, want := last.X, f.X.Map(5); !closef(got, want) {
		t.Errorf("the corner is at x=%v, want the crossing at %v", got, want)
	}
	if got, want := last.Y, f.Y.Map(100); !closef(got, want) {
		t.Errorf("the corner is at y=%v, want the threshold at %v", got, want)
	}
}

func TestALineCrossingTwoBoundariesAtOnceDrawsTheClassBetween(t *testing.T) {
	s := src(map[string][]float64{
		"x": {0, 10},
		"y": {0, 300},
	})
	cs := scale.Threshold(palette.Ramp{palette.Blue, palette.Green, palette.Red}, []float64{100, 200})
	g := geom.Line(s, geom.X("x"), geom.Y("y"), geom.ColorBy("y", cs))
	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	runs := polylines(rec)
	if len(runs) != 3 {
		t.Fatalf("got %d runs, want one per class the edge passes through:\n%s", len(runs), rec.Trace())
	}
	// The middle class is a real stretch of the line, not a zero-length one.
	if runs[1].Points[0] == runs[1].Points[1] {
		t.Error("the middle class is drawn as a point")
	}
	for i, want := range []float32{f.Y.Map(100), f.Y.Map(200)} {
		got := runs[i].Points[len(runs[i].Points)-1].Y
		if !closef(got, want) {
			t.Errorf("corner %d is at y=%v, want the boundary at %v", i, got, want)
		}
	}
}

func TestALineComingBackDownSplitsOnTheSameThreshold(t *testing.T) {
	s := src(map[string][]float64{
		"x": {0, 10, 20},
		"y": {90, 110, 90},
	})
	cs := scale.Threshold(palette.Ramp{palette.Blue, palette.Red}, []float64{100})
	g := geom.Line(s, geom.X("x"), geom.Y("y"), geom.ColorBy("y", cs))
	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	runs := polylines(rec)
	if len(runs) != 3 {
		t.Fatalf("got %d runs, want under, over, under:\n%s", len(runs), rec.Trace())
	}
	if runs[0].Stroke.Color != runs[2].Stroke.Color {
		t.Error("the same class is drawn in two colours depending on which way the line was going")
	}
	for i := range 2 {
		got := runs[i].Points[len(runs[i].Points)-1].Y
		if want := f.Y.Map(100); !closef(got, want) {
			t.Errorf("corner %d is at y=%v, want the threshold at %v", i, got, want)
		}
	}
}

func TestALineColouredByStatusChangesAtTheRowThatChanged(t *testing.T) {
	// Nothing was measured between two rows of a state column, so the change
	// belongs on the row where the new state was first seen.
	s := data.NewTable().
		Float64("x", []float64{0, 10, 20, 30}).
		Float64("y", []float64{5, 5, 5, 5}).
		String("state", []string{"RUN", "RUN", "FAULT", "FAULT"})
	cs := scale.Named(map[string]ir.Color{"RUN": palette.Green, "FAULT": palette.Red})
	g := geom.Line(s, geom.X("x"), geom.Y("y"), geom.ColorBy("state", cs))
	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	runs := polylines(rec)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want one per state:\n%s", len(runs), rec.Trace())
	}
	if runs[0].Stroke.Color != palette.Green || runs[1].Stroke.Color != palette.Red {
		t.Errorf("runs are %v and %v, want the colours the caller named", runs[0].Stroke.Color, runs[1].Stroke.Color)
	}
	// No invented moment: the corner is the third row itself.
	corner := runs[0].Points[len(runs[0].Points)-1]
	if got, want := corner.X, f.X.Map(20); !closef(got, want) {
		t.Errorf("the state changes at x=%v, want the row at %v", got, want)
	}
	if corner != runs[1].Points[0] {
		t.Error("the two states do not share the row the change was seen on")
	}
}

func TestAContinuousRampOnALineIsRefused(t *testing.T) {
	s := src(map[string][]float64{"x": {0, 1}, "y": {0, 1}})
	g := geom.Line(s, geom.X("x"), geom.Y("y"),
		geom.ColorBy("y", scale.Sequential(palette.Viridis)))
	if err := g.Train(scale.Linear(), scale.Linear()); !errors.Is(err, geom.ErrRampOnPath) {
		t.Fatalf("Train: %v, want ErrRampOnPath", err)
	}
}

func TestAColouredLineReportsItsRowsOnce(t *testing.T) {
	// The vertices a boundary adds are not rows, and the vertex two runs
	// share must not be indexed twice.
	s := src(map[string][]float64{
		"x": {0, 10, 20},
		"y": {90, 110, 90},
	})
	cs := scale.Threshold(palette.Ramp{palette.Blue, palette.Red}, []float64{100})
	plain := geom.Line(s, geom.X("x"), geom.Y("y"))
	colored := geom.Line(s, geom.X("x"), geom.Y("y"), geom.ColorBy("y", cs))
	if got, want := marksOf(t, colored), marksOf(t, plain); got != want {
		t.Fatalf("a coloured line reports %d marks, want the %d a plain one does", got, want)
	}
}

// marksOf counts the marks a layer reports to a tracking frame.
func marksOf(t *testing.T, g geom.Geom) int {
	t.Helper()
	rec, f := frame(t, g)
	rows := &countingRows{}
	f.Rows = rows
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	return rows.n
}

type countingRows struct{ n int }

func (c *countingRows) Marks(at []ir.Point, rows []int) { c.n += len(at) }

func closef(a, b float32) bool {
	d := a - b
	return d < 0.001 && d > -0.001
}

func TestAStepColouredByStatusRisesInTheNewColour(t *testing.T) {
	// The machine changed at x=20, and the riser at x=20 is that change. It
	// belongs to the state it rises into.
	s := data.NewTable().
		Float64("x", []float64{0, 20, 40}).
		Float64("y", []float64{1, 2, 2}).
		String("state", []string{"RUN", "FAULT", "FAULT"})
	cs := scale.Named(map[string]ir.Color{"RUN": palette.Green, "FAULT": palette.Red})
	g := geom.Step(s, geom.X("x"), geom.Y("y"), geom.ColorBy("state", cs))
	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	runs := polylines(rec)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want one per state:\n%s", len(runs), rec.Trace())
	}
	if runs[0].Stroke.Color != palette.Green || runs[1].Stroke.Color != palette.Red {
		t.Fatalf("runs are %v and %v, want the colours the caller named", runs[0].Stroke.Color, runs[1].Stroke.Color)
	}
	// The green tread ends at the foot of the riser, and the red run starts
	// there and goes up.
	foot := runs[0].Points[len(runs[0].Points)-1]
	if got, want := foot.X, f.X.Map(20); !closef(got, want) {
		t.Errorf("the change is at x=%v, want %v", got, want)
	}
	if got, want := foot.Y, f.Y.Map(1); !closef(got, want) {
		t.Errorf("the change is at y=%v, want the level held until then, %v", got, want)
	}
	if runs[1].Points[0] != foot {
		t.Error("the riser is not joined to the tread before it")
	}
}

func TestAStepCrossingAThresholdSplitsOnTheRiser(t *testing.T) {
	// A staircase only moves on its risers, so that is the only place a
	// threshold can be crossed — never half way along a flat reading.
	s := src(map[string][]float64{
		"x": {0, 20, 40},
		"y": {90, 110, 110},
	})
	cs := scale.Threshold(palette.Ramp{palette.Blue, palette.Red}, []float64{100})
	g := geom.Step(s, geom.X("x"), geom.Y("y"), geom.ColorBy("y", cs))
	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	runs := polylines(rec)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want one per class:\n%s", len(runs), rec.Trace())
	}
	corner := runs[0].Points[len(runs[0].Points)-1]
	if got, want := corner.X, f.X.Map(20); !closef(got, want) {
		t.Errorf("the crossing is at x=%v, want it on the riser at %v", got, want)
	}
	if got, want := corner.Y, f.Y.Map(100); !closef(got, want) {
		t.Errorf("the crossing is at y=%v, want the threshold at %v", got, want)
	}
}

func TestAContinuousRampOnAStepIsRefused(t *testing.T) {
	s := src(map[string][]float64{"x": {0, 1}, "y": {0, 1}})
	g := geom.Step(s, geom.X("x"), geom.Y("y"),
		geom.ColorBy("y", scale.Sequential(palette.Viridis)))
	if err := g.Train(scale.Linear(), scale.Linear()); !errors.Is(err, geom.ErrRampOnPath) {
		t.Fatalf("Train: %v, want ErrRampOnPath", err)
	}
}

func TestAColouredLineKeepsItsColoursAcrossAHole(t *testing.T) {
	// Interpolate invents rows, and an invented row holds the colour value
	// before it: a category half way between two states is not a category.
	s := src(map[string][]float64{
		"x": {0, 10, 20, 30},
		"y": {90, math.NaN(), 95, 130},
	})
	cs := scale.Threshold(palette.Ramp{palette.Blue, palette.Red}, []float64{100})
	g := geom.Line(s, geom.X("x"), geom.Y("y"),
		geom.ColorBy("y", cs), geom.OnMissing(geom.Interpolate))
	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	runs := polylines(rec)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want the colour to survive the hole:\n%s", len(runs), rec.Trace())
	}
	if got, want := runs[0].Points[len(runs[0].Points)-1].Y, f.Y.Map(100); !closef(got, want) {
		t.Errorf("the crossing is at y=%v, want the threshold at %v", got, want)
	}
}

func TestAColouredStepSnapsItsStateHalfWayThroughATween(t *testing.T) {
	// A state column is a string, and a string does not interpolate — so a
	// chart half way through a transition is still drawn in the two colours
	// the caller named, never in one between them.
	a := data.NewTable().
		Float64("id", []float64{1, 2}).
		Float64("x", []float64{0, 10}).
		Float64("y", []float64{1, 1}).
		String("state", []string{"RUN", "RUN"})
	b := data.NewTable().
		Float64("id", []float64{1, 2}).
		Float64("x", []float64{0, 10}).
		Float64("y", []float64{3, 3}).
		String("state", []string{"RUN", "FAULT"})
	tw, err := data.NewTween(a, b, "id")
	if err != nil {
		t.Fatal(err)
	}
	tw.At(0.5)

	cs := scale.Named(map[string]ir.Color{"RUN": palette.Green, "FAULT": palette.Red})
	g := geom.Step(tw.Source(), geom.X("x"), geom.Y("y"), geom.ColorBy("state", cs))
	rec, f := frame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	for _, run := range polylines(rec) {
		if c := run.Stroke.Color; c != palette.Green && c != palette.Red {
			t.Errorf("a run is %v, want one of the two colours the caller named", c)
		}
	}
}
