# 0022 — The GPU tier is opted into by importing a module

**Status:** Accepted · **Date:** 2026-09-04

## Context

CONCEPT §12 describes two decoupled big-data tiers. The CPU tier — aggregate
before rendering — shipped in v0.4. The GPU tier is v0.6: "interactive pan/zoom
over the full dataset at framerate via the gg GPU backend".

[ADR 0006](0006-gg-coupling-surface.md) forbids `backend/gg` from importing
`gg/gpu`, and every reason still holds. Importing it pulls `wgpu`, `naga` and
`go-webgpu/goffi` into the build; it wants a working Vulkan, Metal or DX12 at
run time; and it does not compile for js/wasm at all, which would cost the
module a platform. `backend/gg` is the supported raster path and has to stay
small, portable and boring.

So the tier has to be reachable without being in the way. Three ways to do that:
a build tag on `backend/gg`, a runtime switch, or a separate module.

A build tag would mean `backend/gg`'s dependency graph depends on how it is
built, which makes "what does importing this cost" unanswerable and makes CI
build the module twice to know it compiles. A runtime switch is not possible:
gg's GPU tier is activated by a package's `init`, so the dependency is decided
at link time whatever the switch says.

## Decision

**`backend/gg/gpu` is a nested module whose import is the opt-in.**

```go
import (
    ggbackend "github.com/timzifer/refract/backend/gg"
    _ "github.com/timzifer/refract/backend/gg/gpu" // GPU tier
)
```

- The module is one file of substance: a blank import of `gg/gpu`, which
  registers gg's GPU accelerator and its tile-based coverage filler, plus
  `Enabled` and `Close`.
- A nested module is excluded from its parent's module graph, so `backend/gg`'s
  own dependencies are unchanged and `go list -deps` on it still shows gg,
  `x/image` and the core. That is the same mechanism [ADR 0001](0001-module-layout.md)
  uses to keep the core stdlib-only, one level down.
- gg is pinned to exactly the version `backend/gg` pins. This module switches a
  tier on inside that build of gg rather than being a renderer of its own, so
  the two pins are one pin.
- Nothing else changes. The IR, the geoms, the marks and the golden files are
  the same; what changes is which coverage filler rasterizes them.

## Consequences

- The tier accelerates everything drawn through gg, which since v0.6 includes
  the native window — so "interactive pan and zoom over the full dataset" is
  reached by adding one import to a program that already has a window.
- Registration fails quietly on a machine with no usable device and gg falls
  back to the CPU. A chart still renders, which is the only acceptable
  behaviour; `Enabled` reports which way it went for a program that would rather
  say so.
- The module does not build for js/wasm, because `gg/gpu` does not. CI
  cross-compiles the other three modules for js/wasm and this one for the
  desktop targets, and the browser keeps the canvas 2D path
  ([ADR 0017](0017-browser-backend.md)).
- It stays **opt-in beta** past v1.0, which is CONCEPT §14's position and not a
  temporary caveat: for high-volume server-side stills the CPU rasterizer and
  the vector emitters remain the supported path.
- What this does not do is float64 origin rebasing, which CONCEPT §12 lists
  beside it. That turned out to be a property of the *domain*, not of the
  rasterizer — refract's device coordinates are pixels within a canvas and never
  large — so it landed in `scale` as `scale.Origin` and is documented there. The
  precision a deep zoom loses is in the float64 a timestamp becomes, and no
  amount of GPU changes that.

## Revisit if

gg's GPU tier stops being init-registered, or grows a way to be selected per
context. Then the tier could be a target option in `backend/gg` rather than a
module, and the import-as-opt-in would be an awkward way to say something the
API could say plainly.
