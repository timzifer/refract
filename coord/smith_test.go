package coord_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// The disc every test below is drawn in: a 200×200 panel, so the centre is
// (100, 100) and — with the radius option turned up to fill the panel — the rim
// is 100 away from it. Choosing round numbers here is what lets a test say
// "the left rim" and mean (0, 100).
var smithArea = ir.Rect{Min: ir.Point{X: 0, Y: 0}, Max: ir.Point{X: 200, Y: 200}}

const (
	smithCX, smithCY = float32(100), float32(100)
	smithR           = float32(100)
)

// smithChart frames a Smith coord filling the panel, with the canonical grid on
// both axes.
func smithChart(opts ...coord.SmithOption) (coord.Coord, scale.Scale, scale.Scale) {
	x := scale.Linear(scale.Domain(0, 20), scale.TickValues(0, 0.2, 0.5, 1, 2, 5))
	y := scale.Linear(scale.Domain(-20, 20), scale.TickValues(-5, -2, -1, -0.5, -0.2, 0, 0.2, 0.5, 1, 2, 5))
	c := coord.Smith(append([]coord.SmithOption{coord.SmithRadius(1)}, opts...)...)
	return c.Frame(smithArea, x, y), x, y
}

// gammaOf reads a device point back as the reflection coefficient it stands
// for, which is what "inside the disc" and "on the rim" are said in.
func gammaOf(p ir.Point) (re, im float64) {
	return float64(p.X-smithCX) / float64(smithR), float64(smithCY-p.Y) / float64(smithR)
}

// The three points a Smith chart is read from: a short at the left rim, a
// matched load in the middle, an open at the right rim. Every other reading is
// relative to these, so if they move the chart is wrong whatever else is right.
func TestSmithPlacesTheShortTheMatchAndTheOpen(t *testing.T) {
	c, _, _ := smithChart()
	for _, tc := range []struct {
		name   string
		r, x   float32
		wx, wy float32
	}{
		{"a short, z = 0", 0, 0, smithCX - smithR, smithCY},
		{"a matched load, z = 1", 1, 0, smithCX, smithCY},
		{"an open, z → ∞", 1e9, 0, smithCX + smithR, smithCY},
		{"a unit inductance, z = j", 0, 1, smithCX, smithCY - smithR},
		{"a unit capacitance, z = −j", 0, -1, smithCX, smithCY + smithR},
	} {
		got := c.Point(tc.r, tc.x)
		near(t, got.X, tc.wx, tc.name+": x")
		near(t, got.Y, tc.wy, tc.name+": y")
	}
}

// Every passive impedance is inside the disc, and an active one — a reflection
// of more than unity — is outside it. That is the whole claim the picture makes.
func TestEveryPassiveImpedanceIsInsideTheDisc(t *testing.T) {
	c, _, _ := smithChart()
	for _, r := range []float64{0, 0.01, 0.5, 1, 3, 50, 1e6} {
		for _, x := range []float64{-1e6, -7, -1, -0.1, 0, 0.1, 1, 7, 1e6} {
			g := math.Hypot(gammaOf(c.Point(float32(r), float32(x))))
			if g > 1+eps {
				t.Errorf("z = %v%+vj is placed at |Γ| = %v, outside the disc", r, x, g)
			}
		}
	}
	if g := math.Hypot(gammaOf(c.Point(-0.5, 0))); g <= 1 {
		t.Errorf("a negative resistance is placed at |Γ| = %v, inside the disc", g)
	}
}

// Invert is what a tooltip reads, and under this coord the pair it hands back
// is the impedance itself — see interact.Index.Hit, which inverts through the
// coord and then through the scales.
func TestSmithInvertsBackToTheImpedance(t *testing.T) {
	c, _, _ := smithChart()
	for _, r := range []float64{0, 0.2, 1, 4, 25} {
		for _, x := range []float64{-9, -1, -0.3, 0, 0.3, 1, 9} {
			gotR, gotX := c.Invert(c.Point(float32(r), float32(x)))
			// The tolerance is relative: the map crowds a large impedance into
			// the last pixels before the rim, so a device point there carries
			// fewer digits of it back than one near the middle does.
			if math.Abs(float64(gotR)-r) > eps*math.Max(1, r) || math.Abs(float64(gotX)-x) > eps*math.Max(1, math.Abs(x)) {
				t.Errorf("z = %v%+vj came back as %v%+vj", r, x, gotR, gotX)
			}
		}
	}
}

// SmithZ is the coord's own inverse in data terms, and a chart is only honest
// if the two agree: a column converted with SmithZ has to land where a tooltip
// says it is.
func TestSmithZIsTheInverseOfThePlacement(t *testing.T) {
	c, _, _ := smithChart()
	for _, g := range [][2]float64{{0, 0}, {0.5, 0}, {-0.5, 0}, {0, 0.5}, {0.3, -0.7}, {-0.9, 0.1}} {
		r, x := coord.SmithZ(g[0], g[1])
		gotRe, gotIm := gammaOf(c.Point(float32(r), float32(x)))
		if math.Abs(gotRe-g[0]) > eps || math.Abs(gotIm-g[1]) > eps {
			t.Errorf("Γ = %v came back through SmithZ and Point as %v, %v", g, gotRe, gotIm)
		}
	}
	if r, x := coord.SmithZ(1, 0); !math.IsInf(r, 1) || !math.IsInf(x, 1) {
		t.Errorf("an open circuit converted to %v%+vj, want an infinite pair", r, x)
	}
}

// flattenPath samples every cubic of a path, so that a test can ask where a
// curve actually goes rather than where its control points are.
func flattenPath(p *ir.Path, per int) []ir.Point {
	var out []ir.Point
	var cur ir.Point
	p.Walk(func(op ir.PathOp, pts []ir.Point) {
		switch op {
		case ir.OpMoveTo, ir.OpLineTo:
			cur = pts[0]
			out = append(out, cur)
		case ir.OpCubicTo:
			p0, c1, c2, p3 := cur, pts[0], pts[1], pts[2]
			for i := 1; i <= per; i++ {
				t := float64(i) / float64(per)
				u := 1 - t
				a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
				out = append(out, ir.Point{
					X: float32(a*float64(p0.X) + b*float64(c1.X) + c*float64(c2.X) + d*float64(p3.X)),
					Y: float32(a*float64(p0.Y) + b*float64(c1.Y) + c*float64(c2.Y) + d*float64(p3.Y)),
				})
			}
			cur = p3
		}
	})
	return out
}

// furnitureOf fills one panel's furniture and hands back the ticks it was
// filled from, so a test can walk the two in step.
func furnitureOf(opts ...coord.SmithOption) (*coord.Furniture, []scale.Tick, []scale.Tick) {
	c, x, y := smithChart(opts...)
	xt, yt := x.Ticks(6), y.Ticks(11)
	fur := new(coord.Furniture)
	c.Furniture(fur, smithArea, coord.Metrics{TickLen: 4, MinorTickLen: 2, LabelPad: 3}, xt, yt)
	return fur, xt, yt
}

// A constant-resistance circle is the image of a vertical line in the impedance
// plane. Every one of them passes through the open circuit, which is why they
// are all tangent to each other at the right rim, and each crosses the real
// axis at Γ = (r−1)/(r+1) — the place a paper chart prints its number.
func TestAConstantResistanceCircleIsTangentAtTheOpen(t *testing.T) {
	fur, xt, _ := furnitureOf()
	for i, tk := range xt {
		if !fur.InX[i] {
			t.Fatalf("r = %v was culled", tk.Value)
		}
		if tk.Value == 0 {
			// The r = 0 circle is the rim itself, which the reactance axis
			// already draws. One circle, not two coats of ink on it.
			if !fur.GridX[i].Empty() {
				t.Error("r = 0 drew a grid circle over the rim")
			}
			continue
		}
		pts := flattenPath(&fur.GridX[i].Path, 24)
		if len(pts) == 0 {
			t.Fatalf("r = %v drew no circle", tk.Value)
		}
		wantC := tk.Value / (1 + tk.Value)
		wantR := 1 / (1 + tk.Value)
		for _, p := range pts {
			re, im := gammaOf(p)
			if d := math.Hypot(re-wantC, im); math.Abs(d-wantR) > eps {
				t.Fatalf("r = %v: a point of its circle is %v from the centre, want %v", tk.Value, d, wantR)
			}
			if math.Hypot(re, im) > 1+eps {
				t.Fatalf("r = %v: its circle leaves the disc at Γ = (%v, %v)", tk.Value, re, im)
			}
		}
		// The label hangs below the real axis, where the circle crosses it.
		near(t, fur.LabelX[i].At.X, smithCX+smithR*float32((tk.Value-1)/(tk.Value+1)),
			"the label for r = "+tk.Label)
		if fur.LabelX[i].At.Y <= smithCY {
			t.Errorf("the label for r = %v sits above the real axis", tk.Value)
		}
	}
}

// A constant-reactance arc runs from the open circuit round to the rim, and
// stays inside the disc the whole way.
//
// Which half of its circle that is cannot be found by taking the shorter way
// round: the arc grows from nothing towards a half turn as the reactance grows,
// so the two candidates come arbitrarily close to equal. x = ±5 is well past
// the quarter turn where a naive reading starts drawing the wrong half.
func TestAConstantReactanceArcStaysInsideTheDisc(t *testing.T) {
	fur, _, yt := furnitureOf()
	for i, tk := range yt {
		if tk.Value == 0 {
			// A reactance of zero is the real axis, which the resistance axis
			// already draws, and whose label r = 0 already writes.
			if fur.InY[i] || !fur.GridY[i].Empty() {
				t.Error("x = 0 drew a second real axis")
			}
			continue
		}
		if !fur.InY[i] {
			t.Fatalf("x = %v was culled", tk.Value)
		}
		pts := flattenPath(&fur.GridY[i].Path, 24)
		if len(pts) < 2 {
			t.Fatalf("x = %v drew no arc", tk.Value)
		}
		for _, p := range pts {
			if g := math.Hypot(gammaOf(p)); g > 1+eps {
				t.Fatalf("x = %v: its arc reaches |Γ| = %v, outside the disc", tk.Value, g)
			}
		}
		// It starts at the open circuit and ends on the rim at Γ(jx), where its
		// label is.
		near(t, pts[0].X, smithCX+smithR, "the start of the x = "+tk.Label+" arc")
		near(t, pts[0].Y, smithCY, "the start of the x = "+tk.Label+" arc")
		last := pts[len(pts)-1]
		if g := math.Hypot(gammaOf(last)); math.Abs(g-1) > eps {
			t.Errorf("x = %v: its arc ends at |Γ| = %v rather than on the rim", tk.Value, g)
		}
		// The arc has to bulge into the half of the disc its sign names.
		mid := pts[len(pts)/2]
		if _, im := gammaOf(mid); (tk.Value > 0) != (im > 0) {
			t.Errorf("x = %v: its arc runs through the wrong half of the disc", tk.Value)
		}
		if lab := fur.LabelY[i]; math.Hypot(gammaOf(lab.At)) <= 1 {
			t.Errorf("the label for x = %v sits inside the rim", tk.Value)
		}
	}
}

// The axis lines are the real axis and the rim: the two curves the grid is read
// against, and the two a paper chart is printed with.
func TestSmithAxesAreTheRealAxisAndTheRim(t *testing.T) {
	fur, _, _ := furnitureOf()
	if len(fur.AxisX.Pts) != 2 {
		t.Fatalf("the real axis is %d points, want a two-point run", len(fur.AxisX.Pts))
	}
	near(t, fur.AxisX.Pts[0].X, smithCX-smithR, "the real axis's left end")
	near(t, fur.AxisX.Pts[1].X, smithCX+smithR, "the real axis's right end")
	near(t, fur.AxisX.Pts[0].Y, smithCY, "the real axis's height")
	for _, p := range flattenPath(&fur.AxisY.Path, 16) {
		if g := math.Hypot(gammaOf(p)); math.Abs(g-1) > eps {
			t.Fatalf("the rim passes through |Γ| = %v", g)
		}
	}
	// The resistance labels sit along one row and can run into each other, so
	// render's overlap filter has to run over them. This is the one place a
	// Smith panel answers the opposite of a polar one.
	if !fur.XLabelsShareARow {
		t.Error("the resistance labels were reported as not sharing a row")
	}
	for i, l := range fur.LabelX {
		// render.selectXLabels measures a label's box from a centred anchor.
		if l.H != ir.AlignCenter {
			t.Errorf("resistance label %d is not centred, so the overlap filter would measure the wrong box", i)
		}
	}
}

// A negative resistance has no place on the chart, and the coord says so
// through InX rather than by drawing something outside the disc. The per-tick
// slices stay parallel to the tick list either way, which is the contract
// render walks them under.
func TestANegativeResistanceIsCulled(t *testing.T) {
	x := scale.Linear(scale.Domain(-1, 5), scale.TickValues(-1, -0.5, 0, 1))
	y := scale.Linear(scale.Domain(-5, 5), scale.TickValues(-1, 1))
	c := coord.Smith(coord.SmithRadius(1)).Frame(smithArea, x, y)
	xt, yt := x.Ticks(4), y.Ticks(2)
	fur := new(coord.Furniture)
	c.Furniture(fur, smithArea, coord.Metrics{TickLen: 4, LabelPad: 3}, xt, yt)
	if len(fur.InX) != len(xt) || len(fur.LabelX) != len(xt) || len(fur.GridX) != len(xt) {
		t.Fatalf("the per-tick slices are %d/%d/%d long for %d ticks",
			len(fur.InX), len(fur.LabelX), len(fur.GridX), len(xt))
	}
	for i, tk := range xt {
		if want := tk.Value >= 0; fur.InX[i] != want {
			t.Errorf("r = %v: in = %v, want %v", tk.Value, fur.InX[i], want)
		}
		if tk.Value < 0 && !fur.GridX[i].Empty() {
			t.Errorf("r = %v drew a circle outside the disc", tk.Value)
		}
	}
}

// The admittance chart is the impedance chart turned through half a turn, and
// nothing else: y = 1/z gives Γ_y = −Γ_z.
func TestAnAdmittanceChartIsTheHalfTurn(t *testing.T) {
	z, _, _ := smithChart()
	y, _, _ := smithChart(coord.SmithAdmittance(true))
	for _, p := range [][2]float32{{0.5, 0.5}, {2, -1}, {0, 0}, {1, 0}, {4, 3}} {
		a, b := z.Point(p[0], p[1]), y.Point(p[0], p[1])
		near(t, b.X, 2*smithCX-a.X, "the admittance placement of z")
		near(t, b.Y, 2*smithCY-a.Y, "the admittance placement of z")
	}
	// A conductance circle therefore crowds towards the left rim, where the
	// resistance circles crowd towards the right.
	fur, xt, _ := furnitureOf(coord.SmithAdmittance(true))
	for i, tk := range xt {
		if tk.Value != 1 {
			continue
		}
		for _, p := range flattenPath(&fur.GridX[i].Path, 8) {
			if p.X > smithCX+eps {
				t.Errorf("the g = 1 circle reaches x = %v, right of the centre", p.X)
			}
		}
	}
}

// An edge is a chord by default, because that is what an instrument draws
// between two samples it measured and did not interpolate.
func TestASmithEdgeIsAChordByDefault(t *testing.T) {
	c, _, _ := smithChart()
	if !c.Straight() {
		t.Fatal("the default edge policy is not straight")
	}
	var p ir.Path
	from, to := c.Point(0.5, 0.5), c.Point(2, -1)
	p.MoveTo(from.X, from.Y)
	c.Edge(&p, from, to)
	if len(p.Ops) != 2 || p.Ops[1] != ir.OpLineTo {
		t.Fatalf("a chord edge appended %v, want one LineTo", p.Ops)
	}
}

// SmithArc draws the true image of the straight line between two impedances.
// The test is not that the edge is curved — any curve is curved — but that it
// is the *right* curve: every impedance along the segment has to land on it.
func TestSmithArcIsTheImageOfTheSegment(t *testing.T) {
	c, _, _ := smithChart(coord.SmithArc())
	if c.Straight() {
		t.Fatal("SmithArc still reports straight edges")
	}
	for _, e := range [][4]float32{
		{0.2, 0.2, 0.2, -0.2}, // a segment whose image wraps round the left
		{0.5, 1, 3, 1},        // a swept resistance
		{1, -2, 1, 2},         // a swept reactance: the constant-r circle itself
	} {
		var p ir.Path
		from, to := c.Point(e[0], e[1]), c.Point(e[2], e[3])
		p.MoveTo(from.X, from.Y)
		c.Edge(&p, from, to)
		drawn := flattenPath(&p, 256)
		for _, at := range []float32{0.25, 0.5, 0.75} {
			want := c.Point(e[0]+(e[2]-e[0])*at, e[1]+(e[3]-e[1])*at)
			best := math.Inf(1)
			for _, q := range drawn {
				best = math.Min(best, math.Hypot(float64(q.X-want.X), float64(q.Y-want.Y)))
			}
			if best > 0.5 {
				t.Errorf("edge %v: the impedance %v of the way along it is %v px off the curve", e, at, best)
			}
		}
	}
}

// A rectangle in impedance space — a tolerance box, a Region — is a
// curvilinear cell here, and it is one whatever the edge policy is: a shape's
// outline is a claim about the region it encloses, and a chord would enclose
// the wrong one.
func TestASmithCellIsCurvilinearUnderEitherEdgePolicy(t *testing.T) {
	for _, opts := range [][]coord.SmithOption{nil, {coord.SmithArc()}} {
		c, _, _ := smithChart(opts...)
		var p ir.Path
		c.Area(&p, 0.5, -0.5, 2, 0.5)
		cubics := 0
		p.Walk(func(op ir.PathOp, _ []ir.Point) {
			if op == ir.OpCubicTo {
				cubics++
			}
		})
		if cubics == 0 {
			t.Errorf("a cell drew no curves with opts %v", opts)
		}
		for _, q := range flattenPath(&p, 16) {
			if g := math.Hypot(gammaOf(q)); g > 1+eps {
				t.Errorf("a cell of passive impedances reaches |Γ| = %v", g)
			}
		}
	}
}

// The panel clips to the disc, so an active impedance is clipped away rather
// than drawn in a corner the chart does not mean.
func TestSmithClipsToTheDisc(t *testing.T) {
	c, _, _ := smithChart()
	var p ir.Path
	c.Clip(&p, smithArea)
	b := p.Bounds()
	near(t, b.Min.X, smithCX-smithR, "the clip's left edge")
	near(t, b.Max.X, smithCX+smithR, "the clip's right edge")
	near(t, b.Min.Y, smithCY-smithR, "the clip's top edge")
	near(t, b.Max.Y, smithCY+smithR, "the clip's bottom edge")
}

// Decimation is defined over pixel columns, and this map crowds the whole far
// half-plane into the last pixels before the rim, so a bucket of equal width is
// a bucket of nothing in particular. See ADR 0011.
func TestSmithDoesNotDecimate(t *testing.T) {
	if c, _, _ := smithChart(); c.Decimates() {
		t.Error("a Smith coord reported that it decimates")
	}
}

// A Smith chart has no middle for a mark to be broken out of: its centre is a
// matched load, not an origin of magnitude, so there is no direction away from
// it that means anything. The omission is deliberate and is pinned the way
// ADR 0026 pins Cartesian's.
func TestSmithHasNoMiddleToBreakOutOf(t *testing.T) {
	if _, ok := coord.Smith().(coord.Exploder); ok {
		t.Error("a Smith coord implements Exploder")
	}
}

// Frame hands back a coord positioned in the panel rather than moving the
// receiver, because panels are built concurrently.
func TestSmithFrameDoesNotMoveTheReceiver(t *testing.T) {
	c := coord.Smith()
	a := c.Frame(smithArea, linear(0, 5), linear(-5, 5))
	b := c.Frame(ir.Rect{Min: ir.Point{X: 400, Y: 400}, Max: ir.Point{X: 600, Y: 600}}, linear(0, 5), linear(-5, 5))
	if a.Point(1, 0) == b.Point(1, 0) {
		t.Fatal("two panels framed to one centre")
	}
	if got := c.Point(1, 0); got != (ir.Point{}) {
		t.Errorf("the receiver was framed in place: it now places z = 1 at %v", got)
	}
}

// The interval a Smith coord's scales map into is the impedance itself, which
// is this coord's answer to the question every coord answers. Extent reports
// the domain, which is what a mark spanning a whole axis needs.
func TestSmithMapsTheImpedanceOntoItself(t *testing.T) {
	x, y := scale.Linear(scale.Domain(0, 20)), scale.Linear(scale.Domain(-20, 20))
	c := coord.Smith().Frame(smithArea, x, y)
	for _, v := range []float64{0, 0.2, 1, 7.5, 20} {
		if got := x.Map(v); math.Abs(float64(got)-v) > 1e-4 {
			t.Errorf("the resistance %v maps to %v, not to itself", v, got)
		}
	}
	x0, x1, y0, y1 := c.Extent()
	if x0 != 0 || x1 != 20 || y0 != -20 || y1 != 20 {
		t.Errorf("the extent is %v..%v, %v..%v, want the two domains", x0, x1, y0, y1)
	}
}

func TestSmithRoundTripsThroughItsDescription(t *testing.T) {
	for _, c := range []coord.Coord{
		coord.Smith(),
		coord.Smith(coord.SmithArc()),
		coord.Smith(coord.SmithRadius(0.75), coord.SmithAdmittance(true), coord.SmithChord()),
	} {
		d, ok := coord.Describe(c)
		if !ok {
			t.Fatalf("%T cannot describe itself", c)
		}
		if d.Type != coord.TypeSmith {
			t.Errorf("described as %q", d.Type)
		}
		back, err := coord.FromDesc(d)
		if err != nil {
			t.Fatalf("FromDesc: %v", err)
		}
		again, _ := coord.Describe(back)
		if again != d {
			t.Errorf("the round trip turned %+v into %+v", d, again)
		}
	}
}

// The half turn is applied to the picture and not to the data, so the two
// changes cancel: a load plotted as its admittance on the Y chart lands on
// exactly the point its impedance lands on over on the Z chart. That identity
// is what makes the option useful rather than decorative — the same physical
// reflection, read against the other grid — and it is the one an example or a
// caller would silently get backwards.
func TestTheSameLoadLandsInOnePlaceOnEitherChart(t *testing.T) {
	z, _, _ := smithChart()
	y, _, _ := smithChart(coord.SmithAdmittance(true))
	for _, load := range [][2]float64{{0.3, -0.5}, {1, 0}, {2, 1.5}, {0.25, 0.25}} {
		// y = 1/z, in the caller's own numbers.
		d := load[0]*load[0] + load[1]*load[1]
		g, b := load[0]/d, -load[1]/d
		want := z.Point(float32(load[0]), float32(load[1]))
		got := y.Point(float32(g), float32(b))
		near(t, got.X, want.X, "the Y chart's placement of the same load")
		near(t, got.Y, want.Y, "the Y chart's placement of the same load")
	}
}
