# refract

**A grammar-driven plotting library for Go: one model, many backends, runs everywhere — built on the GoGPU stack.**

> Status: **v1.6.0 released; stable v1 API.** The API froze at v1.0.0 and
> follows semantic versioning. The v1.3, v1.4, v1.5 and v1.6 milestones below
> are all tagged. See [§14](#14-roadmap--milestones) and the [README](README.md) for
> what each one added. This document remains the working concept for everything
> past them.

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
- **Arrow adapter as a separate module** (`refract/arrow/v18`) so the core never links
  Apache Arrow. Shipped in v0.4: a null-free `float64` column is borrowed
  outright, everything else converts once and caches, and an Arrow null becomes
  a `NaN` so that one missing-data policy covers both
  ([ADR 0013](docs/adr/0013-arrow-adapter.md)).
- **Missing-data policy is explicit** — `NaN`/`Inf`: interpolate, gap, or error.
  (gg is already NaN-safe at the path level, so a gap never corrupts a render.)
  Since v0.2 the same policy covers a value the *scale* cannot place — zero on a
  log axis, a category outside a fixed set — because from the chart's point of
  view those are the same failure ([ADR 0008](docs/adr/0008-categorical-axes.md)).
- **Streaming** via a snapshot model ([§11](#11-concurrency--allocation)):
  produce on one goroutine, render a consistent snapshot on another. Shipped in
  v0.5 as `data.Stream`, which is deliberately *not* a `Source` — a table being
  appended to between two column reads is a table that disagrees with itself,
  so the only way to draw one is to freeze it
  ([ADR 0016](docs/adr/0016-streaming-and-damage.md)).

---

## 8. Model layer (GoG-lite)

The part refract actually builds. Building blocks:

- **Scales** — `Linear`, `Log`, `SymLog`, `Time`, `Ordinal`/categorical, plus
  color scales (sequential, diverging, qualitative). Scales own data→visual
  mapping and tick generation.
- **Coordinate systems** — Cartesian and polar, behind a genuinely pluggable
  stage: a scale keeps mapping a value into an interval, and the coord decides
  what the interval means and turns a pair of them into a device point.
  `Cartesian` is the identity, so the stage costs an existing geom nothing and
  the golden files prove it ([ADR 0018](docs/adr/0018-coordinate-systems.md)).
  Geographic projections are a wider seam — they transform every point with no
  linear interval underneath — and stay beyond v1.0.
- **Geoms** — `Line`, `Scatter`, `Bar`, `Area`, `Step`, `Boxplot`, `Rect`, and
  the distribution marks `Histogram`, `Violin`, `Ridgeline`, `Hexbin`,
  `Beeswarm`, `ECDF` and `Trend`. Geoms know *what* their shape is, never *how*
  it's rendered.
- **Groups and position adjustments** — a layer over a long table is N series,
  split by `geom.GroupBy` and painted from a discrete colour scale; `Stack`,
  `Dodge` and the streamgraph offsets are defined over those groups. The
  offsets are derived in `Train`, because the axis has to describe the totals
  ([ADR 0019](docs/adr/0019-position-adjustments.md)).
- **Stats / transforms** — `Bin` (histogram), `KDE` (density with Silverman's
  bandwidth rule), `Loess` (locally weighted regression), `ECDF`, `Hex`
  (hexagonal binning) and the decimation family. Numbers in, numbers out, each
  with an `Append` form and a determinism test. A distribution stat runs in
  `Train` and the axis is trained on its output, because the summary is what the
  axis has to describe ([ADR 0028](docs/adr/0028-distribution-stats.md)).
- **Facets** — `facet.Wrap` / `facet.Grid`, shared or free scales.
- **Aesthetic channels** — position, colour (`geom.ColorBy` through a
  `ColorScale`) and size (`geom.SizeBy` through a `scale.SizeScale`, mapped by
  area rather than by radius, which is the bubble chart).
- **Guides** — legends, colorbars, size keys, axes as positionable elements. The
  three keys are one ordered column with one stacking rule
  ([ADR 0027](docs/adr/0027-size-channel-and-the-guide-column.md)).
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
- **GPU acceleration (opt-in beta)**, shipped in v0.6 — but not from this
  module. `backend/gg` still must not import `gg/gpu`
  ([ADR 0006](docs/adr/0006-gg-coupling-surface.md)), so the tier is a nested
  module of its own, `backend/gg/gpu`, and importing it is the opt-in
  ([ADR 0022](docs/adr/0022-gpu-tier.md)). Transparent CPU fallback when no GPU
  is present.
- ~~**Browser (`GOOS=js`)** — gg/wgpu target browser WebGPU via `syscall/js`.~~
  gg has no `syscall/js` at the pinned version, so the browser is `backend/canvas`
  in the core instead ([ADR 0017](docs/adr/0017-browser-backend.md)).
- **Native window** via `gogpu/gogpu` for desktop interactive plots, shipped in
  v0.6 as the nested module `backend/window`. It draws with this module's CPU
  rasterizer and presents the result as a texture, so a window and a file are
  the same picture ([ADR 0021](docs/adr/0021-native-window.md)).

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
  renderer reads an immutable snapshot (copy-on-swap), and only the dirty
  regions are repainted. Shipped in v0.5: `data.Stream` is the snapshot and the
  swap, `ir.Damage` compares two recordings to find what moved, and `ir.Partial`
  is the one-method interface a backend implements to act on it. The damage is
  computed above the backends rather than inside one, because gg is one backend
  of four and the browser backend is not it
  ([ADR 0016](docs/adr/0016-streaming-and-damage.md)).
- **Allocation discipline** — refract keeps its IR construction allocation-light
  and leans on gg's zero-alloc fill/stroke/text paths. A benchmark gate asserts no
  per-frame allocations on the hot path. Shipped in v0.4: everything sized by the
  data comes from a pool, so a steady-state frame costs the same handful of
  allocations over a thousand rows and over a million.
  `TestARenderDoesNotAllocatePerPoint` is the gate as a test, and CI's
  `Benchmarks and the allocation gate` job is the gate as a benchmark —
  `.github/scripts/allocgate.awk` reads `go test -bench` output and enforces the
  same property from the numbers, over all three modules.

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
  via the gg GPU backend. **Shipped in v0.6**, as a nested module whose import
  is the opt-in: `_ "github.com/timzifer/refract/backend/gg/gpu"` registers gg's
  accelerator, and importing nothing leaves `backend/gg` exactly as it was
  ([ADR 0022](docs/adr/0022-gpu-tier.md)). Registration fails quietly on a
  machine with no device and gg falls back to the CPU, so a chart still renders.
  Precision at deep zoom turned out to be a separate matter and to belong
  elsewhere: device coordinates are pixels within a canvas and never large, and
  what runs out of digits is the float64 a timestamp becomes — so the rebasing
  is `scale.Origin`, which measures a time domain from an instant near the data.

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

Serialization and the web workflow — **shipped in v0.5**. The document is
Vega-Lite-*shaped* rather than a Vega-Lite subset: it borrows the vocabulary
where the concept exists in both and names refract's own things plainly, and
what it guarantees is the round trip through refract
([ADR 0014](docs/adr/0014-json-spec.md)):

```go
doc, _ := json.Marshal(p)           // a Plot is a json.Marshaler
// ship `doc` to a Go-Wasm frontend, or to a server, which renders it identically
q, _ := refract.ParseJSON(doc)
```

Interactivity — **shipped in v0.5** for the browser, through the built-in
canvas backend rather than through gg, which has no js target at the pinned
version ([ADR 0017](docs/adr/0017-browser-backend.md)). A native window followed
in v0.6, through `backend/window`, and is one call:

```go
show.Plot(p, window.Title("Signal"))   // backend/window/show
```

```go
p.On(refract.Hover, func(ev refract.Event) { /* ev.Point, ev.Series(), ev.Hit */ })
p.On(refract.Zoom,  func(ev refract.Event) { /* ev.Rect, ev.Factor */ })

live, _ := p.Live(canvas.Element(el))   // a surface, redrawn
defer live.Close()
live.Draw()
defer live.Bind(el)()                   // pointer, wheel and drag
```

The kinds share one `Event` struct rather than the per-kind types this section
first sketched: Go has no sum types, and a handler signature per kind means
`On` takes an `any` and the caller writes a type assertion
([ADR 0015](docs/adr/0015-hit-testing.md)).

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
- Allocation pass; a benchmark gate on per-frame allocations, running in CI over
  all three modules. Everything sized by the data comes from a pool, and both
  `TestARenderDoesNotAllocatePerPoint` and `.github/scripts/allocgate.awk`
  assert that a frame over a million rows allocates what a frame over a thousand
  does — 76 either way when the pass landed, and the same count either way
  since; the current numbers are in [docs/benchmarks.md](docs/benchmarks.md).
  Along the way, feeding a column into a scale row by row through a variadic
  interface method turned out to cost one allocation per row; it is now one
  call per column.
- Arrow adapter as a separate module, `refract/arrow` — since the v1 audit
  `refract/arrow/v18`, because its major version is Arrow's
  ([ADR 0030](docs/adr/0030-arrow-major-version.md)). A null-free `float64`
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

Not in v0.4, in case they look like oversights. Streaming was v0.5: there was no
`StreamSource` and no snapshot/swap, because the interesting half of that is
damage-aware repaint, which needs the interactive backends. `stat` carries the
decimation family only — smoothing, regression and hexbin are stats rather than
big-data machinery, and they belong with the geoms that would draw them. And
the GPU tier is untouched, which is the point of the CPU tier: big-data
*stills* are complete without it.

### v0.5 — Web & interactivity — **shipped**

- Browser rendering under `GOOS=js`. **Not** via the gg backend: gg has no
  `syscall/js` anywhere in it at the pinned version, so there is no WebGPU path
  to take and no canvas fallback to fall back to. refract draws on the canvas
  2D context itself, from `backend/canvas` in the core module, because
  `syscall/js` is the standard library and costs the core nothing
  ([ADR 0017](docs/adr/0017-browser-backend.md)). Paths go to `Path2D` as one
  string per drawing call: crossing the WebAssembly boundary is what costs in a
  browser.
- Event system: hover, click, zoom and pan, with hit-testing over the marks a
  render actually emitted. `interact.Index` watches a render — `render.Chart`
  gained an `Observer` that says which panel and which layer is drawing — so
  hit-testing is correct for every geom including ones that do not exist yet,
  and the IR gained no identity channel
  ([ADR 0015](docs/adr/0015-hit-testing.md)). `Plot.On` registers handlers,
  `Plot.Live` draws into a surface, and `Live.Bind` wires a DOM element to it.
- Full-spec JSON serialization, in `spec/`. Vega-Lite-shaped rather than a
  Vega-Lite subset, and §17.3 is settled with the reasoning
  ([ADR 0014](docs/adr/0014-json-spec.md)). A `Plot` is a `json.Marshaler`; the
  round trip through refract is guaranteed and tested per mark and per scale.
  It rests on three new description APIs — `geom.Desc`, `scale.Desc` and
  `facet.Desc` — so that the format lives outside the model packages.
- Streaming with snapshot/double-buffer, and damage-aware updates computed in
  the IR rather than in a backend. `data.Stream` is appended to from any
  goroutine and frozen between frames; `ir.Damage` diffs two recordings;
  `ir.Partial` is what a repaintable backend implements
  ([ADR 0016](docs/adr/0016-streaming-and-damage.md)).
- *DoD:* a chart can be written down as JSON and read back as the same chart, a
  pointer over it reports the row underneath, the wheel zooms about that point
  and a drag pans, a producer can append to it on another goroutine while it is
  drawn, a redraw repaints only what changed and skips a frame that changed
  nothing — and all of that runs in a browser from the same model that renders
  SVG on a server. ✔

- Row identity, opt-in. A hit always reports the data values under the pointer;
  with `Live.TrackRows(true)` it also reports the source row behind the mark,
  which is what highlighting the matching row of a table beside the chart
  needs. Geoms report where each of their rows landed through `geom.Rows`,
  separately from what they drew — a smoothed line is a curve through its rows
  and a bar is four corners around one, so the drawn points are not the rows.
  Decimation is not in the way: LTTB and MinMax keep real rows. Faceting is not
  either: rows are resolved back through `data.Subset` to the table that was
  handed in.

Not in v0.5, in case they look like oversights. There is no native window and
no GPU tier: both landed in v0.6, and both needed `gogpu/gogpu` and `gg/gpu`
rather than anything in this milestone. Row identity is off by default and not
every mark has one — a boxplot's box aggregates many rows, a density raster is
not a mark,
an interpolated point was never measured — and those report no row rather than
a nearby one. And the damage unit is a drawing call, so moving one point of a
line repaints the line's box; what it does not repaint is the title, the axes
and the margins, which is most of the canvas.

### v0.6 — Native interactive & polish — **shipped**

- Native interactive window via `gogpu/gogpu`, in a new nested module,
  `backend/window`. The event system it needed was already here, and so was the
  rasterizer: the window draws with the same CPU backend that writes a PNG, into
  a new in-memory `gg.Surface`, and presents that as one texture per changed
  frame. There is one implementation of every mark, so a window shows exactly
  what a file would ([ADR 0021](docs/adr/0021-native-window.md)). The steering
  moved into the core as `refract.Input` — a portable state machine that turns
  presses, moves and wheel notches into hovers, clicks, pans and zooms — and
  `Live.Bind` was rewritten onto it, so the browser and the window share one
  implementation rather than two that drift.
- GPU tier enabled, opt-in, as a nested module of its own:
  importing `backend/gg/gpu` registers gg's accelerator, and importing nothing
  keeps `backend/gg`'s dependency graph exactly as small as it was
  ([ADR 0022](docs/adr/0022-gpu-tier.md)). Origin rebasing turned out not to
  belong here: refract's device coordinates are pixels within a canvas and are
  never large, and the precision a deep zoom loses is in the float64 a timestamp
  becomes. So it landed in `scale.Origin`, which measures a time domain from an
  instant near the data — and keeps a nanosecond a nanosecond, where absolute
  Unix nanoseconds in this century need 61 bits of a 53-bit mantissa.
- Math typesetting for labels, optional and pluggable. `mathtext.Typesetter` is
  the seam and `mathtext.TeX` is the subset that ships — scripts, `\frac`,
  `\sqrt`, `\mathrm`, the spacing commands and the symbols a chart label
  reaches for. A typesetter is installed by wrapping the backend, so notation
  works in every label a chart has, including the ones layout measures and the
  ones a geom that does not exist yet will draw
  ([ADR 0023](docs/adr/0023-math-typesetting.md)).
- Responsiveness: `refract.Responsive` scales a theme by how much smaller or
  larger the drawing is than the design it was made at, `Live.Resize` is how a
  surface says its size changed, and `ir.Resizer` is what tells a backend
  ([ADR 0025](docs/adr/0025-responsive-charts.md)). Device scale was already
  handled and is a different thing.
- Accessibility, in three channels
  ([ADR 0024](docs/adr/0024-accessibility.md)): a chart's title becomes an SVG
  `<title>` with `role="img"`, a PDF document title and a canvas `aria-label`,
  through a new optional backend interface, `ir.Semantics`; `Plot.Describe`
  reads the data and writes the `<desc>` a screen reader announces after it;
  `Plot.DataTable` writes the rows as an HTML table, which is the fallback a
  picture cannot be; and `theme.Redundant` gives every layer a dash pattern and
  a marker shape of its own, so that colour is not the only channel.
- *DoD:* a chart opens in a native window on the desktop and pans, zooms and
  resizes there; the same chart renders with the GPU tier switched on by one
  import and without it; a label can be written as notation and is measured as
  it is drawn; a chart drawn at a third of its size is the chart rather than a
  photograph of it; and a reader who cannot see it gets a name, a description
  and the numbers. ✔

Not in v0.6, in case they look like oversights. The GPU tier is **opt-in beta**
and stays that way past v1.0: for server-side stills the CPU rasterizer and the
vector emitters are the supported path. The window cannot be opened by CI, so
what is tested is everything that is not the window and the window itself is
only compiled — the same hole `backend/canvas` has about a browser. The built-in
typesetter is a small subset on purpose: no matrices, no growing delimiters, no
document-class macros, and a label needing those wants an engine plugged into
the interface. Accessibility stops at the document: there is no per-mark
`<title>`, no tab order through the data, and no reduced-motion or contrast
handling — a chart with ten thousand points has no useful reading as ten
thousand elements, and the table is the better answer to the same question.

### v0.7 — Marks, groups and adjustments — **shipped**

The plumbing half the catalogue was waiting on. Nothing here is a coordinate
system and nothing here is a stat; it is the four pieces that most missing
chart types share. See [docs/chart-types.md](docs/chart-types.md) for the full
list and what each form costs.

- A data-driven rectangle mark. `Bar` grows from a baseline and `Region` takes
  four literals, so no mark occupied an arbitrary box per row. `geom.Rect`, a
  `geom.X2` beside the existing `geom.Y2`, and a per-row baseline through `Y2`
  turn heatmap, gantt, candlestick, waterfall, bullet, waffle and calendar into
  recipes rather than into seven geoms. An edge the row does not name is the
  slot the axis implies — a band's own width, or the closest spacing in the
  data — so a heatmap over two categorical axes is two columns and a colour.
- Groups within one layer, and a discrete colour scale to paint them.
  `geom.GroupBy` splits a long table into series; `scale.Qualitative` is the
  categorical colour scale, shaped like `scale.Categorical` and riding the
  `ColorScale` interface the same way, so one `geom.ColorBy` binds either kind
  and the guide follows from the scale rather than from a second option
  ([ADR 0020](docs/adr/0020-discrete-colour-and-multi-entry-legends.md)).
- A layer may contribute many legend entries, through `geom.Legender` — an
  optional interface rather than a wider `Geom`. §17.7 freezes that interface
  at v1.0, and a pie with N slices in one layer must not be the reason it
  freezes badly.
- Position adjustments: stack, dodge, fill, and the silhouette and wiggle
  offsets a streamgraph is made of. Derived in `Train`, because the axis has to
  describe the totals; drawn in `Build`, because that is where geometry lives
  ([ADR 0019](docs/adr/0019-position-adjustments.md)). The Byron–Wattenberg
  offset itself is `stat.StackOffsets` — numbers in, numbers out.
- *DoD:* one layer over a long table draws N coloured series and the legend
  names all N; a stacked bar's axis reaches the stacked total and each segment
  is separately hittable and separately attributable to its row; a heatmap and
  a gantt chart render from the public API; every new mark and option survives
  the JSON round trip, including the `("rect", "")` collision between a
  data-driven rect and the region annotation; the allocation gates are
  unchanged. ✔

Not in v0.7, in case they look like oversights. A grouped **bar and area stack
by default** and everything else does not: two lines drawn over one another are
two readings, and adding them would invent a third nobody measured. Stacking
accumulates in group order, so a stack of mixed signs runs each segment from
where the last one ended rather than splitting into a positive and a negative
half — the pictures differ, and the sequential one is the one a running total
is. `geom.WidthBy` gives a bar its width from a column, in the axis's own
units; it does not *reposition* the slots, so a marimekko is one layer whose X
column already holds each column's centre. That is a deliberate line: unequal
slots that label themselves are an axis question rather than an adjustment
one. And a group index is per layer, so a facet whose panels hold different
groups wants an explicit `scale.Qualitative` — a shared scale is what makes one
colour mean one thing across panels, and the palette index cannot be.

### v0.8 — Coordinates — **shipped**

- `coord/`, at last: the stage [§8](#8-model-layer-gog-lite) has promised since
  v0.1. A scale still maps into an interval; a coord decides what the interval
  means. `Cartesian` is the identity and the default, so every existing geom
  draws what it always drew and the golden files are the proof
  ([ADR 0018](docs/adr/0018-coordinate-systems.md)).
- `Polar`, and with it pie, donut, radar/spider, rose/coxcomb, wind rose and
  gauge. A pie is a stacked bar with θ from the Y axis; a radar is a line over
  an ordinal angular axis. Neither is a new geom, which is the point of having
  built v0.7 first.
- Arcs are cubics. The IR gains nothing: ADR 0002 froze the primitive set on the
  claim that every curve a chart needs is expressible as cubics, and a
  coordinate system is the first serious test of it.
- Polar furniture — concentric rings instead of horizontal grid lines, tick
  labels around the ring instead of along an edge. The coord reports the
  geometry; `render` still strokes it, because `render` is the only package that
  knows drawing order.
- *DoD:* the same `geom.Bar` layer draws a bar chart in `coord.Cartesian` and a
  pie in `coord.Polar`; **every existing golden file is unchanged**; a pointer
  over a slice names its category and its row rather than a pixel; a donut's
  hole is an explicit annulus; the spec round-trips the coord; the allocation
  gates are unchanged and the per-point call has a batch form so that they stay
  that way. ✔

Two things the interface grew beyond what
[ADR 0018](docs/adr/0018-coordinate-systems.md) sketched, both for reasons the
sketch could not have known. `Frame` **returns** the coord positioned in a panel
rather than moving the receiver into it, because panels are built concurrently
and a coord that remembered which panel it was in would be a data race — the
same problem `scale.Snapshotter` exists for, answered without a second
interface. And `Furniture` **fills** a struct the caller owns rather than
returning one: a Cartesian panel's furniture is two dozen little slices, and
allocating them per frame would have cost more than the whole rest of the frame
does. The record carries both as an amendment.

Not in v0.8, in case they look like oversights. There is no **geographic
projection**: a projection transforms every point with no linear interval
underneath it, which is a wider seam than this one, and ADR 0018 says it is to
be argued on its own evidence rather than smuggled in as a third `Coord`. A
polar coord **does not decimate** — a bucket of equal angle is not a bucket of
equal width, so a reduction defined over pixel columns would be measuring the
wrong thing, and nothing polar is a big-data chart. `layout` is **untouched**: a
polar coord inscribes itself in whatever rectangle the solver gives it, and
gutter rules that understand a radial axis are a later milestone. A radar's
contour closes because `geom.Closed` says so rather than because the coord
guessed: whether a series wraps is a fact about the series, and a polar time
series spiralling through three revolutions does not. And a **hit on a filled
mark is decided against the outline the drawing call carried**, so a shape whose
edge is a curve can be hit a little way outside its ink at a bulge — the convex
hull of a cubic, which is the same slack a vertex already gets and the opposite
error from missing a slice the pointer is plainly inside.

#### The v0.8 sugar

Three additions, none of them a new mark, all of them things the coordinate
stage made expressible and nothing spelled:

- A slice's **inner and outer radius are columns**. `geom.Bar` reads `X` and
  `X2` as its two edges on the cross axis, exactly as `geom.Rect` has since
  v0.7 — and under a polar coord that axis is the radius. So a donut carries
  three numbers per slice instead of one: how far round it goes, where it
  starts, and where it stops.
- A slice can be **broken out of the ring**. `geom.Explode(f)` moves every mark
  of a layer away from the middle by a fraction of the outer radius and
  `geom.ExplodeBy(col)` reads that per row, which is what pulls one slice out
  and leaves the others in place. It is a displacement along the mark's own
  bisector rather than a longer radius, so the slice still says what it said
  and the gap shows where it came from. The coord answers how far and the geom
  moves the path, because a coord does not draw; `coord.Cartesian` deliberately
  cannot answer, so a Cartesian chart is unchanged by an option every geom now
  accepts ([ADR 0026](docs/adr/0026-breaking-a-mark-out.md)).
- `coord.Pie()` and `coord.Donut(f)` **name the recipe**. They are sugar for
  `Polar(Theta(FromY))` and that plus `Hole(f)`, describe themselves as the
  polar coord they are, and are where the two facts a pie needs beyond the
  coord are written down: that the angular scale must not be niced, and that
  the hole is where the radial scale starts.

### v0.9 — Distributions, density and size — **shipped**

- The stats [§8](#8-model-layer-gog-lite) has promised since v0.1 and `stat/`
  had never carried: a 1-D `Bin`, `KDE` with a bandwidth rule, `Hex`, `ECDF` and
  `Loess`. Each a pure function with an `Append` form, each with a determinism
  test — a reduction that reached for `math/rand` would make a parallel render
  stop being byte-identical to a serial one.
- The charts they carry: `geom.Histogram`, `geom.Violin`, `geom.Hexbin`,
  `geom.Ridgeline`, `geom.Beeswarm`, `geom.ECDF` and `geom.Trend`.
- A size channel, and the bubble chart. `geom.SizeBy` reads a column through
  `scale.Size`, mapped by **area**, not radius, and guided by a third guide kind
  — which was the moment the guide column in `layout` was generalised once
  rather than extended twice ([ADR 0027](docs/adr/0027-size-channel-and-the-guide-column.md)).
- *DoD:* every stat is a pure function of its input under test; violin and
  ridgeline compose with v0.7's groups; a bubble chart's size key sits beside a
  legend and a colourbar without overlap, and doubling a value multiplies the
  diameter by the square root of two, under test. ✔

The line that had to be drawn twice is where a stat runs
([ADR 0028](docs/adr/0028-distribution-stats.md)). ADR 0011 puts decimation in
`Build`, in device space, on the rule that what a chart's axis reports must not
depend on how wide the chart is. A distribution stat is the same rule pointing
the other way: a histogram's Y axis holds counts that appear nowhere in the
table, an ECDF's holds a fraction it computed, and a trend's fit reaches values
no row has — so the summary *is* what the axis has to describe, and it is
computed in `Train`. Two marks are the exception because what they compute is a
length on screen rather than a value: a hexbin bins over the plot rectangle,
because a hexagon laid out in data space comes out stretched by the panel's
aspect ratio, and a beeswarm places its marks against a marker's width.

Not in v0.9, in case they look like oversights. A **hexbin has no colourbar**:
its counts are not known until the plot rectangle is, and the guide column is
measured before that — the same ordering ADR 0011 describes, reached from the
other side. A **histogram ignores `GroupBy`**, because two overlapping
histograms hide each other exactly where the comparison is; `Violin`,
`Ridgeline` and `ECDF` are the three marks that answer that question without
overplotting, and each of them takes the series column. A **sized layer draws
circles rather than markers**, because `ir.Backend.Markers` carries one style per
call and a per-row size would be a call per row — the same refusal
[ADR 0007](docs/adr/0007-per-mark-colour.md) makes about colour, and the same
answer, with better hit-testing as a bonus. And **`stat` still does no sorting**:
`Quantile`, `ECDF` and `Loess` take ordered columns, because sorting means a
buffer and the geom already keeps one.

### v1.0 — Stable & complete enough — **shipped**

- API freeze; semver. The audit that precedes it is
  [docs/v1-api-audit.md](docs/v1-api-audit.md): every exported identifier with
  a verdict, and the changes it asked for are in.
- Stable registration/extension model for third-party geoms and backends —
  **done** ([ADR 0029](docs/adr/0029-extension-model.md)): `geom.Configure`,
  `geom.Extra`, and `Register` in `geom`, `scale` and `coord`, with the JSON
  spec carrying a registered mark's own properties.
- Docs, gallery, public benchmark suite — **done**. The docs are the
  [README](README.md), the API reference on pkg.go.dev, the ADRs and
  [docs/chart-types.md](docs/chart-types.md). The gallery is
  [docs/images](docs/images), rendered by `backend/gg/cmd/gallery` from every
  milestone's marks and checked against the code on every commit. The
  benchmark suite is [docs/benchmarks.md](docs/benchmarks.md): every benchmark
  in the repository, what it measures, which numbers CI gates, and a results
  table CI publishes on every run.
- CPU rendering is the supported baseline; **GPU tier remains opt-in beta** until
  the GoGPU native backends prove out across hardware.
- Tagged. The core was `v1.0.0` and is `v1.6.0`; `backend/gg` and
  `backend/window` share it, the opt-in GPU tier is `backend/gg/gpu/v0.3.0`
  — it stays at `v0` for as long as it is opt-in beta, whatever the core does
  — and the Arrow adapter is `arrow/v18.0.4`, whose major is Arrow's. The
  milestones before `v1.0.0` were tagged at the same time as it, so every one
  of them names a commit. The order — the core first, then the nested modules'
  `require` lines, then their own tags — is in
  [CONTRIBUTING](CONTRIBUTING.md#releasing).

  The post-freeze milestones below shipped in `v1.1.0` — tracks and the text
  mark — in `v1.2.0`, the Smith chart, in `v1.3.0`, the six gaps that were not
  chart types, in `v1.4.0`, the relational and hierarchical layouts, in
  `v1.5.0`, label placement and QQ plots, and in `v1.6.0`, colour ramps that
  compress or class their domain. `v1.3.0` and `v1.4.0` tag the core
  alone: no nested module had a release of its own between `v1.2.0` and
  `v1.5.0`, and a nested tag whose `require` line named an older core than the
  one it was tested against is the failure the release order exists to prevent.
  All of them are additive: no interface gained a method, no struct lost a
  field, and every option they add is one an existing mark accepts and
  ignores.

### v0.10 — Tracks: a band at a panel's edge — **shipped**

The first milestone after the freeze, and additive throughout: `Plot.Track`
attaches a band to the bottom or top of the plot area that shares the plot's X
scale *object* and carries a vertical scale of its own, of a different kind if
that is what the data is — an ordinal strip of machine states under a linear
speed trace, on one time axis. Sharing the object rather than the domain is
what makes a zoom one zoom, and it is why hit-testing, the parallel path and
the Fyne widget needed no changes at all.

Its cost is one narrow widening of the layout solver — `layout.Grid.RowHeights`
gives a row its height instead of deriving one — which is the revisit
[ADR 0010](docs/adr/0010-panel-layout.md) asked for by name, answered without
becoming the general size-per-panel solver it warned about. The rows not given
a height are still all the same size. See
[ADR 0031](docs/adr/0031-tracks.md).

All four edges are there. Bottom and top are rows sharing X; left and right are
columns sharing Y, which is what a colour key or a marginal distribution wants.
They are one code path — `layout.extents` sizes fixed rows and fixed columns
with the same function — and bands on two edges leave the corner between them
empty, which the solver already understood because a wrapped facet leaves holes
too.

The same solver change gives stacked plots on one domain — the linked-axes
shape — as `Grid` options rather than a `Link` API, because a `Grid` already
routes its plots through the one solver and already takes their scale
objects.

### Text: a label per row — **shipped**

`geom.Note` placed one literal string at one literal position, so labelling
rows cost a layer per row — and since a plot only ever gains layers, a chart
whose rows change had to be rebuilt, which takes the reader's zoom with it.
`geom.Text` reads its labels from a column, like every other mark reads its
numbers, and needs no rebuild when the rows change.

The label goes where the encoding says. Naming neither `X2` nor `Y2` puts it at
the row's point; naming either puts it in the middle of the box the row spans —
and that box is the one `Rect` would draw for the same options, so one option
list describes the rectangles and labels them. That is what makes it a mark
rather than a recipe: the anchor is the middle of the box's *visible* part, so a
bar half scrolled off the edge keeps its label; the run is measured through
`ir.Backend.Measure` with the font it will be drawn in and dropped, or elided,
when the box is too narrow, because a label that overruns reads as belonging to
the neighbour; and a layer given `ColorBy` takes each label's ink from the fill
that scale gives the row, so a qualitative palette does not leave half its
categories unreadable.

It pairs with the tracks above: a track gives the state strip its lane, and this
gives its bars something to say — which is what a chart locked against panning
needs, because locked means no hover and no hover means no tooltip.

Neighbouring labels are not moved apart. A box too narrow drops its label
already, and a general de-overlap pass is a layout question rather than a
mark's. See [ADR 0032](docs/adr/0032-text-as-a-mark.md).

### Smith charts: a third coordinate system — **shipped**

`coord.Smith` reads a panel's two axes as a complex impedance — r = R/Z₀ and
x = X/Z₀ — and maps the pair through the reflection coefficient
Γ = (z − 1)/(z + 1), which carries the whole right half-plane, every passive
impedance including the infinite ones, into the unit disc. It is the standard
instrument of RF, microwave and antenna work, and no general-purpose plotting
library draws one, because a library whose coordinate stage is hard-coded
Cartesian cannot.

It is the sharpest evidence that §8's pluggable stage was cut in the right
place: the chart's two grid families are the images of the two axes' own grid
lines, so the constant-resistance circles are what the X ticks look like once
the coord has had them and the constant-reactance arcs are the Y ticks — and
`render` was not touched at all. No new mark either: the locus is a `geom.Line`
from v0.1.

Two additions came with it. `scale.TickValues` pins a linear axis's tick
sequence, because the 0.2 / 0.5 / 1 / 2 / 5 of a paper chart is a convention
rather than the answer to a tick search. And `coord.SmithAdmittance` is the Y
chart — one sign, since y = 1/z gives Γ_y = −Γ_z — applied to the picture and
not to the data, so one load lands in one place whichever chart it is read on.

Not drawn: constant-|Γ| circles, constant-Q arcs and a combined ZY overlay.
Each is a third grid family, and a coord may draw one grid line per tick a
scale emits — the same constraint that made the columns an impedance rather
than the reflection coefficient an instrument reports.
See [ADR 0033](docs/adr/0033-smith-charts.md).

### v1.3 — What is not a chart type — **shipped**

Six gaps that [docs/chart-types.md](docs/chart-types.md) could not hold,
because a catalogue sorted by machinery has no line for a mark that needs
neither a coordinate system nor a stat, and no line at all for the three that
are not marks. None of them is a shape; all of them were the difference
between a chart being *drawable* and being *usable*. The catalogue grew a
bucket H for them.

- **A null is a missing value, in every column kind.** A missing number is NaN
  and every policy refract has is written against that; a missing *category*
  read back as `""` became a band of its own on an ordinal axis and a missing
  *instant* as the zero time stretched a three-hour domain across two
  millennia. `data.Nulls` is the optional interface `data.Source`'s own
  documentation promised, `geom.column` is the one place it is read, and
  everything downstream is the machinery that already handled a NaN
  ([ADR 0034](docs/adr/0034-null-values.md)).
- **A tick label is described rather than computed.** `scale.Desc` carried an
  honest field saying a document had lost the axis's formatter and gave it
  nowhere to put one, so a chart authored as JSON could not set a thousands
  separator, a currency or a decimal place at all. `scale.NumberFormat` and
  `scale.TimeLayout` are the declarative spelling, beside the Go function
  rather than instead of it ([ADR 0035](docs/adr/0035-label-format-and-locale.md)).
- **A chart in a language.** The time ladder rendered through Go's own English
  tables and `strconv` writes a decimal point — which for a German reader is
  not foreign but wrong, because "1.234" reads as one and a bit.
  `refract.Locale` walks the scales the chart description holds, which is what
  reaches a track's own scale and a free facet axis's clone.
- **An interval around a measurement.** Every chart of a mean, a forecast or a
  tolerance carries two numbers per row and refract drew fifteen marks with
  nowhere to put the second. `geom.ErrorBar` takes either spelling a table
  comes in, and which axis it runs along follows from the encoding
  ([ADR 0036](docs/adr/0036-error-bars.md)).
- **A second axis, in both directions.** A `Plot` had one scale per direction
  and a layer no way to name another. `Plot.Y2`/`geom.OnY2` and
  `Plot.X2`/`geom.OnX2` are a scale on the chart and a binding on the layer,
  and each axis reaches the layout, the coord's furniture, hit-testing,
  steering, the description and the document
  ([ADR 0037](docs/adr/0037-secondary-axis.md)). The vertical one is two
  quantities in different units — revenue against margin; the horizontal one is
  most often one reading with two rulers, an oven curve counted in cycles and
  in minutes, which needs no second layer because an axis with nothing drawn on
  it is still an axis. The two are independent: a layer may name both.
- **A PDF in a script WinAnsi cannot hold.** The emitter named the base-14
  Helvetica, so every rune outside Latin-1 became `?` — in the format people
  send to customers. `pdf.WithFont` embeds a face, subset to the glyphs the
  document drew, with a `ToUnicode` map so the text is still selectable
  ([ADR 0038](docs/adr/0038-embedded-fonts.md)). It needed an sfnt parser, and
  it is in `internal/sfnt` because the core module has no dependencies and
  keeps none.
- *DoD:* a null in a text or temporal column is gapped rather than drawn; a
  chart written down as JSON labels its ticks the way it was told to, in the
  language it was told to; a mean and its interval are one mark; revenue and
  margin are one chart with two axes that zoom together and describe
  themselves separately, and so are minutes and cycles along the bottom and the
  top; and a Japanese label reaches a PDF as a glyph rather
  than as a question mark. ✔

Every one of them is additive: no interface gained a method, no struct lost a
field, and a chart that mentions none of them draws exactly what it drew —
which is what every golden file in the repository asserts.

### v1.4 — Bucket E, the last one — **shipped**

The relational and hierarchical layouts: treemap, icicle, sunburst, sankey, arc
diagram, chord diagram. It is the bucket this document called the only family
that shares no machinery with the rest — "its own data shape, its own solver,
its own legend, its own hit-testing" — and half of that sentence turned out to
be wrong, which is the interesting part.

The data shape and the solvers were real, and they are where the work went:
five new channels (`geom.From`, `geom.To`, `geom.ID`, `geom.Parent`,
`geom.Value`) and six pure functions in `stat/` — a depth, a roll-up, a
partition, a squarified packing, a flow layout and a chord layout, each with a
determinism test. The legend and the hit test needed nothing at all. A
multi-entry legend has been what a layer whose series live inside it
contributes since v0.7, and a relational layer's nodes are exactly that; and
`interact` has indexed one mark per subpath of a fill since v0.5, so a layout
that draws one subpath per cell is pointable with no new machinery. The only
line outside `geom`, `stat` and `spec` is `Plot.showLegend` learning that an
edge table is a second kind of series.

**Four marks, six charts.** Every layout fills the unit square — a span across,
a height out — and the coordinate stage decides what that looks like, which is
the v0.8 move made twice. An `Icicle` under `coord.Polar` is a sunburst; an
`Arc` under one, with its rail moved to the rim by `geom.Baseline(1)`, is a
chord diagram. Neither is a mark of its own, for the same reason a pie is not a
second implementation of a bar.

One thing had to move across a line, and it is worth the sentence: a squarified
treemap packs against the *panel* rather than against the unit square, because
what it optimises is an aspect ratio on screen and squarifying a square to
stretch it into a wide panel defeats the algorithm. That makes it the third
instance of [ADR 0028](docs/adr/0028-distribution-stats.md)'s stated exception,
after the hexagonal lattice and the beeswarm's offsets.

- *DoD:* a hierarchy of `(id, parent, value)` draws as a treemap and as a
  sunburst from one layer and two coords; an edge list of `(from, to, value)`
  draws as a sankey and as a chord diagram; every layout is a pure function
  with a determinism test and a fixed sweep count; a frame costs the same over
  a hundred thousand rows as over a thousand; and every existing golden file is
  byte-for-byte what it was. ✔

Still not drawn: node-link and Venn/UpSet. See
[ADR 0039](docs/adr/0039-relational-layouts.md).

### v1.5 — Two diagnostics — **shipped**

Bucket H's de-overlap pass and the QQ plot the ECDF made obvious, both of which
this document had listed as beyond v1.0 and neither of which needed a new
stage. `geom.AvoidOverlap(true)` opts a text layer into panel-local placement:
`render` lends participating layers a placer, measures through the active
backend and solves in `internal/layout`, trying the original anchor and eight
nearby candidates against the panel rectangle and the labels already placed.
It is bounded and deterministic rather than a relaxation, so a parallel or
watched render draws the same chart as a serial one, and a label in a box is
dropped rather than moved into its neighbour's row.
See [ADR 0040](docs/adr/0040-label-collision-avoidance.md).

`geom.QQ` ranks a sample against the standard normal at plotting positions
(i+0.5)/n, with observations in their own units and no fit or reference line
implied. It shares the ECDF's sorting and grouping buffers — both summarise one
sorted series per group — and `stat.QQ` takes any quantile function, so another
distribution is a `Scatter` over its pairs.
See [ADR 0041](docs/adr/0041-qq-plots.md).

Both are additive, both round-trip through the spec, and the allocation gate
covers a full frame of each over a thousand rows and a hundred thousand. ✔

### v1.6 — Colour ramps that compress and class — **shipped**

The colour channel had one shape: a ramp interpolated linearly from one end of
the domain to the other. The positional channel has had `Log` and `SymLog`
since v0.3, and the quantity the colour channel is most often bound to — a bin
count, a hexbin density — is exactly the one a linear reading destroys.

`scale.ColorLog` and `scale.ColorSymLog` give each decade an equal share of the
ramp. The transform is a field beside the kind rather than a value of it,
because a diverging ramp over a log-fold change is both; on a diverging scale
it runs on the signed deviation from the centre, so a logarithm there is a
symmetric one. A log domain is strictly positive as `Log`'s is: an empty bin
gets the undefined colour rather than the colour of the rarest observation.

`scale.Threshold`, `scale.Quantize` and `scale.Quantile` cut the domain into
classes, so a colour names an interval instead of a shade to estimate.
`Quantile` is the one colour scale that keeps its sample: a digest or a
reservoir would make the boundaries depend on row order, which
[ADR 0012](docs/adr/0012-parallel-panels.md) forbids.

The colourbar stopped assuming a linear reading. It asks the scale which values
are worth labelling, where a value sits on the bar, and which value the ramp
reaches at a point of it — so a log bar is ticked at decades and its gradient is
sampled evenly along itself rather than across the domain, and a classed bar is
drawn in bands and labelled at the boundaries.

All of it is additive: `ColorScale` gained no method, and the three capabilities
are optional interfaces beside it in the shape ADR 0020 established.
See [ADR 0042](docs/adr/0042-colour-transforms-and-classes.md). ✔

### v1.7 — Identity, and what it unlocks — **shipped**

The one thing this document had said was blocked, and the two things that
turned out to be the same problem.

A row number is not an identity. `geom.Rows` reports the row behind a mark, and
that row is an index into a table as it stands for one frame — append to it,
filter it, or window a stream and the numbers renumber. So `geom.KeyBy` names a
column instead, and the key is read from the layer's own source at the row that
was already being reported. Nothing in `geom` reads it, `Geom` gained no
method, and the IR gained no identity channel: `geom.Faceter` already exposed
both halves of what this needed, so twenty geoms became identifiable without
one of them changing. See [ADR 0043](docs/adr/0043-mark-identity.md).

With a key, tweening is a join. `data.Tween` blends two states of a table in
**data space, before the scales** — one `Source` whose contents change rather
than a source per frame, the way `data.Stream` already worked — so every geom,
coord and backend animates without knowing it can. The row set is the union and
is fixed for the whole transition, which is what keeps two frames comparable to
`ir.Damage` and an animation off the full-repaint path. `refract.Transition`
drives it, and refract owns no clock: `At(f)` is the whole primitive, which is
also what makes a golden file of a half-finished movement a real frame rather
than an approximation of one. See
[ADR 0044](docs/adr/0044-transitions.md).

The same key is what makes one chart able to say something another understands.
`interact.Index.Locate` is the inverse of a hit test, `Select` and `Event.Rows`
are the brush the audit had committed to, and `Live.Rebuild` now keeps the
reader's zoom — because the caller rebuilding is usually answering something
the reader did, and discarding their view as a side effect is a chart that
fights back. What is deliberately *not* here is a linking engine: a link is a
statement about two charts and this model is about one, so the host is the
link and `examples/linked` is what that looks like. See
[ADR 0045](docs/adr/0045-linked-views.md).

Not in v1.7, in case they look like oversights. A **string does not
interpolate** — half of "ingest" is not a node — so anything a geom decides
from one decides it abruptly: a categorical slot, a discrete colour class, a
group membership. That answers "can text animate" twice: a label over a
*number* counts, because a text layer re-spells its column every frame, and
`data.Round` is what stops it counting in raw floats; a label over a *string*
snaps, with no cross-fade and no character morph, though its position still
moves. **Enter and exit do not fade**, because opacity is a property
of a layer rather than of a row and a per-row opacity channel is the IR change
[ADR 0007](docs/adr/0007-per-mark-colour.md) refuses; `EnterFrom` puts the
answer in data space instead, so a bar grows out of its baseline. A
**`data.Stream` has no identities to join on** and animates by being redrawn,
which already worked. There is **no keyframe timeline**: a sequence is a list of
two-state transitions and building one is a host-side loop, so the pair shipped
first. There was still **no overlay layer** at this point — bucket H's
remainder — so `Input.Dragged` handed the host the rectangle and the host drew
it; v1.8 closed that. And there is **no `Grid.Live`**: a grid composes plots
into a document, and several interactive charts are several `Live`s on several
surfaces. ✔

### v1.8 — The overlay, and the last of bucket H — **shipped**

The chart can now draw over itself. A crosshair, a ring round a mark, the
rectangle a reader is dragging out, a tooltip sized to its own text: all of
them are things whose positions come from a pointer rather than from a column,
which is why none of them could be a layer and why `interact` — which only
reads — could not draw them either. So the overlay is a stage in `render`,
after the guides, given the backend and where the panels are.
See [ADR 0046](docs/adr/0046-overlay-layer.md).

It is announced to no observer, and that is the property that makes it usable
rather than merely present: what an overlay draws is not hit-testable, because
a tooltip a pointer can hit is a tooltip that flickers. Getting there needed a
bug fixed that had been latent since v0.5 — `render.Observer` had no way to
*close* a layer, so everything drawn after the last one was attributed to it,
and a legend's swatches were being indexed as marks. `render.EndData` is the
seam that closes it, in the shape `LayerAxes` already established.

`Crosshair`, `Highlight`, `Brush`, `Tooltip` and `Overlays` ship in the root
package, each a struct whose zero value draws nothing and whose colours come
from the theme when it is not told. `Tooltip` is the only one that measures,
and it measures through the backend that is about to draw it — which is what
`ir.Backend.Measure` has always been for.

Not in v1.8, in case they look like oversights. An overlay that **appears or
disappears is a full repaint**, because it changes how many calls a frame has
and `ir.Damage` compares them call for call; one that only moves is a damage
rectangle, which is the case a crosshair following a pointer is in. An overlay
is **not in the JSON spec**: where a pointer is is not a fact about a chart.
An overlay is given scales and areas but **not the marks**:
a tooltip that snaps to the nearest point gets that from `interact.Index` on the
caller's side, where the hit test already lives. ✔

### v1.9 — A legend you can click — **shipped**

The one piece of furniture a reader expects to act on. v1.8 said a legend was
not clickable because that would mean announcing the guides as hittable, and
this is that change: `render.LegendEntry` reports each row — which layer, what
label, what rectangle, whether it is off — and `interact` indexes them under a
kind of their own, `Guide`. The separation is the point rather than an
implementation detail: a hit on a swatch must not be confusable with a hit on
the thing the swatch stands for, so a guide hit carries a layer and a series and
deliberately carries no value read off an axis.
See [ADR 0047](docs/adr/0047-clickable-legend.md).

Hiding is `render.Chart.Hidden`, indexed by layer, because visibility is a
statement about the chart rather than about a geom — a geom that knew whether
it was being shown would be carrying a fact about a reader. A hidden layer is
not drawn and is not announced, so a pointer where it used to be finds what is
behind it; it still trains its scales, and it keeps its legend row, dimmed.

The axes deliberately do not move. A toggle is a reading aid — let me see this
one without that one on top — and an axis that rescaled under it would make the
two readings incomparable, which is what the toggle was for. A caller who means
"this series is not part of this chart" says that with `Plot.SetLayers`.

And refract does not wire the click. `Live.Toggle`, `Hide`, `IsHidden` and
`ShowAll` are the mechanism, and four lines in a Click handler are the policy —
because a legend that always toggled would be wrong for one that selects rather
than filters, one where a series opens something else, or one where only one
may be shown at a time.

Not in v1.9. A layer contributing several legend rows **toggles as one**: the
rows are one drawing and there is no way to draw a third of it. And a hidden
layer **still costs its Train**, which is what keeps the axes still: hiding a
series does not make a slow chart fast, removing it does. ✔

### v1.10 — The other two guides — **shipped**

A colourbar and a size key answer to a pointer too, and the reason they needed
a record of their own is that neither is a series. A legend row stands for a
layer, which already exists and can be toggled; a colourbar stands for a
continuum, and clicking one could mean a threshold, a range, a filter or
nothing. So a guide hit reports a **quantity** and what the quantity means is
the caller's — the same answer, and load-bearing rather than habitual: a band
that always filtered would be wrong for a chart where it should select.

A classed bar reports one band per class with the interval it covers, because
a band is a discrete thing a reader can mean. A continuous one reports itself,
and the value is read back by inverting the *ramp* — not the axis beside it,
which disagrees wherever the ramp is compressed
([ADR 0042](docs/adr/0042-colour-transforms-and-classes.md) settled which of
the two the reader is looking at). A size key row reports the magnitude its
sample stands for, which meant `sizeSample` finally carrying its value: a
filter written against "1.2k" is a filter against a string.

`interact.Kind` gained `Colorbar` and `SizeKey`, and `Guide` was renamed
`LegendRow` — once there were three kinds of actionable furniture, the general
name on the specific one was misleading.
See [ADR 0048](docs/adr/0048-clickable-colourbar-and-size-key.md).

Not in v1.10. A band reports its **midpoint** rather than a position inside
itself: one colour stands for one interval and there is no gradient in there to
read, so `Lo` and `Hi` are the truth. There is **no drag for a range** on a
continuous bar — the natural gesture, and a brush question rather than a
vocabulary one. And an **axis is still not clickable**: it is drawn per panel
rather than once per chart, so a hit would have to carry which panel and which
axis, which is a third vocabulary. ✔

### Beyond v1.0

- Harden the GPU tier as GoGPU matures.
- More coordinate systems: geographic and map projections. Polar arrived in v0.8
  ([ADR 0018](docs/adr/0018-coordinate-systems.md)) and the **Smith chart** in
  v1.2 ([ADR 0033](docs/adr/0033-smith-charts.md)) — the third `Coord`, and the
  same shape of seam: it maps a *mapped pair* through Γ = (z−1)/(z+1), and its
  grid is the two axes' own ticks, so `render` was not touched. A projection is
  still the wider one — it transforms every point with no linear interval
  underneath it, and its graticule has no tick behind it — and is argued on its
  own evidence rather than smuggled in as a fourth.
- Node-link diagrams and Venn/UpSet, which are what is left of the relational
  family after v1.4 shipped the rest of it
  ([ADR 0039](docs/adr/0039-relational-layouts.md)). A force layout's whole
  method is to run until it settles, so it cannot be a pure function of its
  input at a bounded sweep count that also looks good, and
  [ADR 0012](docs/adr/0012-parallel-panels.md) has to be answered on its own
  terms before it lands. Venn is a circle-packing optimiser and UpSet is a
  matrix chart rather than a relational layout at all.
- More stats: contour. Normal QQ plots shipped in v1.5, with a general
  quantile-function API in `stat` ([ADR 0041](docs/adr/0041-qq-plots.md)).
- ~~The rest of bucket H~~ — **shipped in v1.8**: the overlay layer the chart
  itself owns, a tooltip, a crosshair and a brush rectangle
  ([ADR 0046](docs/adr/0046-overlay-layer.md)). Linked brushing turned out not
  to need it first after all — v1.7 shipped the identification and the
  selection, and this is the feedback. Opt-in text collision avoidance shipped in v1.5
  ([ADR 0040](docs/adr/0040-label-collision-avoidance.md)); automatic avoidance
  of every kind of mark remains open.
- ~~Animations / transitions.~~ **Shipped in v1.7.** The blocker was stated
  here for six milestones: `ir.Damage` diffs two recordings at the level of
  drawing calls, so it says that something changed and not which path
  corresponds to which, and tweening needs mark identity across frames — a join
  key — where the nearest thing that existed was `geom.Rows`, which holds only
  within one frame. The sentence that resolved it was the last one: *the step
  is a key concept, not an interpolation layer*. So the key is a column the
  caller names ([ADR 0043](docs/adr/0043-mark-identity.md)) and the blend
  happens in data space, before the scales, where identity is the only thing
  that means anything ([ADR 0044](docs/adr/0044-transitions.md)). Nothing under
  the IR was touched.
- A keyframe timeline, over more than two states. A sequence is a list of
  two-state transitions and building one is a host-side loop, so the pair
  shipped first; an API that owned the sequence would be a real addition rather
  than sugar, and would be argued on the evidence of people writing that loop.
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
- **The growth rule.** An interface a third party implements — `data.Source`,
  `scale.Scale`, `coord.Coord`, `geom.Geom`, `ir.Backend`, `ir.Target`,
  `render.Observer`, `mathtext.Typesetter` and the colour and size scales —
  never gains a method. A new capability is an optional interface beside it,
  asked for with a type assertion, the way `Resizer`, `Definite` and `Legender`
  already are. A struct with exported fields gains fields and never loses one,
  and its zero value keeps its meaning. A string-typed name (`geom.Mark`,
  `scale.Kind`, `coord.Type`) is open to third-party values; an iota enum grows
  at the end.
- **The JSON dialect.** Within v1 a field is only ever added; a reader ignores a
  field it does not know; `$schema` moves only with the module's major version.
- **Nested modules.** `backend/gg` and `backend/window` tag `v1.0.0` with the
  core: they are the supported raster and native paths, and their own APIs are
  small. `backend/gg/gpu` stays `v0.x` for as long as the GPU tier is opt-in
  beta — the tag says what the README says. `arrow/v18` tracks its upstream:
  its major version follows `apache/arrow-go`'s, which is the reason it is a
  module of its own, and Go's rule that a major version above 1 is spelled in
  the import path is why the module is `…/arrow/v18` in a directory of that
  name and tags as `arrow/v18.x.y`
  ([ADR 0030](docs/adr/0030-arrow-major-version.md)).
- **License:** permissive (e.g. MIT), and the core links only permissive deps
  (gg is MIT). This is a requirement, not a preference: refract must be embeddable
  by downstream frameworks under any license — including dual-licensed ones such
  as `lux` — so no copyleft or noncommercial-licensed code enters the dependency
  graph.

---

## 16. Repository layout

Dependency boundaries enforce the "lean by default" promise: the core depends on
nothing at all, and GoGPU enters only through the nested `backend/gg` module.

Every package below exists; the milestone each arrived in is marked. `coord/`
was the last one outstanding — the pluggable stage the Cartesian mapping used to
be hard-coded into — and it landed in v0.8, settled by
[ADR 0018](docs/adr/0018-coordinate-systems.md).

```
refract/                     # core module — pure Go, STDLIB ONLY (no requires)
  refract.go                 # top-level API: New, X, Y, Add, Render      (v0.1)
  input.go                   # the portable pointer state machine         (v0.6)
  describe.go                # Plot.Describe, Plot.DataTable              (v0.6)
  data/                      # Source, Float64Columns, Table              (v0.1)
  scale/                     # linear, time (+ log, symlog, ordinal, colour) (v0.1)
                             # + Origin, for a time domain that keeps its
                             # nanoseconds at any zoom                    (v0.6)
                             # + Qualitative, the discrete colour scale   (v0.7)
                             # + Size, the area-mapped size channel       (v0.9)
  geom/                      # line, scatter, bar (+ area, step, boxplot) (v0.1)
                             # + rect, groups and the position adjustments (v0.7)
                             # + histogram, violin, ridgeline, hexbin,
                             #   beeswarm, ecdf, trend, and SizeBy        (v0.9)
  stat/                      # LTTB, min/max, density binning              (v0.4)
                             # + StackOffsets, the streamgraph baselines   (v0.7)
                             # + Bin, KDE, ECDF, Loess, Hex               (v0.9)
  coord/                     # cartesian + polar (the pluggable stage)   (v0.8)
  internal/layout/           # panel-grid constraint solver; one importer,  (v0.3)
                             # so not public — ADR 0029
  render/                    # model -> IR lowering, + Observer           (v0.1)
  facet/                     # faceting (wrap/grid)                       (v0.3)
  theme/  palette/           # tokens, colourblind-safe palettes          (v0.1)
                             # + Scaled and Redundant                     (v0.6)
  ir/                        # IR primitives + Backend interface          (v0.1)
                             # + Damage and Partial                       (v0.5)
                             # + Semantics and Resizer                    (v0.6)
  interact/                  # hit index and event vocabulary             (v0.5)
  spec/                      # JSON (Vega-Lite-shaped) marshal/unmarshal  (v0.5)
  mathtext/                  # pluggable notation, and a TeX subset       (v0.6)
  a11y/                      # description and data-table fallback        (v0.6)
  backend/svg/               # built-in, zero-dependency SVG emitter      (v0.1)
  backend/pdf/               # built-in, zero-dependency PDF emitter      (v0.3)
  backend/canvas/            # built-in canvas 2D, js/wasm only           (v0.5)
  internal/fontmetrics/      # stdlib hmtx/cmap reader + Helvetica table  (v0.1)
  internal/markers/          # the marker outlines both emitters share    (v0.3)

  backend/gg/                # NESTED MODULE: the raster backend, and the  (v0.1)
                             # in-memory Surface a window draws into.
                             # Depends on gogpu/gg. Zero CGO. PDF did not
                             # land here — see ADR 0009 — and neither did
                             # the browser — see ADR 0017.
    cmd/gallery/             # renders every documented figure            (v0.1)
    gpu/                     # NESTED MODULE: the opt-in GPU tier.        (v0.6)
                             # Importing it is the opt-in — ADR 0022.

  backend/window/            # NESTED MODULE: a native window.            (v0.6)
                             # Depends on gogpu/gogpu and on backend/gg.
                             # Zero CGO. See ADR 0021.
    show/                    # one call: show.Plot(p)                     (v0.6)
    cmd/demo/                # a signal to pan and zoom by hand           (v0.6)

  arrow/v18/                 # NESTED MODULE: Apache Arrow adapter        (v0.4)
                             # Depends on apache/arrow-go. Zero CGO. The
                             # directory is its major version — ADR 0030.
```

`backend/gg`, `backend/gg/gpu`, `backend/window` and `arrow/v18` depend on the core,
never the reverse. A nested module is excluded from its parent's module graph, so
importing only `refract` yields a graph with no external packages in it at all —
CI asserts this on every commit, and the same mechanism one level down is what
keeps `backend/gg` free of a GPU stack while `backend/gg/gpu` offers one. SVG
output works with no GoGPU present. See
[ADR 0001](docs/adr/0001-module-layout.md) for why the gg backend is a nested
module here rather than the separate `refract-gg` repository this section
originally proposed.

---

## 17. Open decisions

Six of the seven were settled while building v0.1 and the seventh — spec
fidelity — while building v0.5; the extension model, listed here as the one
that stayed open, was settled before the v1 freeze. Each has a record in
[docs/adr](docs/adr/).

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
3. ~~**Spec fidelity to Vega-Lite**~~ — **settled:** neither a strict subset nor
   "inspired by", but Vega-Lite's *vocabulary* with refract's semantics. The
   names and the structure are Vega-Lite's wherever the concept exists in both;
   what refract has and Vega-Lite does not is named plainly rather than
   smuggled through a borrowed name; and `$schema` says which dialect the
   document is. What is guaranteed is the round trip through refract, tested
   per mark and per scale.
   → [ADR 0014](docs/adr/0014-json-spec.md)
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
   it became live at v0.6, where the GPU tier is a module a caller opts into
   rather than something on the supported path.
   → [ADR 0005](docs/adr/0005-go-version.md)
7. ~~**Geom/backend extension API**~~ — the interfaces third parties implement.
   **Settled** by [ADR 0029](docs/adr/0029-extension-model.md): a third-party
   geom reads the shared options through `geom.Configure`, takes its own through
   `geom.Extra`, and is read back from a document through `geom.Register`;
   `scale` and `coord` register the same way; the JSON spec carries a registered
   mark's own properties on the mark object. Freezing it well is what makes the
   "last plotting library" claim survivable. What that costs in scheduling is
   now explicit: `geom.Frame` and the `Geom` interface are part of what freezes,
   so anything that has to widen them has to land first. The coordinate stage is
   one such thing — a `Frame` frozen as a rectangle with an X scale and a Y
   scale is a library that is Cartesian forever — and it landed in v0.8 for
   exactly that reason ([ADR 0018](docs/adr/0018-coordinate-systems.md)). A
   multi-entry legend
   is deliberately *not*, because an optional interface widens nothing
   ([ADR 0020](docs/adr/0020-discrete-colour-and-multi-entry-legends.md)). That
   is the test to apply to anything else queued behind this item: does it change
   an interface a third party implements, or ride beside it?

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
