module github.com/timzifer/refract/backend/window

go 1.25.0

// GoGPU is pinned to an exact version, for the same reason the gg backend pins
// gg: the stack is young, and a release of this backend is validated against
// exactly one release of the window layer and says which.
//
// gogpu v0.53.1 is the newest release that still resolves against gg v0.52.5.
// wgpu v0.33 turned the render-pass calls into struct arguments — Draw(a, b, c,
// d) became Draw(gputypes.DrawArgs{...}) — and gg at this pin has not followed,
// so anything that drags wgpu to v0.33 or later breaks gg's own internal/gpu
// package rather than any code here. gogpu v0.53.2 does exactly that, and so
// does gpucontext v0.30 and later by way of a DeviceProvider that gained a
// method gogpu v0.53.1 does not implement. The whole GoGPU stack moves as a
// unit and gg leads it; the ceiling lifts when gg ships a release built against
// the new wgpu. Dependabot is told to hold at it in .github/dependabot.yml.
require (
	github.com/gogpu/gogpu v0.53.1
	github.com/timzifer/refract v1.5.0
	github.com/timzifer/refract/backend/gg v1.5.0
)

require github.com/gogpu/gpucontext v0.29.0

require (
	github.com/go-webgpu/goffi v0.6.3 // indirect
	github.com/go-webgpu/webgpu v0.5.5 // indirect
	github.com/gogpu/gg v0.52.5 // indirect
	github.com/gogpu/gputypes v0.6.0 // indirect
	github.com/gogpu/naga v0.19.0 // indirect
	github.com/gogpu/wgpu v0.32.1 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
