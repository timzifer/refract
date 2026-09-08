# 0050 — A mark gains volume before a chart gains a dimension

**Status:** Planned · **Date:** 2026-09-08 · **Implementation:** not started

## Context

Three documents in this repository say the same thing about 3D and none of them
says what it is. [CONCEPT §5](../../CONCEPT.md#5-non-goals) has "**No 3D until
well after v1.0**, and only then tightly scoped"; §14 files "3D
(surface/scatter3d) — deliberately late, tightly scoped" under the roadmap;
[the v1 audit](../v1-api-audit.md) parks it in one table cell — "3D | Its own
module | later". `README.md` lists it among the things deliberately not here.

Four years of "later" is a decision that was never taken, and it has stayed
untaken because **three unrelated features share the name**:

1. a chart of two variables **drawn with volume** — extruded bars, a slab of a
   panel seen from a corner. Depth carries nothing; it is ink;
2. a chart of **three variables**, x, y and z, projected onto the plane — a
   surface, a 3D scatter, a ribbon;
3. the same chart **turned with the mouse**, at framerate.

They have nothing in common but the word. The first touches one coord and two
geoms and cannot change what a chart *says*. The second needs a third scale,
which the frozen `geom.Frame` cannot carry ([ADR 0029](0029-extension-model.md)).
The third needs a camera, a frame loop and an answer about which backends can
even have one. Priced together they look like a rewrite, which is exactly why
they kept being deferred as a unit.

So they are three records, and this is the first:

| # | What | Blast radius |
|---|---|---|
| 0050 (this) | Depth as decoration: an oblique coord and extruded marks | one coord, two geoms, the IR untouched |
| [0051](0051-three-dimensional-charts.md) | A third scale, projected: surface, scatter3d, line3d, bar3d | three widened seams and a `v2` tag, the IR still untouched |
| [0052](0052-orbiting-a-chart.md) | Turning it with the mouse | that module's live loop; no core change |

Each is worth having on its own, and each is the honest prerequisite for the
next. This one is worth having because it is the whole of what most people
asking for "3D bars" are asking for, and it costs almost nothing.

## Decision

**Depth is a displacement with no meaning attached. It is chosen once per
chart by the coord, applied per mark by the geom, and no column may be bound
to it.**

That last clause is the record. A chart drawn with volume is a chart of two
variables, and the moment a third column could set the depth it would be a
chart of three variables drawn without a scale, without ticks and without a
legend — a quantity the reader can see and cannot measure. There is no
`geom.ExtrudeBy(col)`, and there will not be one: [0051](0051-three-dimensional-charts.md)
is where a third column gets an axis, and building a nameless version of it
here would poison that name before it was used.

### `coord.Oblique` is Cartesian with a depth vector

```go
refract.New(src).
    Coord(coord.Oblique(coord.Depth(14), coord.DepthAngle(-math.Pi/4))).
    Layer(geom.Bar(src, geom.X("quarter"), geom.Y("revenue"), geom.Extrude(true)))
```

`Oblique` embeds `cartesian`. Its `Point` is the identity its parent's is, its
`Straight` is true, its `Invert` is the inverse it always was, its `Edge` is
one `LineTo`, and `Decimates` stays true — a pixel column is still a pixel
column, so [ADR 0011](0011-decimation.md) keeps its guarantee unreduced. What
it adds is one method, behind the optional interface [ADR 0026](0026-breaking-a-mark-out.md)
established the shape of:

```go
// Extruder is implemented by a coord that sees its panel from an angle: a
// mark drawn under it has a back as well as a front.
type Extruder interface {
	// Extrude reports the device offset from a mark's front face to its
	// back one. It is the same vector for every mark in the panel.
	Extrude() (dx, dy float32)
}
```

`Cartesian` deliberately does not implement it, exactly as it declines
`Exploder`: a layer that asks to be extruded under a coord with no angle to
see it from draws precisely what it always drew, and nothing is invented. A
geom resolves the interface once per `Build`, never per mark.

**`Oblique.Frame` insets the panel rectangle by the depth vector before it
frames the scales.** The volume comes out of the plot area, not out of the
margins: the axes still bound the data, the silhouettes stay inside `Clip`, and
the layout solver of [ADR 0010](0010-panel-layout.md) is not asked a new
question.

**The front plane is unforeshortened, and that is the only reason any of this
is allowed.** An oblique projection moves the back face and leaves the front
one alone, so a bar's height in pixels is the height it would have had on a
flat chart, and two equal bars are equal wherever they stand. A perspective
projection — a vanishing point, a camera with a field of view — is refused
here and refused again in 0051: it makes the same value taller at the front of
the scene than at the back, which is the misreading that gave 3D charts their
reputation, and it would be one this library shipped on purpose.

### Three faces, one row, and one more area under the pointer

An extruded bar is a front face, a top face and a side face, filled in the
mark's colour and two fixed shades of it from the theme (`theme.DepthTop`,
`theme.DepthSide`, mixed in the linear-light space `palette` already mixes in —
there is no light model, no normals and no material).

[ADR 0015](0015-hit-testing.md) indexes **one mark per subpath**, so the
pointer sees three areas where it used to see one. That is correct rather than
unfortunate: each face is a place a reader can point, and all three answer with
the same row, because `Frame.Marks` still reports one position per row — the
front face's anchor, the position that row would have had on a flat chart. The
one visible consequence is that `Live.Select` gathers rows per mark, so a
rectangle over an extruded layer would name a row once per face it covered; the
row list a `Select` reports becomes a set.

### Paint order within the layer is decided, not left to chance

Extruded marks overlap, so a layer draws its marks back to front: sorted by the
projection of each mark's anchor onto the depth vector, ties broken by row
index. The tie-break is not decoration —
[ADR 0012](0012-parallel-panels.md) requires that nothing a chart draws depends
on scheduling, and a sort with an undefined order on equal keys is a golden
file that changes when the runtime feels like it.

The sort is over marks, not over pixels. There is no depth buffer here, and
[0051](0051-three-dimensional-charts.md) explains why there cannot be one under
a vector IR.

### The extruded pie is refused

`geom.Bar` and `geom.Rect` extrude. `geom.Arc` under `coord.Polar` does not,
and the refusal is the second half of this record's argument.

An extruded bar keeps its height, so the quantity survives the decoration. A
tilted pie does not: foreshortening the disc into an ellipse makes the near
slices cover more area than the far ones for the same angle, and area is what a
reader estimates a pie with. The chart would be lying in the one channel it
has. A library that draws a mark whose whole purpose is to distort a reading
cannot then say — as `docs/chart-types.md` does throughout — that it sorts
charts by what they cost the reader.

So `coord.Oblique` does not implement `Extruder` for a polar panel, because it
is not a polar coord, and `coord.Polar` does not gain the method. A layer
asking for it there draws a flat pie, which is a pie.

### What it costs elsewhere

- **The IR does not change.** Three faces are three fills, and a fill has been
  in `ir.Backend` since v0.1. Every backend — SVG, PDF, canvas, gg, window,
  GPU — draws this without knowing it exists.
- **The spec round-trips it.** `coord.TypeOblique` joins the built-in `Type`
  constants (and is therefore refused by `Register` afterwards, as the other
  three are), `coord.Desc` gains `Depth` and `DepthAngle`, and `geom.Desc`
  gains `Extrude bool`. Both are additive fields on structs that already carry
  per-coord and per-geom options.
- **The description does not change.** [ADR 0024](0024-accessibility.md) says a
  chart reports what it plots, over what range, and how much of it there is.
  Depth is none of those, and a `Detail` that mentioned it would be describing
  the ink.
- **Responsiveness works because the depth is a theme length.** A chart that
  follows its surface by scaling its theme ([ADR 0025](0025-responsive-charts.md))
  scales its depth with everything else, rather than keeping a 14-pixel slab on
  a chart half the size.

## What this does not do

- **No third column, no z axis, no z ticks.** That is 0051, and the split is
  the point of this record.
- **No perspective.** See above; 0051 keeps the refusal.
- **No shadows, no ambient occlusion, no bevels.** Two shades and a silhouette
  are what an extrusion is; anything more is a renderer, and CONCEPT §5 says
  refract is not one.
- **No extruded line or area.** A ribbon behind a line is a surface with a
  depth of one row, and it belongs with the surfaces in 0051 rather than
  alongside the two marks that are already rectangles.
- **No per-mark depth.** The depth vector is the panel's. A mark that stood
  further back than its neighbour would be encoding something, and this record
  is about the case where nothing is encoded.
