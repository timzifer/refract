# 0026 — A mark is broken out by displacing it, and the coord answers how far

**Status:** Accepted · **Date:** 2026-09-05

## Context

v0.8 made a pie out of the marks that already existed: a stacked `geom.Bar` in
`coord.Polar` with θ from the Y axis ([ADR 0018](0018-coordinate-systems.md)).
Two things that readers of a donut expect were still missing from that, and
both of them look at first like they need a new mark.

**A slice's radii.** The radial axis of a pie carries nothing. Every slice fills
the same slot, because the X column is a constant and the only reason for it is
that a bar needs a position. A donut where each slice reaches a different
distance — the second measure of a two-measure comparison, a budget against its
ceiling, a nightingale rose with a floor — has no spelling at all.

**A slice pulled out of the ring.** Every spreadsheet has it, and it is the one
piece of pie-chart vocabulary that is about emphasis rather than about the data.
It has no spelling either, and the obvious ways to fake it are all wrong: adding
a constant to the radius moves the ink and changes what the slice says, and
painting a second layer over the first duplicates the row.

## Decision

### The radii are `X` and `X2`, and nothing new

`geom.Rect` has read a pair of columns per axis since v0.7 — a gantt bar runs
from a start column to an end column. `geom.Bar` now reads the same pair on its
cross axis, and under a polar coord that axis is the radius. So a slice's inner
and outer radius are dimensions of the data in exactly the way its angle is:

```go
geom.Bar(src, geom.X("floor"), geom.X2("reach"), geom.Y("share"),
    geom.GroupBy("browser"))
```

A bar that names both of its edges is not widened at training either, for the
reason a rect with an `X2` is not: its width is in the domain already.

The alternative was a `geom.Slice` with `Inner`/`Outer` channels. It was
rejected because those channels cannot exist: `Geom.Train` is handed the two
scales and no coord, so a layer cannot know at training time which of them is
the radius — and a mark that only draws under one coord is the pie geom this
project has said four times it does not have.

### The break-out is a displacement, and the coord reports it

`geom.Explode(f)` moves every mark of a layer away from the middle of the coord
by the fraction `f` of its outer radius; `geom.ExplodeBy(col)` reads that
fraction per row, which is what pulls one slice out and leaves the rest in the
ring.

Three properties fall out of making it a displacement rather than a change of
extent:

- **The slice still says what it said.** Its angle, its inner radius and its
  outer radius are the ones the data gave it. Growing the radius instead — the
  way an exploded pie is usually faked — moves the ink *and* the reading, which
  is how exploded pies got their reputation.
- **The ring it left is still there.** The gap is the evidence: a reader can see
  where the slice came from, because nothing else moved.
- **It is hit-testable for free.** The path a geom hands the backend is the path
  that gets indexed ([ADR 0015](0015-hit-testing.md)), so a pointer follows the
  slice out of the ring and a pointer in the gap finds nothing. The reported row
  position is displaced with it.

The direction is the mark's own bisector. A wedge moved along its middle keeps
an equal gap on both of its radial edges; one moved along an edge closes the gap
on that side and opens twice as much on the other.

### `coord.Exploder` is an optional interface, and Cartesian does not implement it

The coord is the only thing that knows where the middle is, so it answers *how
far* — and the geom, which built the path, moves it. That keeps the split the
coordinate stage already draws: a coord reports geometry and does not paint, and
`geom` works in mapped space and lets the coord place the point.

`coord.Cartesian` deliberately does not implement it. A rectangle on a Cartesian
panel has no direction to be broken out in; moving every bar the same way is a
translation of the layer rather than a reading of it. A layer that asks for a
break-out under a coord with no middle draws exactly what it drew — which is
what keeps every golden file in the repository unchanged by an option every geom
now accepts.

Two smaller consequences are worth writing down. The displacement is collected
once per mark into the frame's scratch and carried through the colour and group
batching, so a broken-out layer costs the same drawing calls as a whole one and
allocates nothing per mark. And the arithmetic is applied to the points the
`Area` call appended, rather than by asking the coord for a second transform:
there is no second path and no second traversal.

### `coord.Pie` and `coord.Donut` name the recipe

`coord.Donut(0.45)` is `coord.Polar(coord.Theta(coord.FromY), coord.Hole(0.45))`
and is nothing else — the description round-trips as the polar coord it is, and
a test asserts the two draw the same chart. It is sugar because the recipe was
the one thing about v0.8 that had to be memorised rather than discovered, and
because a name is where its two neighbouring facts can be written down: that
neither scale is niced, and that the hole is where the radial scale starts.

## Consequences

- A donut carries three dimensions per slice instead of one, without a new mark
  and without a new channel: `Y` goes round, `X` and `X2` go out.
- `geom.Desc` gains `Explode` and `ExplodeCol`, and the document gains an
  `explode` mark property and an `explode` channel. Both are refract's own —
  Vega-Lite has no coordinate stage and so no middle to move away from.
- A layer with a break-out is drawn under a coord that cannot answer as though
  it had asked for nothing. That is silent by design; the alternative is an
  error for an option that is meaningful in one coord and meaningless in
  another, which would make every Cartesian chart's option list conditional.
- The break-out is per row, not per group. A donut has one row per slice, so
  that is the same thing there; a stack of several rows per series would move
  its segments independently, which is what a per-row channel means everywhere
  else.
