module github.com/timzifer/refract/backend/gg

go 1.25.0

// GoGPU is pinned to an exact version. The stack is young and moves fast, so
// each release of this backend is validated against exactly one gg release and
// says which.
require (
	github.com/gogpu/gg v0.52.5
	github.com/timzifer/refract v1.0.0
	golang.org/x/image v0.44.0
)

require (
	github.com/gogpu/gpucontext v0.28.0 // indirect
	github.com/gogpu/gputypes v0.5.2 // indirect
	golang.org/x/text v0.40.0 // indirect
)
