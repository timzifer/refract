# refract

**A grammar-driven plotting library for Go: one model, many backends, runs everywhere — built on the GoGPU stack.**

> Status: **pre-alpha.** Milestones **v0.1 through v0.4 are implemented** — see
> [§14](#14-roadmap--milestones) for what that covers and the
> [README](README.md) to use it. This document remains the working concept for
> everything past v0.4. The API is **not** stable: every release below `v1.0.0`
> may contain breaking changes without deprecation cycles (see
> [Versioning](#15-versioning--stability)) — v0.2 added a method to
> `data.Source`, which is exactly the kind of break that policy exists for.

> **Implementation note.** Where this document and the code disagree, the code
> wins and this document is wrong — please fix it. Decisions that were open in
> [§17](#17-open-decisions) and have since been settled are recorded in
> [docs/adr](docs/adr/), and are marked below.

> **Revision note (this version):** the rendering strategy has been rewritten
> around the **GoGPU** ecosystem — `gogpu/gg` (2D graphics), `gogpu/wgpu` (pure-Go
> WebGPU), `gogpu/gogpu` (windowing). An earlier draft assumed refract would need
> its own rasterizer, its own GPU path (possibly cgo/Vello), and its own text
> stack. `gogpu/gg` already provides all of that in pure Go with zero CGO, so
> refract now sits *on top of* it. This shrinks refract to its actual value —
> the grammar, data, and interaction layers — and keeps the whole stack cgo-free.
> See [§4a](#4a-relationship-to-gogpu) and [§10](#10-rendering-via-gogpugg).

---

## 1. One-paragraph pitch

`refract` turns a single, declarative chart specification into any output you
need — SVG, PNG, PDF, an interactive browser canvas, or a GPU-accelerated native
window — from **the same model**, with **the same visual result**. It renders
through the **GoGPU** stack, which is **pure Go with zero CGO**, so refract
compiles to a single static binary (`CGO_ENABLED=0`), runs unchanged on a server,
cross-compiles to any target, and runs in the browser via WebAssembly. GPU
acceleration exists and is also cgo-free, but it is strictly **opt-in**: the
default path is a high-quality CPU rasterizer. refract's own job is the part
GoGPU does not do — the grammar of graphics, the data layer, and interaction.

The name is the thesis. Refraction is what happens inside a prism: one beam enters,
a spectrum comes out. One scene enters `refract`; a spectrum of output formats
comes out.

---

## 2. Motivation

Go's plotting ecosystem is real but fragmented, and no single library owns the
combination this project targets:

- **`gonum/plot`** — the de-facto standard. Pure Go, static-only, verbose API,
  no grammar-of-graphics model, no faceting, weak big-data handling.
- **`go-echarts`** — emits HTML + JavaScript wrapping Apache ECharts. Great in a
  browser, but there is no Go renderer underneath; server-side PNG needs a
  headless browser. Chart logic lives in JS.
- **`go-chart`** — simple raster/SVG charts, limited model.
- **Gio / Cogent Core** — GUI toolkits, not plotting libraries; their desktop GPU
  paths historically pull in cgo.
- **Plotly / Vega-Lite** — the reference for declarative, serializable,
  interactive charts, but not Go.

Two things changed the calculus versus older Go plotting attempts. First, a
grammar-driven, serializable model (à la Vega-Lite) is now the expected shape of a
modern charting library. Second — and decisively — **the rendering layer that used
to be the multi-year blocker now exists in pure Go**: the GoGPU stack provides a
Skia-quality 2D engine, a pure-Go WebGPU implementation, and a windowing layer,
all cgo-free. That removes the single biggest reason ambitious Go plotters stalled.

So the unoccupied position is: **a grammar-driven Go plotting library whose entire
stack is cgo-free and portable, that renders identically across static, web, and
(optionally) GPU backends, and that scales up to big data.** That is the space
`refract` claims. It does not try to be a renderer; it is the grammar on top of one.

---

## 3. Guiding principles

1. **Pure Go, zero CGO, end to end.** The core, the standard backends, and the
   optional GPU path are all cgo-free. `CGO_ENABLED=0` builds and cross-compiles
   everywhere, including `GOOS=js` for the browser.

2. **Decouple "process millions of points" from "interact with millions of points
   at 60 fps."** The former is CPU aggregation (decimation, binning → raster),
   pure Go, covers big-data *stills* completely. Only the latter needs a GPU.

3. **One model → many backends, identical output.** The user builds a spec once.
   Backends are interchangeable consumers of a small intermediate representation.

4. **Don't build a renderer — stand on one, but stay insulated.** refract renders
   through `gogpu/gg`. It does **not** reimplement rasterization, text shaping,
   GPU pipelines, or vector export. To avoid being welded to a young, fast-moving
   dependency, refract defines its own thin `Backend` interface and small IR; the
   GoGPU integration is *one* backend behind that interface (see [§9](#9-intermediate-representation-ir), [§10](#10-rendering-via-gogpugg)).

5. **Correctness is testable or it isn't real.** Golden-image tests from day one.
   `gogpu/gg` already ships golden-image infrastructure and a deterministic CPU
   rasterizer; refract's tests build on that.

---

## 4. Positioning & differentiation

| | Rendering | Web / interactive | Big data | cgo | Model |
|---|---|---|---|---|---|
| gonum/plot | raster, SVG, PDF | no | none | free | verbose, no GoG |
| go-echarts | HTML + JS (ECharts) | yes (browser) | via ECharts | free¹ | chart-type |
| go-chart | raster, SVG | no | none | free | simple |
| Gio / Cogent Core | GPU native | yes | partial | cgo (desktop) | GUI toolkit |
| Plotly / Vega-Lite | Canvas / SVG (JS) | yes (reference) | yes | — (not Go) | GoG / spec |
| **refract (goal)** | SVG / raster / PDF / GPU / browser via one model | yes (Wasm, cgo-free) | two tiers | **zero CGO throughout** | GoG-lite + Vega-Lite-compatible spec |

¹ go-echarts is cgo-free but needs a headless browser for server-side images.

### The sharp USPs

1. **Zero-CGO end to end, including GPU.** Static binary on a server, Wasm in the
   browser, optional GPU in a native window — all without a C toolchain. No other
   Go charting library offers this whole span.
2. **One spec → many backends, identical output.** gonum/plot can't do
   web/interactive; go-echarts can't do native server-side images without a
   browser. refract does both from one specification.
3. **Big data in two decoupled tiers** — CPU aggregation always, GPU interaction
   optional.
4. **Vega-Lite-compatible JSON spec** — ship a chart from a Go backend to a
   Go-Wasm *or* JS frontend that renders it identically.
5. **First-class concurrency & allocation discipline**, inherited from and
   extending `gogpu/gg`'s zero-alloc hot paths.
6. **Serious text for free** — real shaping (GSUB/GPOS, ligatures, kerning, CJK,
   bidi, color emoji, variable fonts) via `gogpu/gg`'s pure-Go font stack.

### 4a. Relationship to GoGPU

refract is to `gogpu/gg` what a plotting library is to a canvas: gg is the canvas
and the pen; refract is the grammar that decides *what* to draw. The split of
responsibilities:

| Concern | Owner |
|---|---|
| Rasterization (CPU + GPU), AA, dashes, gradients, blend modes, clipping/masks | `gogpu/gg` |
| Text: font parsing, shaping (GSUB/GPOS), layout, MSDF/glyph-mask rendering | `gogpu/gg` |
| Vector export (SVG, PDF) | **refract** — two built-in emitters, no dependency. `gg-pdf` cannot draw geometry ([ADR 0009](docs/adr/0009-pdf-backend.md)) |
| GPU device / backends (Vulkan, Metal, DX12, GLES, software, browser WebGPU) | `gogpu/wgpu` |
| Native window + input | `gogpu/gogpu` |
| Linear-space color, HiDPI/device scale, damage tracking | `gogpu/gg` |
| **Scales, geoms, stats, coords, facets, guides, layout, ticks** | **refract** |
| **Data layer (columnar/batch, Arrow, decimation)** | **refract** |
| **Serializable spec (Vega-Lite-compatible), interaction/event model** | **refract** |

**Honest caveats about depending on GoGPU.** The stack is young (`v0.x`, rapid
release cadence, small maintainer base — its own materials describe reaching
~100K LOC extremely fast). The native GPU backends are impressive but unproven
across the full real-world hardware matrix, and the zero-CGO FFI approach (goffi)
has shown sensitivity to Go's internal ABI across versions. refract's mitigations:
render through refract's own `Backend` interface so a gg API change is contained to
one adapter; pin exact GoGPU versions; treat the **CPU rasterizer as the stable,
supported path** and the **native GPU tier as opt-in beta**; and keep a
**zero-dependency SVG backend** (see [§10](#10-rendering-via-gogpugg)) so the
leanest static use case does not depend on GoGPU at all.

---

## 5. Non-goals

- **Not a dashboarding framework.** No server, no WebSocket layer.
- **Not a GUI toolkit.** It renders into a window (via `gogpu/gogpu`); it does not
  own widgets or the event loop.
- **Not a grammar-of-graphics purist clone.** A pragmatic subset ("GoG-lite").
- **Not a renderer.** Rasterization, text, and GPU pipelines belong to GoGPU.
- **Not a text-layout engine.** Chart text is short single-line runs (ticks, axis
  titles, legends, annotations). Paragraph wrapping, BiDi paragraph flow, and
  editable/cursored text are **out of scope**; shaping is delegated to the active
  backend (see [§9](#9-intermediate-representation-ir), [§10](#10-rendering-via-gogpugg)).
- **No 3D until well after v1.0**, and only then tightly scoped.

**Positioning.** refract is a **standalone, permissively-licensed** library. It
never depends on a specific application framework. The dependency runs the other
way: a UI framework such as `lux` may *embed* refract as a charting widget and
supply its own text/rendering backend. This keeps refract's license unencumbered
(it can be embedded by projects with any license, including dual-licensed ones)
and forbids pulling framework-specific code (e.g. a UI framework's rich-text
stack) into the core.

---

## 6. Architecture overview

One-directional lowering from a high-level model to a backend:

```
   User spec (GoG-lite API)  ──►  Model  ──►  IR  ──►  Backend interface  ──►  output
   ────────────────────────      ─────      ────      ─────────────────       ──────
   geoms, scales, coords,        resolved   small     svg (built-in) │         SVG
   facets, theme, guides         layout,    backend-  gg  (adapter) ─┼──►      PNG/PDF/
                                 ticks,     agnostic                 │         GPU frame/
                                 mappings   drawing                  │         window/
                                            ops                      │         browser
```

- **Data** enters through a batch/columnar interface ([§7](#7-data-layer)).
- **The model** ([§8](#8-model-layer-gog-lite)) resolves the abstract chart into
  concrete geometry and text runs.
- **The IR** ([§9](#9-intermediate-representation-ir)) is a small set of drawing
  primitives consumed by any `Backend`.
- **Backends** ([§10](#10-rendering-via-gogpugg)): a built-in zero-dependency SVG
  emitter, and a `gg` adapter that unlocks raster, PDF, GPU, browser, and
  interactive rendering — all cgo-free.

### Design tenets that resolve known tensions

- **Batch data access, not scalar** — the data interface returns typed columns,
  never one value at a time ([§7](#7-data-layer)).
- **Float64 origin rebasing for deep zoom** — subtract a per-view f64 anchor on the
  CPU, hand gg/GPU f32 deltas; never "fix it in the shader" after precision is lost.
- **Thin IR, adapter-based lowering** — refract's IR is small and stable; the gg
  adapter maps it to `gg.Context`/`scene`, insulating refract from gg churn.
- **Decimation is a family, not an algorithm** — LTTB (lines), min/max-per-column
  (envelopes), density binning (point clouds) ([§12](#12-big-data-strategy)).

---

## 7. Data layer

**Batch/columnar in, zero-copy where possible, Arrow as an optional adapter.**

```go
// DataSource exposes columnar, batch access. Implementations return typed
// slices the caller must not mutate; refract never copies when it can borrow.
type DataSource interface {
    Len() int
    Columns() []string
    Float64Column(name string) (data []float64, ok bool) // read-only view
    // TimeColumn, StringColumn, etc.
}
```

- **Zero-copy common case** — a `[]float64`-backed source returns its slice
  directly.
- **Arrow adapter as a separate module** (`refract/arrow`) so the core never links
  Apache Arrow. Shipped in v0.4: a null-free `float64` column is borrowed
  outright, everything else converts once and caches, and an Arrow null becomes
  a `NaN` so that one missing-data policy covers both
  ([ADR 0013](docs/adr/0013-arrow-adapter.md)).
- **Missing-data policy is explicit** — `NaN`/`Inf`: interpolate, gap, or error.
  (gg is already NaN-safe at the path level, so a gap never corrupts a render.)
  Since v0.2 the same policy covers a value the *scale* cannot place — zero on a
  log axis, a category outside a fixed set — because from the chart's point of
  view those are the same failure ([ADR 0008](docs/adr/0008-categorical-axes.md)).
- **Streaming** via a `StreamSource` with a snapshot model
  ([§11](#11-concurrency--allocation)): produce on one goroutine, render a
  consistent snapshot on another.

---

## 8. Model layer (GoG-lite)

The part refract actually builds. Building blocks:

- **Scales** — `Linear`, `Log`, `SymLog`, `Time`, `Ordinal`/categorical, plus
  color scales (sequential, diverging, qualitative). Scales own data→visual
  mapping and tick generation.
- **Coordinate systems** — Cartesian for v1; the transform is a **pluggable stage**
  so polar/geographic slot in later without touching geoms.
- **Geoms** — `Line`, `Scatter`, `Bar`, `Area`, `Step`, `Boxplot`, … Geoms know
  *what* their shape is, never *how* it's rendered.
- **Stats / transforms** — `Bin` (histogram), `Density` (KDE), `Smooth`
  (regression/loess), aggregation. A histogram is `geom.Bar` + `stat.Bin`.
- **Facets** — `facet.Wrap` / `facet.Grid`, shared or free scales.
- **Guides** — legends, colorbars, axes as positionable elements.
- **Annotations** — reference lines, spans, callouts, arrows.
- **Theme** — global tokens (fonts, palettes, grid styling, dark/light), applied
  as a resolved layer.

### Layout

A **constraint-based layout** (flex/grid-like) with margins, padding, relative
sizing, and — critically — **axis alignment** across subplots. Faceting builds on
it. (gg supplies device-scale/HiDPI awareness underneath.)

---

## 9. Intermediate representation (IR)

A small, backend-agnostic scene description, kept deliberately thin so it maps
cleanly onto `gg.Context` (immediate) or `gg/scene` (retained) — and onto the
built-in SVG emitter without gg.

Primitive set (frozen early in v0.1):

- `Polyline` — points, stroke style (width, dashes, joins, caps).
- `FilledPath` — path + fill (solid / gradient), fill rule.
- `TextRun` — a single-line string with font reference, size, style, and anchor
  position. refract passes strings, never pre-shaped glyphs: **shaping belongs to
  the active backend** (gg's shaper for the gg backend; the SVG viewer for the SVG
  backend; a host framework's text stack if refract is embedded). refract owns only
  *placement* — formatting, rotation, and overlap/collision avoidance. Paragraph
  layout is not represented in the IR (see [§5](#5-non-goals)).
- `Marker` / `Instances` — one marker shape at N positions (scatter fast path;
  maps to gg SDF/instancing on GPU).
- `Image` — raster blit (the density-raster big-data path).
- `Group` / `Clip` — transform + clip region, nestable.

The `Backend` interface is what every renderer implements:

```go
type Backend interface {
    Polyline(pts []f32.Point, style Stroke)
    FillPath(p Path, fill Fill, rule FillRule)
    Text(run TextRun)
    Markers(shape Marker, at []f32.Point, style Style)
    Image(img image.Image, dst f32.Rect)
    Push(clip *Path, xform Affine); Pop()
    Flush() error

    // Measure reports advance width and bounding box for a run, so layout can
    // size margins, space ticks, and detect label overlap. It is the ONLY text
    // capability refract requires beyond drawing — refract has no shaper.
    Measure(run TextRun) TextMetrics
}
```

The `Measure` seam is the whole of refract's "text problem": layout needs exact
metrics from whatever shaper will actually draw the text. The gg backend answers
from gg's shaper (exact); a host-framework backend answers from its own stack
(exact); the built-in SVG backend answers from a lightweight pure-Go metrics
reader (`hmtx`/`cmap`, approximate for complex scripts — acceptable, since SVG
text is shaped by the viewer anyway).

**Corrected by implementation.** Geometry is identical across backends *given
identical metrics* — but the metrics are not identical, so the layout differs
slightly, and the imprecision is a little wider than this section originally
claimed. Even when both backends are handed the same font file, gg hints glyph
advances to whole pixels while refract's own `hmtx` reader sums them unrounded,
and the two read vertical metrics from different tables. Measured for Go
Regular: advances differ by at most about half a pixel per glyph, the font box
height by about 15%, and the resulting plot rectangle by a few pixels. Those
bounds are asserted in `backend/gg/parity_test.go`, so they cannot drift
unnoticed; see [ADR 0003](docs/adr/0003-text-and-fonts.md).

Two implementations ship: `backend/svg` (pure `encoding/xml`, zero deps) and
`backend/gg` (adapter to `gogpu/gg`, in a separate nested module).

---

## 10. Rendering via gogpu/gg

The rendering tiers, organized by what they cost the user's build:

### The built-in SVG backend — zero dependencies

`backend/svg` emits SVG with nothing but the standard library. This is the leanest
possible target: a static chart with no GoGPU dependency at all. It exists so the
minimal use case ("give me an SVG on a server") pulls in nothing native and
nothing young. Text is emitted as `<text>` elements — the SVG viewer or browser
shapes them — and measured via a lightweight `hmtx`/`kern` reader for layout; no
shaper is linked.

### The gg backend — everything else, still zero CGO

`backend/gg` (module `github.com/timzifer/refract/backend/gg`) adapts the IR onto `gogpu/gg` and unlocks the
full span, all `CGO_ENABLED=0`:

- **Raster (PNG/JPEG/WebP)** via gg's CPU rasterizer (Skia-AAA analytic AA, smart
  per-path algorithm selection). This is the **stable, supported** rendering path.
- ~~**PDF and SVG** via gg's recording API + `gg-pdf` / `gg-svg`.~~ Both vector
  formats are built-in emitters in the core module instead. `gg-pdf` was tried
  and cannot draw geometry — its path operations reach a stub in `gxpdf` — so
  routing PDF through the recording API would have produced pages containing
  tick labels and nothing else ([ADR 0009](docs/adr/0009-pdf-backend.md)).
- **Text** — gg's pure-Go font stack: GSUB/GPOS shaping, ligatures, kerning,
  variable fonts, OpenType features, CJK, bidi, color emoji. refract adds no text
  dependency of its own.
- **GPU acceleration (opt-in beta)** via `import _ "github.com/gogpu/gg/gpu"` →
  `gogpu/wgpu` (Vulkan/Metal/DX12/GLES, or software fallback). Transparent CPU
  fallback when no GPU is present. This is the "60 fps pan/zoom over millions of
  points" tier, with float64 origin rebasing for deep zoom.
- **Browser (`GOOS=js`)** — gg/wgpu target browser WebGPU via `syscall/js`; Wasm
  has no cgo, so this is inherently cgo-free. Big-data interaction on the web.
- **Native window** via `gogpu/gogpu` for desktop interactive plots.

### What this buys refract (and what it must still not assume)

gg already provides: linear-space color, dashes, gradients, 29 blend modes,
clipping/masks, HiDPI/device scale, damage-aware partial repaint, zero-alloc hot
paths, and golden-image test infrastructure. refract inherits all of it.

What refract must **not** naively assume: that the young native GPU backends are
production-grade on every GPU (hence opt-in beta), and that gg's API is stable
(hence the `Backend` interface and pinned versions). For high-volume server-side
stills, prefer the CPU rasterizer or the built-in SVG emitter over the software
GPU path, which is a compatibility fallback, not a throughput path.

---

## 11. Concurrency & allocation

- **Parallel subplots** — independent subplots build IR on separate goroutines,
  composited at the end. Shipped in v0.4: each panel records into an
  `ir.Recorder` and the recordings replay into the backend *in panel order*, so
  a parallel render emits byte-identical output to a serial one
  ([ADR 0012](docs/adr/0012-parallel-panels.md)).
- **Snapshot streaming** — producer and renderer never share mutable state; the
  renderer reads an immutable snapshot (copy-on-swap). gg's damage tracking then
  repaints only dirty regions. **v0.5, not yet implemented.**
- **Allocation discipline** — refract keeps its IR construction allocation-light
  and leans on gg's zero-alloc fill/stroke/text paths. A benchmark gate asserts no
  per-frame allocations on the hot path. Shipped in v0.4: everything sized by the
  data comes from a pool, so a steady-state frame costs the same handful of
  allocations over a thousand rows and over a million. `TestARenderDoesNotAllocatePerPoint`
  is the gate.

---

## 12. Big-data strategy

Two decoupled tiers:

- **CPU tier (always, pure Go).** Aggregate before rendering — **shipped in
  v0.4**, in `stat/`, applied by geoms at draw time
  ([ADR 0011](docs/adr/0011-decimation.md)):
    - **LTTB** for line/time-series (sorted x, one y per x; lossy but shape-preserving).
    - **Min/max-per-pixel-column** for signal envelopes and staircases.
    - **Density binning → raster** (datashader-style) for large scatter / point
      clouds; emit an `Image` primitive.
- **GPU tier (opt-in).** Interactive pan/zoom over the full dataset at framerate
  via the gg GPU backend, with float64 origin rebasing for precision at deep zoom.
  **v0.6, not yet implemented.**

Default per geom and dataset size; user-overridable through `geom.Decimate`,
`geom.Budget` and `geom.NoDecimation`. The reduction happens in `Build`, never
in `Train`, so an axis always reports the data rather than the subset that
survived.

---

## 13. API sketch

**GoG-lite with ergonomic constructors and functional options.**

```go
package main

import (
    "github.com/timzifer/refract"
    "github.com/timzifer/refract/geom"
    "github.com/timzifer/refract/scale"
    "github.com/timzifer/refract/palette"
    "github.com/timzifer/refract/theme"

    ggbackend "github.com/timzifer/refract/backend/gg" // raster; PDF/GPU/browser later
)

func main() {
    // A []time.Time column and a []float64 column, both borrowed, not copied.
    src := refract.NewTable().Time("t", times).Float64("y", values)

    p := refract.New(
        refract.Theme(theme.Dark),
        refract.Size(800, 500),
        refract.Title("Signal"),
    )
    p.X(scale.Time())
    p.Y(scale.Linear(scale.Nice()))
    p.Add(
        geom.Line(src, geom.X("t"), geom.Y("y"),
            geom.Color(palette.Blue),
            geom.Tension(0.4),
            geom.OnMissing(geom.Gap),
        ),
    )

    // Lean, zero-dependency SVG:
    _ = p.Render(refract.SVG("signal.svg"))

    // Or raster via the gg adapter (PDF and GPU are later milestones):
    _ = p.Render(ggbackend.PNG("signal.png"))
}
```

Serialization and the web workflow — **v0.5, not yet implemented**:

```go
spec, _ := p.Spec().MarshalJSON()   // Vega-Lite-compatible where feasible
// ship `spec` to a Go-Wasm or JS frontend, which renders it identically
```

Interactivity (native window or browser, via the gg backend) — **v0.5/v0.6, not
yet implemented**. `Spec` and `On` are deliberately absent from the v0.1 API
rather than stubbed, so that nothing compiles against a shape that has not been
designed yet:

```go
p.On(refract.Hover, func(ev refract.HoverEvent) { /* ev.Point, ev.Series */ })
p.On(refract.Zoom,  func(ev refract.ZoomEvent)  { /* ev.Rect */ })
```

---

## 14. Roadmap & milestones

Because rendering is delegated to gg, effort concentrates on the model, data, and
interaction. Each milestone has a Definition of Done. Pre-1.0, breaking changes
between milestones are expected.

### v0.1 — "It plots" (static) — **shipped**

- IR + `Backend` interface defined and frozen for the cycle
  ([ADR 0002](docs/adr/0002-ir-and-backend.md)).
- Two backends: built-in `svg` (zero deps) and `gg` raster (text/AA via gg).
- Scales: linear + time; extended-Wilkinson ticks; dedicated time-axis ticks.
- Geoms: line, scatter, bar; axes, title, basic legend.
- Golden-image tests: golden SVG in the core and golden PNG in the gg backend,
  both compared with a tolerance below anything visible — bit-identical float
  results are not available across architectures — plus a cross-backend parity
  test.
- **API model decided and documented** (GoG-lite, per [§13](#13-api-sketch)).
- *DoD:* static SVG + PNG, `CGO_ENABLED=0`, gg pinned. ✔

Beyond the stated DoD, v0.1 also carries: tick-label collision avoidance, a
missing-data policy per geom (gap / interpolate / error), light and dark themes
with a colourblind-safe palette, and a generated figure gallery that CI
re-renders and checks against the committed images.

### v0.2 — Data layer & scales — **shipped**

- Columnar/batch `DataSource`; zero-copy for `[]float64`. `StringColumn` closes
  the interface sketched in [§7](#7-data-layer), so a table can carry categories
  alongside numbers and times.
- Log, symlog, ordinal/categorical scales. Log and symlog emit minor ticks; the
  ordinal scale is a band scale, so bars and boxplots take their width from it
  ([ADR 0008](docs/adr/0008-categorical-axes.md)).
- `NaN`/`Inf` policies (interpolate / gap / error), extended to cover values a
  scale cannot place — zero on a log axis is the same failure as a `NaN`, and
  one policy answers both.
- Geoms: area (with `Y2` for bands), step (pre/mid/post), boxplot (Tukey
  whiskers, type-7 quartiles, outliers).
- Color scales mapped onto gg's linear-space pipeline — ramps interpolate in
  linear light, not in sRGB bytes — with colorblind-safe sequential (Viridis,
  Cividis, Magma) and diverging (blue/orange, purple/green) palettes.
- *DoD:* a chart can be built over categories, over orders of magnitude, and
  over signed data that crosses zero, and a mark's colour can come from the
  data. ✔

Colourbars are **not** in v0.2: a layer coloured from a continuous scale
contributes no legend entry rather than a swatch that would misrepresent it.
Guides are v0.3.

### v0.3 — Layout, theming, PDF — **shipped**

- Constraint layout for subplots with axis alignment; faceting (wrap/grid).
  `layout.Panels` solves one grid: per-column left gutters and per-row bottom
  gutters, with every panel the same size, so a position means the same thing
  in every panel. `Compute` is that solver over a one-by-one grid, which is
  what keeps a lone chart and a facet of one identical.
- Theme engine: `theme.Tokens` holds the dozen choices a theme actually makes
  and `theme.Build` derives the other forty, `Theme.With` edits a built theme,
  and a registry resolves one by name for a config file or a flag.
- Annotations (`HLine`, `VLine`, `HBand`, `VBand`, `Segment`, `Region`,
  `Note`), colourbars, and a guide column that stacks them beside the plot.
- PDF output. **Not** through gg recording and `gg-pdf`: that library cannot
  draw geometry — its path operations reach a stub in `gxpdf` that has been a
  `TODO` since v0.4.0 and still is at v0.9.4 — so refract emits PDF itself,
  from the core module, with no dependency at all
  ([ADR 0009](docs/adr/0009-pdf-backend.md)).
- *DoD:* a chart can be split into small multiples with aligned axes, annotated
  with things that are not data, given a guide for a continuous colour scale,
  restyled by editing tokens rather than fifty fields, and written as a vector
  PDF. ✔

Colourbars close the gap v0.2 left open: a layer using `geom.ColorBy` now
contributes a colour guide instead of nothing.

### v0.4 — Big data (CPU tier) — **shipped**

- Decimation family in `stat/`: LTTB, min/max-per-pixel-column, density binning
  → raster. Geoms apply it at draw time, on device coordinates, and choose one
  by mark and by size unless told otherwise
  ([ADR 0011](docs/adr/0011-decimation.md)). `Train` still sees every row, so a
  reduced chart's axes are the data's, not the subset's.
- Allocation pass; a benchmark gate on per-frame allocations. Everything sized
  by the data comes from a pool, and `TestARenderDoesNotAllocatePerPoint`
  asserts that a frame over a million rows allocates what a frame over a
  thousand does. Along the way, feeding a column into a scale row by row
  through a variadic interface method turned out to cost one allocation per
  row; it is now one call per column.
- Arrow adapter as a separate module, `refract/arrow`. A null-free `float64`
  column is Arrow's own buffer; everything else converts once and caches; an
  Arrow null becomes `NaN`, so the missing-data policy that was already there
  covers it ([ADR 0013](docs/adr/0013-arrow-adapter.md)).
- Parallel subplot rendering. Each panel records into an `ir.Recorder` on its
  own goroutine and the recordings replay **in panel order**, so the output is
  byte-identical to a serial render and one set of golden files covers both
  ([ADR 0012](docs/adr/0012-parallel-panels.md)). `scale.Snapshotter` is what
  removes the sharing that panels sharing an axis otherwise have.
- *DoD:* a chart over a million rows renders, with no option set and no spike
  lost, in around 60 ms into under 30 kB of SVG — where drawing every row takes
  six times as long and produces 15 MB — and the same chart redrawn every frame
  allocates nothing that grows with its data. ✔

Not in v0.4, in case they look like oversights. Streaming is v0.5: there is no
`StreamSource` and no snapshot/swap, because the interesting half of that is
damage-aware repaint, which needs the interactive backends. `stat` carries the
decimation family only — smoothing, regression and hexbin are stats rather than
big-data machinery, and they belong with the geoms that would draw them. And
the GPU tier is untouched, which is the point of the CPU tier: big-data
*stills* are complete without it.

### v0.5 — Web & interactivity

- Browser rendering via the gg backend under `GOOS=js` (WebGPU; canvas fallback
  where WebGPU is unavailable).
- Event system: hover/click/zoom/pan with IR-level hit-testing.
- Full-spec JSON serialization (Vega-Lite-compatible where feasible).
- Streaming with snapshot/double-buffer + gg damage-aware updates.

### v0.6 — Native interactive & polish

- Native interactive window via `gogpu/gogpu`.
- GPU tier enabled (opt-in) via `gg/gpu`; float64 origin rebasing for deep zoom.
- Math typesetting for labels (optional, pluggable).
- Responsiveness (line widths/font sizes on resize — leans on gg device scale).
- Accessibility: redundant encoding (patterns/dashes), SVG `title`/`desc`/ARIA,
  data-table fallback.

### v1.0 — Stable & complete enough

- API freeze; semver; stable registration/extension model for third-party geoms
  and backends.
- Docs, gallery, public benchmark suite.
- CPU rendering is the supported baseline; **GPU tier remains opt-in beta** until
  the GoGPU native backends prove out across hardware.

### Beyond v1.0

- Harden the GPU tier as GoGPU matures.
- More coordinate systems: polar, geographic/maps.
- More stats: hexbin, contour, violin, KDE, regression smoothing.
- Animations / transitions (gg retained-scene + damage tracking make this cheap).
- Community plugin ecosystem.
- 3D (surface/scatter3d) — deliberately late, tightly scoped.

---

## 15. Versioning & stability

- **Pre-1.0 (`v0.x`):** breaking changes permitted in any release. The model, IR,
  and `Backend` interface need room to be gotten right before they freeze. GoGPU
  dependencies are pinned to exact versions per release.
- **1.0 onward:** standard Go semver. Public API, IR, `Backend` interface, and the
  JSON spec become stability surfaces. New geoms/backends arrive through the
  extension interface, not by changing existing signatures.
- **License:** permissive (e.g. MIT), and the core links only permissive deps
  (gg is MIT). This is a requirement, not a preference: refract must be embeddable
  by downstream frameworks under any license — including dual-licensed ones such
  as `lux` — so no copyleft or noncommercial-licensed code enters the dependency
  graph.

---

## 16. Repository layout

Dependency boundaries enforce the "lean by default" promise: the core depends on
nothing at all, and GoGPU enters only through the nested `backend/gg` module.

Packages marked *(v0.1)* exist; the rest are placed by milestone.

```
refract/                     # core module — pure Go, STDLIB ONLY (no requires)
  refract.go                 # top-level API: New, X, Y, Add, Render      (v0.1)
  data/                      # Source, Float64Columns, Table              (v0.1)
  scale/                     # linear, time (+ log, symlog, ordinal, colour) (v0.1)
  geom/                      # line, scatter, bar (+ area, step, boxplot) (v0.1)
  stat/                      # LTTB, min/max, density binning              (v0.4)
                             # (smooth and aggregate are still to come)
  coord/                     # cartesian (pluggable stage)
  layout/                    # panel-grid constraint solver               (v0.3)
  render/                    # model -> IR lowering                       (v0.1)
  facet/                     # faceting (wrap/grid)                       (v0.3)
  theme/  palette/           # tokens, colourblind-safe palettes          (v0.1)
  ir/                        # IR primitives + Backend interface          (v0.1)
  backend/svg/               # built-in, zero-dependency SVG emitter      (v0.1)
  backend/pdf/               # built-in, zero-dependency PDF emitter      (v0.3)
  internal/fontmetrics/      # stdlib hmtx/cmap reader + Helvetica table  (v0.1)
  internal/markers/          # the marker outlines both emitters share    (v0.3)
  spec/                      # JSON (Vega-Lite-compatible) marshal/unmarshal

  backend/gg/                # NESTED MODULE: raster now; GPU, browser    (v0.1)
                             # and native window later. Depends on
                             # gogpu/gg. Zero CGO. PDF did not land here —
                             # see ADR 0009.
    cmd/gallery/             # renders every documented figure            (v0.1)

  arrow/                     # NESTED MODULE: Apache Arrow adapter        (v0.4)
                             # Depends on apache/arrow-go. Zero CGO.
```

`backend/gg` and `arrow` depend on the core, never the reverse. A nested
module is excluded from its parent's module graph, so importing only `refract`
yields a graph with no external packages in it at all — CI asserts this on every
commit. SVG output works with no GoGPU present. See
[ADR 0001](docs/adr/0001-module-layout.md) for why the gg backend is a nested
module here rather than the separate `refract-gg` repository this section
originally proposed.

---

## 17. Open decisions

Six of the seven were settled while building v0.1; each has a record in
[docs/adr](docs/adr/). Two remain genuinely open, and are marked so.

1. ~~**gg coupling surface**~~ — **settled:** pin gg to an exact version and
   import only `gg` and `gg/text`; never `gg/gpu`, `gg/scene` or `gg/recording`.
   The whole adapter is about 300 lines.
   → [ADR 0006](docs/adr/0006-gg-coupling-surface.md)
2. ~~**SVG source of truth**~~ — **settled:** the built-in emitter is the only
   SVG path, and since v0.3 PDF is a built-in emitter too. The reopening this
   item anticipated happened and went the other way: `gg-pdf` cannot draw
   geometry, so no gg vector path exists to unify with.
   → [ADR 0004](docs/adr/0004-svg-source-of-truth.md),
   [ADR 0009](docs/adr/0009-pdf-backend.md)
3. **Spec fidelity to Vega-Lite** — strict round-trippable subset vs "inspired
   by". **Still open**; belongs to v0.5, and deciding it now would be guessing.
4. ~~**Text ownership**~~ — **settled:** the backend shapes, refract places, and
   `Measure` is the entire seam between them. The core carries a stdlib metrics
   reader and a fallback advance table, never a shaper.
   → [ADR 0003](docs/adr/0003-text-and-fonts.md)
5. ~~**Import path / module org**~~ — **settled:**
   `github.com/timzifer/refract`, with the gg backend at
   `github.com/timzifer/refract/backend/gg`.
   → [ADR 0001](docs/adr/0001-module-layout.md)
6. ~~**Minimum Go version**~~ — **settled:** Go 1.25, forced by gg's
   `iter.Seq`-returning `Face`. The ABI sensitivity this item worried about
   lives in `goffi`, reachable only through `gg/gpu`, which v0.1 excludes — so
   it becomes live at v0.6, not now.
   → [ADR 0005](docs/adr/0005-go-version.md)
7. **Geom/backend extension API** — the interfaces third parties implement.
   **Still open**; it freezes at v1.0, and freezing it well is what makes the
   "last plotting library" claim survivable.

---

## 18. References (ecosystem context)

- Rendering substrate: `gogpu/gg` (2D graphics, pure Go, zero CGO),
  `gogpu/wgpu` (pure-Go WebGPU: Vulkan/Metal/DX12/GLES/software/browser),
  `gogpu/gogpu` (windowing/input), `gg-svg`, `gg-pdf`, `gogpu/naga` (WGSL compiler),
  `go-webgpu/goffi` (pure-Go FFI). gg's GPU tiers port techniques from Vello and
  tiny-skia; its CPU AA ports Skia's analytic rasterizer.
- Existing Go plotting: `gonum/plot`, `go-echarts`, `go-chart`.
- Declarative reference: Vega-Lite, Plotly.
- Algorithms: LTTB (decimation), extended Wilkinson (ticks), datashader
  (density binning).
- Fallback text stacks (only if a gg gap appears): `go-text/typesetting`,
  `boxesandglue/textshape`.