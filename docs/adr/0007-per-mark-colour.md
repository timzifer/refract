# 0007 — Colour varies per mark without changing the IR

**Status:** Accepted · **Date:** 2026-09-04 · **Closes:** —

## Context

v0.2 adds colour scales: `geom.ColorBy("depth", scale.Sequential(palette.Viridis))`
gives every point in a scatter its own colour from a continuous ramp.

The v0.1 IR carries one style per drawing call. `Markers(shape, at, style)` takes
a slice of positions and a single `MarkerStyle`; `FillPath(p, fill, rule)` takes
one `Fill`. Neither has a per-vertex colour channel. So a naive per-mark colour
does not fit, and the obvious move is to add one — an `[]Color` alongside the
positions, or a `Fill` that can vary along a path.

ADR 0002 froze the IR and the `Backend` interface for the v0.1 cycle. That freeze
has expired with the cycle, so the question is live rather than closed: should
v0.2 widen the IR?

## Decision

**No. Geoms batch their marks by colour and emit one call per distinct colour.**

`geom.groupByColor` walks the marks once, buckets them by resolved colour, and
emits one `Markers` (or one `FillPath`) per bucket, in order of first
appearance. Nothing about the IR or the `Backend` interface changes, and no
backend has to learn anything.

Considered and rejected:

- **A parallel `[]Color` on `Markers`.** Every backend gains a code path; the SVG
  emitter loses the ability to write one `<g>` with a shared `fill`; and the
  gradient case (`Fill.Stops`) still would not compose with it. It buys a call
  count, not a capability.
- **A per-mark `Fill` on `FillPath`.** Meaningless: a path has one interior.

## Consequences

- **Cost is one call per distinct colour, not one per mark.** For a categorical
  colouring that is a handful. For a continuous ramp over real data it is
  bounded by how many distinct 8-bit sRGB triples the ramp actually produces for
  the domain, which is far fewer than the point count on any chart big enough for
  the difference to matter — and where it is not, that chart wants a density
  raster (`Backend.Image`, v0.4), not a million markers.
- **Draw order within a layer changes.** Marks are drawn grouped by colour rather
  than in row order. Overlapping semi-transparent marks can therefore composite
  differently than a strict row-order render would. This is why grouping is by
  *first appearance* rather than by map iteration: the order is at least stable
  and reproducible, which is what the golden tests need.
- **Bars are not batched at all** — each gets its own `FillPath`, because bars in
  one path share a fill by construction. The saving would be illusory.

## Revisit if

A geom appears whose marks are genuinely all distinct — a per-point-coloured
scatter of a hundred thousand rows that is *not* better served by decimation —
or when v0.4's allocation pass measures the batching and finds it is the thing
in the way.
