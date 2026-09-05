package scale_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/scale"
)

func TestDoublingAValueMultipliesTheDiameterByRootTwo(t *testing.T) {
	// The property the whole channel exists for: a bubble's ink is its value,
	// so twice the value is twice the area and √2 times across. It holds for
	// every pair in the domain, not just for the extremes.
	s := scale.Size()
	s.Train(0, 100)
	s.SetRange(0, 40)

	for _, v := range []float64{1, 2.5, 10, 25, 50} {
		small, large := s.Size(v), s.Size(2*v)
		ratio := float64(large) / float64(small)
		if math.Abs(ratio-math.Sqrt2) > 1e-5 {
			t.Errorf("%v draws %v across and %v draws %v: the ratio is %v, want √2",
				v, small, 2*v, large, ratio)
		}
	}
}

func TestASizeScaleSpansTheRangeItWasGiven(t *testing.T) {
	s := scale.Size()
	s.Train(0, 50)
	s.SetRange(4, 40)
	if got := s.Size(50); math.Abs(float64(got)-40) > 1e-4 {
		t.Errorf("the largest value draws %v across, want the range's top of 40", got)
	}
	if got := s.Size(0); math.Abs(float64(got)-4) > 1e-4 {
		t.Errorf("the anchor draws %v across, want the range's floor of 4", got)
	}
	// Halfway up the domain is halfway up the *area*, not halfway across.
	mid := float64(s.Size(25))
	wantArea := (16.0 + 1600) / 2
	if math.Abs(mid*mid-wantArea) > 1e-3 {
		t.Errorf("the middle of the domain draws %v across, an area of %v, want %v", mid, mid*mid, wantArea)
	}
}

func TestASizeScaleAnchorsAtZeroRatherThanAtTheSmallestValue(t *testing.T) {
	s := scale.Size()
	s.Train(100, 101, 102)
	s.SetRange(0, 30)
	lo, _ := s.Domain()
	if lo != 0 {
		t.Errorf("the domain starts at %v; a size channel that started at the data would draw the smallest of three near-equal values as nothing", lo)
	}
	if a, b := s.Size(100), s.Size(102); math.Abs(float64(b-a)) > 0.5 {
		t.Errorf("three values within 2%% of each other draw at %v and %v, which reads as a difference the data does not have", a, b)
	}
}

func TestASizeScaleDrawsNothingForWhatItCannotPlace(t *testing.T) {
	s := scale.Size()
	s.Train(0, 10)
	s.SetRange(2, 20)
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1} {
		if got := s.Size(v); got != 0 {
			t.Errorf("Size(%v) = %v, want 0 so nothing is drawn", v, got)
		}
	}
}

func TestAPinnedSizeRangeOutranksTheTheme(t *testing.T) {
	s := scale.Size(scale.SizeRange(5, 25))
	s.Train(0, 10)
	s.SetRange(0, 60) // what a geom would do from its theme
	if got := s.Size(10); math.Abs(float64(got)-25) > 1e-4 {
		t.Errorf("the largest value draws %v across, want the pinned 25", got)
	}
}

func TestASizeScaleRoundTripsThroughItsDescription(t *testing.T) {
	s := scale.Size(scale.SizeDomain(0, 500), scale.SizeRange(3, 33), scale.SizeZero(10))
	d, ok := scale.DescribeSize(s)
	if !ok {
		t.Fatal("the built-in size scale cannot describe itself")
	}
	back, err := scale.SizeFromDesc(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{10, 50, 200, 500} {
		if a, b := s.Size(v), back.Size(v); a != b {
			t.Errorf("Size(%v) is %v before the round trip and %v after", v, a, b)
		}
	}
}
