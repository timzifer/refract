# 0002 — The v0.1 IR and `Backend` interface

**Status:** Accepted · **Date:** 2026-09-04

## Context

`CONCEPT.md` §9 calls for a small, backend-agnostic scene description, frozen
early in v0.1, that maps cleanly onto an immediate-mode canvas and onto a
zero-dependency SVG emitter. Freezing it early is the point: everything above it
— geoms, layout, guides — is written against it, and everything below it is an
adapter.

## Decision

The primitive set is `Polyline`, `StrokePath`, `FillPath`, `Text`, `Markers`,
`Image`, and the `Push`/`Pop` transform-and-clip pair, plus `Measure` and
`Flush`. See `ir/backend.go` for the interface as it actually stands.

Four choices differ from the §9 sketch, deliberately:

**`StrokePath` was added.** §9 listed `Polyline` and `FillPath` but nothing that
strokes an arbitrary path. A tension-smoothed line is a stroked cubic path, and
`geom.Line` needs one the moment `geom.Tension` is non-zero. Filling a stroke
outline in the model layer would mean reimplementing stroking — exactly the work
§3 says refract does not do.

**`Polyline` stays, even though `StrokePath` subsumes it.** It is the hot path
for line geoms: it lets a backend consume points directly instead of allocating
and walking a path, and it is what a GPU backend would batch.

**`ir.Point` is a plain `struct{ X, Y float32 }`.** The sketch wrote
`f32.Point`; a package for two fields is not worth its import. Positions are
float32 because they are device-space values the model layer has already
computed in float64, and float32 is what a GPU consumes.

**Paths carry ops and points in two parallel slices.** A path is two allocations
regardless of length, and `Path.Reset` lets a caller reuse both across frames —
which is what §11's allocation discipline needs.

Everything else follows §9, including the crucial one: `Text` carries a string
and a font reference, never glyphs. Shaping belongs to the backend.

## Consequences

- The interface is frozen for the v0.1 cycle. Pre-1.0 it may still change
  between minor releases (§15), but not within one.
- Gradients are linear-only. No v0.1 geom needs radial or sweep, and adding them
  later is additive rather than breaking.
- `Backend` has no state beyond the `Push`/`Pop` stack: every drawing call
  carries its own style. That makes a backend adapter a pure translation and
  removes a whole class of "who set this state last" bugs.

## Revisit if

A geom needs a primitive that cannot be expressed as a path — the obvious
candidate is a genuine instanced-marker fast path with per-instance colour,
which `Markers` currently does not support.
