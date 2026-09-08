package geom_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

func TestQQTrainsOnQuantilesAndKeepsTheSample(t *testing.T) {
	vs := []float64{7, 1, 3}
	g := geom.QQ(data.NewTable().Float64("v", vs), geom.X("v"))
	f := textFrame(t, g, ir.R(0, 0, 100, 100))
	lo, hi := f.X.Domain()
	if math.Abs(lo+0.967421566101701) > 1e-10 || math.Abs(hi-0.967421566101701) > 1e-10 {
		t.Fatalf("theoretical domain %v..%v", lo, hi)
	}
	lo, hi = f.Y.Domain()
	if lo != 1 || hi != 7 {
		t.Fatalf("observed domain %v..%v", lo, hi)
	}
	calls := draw(t, f, g).Filter("Markers")
	if len(calls) != 1 || len(calls[0].Points) != 3 {
		t.Fatalf("marks: %v", calls)
	}
	want := []ir.Point{{X: 0, Y: 100}, {X: 50, Y: f.Y.Map(3)}, {X: 100, Y: 0}}
	for i, p := range calls[0].Points {
		if !samePoint(p, want[i]) {
			t.Errorf("point %d: %v, want %v", i, p, want[i])
		}
	}
	if vs[0] != 7 || vs[1] != 1 || vs[2] != 3 {
		t.Fatal("sample was sorted in place")
	}
}

func TestQQGroupsFacetsNullsAndLogHoles(t *testing.T) {
	src := data.NewTable().Float64("v", []float64{0, 1, 4, 10, 20, 30}).String("group", []string{"a", "a", "a", "b", "b", "b"}).WithNulls("v", []bool{false, false, false, false, true, false})
	g := geom.QQ(src, geom.X("v"), geom.GroupBy("group"))
	x, y := scale.Linear(), scale.Log()
	if err := g.Train(x, y); err != nil {
		t.Fatal(err)
	}
	x.SetRange(0, 100)
	y.SetRange(100, 0)
	f := geom.Frame{Area: ir.R(0, 0, 100, 100), X: x, Y: y}
	calls := draw(t, f, g).Filter("Markers")
	if len(calls) != 2 || len(calls[0].Points) != 2 || len(calls[1].Points) != 2 {
		t.Fatalf("groups or missing rows lost: %v", calls)
	}
	// Zero cannot be drawn on log Y, but remains the first ranked observation.
	if math.Abs(float64(calls[0].Points[0].X-50)) > 0.01 {
		t.Fatal("log display changed the ranks")
	}
	cut := g.(geom.Faceter).Subset([]int{3, 5})
	cf := textFrame(t, cut, ir.R(0, 0, 100, 100))
	if n := len(draw(t, cf, cut).Filter("Markers")); n != 1 {
		t.Fatalf("facet has %d series", n)
	}
	bad := geom.QQ(src, geom.X("v"), geom.OnMissing(geom.Error))
	if err := bad.Train(scale.Linear(), scale.Linear()); err == nil {
		t.Fatal("Error policy accepted null")
	}
}

func TestQQRefusesCategoricalAxes(t *testing.T) {
	g := geom.QQ(data.NewTable().Float64("v", []float64{1}), geom.X("v"))
	if err := g.Train(scale.Ordinal(), scale.Linear()); err == nil {
		t.Fatal("accepted categorical theoretical axis")
	}
}
