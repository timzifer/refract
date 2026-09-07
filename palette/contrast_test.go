package palette_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
)

func TestLuminanceRunsFromBlackToWhite(t *testing.T) {
	if l := palette.Luminance(palette.Black); l != 0 {
		t.Errorf("black has luminance %v, want 0", l)
	}
	if l := palette.Luminance(palette.White); l != 1 {
		t.Errorf("white has luminance %v, want 1", l)
	}
}

// The midpoint of black and white is 188 rather than 128 because the blend is
// in linear light — the same fact palette's ramp test pins from the other
// side. Its luminance is therefore near a half, which is what makes it the
// place a contrast decision flips.
func TestTheMidGreyIsHalfWayInLight(t *testing.T) {
	mid := palette.Lerp(palette.Black, palette.White, 0.5)
	if mid.R != 188 {
		t.Fatalf("the midpoint is %d, want 188", mid.R)
	}
	if l := palette.Luminance(mid); math.Abs(l-0.5) > 0.01 {
		t.Errorf("the midpoint has luminance %v, want a half", l)
	}
	// Averaging the encoded bytes instead lands here, which is the error this
	// decode exists to avoid: visibly darker than half.
	if l := palette.Luminance(ir.RGB(128, 128, 128)); l > 0.3 {
		t.Errorf("mid-grey by bytes has luminance %v, want well under a half", l)
	}
}

// Luminance is what makes a fill light or dark to a reader, so it has to
// follow the eye rather than the channels: a saturated yellow is light and a
// saturated blue is dark, though both are one full channel plus zero or two.
func TestLuminanceWeightsTheChannelsForTheEye(t *testing.T) {
	yellow, blue := ir.RGB(255, 255, 0), ir.RGB(0, 0, 255)
	if palette.Luminance(yellow) <= palette.Luminance(blue) {
		t.Error("yellow does not read as lighter than blue")
	}
	if palette.Luminance(ir.RGB(0, 255, 0)) <= palette.Luminance(ir.RGB(255, 0, 0)) {
		t.Error("green does not read as lighter than red")
	}
}

// A translucent fill is composited over something this package cannot see, so
// its luminance is that of the colour it states.
func TestLuminanceIgnoresAlpha(t *testing.T) {
	faded := ir.Fade(palette.White, 0.1)
	if palette.Luminance(faded) != palette.Luminance(palette.White) {
		t.Error("alpha changed the luminance of a colour")
	}
}
