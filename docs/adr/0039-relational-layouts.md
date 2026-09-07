# 0039 — A relational layout is a stat in the unit square, and the coord decides what it looks like

**Status:** Accepted · **Date:** 2026-09-07 · **Implemented:** v1.4

## Context

`docs/chart-types.md` sorts every chart refract cannot draw into buckets by what
machinery each one needs. A–D and F–H have shipped. **Bucket E was the last**,
and `CONCEPT §14` names it the only family that shares nothing with the rest and
can therefore be moved without cost:

> Relational and hierarchical layouts: sankey/alluvial, chord, arc diagrams,
> treemap, sunburst. The one family in `docs/chart-types.md` that shares no
> machinery with the rest — its own data shape, its own solver, its own legend,
> its own hit-testing.

Three of those four claims turned out to be right and one turned out to be
wrong, which is most of what this record is about.

`docs/chart-types.md §E` had already settled the parts that are not
controversial: the data layer does not change, because an edge list is two
string columns and a value column and a hierarchy is `(id, parent, value)` — a
self-referential edge table, which `data.Source` already returns. And the layout
algorithms belong in `stat`, because a squarified treemap is values plus a
rectangle in and rectangles out, which is exactly what AGENTS.md scopes `stat`
to.

What was left open is the shape of the seam: how many marks this is, where the
layouts run, and what a document calls them.

## Decision

**A relational layout is a pure function that fills the unit square, and the
coordinate stage decides what the unit square looks like.** Four marks, and two
of the six charts are recipes over them.

| Mark | under `coord.Cartesian` | under `coord.Polar` |
|---|---|---|
| `geom.Treemap` | treemap | — |
| `geom.Icicle` | icicle, flame graph | **sunburst** |
| `geom.Sankey` | sankey, alluvial | — |
| `geom.Arc` | arc diagram | **chord diagram** |

### The one claim in CONCEPT §14 that was wrong

"Its own legend, its own hit-testing" — neither turned out to be true, and the
reason is the same in both cases: v0.7 and v0.8 had already generalised the
thing this bucket would have needed.

`geom.Legender` ([ADR 0020](0020-discrete-colour-and-multi-entry-legends.md)) is
a layer that contributes *N* entries because its series live inside it. A
relational layer's nodes are exactly that, so the legend needed nothing. The
only change outside `geom` was `Plot.showLegend`, which asked whether a layer
named a group column and now also asks whether it named an edge table — the same
question about a second kind of series.

Hit-testing ([ADR 0015](0015-hit-testing.md)) indexes one mark per *subpath* of a
fill, so a layout that draws one subpath per cell is pointable with no new
machinery. `interact` was not touched.

`coord.Coord.Area` was the third piece already in place: it appends a Cartesian
rectangle or a polar annular sector
([ADR 0018](0018-coordinate-systems.md)), which is why a treemap cell, an icicle
band and a chord diagram's arc are one call.

So bucket E needed its own data shape and its own solvers, and nothing else.

### Why sunburst and chord are recipes rather than marks

This is v0.8's move made twice. One bar layer draws a bar chart and a pie, and
the pie is not a second implementation of anything — it is the same mark under a
coord that wraps one axis round a circle. An icicle's layout is a span across and
a depth out; wrapped round a circle, the root is at the middle and the leaves are
at the rim, and that is a sunburst. An arc diagram's layout is a position along
a rail and a height off it; with the rail at the rim, the ribbons cross the
middle, and that is a chord diagram.

Making either a mark of its own would mean two implementations of one layout,
which is precisely what the coordinate stage exists to prevent. The cost is
written down under **Consequences**.

### The convention: rim is y = 1, hub is y = 0

Every layout here puts its span on X in `[0, 1]` and its height on Y in `[0, 1]`,
and both scales are trained on exactly that.

`rim = y1` is not arbitrary. The two coords disagree about which end of Y is
"out": a Cartesian panel flips it, so `y = 1` is the top of the plot
(`coord/coord.go`, `y.SetRange(area.Max.Y, area.Min.Y)`), and a polar one does
not, so `y = 1` is the outer rim (`coord/polar.go`, `rad.SetRange(q.r0, q.r1)`).
`rim = y1` is the one choice that makes both polar recipes work with one pair of
scales and no reversed domain — and a reversed positional domain would not have
survived the round trip anyway, because the JSON dialect's `scale.reverse` is a
colour scale's.

A sunburst's depth is therefore `y = depth / maxDepth`, root at the hub.

`coord.Polar()` and not `coord.Pie()`: a pie sweeps the *Y* axis round the
circle, and these marks' Y is their depth.

### The layouts see numbers and never a name

`stat` takes `[]int` node ids. Interning a name is where the order of everything
downstream is decided — which column a sankey's node stands in, which way round a
chord diagram goes, which entry of the palette each node takes — and that order
has to be the order the rows appeared in rather than a map's
([ADR 0012](0012-parallel-panels.md)). A package that knows about numbers and
nothing else cannot intern a string without inventing an order, so it does not
try: the geom interns, with a map that is only ever asked whether it has seen a
name and never what it holds.

`stat.SankeySweeps` is a constant and not a tolerance, for the same record's
sake. A relaxation that ran until it settled would make the picture depend on
floating-point noise, and a chart whose panels are built on separate goroutines
has to be byte-identical to one built serially.

### The squarify runs in Build, and that is ADR 0028's exception a third time

[ADR 0028](0028-distribution-stats.md) puts a distribution stat in `Train`,
because its output is what the axis has to describe, and names two exceptions:
a hexagonal lattice and a beeswarm's offsets, both of which compute a length on
screen.

A squarified treemap is the third, and for a stronger reason than convenience.
What it optimises is an *aspect ratio on screen*. Squarifying the unit square and
then stretching it into a 16:9 panel produces cells whose aspect ratio is 16:9
times worse than the algorithm chose — the algorithm's entire purpose, defeated.
So it runs in `Build`, against the rectangle the scales actually map into. It
costs nothing on the axis side, because the axis reports the unit square whatever
the panel's shape.

Everything else — the node table, the depths, the roll-up, the spans, the layer
assignment, the relaxation — runs in `Train`, where ADR 0028 puts it.

### Five channels, and why they are not three

A hierarchy *is* an edge table, so one vocabulary was tempting. It is wrong.
A hierarchy's edge runs from the child to its parent and a flow's runs from
source to target, so a document reading

```json
{"from": {"field": "task"}, "to": {"field": "phase"}}
```

on a treemap would read as a flow. `geom.From`/`geom.To` name a flow's ends,
`geom.ID`/`geom.Parent` a hierarchy's, and `geom.Value` is shared. In the
document they are five channels on `encoding`, beside `width` and `explode`,
because a channel is what names a column.

### The arc diagram is `"arc-diagram"` in the document

Vega-Lite's `arc` is a pie wedge. `spec`'s own vocabulary rule is to make the
nearest true statement rather than borrow a name that means something else
([ADR 0014](0014-json-spec.md)), and borrowing this one would make a Vega-Lite
document decode into a mark that draws something entirely different. The Go
constructor stays `geom.Arc`, which is the name a reader looks for.

### `geom.Thickness` is a new option and not `geom.BarWidth`

`BarWidth`'s shared default is `0.8`, so a mark could not tell a caller who asked
for 0.8 from one who asked for nothing. That is the trap `DashSet`, `StackSet`
and `AlignSet` each carry a companion flag to avoid; a new option whose zero
value means "the mark's own" avoids it without a flag.

## Consequences

**The Cartesian readings are unconventionally oriented.** `rim = y1` puts an
icicle's root along the bottom — the flame-graph orientation rather than the
sunburst-unrolled one — and an arc diagram's rail at the bottom with the ribbons
above, which is conventional, only because `geom.Baseline` defaults to 0. That
asymmetry is the honest price of one mark drawing two charts, and `Baseline` is
the knob that moves the rail.

**Both axes describe the unit square,** which is the truthful answer — a
treemap's numbers are shares of the whole — and nothing a reader needs to see. A
chart of one of these marks wants a theme with no grid, no axis lines and no
ticks, exactly as a pie does.

**A hit reports the layout's own coordinates.** `interact` inverts the coord and
then the scales, so `Hit.X` and `Hit.Y` on a sankey are positions in the unit
square. For these marks the *row* is the reading and the values are the
layout's, which is the first time that has been true. It is a consequence to
write down rather than a bug to fix in `interact`; ADR 0015's "revisit if a geom
appears whose marks are not where its rows are" is the clause it lands under.

**A node reports no row.** A node is what several rows have in common rather
than a row of its own, so a pointer on one leaves `Hit.Row` at −1 rather than
guessing a neighbour. The links are the rows. A treemap and an icicle are the
other way round: every node is a row, and every one of them reports.

**Zoom works and means something.** `scale.Zoomer` on a `[0, 1]` axis pans and
scales the layout; under a polar coord it rotates and scales the ring. That is
free rather than designed.

**These are the second and third marks to refuse a scale.** `geom.Ridgeline`
refuses a continuous axis; these four refuse an ordinal one, with
`geom.ErrNotContinuous`. A mark that places its own geometry needs an axis it
can put a fraction on, and an ordinal scale has slots — so every node would land
in slot zero, drawn on top of each other rather than refused.

## Not in scope

**A node-link / force layout.** It is in bucket E, and it is the one member that
cannot be a pure function of its input in any bounded sweep count that also
looks good — a force simulation's whole method is to run until it settles. ADR
0012 would have to be answered on its own terms first, and it is not answered by
this record.

**Venn and UpSet.** A Venn layout is a circle-packing optimiser with its own
failure modes, and UpSet is a matrix chart rather than a relational layout at
all.

**A size channel.** `geom.SizeBy` is accepted and ignored by all four: a size
per node is meaningless when the value already *is* the size.

**Label de-overlap.** A treemap cell too small for its label drops it. The
de-overlap pass is bucket G's open item ([ADR 0032](0032-text-as-a-mark.md)
deferred it) and stays there.

**Crossing minimisation.** A sankey keeps the node order its rows gave it and
relaxes only their positions. Reordering to reduce crossings means a sort per
sweep, and a sort is where a layout stops being a pure function of its input and
starts depending on how a tie was broken. `geom.Order` is how a caller asks for
a different order — by sorting its own rows.

## Revisit if

- A third chart wants the rim/hub convention with the opposite orientation. A
  positional `scale.Reverse` that survives the round trip would be the answer,
  and it would be a change to `scale` and to the dialect rather than to these
  marks.
- A node-link layout arrives with a bounded, deterministic solver. The node
  table and the five channels here are what it would read.
- Hit values in the unit square turn out to be what people actually want
  inverted. Then ADR 0015's revisit clause is due, not this one.
