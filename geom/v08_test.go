package geom_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// What v0.8 changed inside a geom: a mapped pair is no longer a device point,
// an edge is no longer a straight line, and a rectangle in data space is no
// longer a rectangle on screen. Nothing else about a geom moved, which is the
// claim these tests exist to hold.

// polarFrame builds a frame in a polar coord, framed the way render would.
func polarFrame(t *testing.T, g geom.Geom, x, y scale.Scale, c coord.Coord, size float32) (*irtest.Recorder, geom.Frame) {
	t.Helper()
	if err := g.Train(x, y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	area := ir.R(0, 0, size, size)
	cd := c.Frame(area, x, y)
	return irtest.New(), geom.Frame{Area: area, X: x, Y: y, Coord: cd, Theme: theme.Light}
}

// A frame that names no coord draws what it always drew. That is what lets a
// caller driving a geom directly — and every test written before v0.8 — keep
// working unchanged.
func TestAFrameWithNoCoordIsCartesian(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{"x": {0, 1, 2}, "y": {0, 1, 2}})
	g := geom.Scatter(src, geom.X("x"), geom.Y("y"))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 100, 100)
	if f.Coord != nil {
		t.Fatal("frameOn set a coord")
	}
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	pts := rec.Filter("Markers")[0].Points
	for i, p := range pts {
		want := ir.Point{X: f.X.Map(float64(i)), Y: f.Y.Map(float64(i))}
		if p != want {
			t.Errorf("marker %d at %v, want the mapped pair %v", i, p, want)
		}
	}
}

// A bar's rectangle becomes whatever the coord makes of it: four corners under
// Cartesian, an annular sector under Polar. The layer is the same layer.
func TestABarsRectangleGoesThroughTheCoord(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{"x": {0}, "y": {1}})

	cart := geom.Bar(src, geom.X("x"), geom.Y("y"))
	rec, f := frameOn(t, cart, scale.Linear(), scale.Linear(), 200, 200)
	if err := cart.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	fill := rec.Filter("FillPath")[0]
	if got := fill.Path.Ops; !sameOps(got, []ir.PathOp{
		ir.OpMoveTo, ir.OpLineTo, ir.OpLineTo, ir.OpLineTo, ir.OpClose,
	}) {
		t.Errorf("a Cartesian bar is %v, want the four corners it always was", got)
	}

	pol := geom.Bar(src, geom.X("x"), geom.Y("y"))
	rec, f = polarFrame(t, pol, scale.Linear(), scale.Linear(), coord.Polar(), 200)
	if err := pol.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if ops := rec.Filter("FillPath")[0].Path.Ops; !hasCubic(ops) {
		t.Errorf("a polar bar is %v, want an arc in it", ops)
	}
}

// A polar coord reports that it does not decimate, and the layer believes it.
// A bucket of equal angle is not a bucket of equal width, so a reduction
// defined over pixel columns would be measuring the wrong thing.
func TestAPolarLayerKeepsEveryRow(t *testing.T) {
	const n = 5000
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = float64(i)
		y[i] = math.Sin(float64(i) / 37)
	}
	src := data.Float64Columns(map[string][]float64{"x": x, "y": y})

	g := geom.Line(src, geom.X("x"), geom.Y("y"))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 200, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if got := len(rec.Filter("Polyline")[0].Points); got >= n {
		t.Errorf("a Cartesian line drew %d of %d rows; it should have reduced", got, n)
	}

	g = geom.Line(src, geom.X("x"), geom.Y("y"))
	rec, f = polarFrame(t, g, scale.Linear(), scale.Linear(), coord.Polar(coord.Chord()), 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if got := len(rec.Filter("Polyline")[0].Points); got != n {
		t.Errorf("a polar line drew %d of %d rows", got, n)
	}
}

// A staircase's corner is a statement about the data — the value held until
// here and then changed — so it is placed before the coord rather than after.
// Under a polar coord the two are not the same point.
func TestAStaircaseTurnsWhereTheDataDoes(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {1, 2}})
	g := geom.Step(src, geom.X("x"), geom.Y("y"))
	x, y := scale.Linear(), scale.Linear()
	rec, f := polarFrame(t, g, x, y, coord.Polar(coord.Chord()), 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	pts := rec.Filter("Polyline")[0].Points
	if len(pts) != 3 {
		t.Fatalf("%d points, want a two-row staircase", len(pts))
	}
	// StepPost holds the first row's value until the second row's position, so
	// the corner is at the second angle and the first radius — a point on the
	// circle, not the midpoint of two device positions.
	want := f.Coord.Point(x.Map(1), y.Map(1))
	if d := math.Hypot(float64(pts[1].X-want.X), float64(pts[1].Y-want.Y)); d > 0.01 {
		t.Errorf("the corner is at %v, want %v — it was placed after the transform", pts[1], want)
	}
}

// A closed contour is what makes a radar a shape rather than an open line with
// a gap in it, and it works in either coord: it is a fact about the series.
func TestClosedJoinsTheLastMarkBackToTheFirst(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{
		"x": {0, 1, 2},
		"y": {0, 2, 1},
	})
	g := geom.Line(src, geom.X("x"), geom.Y("y"), geom.Closed(true))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 100, 100)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	// Closing turns the run into a path: a polyline cannot say "and back".
	if n := rec.Count("Polyline"); n != 0 {
		t.Errorf("a closed contour drew %d polylines", n)
	}
	p := rec.Filter("StrokePath")[0].Path
	if p.Ops[len(p.Ops)-1] != ir.OpClose {
		t.Errorf("the contour is %v, want it closed", p.Ops)
	}
	if p.Pts[0] != p.Pts[len(p.Pts)-1] {
		t.Errorf("the closing edge runs to %v, want the first vertex %v",
			p.Pts[len(p.Pts)-1], p.Pts[0])
	}
}

// A rule spans the other axis end to end, and where that axis ends is what the
// coord decided when it framed the panel — the edge of the rectangle under
// Cartesian, and a whole turn of the circle under Polar, where a rule at a
// constant Y is a ring.
func TestARuleUnderAPolarCoordIsARing(t *testing.T) {
	g := geom.HLine(1)
	x, y := scale.Linear(scale.Domain(0, 4)), scale.Linear(scale.Domain(0, 2))
	rec, f := polarFrame(t, g, x, y, coord.Polar(), 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if n := rec.Count("Polyline"); n != 0 {
		t.Fatalf("a polar rule drew %d polylines, want an arc", n)
	}
	p := rec.Filter("StrokePath")[0].Path
	if !hasCubic(p.Ops) {
		t.Errorf("a polar rule is %v, want the cubics of a ring", p.Ops)
	}
	// Every point of a ring at a constant radius is that far from the centre,
	// give or take the bulge of a control point.
	r := float64(y.Map(1) - 0)
	for _, q := range p.Pts {
		d := math.Hypot(float64(q.X-100), float64(q.Y-100))
		if d < r*0.99 || d > r*1.16 {
			t.Fatalf("a point %.2f from the centre is not on the ring at %.2f", d, r)
		}
	}
}

func sameOps(got, want []ir.PathOp) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func hasCubic(ops []ir.PathOp) bool {
	for _, op := range ops {
		if op == ir.OpCubicTo {
			return true
		}
	}
	return false
}
