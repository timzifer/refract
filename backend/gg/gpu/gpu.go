package gpu

import (
	"github.com/gogpu/gg"

	// The blank import is the whole mechanism: gg's gpu package registers an
	// accelerator and a coverage filler in gg's own registry from its init,
	// and every context made after that uses them.
	_ "github.com/gogpu/gg/gpu"

	// The other half of the mechanism, and the half nobody imports: wgpu's HAL
	// backends — Vulkan, DX12, Metal, GLES — register themselves from their own
	// init, and neither gg nor gpucontext pulls one in. Without this, wgpu
	// enumerates no adapters on a machine with a working GPU, the probe below
	// finds no ink and the tier gives itself back on hardware that was fine.
	_ "github.com/gogpu/wgpu/hal/allbackends"
)

// init keeps the accelerator only if it can be shown to draw.
//
// Registration in gg is not an answer about the hardware: it happens in an
// init, before anything has asked for a device. Up to and including gg
// v0.52.5 the path operations queue a draw without checking that a device can
// be had, and gg reports the failure at flush time, where the draw is dropped
// rather than rasterized — so on a machine with no adapter a chart came out
// with its labels and none of its geometry, silently. Text was unaffected,
// which is what made it look like a rendering artefact rather than a missing
// device.
//
// So the tier proves the accelerator before trusting it: one stroke into a
// small buffer, and if the pixels are not there the accelerator is given back
// and everything falls to the CPU rasterizer — which is what importing this
// package has always promised. The cost is one device probe at startup, which
// is the probe the first chart would have paid for anyway.
func init() {
	if gg.Accelerator() != nil && !accelerateDraws() {
		gg.CloseAccelerator()
	}
}

// accelerateDraws reports whether a stroked path put by the registered
// accelerator actually lands in the pixel buffer.
func accelerateDraws() bool {
	c := gg.NewContext(probeSize, probeSize)
	defer func() { _ = c.Close() }()

	c.ClearPath()
	c.MoveTo(1, 1)
	c.LineTo(probeSize-1, probeSize-1)
	c.SetStroke(gg.Stroke{Width: 2})
	c.SetColor(gg.RGBA{R: 1, A: 1})
	if err := c.Stroke(); err != nil {
		return false
	}
	_ = c.FlushGPU()

	img := c.Image()
	b := img.Bounds()
	first := img.At(b.Min.X, b.Min.Y)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.At(x, y) != first {
				return true
			}
		}
	}
	return false
}

// probeSize is the probe's buffer, big enough that a two-pixel stroke across
// the diagonal cannot be lost to rounding and small enough to be free.
const probeSize = 16

// Enabled reports whether the GPU tier actually took.
//
// Importing this package asks for the GPU; a machine with no Vulkan, Metal or
// DX12 — a container, a VM, a CI runner — says no, and gg falls back to the CPU
// rasterizer without a word. That is the right default: a chart that renders
// slowly beats a chart that does not render. This is how a program that would
// rather know can find out.
//
// A program that opts in and then exits should call [Close] to give the device
// back, which is gg's own advice for the accelerator it registers here.
func Enabled() bool { return gg.Accelerator() != nil }

// Close releases the GPU device and everything held on it, after which
// rendering falls back to the CPU rasterizer. It is what a program defers from
// main; calling it twice, or with no GPU registered, does nothing.
func Close() { gg.CloseAccelerator() }
