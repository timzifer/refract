module github.com/timzifer/refract/backend/window

go 1.25.0

// GoGPU is pinned to an exact version, for the same reason the gg backend pins
// gg: the stack is young, and a release of this backend is validated against
// exactly one release of the window layer and says which. gogpu v0.52 is the
// one that resolves gg v0.52.5's own gputypes and gpucontext versions; a newer
// gogpu pulls a gputypes that gg at this pin cannot build against.
require (
	github.com/gogpu/gogpu v0.52.1
	github.com/timzifer/refract v0.2.0
	github.com/timzifer/refract/backend/gg v0.2.0
)

require github.com/gogpu/gpucontext v0.28.0

require (
	github.com/go-webgpu/goffi v0.6.3 // indirect
	github.com/go-webgpu/webgpu v0.5.5 // indirect
	github.com/gogpu/gg v0.52.5 // indirect
	github.com/gogpu/gputypes v0.5.2 // indirect
	github.com/gogpu/naga v0.18.0 // indirect
	github.com/gogpu/wgpu v0.31.6 // indirect
	golang.org/x/image v0.44.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)
