package scale_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

func TestThresholdCutsAtTheBoundariesItWasGiven(t *testing.T) {
	c := scale.Threshold(palette.Viridis, []float64{10, 100})
	c.Train(0, 1000)
	if got := c.Classes(); got != 3 {
		t.Errorf("Classes() = %d, want 3 for two breaks", got)
	}
	// A value equal to a break is in the class above it.
	below, at, over := c.Color(9.999), c.Color(10), c.Color(100)
	if below == at {
		t.Error("a value equal to a break got the class below it")
	}
	if at == over {
		t.Error("the middle and top classes are the same colour")
	}
	// Everything inside one class is one colour: that is the whole point.
	if c.Color(10) != c.Color(99.9) {
		t.Error("two values in one class got two colours")
	}
	if c.Color(0) != c.Color(9) {
		t.Error("two values in the bottom class got two colours")
	}
}

func TestThresholdSortsAndDeduplicatesItsBreaks(t *testing.T) {
	c := scale.Threshold(palette.Viridis, []float64{100, 10, 10, math.NaN()})
	got := c.Breaks()
	if len(got) != 2 || got[0] != 10 || got[1] != 100 {
		t.Errorf("Breaks() = %v, want [10 100]", got)
	}
	// A repeated break would be a class of zero width, which no value can be
	// in and no reader can see.
	if c.Classes() != 3 {
		t.Errorf("Classes() = %d, want 3", c.Classes())
	}
}

func TestThresholdWidensItsDomainToCoverEveryClass(t *testing.T) {
	// A class nobody has data in is still a class the bar has to show.
	c := scale.Threshold(palette.Viridis, []float64{10, 100})
	c.Train(20, 30)
	lo, hi := c.Domain()
	if lo > 10 || hi < 100 {
		t.Errorf("Domain() = (%g, %g), want it to cover the breaks", lo, hi)
	}
	// Untrained, the outermost classes still get width on the bar.
	u := scale.Threshold(palette.Viridis, []float64{10, 100})
	ulo, uhi := u.Domain()
	if ulo >= 10 || uhi <= 100 {
		t.Errorf("untrained Domain() = (%g, %g), want it outside the breaks", ulo, uhi)
	}
}

func TestThresholdWithNoBreaksIsOneClass(t *testing.T) {
	c := scale.Threshold(palette.Viridis, nil)
	c.Train(0, 100)
	if got := c.Classes(); got != 1 {
		t.Errorf("Classes() = %d, want 1", got)
	}
	if c.Color(0) != c.Color(100) {
		t.Error("a scale that cuts nowhere painted two colours")
	}
}

func TestQuantizeCutsTheDomainIntoEqualClasses(t *testing.T) {
	c := scale.Quantize(palette.Viridis, 4)
	c.Train(0, 100)
	want := []float64{25, 50, 75}
	got := c.Breaks()
	if len(got) != len(want) {
		t.Fatalf("Breaks() = %v, want %v", got, want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Errorf("break %d = %g, want %g", i, got[i], want[i])
		}
	}
	// Four classes, four colours, spread across the whole ramp rather than
	// bunched at one end.
	seen := map[ir.Color]bool{}
	for _, v := range []float64{0, 30, 60, 90} {
		seen[c.Color(v)] = true
	}
	if len(seen) != 4 {
		t.Errorf("four classes painted %d colours", len(seen))
	}
	if c.Color(0) == c.Color(100) {
		t.Error("the ends of a quantized ramp are the same colour")
	}
}

func TestQuantizeUnderALogRampCutsAtDecades(t *testing.T) {
	c := scale.Quantize(palette.Viridis, 4, scale.ColorLog(0))
	c.Train(1, 10000)
	want := []float64{10, 100, 1000}
	got := c.Breaks()
	if len(got) != len(want) {
		t.Fatalf("Breaks() = %v, want the decades %v", got, want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i])/want[i] > 1e-9 {
			t.Errorf("break %d = %g, want %g", i, got[i], want[i])
		}
	}
	// And a value in a decade gets that decade's colour.
	if c.Color(5) == c.Color(50) {
		t.Error("two decades got one colour")
	}
	if c.Color(11) != c.Color(99) {
		t.Error("one decade got two colours")
	}
}

func TestQuantizeClampsAnEmptyClassCount(t *testing.T) {
	c := scale.Quantize(palette.Viridis, 0)
	c.Train(0, 100)
	if got := c.Classes(); got != 1 {
		t.Errorf("Classes() = %d, want 1", got)
	}
	if got := c.Breaks(); len(got) != 0 {
		t.Errorf("Breaks() = %v, want none", got)
	}
}

func TestAClassedBarIsStillAnAxis(t *testing.T) {
	// A value sits on the bar where its number puts it: the classes are as
	// wide on the bar as they are in the data.
	c := scale.Threshold(palette.Viridis, []float64{10, 20})
	c.Train(0, 100)
	if got := scale.ColorPositionOf(c, 50); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("position(50) over 0..100 = %g, want 0.5", got)
	}
	for i := 0; i <= 10; i++ {
		want := float64(i) / 10
		if got := scale.ColorPositionOf(c, scale.ColorValueOf(c, want)); math.Abs(got-want) > 1e-9 {
			t.Errorf("position(value(%g)) = %g", want, got)
		}
	}
}

func TestClassedReportsAClassedScale(t *testing.T) {
	if _, ok := scale.Classed(scale.Quantize(nil, 3)); !ok {
		t.Error("a quantize scale does not report itself as classed")
	}
	if _, ok := scale.Classed(scale.Sequential(nil)); ok {
		t.Error("a continuous ramp reports itself as classed")
	}
	// A classed scale is not a discrete one: its classes are intervals of a
	// quantity, not names, so it gets a bar rather than legend entries.
	if _, ok := scale.Discrete(scale.Quantize(nil, 3)); ok {
		t.Error("a quantize scale reports itself as discrete")
	}
}

func TestAClassedScaleRoundTripsThroughADesc(t *testing.T) {
	for name, orig := range map[string]scale.ColorScale{
		"threshold":     scale.Threshold(palette.Viridis, []float64{10, 100}),
		"quantize":      scale.Quantize(palette.Viridis, 5),
		"quantize log":  scale.Quantize(palette.Viridis, 4, scale.ColorLog(0)),
		"threshold rev": scale.Threshold(palette.Viridis, []float64{2}, scale.ColorReverse()),
	} {
		d, ok := scale.DescribeColor(orig)
		if !ok {
			t.Fatalf("%s: does not describe itself", name)
		}
		back, err := scale.ColorFromDesc(d)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		orig.Train(1, 1000)
		back.Train(1, 1000)
		for i := 0; i <= 20; i++ {
			v := scale.ColorValueOf(orig, float64(i)/20)
			if got, want := back.Color(v), orig.Color(v); got != want {
				t.Errorf("%s: Color(%g) = %v after a round trip, want %v", name, v, got, want)
			}
		}
	}
}

func TestClassedColoursDoNotAllocate(t *testing.T) {
	// Color is called once per mark, so it must not build the break list.
	c := scale.Quantize(palette.Viridis, 5)
	c.Train(0, 1000)
	th := scale.Threshold(palette.Viridis, []float64{1, 10, 100})
	th.Train(0, 1000)
	for name, cs := range map[string]scale.ColorScale{"quantize": c, "threshold": th} {
		got := testing.AllocsPerRun(100, func() {
			for v := 0.0; v < 1000; v += 100 {
				_ = cs.Color(v)
			}
		})
		if got != 0 {
			t.Errorf("%s: Color allocated %v times per run", name, got)
		}
	}
}
