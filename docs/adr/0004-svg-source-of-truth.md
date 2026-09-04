# 0004 — The built-in emitter is the only SVG path in v0.1

**Status:** Accepted · **Date:** 2026-09-04 · **Closes:** CONCEPT.md §17.2

## Context

Two things can produce SVG: refract's own `backend/svg`, which uses nothing but
the standard library, and gg's recording API through `gg-svg`. §17.2 framed the
choice as parity versus leanness — if the gg backend emitted SVG through gg,
raster and vector output would come from one rasterizer and agree exactly.

## Decision

In v0.1 the built-in emitter is the only SVG path. `backend/gg` produces raster
only: PNG and JPEG.

## Consequences

- The lean case stays genuinely lean. "Give me an SVG on a server" links no
  rendering engine, no font stack and no GoGPU.
- SVG and raster output are produced by different code, so they agree only as
  closely as their text measurement does — see [0003](0003-text-and-fonts.md),
  where that difference is measured rather than assumed.
- The built-in emitter must be deterministic, because golden files compare
  against it. Attribute order and number formatting are fixed for that reason,
  and `TestOutputIsDeterministic` holds it to it. That buys determinism *on one
  machine*: it does not buy bit-identical output across architectures, because
  Go contracts `a*b + c` into an FMA on arm64 and not on amd64, so a coordinate
  can differ by a float32 unit in the last place. Golden SVG is therefore
  compared with `internal/svgdiff` — everything but the numbers exactly,
  coordinates to within a hundredth of a pixel.
- PDF, which §14 puts at v0.3, will have to come through gg's recording API.
  When it does, the question of whether SVG should follow it there is worth
  reopening — one code path producing both vector formats is a real
  simplification.

## Revisit if

PDF lands and brings the gg recording API into the backend anyway, or if the
divergence in [0003](0003-text-and-fonts.md) turns out to matter to someone in
practice.