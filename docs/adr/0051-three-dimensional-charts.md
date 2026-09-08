# 0051 — A third axis widens the seams that count scales, and the IR stays two-dimensional

**Status:** Planned · **Date:** 2026-09-08 · **Implementation:** not started

## Context

[ADR 0050](0050-depth-without-a-third-axis.md) split "3D" into three features
and took the one that encodes nothing. This is the second: **x, y and z are all
data**, and the chart is a surface, a 3D scatter, a 3D line or a field of 3D
bars.

The obstacle is not the arithmetic. Projecting a point with a 4×4 matrix is
twenty lines, and `scale.Scale` already maps a value into an interval without
caring what the interval means — the property [ADR 0018](0018-coordinate-systems.md)
was built on.

**Nor is the obstacle the struct.** `geom.Frame` can take a `Z scale.Scale`,
and saying otherwise would be inventing a constraint this repository does not
have: `render`'s own package doc says "[Chart] and [Panel] grow by gaining
fields, and a zero field always means what it meant", ADR 0018 widened `Frame`
itself, and a keyed field is additive under Go semver. `Frame.Z` compiles, and
every chart drawn today keeps drawing.

**The obstacle is that the field is one line and four parallel paths.**

**Nothing trains it.** `Geom.Train(x, y scale.Scale) error` is an interface
implemented outside this module, so under v1's growth rule it never gains a
parameter — and a z domain has to exist before layout, because layout needs
tick labels and tick labels need a domain. Training z is therefore a second training path beside
the frozen one, reached through an optional interface and called from a
`render` that knows when to look for it.

**Nothing indexes it.** `render.Observer.Panel(i, area, x, y, cd)` is the same
kind of interface for the same reason. `LayerAxes` shows how a scale is added
beside it without touching it — and shows the cost: another optional
interface, another branch in the draw path, another thing an index has to
implement to be correct.

**Nothing orders it, and this one is not a signature.** `drawPanel` walks
`p.Layers` and each layer's `Build` streams straight into the backend
([render/render.go:1060](../../render/render.go)), so **a layer is a paint
unit**. A projected scene has no paint unit smaller than the panel: a point of
a scatter can be in front of one part of a surface and behind another, so
correct occlusion is a single depth order over the primitives of *every* layer
at once. Producing that inside `render` needs one of two things, and both are
already refused — a depth key on every drawing call, which is the identity
channel [ADR 0007](0007-per-mark-colour.md) and [ADR 0015](0015-hit-testing.md)
exist to keep out of the IR; or recording each layer and merging the recordings
by depth, which leaves `render` holding two drawing orders and is precisely
what [ADR 0010](0010-panel-layout.md) exists to prevent.

**Nothing warns about it.** A `geom.Line` handed a `Frame` with a `Z` ignores
it and draws a flat line inside a projected box — correct by its own lights,
wrong by the chart's, and silent either way. Every one of the twenty
data-bearing marks would have to be taught to refuse a dimension it has never
heard of.

So the question was never whether a struct can grow. It is where four parallel
paths live, and three answers were available.

**Bolt it on additively.** `Frame.Z`, an optional `Trainer3`, an optional
`Panel3`, a depth-merge stage in `render`. Nothing breaks, and every one of
those seams carries two paths forever — one of which exists only because of the
order features arrived in.

**Put it in a nested module at `v0.x`.** Which is what
[the v1 audit](../v1-api-audit.md) wrote in the one cell it gave the subject,
and what the first draft of this record decided. It keeps the v1 promise intact
by keeping 3D outside the thing the promise covers.

**Take the break.** Widen the seams that are the wrong shape, tag `v2.0.0`, and
say so in the release notes.

The third is chosen, and the reason it can be chosen is a fact about this
library rather than about the design: it has few enough users that a migration
is a compiler pass. That fact has an expiry date, which is why the record is
written now.

## Decision

**3D is `refract/three`, a package of the core module, and the three seams that
take positional scale arguments are widened to carry a third. That is a
breaking change, it is spent deliberately, and it is the last one those seams
need.**

### Why the core, and what the break costs

The first draft of this record put `three` in a nested module at `v0.x`, so
that a first 3D API could be wrong without breaking the library's v1 promise.
That was architecture chosen to avoid a version number, and it was the wrong
trade for a library at this age.

**A v2 with no subscribers is a v1.** refract tagged `v1.0.0` and is at
`v1.7.0`; the freeze was the right discipline for getting the model right and
it has done its work. What it is not is a reason to bolt four optional
interfaces onto three frozen signatures and call the result a design. The break
is cheap exactly once — now, while the cost is a line in a changelog rather
than a migration for other people's code — and a library that spends it on the
thing that shapes its next decade is spending it well.

So the decision is the honest one: **take the break, write it down, and fix all
three seams in the same release**, because a major version is paid once whether
it carries one change or three.

#### What actually breaks

Nothing in 3D *requires* a break. `Frame.Z` is additive, and z could reach a
geom through an optional `Trainer3`, an optional `Panel3` on the observer and
an optional `Frame3` on the coord — the pattern this repository already uses
eight times. That would work, and it would leave every one of those seams with
two paths through it forever, one of which exists only because of when a
feature arrived. The break does not buy 3D. **It buys 3D having one path.**

The three seams are the ones that take scales *positionally*, which is why they
are the three that a third dimension breaks:

| Seam | v1 | v2 |
|---|---|---|
| `geom.Geom` | `Train(x, y scale.Scale) error` | `Train(t geom.Training) error` |
| `render.Observer` | `Panel(i int, area ir.Rect, x, y scale.Scale, cd coord.Coord)` | `Panel(p render.PanelInfo)` |
| `coord.Coord` | `Frame(area ir.Rect, x, y scale.Scale) Coord` | `Frame(f coord.Framing) Coord` |

Each becomes one growable struct, which puts them under the half of the growth
rule that has never been the problem: *"a struct with exported fields gains
fields and never loses one, and its zero value keeps its meaning."* A fourth
dimension, a second radial scale, a time axis — none of them breaks these
seams again. **The migration is mechanical**: a method's two parameters become
two field reads, and the compiler finds every site.

The rest of the cost is Go's, and it is the part worth stating plainly:

- the import path becomes `github.com/timzifer/refract/v2`, in every file of
  every caller and in each nested module's `require`;
- `$schema` moves with the major version, which CONCEPT §15 already says it
  does;
- `backend/gg`, `backend/window` and `arrow/v18` re-require the core and tag
  again. Their own APIs do not change: `ir.Backend` is untouched by this
  record, so a third-party backend recompiles and is done.

#### What is written down, and where

CONCEPT §15 currently reads as though the v1 surface is permanent. When this
record is implemented it gains the clause that makes the actual policy legible,
and the wording is decided here rather than left to the commit that does it:

> **A major version is a tool, not a failure.** Within a major version the
> growth rule holds absolutely. Between them, a seam whose *shape* is wrong is
> corrected rather than papered over with a parallel path — and while refract
> has few enough users that a migration is a compiler pass, that correction is
> preferred to carrying the mistake. `v2.0.0` widens `Geom.Train`,
> `Observer.Panel` and `Coord.Frame` from positional scale arguments to
> growable parameter structs, so that adding a dimension is additive from then
> on ([ADR 0051](docs/adr/0051-three-dimensional-charts.md)).

The deprecation cycle CONCEPT promises for the removals is not skipped so much
as unnecessary — it exists for callers, and the release notes say precisely who
had to change what. If that stops being true before this lands, this paragraph
is what has to be revisited first.

### The package still has to be its own package

Dropping the module boundary does not dissolve the parallel stack the Context
argued for; it moves it inside the perimeter, where it is `refract/three`
beside `refract/render` rather than a second path *through* `render`. `three`
owns the 3D draw loop, the depth ordering, the cube's furniture and the camera.
`render` keeps its one drawing order for 2D panels and gains nothing.

The import path is `github.com/timzifer/refract/v2/three` either way — a nested
module and a package of its parent are indistinguishable to a caller — so this
decision was never visible from outside, which is the last argument against
having contorted the design for it.

### The IR does not gain a dimension

**There is no `ir.Point3`, no `ir.Affine3`, no depth field on a drawing call.**
`three` owns its own `Vec3`, its own 4×4 and its own `Camera`, projects
everything above the seam, and calls `Polyline`, `FillPath`, `Markers` and
`Text` with the plain 2D coordinates those have taken since v0.1.

This is the same trade `coord` made — "the IR is untouched, because an arc is
cubics and `ir.Path` has always had those" — and it buys the same thing,
larger. **Every backend renders 3D on the day `three` compiles**: SVG, PDF,
canvas, `backend/gg`, the native window, the GPU tier. No capability
negotiation, no optional interface, no backend that draws a chart the others
cannot. A third-party backend written against v0.1 draws a surface without its
author ever having heard of one.

The alternative — a `Backend3`, or a depth channel on the existing calls — puts
a scene graph into an immediate-mode drawing sink and hands every backend
author a hidden-surface problem. `ir.Backend` is a sink for ink.

### Hidden surfaces are ordered, not buffered, and the scope follows from that

A z-buffer needs pixels. SVG and PDF have none, and they are the outputs
[ADR 0004](0004-svg-source-of-truth.md) and [ADR 0009](0009-pdf-backend.md)
make the reference path. So depth is resolved by **drawing back to front**, and
a painter's algorithm is exact only when the pieces can be totally ordered.

That is not a limitation to be papered over; it is the scope:

- **A surface over a regular grid** is ordered exactly. The camera's view
  direction picks a corner of the (i, j) lattice, and iterating rows and columns
  outward from it visits every quad in strict back-to-front order. No sorting,
  no comparisons, no ambiguity — and it happens to be the chart people mean
  when they say 3D.
- **Points, markers, 3D bars and polyline segments** sort by centroid depth,
  ties broken by (layer, row) so the picture never depends on scheduling
  ([ADR 0012](0012-parallel-panels.md)).
- **Arbitrary meshes are not offered.** Two triangles that interpenetrate, or
  three that overlap cyclically, have no correct order and would need splitting
  along the intersections — a BSP tree, which is a renderer. `three` draws the
  shapes whose order is decidable and declines the ones that are not, rather
  than drawing those wrong and calling it a limitation in a doc comment.

A quad of a surface is drawn as a single filled subpath with a stroked outline,
which is also what makes it one mark to [ADR 0015](0015-hit-testing.md)'s index.

### Text is never projected; only its anchor is

`ir.TextRun` carries a position, an alignment and one rotation about the anchor
([ir/text.go](../../ir/text.go)). A label lying in a projected plane needs a
shear, and the IR has no shear for text — deliberately, since a backend shapes
its own runs.

So **tick labels and axis titles are upright at projected anchors**, and an axis
title may take the screen angle of its projected axis through `Rotation`, which
is a rotation and therefore expressible. This is not a concession: a sheared
tick label is harder to read than an upright one, and every serious 3D plotting
tool that projects its text ends up offering an option to stop.

### The cube is furniture, and which walls it draws depends on the camera

Three axes bound a box. `three` draws the two back walls and the floor — the
three faces pointing away from the camera — with the grid on them, and picks
which three those are from the sign of the view direction's components. They
are recomputed per frame, which is what makes the picture stay readable as
[0052](0052-orbiting-a-chart.md) turns it.

Ticks come from the scales unchanged. `scale.Scale` maps z into a depth
interval exactly as it maps x into a width, the tick search is the one that
already exists, and `scale.TickValues` pins them where a caller wants them
([ADR 0033](0033-smith-charts.md)). A third scale is a third scale; nothing
about it is new.

### What a pointer can ask, and what it cannot

`interact.Index` implements `render.Observer`, and both `Panel` and `Layer` are
exported methods on it. `three` announces its panels and layers the same way
`render` does, wraps the backend with `Index.Watch`, and hit-testing works
unchanged: the marks in the index are the projected ones, which is the
definition [ADR 0015](0015-hit-testing.md) already uses — what the reader can
see.

Two honest limits:

- `Index.Panel` takes an x scale and a y scale, so `Hit.X` and `Hit.Y` invert
  through *screen* axes that a projected scene does not have. `three` reports
  `Hit.Kind` and the row, and **the third value is read from the row, not from
  the geometry** — with row tracking on, the caller has an index into the table
  it supplied. This is [ADR 0045](0045-linked-views.md)'s bargain again: refract
  says exactly which datum it is, and the program says what that datum contains.
- A hit resolves to the mark drawn last where several overlap, which under a
  painter's order is the nearest one. That is the answer a reader expects, and
  it falls out of the drawing order rather than needing a ray.

### Decimation is off, and the reason is 0018's

`stat.LTTB` and `MinMax` bucket by a column of screen pixels, and a projected
scene's pixel column mixes values from everywhere along the view direction —
the same argument that made `coord.Polar` report that it does not decimate. A
surface is a grid whose size the caller chose; a 3D scatter that needs a million
points needs aggregation in `stat`, before the projection, where a bucket still
means something.

## What this does not do

- **No perspective camera.** Orthographic only, for [0050](0050-depth-without-a-third-axis.md)'s
  reason: under perspective the same value is taller at the front of the scene
  than at the back, and a chart is a measuring instrument first.
- **No lighting model.** One directional shade per face, from the face normal
  and a fixed light in the theme. No specular, no shadows, no ambient
  occlusion, no textures.
- **No volume rendering, no isosurfaces, no voxels.** Those are visualisation
  of fields, not plots of tables, and `data.Source` hands out columns.
- **No 3D pie**, for the reason 0050 gives, which does not weaken by being
  restated.
- **No facets of 3D panels in the first cut.** Layout is `internal/layout`'s
  and lives in the core; a 3D panel inside a facet grid is a second
  conversation, and `three` can draw one chart before it draws nine.
- **The IR gains nothing**, and that is the invariant this record actually
  defends. The three seams that widen are all above it. If an implementation
  finds itself adding a depth to a drawing call, an `ir.Point3` or a
  `Backend3`, the design is wrong and it comes back here before it goes
  further.
