module github.com/timzifer/refract/arrow/v18

go 1.25.0

// Apache Arrow is pinned to an exact version, for the same reason the gg
// backend pins gg: an adapter is validated against exactly one release of what
// it adapts, and says which.
require (
	github.com/apache/arrow-go/v18 v18.7.0
	github.com/timzifer/refract v1.0.0
)

require (
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/klauspost/cpuid/v2 v2.4.0 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	golang.org/x/exp v0.0.0-20260112195511-716be5621a96 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
