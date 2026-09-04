// Package svg is refract's built-in, zero-dependency SVG backend.
//
// It emits SVG using nothing but the standard library. That is its whole
// reason to exist: the leanest use case — "give me a chart as SVG on a
// server" — should pull in no rendering engine, no font stack, and nothing
// young. Importing refract and this backend yields a stdlib-only dependency
// graph and a static binary.
//
// # Text
//
// Text is emitted as <text> elements, so the viewer or browser shapes it. For
// layout, this backend measures with internal/fontmetrics: exact advances when
// a font file is supplied via [WithFont], and a built-in generic-sans table
// otherwise. Geometry is therefore identical to any other backend given
// identical metrics, but the metrics themselves differ slightly between
// backends, so the layout does too — the accepted imprecision in refract's
// "one model, identical output" promise. The bounds are measured in
// backend/gg/parity_test.go and explained in docs/adr/0003.
//
// # Determinism
//
// Attribute order and number formatting are fixed, and nothing here iterates a
// map or reads a clock, so the same chart produces byte-identical SVG on every
// run on a given machine.
//
// Across architectures it does not, and no amount of care here would change
// that: Go may contract a*b+c into a fused multiply-add, arm64 does and amd64
// does not, and a float32 coordinate can consequently differ in its last bit.
// Golden-file tests therefore compare with internal/svgdiff — everything but
// the numbers exactly, coordinates to within a hundredth of a pixel.
package svg
