package coord_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// The tolerance device coordinates are compared at. Everything downstream of a
// scale is ordinary float arithmetic the compiler may contract into a fused
// multiply-add on one architecture and not on another, so a coordinate is
// never compared with ==. See AGENTS.md.
const eps = 0.01

func near(t *testing.T, got, want float32, what string) {
	t.Helper()
	if math.Abs(float64(got-want)) > eps {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func linear(lo, hi float64) scale.Scale {
	s := scale.Linear(scale.Domain(lo, hi))
	return s
}

// Cartesian is the identity, and that is the whole of its contract: a mapped
// pair is the device point, an edge is a straight line, and a data rectangle
// is the four corners ir.Path.Rect writes. Every golden file in the repository
// depends on all three.
func TestCartesianIsTheIdentity(t *testing.T) {
	c := coord.Cartesian()
	if got := c.Point(3, 4); got != (ir.Point{X: 3, Y: 4}) {
		t.Errorf("Point = %v, want {3 4}", got)
	}
	if !c.Straight() {
		t.Error("Cartesian does not report straight edges")
	}
	if !c.Decimates() {
		t.Error("Cartesian does not decimate; a pixel column is its own unit")
	}

	var got, want ir.Path
	c.Area(&got, 1, 2, 3, 4)
	want.Rect(ir.R(1, 2, 3, 4))
	if len(got.Ops) != len(want.Ops) {
		t.Fatalf("Area wrote %d ops, Rect writes %d", len(got.Ops), len(want.Ops))
	}
	for i := range got.Ops {
		if got.Ops[i] != want.Ops[i] {
			t.Fatalf("op %d = %v, want %v", i, got.Ops[i], want.Ops[i])
		}
	}
	for i := range got.Pts {
		if got.Pts[i] != want.Pts[i] {
			t.Errorf("point %d = %v, want %v", i, got.Pts[i], want.Pts[i])
		}
	}
}

func TestCartesianFramesTheRectangleWithYFlipped(t *testing.T) {
	x, y := linear(0, 10), linear(0, 10)
	c := coord.Cartesian().Frame(ir.R(20, 30, 120, 230), x, y)
	near(t, x.Map(0), 20, "x lo")
	near(t, x.Map(10), 120, "x hi")
	// Larger values are higher on screen, which is a smaller device Y.
	near(t, y.Map(0), 230, "y lo")
	near(t, y.Map(10), 30, "y hi")

	x0, x1, y0, y1 := c.Extent()
	near(t, x0, 20, "extent x0")
	near(t, x1, 120, "extent x1")
	near(t, y0, 230, "extent y0")
	near(t, y1, 30, "extent y1")
}

func TestPointsIsPointInBatch(t *testing.T) {
	for _, c := range []coord.Coord{
		coord.Cartesian().Frame(ir.R(0, 0, 100, 100), linear(0, 1), linear(0, 1)),
		coord.Polar().Frame(ir.R(0, 0, 200, 200), linear(0, 1), linear(0, 1)),
	} {
		xs := []float32{0, 1, 2, 3}
		ys := []float32{10, 20, 30, 40}
		got := c.Points(nil, xs, ys)
		if len(got) != len(xs) {
			t.Fatalf("%T: Points returned %d of %d", c, len(got), len(xs))
		}
		for i := range xs {
			want := c.Point(xs[i], ys[i])
			if got[i] != want {
				t.Errorf("%T: Points[%d] = %v, Point = %v", c, i, got[i], want)
			}
		}
	}
}

// Points appends, so a caller reusing a buffer between frames gets its own
// memory back rather than a new allocation. That is the property the
// allocation gate rests on.
func TestPointsAppendsIntoTheCallersBuffer(t *testing.T) {
	c := coord.Polar().Frame(ir.R(0, 0, 100, 100), linear(0, 1), linear(0, 1))
	buf := make([]ir.Point, 0, 8)
	xs, ys := []float32{1, 2, 3}, []float32{4, 5, 6}
	got := c.Points(buf, xs, ys)
	if &got[0] != &buf[:1][0] {
		t.Error("Points allocated instead of filling the buffer it was given")
	}
}

func TestPolarPlacesAnglesClockwiseFromNoon(t *testing.T) {
	x, y := linear(0, 4), linear(0, 1)
	c := coord.Polar().Frame(ir.R(0, 0, 200, 200), x, y)
	// The X scale sweeps the circle: a quarter of its domain is a quarter turn.
	// The Y scale is the radius, and 1 is the rim.
	for _, tc := range []struct {
		at    float64
		wantX float32
		wantY float32
		what  string
	}{
		{0, 100, 10, "noon"},
		{1, 190, 100, "three o'clock"},
		{2, 100, 190, "six o'clock"},
		{3, 10, 100, "nine o'clock"},
	} {
		got := c.Point(x.Map(tc.at), y.Map(1))
		near(t, got.X, tc.wantX, tc.what+" x")
		near(t, got.Y, tc.wantY, tc.what+" y")
	}
}

func TestPolarInvertsBackToTheMappedPair(t *testing.T) {
	x, y := linear(0, 100), linear(0, 10)
	c := coord.Polar().Frame(ir.R(0, 0, 300, 240), x, y)
	for _, v := range []struct{ vx, vy float64 }{{0, 10}, {25, 5}, {60, 8}, {99, 1}} {
		mx, my := x.Map(v.vx), y.Map(v.vy)
		gx, gy := c.Invert(c.Point(mx, my))
		near(t, gx, mx, "inverted angle")
		near(t, gy, my, "inverted radius")
		// And through the scales, which is what a tooltip actually asks.
		if got := x.Invert(gx); math.Abs(got-v.vx) > 1e-2 {
			t.Errorf("x = %v, want %v", got, v.vx)
		}
		if got := y.Invert(gy); math.Abs(got-v.vy) > 1e-2 {
			t.Errorf("y = %v, want %v", got, v.vy)
		}
	}
}

// ADR 0002 froze the IR's path verbs on the claim that every curve a chart
// needs is expressible as cubics, and an arc is the first serious test of it.
// The control-point rule k = (4/3)·tan(φ/4) is exactly internal/markers'
// kappa at a quarter turn — in real arithmetic. By the time math.Tan has
// evaluated it, it is float arithmetic, so the two are compared at a tolerance
// rather than with ==.
func TestAQuarterArcIsTheKappaCircle(t *testing.T) {
	const kappa = 0.5522847498307936
	if got := 4.0 / 3.0 * math.Tan(math.Pi/2/4); math.Abs(got-kappa) > 3e-16 {
		t.Errorf("(4/3)tan(pi/8) = %.17g, kappa = %.17g", got, kappa)
	}

	// And the arc the coord draws stays on the circle it claims to be.
	x, y := linear(0, 1), linear(0, 1)
	c := coord.Polar().Frame(ir.R(0, 0, 200, 200), x, y)
	var p ir.Path
	c.Area(&p, x.Map(0), y.Map(1), x.Map(1), y.Map(1))
	r := float64(90) // half of 200, times the default 0.9 radius fraction
	for _, q := range p.Pts {
		d := math.Hypot(float64(q.X-100), float64(q.Y-100))
		if d > r*1.15 {
			t.Errorf("a control point %0.2f from the centre bulges past the %0.2f rim", d, r)
		}
	}
}

// A ring closes on itself, and there is no seam at twelve o'clock. The
// stacking adjustment gives each slice a bound from a running total rather
// than by accumulating deltas, so the last slice ends at exactly the angle the
// first began at — and the coord has to put equal angles in equal places for
// that to be worth anything.
func TestARingHasNoSeam(t *testing.T) {
	x, y := linear(0, 100), linear(0, 1)
	c := coord.Polar().Frame(ir.R(0, 0, 200, 200), x, y)

	start := c.Point(x.Map(0), y.Map(1))
	wrap := c.Point(x.Map(100), y.Map(1))
	near(t, wrap.X, start.X, "the seam x")
	near(t, wrap.Y, start.Y, "the seam y")

	// And the sector that sweeps the whole circle comes back to where it began
	// rather than a rounding error short of it. The wedge's last point is the
	// centre it closes through, so the arc's own end is the one before it.
	var p ir.Path
	c.Area(&p, x.Map(0), y.Map(0), x.Map(100), y.Map(1))
	arcEnd := p.Pts[len(p.Pts)-2]
	near(t, arcEnd.X, p.Pts[0].X, "the arc's end x")
	near(t, arcEnd.Y, p.Pts[0].Y, "the arc's end y")
}

// A polar coord reports that it does not decimate: a bucket of equal angle is
// not a bucket of equal width, so a reduction defined over pixel columns would
// be measuring the wrong thing. See ADR 0018, property 4.
func TestPolarDoesNotDecimate(t *testing.T) {
	if coord.Polar().Decimates() {
		t.Error("a polar coord claims a pixel column is its own unit")
	}
}

func TestChordAndArcAreAPolicy(t *testing.T) {
	if !coord.Polar(coord.Chord()).Straight() {
		t.Error("coord.Chord did not ask for straight edges")
	}
	if coord.Polar(coord.Arc()).Straight() {
		t.Error("coord.Arc reports straight edges")
	}
	if coord.Polar().Straight() {
		t.Error("the default edge is an arc, not a chord")
	}
}

func TestHoleIsAnAnnulus(t *testing.T) {
	x, y := linear(0, 1), linear(0, 1)
	c := coord.Polar(coord.Hole(0.5), coord.Radius(1)).Frame(ir.R(0, 0, 200, 200), x, y)
	// The radial scale starts at the inner radius, so nothing a geom draws
	// enters the hole — the hole is where the scale is not.
	inner := c.Point(x.Map(0), y.Map(0))
	near(t, float32(math.Hypot(float64(inner.X-100), float64(inner.Y-100))), 50, "the inner radius")
	outer := c.Point(x.Map(0), y.Map(1))
	near(t, float32(math.Hypot(float64(outer.X-100), float64(outer.Y-100))), 100, "the outer radius")
}

func TestCartesianFurnitureIsTheStraightRunsItAlwaysWas(t *testing.T) {
	x, y := linear(0, 10), linear(0, 10)
	area := ir.R(50, 20, 250, 220)
	c := coord.Cartesian().Frame(area, x, y)
	var f coord.Furniture
	m := coord.Metrics{TickLen: 5, MinorTickLen: 3, LabelPad: 4}
	c.Furniture(&f, area, m, x.Ticks(5), y.Ticks(5))

	if !f.XLabelsShareARow {
		t.Error("a Cartesian axis writes its labels along one row and must say so")
	}
	if len(f.AxisX.Pts) != 2 || !f.AxisX.Path.Empty() {
		t.Errorf("the X axis line is %d points and a path of %d ops, want a two-point run",
			len(f.AxisX.Pts), len(f.AxisX.Path.Ops))
	}
	for i, g := range f.GridX {
		if g.Empty() {
			continue
		}
		if len(g.Pts) != 2 {
			t.Errorf("grid line %d is not a two-point polyline: %v", i, g.Pts)
		}
		near(t, g.Pts[0].Y, area.Min.Y, "grid top")
		near(t, g.Pts[1].Y, area.Max.Y, "grid bottom")
	}
}

func TestPolarFurnitureIsRingsAndSpokes(t *testing.T) {
	x, y := linear(0, 4), linear(0, 10)
	area := ir.R(0, 0, 200, 200)
	c := coord.Polar().Frame(area, x, y)
	var f coord.Furniture
	c.Furniture(&f, area, coord.Metrics{TickLen: 5, MinorTickLen: 3, LabelPad: 4}, x.Ticks(5), y.Ticks(5))

	if f.XLabelsShareARow {
		t.Error("labels round a ring do not share a row and must not be culled as though they did")
	}
	// The angular axis is the rim, which is a curve, and its grid lines are
	// spokes, which are not.
	if f.AxisX.Path.Empty() {
		t.Error("the angular axis is not a ring")
	}
	for i, g := range f.GridX {
		if !g.Empty() && len(g.Pts) != 2 {
			t.Errorf("spoke %d is not a straight run: %v", i, g.Pts)
		}
	}
	// The radial axis's grid lines are the concentric rings.
	rings := 0
	for _, g := range f.GridY {
		if !g.Path.Empty() {
			rings++
		}
	}
	if rings == 0 {
		t.Error("a polar coord reported no concentric rings")
	}
}

// Reset keeps every buffer, including those of the shapes past the current
// length — otherwise the next frame hands a caller a shape still holding the
// last frame's points.
func TestFurnitureResetKeepsItsBuffersAndForgetsItsPoints(t *testing.T) {
	x, y := linear(0, 10), linear(0, 10)
	area := ir.R(0, 0, 200, 200)
	c := coord.Cartesian().Frame(area, x, y)
	m := coord.Metrics{TickLen: 5, MinorTickLen: 3, LabelPad: 4}

	var f coord.Furniture
	c.Furniture(&f, area, m, x.Ticks(8), y.Ticks(8))
	gridCap := cap(f.GridX)
	f.Reset()
	if cap(f.GridX) != gridCap {
		t.Errorf("Reset gave up the grid buffer: cap %d, was %d", cap(f.GridX), gridCap)
	}
	// One tick this time: the shapes behind it must come back empty.
	c.Furniture(&f, area, m, x.Ticks(2)[:1], nil)
	full := f.GridX[:cap(f.GridX)]
	for i := 1; i < len(full); i++ {
		if !full[i].Empty() {
			t.Fatalf("shape %d still holds last frame's points: %v", i, full[i].Pts)
		}
	}
}

func TestACoordRoundTripsThroughItsDescription(t *testing.T) {
	for _, want := range []coord.Coord{
		coord.Cartesian(),
		coord.Polar(),
		coord.Polar(coord.Theta(coord.FromY), coord.Hole(0.4), coord.Radius(0.75),
			coord.Start(math.Pi/6), coord.Sweep(math.Pi), coord.Counterclockwise(true), coord.Chord()),
	} {
		d, ok := coord.Describe(want)
		if !ok {
			t.Fatalf("%T cannot describe itself", want)
		}
		got, err := coord.FromDesc(d)
		if err != nil {
			t.Fatalf("FromDesc: %v", err)
		}
		back, _ := coord.Describe(got)
		if back != d {
			t.Errorf("round trip changed the coord:\n got %+v\nwant %+v", back, d)
		}
	}
}

func TestAnUnknownCoordTypeIsAnError(t *testing.T) {
	if _, err := coord.FromDesc(coord.Desc{Type: "hyperbolic"}); err == nil {
		t.Error("FromDesc accepted a coord type it cannot build")
	}
}
