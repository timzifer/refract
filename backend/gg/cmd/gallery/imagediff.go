package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

// pngTolerance is the fraction of colour channels allowed to differ by more
// than channelTolerance before two renders count as different figures.
//
// gg's CPU rasterizer is deterministic, so in practice the diff is exactly
// zero. The tolerance exists so that a future rasterizer improvement, or a
// platform with different floating-point rounding in the AA path, produces a
// review-worthy signal rather than a red build on every machine.
const (
	pngTolerance     = 0.001 // 0.1% of channels
	channelTolerance = 2     // out of 255
)

// pngDiff decodes two PNGs and reports the fraction of channels that differ by
// more than channelTolerance.
func pngDiff(a, b []byte) (float64, error) {
	if bytes.Equal(a, b) {
		return 0, nil
	}
	ia, err := png.Decode(bytes.NewReader(a))
	if err != nil {
		return 0, fmt.Errorf("decoding the committed image: %w", err)
	}
	ib, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return 0, fmt.Errorf("decoding the rendered image: %w", err)
	}
	ra, rb := ia.Bounds(), ib.Bounds()
	if ra.Dx() != rb.Dx() || ra.Dy() != rb.Dy() {
		return 1, fmt.Errorf("size changed: %v on disk, %v rendered", ra.Size(), rb.Size())
	}

	var bad, total int
	for y := range ra.Dy() {
		for x := range ra.Dx() {
			c1 := colorOf(ia, ra.Min.X+x, ra.Min.Y+y)
			c2 := colorOf(ib, rb.Min.X+x, rb.Min.Y+y)
			for i := range c1 {
				total++
				if absDiff(c1[i], c2[i]) > channelTolerance {
					bad++
				}
			}
		}
	}
	if total == 0 {
		return 0, nil
	}
	return float64(bad) / float64(total), nil
}

func colorOf(img image.Image, x, y int) [4]uint8 {
	r, g, b, a := img.At(x, y).RGBA()
	return [4]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a) - int(b)
	}
	return int(b) - int(a)
}
