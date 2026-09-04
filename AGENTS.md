# Working in this repository

Notes for anyone — human or agent — making changes here. Read
[CONTRIBUTING.md](CONTRIBUTING.md) for the commands; this file is about the
constraints that are easy to break without noticing.

## The one rule

**The core module must never gain a dependency.** `go.mod` at the repository
root has no `require` block, and it stays that way. Anything that needs
`gogpu/gg`, `x/image`, Arrow, or anything else belongs in a nested module —
`backend/gg` and `arrow` are the two that exist.

CI checks it:

```sh
go list -deps ./... | grep -v '^github.com/timzifer/refract' | grep '\.'
```

If that prints anything, the build fails. This is the promise the whole
positioning rests on — see [ADR 0001](docs/adr/0001-module-layout.md).

## Layer discipline

```
data, stat                           →  rows in, rows out
geom, scale, facet, layout, render   →  produce IR
ir                                   →  the interface
backend/svg, backend/pdf, backend/gg →  consume IR
```

- A geom must not import a backend, know about SVG, or measure text except
  through `ir.Backend.Measure`.
- `stat` knows about numbers and nothing else — no scales, no theme, no geoms.
  A reduction that needed one of those would be a geom in the wrong package.
- A backend must not import `geom`, `scale`, `theme` or `render`.
- `render` is the only package that knows the drawing order of a chart.

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

**A density-raster figure embeds a PNG inside its SVG.** `docs/images/density.svg`
therefore contains a base64 deflate stream, and `svgdiff` compares non-numeric
tokens exactly. If the standard library's compressor ever changes its output,
that figure will fail `-check`. Regenerate it; do not widen a tolerance. That
is also why there is no golden SVG for a density chart in `testdata/golden` —
pinning the standard library's deflate output is not a test of refract.

**A geom that holds data implements `geom.Faceter`; one that does not, must
not.** Faceting splits the layers that have rows and replicates the ones that
do not, which is why an `HLine` appears on every panel. Adding `Source`/`Subset`
to an annotation would make a threshold vanish from every panel but the one its
value happens to fall in.

## Open questions

[CONCEPT.md §17](CONCEPT.md#17-open-decisions) lists the design decisions that
were genuinely open. Six are closed and recorded in [docs/adr](docs/adr). Two
remain open on purpose — Vega-Lite spec fidelity (§17.3) and the third-party
extension API (§17.7) — because they belong to milestones that have not started.
Do not settle them in passing; they need their own ADR.

Optional interfaces are how this codebase extends a type without breaking
everyone who implements it: `scale.Definite`, `scale.Categorical`,
`scale.Band`, `scale.Cloner`, `scale.Snapshotter`, `geom.Faceter`,
`geom.Guided`. Reach for one before adding a method to `Scale` or `Geom`.

## Scope

The roadmap in [CONCEPT.md §14](CONCEPT.md#14-roadmap--milestones) is what this
project is doing and in what order. Through v0.4 the project deliberately omits
the JSON spec, interactivity, streaming, GPU and the browser. Adding a stub for
one of them is not progress towards it — the seams exist, that is enough.

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
