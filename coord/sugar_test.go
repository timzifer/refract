package coord_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// The v0.8 sugar, on the coord side: two recipes that name a chart, and the
// break-out that a stacked bar in a polar coord could not already say.

// Pie and Donut are Polar with the options spelled out, and nothing else. The
// test is worth having because it is the whole claim: sugar that drew a
// different chart would be a second implementation of a pie.
func TestPieAndDonutAreTheRecipesTheyName(t *testing.T) {
	for _, tc := range []struct {
		name  string
		sugar coord.Coord
		long  coord.Coord
	}{
		{"pie", coord.Pie(), coord.Polar(coord.Theta(coord.FromY))},
		{"donut", coord.Donut(0.4), coord.Polar(coord.Theta(coord.FromY), coord.Hole(0.4))},
		{
			"half a donut",
			coord.Donut(0.4, coord.Sweep(math.Pi)),
			coord.Polar(coord.Theta(coord.FromY), coord.Hole(0.4), coord.Sweep(math.Pi)),
		},
	} {
		a, ok := coord.Describe(tc.sugar)
		if !ok {
			t.Fatalf("%s does not describe itself", tc.name)
		}
		b, _ := coord.Describe(tc.long)
		if a != b {
			t.Errorf("%s is\n %+v\nwant\n %+v", tc.name, a, b)
		}
	}
}

// A break-out moves a mark along its own bisector, by a fraction of the outer
// radius. Both halves matter: the direction is what keeps the gap on either
// side of a slice equal, and the fraction is what makes it independent of how
// big the chart is.
func TestABreakOutMovesAMarkAlongItsBisector(t *testing.T) {
	x, y := linear(0, 1), linear(0, 4)
	// Theta from Y, so the pair is (radius, angle) and a quarter of the Y
	// domain is a quarter turn.
	c := coord.Polar(coord.Theta(coord.FromY), coord.Radius(1)).Frame(ir.R(0, 0, 200, 200), x, y)
	e, ok := c.(coord.Exploder)
	if !ok {
		t.Fatal("a polar coord cannot break a mark out")
	}
	// The mark from noon to three o'clock: its bisector is half past one.
	dx, dy := e.Explode(x.Map(0), y.Map(0), x.Map(1), y.Map(1), 0.1)
	want := 0.1 * 100.0
	if d := math.Hypot(float64(dx), float64(dy)); math.Abs(d-want) > eps {
		t.Errorf("the break-out is %v away, want %v — a tenth of the outer radius", d, want)
	}
	s, cs := math.Sincos(math.Pi / 4)
	near(t, dx, float32(want*s), "the break-out's x")
	near(t, dy, float32(-want*cs), "the break-out's y")

	// And nothing moves by nothing.
	if dx, dy := e.Explode(x.Map(0), y.Map(0), x.Map(1), y.Map(1), 0); dx != 0 || dy != 0 {
		t.Errorf("a break-out of zero moved the mark to (%v, %v)", dx, dy)
	}
}

// Cartesian is deliberately not an Exploder: a rectangle on a Cartesian panel
// has no middle to be moved away from, and moving every bar the same way would
// be a translation of the layer rather than a reading of it.
func TestCartesianHasNoMiddleToBreakOutOf(t *testing.T) {
	if _, ok := coord.Cartesian().(coord.Exploder); ok {
		t.Error("Cartesian claims it can break a mark out")
	}
	framed := coord.Cartesian().Frame(ir.R(0, 0, 100, 100), scale.Linear(), scale.Linear())
	if _, ok := framed.(coord.Exploder); ok {
		t.Error("a framed Cartesian coord claims it can break a mark out")
	}
}
