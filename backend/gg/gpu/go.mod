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

// wgpu is held below v0.33, which turned the render-pass calls into struct
// arguments; gg v0.52.5 above still makes the old calls, so a newer wgpu fails
// to build gg rather than this package. See backend/window/go.mod, which
// carries the same ceiling, and .github/dependabot.yml.
require (
	github.com/gogpu/wgpu v0.32.1
	github.com/timzifer/refract v1.2.0
	github.com/timzifer/refract/backend/gg v1.2.0
)

require (
	github.com/go-webgpu/goffi v0.6.3 // indirect
	github.com/go-webgpu/webgpu v0.5.5 // indirect
	github.com/gogpu/gpucontext v0.29.0 // indirect
	github.com/gogpu/gputypes v0.6.0 // indirect
	github.com/gogpu/naga v0.19.0 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
