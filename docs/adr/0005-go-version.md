# 0005 — Go 1.25 is the minimum

**Status:** Accepted · **Date:** 2026-09-04 · **Closes:** CONCEPT.md §17.6

## Context

§17.6 flagged the Go version as open, noting that GoGPU requires Go 1.25+ and
that the zero-CGO FFI path has shown sensitivity to Go's internal ABI across
releases.

## Decision

Both modules declare `go 1.25.0`.

The floor is set by the dependency, not by preference: `github.com/gogpu/gg`
declares `go 1.25.0`, and its `text.Face` interface returns `iter.Seq[Glyph]`.
There is no version of the gg backend that compiles below 1.25.

The core module has no such constraint of its own — it is stdlib-only — but is
held at the same floor so that the two modules never disagree about which
toolchain builds this repository.

## Consequences

- CI builds and tests on the declared minimum as well as on the current stable
  toolchain, so the declaration is checked rather than asserted.
- The ABI sensitivity §17.6 worries about lives in `goffi`, which is reached
  only through `wgpu`, which is reached only through `gg/gpu` — and
  [0006](0006-gg-coupling-surface.md) keeps `gg/gpu` out of the graph entirely
  in v0.1. So the concrete risk that motivated the question does not apply yet.
  It becomes live when the GPU tier is enabled at v0.6.

## Revisit if

The GPU tier lands and turns out to need a narrower or later toolchain range
than the CPU path, in which case the two modules may need to diverge.
