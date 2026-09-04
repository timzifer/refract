# 0017 — The browser backend is canvas 2D, in the core, and not gg

**Status:** Accepted · **Date:** 2026-09-04

## Context

CONCEPT §14 puts browser rendering behind the gg backend: "browser rendering via
the gg backend under `GOOS=js` (WebGPU; canvas fallback where WebGPU is
unavailable)". That is the plan of record, and it does not survive contact with
the dependency:

```sh
$ grep -rl "syscall/js" $(go env GOMODCACHE)/github.com/gogpu/gg@v0.52.5
$
```

gg at the pinned version has no js support at all. There is no WebGPU path to
take and no canvas fallback to fall back to. `gogpu/wgpu` advertises a browser
target, but reaching it means `gg/gpu`, which [ADR 0006](0006-gg-coupling-surface.md)
excludes for the whole CPU tier and which would put a young GPU stack on the
critical path of the one platform where a chart most needs to just appear.

This is the same shape as [ADR 0009](0009-pdf-backend.md): the roadmap routed an
output through gg, gg turned out not to be able to draw it, and the question is
what to do instead.

## Decision

**refract draws on the canvas 2D context itself, from a package in the core
module.**

- `backend/canvas` implements `ir.Backend` and `ir.Target` against a
  `CanvasRenderingContext2D` reached through `syscall/js`. `syscall/js` is the
  standard library, so this costs the core no dependency and
  [ADR 0001](0001-module-layout.md)'s rule is untouched — the same reason PDF
  lives in the core.
- Every file is behind `//go:build js && wasm`, so on any other platform the
  package is empty. A stub that compiled everywhere and failed at run time
  would let a chart reach production before finding out there is no canvas on
  a Linux server.
- Paths are built as SVG path data and handed to `Path2D` in one call.
  Crossing the WebAssembly boundary is what costs in a browser: a five-hundred
  point line is one string and one `stroke`, not five hundred `lineTo` calls,
  and a scatter is one path per style rather than one per point.
- `Measure` goes through `measureText` on the very context that will draw. That
  is [ADR 0003](0003-text-and-fonts.md)'s seam working as intended: a chart's
  margins are sized by the font the reader actually has, not by a table of what
  a font usually looks like.
- The backend implements `ir.Partial` ([ADR 0016](0016-streaming-and-damage.md)):
  a damaged frame clips to the rectangles, clears them and repaints only those.
- Event wiring — `Live.Bind` — is in the *root* package under the same build
  tag, not in the backend. A backend consumes IR and must not know about scales
  or panels; turning a wheel event into a zoom is not drawing.

## Consequences

- A chart in a browser is the same chart: the same model, the same geoms, the
  same scales, the same themes, the same decimation. The browser example builds
  a `*refract.Plot` exactly as a server would.
- No WebGPU, and so no GPU tier in the browser. For the CPU tier that is not a
  loss worth paying for: the marks a chart draws after decimation are in the
  thousands, and a 2D context draws thousands of paths comfortably. The GPU
  tier was always v0.6 and opt-in.
- The backend is tested against a recording context under node rather than
  against pixels. That is the choice the rest of the repository makes — assert
  on the primitives that were emitted — and here it is also the only one
  available: node has no canvas, and a test that needed one would be a test
  nobody could run. CI runs it as its own job.
- A canvas is a surface, not a document. `Plot.Live` is the entry point;
  `Plot.Render` into a canvas target draws one frame, which is what an export
  button wants.
- One more platform's text stack to be approximately right about. A browser
  that does not report `fontBoundingBox*` metrics falls back to the em box,
  which over-reports the ascent slightly and so spaces labels generously rather
  than overlapping them.

## Revisit if

gg ships a js target. Then there is a real choice between this backend and a
gg-backed one, and the arguments are the ones ADR 0006 already makes about how
much of gg to depend on — with the added weight that a second implementation of
the same picture is a second set of golden files.
