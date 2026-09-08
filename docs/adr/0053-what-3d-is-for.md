# 0053 — What the third dimension is for, and where it stops paying

**Status:** Planned · **Date:** 2026-09-08 · **Implementation:** not started

## Context

[ADR 0050](0050-depth-without-a-third-axis.md), [0051](0051-three-dimensional-charts.md)
and [0052](0052-orbiting-a-chart.md) decide *how* 3D is built. None of them
says what it is for, and a machinery whose charts are never enumerated is a
machinery whose scope creeps: the surface ships, then a mesh loader looks like
one more geom, then the library is a renderer.

[docs/chart-types.md](../chart-types.md) is the precedent and the method —
"sorted **by the machinery each form needs** rather than by how popular it is.
Sorted that way the list stops being a wish list and becomes a schedule." It
worked because in 2D the machinery is the hard part and the chart is a recipe
over it.

**In 3D that method is not enough on its own,** because the machinery answers a
question nobody was asking about half the forms. Whether refract *can* draw a
3D pie is not interesting; whether a reader learns more from one is, and the
answer is no. So this catalogue is sorted by a second axis first.

## Decision

**A form is ranked by what the third dimension gives the reader that the flat
chart of the same data does not. The machinery decides the order of work within
a rank; it does not promote a form between them.**

Three answers to that question, and each rank is one of them:

- **it carries a reading the 2D chart cannot** — the forms that justify 0051;
- **it carries the same reading, differently** — worth having, usually not
  worth choosing;
- **it carries less** — refused, however easy it would be.

### Rank 1 — the forms that justify the machinery

Each needs exactly what 0051 builds and nothing more: a camera, a projection,
back-to-front order over an orderable set, and a cube of furniture.

| Form | What the third dimension carries | Machinery |
|---|---|---|
| **Surface** over a grid, z = f(x, y) | the shape of a response between its samples: a ridge, a saddle, a plateau's edge. A heatmap of the same grid gives the *values* and hides which way the ground falls | 0051 exactly: the grid's own back-to-front order |
| **3D scatter**, three measured columns | whether a cluster is a cluster or two clouds that overlap along the axis you happened not to plot | 0051, marker path, depth sort |
| **Trajectory / phase space**, an ordered path in x, y, z | a path that crosses itself in every 2D projection and not in the data — an orbit, a tool path, an IMU track, an attractor | 0051, polyline per segment |
| **Cascade / waterfall**, a family of traces offset in z | how a spectrum *moves*: a peak drifting, a harmonic appearing, a resonance splitting. It is the display a spectrum analyser has had since the seventies, and it is the tier-1 form this library's existing audience will ask for first | 0051 plus nothing: a recipe over N 3D lines |

The last row is the one that decides whether 0051 pays for itself. A library
that ships `coord.Smith` ([ADR 0033](0033-smith-charts.md)), tracks, thresholds
and error bars has an RF and instrumentation audience already, and the cascade
is the chart that audience draws on a whiteboard when explaining what they
measured.

### Rank 2 — the same reading, differently

These are recipes over rank 1's machinery. They ship because they cost nothing
once it exists, and each one carries a note saying which 2D chart usually beats
it — the catalogue is not neutral about this, because a reader who chose wrong
is a reader who was not told.

| Form | Recipe | The 2D chart that usually wins |
|---|---|---|
| **Terrain / DEM** | surface + a sequential ramp on z | a hillshaded heatmap, until the reader has to judge slope |
| **Ribbon** | a 3D line with width | a line with a band, which is what an interval means |
| **Stem / dropline 3D** | scatter + a rule to the floor | nothing: it is what makes a 3D scatter's heights readable, so it ships *with* the scatter |
| **Projected contours on the walls** | `stat.Contour` evaluated on the same grid, drawn on the floor and back walls | the contour plot itself — see below |
| **3D bars** over two categoricals | `geom.Bar` in a projected box | **almost always the heatmap.** Bars occlude each other, the back row is unreadable, and the height of a bar behind another cannot be compared to it. It ships because refusing it invites a worse reimplementation by every caller, and its doc comment says this |

**`stat.Contour` is a 2D feature that 3D wants.** `README.md` lists contour
plots among the things deliberately not here; they are a pure function in
`stat` over a grid and a `geom` that strokes the level sets
([ADR 0028](0028-distribution-stats.md)'s shape). It is not blocked on 3D and
3D is not blocked on it — but the same function serves the flat contour plot
and the surface's floor projection, so it is scheduled between them.

### Rank 3 — the sphere, and the family it unlocks

One more piece of machinery buys a whole family, exactly as `coord.Polar` did
in v0.8. A **spherical coord** — a direction (θ, φ) and a radius, mapped into
0051's scene — is one implementation, and five forms fall out of it.

| Form | Field | What the sphere carries |
|---|---|---|
| **Antenna radiation pattern**, r = f(θ, φ) | RF | the whole pattern: main lobe, nulls, back lobe, and their relation. The two cut planes an antenna datasheet prints are the 2D fallback, and they are cuts *because* the page is flat |
| **Directivity / beam scan** | RF, acoustics | the same, swept |
| **Poincaré sphere**, the Stokes vector | optics, RF | polarisation as one point instead of three coupled numbers |
| **Bloch sphere** | quantum | a two-level state as a point, which is the entire pedagogical reason it exists |
| **Stereonet / orientation density** | structural geology | a distribution of *directions*, whose flat form is a projection of this sphere and always has been |

They are one machinery and five audiences, which is the argument for building
it — and it is a separate step from 0051 for the reason polar was a separate
step from Cartesian: the first proves the projection, the second proves it was
general.

A **geographic globe** sits here too, and is the interesting case: the v1 audit
files geographic projections as "a third `Coord` behind the same interface".
Orthographic and perspective globes are *this* sphere rather than that coord,
and the flat projections stay 2D — so the two remain separate features that
happen to share a noun.

### Rank 4 — the absolute niche, and why it is in the catalogue at all

**The 3D Smith chart.** The 2D one maps impedance to the reflection coefficient
Γ inside the unit disc, and everything with |Γ| > 1 — an active device, an
oscillator's negative resistance, an unstable region — is *off the page*. Its
3D form projects the Γ-plane onto a sphere, where the outside of the unit
circle is simply the other hemisphere: the infinite plane becomes finite, and
the one class of circuit the flat chart cannot show becomes a place you can
point at.

It is the nichest chart in this document by a wide margin, and it is here for
three reasons that matter more than its audience size:

1. **It is rank 1 by the ranking rule.** The third dimension carries a reading
   the flat chart provably cannot — that is the definition, and popularity is
   not part of it. The catalogue would be dishonest if it quietly demoted a
   form for being rare.
2. **It is the proof the sphere is general.** A spherical coord that draws an
   antenna pattern and a Smith sphere is a coordinate system; one that draws
   only patterns is a chart type with delusions.
3. **This library already shipped its 2D counterpart**, and ADR 0033 argued
   that a Smith chart was not a special case but "the polar-shaped coord seam,
   over normalised impedance". The same sentence should survive one dimension
   up, and if it does not, the sphere is shaped wrong.

Beside it, at the same distance from the mainstream and reachable by the same
machinery: the **ternary prism** (a ternary diagram extruded by a fourth
variable — metallurgy, petrology) and the **spherical histogram** over a
direction column.

### What 3D does not enable

Named because they are what "we have 3D now" invites, and each is a different
library:

- **Arbitrary meshes, STL/glTF/OBJ, CAD.** 0051's painter order is exact only
  over orderable sets; a triangle soup needs a BSP tree or a depth buffer, and
  a depth buffer needs pixels the SVG and PDF backends do not have.
- **Volume rendering, isosurfaces, voxels.** These visualise fields;
  `data.Source` hands out columns.
- **Point clouds at LIDAR scale.** Decimation is off in a projected scene
  (0051), so the reduction has to happen in `stat` before projection, and a
  point cloud's reduction is a spatial one nobody has asked this library for.
- **The 3D pie.** [ADR 0050](0050-depth-without-a-third-axis.md) refused it
  where it was cheapest to draw; a real projection does not make it truer.
- **Animated 3D as a chart type.** Turning a scene is 0052; a scene that turns
  by itself is a video, and a reader cannot compare two moments of one.

## Order of work

1. **0051's machinery, with surface, 3D scatter + droplines, and 3D line.**
   Rank 1 minus the cascade, which needs no code.
2. **The cascade**, as an example and a doc page rather than a geom.
3. **`stat.Contour`**, which pays for the flat contour plot README currently
   disclaims and for the surface's floor in the same function.
4. **0052's orbit.** Independent of 2 and 3, and the point at which the surface
   stops being a picture and starts being an instrument.
5. **The spherical coord**, and with it rank 3 — patterns first, because they
   have the largest audience, and the Smith sphere last, because it is the
   test that the coord is a coord.

3D bars land wherever they land. They are four lines over the surface's
machinery and they are the one form in this document whose main purpose is to
be available so that nobody builds a worse one.
