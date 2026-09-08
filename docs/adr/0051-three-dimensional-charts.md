# 0051 — A third axis is a module above the IR, and the IR stays two-dimensional

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
implemented outside this module, so it never gains a parameter — and a z
domain has to exist before layout, because layout needs tick labels and tick
labels need a domain. Training z is therefore a second training path beside
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

**Widen the core.** `Frame.Z`, an optional `Train3`, an optional `Panel3`, a
depth-merge stage in `render`. It breaks the freeze the v1 positioning rests on
— not through the struct, through the drawing order — for a feature
CONCEPT §5 says arrives well after v1.0.

**A parallel stack inside the core.** `frame3`, `geom3`, `coord3`, `draw3`
beside the existing ones. Nothing breaks, and the core acquires a second
drawing order and a second set of interfaces inside the perimeter it has just
frozen — 0010's objection, one layer up, and now permanent.

**Its own module.** Which is what [the v1 audit](../v1-api-audit.md) wrote in
the one cell it gave the subject. The parallel stack is unavoidable; what is
still open is whether it sits inside the stability perimeter or beside it.

## Decision

**3D is `github.com/timzifer/refract/three`, a nested module that projects a
scene into the two-dimensional IR the core already has. Nothing in the core
changes.**

### Why a module, when it needs no dependency

This is the weakest part of the record and it is written as such, because the
objection is good: every nested module here exists to keep a dependency out of
the core. `backend/gg` and `backend/window` hold `gogpu`, `backend/gg/gpu`
holds `wgpu`, `arrow/v18` holds Arrow — [ADR 0001](0001-module-layout.md)'s one
rule, four times. `three` is arithmetic and `ir`; it has nothing to quarantine.
It would be the first nested module here whose reason is not a dependency, and
"the audit wrote it in a table cell" is not a reason.

The reason is that **a module has its own version**, and the repository already
uses that: the audit's own line for the GPU tier is "`backend/gg/gpu` stays
`v0.x`", which is a stability decision made by packaging rather than by a
dependency. The core is v1 and its API is frozen. A first 3D API will be wrong
in the way first APIs are wrong — the camera constructor, where the z scale is
bound, whether a surface takes a grid or three columns — and the two ways to
be wrong inside a v1 module are to freeze it before anyone has used it, or to
break the promise the library's positioning rests on. `three` at `v0.x` breaks
its own users instead, which is the deal every v0 makes, and `go.work` already
builds it with the core so a contributor sees a break the moment they cause one.

**And the choice is reversible, which is why it is the right one to make
first.** A nested module and a package of the parent module have the *same
import path*: `github.com/timzifer/refract/three` either way. Deleting its
`go.mod` folds it into the core, and no caller's import changes — what changes
is which version governs it, and therefore which promise it is under. So the
sequence is: land it as a module while it is still moving, fold it into the
core when its API has stopped, and let the users who were there for v0 keep
the line they already wrote.

The zero-dependency property is what keeps that door open rather than what
argues against the module. A nested module is excluded from its parent's module
graph, so ADR 0001's CI gate never sees it; the day `three` acquires a
dependency is the day it can never be folded in. It therefore has the same
rule as the core — **stdlib only** — and the fold-in is checked by the gate
that already exists.

The opt-in is the import, and the cost of not importing it is exactly zero.

### The IR does not gain a dimension

**There is no `ir.Point3`, no `ir.Affine3`, no depth field on a drawing call.**
`three` owns its own `Vec3`, its own 4×4 and its own `Camera`, projects
everything above the seam, and calls `Polyline`, `FillPath`, `Markers` and
`Text` with the plain 2D coordinates those have taken since v0.1.

This is the same trade `coord` made — "the IR is untouched, because an arc is
cubics and `ir.Path` has always had those" — and it buys the same thing,
larger. **Every backend renders 3D on the day the module compiles**: SVG, PDF,
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
  conversation, and the module can draw one chart before it draws nine.
- **The core gains nothing.** No new field, no new interface, no new package.
  If this record cannot be implemented without one, the design is wrong and the
  record comes back here first.
