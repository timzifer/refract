# 0021 — The native window rasterizes on the CPU and presents one texture

**Status:** Accepted · **Date:** 2026-09-04

## Context

CONCEPT §14 puts a native interactive window in v0.6, "via `gogpu/gogpu`", and
notes that the event system it needs already exists: `Plot.Live` takes any
`ir.Target`, and a window backend has only to implement `ir.Backend` and, for
cheap repaints, `ir.Partial`.

That leaves the question the roadmap does not answer: what draws the marks. A
window is a GPU surface. There are three ways to put a chart on one.

1. **Draw on the GPU directly**, through `gg/gpu` and `wgpu`. gg's GPU tier
   accelerates fills and complex paths inside a gg context — it is not a
   separate renderer with its own API, so "drawing on the GPU" still means
   drawing through gg, and the difference is which coverage filler runs.
   [ADR 0006](0006-gg-coupling-surface.md) keeps `gg/gpu` out of `backend/gg`
   for good reasons: it pulls `wgpu`, `naga` and `goffi` into the build, needs a
   working Vulkan, Metal or DX12 at run time, and does not build for js/wasm.
2. **Write a second rasterizer** against the window's swapchain. That is a
   second implementation of every mark refract draws, and the day it disagrees
   with the first is the day a chart looks different on screen than in the file
   it exports.
3. **Rasterize with the backend that already exists** and present the result.

## Decision

**The window draws with `backend/gg`'s CPU rasterizer into memory, and presents
that memory to the GPU as one texture per changed frame.**

- `backend/gg` gains `Surface`: a target that keeps its pixels instead of
  encoding them, and whose backend implements `ir.Resizer` and `ir.Partial`.
  It is useful on its own — compositing a chart into a larger picture wants
  exactly this — and it is what the window draws into.
- `backend/window` is a **nested module** requiring `gogpu/gogpu` and
  `backend/gg`. Importing refract's core still yields a stdlib-only dependency
  graph, and a program that renders SVG on a server links no window layer. It is
  [ADR 0001](0001-module-layout.md)'s arrangement, applied again.
- `window.Window` is a surface and an event loop, and knows nothing about
  charts. `window/show` is a second package that joins one to a `*refract.Plot`.
  A backend must not import `geom`, `scale` or `render`, and the steering does;
  the core cannot hold the steering either, because a window is a dependency and
  the core has none. Two packages is what satisfies both — the same split
  `Live.Bind` makes between the root package and `backend/canvas`, moved into
  the module that can afford the import.
- The steering itself is `refract.Input`, in the core: a portable state machine
  that turns presses, moves and wheel notches into hovers, clicks, pans and
  zooms. `Live.Bind` was rewritten onto it, so the browser and the window share
  one implementation and one set of tests rather than two that drift.
- The GPU tier is available underneath, unchanged, by importing
  `backend/gg/gpu` — see [ADR 0022](0022-gpu-tier.md).

## Consequences

- A window shows exactly what a file would: same model, same geoms, same marks,
  same golden files. There is one rasterizer in the project.
- The cost is one texture upload per changed frame, and it is paid only when the
  frame changed. `Live.Draw` paints nothing when a frame is identical to the
  last one, gg stamps its pixel buffer with a generation that says whether it
  did, and the window compares two integers rather than two buffers. With
  gogpu's event-driven loop — which blocks on the OS queue when idle — a chart
  nobody is touching costs no CPU.
- Damage tracking reaches the window: `Surface`'s backend clips a frame to the
  damaged rectangles and clears them first, because compositing an antialiased
  chart over itself darkens every edge in it. The clip and the clear take the
  bounding box of the damage rather than each rectangle, which repaints a few
  pixels for nothing and saves the rest of the canvas.
- A very large window at a high frame rate is bounded by the CPU rasterizer, not
  by the GPU. That is the honest limit of this arrangement, and it is why the
  GPU tier exists as an opt-in underneath rather than as a second backend.
- gogpu is pinned exactly, to v0.52.1: it is the release whose `gputypes` and
  `gpucontext` versions match gg v0.52.5's. A newer gogpu resolves a `gputypes`
  that gg at this pin does not compile against, which is the pinning discipline
  in [ADR 0006](0006-gg-coupling-surface.md) meeting a second young dependency.
- The window cannot be tested by opening one: CI has no display. What is tested
  is everything that is not the window — the input state machine in the core,
  the wheel-unit conversion and double-click policy in the module, the surface
  in `backend/gg` — and the window layer itself is compiled on every platform in
  CI. That is the same bargain `backend/canvas` makes about a browser, and it
  has the same hole in it: nobody but a person at a screen finds out that the
  window opens.

## Revisit if

gg grows a way to draw into a caller's swapchain without the rest of the GPU
tier, or the CPU rasterizer becomes the frame budget on charts people actually
open. Either would make drawing directly onto the surface worth its second code
path — and the first thing to check then is still whether the two paths draw the
same picture.
