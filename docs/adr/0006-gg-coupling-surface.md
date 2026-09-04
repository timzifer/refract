# 0006 — How much of gg the adapter is allowed to touch

**Status:** Accepted · **Date:** 2026-09-04 · **Closes:** CONCEPT.md §17.1

## Context

§4a is honest about the bet: GoGPU is young, moves fast, and its native GPU
backends are unproven across the real hardware matrix. §17.1 asks how much of
gg's API the adapter should touch, and whether to track `main` or pin.

The mitigation refract already has is structural — the `ir.Backend` interface
means a gg API change is contained to one adapter. What remains is deciding how
large that adapter's contact patch is.

## Decision

**Pin exactly.** `backend/gg/go.mod` requires `github.com/gogpu/gg v0.52.5`. Each
release of the backend is validated against exactly one gg release and says
which. No version ranges, no tracking `main`.

**Import two packages and no more.** The adapter imports `github.com/gogpu/gg`
and `github.com/gogpu/gg/text`. It does **not** import:

- `gg/gpu` — that is what activates the GPU tier. Not importing it means no GPU
  device is ever created, the CPU rasterizer is what runs, and CI needs no
  graphics hardware. It also keeps `wgpu` and `goffi` out of the build entirely.
- `gg/scene`, `gg/recording` — retained-mode and vector export. Neither is
  needed until animation (post-1.0) and PDF (v0.3).

**Use the stable middle of gg's API.** The adapter touches the canvas surface
that has an obvious analogue in every 2D library — `NewContextWithScale`, path
building, `SetStroke`/`SetColor`/`SetFillRule`, `Fill`/`Stroke`, `Push`/`Pop`,
`Clip`, `Transform`, `DrawString`, `Image` — plus `text.NewFontSource` and
`Face`. These are the parts most likely to be stable, because they are the parts
every consumer uses.

## Consequences

- The whole adapter is about 300 lines. Following a breaking gg release is a
  bounded afternoon, not a project.
- Upgrading gg is a deliberate act with a diff, not something that happens on
  someone's machine because their module cache was warmer.
- The GPU tier, PDF and a native window are not merely unimplemented — they are
  actively excluded. Enabling them means revisiting this record, which is the
  point: the moment `gg/gpu` appears in an import block, the risk profile of
  this module changes and that should be visible in review.

## Revisit if

The GPU tier is enabled (v0.6), PDF lands (v0.3), or gg reaches a stability
commitment that makes an exact pin more cost than benefit.
