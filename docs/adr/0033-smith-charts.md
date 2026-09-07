# 0033 — A Smith chart is the polar-shaped coord seam, over normalised impedance

**Status:** Accepted · **Date:** 2026-09-07 · **Implemented:** v1.2

## Context

[ADR 0018](0018-coordinate-systems.md) ends by declining a coordinate system:

> There is no **geographic projection**: a projection transforms every point
> with no linear interval underneath it, which is a wider seam than this one,
> and ADR 0018 says it is argued on its own evidence rather than smuggled in as
> a third `Coord`.

A Smith chart is a third `Coord`, so that sentence is the first thing this
record has to answer.

The chart itself: RF, microwave and antenna engineers read a complex impedance
against a picture of the whole right half-plane squeezed into a disc, by the
reflection coefficient

```
Γ = (z − 1) / (z + 1),   z = r + jx
```

which is a Möbius map — conformal, and one that carries every passive impedance,
including the infinite ones, inside the unit circle. It has been the standard
instrument of the field since 1939, every vector network analyser draws one, and
no general-purpose plotting library draws one, because a library whose
coordinate stage is hard-coded Cartesian cannot express it at any price. Nothing
in `refract` mentioned impedance before this record.

`docs/chart-types.md` sorts the missing charts by the machinery each one needs.
A Smith chart needs exactly one piece: a coord. Everything else it wants — a
line over two columns, a scatter, a rectangle for a tolerance region, a legend,
hit-testing — shipped between v0.1 and v0.9.

## Decision

**A Smith coord is the same shape of seam `Polar` is, and it is over normalised
impedance: X is r = R/Z₀ and Y is x = X/Z₀.**

### Why it is the polar seam and not the projection seam

The three things that make a projection wide are absent here.

1. **It transforms a mapped pair, not a raw point.** `Polar` takes (θ, r) and
   applies sin and cos; `Smith` takes (r, x) and applies a Möbius map. Both are
   nonlinear functions of a pair the scales already produced. There is a linear
   interval under each axis, and `scale.Scale` does not change — which is the
   affordability argument ADR 0018 was built on, unaltered.
2. **Its grid comes from the panel's own ticks, so `render` is untouched.**
   `render.drawAxes` walks `for i, t := range xTicks`, takes geometry from
   `fur.GridX[i]` and label text from `t.Label`. A coord may therefore draw one
   grid line per tick a scale emits, and may label nothing the scale did not. A
   Smith chart's two grid families are the constant-resistance circles and the
   constant-reactance arcs — and those are exactly the images of the two axes'
   own grid lines. So the grid is not drawn by anything: it is what the X and Y
   ticks look like once the coord has had them, the same way a polar coord turns
   a Y tick into a ring. Not one line of `render/` changed for this milestone. A
   projection's graticule has no such tick behind it, which is the concrete
   difference between the two seams.
3. **The IR gains nothing.** Every curve here is a circular arc, and an arc is
   cubics, which `ir.Path` has had since v0.1. ADR 0002's four operations still
   describe the whole chart.

ADR 0018's four binding properties hold unchanged: Cartesian is still the
identity and the golden files still prove it; the coord reports geometry and
does not paint; `Points` is still the batch form and the allocation gate is
still green; and this coord declines to decimate, for ADR 0011's reason — the
map crowds the whole far half-plane into the last pixels before the rim, so a
bucket of equal width on screen measures nothing in particular. Nothing on a
Smith chart is a big-data chart.

### The data is impedance, and that follows from the constraint above

The alternative was to read the reflection coefficient itself — which is what an
instrument reports, and what is numerically bounded. It was not taken because of
property 2: with Γ on the axes, the ticks are values of Γ, and the impedance
grid would have no tick behind it. The coord would then have to supply its own
grid lines *and their labels*, which means widening `Furniture` to carry
families rather than sides and teaching `render` to draw a label the scale did
not write. That is the wider seam, and it is deferred to its own evidence
exactly as a projection is.

`coord.SmithZ` is the bridge: `z = (1+Γ)/(1−Γ)`, so a measured sweep is one line
at the call site. It is the coord's own inverse expressed in data terms, and
`smith.Invert` calls it, so there is one copy of the arithmetic and a tooltip
and a column cannot disagree.

### The interval a Smith coord chooses is the impedance itself

`Frame` gives each scale the range its own domain already is:

```go
lo, hi := x.Domain()
x.SetRange(float32(lo), float32(hi))
```

so a linear scale's `Map` is the identity and the pair reaching `Point` is the
impedance. Choosing what the interval means *is* the definition of a coord in
this design — Cartesian chooses a distance along an edge, Polar chooses radians
and pixels — so this is the mechanism rather than an exception to it.

The honest cost, recorded in the doc comment and in `AGENTS.md`: **it assumes an
affine scale.** A log or symlog axis under this coord would hand `Point` the log
of a resistance, which is a different chart, and the coord does not guess which
one was meant. The alternative — keeping the scale references in the framed
coord and calling `Invert` per point — was rejected on three counts: it puts two
interface calls on the hot path that ADR 0018 property 3 exists to keep out, it
rounds twice, and a framed coord holding scale pointers reads state another
panel's `Frame` may be writing, which is precisely what `Frame` returning a copy
exists to prevent.

### Edges are chords by default, and cells are always curvilinear

A Möbius map takes a straight segment in impedance to a circular arc. The
default is nevertheless the chord, because a chord is what the chart *means*: an
instrument reports samples, and a line between two of them asserting a linear
sweep in z is an assertion the instrument did not make. `coord.SmithArc` is the
opt-in for a locus that genuinely is straight in impedance — a series reactance
tuning path — where the arc is the truth and a chord cuts across it. It is the
same policy `Polar` makes, made the other way round: there the default is the
arc and a radar asks for chords.

`Area` is not a policy. A `geom.Rect` or a Region is a claim about a region of
impedance, so its boundary is the image of its boundary, whatever the edge
policy says. It is the exact arcs in both modes.

The arc through two impedances is settled by three points: the two ends and the
image of the line's own point at infinity, which is Γ = 1 for every line —
because every line runs out to an impedance of infinite magnitude, and that is
an open circuit. The arc is the one that does not pass through it, because a
segment between two finite impedances does not pass through infinity. A line
through the map's pole at z = −1 has a straight line for its image and no circle
through the three points; the construction reports that and the edge is drawn as
the chord it is.

### Two additions beside the coord

- **`scale.TickValues(vs ...float64)`** pins a linear axis's tick positions.
  The canonical Smith grid is 0 / 0.2 / 0.5 / 1 / 2 / 5 — six values every
  engineer expects in those places, not evenly spaced and not meant to be, and
  no tick-choosing algorithm produces them. It is additive, it is useful to any
  axis whose ticks are a convention, and it round-trips through `scale.Desc` and
  the document.
- **`coord.SmithAdmittance`** draws the Y chart, whose columns are a
  conductance and a susceptance. It is one sign: y = 1/z gives Γ_y = −Γ_z. The
  half turn is applied to the picture and not to the data, so the two changes
  cancel and one load lands in one place whichever chart it is read on — the
  same physical reflection against the other grid, which is what makes it a
  second reading rather than a second measurement.

## Consequences

| | |
|---|---|
| `render`, `ir`, `geom`, `layout` | unchanged |
| `scale` | one additive option, one `Desc` field |
| `coord` | one type, four options, one helper, one `Desc` field; `polar`'s arc construction lifted into `coord/arc.go` and shared, which the polar golden files prove was inert |
| `spec` | a `"smith"` coord type, an `admittance` field, a `tickValues` field, and a per-type default for `edge` — the two coords default opposite ways, so an absent field has to mean each one's own default |
| `interact` | works unchanged: a hover inverts through the coord and then through the scales, and under this coord the second step is the identity, so a hit reports the impedance |
| `a11y` | unchanged; it never mentions a coord |
| layout | inherited from Polar: the solver reserves gutters for tick labels that a disc writes inside itself or around its rim, so the disc is a little smaller and a little higher than centre. `SmithRadius` is the same mitigation Polar's `Radius` is. ADR 0018 declined to teach `layout` about a radial axis and this record declines again |
| zoom and pan | a no-op. `Frame` re-derives the range from the domain every frame, so moving the domain relabels the grid and moves nothing. That is right: a Smith chart's extent is the unit disc, always |

## Not in scope

- **Constant-|Γ| (VSWR) circles and constant-Q arcs.** A third and fourth grid
  family. `Furniture` carries two per-tick lists because `render` walks two tick
  lists; a family with no tick behind it has nowhere to come from and nothing to
  be labelled by. It is the same constraint that chose the data model, and it is
  one constraint rather than two.
- **A combined ZY overlay.** Two grids from one coord, for the same reason.
- **Γ as input**, per the argument above.
- **Touchstone (`.s1p`) parsing.** A file format is a `data.Source`.
- **A log or symlog axis under this coord.** Out of domain by construction.

## Revisit if

- A second chart wants a grid family its axes have no tick for. That is the
  moment to argue a `Furniture` that carries families rather than sides — and
  it would reopen Γ-as-input, VSWR circles and a projection's graticule
  together, which is the right way to spend that seam.
- A Smith chart turns out to be a big-data chart, which would reopen
  `Decimates` — though the reduction, not the answer, is what would have to
  change.
