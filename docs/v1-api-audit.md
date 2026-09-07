# v1 API freeze audit

This is the last question asked of the public API before it freezes: not *what
is missing*, but *which decision would we regret in six months once it is
public*. It walks every exported identifier in the core module and the four
nested modules as they stand at the v0.9 merge (`5d6f7c9`), taken from
`go doc -short` per package, and gives each one of three verdicts.

| Verdict | Meaning |
|---|---|
| **FREEZE** | Ship as is. Anything that grows from here grows additively. |
| **CHANGE BEFORE V1** | Change now while it is free. Once tagged, the same change is a v2. The change may be to a signature, a package boundary, or only to a doc comment that pins a rule. |
| **DEFER** | Not a v1 question. It lands after the tag without touching what froze. |

The result in one paragraph: **four of the five stability surfaces freeze as
they stand.** The data boundary, the IR and `Backend` contract, the coordinate
stage and the event model are the right abstractions and are shaped so they can
grow without breaking. **The fifth — the extension model — is not done**, and
the repository already says so: `CONCEPT.md` §17 item 7 is the one open
decision, and §14's v1.0 line promises a "stable registration/extension model
for third-party geoms and backends". The backend half of that promise is kept.
The geom half is not: a third-party geom cannot read the shared option set, and
nothing a third party writes can be reached by `FromDesc`, `spec` or `ByName`.
That is the blocker, and it is the only one. Beside it sit a handful of cheap
renames and one package to pull under `internal/`, all of which cost nothing
today and a major version tomorrow.

## Status

The audit was taken at `5d6f7c9`. Everything it asked for under CHANGE BEFORE
V1 landed on the same branch, recorded in
[ADR 0029](adr/0029-extension-model.md):

| # | Change | Done |
|---|---|---|
| 1 | `geom.Configure(opts...) Desc` and `Desc.Options()` — a third-party geom reads the shared options; `geom.Extra(key, v)` gives it a knob of its own | yes |
| 2 | `geom.Register`, `scale.Register`, `scale.RegisterColor`, `coord.Register`; every `FromDesc` falls through to its registry; `spec.Mark.Extra` carries a registered mark's own properties on the mark object | yes |
| 3 | `coord.Option` → `coord.PolarOption` | yes |
| 4 | The scale option-family naming is decided and written in the `scale` package doc: the bare names belong to the default scale | yes |
| 5 | `layout` → `internal/layout` | yes |
| 6 | `mathtext.Symbols` → `RegisterSymbol` and `Symbol` over a locked table | yes |
| 7 | `spec.Schema` is `…/spec/v1`, its comment states the additive-field policy, the README example agrees | yes |
| 8 | The growth rule is in the doc comment of every interface a third party implements, and `stat`'s package doc says why there is no `Stat` interface | yes |
| 9 | CONCEPT §15 says which nested modules tag with the core | yes |
| 10 | `arrow` tracks its upstream's major version — which Go only allows with the version in the import path, so the module is `…/arrow/v18` in the `arrow/v18` directory ([ADR 0030](adr/0030-arrow-major-version.md)) | yes |

One decision differs from the sketch below: the audit proposed a new `Config`
type with accessors, and the ADR chose to return the existing `Desc` instead.
The package-by-package tables that follow are the findings as taken; the
verdicts are what the tag rests on, and the changes above are what turned the
CHANGE BEFORE V1 rows into FREEZE.

## The rules the audit applied

Go's compatibility rules decide what is expensive to freeze, so the verdicts
follow five of them rather than taste.

1. **An interface a third party *implements* cannot gain a method.** `Geom`,
   `Scale`, `ColorScale`, `SizeScale`, `Coord`, `Backend`, `Target`, `Source`,
   `Typesetter`, `Rows` and `Observer` are that kind. They grow through optional
   interfaces — the pattern `Resizer`, `Definite`, `Legender` and `Subset`
   already set — and the freeze must say so in each one's doc comment.
2. **An interface or concrete type a third party only *calls* can gain
   methods freely.** `Plot`, `Live`, `Input`, `Grid`, `Index`, `Stream`,
   `Table` are cheap to freeze.
3. **A struct with exported fields can gain fields.** It cannot lose or rename
   one, and its zero value has to keep meaning what it means. `Frame`,
   `Event`, `Hit`, `Theme`, `Desc`, `render.Chart` are all fine on this rule.
4. **A functional option over an unexported config is additive-safe but
   closed.** New options can be added forever; nobody outside the package can
   read what an option set. That is the shape of the blocker.
5. **A string enum is open, a `uint8` iota enum is append-only.** `geom.Mark`,
   `scale.Kind`, `coord.Type` can take third-party values; `Missing`,
   `Decimation`, `EventKind` can only grow at the end.

The test from `CONCEPT.md` §17 is the same one in different words: *does this
change an interface a third party implements, or ride beside it?*

## Summary of what has to change

| # | Where | Change | Cost now | Cost after v1 |
|---|---|---|---|---|
| 1 | `geom` | Third-party geoms can read the shared options | A new exported type and one function | Impossible without a second option type |
| 2 | `geom`, `scale`, `coord`, `spec` | Registries replace the closed `FromDesc` switches; `spec` decides how a registered mark's own properties travel | Three `Register` functions and one decision | Every third-party geom is unserialisable forever |
| 3 | `coord` | `Option` becomes `PolarOption` | A type rename nobody spells | The name is taken when a second coord with options arrives |
| 4 | `scale` | Decide the option-family naming and write the decision down | A paragraph, or a rename | A rename is a v2 |
| 5 | `layout` | Move under `internal/` | A path change with one importer | Every layout refinement is a breaking change |
| 6 | `mathtext` | Decide whether `Symbols` stays a mutable package map | A doc comment or a `RegisterSymbol` | A race nobody can fix without breaking callers |
| 7 | `spec` | `Schema` moves to `/spec/v1`; the additive-field policy is written; the README example stops saying `v0.7` | Docs | A schema string that lies |
| 8 | `data`, `scale`, `geom`, `ir`, `interact` | The growth rule of each frozen interface goes into its doc comment | Docs | Argued from scratch at the first request |
| 9 | Nested modules | Decide which tag with the core and which stay `v0.x` | A paragraph in CONCEPT §15 | Confusion about what v1 promised |

Items 1 and 2 are the blocker. Items 3 to 9 are the cheap regrets. Everything
else in this document is FREEZE or DEFER.

## Package by package

### `refract` (root)

| Identifier | Verdict | Note |
|---|---|---|
| `Plot`, `New`, `Option` and the thirteen option functions | FREEZE | Struct is opaque; methods can be added. `Coord`, `Theme`, `Size` share names with packages and with `geom.Size`, `scale.Size`; package qualification carries it. |
| `Plot.X/Y/Add/Facet/On/Render/Live/Size` | FREEZE | |
| `Plot.Spec/MarshalJSON/UnmarshalJSON`, `FromSpec`, `ParseJSON` | FREEZE | Contingent on `spec` (below); the Go signatures are right. |
| `Plot.Describe/Description/DataTable`, `Description` option | FREEZE | Three near-identical names for one concern. Accepted: the option sets, the method computes, the getter reports. |
| `Grid`, `NewGrid`, `GridOption` and the nine `Grid*` functions | FREEZE | One-for-one mirror of the `Plot` options. A shared option type would need a shared receiver and there is none. |
| `Live` and its seventeen methods | FREEZE | `Move`, `Click`, `Leave` return `Event`; `Wheel`, `PanBy`, `ZoomTo`, `Autoscale` return `error`. The split is real — the first three do not redraw, the rest do — and the doc comment should say so once. |
| `Input` and its eleven methods, `DefaultClickSlop`, `WheelFactor` | FREEZE | The portable state machine. `Input.Live()` and `Live.Input()` are a pair. |
| `Source`, `Target`, `Backend`, `Event`, `EventKind`, `Hit` aliases; `Hover`…`Pan` | FREEZE | Aliases, so they are the same types; nothing can drift. |
| `Float64Columns`, `NewTable`, `SVG`, `SVGWriter`, `PDF`, `PDFWriter` | FREEZE | |
| `ErrNoLayers`, `ErrEmptyGrid` | FREEZE | |

The root package exposes no getters (`Layers()`, `Theme()`): a plot is written,
not read. That is fine — `Plot.Spec()` is the read path — and adding a getter
later is additive.

### `data`

| Identifier | Verdict | Note |
|---|---|---|
| `Source` | FREEZE — **write the growth rule** | Five methods, three column kinds. Rule to pin in the doc comment: **`Source` never gains a method.** A fourth column kind — exact integers, booleans, durations — arrives as an optional interface (`type IntSource interface { Int64Column(name) ([]int64, bool) }`), the way `Subset` did. Without the rule written down the first request will argue for widening. |
| `Float64Columns`, `Table`, `NewTable`, `Table.Float64/Time/String` | FREEZE | Borrowing semantics documented. |
| `Rows`, `Subset`, `Origins` | FREEZE | The row-identity chain. One level, documented. |
| `GroupBy`, `Labels` | FREEZE | |
| `Stream`, `NewStream` and its methods, `ErrColumnCount` | FREEZE | Numeric-only is documented and reasoned (a timestamp is nanoseconds). A time-carrying stream later is a new type, not a change. |
| `FormatNumber` | FREEZE | An odd home for a `strconv` wrapper, but `a11y` and `geom` need one canonical formatter and this is it. Not worth a move. |

Missing data is NaN in a float column and nothing in a time or string column.
That is a documented limitation rather than a gap: a missing category is an
empty string, and an empty string is a legal category. DEFER a null mask; it
would be an optional interface if it ever comes.

### `stat`

Everything here is FREEZE. Pure functions, an `Append` form beside each
allocating one, generics over `Float` for the device-space family, `float64`
for the data-space family. `Bucket`, `Point`, `Cell`, `Grid`, `Hex` are plain
structs. `Scaling` and `StackMode` are append-only enums.

`stat.Point` is `float64` and `ir.Point` is `float32`: data space versus device
space, and the two never meet. `stat.Linear` (a `Scaling`) and `scale.Linear`
(a constructor) share a name across packages; acceptable.

**By design there is no `Stat` interface.** A stat is a function a geom calls
in `Train` ([ADR 0028](adr/0028-distribution-stats.md)), not a pluggable layer
between data and geom. That is the GoG-lite decision and the audit agrees with
it — a pluggable stat would need to know about scales and the layer's missing
policy, which is exactly what `AGENTS.md` forbids `stat` to know. It should be
said once, in the package doc, before v1: the first question after the tag
will be "how do I plug in my own stat", and the answer is "write a geom that
calls it".

### `scale`

| Identifier | Verdict | Note |
|---|---|---|
| `Scale` | FREEZE — **write the growth rule** | Six methods since v0.1. Seven optional interfaces already ride beside it. Pin the rule: `Scale` never gains a method. |
| `Definite`, `Categorical`, `Band`, `Temporal`, `Zoomer`, `Snapshotter`, `Cloner` | FREEZE | The precedent everything else follows. |
| `Tick` | FREEZE | Struct; fields addable. |
| `ColorScale`, `DiscreteColorScale`, `Discrete`, `SizeScale` | FREEZE | Same shape, same rule. |
| `Linear`, `Log`, `SymLog`, `Time`, `Ordinal`, `Size`, `Sequential`, `Diverging`, `Qualitative` | FREEZE | |
| `LinearOption`, `LogOption`, `SymLogOption`, `TimeOption`, `OrdinalOption`, `SizeOption`, `ColorOption` and their functions | **CHANGE BEFORE V1** — decide | Seven option types. Linear's options are bare (`Domain`, `Nice`, `Zero`, `Format`), Log's and SymLog's are prefixed (`LogDomain`, `LogNice`, …), Size's and Color's are prefixed, Time's are mixed (`In`, `Origin`, `TimeFormat`), Ordinal's are mixed (`Categories`, `OrdinalPadding`). A reader cannot predict a name. Two honest resolutions: (a) prefix Linear's too (`LinearDomain`, …), which breaks every quick start ever written; (b) keep it and write down that **the unprefixed set belongs to the default scale**. Recommend (b), recorded in the package doc — but decided, not inherited. |
| `Kind`, `Desc`, `Describe`, `Describer`, `FromDesc`, `ErrUnknownKind` | **CHANGE BEFORE V1** | `FromDesc` is a closed switch over five kinds. See *The blocker* below. `Describer` and `Desc` themselves are right. |
| `ColorKind`, `ColorDesc`, `ColorDescriber`, `DescribeColor`, `ColorFromDesc`, `SizeDesc`, `SizeDescriber`, `DescribeSize`, `SizeFromDesc` | **CHANGE BEFORE V1** | Same: closed switches. |
| `Nanos`, `FromNanos`, `InstantOf`, `ValueOf`, `DefaultMaxSize` | FREEZE | |

### `coord`

| Identifier | Verdict | Note |
|---|---|---|
| `Coord` | FREEZE — **write the growth rule** | Eleven methods is a lot for a third party to implement, but every one is used and a coord that cannot answer `Furniture` or `Clip` is not a coord. Pin the rule; a projection's extra needs (`Graticule`?) ride beside it. |
| `Describer`, `Exploder` | FREEZE | Optional, as they should be. |
| `Metrics`, `Furniture`, `Shape`, `Label` | FREEZE | Structs. `Furniture` is filled, not returned — the allocation contract is part of the freeze. |
| `Cartesian`, `Polar`, `Pie`, `Donut` | FREEZE | |
| `Option` and `Arc`, `Chord`, `Counterclockwise`, `Hole`, `Radius`, `Start`, `Sweep`, `Theta` | **CHANGE BEFORE V1** — rename the type | `coord.Option` is `func(*polar)`: it is a **polar** option wearing the package's generic name. The moment a second coord with options exists — a projection has several — the name is taken. Rename to `PolarOption` now. No call site spells the type, so nothing breaks today. |
| `Axis`, `FromX`, `FromY`, `FullTurn` | FREEZE | |
| `Type`, `Desc`, `Describe`, `FromDesc` | **CHANGE BEFORE V1** | Closed switch. See *The blocker*. |

### `geom`

| Identifier | Verdict | Note |
|---|---|---|
| `Geom` | FREEZE — **write the growth rule** | Three methods, and `Legender`'s own doc comment already says why it stays three. Pin it. |
| `Frame`, `Frame.Coords`, `Frame.Marks` | FREEZE | Struct with a nil-safe accessor; fields addable. `Index` for the default palette colour is a little bare, but changing it buys nothing. |
| `Faceter`, `Guided`, `Sized`, `Legender`, `Describer`, `Rows` | FREEZE | The optional interfaces. Every one is documented as optional and why. |
| `Legends`, `LegendsOr`, `LegendEntry`, `SwatchKind`, `ColorGuide`, `SizeGuide` | FREEZE | |
| The twenty-one constructors | FREEZE | Fourteen data marks and seven annotations. |
| `Option` and its forty-two functions | **CHANGE BEFORE V1** | This is the blocker's first half. `Option` is `func(*config)` over an unexported struct with no exported accessor. A geom outside this package can accept `...geom.Option` and then **cannot read a single one of them** — not `X`, not `Color`, not `Label`, not `OnMissing`. `CONTRIBUTING.md` tells a contributor to "reuse the shared `geom.Option` set rather than inventing per-geom options", and the package doc promises "one namespace instead of six"; both are true only inside the package. |
| `Mark` and its twenty-one constants, `Desc`, `Datum`, `Describe`, `FromDesc`, `ErrUnknownMark` | **CHANGE BEFORE V1** | The blocker's second half. `Mark` is a string and therefore open; `FromDesc` is a switch and therefore closed. `Desc` has over fifty exported fields and is fine on rule 3. |
| `Missing`, `Decimation`, `Stacking`, `Ordering`, `StepPos`, `Smoothing`, `SwatchKind` | FREEZE | Append-only enums. |
| `ErrCategorical`, `ErrNoColumn`, `ErrNotCategorical`, `DefaultHexRadius` | FREEZE | |

### `facet`

All FREEZE. `Spec` is a concrete type with `Wrap` and `Grid` behind it; a
third party cannot add a facet strategy, and that is right for now — faceting
is a data operation over one column, and `Faceter` on the geom side is where
the extension point already is. `Spec.Split` and `Spec.FreeScales` are public
because the root package needs them; they are the kind of method rule 2 lets grow.
DEFER a pluggable facet strategy until a chart needs one.

### `theme` and `palette`

All FREEZE. `Theme` has nearly sixty exported fields; rule 3 makes that
cheap, and `Tokens` + `Build` is the documented way to make a theme without
naming them. `Register`/`ByName`/`Names` and `RegisterRamp`/`RampByName` are
the registry precedent the blocker fix should copy exactly.

`DefaultSeriesDashes` and `DefaultSeriesMarkers` are exported mutable slices,
like `Symbols` below. They are read when a `Theme` takes them, never during a
render; a caller mutating them at runtime gets what they asked for. Acceptable.

### `layout`

**CHANGE BEFORE V1: move to `internal/layout`.**

`Chart`, `Guide`, `GuideKind`, `Result`, `Grid`, `Panel`, `GridResult`,
`Measurer`, `Compute` and `Panels` have exactly one importer, `render`, and no
conceivable third-party caller: a backend consumes IR and never lays out, a
geom is handed its rectangle, and a tool that wants a layout calls `render`.
Freezing these publicly means every refinement of the solver — a fourth guide
kind, a margin field, a different result shape — is a breaking change to the
module for the benefit of nobody. `ADR 0010` does not promise the solver's
shape, only its existence. Pull it under `internal/` now; the import path
changes in two files.

If there is a reason to keep it public that the audit does not see, the
alternative is to say in the package doc that `layout` is outside the
compatibility promise — but Go has no mechanism for that, and `internal/` is
the honest spelling.

### `render`

| Identifier | Verdict | Note |
|---|---|---|
| `Draw` | FREEZE | The one drawing order, in one place. |
| `Chart`, `Panel` | FREEZE — **say who it is for** | `Chart` is the resolved model, and `spec.Chart` mirrors the half of it that can be written down. It is the right entry for a caller assembling a chart without the root `Plot`. It also carries plumbing — `Serial`, `Observer`, `RowSink` — that a caller will set wrong. The package doc should say the root package is the supported entry and this is for tool authors. Fields are addable. |
| `Observer` | FREEZE | The hit-testing seam ([ADR 0015](adr/0015-hit-testing.md)). Two methods a third party implements — so it too never gains one; pin the rule. |

### `ir`

All FREEZE. `Backend` has been frozen since v0.1 ([ADR 0002](adr/0002-ir-and-backend.md))
and five backends consume it. `Target`, `Resizer`, `Partial`, `Semantics`,
`Measurer` follow the optional-interface pattern and say why in their comments.
`Recorder`, `Damage`, `Path`, `Affine`, the style structs, `Color = color.NRGBA`
and the enums are all right on rules 2, 3 and 5.

The one thing ADR 0002 lists under *revisit if* — instanced markers with
per-instance colour or size — is still open and is **DEFER**: it arrives as an
optional interface (`InstancedMarkers`) that a backend may implement and the
scatter geom may probe, exactly as `Partial` did. Nothing in the frozen set has
to move for it.

### `interact`

All FREEZE. `Event` is one struct with a `Kind`, chosen over one type per kind
for exactly the reason the freeze needs: a new kind is an appended constant and
a new field is an added field. `Hit` carries `Row` when tracking is on. A
brush or a linked selection later is `Select EventKind` plus a `Rows []int`
field — additive. `Index` is concrete and its methods can grow. `Kind`
(`Vertex`, `Area`, `Label`) is append-only.

### `a11y`

All FREEZE. `Chart` is the model, not the picture, which is the right thing
to describe. `Describe`, `Table`, `WriteTable` are functions over it. A
third-party geom that does not implement `geom.Describer` is named by position
rather than skipped — documented, correct, and the extension fix makes it
rarer.

### `spec`

| Identifier | Verdict | Note |
|---|---|---|
| `Spec`, `Of`, `Parse`, `Spec.Chart`, `Spec.Marshal`, `Chart` | FREEZE | The Go API is right. |
| `Layer`, `Mark`, `Encoding`, `Channel`, `Scale`, `Coord`, `Data`, `Format`, `Facet`, `FacetField`, `Resolve`, `ResolveScale`, `Config` and the string constants | FREEZE | Struct fields are addable. **The JSON dialect is the real surface**, and it is more constrained than Go: a field written by v1.0 must read in v1.x forever. `Parse` is already not gated on `$schema`, which is the right policy for reading. |
| `Schema` | **CHANGE BEFORE V1** | Becomes `…/spec/v1` at the tag, and the comment states the policy: **within v1, fields are only ever added, and a v1.0 reader given a v1.x document ignores what it does not know.** The README's JSON example still says `v0.7`; fix it in the same change. |
| Encoding of third-party marks | **CHANGE BEFORE V1** — decide | `encode.go` and `decode.go` switch on `geom.Mark` per property. A mark registered by a third party (see *The blocker*) has no way to carry its own properties through `spec.Mark`. Either (a) `spec.Mark` gains an `Extra map[string]any` (or `json.RawMessage`) that a registered mark's `Desc` can read and write, or (b) v1 says plainly that a third-party mark round-trips its shared properties only. Either is defensible. Not deciding is not. |

### `mathtext`

| Identifier | Verdict | Note |
|---|---|---|
| `Typesetter`, `Measurer`, `Plainer` | FREEZE | `Typesetter` is implemented by third parties — one method, never gains another; pin the rule. |
| `Layout`, `Run`, `Layout.Empty`, `Layout.Metrics`, `Draw`, `PlainOf` | FREEZE | |
| `TeX`, `TeXOption`, `Italic`, `ScriptScale` | FREEZE | |
| `Symbols` | **CHANGE BEFORE V1** — decide | An exported mutable `map[string]rune` read by every `TeX` typesetter. The comment says "a plain map so that a caller can add to it". A caller adding to it while another goroutine renders is a data race, and once frozen there is no way to make it safe without breaking whoever writes to it. Either say **init-time only** in the comment and accept the footgun, or make it unexported behind `RegisterSymbol(name string, r rune)` with a lock. Recommend the second; it is the same shape as the other registries. |

### `backend/svg`, `backend/pdf`, `backend/canvas`

All FREEZE. Each is `File`/`Writer` (or `Element`/`Context`) plus a small
`Option` type over an unexported struct — closed, additive-safe, and nothing
outside needs to read them.

### Nested modules

Each is its own module with its own tag, so "v1" is a statement about the core
unless CONCEPT §15 says otherwise. It should.

| Module | Verdict | Note |
|---|---|---|
| `backend/gg`: `PNG`, `JPEG`, `Writer`, `Format`, `Option`, `WithFont`, `JPEGQuality`, `Surface`, `NewSurface` | FREEZE | Tag `backend/gg/v1.0.0` with the core: it is the supported raster path. Its own API is small and settled. |
| `backend/gg/gpu`: `Enabled`, `Close` | DEFER | Stays `v0.x`. The README and CONCEPT both say opt-in beta past v1.0; the module tag should agree. |
| `backend/window`: `Window`, `New`, `Handler`, `Option` and its four functions, `Window.Run`; `show.Plot`, `show.Into` | FREEZE | `Handler` is a struct of callbacks — fields addable. Tag with the core; it rasterises on the CPU through `backend/gg` and depends on nothing beta. |
| `arrow`: `Source`, `TableSource`, `Materialize` | FREEZE | Three functions. Its major version is coupled to `apache/arrow-go`'s, which is why it is a module; say so in §15. |

## The blocker, spelled out

Three pieces, one ADR. Suggested number 0029, closing §17 item 7.

**1. A third-party geom can read the shared options.**

```go
// geom
type Config struct{ /* unexported */ }

// Configure applies opts and returns what they set.
func Configure(opts ...Option) Config

func (c Config) X() string
func (c Config) Y() string
func (c Config) Color() (ir.Color, bool)   // set or not
func (c Config) Label() string
func (c Config) Missing() Missing
// … one accessor per option that a foreign mark could honour
```

Accessors rather than exported fields, so that the set can grow without
freezing the representation — the same reason `Theme` has `Tokens`. The
package's own geoms keep using `config` directly; `Config` wraps it.

Whether a third party can also *add* an option is a separate question. With
`Option` closed, a foreign constructor either takes two variadic lists
(`Foo(src, geomOpts, fooOpts)`) or `...any`. The cheapest opening is one
escape hatch — `geom.Extra(key string, v any) Option` and
`Config.Extra(key) (any, bool)` — and it is optional: v1 can ship without it
and add it later, because it is additive. Decide, and say which.

**2. Registries replace the switches.**

```go
// geom
func Register(m Mark, build func(Desc) (Geom, error))
// scale
func Register(k Kind, build func(Desc) (Scale, error))
func RegisterColor(k ColorKind, build func(ColorDesc) (ColorScale, error))
func RegisterSize(...)
// coord
func Register(t Type, build func(Desc) (Coord, error))
```

`FromDesc` consults the registry after its own switch, or the built-ins
register themselves in `init` and `FromDesc` is nothing but the lookup. Same
shape as `theme.Register` and `palette.RegisterRamp`, which already exist and
are already tested. Registration order must not affect output — ADR 0012
applies — so the registry is a map guarded by a mutex and nothing iterates it.

**3. `spec` decides what a registered mark carries.** See the `spec` table.

With those three, the sentence in §14 — "stable registration/extension model
for third-party geoms and backends" — is true, and the audit has no remaining
blocker.

## What this audit confirms is not a v1 question

These are the things the freeze does not have to wait for, because each lands
beside the frozen surface rather than through it.

| Item | Why it is additive | Where it lands |
|---|---|---|
| Animation and transitions | A `Live` method, an `ir.Partial` already there | v1.x |
| Geographic projections | A third `Coord` behind the same interface; ADR 0018 already argues it | v1.x |
| Relational layouts (sankey, treemap, chord) | New geoms; `data.Source` already returns an edge list | v1.x |
| 3D | Its own module | later |
| Contour, QQ, more stats | New pure functions in `stat`, new geoms that call them | v1.x |
| Brush, linked views | `Select EventKind`, `Event.Rows []int` | v1.x |
| Per-instance marker colour or size | An optional `ir.InstancedMarkers` a backend may implement | v1.x |
| A fourth column type, a null mask | Optional interfaces beside `data.Source` | v1.x |
| Pluggable facet strategies, pluggable stats | Not wanted yet; `Faceter` and "a geom that calls a stat" are the extension points that exist | when asked |
| GPU tier hardening | `backend/gg/gpu` stays `v0.x` | as GoGPU matures |

## Order of work to the tag

1. **ADR 0029, the extension model** — `geom.Config`/`Configure`, the three
   registries, the `spec` passthrough decision. This is the only item that
   changes what a third party can build.
2. **The cheap renames and moves** — `coord.Option` → `PolarOption`,
   `layout` → `internal/layout`, `mathtext.Symbols` → `RegisterSymbol`, the
   scale option-naming decision written into the package doc.
3. **The growth rules** — one sentence in each of `data.Source`, `scale.Scale`,
   `scale.ColorScale`, `scale.SizeScale`, `coord.Coord`, `geom.Geom`,
   `ir.Backend`, `ir.Target`, `render.Observer`, `mathtext.Typesetter`: *this
   interface does not gain methods; capabilities arrive as optional
   interfaces.* Plus the sentence in `stat`'s package doc that there is no
   `Stat` interface and why.
4. **`spec.Schema` to `/spec/v1`**, the additive-field policy in its comment,
   the README example corrected.
5. **CONCEPT §15** — which nested modules tag `v1.0.0` with the core
   (`backend/gg`, `backend/window`), which stay `v0.x` (`backend/gg/gpu`),
   and that `arrow` tracks its upstream. Mark §17 item 7 closed.
6. Tag.

Nothing on the list adds a chart type, and that is the point.
