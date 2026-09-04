package palette_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
)

func TestRampReturnsItsEndsExactly(t *testing.T) {
	r := palette.Viridis
	if got := r.At(0); got != r[0] {
		t.Errorf("At(0) = %v, want %v", got, r[0])
	}
	if got := r.At(1); got != r[len(r)-1] {
		t.Errorf("At(1) = %v, want %v", got, r[len(r)-1])
	}
	if r.At(-3) != r.At(0) || r.At(7) != r.At(1) {
		t.Error("At must clamp outside [0, 1]")
	}
}

// A midpoint blended in sRGB byte space comes out about 20% too dark. This is
// the test that catches a "simplification" back to averaging bytes.
func TestLerpBlendsInLinearLight(t *testing.T) {
	got := palette.Lerp(ir.RGB(0, 0, 0), ir.RGB(255, 255, 255), 0.5)
	const wantNaive = 128 // what averaging the encoded bytes would give
	if int(got.R) <= wantNaive {
		t.Fatalf("Lerp(black, white, 0.5) = %d, which is no lighter than the sRGB average", got.R)
	}
	// Half the light between black and white encodes to sRGB 188.
	if math.Abs(float64(got.R)-188) > 1 {
		t.Errorf("Lerp(black, white, 0.5) = %d, want about 188", got.R)
	}
	if got.R != got.G || got.G != got.B {
		t.Errorf("a grey blend came out coloured: %v", got)
	}
	if got.A != 255 {
		t.Errorf("alpha = %d, want 255", got.A)
	}
}

func TestLerpAtTheEndsIsExact(t *testing.T) {
	a, b := ir.RGB(1, 2, 3), ir.RGB(250, 251, 252)
	if palette.Lerp(a, b, 0) != a || palette.Lerp(a, b, 1) != b {
		t.Error("a round trip through linear light must not disturb the endpoints")
	}
}

func TestLerpInterpolatesAlpha(t *testing.T) {
	got := palette.Lerp(ir.RGBA(10, 10, 10, 0), ir.RGBA(10, 10, 10, 200), 0.5)
	if got.A != 100 {
		t.Errorf("alpha = %d, want 100", got.A)
	}
}

func TestReverseDoesNotAliasTheOriginal(t *testing.T) {
	r := palette.Viridis
	rev := r.Reverse()
	if rev[0] != r[len(r)-1] {
		t.Error("Reverse did not reverse")
	}
	rev[0] = ir.RGB(0, 0, 0)
	if r[len(r)-1] == (ir.Color{A: 255}) {
		t.Error("Reverse shares its backing array with the original")
	}
}

func TestRampEdgeCases(t *testing.T) {
	if got := (palette.Ramp{}).At(0.5); got != ir.Transparent {
		t.Errorf("an empty ramp returned %v, want transparent", got)
	}
	one := palette.Ramp{palette.Blue}
	if one.At(0) != palette.Blue || one.At(1) != palette.Blue {
		t.Error("a one-colour ramp must be constant")
	}
	if got := palette.Viridis.At(math.NaN()); got != ir.Transparent {
		t.Errorf("At(NaN) = %v, want transparent", got)
	}
}

// Every sequential ramp shipped here is meant to survive being printed in
// greyscale, which means its perceived lightness has to climb monotonically.
func TestSequentialRampsAreMonotoneInLightness(t *testing.T) {
	for name, r := range map[string]palette.Ramp{
		"Viridis": palette.Viridis,
		"Cividis": palette.Cividis,
		"Magma":   palette.Magma,
		"Blues":   palette.Blues.Reverse(),
		"Greys":   palette.Greys.Reverse(),
	} {
		prev := -1.0
		for i := 0; i <= 20; i++ {
			l := luminance(r.At(float64(i) / 20))
			if l < prev-1e-6 {
				t.Errorf("%s: lightness falls at t = %.2f (%.4f after %.4f)", name, float64(i)/20, l, prev)
				break
			}
			prev = l
		}
	}
}

// luminance is relative luminance in linear light, the quantity a greyscale
// print preserves.
func luminance(c ir.Color) float64 {
	lin := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.04045 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}
