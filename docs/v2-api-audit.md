# v2 API audit — which calls stop taking arguments

**Status:** Proposed · **Date:** 2026-09-08 · **Feeds:**
[ADR 0051](adr/0051-three-dimensional-charts.md)

## Why there is a second audit

[ADR 0051](adr/0051-three-dimensional-charts.md) decides that 3D is worth a
major version, and widens three seams from positional scale arguments to
growable parameter structs. It also supplies the reason this document exists:

> a major version is paid once whether it carries one change or three.

If that is true, the release is not "the 3D break". It is **the only chance to
correct the shape of any call that cannot be corrected additively**, and the
list of those calls has to be drawn deliberately rather than discovered a
milestone later. This walks the v1 surface a second time and asks one question
of each seam: *is this parameter list, or this result list, going to want
another entry?*

The [v1 audit](v1-api-audit.md) asked what would be regretted once the API was
public. This asks the narrower question that a major version actually answers.

## The rule

The break is spent only where growth is otherwise impossible. That is not a
preference; it follows from what Go's compatibility rules make cheap:

- **A function, or a method on a concrete type, is never a reason to break.**
  It is superseded additively — a new name, or a variadic option — and the old
  one carries a `// Deprecated:` line until the *next* major. So no function in
  this repository appears below.
- **A struct never needs the break.** It gains fields; its zero value keeps its
  meaning. CONCEPT §15 already says so, and `render.Chart` gaining `Y2` and
  `X2` in v1.3 is the proof.
- **An interface a third party implements is the only permanent shape.** It
  cannot gain a method — the growth rule says so — and it cannot gain a
  parameter, which the growth rule does not mention because until now there was
  nothing to be done about it.
- **Multiple return values are as permanent as parameters**, and are easier to
  miss because nothing about them looks like a signature that could grow.

So the audit walks the interfaces CONCEPT §15's growth rule names, and nothing
else.

## The evidence that the question is real

The v1 surface carries some forty interfaces beside the handful a third party
implements outright, and most of them are the growth rule working exactly as
designed: `data.Nulls`, `data.Subset`,
`ir.Resizer`, `ir.Partial`, `ir.Semantics`, `scale.Zoomer`, `scale.Definite`,
`scale.Snapshotter`, `geom.Rows`, `geom.Faceter` and the rest each add a
**capability** a third party may or may not have. A caller asks with a type
assertion and the answer means something.

Three do not. They exist because a call needed **another argument**, or a wider
result, and could not have one:

| Optional interface | What it really is | Landed in |
|---|---|---|
| `coord.Opposite` — `FurnitureY2`, `FurnitureX2` | `Coord.Furniture`'s tick arguments, for a second axis | [ADR 0037](adr/0037-secondary-axis.md) |
| `render.LayerAxes` — `LayerAxes(x, y)` | `Observer.Panel`'s scales, for a layer drawn against a different pair | [ADR 0037](adr/0037-secondary-axis.md) |
| `geom.Legender` — `Legends(f) []LegendEntry` | `Geom.Legend`'s single result, for a layer with several entries | [ADR 0020](adr/0020-discrete-colour-and-multi-entry-legends.md) |

Each cost a type assertion in the draw path, a second implementation in every
type that has one, and a rule a third party has to know. `coord.OppositeFurniture`
is a helper whose entire body is one of those assertions, written once so that
callers do not each write it.

That is the tax ADR 0051 declines to pay a fourth time for 3D. It is the same
tax, and the same three seams keep paying it.

## Verdicts

**WIDEN** — the shape is wrong and only a major version fixes it.
**FOLD** — an optional interface that disappears into a widened call.
**KEEP** — the shape is right, or the cost of changing it exceeds the growth it
would buy.

### The three ADR 0051 already names

| Seam | v1 | v2 |
|---|---|---|
| `geom.Geom` | `Train(x, y scale.Scale) error` | `Train(t Training) error` |
| `render.Observer` | `Panel(i int, area ir.Rect, x, y scale.Scale, cd coord.Coord)` | `Panel(p PanelInfo)` |
| `coord.Coord` | `Frame(area ir.Rect, x, y scale.Scale) Coord` | `Frame(f Framing) Coord` |

### WIDEN — ride along

| # | Seam | v1 | Why it grows |
|---|---|---|---|
| 1 | `coord.Coord.Furniture` | `Furniture(dst *Furniture, area ir.Rect, m Metrics, xTicks, yTicks []scale.Tick)` | The seam that has **already** grown a parallel path once. Its request struct carries area, metrics and ticks per axis — x, y, and whatever else the chart has — so `coord.Opposite` folds in and a third axis needs nothing new. |
| 2 | `coord.Coord.Extent` | `Extent() (x0, x1, y0, y1 float32)` | Four results cannot become six. Every mark that spans an axis reads it, and a depth interval has nowhere to appear. `Extent() Extent`. |
| 3 | `render.Observer.Layer` | `Layer(i int, label string)` | Two arguments that have wanted three since [ADR 0043](adr/0043-mark-identity.md) gave a layer a key. `Layer(l LayerInfo)` absorbs `render.LayerAxes` in the same move. |
| 4 | `render.LegendEntry` | `LegendEntry(layer int, label string, area ir.Rect, hidden bool)` | Four positional arguments describing one row of a legend, the newest surface in the library ([ADR 0047](adr/0047-clickable-legend.md)), and the one most likely to gain a swatch kind or an entry index. |
| 5 | `render.ColorbarEntry` | `ColorbarEntry(cs scale.ColorScale, class int, lo, hi float64, area ir.Rect)` | Five arguments with a sentinel in them: `class` is -1 for a continuous bar, and `lo` and `hi` then mean something else. That is a struct with a variant field, spelled as a parameter list. |
| 6 | `render.SizeKeyEntry` | `SizeKeyEntry(value float64, label string, area ir.Rect)` | The sibling of the two above ([ADR 0048](adr/0048-clickable-colourbar-and-size-key.md)); it should not be the one left in the old shape. |
| 7 | `ir.Target.Open` + `ir.Resizer.Resize` | `Open(widthPx, heightPx int, dpr float64) (Backend, error)`, `Resize(widthPx, heightPx int, dpr float64) error` | Two calls, one meaning — `Resize`'s doc comment says "the arguments mean what `Target.Open`'s do", which is a `Surface` struct that was never written. A background colour, a colour space, a page size or an orientation all belong there and none of them can get there today. |
| 8 | `mathtext.Typesetter.Typeset` | `Typeset(src string, font ir.FontRef, m Measurer) (Layout, bool)` | A typesetting request that cannot carry a direction, a language or a size context. It is the smallest of the eight and the cheapest to widen — one implementation in the tree. |

### FOLD — parallel paths v2 absorbs

| Removed | Into |
|---|---|
| `coord.Opposite`, `coord.OppositeFurniture` | `Coord.Furniture`'s request (WIDEN 1) |
| `render.LayerAxes` | `Observer.Layer`'s struct (WIDEN 3) |
| `geom.Legender`; `Geom.Legend` becomes `Legends(f Frame) []LegendEntry` | The interface itself — one method, one result shape, no fallback |

Folding is the half of the release that pays for itself immediately: three type
assertions leave the draw path, and three rules leave the documentation a third
party has to read.

### KEEP

| Seam | Verdict | Why |
|---|---|---|
| `ir.Backend` — all ten methods | **KEEP** | Nothing is pending against it. Its parameters are already structs (`Stroke`, `Fill`, `MarkerStyle`, `TextRun`), it has not grown a parameter since v0.1, and ADR 0051 makes "every backend renders 3D on the day `three` compiles" the invariant the whole design rests on. `FillPath`'s `rule` and `Markers`' `shape` could fold into their style structs; that is tidying, and it would cost every third-party backend a rewrite in exchange for nothing it can observe. |
| `data.Source` — five methods | **KEEP** | A column accessor takes a name and returns `(data, ok)`. A fourth column kind is a capability, not a parameter, and the v1 audit already routed it to an optional interface. That routing is correct. |
| `scale.Scale.Train`, `SetRange`, `Domain`, `Map`, `Invert` | **KEEP** | Scalar calls on the hot path. `Map` is called once per row per frame; wrapping its argument is the allocation the IR's whole design exists to avoid, and none of them has anywhere to grow. A third axis is a third `Scale`, which is what [ADR 0018](adr/0018-coordinate-systems.md) bought. |
| `scale.ColorScale`, `SizeScale`, and the classed and discrete extensions | **KEEP** | Same shape, same reason. `ClassedColorScale` and `ColorTransformer` are capabilities and stay optional interfaces. |
| `coord.Coord.Point`, `Points`, `Edge`, `Area`, `Clip`, `Invert`, `Straight`, `Decimates` | **KEEP**, subject to Q1 below | Per-point and per-batch calls whose arguments are complete. |
| `geom.Geom.Build` | **KEEP** | `Build(b ir.Backend, f Frame) error` is already the pattern the other three seams are being moved to. `Frame` grows fields. |
| `coord.Exploder.Explode` | **KEEP** | Five scalars, but they are the two mapped corners and a fraction — a closed description of a displacement, with nothing pending. |

## Two questions this audit cannot answer alone

**Q1 — is the coord z-aware, or is it not?** ADR 0051 widens `Coord.Frame` to a
`Framing`, which implies the coord is handed a third scale; it also says
`three` owns the projection, the camera and the depth order, and that `render`
gains nothing. Both cannot be fully true. If `three` projects, then `Framing`
is the only z-shaped thing in `coord` and `Extent`, `Point`, `Points` and
`Furniture` stay two-dimensional — in which case widening `Frame` buys
consistency rather than capability, which is a fine reason but a different one.
If instead the coord is the stage a third dimension passes through, then WIDEN
2 is mandatory and `Point`/`Points` move out of KEEP. **The answer changes this
table, so it is decided in ADR 0051 before any of this is implemented.**

**Q2 — `scale.Scale.Ticks(want int) []Tick`.** Resolved below, under what the
toll costs: **WIDEN**. A tick search that knew the device width available for
labels could stop proposing labels that will collide — the problem
[ADR 0040](adr/0040-label-collision-avoidance.md) solves for marks and not for
axes, and the one growth this signature has a name for. Every tick concern so
far has been absorbed by `scale.Locale`, `TickValues` and the `Labeller`
optional interface without touching it, which is the case for DEFER; the case
against DEFER is that the next one cannot be, and the toll for finding that out
is a whole major version. `Ticks(req TickRequest) []Tick`.

The principle does not generalise to widening everything that *could* grow.
It generalises to widening what has a named pending growth, which is why
`ir.Backend` stays KEEP and this does not.

## What this does not propose

- **No renames.** Cheap in a major version, but a rename that is not a shape
  fix is churn a caller pays for and learns nothing from.
- **No removal of the option functions.** `Plot`, `Grid`, `Track`, the scales
  and the geoms take variadic options, which is the growable form already.
- **No change to the JSON spec beyond `$schema`.** CONCEPT §15 says the major
  version moves it; nothing in this audit adds to that.
- **Nothing in the nested modules' own APIs.** `backend/gg`, `backend/window`
  and `arrow/v18` re-require the core and tag again — ADR 0051's list — but
  their surfaces are unaffected by anything above.

## Cost

Counted in the tree as it stands:

| Change | Implementations to update |
|---|---|
| `Geom.Train` | 26 geoms |
| `Coord.Frame`, `Extent`, `Furniture` | 3 coords |
| `Observer.Panel`, `Layer`, and the three guide-entry calls | 1 (`interact.Index`) plus test observers |
| `Target.Open`, `Resizer.Resize` | 11, of which 5 are shipped backends and the rest examples and test doubles |
| `Typesetter.Typeset` | 1 |

Every one is mechanical: a parameter list becomes a field read, and the
compiler finds each site. That is the same claim ADR 0051 makes for its three,
and it survives being extended to eight because the eight are the same kind of
change.

## What has to change elsewhere

The CONCEPT §15 clause ADR 0051 drafts names three seams. If this audit is
accepted it names the set instead, and the sentence that matters — *a seam
whose shape is wrong is corrected rather than papered over with a parallel
path* — is unchanged by the list getting longer. So is the reason it is
affordable now and will not be later.

## What the resolver does, and what that costs the nested modules

A major version is a different module with a different import path, so **no Go
command ever crosses one**. `go get -u`, `go get -u ./...` and `go list -m -u
all` upgrade within v1 and never see v2 as an update; a caller moves by running
`go get github.com/timzifer/refract/v2` and rewriting every import line. Both
majors may sit in one build — they are different packages, and their types do
not interconvert — which is what makes a partial migration possible and is also
the only way to get one by accident. The single automatic signpost is
pkg.go.dev, which notes on the v1 page that a higher major is tagged.

That is fine for the core. It is not fine for the nested modules, and ADR 0051
currently says it is:

> `backend/gg`, `backend/window` and `arrow/v18` re-require the core and tag
> again. Their own APIs do not change.

**Their own APIs do change**, because each names a core type in its signatures:

| Module | Exported signature | Core type |
|---|---|---|
| `backend/gg` | `PNG`, `JPEG`, `Writer` | return `ir.Target` |
| `backend/window` | `Window`, `Handler` | take the core's backends and events |
| `arrow/v18` | `Source`, `TableSource`, `Materialize` | return `data.Source`, `*data.Table` |

A `backend/gg v1.8.0` that requires the core at v2 returns **v2's** `ir.Target`,
which a caller still on core v1 cannot pass anywhere. That is a breaking change
published as a minor — and because `go get -u` *does* move within a major, a
caller on core v1 who runs it lands on that release, pulls a second copy of the
core into the graph, and gets a type error naming two identically spelled types.
The resolver being unable to cross a major is what makes this silent: nothing
warns, because from Go's point of view nothing unusual happened.

So the rule for the release is one line: **a module that names a core type in
its exported API tags a major of its own alongside the core.** `backend/gg` and
`backend/window` become `…/backend/gg/v2` and `…/backend/window/v2`.
`backend/gg/gpu` is at `v0.x`, where breaking is permitted, and simply moves to
`v0.4.0`.

### `arrow/v18` cannot follow, and does not have to

Its major is Apache Arrow's ([ADR 0030](adr/0030-arrow-major-version.md)), so it
cannot spend one on refract's break without claiming an Arrow release that does
not exist. The way out is available only because this audit puts `data.Source`
under KEEP:

**Go interfaces are structural.** A concrete type with `Len`, `Columns`,
`Float64Column`, `TimeColumn` and `StringColumn` satisfies v1's `data.Source`
and v2's, because neither changed. So `Source` and `TableSource` return their
own exported concrete type instead of the core's interface, the module stops
importing the core altogether, and it works against both majors — and against
every major after them, for as long as `data.Source` holds still. `Materialize`
is the one casualty: `*data.Table` is a struct, and a structurally identical
struct in another module is a different type. It moves into the core or it goes.

That is not a workaround for v2. It is the fix for ADR 0030's conflict in
general, and v2 is when it becomes cheap.

**`backend/gg` has no such escape**, and the reason marks the line exactly: it
consumes `ir.Point`, `ir.Stroke`, `ir.Rect` and `ir.TextRun`, which are structs.
Only a boundary made entirely of interfaces can be crossed structurally. This is
independent of WIDEN 7 — gg needs a major of its own whether or not
`ir.Target.Open` widens, so nothing above is an argument against widening it.


## The toll on a newcomer, and why it argues for one break

Go has no forwarding from a major version to the next. A newcomer who copies
`go get github.com/timzifer/refract` gets v1 and is told nothing, and no
`replace`, no field in `go.mod` and no tag can change that. It is the import
compatibility rule working as designed, and it is the part of the design that
costs a library something real.

Four channels exist. Only the first is in-band:

**1. Deprecate the v1 module.** A comment paragraph beginning `Deprecated: `
directly above the `module` line:

```
// Deprecated: use github.com/timzifer/refract/v2 instead.
module github.com/timzifer/refract
```

It is a first-class field of `go.mod` — `go mod edit -json` reports it as
`ModPath.Deprecated` — and `github.com/golang/protobuf@v1.5.4` is the reference
example in the wild. The go command prints it from `go get` and `go list -m -u
all`, and pkg.go.dev banners the whole module page with it.

**It is read from the module's *latest* version**, so it needs a final v1
release that carries nothing else. That release is part of tagging v2, not an
afterthought to it: without it, none of the three effects above happen.

What it actually does, measured against `github.com/golang/protobuf@v1.5.4`,
which carries exactly this marker:

| Command | Says it? | Result |
|---|---|---|
| `go get <v1 path>` | **yes**, naming the successor | installs **v1** |
| `go get <v1 path>@latest` | **yes**, naming the successor | stays on v1 |
| `go get -u ./...` | **yes**, naming the successor, even when it upgrades nothing | stays on v1 |
| `go list -m -u all` | marks the line `(deprecated)` — **without** the successor path | — |
| `go mod tidy` | **no** | pulls v1 silently |
| `go build` | **no** | — |

So nothing ever installs v2, and nothing ever offers to. A caller who asks for
the v1 path gets v1 and a sentence telling them where v2 is; a caller who writes
the import first and runs `go mod tidy` gets v1 and **silence**. That second path
is the one a newcomer copying an example actually takes, and it is why channel 3
below is not a formality: the deprecation marker covers the `go get` route, and
the documentation is the only thing covering the other one.

**2. pkg.go.dev's own note**, "The highest tagged major version is …/v2".
Automatic, and quiet.

**3. The documentation that actually gets copied.** In practice the install line
comes from the README, the badges and the first example — so all of them carry
`/v2`, including `go.mod`'s module line in every snippet and the package doc.
It is the least clever channel and the one with the most traffic.

**4. Optionally, `// Deprecated:` on v1's exported identifiers** in that final
release, so an editor strikes them through for whoever is already on v1. It
costs one pass and breaks nothing.

**`retract` is not one of these.** It means "these versions are broken", not
"these versions are old". v1 is not broken, and retracting it would put a
warning in the builds of everyone who did nothing wrong, in service of a claim
that is false.

### What that implies

The toll is paid per major version, and its size is *adoption × time spent on
the old path*. Both factors are near zero today and only grow. That is ADR 0051's
argument arriving from the other side: not only is the break cheap now, the
**toll** is cheap now, and it is the toll — not the migration — that is
permanent.

Which is what this audit is actually for. Its value is not tidiness; it is that
**v2 should be the last major version this library needs**. Every seam left in
the wrong shape today is a second toll later, collected from a larger population
than the one that would pay it now. That is the reasoning that moves Q2 from
DEFER to WIDEN, and it is the standard the rest of the table should be re-read
against before anything is implemented.
