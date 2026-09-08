package scale_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

func TestColorLogGivesEachDecadeAnEqualShareOfTheRamp(t *testing.T) {
	c := scale.Sequential(palette.Viridis, scale.ColorLog(0))
	c.Train(1, 10000)
	for _, tc := range []struct {
		v    float64
		want float64
	}{
		{1, 0}, {10, 0.25}, {100, 0.5}, {1000, 0.75}, {10000, 1},
	} {
		if got := scale.ColorPositionOf(c, tc.v); math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("position(%g) = %g, want %g", tc.v, got, tc.want)
		}
	}
	// The same values under a linear ramp are all but the last crushed into
	// the bottom tenth, which is the failure the transform exists to fix.
	lin := scale.Sequential(palette.Viridis)
	lin.Train(1, 10000)
	if got := scale.ColorPositionOf(lin, 1000); got > 0.11 {
		t.Errorf("a linear ramp puts 1000 of 10000 at %g, want it crushed low", got)
	}
}

func TestColorLogRefusesNonPositiveValues(t *testing.T) {
	undef := ir.RGB(255, 0, 0)
	c := scale.Sequential(palette.Viridis, scale.ColorLog(0), scale.ColorUndefined(undef))
	c.Train(1, 100)
	// Training must not drag the domain to zero: a log ramp has no colour
	// there, and clamping an empty bin to the rarest observation's colour
	// would be inventing a count.
	c.Train(0, -5)
	if lo, hi := c.Domain(); lo != 1 || hi != 100 {
		t.Errorf("Domain() = (%g, %g) after training on 0 and -5, want (1, 100)", lo, hi)
	}
	for _, v := range []float64{0, -1} {
		if got := c.Color(v); got != undef {
			t.Errorf("Color(%g) = %v, want the undefined colour %v", v, got, undef)
		}
	}
}

func TestColorLogUntrainedFallsBackToADecade(t *testing.T) {
	c := scale.Sequential(palette.Viridis, scale.ColorLog(0))
	if lo, hi := c.Domain(); lo != 1 || hi != 10 {
		t.Errorf("untrained Domain() = (%g, %g), want (1, 10)", lo, hi)
	}
	// One repeated value still has to paint something.
	c.Train(50, 50)
	if lo, hi := c.Domain(); lo != 5 || hi != 500 {
		t.Errorf("Domain() over one value = (%g, %g), want a decade either side", lo, hi)
	}
}

func TestColorSymLogSpansZero(t *testing.T) {
	c := scale.Sequential(palette.BlueOrange, scale.ColorSymLog(10, 1))
	c.Train(-1000, 1000)
	if got := scale.ColorPositionOf(c, 0); math.Abs(got-0.5) > 1e-12 {
		t.Errorf("position(0) = %g, want the middle of the ramp", got)
	}
	if got := c.Color(-1000); got == c.Color(1000) {
		t.Error("the two ends of a symlog ramp are the same colour")
	}
	// Every finite value has a colour, including the negative ones a plain
	// log ramp has nothing to say about.
	if got := c.Color(-1); got == ir.Transparent {
		t.Error("Color(-1) is undefined on a symlog ramp")
	}
}

func TestDivergingTransformRunsOnTheDeviation(t *testing.T) {
	c := scale.Diverging(palette.BlueOrange, scale.ColorCenter(5), scale.ColorLog(0))
	c.Train(-995, 1005)
	if got := scale.ColorPositionOf(c, 5); math.Abs(got-0.5) > 1e-12 {
		t.Errorf("position(centre) = %g, want 0.5", got)
	}
	// Equal deviations either way are equally far from the middle, which is
	// the property a diverging ramp exists for and the transform must keep.
	up := scale.ColorPositionOf(c, 5+100) - 0.5
	down := 0.5 - scale.ColorPositionOf(c, 5-100)
	if math.Abs(up-down) > 1e-12 {
		t.Errorf("deviation +100 sits at %g from the middle, -100 at %g", up, down)
	}
	// A deviation of zero is the centre rather than an undefined value: on a
	// diverging scale a log transform is a symmetric one.
	if got := c.Color(5); got == ir.Transparent {
		t.Error("the centre of a diverging log ramp is undefined")
	}
}

// TestColorValueInvertsColorPosition is the guard on the two halves of the
// transform being written twice: the ramp is sampled through ColorValueAt and
// read back through ColorPosition, and a drift between them shows up as a
// colourbar whose gradient and ticks disagree.
func TestColorValueInvertsColorPosition(t *testing.T) {
	for name, c := range map[string]scale.ColorScale{
		"linear":          colorTrained(scale.Sequential(palette.Viridis), 1, 10000),
		"log":             colorTrained(scale.Sequential(palette.Viridis, scale.ColorLog(0)), 1, 10000),
		"log base 2":      colorTrained(scale.Sequential(palette.Viridis, scale.ColorLog(2)), 1, 10000),
		"symlog":          colorTrained(scale.Sequential(palette.Viridis, scale.ColorSymLog(0, 0)), -1000, 1000),
		"diverging":       colorTrained(scale.Diverging(palette.BlueOrange), -10, 30),
		"diverging log":   colorTrained(scale.Diverging(palette.BlueOrange, scale.ColorLog(0)), -10, 30),
		"diverging shift": colorTrained(scale.Diverging(palette.BlueOrange, scale.ColorCenter(5), scale.ColorSymLog(2, 0.5)), -95, 105),
	} {
		for i := 0; i <= 20; i++ {
			want := float64(i) / 20
			got := scale.ColorPositionOf(c, scale.ColorValueOf(c, want))
			if math.Abs(got-want) > 1e-9 {
				t.Errorf("%s: position(value(%g)) = %g", name, want, got)
			}
		}
	}
}

// TestColorAxisAgreesWithTheRamp guards the other seam: the colourbar takes
// its tick values from a positional scale and its gradient from the colour
// scale, and the two carry the transform's arithmetic separately.
func TestColorAxisAgreesWithTheRamp(t *testing.T) {
	for name, c := range map[string]scale.ColorScale{
		"linear": colorTrained(scale.Sequential(palette.Viridis), 1, 10000),
		"log":    colorTrained(scale.Sequential(palette.Viridis, scale.ColorLog(0)), 1, 10000),
		"symlog": colorTrained(scale.Sequential(palette.Viridis, scale.ColorSymLog(0, 0)), -1000, 1000),
	} {
		axis := scale.ColorAxisOf(c)
		axis.SetRange(0, 1)
		lo, hi := c.Domain()
		for i := 0; i <= 20; i++ {
			v := lo + float64(i)/20*(hi-lo)
			want := float64(axis.Map(v))
			if got := scale.ColorPositionOf(c, v); math.Abs(got-want) > 1e-6 {
				t.Errorf("%s: the ramp puts %g at %g, its axis at %g", name, v, got, want)
			}
		}
	}
}

func TestColorAxisTicksAreDecadesUnderALogRamp(t *testing.T) {
	c := scale.Sequential(palette.Viridis, scale.ColorLog(0))
	c.Train(1, 10000)
	axis := scale.ColorAxisOf(c)
	axis.SetRange(0, 100)
	var labelled []float64
	for _, tick := range axis.Ticks(5) {
		if !tick.Minor && tick.Label != "" {
			labelled = append(labelled, tick.Value)
		}
	}
	want := []float64{1, 10, 100, 1000, 10000}
	if len(labelled) != len(want) {
		t.Fatalf("labelled ticks = %v, want the decades %v", labelled, want)
	}
	for i, v := range want {
		if math.Abs(labelled[i]-v)/v > 1e-9 {
			t.Errorf("tick %d = %g, want %g", i, labelled[i], v)
		}
	}
}

func TestColorTransformRoundTripsThroughADesc(t *testing.T) {
	for name, orig := range map[string]scale.ColorScale{
		"log":           scale.Sequential(palette.Viridis, scale.ColorLog(2)),
		"symlog":        scale.Sequential(palette.Viridis, scale.ColorSymLog(2, 0.25)),
		"diverging log": scale.Diverging(palette.BlueOrange, scale.ColorCenter(3), scale.ColorLog(0)),
		"linear":        scale.Sequential(palette.Viridis),
	} {
		d, ok := scale.DescribeColor(orig)
		if !ok {
			t.Fatalf("%s: does not describe itself", name)
		}
		back, err := scale.ColorFromDesc(d)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		orig.Train(-100, 1000)
		back.Train(-100, 1000)
		for i := 0; i <= 20; i++ {
			at := float64(i) / 20
			v := scale.ColorValueOf(orig, at)
			if got, want := back.Color(v), orig.Color(v); got != want {
				t.Errorf("%s: Color(%g) = %v after a round trip, want %v", name, v, got, want)
			}
		}
	}
}

func TestColorTransformOfReportsTheTransform(t *testing.T) {
	if got := scale.ColorTransformOf(scale.Sequential(nil)); got != scale.TransformLinear {
		t.Errorf("a plain sequential scale reports %q", got)
	}
	if got := scale.ColorTransformOf(scale.Sequential(nil, scale.ColorLog(0))); got != scale.TransformLog {
		t.Errorf("a log scale reports %q", got)
	}
	// A scale that is not a transformer reads as linear rather than failing.
	if got := scale.ColorTransformOf(scale.Qualitative(nil)); got != scale.TransformLinear {
		t.Errorf("a qualitative scale reports %q", got)
	}
}

func TestColorFromDescRejectsAnUnknownTransform(t *testing.T) {
	_, err := scale.ColorFromDesc(scale.ColorDesc{Kind: scale.KindSequential, Transform: "cbrt"})
	if err == nil {
		t.Fatal("a colour scale was built from a transform nobody defines")
	}
}

func colorTrained(c scale.ColorScale, lo, hi float64) scale.ColorScale {
	c.Train(lo, hi)
	return c
}
