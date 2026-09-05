# Chart types: what exists, what is missing, and what each one costs

refract draws eight data-bearing marks today — `Line`, `Scatter`, `Bar`, `Area`,
`Step`, `Boxplot`, `Rect`, and the annotations in `geom/annotate.go`. This
document is
the catalogue of what it does not draw yet, sorted **by the machinery each form
needs** rather than by how popular it is. Sorted that way the list stops being a
wish list and becomes a schedule: half of these charts share four pieces of
plumbing, and once those exist the charts themselves are small.

The milestone column follows [CONCEPT §14](../CONCEPT.md). Nothing here is a
commitment to draw every form as a named constructor; several are recipes over a
mark that does not exist yet, and the catalogue says which.

## The plumbing, and what it unlocks

| Piece | Status | Unlocks |
|---|---|---|
| A data-driven rectangle mark (`geom.Rect`) | **shipped in v0.7** | heatmap, gantt, candlestick, waterfall, bullet, waffle, calendar |
| Groups in one layer (`geom.GroupBy`) + discrete colour (`scale.Qualitative`) | **shipped in v0.7** — [ADR 0020](adr/0020-discrete-colour-and-multi-entry-legends.md) | every multi-series form; prerequisite for stacking |
| Multi-entry legends (`geom.Legender`) | **shipped in v0.7** — [ADR 0020](adr/0020-discrete-colour-and-multi-entry-legends.md) | pie, stacks, treemap, waffle, sankey |
| Position adjustments (stack / dodge / fill / wiggle) | **shipped in v0.7** — [ADR 0019](adr/0019-position-adjustments.md) | stacked and grouped bars, stacked area, streamgraph, funnel, marimekko, ridgeline, **and pie** |
| Coordinate systems (`coord.Polar`) | **shipped in v0.8** — [ADR 0018](adr/0018-coordinate-systems.md) | pie, donut, radar, rose, wind rose, gauge |
| A size channel (`geom.SizeBy` + a size scale) | missing | bubble |
| Distribution stats (`Bin`, KDE, hexbin, ECDF, loess) | missing | histogram, violin, hexbin, ridgeline, beeswarm, smoothing |
| Relational layouts (squarify, sankey, chord) | missing | treemap, sunburst, sankey, alluvial, chord, arc diagram |

## A — needs a rectangle mark, and nothing else — **shipped in v0.7**

`geom.Rect` occupies an arbitrary `[x0,x1] × [y0,y1]` per row: `geom.X2(col)`
gives the far horizontal edge and `geom.Y2(col)` the far vertical one, and an
edge no column names is the slot the axis implies — a band's own width, or the
closest spacing in the data. `Bar` still grows from a baseline to a value and
`Region` is still an annotation taking four literals; this is the third shape,
and it turns eight charts into recipes.

| Chart | Recipe |
|---|---|
| Heatmap | `Rect` + `ColorBy` over two band scales — see `examples/groups` |
| Calendar heatmap | `Rect` + a date→(week, weekday) helper |
| Gantt / timeline | `Rect` on a time X against an ordinal Y |
| Candlestick / OHLC | `Rect` for open..close, a rule for low..high, colour by sign |
| Waterfall | `Rect` with per-row `y0`/`y1` from a running total, through `Y` and `Y2` |
| Bullet | `Rect` bands, a measure bar and a target rule |
| Waffle | a `Rect` grid from counts |
| Lollipop / Cleveland dot | a rule per row plus markers — its own small geom, reusing `markSpan` |

**The trap, and how it was resolved.** `spec/vocab.go` maps `geom.MarkRegion` to
`("rect", "")` and back, and a data-driven rect collides with that. `geomMark`
is therefore passed the layer's encoding as well as the mark: a rect with a
*field* is data and a rect with a *datum* is an annotation, which is how
Vega-Lite resolves the same ambiguity. `spec`'s round-trip test draws both in
one chart, which is what would catch getting it wrong.

## B — needs a position adjustment — **shipped in v0.7**

See [ADR 0019](adr/0019-position-adjustments.md). Stacked bars, grouped bars,
stacked area, 100 % stacked and streamgraph/ThemeRiver are `geom.GroupBy` plus
`geom.Stack` or `geom.Dodge` over a long table; the streamgraph's baseline is
`stat.StackOffsets` (Byron–Wattenberg), which is numbers in and numbers out.

Three of the family are recipes rather than options, and are worth spelling out.
A **funnel** and a **pyramid** are a silhouette stack over one row per stage —
`geom.Stack(geom.StackSilhouette)` centres each slot, which is the shape both
are. A **marimekko** is `geom.WidthBy` over X positions the caller has already
accumulated: the width option gives a bar its width in the axis's own units and
deliberately does not move the slots, because unequal slots that label
themselves are an axis question rather than an adjustment one. A **ridgeline**
still waits on the KDE in F.

## C — needs polar coordinates — **shipped in v0.8**

See [ADR 0018](adr/0018-coordinate-systems.md). `coord.Polar` wraps one axis
around a circle and reads the other as a radius; nothing in this family is a new
geom, which is why the plumbing in A and B landed first. Each of them needed
stacking (B) and multi-entry legends before the coordinate system was any use —
a pie has N slices inside *one* layer, and one legend entry per layer cannot
name them.

| Chart | Recipe |
|---|---|
| Pie | `Bar` + `GroupBy` in `coord.Polar(coord.Theta(coord.FromY))`, over one X slot |
| Donut | the same, plus `coord.Hole(f)` — the hole is where the radial scale starts |
| Radar / spider | `Line` or `Area` over an ordinal angular axis, with `coord.Chord()` and `geom.Closed(true)` |
| Rose / coxcomb, wind rose | `Bar` with the direction on the angular axis and the count on the radial one |
| Gauge | `Bar` over `coord.Sweep(math.Pi)` — a partial ring — usually with a hole |
| Polar boxplot | `Boxplot`, unchanged: its box goes through the coord like every other rectangle |

Three things are worth knowing before drawing one.

**Do not nice the angular scale.** A pie's ring closes because the stacked
domain ends at the total and starts at zero; a domain rounded up to the next
round number leaves a wedge of nothing at twelve o'clock. `scale.Linear()`
without `scale.Nice()` is the recipe, and `refract.Coord`'s doc comment says so.

**An edge is an arc by default and a chord on request.** That is
`coord.Chord()`, and it is what tells a radar's sides apart from a rose petal's.
Without it a spider chart comes out with bowed sides.

**A pie has no axis worth labelling**, so `theme.Grid(false, false)`,
`theme.AxisLines(false, false)` and `theme.Ticks(false, false)` are the three
switches that turn the furniture off. The third of them is new in v0.8, and
[ADR 0018](adr/0018-coordinate-systems.md) is why: "suppress the furniture" had
no home before the coord gave it one.

## D — needs a new aesthetic channel

**Bubble** needs a size scale. `geom.Size` is a scalar today and
`spec.Encoding` carries `x, y, x2, y2, color, detail` and `width` and nothing
else. The scale must map by **area**, not radius —
doubling a value multiplies the diameter by √2 — because a reader compares
bubbles by how much ink they occupy. It also needs a third guide kind beside the
legend and the colourbar, which is the moment `layout`'s guide column should be
generalised once rather than extended twice.

**Parallel coordinates** needs no new channel but does need per-axis scales
inside one panel, which makes it a near-relative of radar: both draw their own
axes inside the plot area. Radar got its axes from `coord.Polar`'s furniture in
v0.8 — spokes and rings the coord reports and `render` strokes — and parallel
coordinates would want the same shape of answer from a coord of its own rather
than a second one drawn by a geom.

## E — needs a relational layout

Sankey/alluvial, chord, arc diagram, node-link, treemap, sunburst/icicle,
Venn/UpSet.

**The data layer does not change.** An edge list is `StringColumn("from")`,
`StringColumn("to")`, `Float64Column("value")` — three columns, exactly what
`data.Source` already returns. A hierarchy is `(id, parent, value)`, a
self-referential edge table, equally columnar. Widening the `Source` interface
for this would be a breaking change for no gain.

**The layout algorithms belong in `stat`.** A squarified treemap is values plus a
rectangle in, rectangles out. Sankey node placement is an edge list in,
coordinates in the unit square out. A chord layout is a matrix in, arcs and
ribbons out. All of it is numbers in, numbers out, which is exactly what
AGENTS.md scopes `stat` to — "no scales, no theme, no geoms". They return plain
`float64` tuples rather than `ir.Rect`, so `stat` still does not import `ir`.

Two properties these must have, because both are ADR 0012's business: node and
link order comes from first appearance in the source table, never from map
iteration, and any relaxation loop runs a fixed number of sweeps rather than to
convergence — so the result is a pure function of its input and a parallel render
stays byte-identical to a serial one.

## F — needs new stats

`CONCEPT §8` has promised `Bin`, `Density` and `Smooth` since v0.1; `stat/`
carries the decimation family and a 2-D density grid, and none of the three.

| Chart | Stat |
|---|---|
| Histogram | `stat.Bin` — a 1-D binner. `stat.Bin` today bins into the 2-D density `Grid` (`stat/density.go:107`) and is a different function |
| Violin | `stat.KDE` + `stat.Bandwidth` (Silverman) |
| Ridgeline | KDE + groups (B) + a per-group offset |
| Hexbin | `stat.Hexbin`, a hex lattice beside `stat.Grid` |
| Beeswarm | `stat.Beeswarm` — a deterministic 1-D dodge, and **no `math/rand`** |
| ECDF / QQ | `stat.ECDF` |
| Trend line | `stat.Loess` |
| Contour | `stat.Contour` |

Every one of these is a pure function with an `Append` form, per CONTRIBUTING's
rule for reductions, and every one needs a determinism test in the shape of
`TestDecimationIsAPureFunctionOfItsInput`.

## Already possible today

Worth saying plainly, because they look like gaps and are not: a **band /
uncertainty ribbon** is `Area` with `Y2`; a **step chart** is `Step`; a
**density cloud** over a million points is `Scatter` with
`geom.Decimate(geom.DensityRaster)`; **reference lines, spans, regions and
callouts** are the annotations in `geom/annotate.go`; a **slope chart** is a
line over two ordinal positions. What these lack is gallery figures, not code.

## Sequence

The dependency order is not a preference:

1. ~~**`geom.Rect`**~~ — shipped in v0.7; unlocked eight charts (A).
2. ~~**`GroupBy` + discrete colour + `Legender`**~~ — shipped in v0.7; the
   keystone nothing after it works without
   ([ADR 0020](adr/0020-discrete-colour-and-multi-entry-legends.md)).
3. ~~**Position adjustments**~~ — shipped in v0.7
   ([ADR 0019](adr/0019-position-adjustments.md)): B, and the second half of
   what a pie needs.
4. **`coord/`** ([ADR 0018](adr/0018-coordinate-systems.md)) — C, and the reason
   the whole sequence has to finish before the v1.0 API freeze.
5. **The size channel** — D.
6. **The stat family** — F, large by count but each item small and independent.
7. **Relational layouts** — E, the only bucket that shares nothing with the
   others and therefore the only one that can be moved without cost.

**Sankey deliberately sits last.** It is the single most-requested form in this
catalogue that benefits from none of the plumbing above: its own data shape, its
own solver, its own legend, its own hit-testing. Pulling it forward would delay
the four pieces that unlock everything else.
