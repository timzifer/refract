package gpu

import (
	"github.com/gogpu/gg"

	// The blank import is the whole mechanism: gg's gpu package registers an
	// accelerator and a coverage filler in gg's own registry from its init,
	// and every context made after that uses them.
	_ "github.com/gogpu/gg/gpu"
)

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
