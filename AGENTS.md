# Working in this repository

Notes for anyone — human or agent — making changes here. Read
[CONTRIBUTING.md](CONTRIBUTING.md) for the commands; this file is about the
constraints that are easy to break without noticing.

## The one rule

**The core module must never gain a dependency.** `go.mod` at the repository
root has no `require` block, and it stays that way. Anything that needs
`gogpu/gg`, `gogpu/gogpu`, `x/image`, Arrow, or anything else belongs in a
nested module — `backend/gg`, `backend/gg/gpu`, `backend/window` and `arrow` are
the four that exist. `syscall/js` is the standard library, which is why the
browser backend is in the core rather than beside them.

CI checks it:

```sh
go list -deps ./... | grep -v '^github.com/timzifer/refract' | grep '\.'
```

If that prints anything, the build fails. This is the promise the whole
positioning rests on — see [ADR 0001](docs/adr/0001-module-layout.md).

## Layer discipline

```
data, stat                                    →  rows in, rows out
geom, scale, facet, layout, render            →  produce IR
ir                                            →  the interface
interact                                      →  reads IR back
spec, a11y                                    →  write the model down
mathtext                                      →  places text, measures through IR
backend/svg, backend/pdf, backend/canvas,     →  consume IR
backend/gg, backend/window
```

- A geom must not import a backend, know about SVG, or measure text except
  through `ir.Backend.Measure`.
- `stat` knows about numbers and nothing else — no scales, no theme, no geoms.
  A reduction that needed one of those would be a geom in the wrong package.
- A backend must not import `geom`, `scale`, `theme` or `render`. That is why
  `Live.Bind` — which turns a wheel event into a zoom — is in the root package
  under a js build tag and not in `backend/canvas`: wiring input is not drawing.
- `render` is the only package that knows the drawing order of a chart.
- `spec` may read every model package and must never be read by one. A geom
  that knew about JSON would be a geom in the wrong package; `geom.Desc`,
  `scale.Desc` and `facet.Desc` are how a model type says what it is without
  knowing what will be done with the answer.
- `a11y` sits exactly where `spec` does, for the same reason: a geom that knew
  what a screen reader was would be a geom in the wrong package.
- `mathtext` neither draws nor knows about a chart. It is given a label, a font
  and a `Measure`, and it answers with positions — which is why it can be
  installed by wrapping the backend and reach every label at once.
- `interact` consumes IR and scales and produces neither. It wraps a backend
  rather than replacing one.

If a change needs to cross one of these lines, that is a signal the seam is in
the wrong place — say so rather than routing around it.

## Things that will bite

**Golden files are compared structurally, not byte for byte.** The SVG emitter
fixes attribute order and number formatting on purpose, so two runs on one
machine are identical — but arm64 and amd64 disagree in the last bit of a
float32, because Go contracts `a*b + c` into an FMA on one and not the other. So
`internal/svgdiff` compares everything but the numbers exactly and allows
coordinates a hundredth of a pixel. If output changes beyond that, the golden
tests fail; that is correct. Regenerate with `-update`, read the diff, and only
then commit. **Never widen a tolerance to make a failure go away.**

**Documentation figures are generated.** Everything in `docs/images/` comes from
`backend/gg/cmd/gallery`. Never hand-edit them. A test and a CI job both check
they match the code.

**`backend/gg` must not import `gg/gpu`.** Importing it activates the GPU tier,
pulls `wgpu` and `goffi` into the build, and makes CI need graphics hardware.
The CPU rasterizer is the supported path —
[ADR 0006](docs/adr/0006-gg-coupling-surface.md).

**gg is pinned exactly.** Upgrading it is a deliberate change with its own
commit, not a side effect of `go get -u`.

**Scales place a value with one explicit rounding, and it is not redundant.**
`scale.place` writes `rlo + float32(float32(t)*(rhi-rlo))`. The inner
conversion looks like a no-op — `t` is already being converted — and it is not:
it is what stops Go contracting the multiply and the add into a fused
multiply-add. The spec permits that contraction "possibly across statements",
arm64 takes it and amd64 does not, and the result is a coordinate one float32
ulp apart on the two.

An ulp is invisible in a chart, which is why the golden files tolerate one. It
is not invisible to a *decision*: `stat.LTTB` picks the row forming the largest
triangle, so two candidates a hair apart swap places and a whole vertex moves.
That is how this was found — a documentation figure that differed between
architectures by a chosen sample rather than by a bit. The same rounding is
therefore forced in LTTB's own area computation. Do not "simplify" either one
away; nothing in the toolchain will tell you that you have.

**Scales snap their endpoints.** `Map` returns the exact range bounds for the
exact domain bounds. Without that, a tick on the plot edge lands a float32 ulp
outside it and gets culled. There is a test; do not "simplify" it away. Every
scale does this, including the ones added in v0.2.

**There is one layout solver, and every chart goes through it.**
`layout.Compute` is `layout.Panels` over a one-by-one grid, and `render.Draw`
resolves a single-panel chart into a one-panel grid. That is deliberate — see
[ADR 0010](docs/adr/0010-panel-layout.md). Adding a second path for "the simple
case" reintroduces exactly the divergence this arrangement exists to prevent,
and the golden files would only cover one of them.

**Panels sharing an axis share the scale object.** So the device range is set
per panel, twice: once before the furniture pass and once before the data pass.
Dropping the second call leaves every panel but the last drawing its data where
the last panel's axis is. There is a test.

**PDF is refract's own emitter, not `gg-pdf`.** That library cannot draw
geometry — its path operations reach a stub in `gxpdf` — so the roadmap's plan
of routing PDF through gg's recording API would have produced pages with tick
labels and nothing else. `gg/recording` therefore stays out of `backend/gg`,
and [ADR 0006](docs/adr/0006-gg-coupling-surface.md)'s import rule is unchanged.
See [ADR 0009](docs/adr/0009-pdf-backend.md) for the evidence.

**The PDF backend measures with the font it draws with.** `internal/fontmetrics`
carries Helvetica's advance table, and PDF's base-14 Helvetica is that font.
Every other backend approximates; this one does not. Do not "improve" it by
measuring with something else.

**Colour ramps interpolate in linear light.** `palette.Lerp` decodes sRGB,
blends, and re-encodes. Averaging the encoded bytes instead is about 20% too
dark at the midpoint, which shows up as a band across a gradient. gg composites
in linear space for the same reason. There is a test that pins the midpoint of
black-to-white at 188, not 128.

**A geom must not assume a scale accepts every finite value.** A log scale has
no position for zero and returns NaN from `Map`. Geoms ask `scale.Definite`
through `geom.defined` and treat such a row as missing, so a NaN coordinate
never reaches a backend — [ADR 0008](docs/adr/0008-categorical-axes.md). Adding
a geom means computing `scratch.plottable` once and using it, not re-testing
`finite` per traversal: the traversals have to agree about where the holes are.

**Per-mark colour batches; it does not widen the IR.** `geom.groupByColor`
emits one drawing call per distinct colour. Adding a per-vertex colour channel
to `ir.Backend` is the thing that record exists to refuse —
[ADR 0007](docs/adr/0007-per-mark-colour.md).

**Decimation happens in `Build`, never in `Train`.** A geom trains on every row
and reduces only when it draws, so an axis reports the data rather than the
subset that survived — [ADR 0011](docs/adr/0011-decimation.md). Reducing in
`Train`, or in the data layer, would make a chart's axis depend on how wide the
chart is, and there is a test that catches it.

**The reduction runs on device coordinates.** A pixel column is the unit the
whole exercise is about, and on a log axis equal steps in data are not equal
steps on screen. `stat.LTTB` and `stat.MinMax` are generic over `float32` and
`float64` precisely so the device-space path costs no conversion.

**Everything sized by the data comes from a pool.** `geom.scratch` holds the
per-Build buffers and is returned at the end of the call, which is what makes a
redrawn chart allocate the same handful of times over a million rows as over a
thousand. Two consequences: a slice handed to a backend is **lent**, not given —
a backend that keeps one will show the next frame's data, which is why
`ir.Recorder` and `internal/irtest.Recorder` both copy — and a new allocation on
the data path will fail `TestARenderDoesNotAllocatePerPoint` rather than merely
slow things down. Find it with `-memprofile` and
`pprof -sample_index=alloc_objects`, do not widen the gate. The gate is behind
`//go:build !race` because the race detector allocates on refract's behalf, so
run it without `-race` when you are checking it — and it is checked a second
way, from real benchmark output, by `.github/scripts/allocgate.awk` in the
`Benchmarks and the allocation gate` CI job.

**The benchmark gate pins allocations, never times.** A shared CI runner cannot
measure a nanosecond usefully, and a gate that flakes is a gate people learn to
ignore. If you add a timing assertion there it will go red on an unlucky
Tuesday and be deleted, taking the allocation checks with it.

**A variadic interface method called per row allocates per row.** `Train(v)` on
a `scale.Scale` cannot be proved non-escaping by the compiler, so the argument
slice goes to the heap — a million times, on a million-row column. `Train` is
documented to ignore NaN and infinities, so the whole column goes in one call.
The same shape will appear again; look for it.

**A parallel render must be byte-identical to a serial one.** Panels record into
an `ir.Recorder` and replay **in panel order**, never in completion order —
[ADR 0012](docs/adr/0012-parallel-panels.md). That is what lets one set of
golden files cover both paths, and there is a test asserting the two produce
identical bytes. A change that makes the order depend on scheduling silently
halves the coverage of every golden file in the repository.

**Panels on separate goroutines need `scale.Snapshotter`, not `scale.Cloner`.**
Snapshot copies a scale *including* its trained domain; Clone deliberately does
not, because it is for a free facet axis. They are opposites and neither is a
substitute for the other. A scale implementing neither still works — the chart
is drawn serially.

**A density-raster figure embeds a PNG inside its SVG, and that payload is
compared as pixels.** `docs/images/density.svg` carries a base64 deflate stream,
and deflate output is the standard library's business rather than refract's —
the same chart on two Go releases produces two different streams, which is
exactly how this first went red. So the gallery lifts embedded payloads out of
both documents and compares them with the same per-channel tolerance the PNG
half already uses; the vector half, including the `<image>` element's own
position and size, is still compared exactly. See
`backend/gg/cmd/gallery/embedded.go`, and note this is not a widened tolerance
but the same one applied to pixels rather than to an encoding of them. It is
also why there is no golden SVG for a density chart in `testdata/golden`:
pinning `compress/flate` is not a test of refract.

**Do not compare device coordinates with `==` in a test.** `scale.place` now
removes the fusion that a scale itself introduced, but everything downstream of
it — a Catmull-Rom control point, a bar's half-width, a boxplot quantile — is
still ordinary float arithmetic the compiler may contract. `geom`'s annotation
tests carry `sameRect`/`samePoint` at `svgdiff.DefaultTolerance` for this, and
`TestTheCoordinateSlackIsTheRightWidth` pins that slack from both sides. Exact
comparisons were green on amd64 for three milestones and red on every macOS
run.

**A geom that holds data implements `geom.Faceter`; one that does not, must
not.** Faceting splits the layers that have rows and replicates the ones that
do not, which is why an `HLine` appears on every panel. Adding `Source`/`Subset`
to an annotation would make a threshold vanish from every panel but the one its
value happens to fall in.

**A watched render must draw exactly what an unwatched one draws.**
`interact.Index.Watch` wraps a backend and indexes what it sees; every method
forwards first and indexes second. There is a test comparing the two traces
call for call, and `TestAWatchedRenderDrawsWhatAnUnwatchedOneDoes` compares the
SVG bytes. A probe that changed anything would silently halve the coverage of
every golden file, the same way a scheduling-dependent panel order would.

**A watched render is serial, on purpose.** `render.Chart.Observer` forces the
serial path in `concurrent`. The observer is told which layer is drawing so a
caller can attribute the calls that follow; two panels drawing at once have no
order to be told in. Do not "optimise" this by giving each panel its own
observer — the index would then depend on scheduling, and so would every
tooltip.

**Hit-testing indexes one mark per subpath, not one per call.** A layer draws
all its bars in a single path, because `geom.groupByColor` batches by colour. A
mark per call would make the row of bars one shape, so pointing at the fourth
bar would report whichever corner of whichever bar happened to be nearest.
There is a test.

**A geom reports its rows separately from what it draws, and that is the
whole point.** `geom.Rows` takes the positions a row landed at, not the points
of a drawing call — a smoothed line is a Bézier path whose control points are
not measurements, a staircase draws two points per row, a bar is four corners
around one value. Attributing rows to a call's points would attribute them to
whichever encoding the geom happened to use, and would be wrong for three of
the six geoms that have rows. There is a test per geom.

**Row reporting is gated on `Frame.Rows != nil` at every step.** `acquire(f)`
records it on the scratch as `wantRows`, and `rowsOf`, `sourceRows` and the
interpolated series' row list all check it. An ordinary render must keep
costing exactly what it did — `BenchmarkFrame1k` is still 76 allocations, and
`BenchmarkWatchedFrame` against `BenchmarkWatchedFrameRows` is pinned flat.

**A row is reported in the caller's table, not in the cut refract made.**
Faceting cuts each layer with `data.Rows`, and a row number relative to that
cut is a row number in a table nobody holds. `series.origin` comes from
`data.Origins` and `series.rowAt` resolves through it. A geom that collects its
own element list — a scatter, a bar, both for per-mark colour — must report
through `scratch.sourceRows` rather than handing over the element numbers: the
two lists look identical and mean different things, and a faceted chart is
where that shows.

**A pinned scale domain skips nicing, and that is not an oversight.**
`scale.Zoomer.SetDomain` sets `pinned` as well as `fixed`, and `effective()`
returns the pinned domain before nicing or zero-forcing get a chance. An axis
that snapped to round numbers after every wheel notch would not follow the
pointer, and on a log axis nicing rounds the view out to whole decades. `fixed`
alone stops *training*; `pinned` also stops *framing*.

**`data.Stream` is deliberately not a `data.Source`.** A Source is read column
by column over several calls, and a table appended to between two of them
disagrees with itself. `Snapshot` freezes and swaps; `Source()` reads whatever
the last snapshot froze. Making Stream implement Source would compile, and
would produce a chart with more timestamps than values under load. The two
buffers are reused, so a snapshot is valid only until the next one — that is
the price of the frame costing no allocations, and it is why Snapshot belongs
between frames rather than during one.

**Unwrap a ring buffer before rewriting it, not while.** `Stream.compact`
resolves every slot number first and rewrites the columns afterwards, and takes
the *old* window as a parameter. `at` answers from the ring's current length,
so rewriting the first column changes the answer for the second — a bug that
only appears on the second column, and only after the ring has wrapped. There
is a test.

**Damage is per drawing call, and reports `ok == false` rather than guessing.**
`ir.Damage` compares two recordings call for call; a different call count, a
different kind, or a moved transform means the chart's *structure* changed and
the answer is a full repaint. Do not make it clever about realigning: a list of
rectangles that describes half a frame is worse than repainting the frame.

**`ir.Partial.Damage(nil)` means the whole frame, and an empty list never
arrives.** A frame identical to the last one is not painted at all rather than
painted with no damage, which is why the nil case is unambiguous.

**A Stroke cannot be compared with `==`.** It holds a dash slice. `ir` carries
`Stroke.same`, `Fill.same` and `MarkerStyle.same` for this; reach for those
rather than adding a comparison that compiles today because the field you would
have missed happens not to be there yet.

**The JSON spec writes what a mark uses, not what it was given.** A geom
accepts every option and ignores the ones it has no use for, which is what
keeps the option set one namespace; a *document* listing a line's whisker
extent would read as though that meant something. `spec.writeMarkProps` decides
per mark. Adding an option means adding it there too, and the round-trip test
in `spec/` is what catches forgetting.

**`geom.Desc` and `scale.Desc` are complete, not partial.** Every field with a
non-zero default — `BarWidth`, `Whisker`, `Outliers`, `Extend`, `Opacity` —
carries the value the layer is actually using, so nothing downstream has to
know what the defaults are. `Opacity` is `-1` when unset, matching `config`.

**The canvas backend builds one path string per drawing call.** Crossing the
WebAssembly boundary is what costs in a browser: a five-hundred-point line is
one `Path2D` and one `stroke`, not five hundred `lineTo` calls. There is a test
that fails if a chart over five hundred rows starts making more than a hundred
context calls, and one that fails if `beginPath` reappears.

**`backend/canvas` is empty except on js/wasm, and that is what the doc.go is
for.** Every implementation file carries `//go:build js && wasm`; `doc.go`
carries no constraint, so `go build ./...` on a server has a package to build
rather than an error. Its tests run under node in CI, against a recording
context — node has no canvas, and a test that needed one would be a test nobody
could run.

**`backend/gg` still must not import `gg/gpu`, and the GPU tier is a module
below it.** `backend/gg/gpu` is nested *inside* `backend/gg` so that the raster
backend's own module graph stays gg, `x/image` and the core; importing the tier
is the whole opt-in — [ADR 0022](docs/adr/0022-gpu-tier.md). Moving the blank
import up one directory would put `wgpu`, `naga` and `goffi` into every program
that renders a PNG, and would cost the module its js/wasm target.

**gogpu is pinned exactly, and its pin is tied to gg's.** `backend/window`
requires gogpu v0.52.1 because that is the release whose `gputypes` and
`gpucontext` resolve to the versions gg v0.52.5 compiles against. A newer gogpu
pulls a `gputypes` that gg at this pin does not build with, and the failure is a
wall of "too many arguments" inside gg's internals rather than anything naming a
version. Upgrade the two together or not at all.

**A backend wrapper hides the optional interfaces of what it wraps.** `Damage`,
`Resize` and `Describe` are all reached by type assertion, so a wrapper that
does not declare them silently drops them. `ir.Recorder` and `interact`'s probe
both forward `Describe`; `render.Draw` keeps the *unwrapped* backend for the
semantics question, because it wraps the backend to install a typesetter. A
`<title>` that quietly disappears from interactive charts only is what this
costs when it is forgotten, and there is a test.

**A description costs a pass over the data, so a render must not take one.**
`Plot.Describe` is a method that does work rather than an option that sets a
flag: it reads every plotted column. The title alone is free and is always
written; the long description is written when someone asked for one. Wiring the
description into `chart()` so that every frame recomputes it would put a data
pass in the frame budget, and `TestARenderDoesNotAllocatePerPoint` would not
notice — it counts allocations, not passes.

**Redundant encoding is a default, not an override.** `theme.Redundant` fills in
a dash and a marker ladder, and `config.dashFor`/`markerFor` use them only when
the layer set neither `geom.Dash` nor `geom.Shape`. That is why `config` carries
`markerSet` beside `dashSet` and why `geom.Desc` carries `MarkerSet`: a circle
is both the zero value and a shape somebody may have asked for, and without the
flag a round trip through the spec turns an unset shape into a pinned one.

**A time domain is nanoseconds since the scale's origin, not since 1970.**
`scale.Origin` moves it, `scale.ValueOf`/`InstantOf` convert across it, and
`geom.column` uses them — so a time column is read in the axis's own space.
The default origin is the Unix epoch, which is exactly `scale.Nanos`, so nothing
changes for a scale that never asked. What does change is that `scale.Nanos` is
the wrong conversion to reach for near a rebased axis: the domain values, the
`Invert` results and the `Desc` bounds are all measured from the origin, and the
JSON spec writes it out for that reason.

**The raster backend draws italic now, and `WithFont` takes three fonts.**
`ir.FontRef.Italic` has existed since v0.1 and `backend/gg` ignored it, which
was invisible until a typesetter started producing italic runs — at which point
the PNG and the SVG of the same chart would have disagreed in the
documentation. The default set parses `goitalic` alongside `goregular` and
`gobold`; a caller supplying their own passes nil for a style they do not have,
and gets the regular face for it.

**A typeset label is measured by laying it out.** `render`'s `mathBackend`
answers `Measure` from the typesetter and `Text` from the same layout, so the
margin a title is given is the width the title turns out to have. Measuring the
markup and drawing the notation would leave a fraction hanging out of the
canvas, and nothing would fail.

**A resized frame is not comparable with the last one.** `Live.Resize` clears
`drawn`, because every coordinate moved and there is no damage to compute. It
also keeps whatever the scales were zoomed to, on purpose: a reader who dragged
a view into place has not asked to leave it.

**A position adjustment is derived in `Train` and drawn in `Build`, and both
halves are forced.** `render.Draw` trains every layer before it measures
anything, because tick labels need a domain — so a stacked bar's Y scale has to
be trained on the *totals*, or the tallest stack runs off the top of an axis
that describes rows nobody can see. `geom.groups.train` therefore computes the
per-row `(lo, hi)` pair, and `Build` only maps it through the scales. This is
the same boundary ADR 0011 draws for decimation, drawn for the same reason: what
a chart's axis says must not depend on how wide the chart is. See
[ADR 0019](docs/adr/0019-position-adjustments.md).

**A stacked layer's holes are decided once, and both traversals read that
answer.** `groups.ok` is computed in `Train` and an unplottable row gets `NaN`
bounds, so the cumulative sum and the drawing traversal agree about where the
holes are. A NaN the sum skipped but the draw did not would shift every segment
above it — there is a test, and it compares a table with a hole against the same
table with the row removed.

**A grouped layer's buffers live on the layer, not in the frame's pool.** The
group index, the row lists and the derived bounds are refilled by every `Train`
out of memory the geom keeps: `Train` runs as often as `Build` does, and a chart
redrawn over a live table would otherwise allocate its group index per frame.
`TestAStackedLayerDoesNotAllocatePerPoint` is what holds that. Note that the
rows of each group are listed once, in `groups.split`, rather than found by
scanning the table per group — the scan is what makes a chart with many series
quadratic.

**There is exactly one scratch per `Build`, and a helper takes the caller's
rather than acquiring its own.** Two scratches held at once put two objects with
*disjoint* buffers into one pool; whichever comes back in the other's role next
frame has to grow the other's buffers, which is a per-row allocation on the path
whose whole point is not having one. `geom.eachGroup` therefore takes a
`*scratch` parameter. This shipped broken for exactly one commit and cost a
grouped frame 127 kB and seven allocations it did not need; nothing failed
except the allocation gate, which is what the gate is for.

**A benchmark's allocation count is not stable enough to compare across sizes
when a pool miss is in play.** `sync.Pool` is emptied by every collection, and
how many collections land inside `-benchtime=10x` depends on the garbage the
*other* benchmarks in that process left behind — so `BenchmarkStacked100k`
comes out at 91, 98 or 106 for the same code. The flat comparison therefore
lives in `TestAStackedLayerDoesNotAllocatePerPoint`, which averages twenty runs
in a process running nothing else, and `.github/scripts/allocgate.awk` pins a
*budget* for that benchmark instead. Reach for `atMost` rather than a wider
`flat` slack the next time a pair will not sit still: a gate that flakes is a
gate people learn to ignore.

**Group order is order of first appearance, and `geom.Order` is the only thing
that changes it.** Map iteration order is not an order; ADR 0012 requires a
parallel render to be byte-identical to a serial one, and `scale.Qualitative`,
`geom.groupByColor` and `boxplot.summarise` all already establish the
convention.

**A grouped `Bar` and `Area` stack by default; nothing else does.** That is
`config.stackFor`, and it is why `geom.Desc` carries `Stack` *and* `StackSet` —
`NoStack` is both the zero value and an adjustment somebody may have asked for,
exactly as `Dash`/`DashSet` and `Marker`/`MarkerSet` already are. Without the
flag, a round trip through the spec turns a grouped bar's default into a pinned
"do not stack", and the chart silently changes.

**Which guide a layer contributes follows from the kind of colour scale it was
handed.** `scale.Qualitative` is a `ColorScale` — it rides that interface the way
`scale.Categorical` rides `Scale` — so `geom.ColorBy` takes either kind, and
`config.colorGuide` returns false for a discrete one while `config.legends`
returns an entry per category. A colourbar over eight categories would be a ramp
through colours nothing is painted with.

**A layer that implements `geom.Legender` is not asked for `Legend` as well.**
`geom.Legends` prefers the list; `geom.LegendsOr` is the fallback written once,
and a geom that answers `Legends` without it silently loses its single entry.
There is a test — it was the first thing that broke when the interface landed.

**A rect and a region are both `("rect", "")` in the document, and the encoding
is what tells them apart.** `spec.geomMark` is passed the layer's encoding for
that one reason: a rect with a *field* is data, a rect with a *datum* is an
annotation. Dropping the parameter compiles and turns every heatmap into a
four-literal annotation.

**Responsive scaling multiplies lengths and must not mutate a shared theme.**
`theme.Scaled` copies every dash slice it touches rather than scaling in place —
`theme.Light` is a package variable, and scaling its grid dash would scale it
for every chart in the process, once per render. There is a reason that is a
sentence in the code as well as here.

## Open questions

[CONCEPT.md §17](CONCEPT.md#17-open-decisions) lists the design decisions that
were genuinely open. Seven are closed and recorded in [docs/adr](docs/adr) —
§17.3, Vega-Lite spec fidelity, was settled in v0.5 by
[ADR 0014](docs/adr/0014-json-spec.md). One remains open on purpose: the
third-party extension API (§17.7), which belongs to v1.0. Do not settle it in
passing; it needs its own ADR.

Optional interfaces are how this codebase extends a type without breaking
everyone who implements it: `scale.Definite`, `scale.Categorical`,
`scale.Band`, `scale.Cloner`, `scale.Snapshotter`, `scale.Zoomer`,
`scale.Describer`, `scale.ColorDescriber`, `scale.DiscreteColorScale`,
`scale.Temporal`, `geom.Faceter`, `geom.Guided`, `geom.Legender`,
`geom.Describer`, `ir.Partial`, `ir.Semantics`, `ir.Resizer`,
`mathtext.Plainer`. Reach for one before adding a method to `Scale`, `Geom` or
`Backend`. `geom.Legender` is the newest and the argument is worth keeping in
view: a pie, a stack and a waffle contribute N legend entries from one layer,
and adding a second method to `Geom` for them would have broken every
implementation and spent the v1.0 freeze before the evidence for it existed.

`Cloner`, `Snapshotter` and `Zoomer` are three different things and none
substitutes for another: Clone hands back an *untrained* copy for a free facet
axis, Snapshot an *exact* copy for another goroutine, and SetDomain changes the
scale in place for a pan or a zoom.

## Scope

The roadmap in [CONCEPT.md §14](CONCEPT.md#14-roadmap--milestones) is what this
project is doing and in what order. Everything through v0.7 has shipped; next is
`coord/` in v0.8 — which is why the groups and the adjustments landed first, a
pie being a stacked bar with θ from the Y axis — then the stats in v0.9, and
then the extension API, the docs and a public benchmark suite for v1.0. Adding a
stub for one of those is not progress towards it — the seams exist, that is
enough.

Things v0.7 deliberately did not do. A grouped **line, step and scatter do not
stack**: two series drawn over one another are two readings, and adding them
would invent a third nobody measured. Stacking accumulates in group order, so a
stack of mixed signs runs each segment from where the last one ended rather than
splitting into a positive and a negative half. `geom.WidthBy` gives a bar its
width from a column and does **not** reposition the slots — a marimekko is one
layer whose X column already holds each column's centre, and unequal slots that
label themselves are an axis question rather than an adjustment one. A group
index is per layer, so a facet whose panels hold different groups needs an
explicit `scale.Qualitative` for a series to keep its colour across panels. And
a group column that is numeric or temporal is formatted into a label per row,
which is a cost of naming a category with a number rather than of grouping: the
allocation gate uses a text column, which is what a series column is.

Things v0.6 deliberately did not do. The GPU tier is opt-in beta and stays that
way past v1.0 — for server-side stills the CPU rasterizer and the vector
emitters are the supported path. The window is compiled by CI and never opened
by it, because a runner has no display; what is tested is everything that is not
the window, which is the same hole `backend/canvas` has about a browser. The
built-in typesetter is a deliberately small subset: no matrices, no growing
delimiters, no document-class macros, and a label needing those wants an engine
plugged into `mathtext.Typesetter`. Accessibility stops at the document: no
per-mark `<title>`, no tab order through the marks, no reduced-motion or
contrast handling — a chart with ten thousand points has no useful reading as
ten thousand elements, and the data table is the better answer to the same
question. And a description is a snapshot: data that changes afterwards leaves
it stale, which is why `Describe` is a call rather than a flag.

Things v0.5 deliberately did not do. There was no native window and no GPU tier;
both landed in v0.6 and both needed GoGPU packages that milestone did not touch.
A hit
reports data values rather than a row number, because carrying row identity
through decimation is bookkeeping the design avoids. Damage is per drawing
call, so moving one point of a line repaints the line's box. And the browser
path is canvas 2D, not WebGPU — gg has no `syscall/js` in it at the pinned
version, so there was no WebGPU path to take
([ADR 0017](docs/adr/0017-browser-backend.md)).

Things v0.4 deliberately did not do, in case they look like oversights.
`stat` carries the decimation family and nothing else: smoothing, regression and
hexbin are stats rather than big-data machinery, and they belong with the geoms
that would draw them. There is no `StreamSource` and no snapshot/swap, because
the interesting half of streaming is damage-aware repaint and that needs the
interactive backends. Training the scales and splitting a facet's rows are still
serial — only the data pass is parallel — and making them parallel is a
different decision with a different shape. And the Arrow adapter does not handle
`float16`, decimals or extension types; nothing that plots produces them yet,
and an untested conversion is worse than an absent one.

Two things v0.3 deliberately did not do, still true. A PDF is one page with no
embedded font: text outside WinAnsi becomes `?`, and fixing that means embedding
a font. And a colourbar is vertical, in the guide column; a horizontal one under
the plot is a layout question, not a drawing one.
