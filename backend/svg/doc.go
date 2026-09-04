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
// otherwise. Geometry is therefore identical to any other backend, while text
// metrics are only approximately identical — the single accepted imprecision
// in refract's "one model, identical output" promise (CONCEPT.md §9).
//
// # Determinism
//
// Attribute order and number formatting are fixed, so the same chart produces
// byte-identical SVG on every run and on every platform. Golden-file tests
// depend on this.
package svg
