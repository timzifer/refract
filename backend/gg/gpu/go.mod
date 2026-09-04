module github.com/timzifer/refract/backend/gg/gpu

go 1.25.0

// The GPU tier is a module of its own so that importing the raster backend
// cannot pull a GPU stack in by accident: a nested module is excluded from its
// parent's module graph, so backend/gg's dependencies stay gg, x/image and the
// core. See docs/adr/0022.
//
// gg is pinned to exactly the version backend/gg pins, because this module
// enables a tier inside that build of it rather than a separate renderer.
require github.com/gogpu/gg v0.52.5

require (
	github.com/timzifer/refract v0.2.0
	github.com/timzifer/refract/backend/gg v0.2.0
)

require (
	github.com/go-webgpu/goffi v0.6.3 // indirect
	github.com/go-webgpu/webgpu v0.5.5 // indirect
	github.com/gogpu/gpucontext v0.28.0 // indirect
	github.com/gogpu/gputypes v0.5.2 // indirect
	github.com/gogpu/naga v0.18.0 // indirect
	github.com/gogpu/wgpu v0.31.6 // indirect
	golang.org/x/image v0.44.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)
