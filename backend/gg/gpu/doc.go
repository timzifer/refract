// Package gpu turns on gg's GPU tier for refract's raster backend.
//
// Importing it registers gg's GPU accelerator and its tile-based coverage
// filler, which every gg context created afterwards uses: circles and rounded
// rectangles are evaluated as signed distance fields on the GPU, and complex
// paths are rasterized in tiles rather than scanline by scanline. Nothing in
// refract changes — the same IR, the same marks, the same output — and nothing
// needs to be passed anywhere:
//
//	import (
//	    ggbackend "github.com/timzifer/refract/backend/gg"
//	    _ "github.com/timzifer/refract/backend/gg/gpu" // opt into the GPU tier
//	)
//
//	err := p.Render(ggbackend.PNG("chart.png"))
//
// # Why a module of its own
//
// Because the import is the opt-in, and an opt-in that arrives with a
// dependency nobody asked for is not one. gg's GPU package pulls wgpu, naga and
// a foreign-function layer into the build, wants a working Vulkan, Metal or
// DX12 at run time, and does not build for js/wasm at all. backend/gg is the
// supported path and must keep its dependency graph small and its build
// portable, so [ADR 0006] forbids it from importing gg/gpu — and a nested
// module is how the tier is offered without breaking that rule, exactly as
// backend/gg is how gg is offered without breaking the core's promise to
// depend on nothing.
//
// # What it is for, and what it is not for
//
// It is for interaction over a lot of data: a window panning and zooming
// through millions of points, where the CPU rasterizer's scanline pass is the
// frame budget. It is not for server-side stills — a process rendering charts
// in bulk should stay on the CPU rasterizer, which needs no device, no driver
// and no first-frame compilation.
//
// It is opt-in beta until the GoGPU native backends prove out across hardware,
// which is CONCEPT.md §14's position for v1.0 and not a temporary caveat.
//
// # When there is no GPU
//
// Registration fails quietly and gg falls back to the CPU: a chart still
// renders on a machine with no usable device, which is the only acceptable
// behaviour for a plotting library. [Enabled] reports which way it went, for a
// program that would rather say so than wonder.
//
// [ADR 0006]: https://github.com/timzifer/refract/blob/main/docs/adr/0006-gg-coupling-surface.md
package gpu
