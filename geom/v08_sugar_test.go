package geom_test

import (
	"math"
	"testing"
	"time"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// The v0.8 sugar, on the geom side. Two things a donut could not say before,
// and neither of them is a new mark: a slice's inner and outer radius are
// columns like any other, and a slice can be broken out of the ring it belongs
// to.

// slices is a donut's table: a constant share so that the four slices are
// quarters, and a radial pair per slice.
func slices() data.Source {
	return data.Float64Columns(map[string][]float64{
		"share": {1, 1, 1, 1},
		"floor": {0.2, 0.2, 0.2, 0.2},
		"reach": {0.5, 0.7, 0.9, 1.0},
		"pull":  {0, 0.1, 0, 0},
	})
}

// radii reports how far the path's on-curve points are from the middle of the
// panel.
//
// On-curve rather than every point: an arc is cubics, and the control points
// of one sit outside the circle it draws — measuring those would report a
// slice reaching further than its own rim.
func radii(p *ir.Path, cx, cy float32) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	p.Walk(func(op ir.PathOp, pts []ir.Point) {
		if len(pts) == 0 {
			return
		}
		q := pts[len(pts)-1]
		d := math.Hypot(float64(q.X-cx), float64(q.Y-cy))
		lo, hi = math.Min(lo, d), math.Max(hi, d)
	})
	return lo, hi
}

// A row that names both of its radial edges gets exactly those. That is what
// makes the inner and the outer radius dimensions of the data rather than a
// slot the coord chose: the same layer that draws a ring draws slices of four
// different lengths, from one more column.
func TestASlicesRadiiComeFromTwoColumns(t *testing.T) {
	g := geom.Bar(slices(),
		geom.X("floor"), geom.X2("reach"), geom.Y("share"),
		geom.GroupBy("share"))
	x, y := scale.Linear(scale.Domain(0, 1)), scale.Linear(scale.Domain(0, 4))
	rec, f := polarFrame(t, g, x, y, coord.Polar(coord.Theta(coord.FromY), coord.Radius(1)), 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	calls := rec.Filter("FillPath")
	if len(calls) != 1 {
		t.Fatalf("%d fill calls, want the one the layer batches into", len(calls))
	}
	// One subpath per slice, in row order, so each can be pointed at — and so
	// each can be measured here.
	subs := subpaths(t, calls[0].Path)
	if len(subs) != 4 {
		t.Fatalf("%d slices, want 4", len(subs))
	}
	cx, cy := f.Area.Dx()/2, f.Area.Dy()/2
	for i, sub := range subs {
		lo, hi := radii(sub, cx, cy)
		// The radial scale maps into the circle, so the row's values are the
		// distances themselves: the panel is 200 wide and the radius fills it.
		wantLo := float64(f.X.Map(0.2))
		wantHi := float64(f.X.Map([]float64{0.5, 0.7, 0.9, 1.0}[i]))
		if math.Abs(lo-wantLo) > 0.05 || math.Abs(hi-wantHi) > 0.05 {
			t.Errorf("slice %d reaches from %.2f to %.2f, want %.2f to %.2f",
				i, lo, hi, wantLo, wantHi)
		}
	}
}

// Under Cartesian the same pair is the bar's two edges, which is the reading
// [geom.Rect] already had. A bar that names both of them is not widened either:
// its width is in the domain already.
func TestABarThatNamesBothEdgesGetsThem(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{
		"lo": {1, 4}, "hi": {2, 6}, "y": {10, 20},
	})
	g := geom.Bar(src, geom.X("lo"), geom.X2("hi"), geom.Y("y"))
	x, y := scale.Linear(), scale.Linear()
	rec, f := frameOn(t, g, x, y, 200, 100)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if lo, hi := x.Domain(); lo != 1 || hi != 6 {
		t.Errorf("the X domain is [%v, %v], want the two columns' extent [1, 6]", lo, hi)
	}
	rects := rectsOf(t, rec.Filter("FillPath")[0])
	if len(rects) != 2 {
		t.Fatalf("%d bars, want 2", len(rects))
	}
	for i, want := range [][2]float64{{1, 2}, {4, 6}} {
		near(t, rects[i].Min.X, f.X.Map(want[0]), "the bar's near edge")
		near(t, rects[i].Max.X, f.X.Map(want[1]), "the bar's far edge")
	}
}

// A break-out moves the mark and changes nothing else about it. The slice
// keeps the angle and the radii the data gave it — which is what separates
// this from growing a slice, and what keeps an exploded donut honest.
func TestABrokenOutSliceIsTheSameSliceMoved(t *testing.T) {
	build := func(opts ...geom.Option) []*ir.Path {
		t.Helper()
		g := geom.Bar(slices(), append([]geom.Option{
			geom.X("floor"), geom.X2("reach"), geom.Y("share"), geom.GroupBy("share"),
		}, opts...)...)
		x, y := scale.Linear(scale.Domain(0, 1)), scale.Linear(scale.Domain(0, 4))
		rec, f := polarFrame(t, g, x, y, coord.Polar(coord.Theta(coord.FromY), coord.Radius(1)), 200)
		if err := g.Build(rec, f); err != nil {
			t.Fatal(err)
		}
		return subpaths(t, rec.Filter("FillPath")[0].Path)
	}

	still := build()
	moved := build(geom.ExplodeBy("pull"))
	if len(still) != len(moved) {
		t.Fatalf("%d slices became %d", len(still), len(moved))
	}
	// The second row is pulled out by a tenth of the outer radius, along the
	// bisector of the slice from a quarter turn to a half: south-east, at 135
	// degrees from noon.
	s, c := math.Sincos(3 * math.Pi / 4)
	want := ir.Point{X: float32(0.1 * 100 * s), Y: float32(-0.1 * 100 * c)}
	for i := range still {
		d := ir.Point{}
		if i == 1 {
			d = want
		}
		for j := range still[i].Pts {
			got := moved[i].Pts[j]
			near(t, got.X-still[i].Pts[j].X, d.X, "slice's move in x")
			near(t, got.Y-still[i].Pts[j].Y, d.Y, "slice's move in y")
		}
	}
}

// A constant break-out moves every mark, a column moves the rows it names, and
// a row the column has no number for stays where it is.
func TestABreakOutIsPerLayerOrPerRow(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{
		"share": {1, 1},
		"all":   {0, 0},
		"pull":  {0.1, math.NaN()},
	})
	moved := func(opts ...geom.Option) []ir.Point {
		t.Helper()
		g := geom.Bar(src, append([]geom.Option{
			geom.X("all"), geom.Y("share"), geom.GroupBy("share"),
		}, opts...)...)
		x, y := scale.Linear(scale.Domain(0, 1)), scale.Linear(scale.Domain(0, 2))
		rec, f := polarFrame(t, g, x, y, coord.Polar(coord.Theta(coord.FromY), coord.Radius(1)), 200)
		if err := g.Build(rec, f); err != nil {
			t.Fatal(err)
		}
		var out []ir.Point
		for _, sub := range subpaths(t, rec.Filter("FillPath")[0].Path) {
			out = append(out, sub.Pts[0])
		}
		return out
	}

	still, layer, rows := moved(), moved(geom.Explode(0.1)), moved(geom.ExplodeBy("pull"))
	for i := range still {
		if layer[i] == still[i] {
			t.Errorf("slice %d did not move, and the whole layer was broken out", i)
		}
	}
	if rows[0] == still[0] {
		t.Error("the row the column named did not move")
	}
	if rows[1] != still[1] {
		t.Errorf("the row with no number moved to %v from %v", rows[1], still[1])
	}
}

// A coord with no middle ignores a break-out rather than inventing a direction
// to move in. That is what keeps every Cartesian golden file in the repository
// unchanged by an option every geom accepts.
func TestACartesianMarkIsNotBrokenOut(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {1, 2}})
	draw := func(opts ...geom.Option) *ir.Path {
		t.Helper()
		g := geom.Bar(src, append([]geom.Option{geom.X("x"), geom.Y("y")}, opts...)...)
		rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 200, 100)
		if err := g.Build(rec, f); err != nil {
			t.Fatal(err)
		}
		return rec.Filter("FillPath")[0].Path
	}
	still, asked := draw(), draw(geom.Explode(0.5))
	if len(still.Pts) != len(asked.Pts) {
		t.Fatalf("%d points became %d", len(still.Pts), len(asked.Pts))
	}
	for i := range still.Pts {
		if still.Pts[i] != asked.Pts[i] {
			t.Fatalf("point %d moved to %v from %v", i, asked.Pts[i], still.Pts[i])
		}
	}
}

// ragged is a Source whose columns disagree about how many rows there are.
// The built-in tables refuse to be built that way, so a layer's own length
// check needs one that does not — a Source is an interface, and a caller's own
// implementation is free to hand back whatever it holds.
type ragged struct{ cols map[string][]float64 }

func (r ragged) Len() int { return len(r.cols["x"]) }

func (r ragged) Columns() []string { return []string{"x", "y", "short"} }

func (r ragged) Float64Column(name string) ([]float64, bool) {
	v, ok := r.cols[name]
	return v, ok
}

func (ragged) TimeColumn(string) ([]time.Time, bool) { return nil, false }

func (ragged) StringColumn(string) ([]string, bool) { return nil, false }

// A column of the wrong length is an error at Train, where every other
// mismatched column is caught, rather than a panic at Build.
func TestAMismatchedSugarColumnIsAnError(t *testing.T) {
	src := ragged{cols: map[string][]float64{
		"x": {0, 1}, "y": {1, 2}, "short": {1},
	}}
	for _, tc := range []struct {
		what string
		opt  geom.Option
	}{
		{"a second radius", geom.X2("short")},
		{"a break-out", geom.ExplodeBy("short")},
	} {
		g := geom.Bar(src, geom.X("x"), geom.Y("y"), tc.opt)
		if err := g.Train(scale.Linear(), scale.Linear()); err == nil {
			t.Errorf("%s of the wrong length trained without complaint", tc.what)
		}
	}
}

// subpaths cuts a path into the subpaths it holds: one per mark, which is what
// makes each mark separately pointable and each of them measurable here.
func subpaths(t *testing.T, p *ir.Path) []*ir.Path {
	t.Helper()
	if p == nil {
		t.Fatal("a fill call carried no path")
	}
	var out []*ir.Path
	var cur *ir.Path
	p.Walk(func(op ir.PathOp, pts []ir.Point) {
		if op == ir.OpMoveTo {
			cur = &ir.Path{}
			out = append(out, cur)
		}
		if cur != nil {
			cur.Ops = append(cur.Ops, op)
			cur.Pts = append(cur.Pts, pts...)
		}
	})
	return out
}

func near(t *testing.T, got, want float32, what string) {
	t.Helper()
	if math.Abs(float64(got-want)) > 0.05 {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}
