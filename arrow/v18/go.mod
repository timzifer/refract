module github.com/timzifer/refract/arrow/v18

go 1.25.0

// Apache Arrow is pinned to an exact version, for the same reason the gg
// backend pins gg: an adapter is validated against exactly one release of what
// it adapts, and says which.
require (
	github.com/apache/arrow-go/v18 v18.7.0
	github.com/timzifer/refract v0.2.0
)
