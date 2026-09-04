# Working in this repository

Notes for anyone — human or agent — making changes here. Read
[CONTRIBUTING.md](CONTRIBUTING.md) for the commands; this file is about the
constraints that are easy to break without noticing.

## The one rule

**The core module must never gain a dependency.** `go.mod` at the repository
root has no `require` block, and it stays that way. Anything that needs
`gogpu/gg`, `x/image`, Arrow, or anything else belongs in a nested module.

CI checks it:

```sh
go list -deps ./... | grep -v '^github.com/timzifer/refract' | grep '\.'
```

If that prints anything, the build fails. This is the promise the whole
positioning rests on — see [ADR 0001](docs/adr/0001-module-layout.md).

## Layer discipline

```
geom, scale, layout, render   →  produce IR
ir                            →  the interface
backend/svg, backend/gg       →  consume IR
```

- A geom must not import a backend, know about SVG, or measure text except
  through `ir.Backend.Measure`.
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

**Colour ramps interpolate in linear light.** `palette.Lerp` decodes sRGB,
blends, and re-encodes. Averaging the encoded bytes instead is about 20% too
dark at the midpoint, which shows up as a band across a gradient. gg composites
in linear space for the same reason. There is a test that pins the midpoint of
black-to-white at 188, not 128.

**A geom must not assume a scale accepts every finite value.** A log scale has
no position for zero and returns NaN from `Map`. Geoms ask `scale.Definite`
through `geom.defined` and treat such a row as missing, so a NaN coordinate
never reaches a backend — [ADR 0008](docs/adr/0008-categorical-axes.md). Adding
a geom means computing `series.plottable` once and using it, not re-testing
`finite` per traversal: the traversals have to agree about where the holes are.

**Per-mark colour batches; it does not widen the IR.** `geom.groupByColor`
emits one drawing call per distinct colour. Adding a per-vertex colour channel
to `ir.Backend` is the thing that record exists to refuse —
[ADR 0007](docs/adr/0007-per-mark-colour.md).

## Open questions

[CONCEPT.md §17](CONCEPT.md#17-open-decisions) lists the design decisions that
were genuinely open. Six are closed and recorded in [docs/adr](docs/adr). Two
remain open on purpose — Vega-Lite spec fidelity (§17.3) and the third-party
extension API (§17.7) — because they belong to milestones that have not started.
Do not settle them in passing; they need their own ADR.

## Scope

The roadmap in [CONCEPT.md §14](CONCEPT.md#14-roadmap--milestones) is what this
project is doing and in what order. Through v0.2 the project deliberately omits
faceting, constraint layout, PDF, colourbars and other guides, decimation,
Arrow, the JSON spec, interactivity and GPU. Adding a stub for one of them is
not progress towards it — the seams exist, that is enough.

The colourbar is the one worth naming, because its absence is visible: a layer
using `geom.ColorBy` contributes **no** legend entry. That is deliberate. A
single swatch cannot represent a continuous ramp, and saying nothing is better
than saying something false. Guides are v0.3.
