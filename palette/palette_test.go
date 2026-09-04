package palette_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
)

func TestQualitativeWrapsAndHandlesNegativeIndices(t *testing.T) {
	p := palette.Qualitative{ir.RGB(1, 0, 0), ir.RGB(0, 1, 0), ir.RGB(0, 0, 1)}

	if got, want := p.At(3), p.At(0); got != want {
		t.Errorf("At(3) = %v, want the wrap to %v", got, want)
	}
	if got, want := p.At(-1), p.At(2); got != want {
		t.Errorf("At(-1) = %v, want %v — a negative index must not panic or return black", got, want)
	}
}

func TestEmptyPaletteHasAFallback(t *testing.T) {
	if got := (palette.Qualitative{}).At(0); got.A == 0 {
		t.Fatal("an empty palette returned a transparent colour")
	}
}

func TestOkabeItoEntriesAreDistinguishable(t *testing.T) {
	// The point of this palette is that adjacent series are told apart. A
	// crude perceptual distance is enough to catch an accidental duplicate or
	// a near-duplicate introduced by a well-meaning edit.
	for i := range palette.OkabeIto {
		for j := i + 1; j < len(palette.OkabeIto); j++ {
			a, b := palette.OkabeIto[i], palette.OkabeIto[j]
			if d := distance(a, b); d < 40 {
				t.Errorf("colours %d and %d are only %.0f apart: %v vs %v", i, j, d, a, b)
			}
		}
	}
}

func TestOkabeItoIsFullyOpaque(t *testing.T) {
	for i, c := range palette.OkabeIto {
		if c.A != 255 {
			t.Errorf("entry %d has alpha %d, want 255", i, c.A)
		}
	}
}

// distance is a rough luminance-weighted RGB distance. It is not a colour
// science model, just a guard against two entries being effectively the same.
func distance(a, b ir.Color) float64 {
	dr := float64(a.R) - float64(b.R)
	dg := float64(a.G) - float64(b.G)
	db := float64(a.B) - float64(b.B)
	return math.Sqrt(2*dr*dr + 4*dg*dg + 3*db*db)
}
