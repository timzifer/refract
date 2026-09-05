# Chart types: what exists, what is missing, and what each one costs

refract draws fifteen data-bearing marks today — `Line`, `Scatter`, `Bar`,
`Area`, `Step`, `Boxplot`, `Rect`, `Histogram`, `Violin`, `Ridgeline`, `Hexbin`,
`Beeswarm`, `ECDF` and `Trend`, plus the annotations in `geom/annotate.go`. This
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
| A size channel (`geom.SizeBy` + a size scale) | **shipped in v0.9** — [ADR 0027](adr/0027-size-channel-and-the-guide-column.md) | bubble |
| Distribution stats (`Bin`, KDE, hexbin, ECDF, loess) | **shipped in v0.9** — [ADR 0028](adr/0028-distribution-stats.md) | histogram, violin, hexbin, ridgeline, beeswarm, smoothing |
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
| Pie | `Bar` + `GroupBy` in `coord.Pie()` — `coord.Polar(coord.Theta(coord.FromY))` — over one X slot |
| Donut | the same, plus `coord.Hole(f)`, which `coord.Donut(f)` is sugar for — the hole is where the radial scale starts |
| Donut with a second measure | the same, plus `geom.X("floor")` and `geom.X2("reach")`: a slice's inner and outer radius are columns, so how far it reaches is a reading |
| Exploded pie or donut | the same, plus `geom.ExplodeBy(col)` — one slice leaves the ring along its own bisector, and `geom.Explode(f)` moves them all |
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

**A slice's radial edges are dimensions, and its break-out is not.** `X` and
`X2` name where a slice starts and stops, so a donut can carry a second measure
against the rim — which is the honest version of the chart an exploded pie
usually fakes by growing a slice. The break-out itself moves the mark and
changes nothing it says, which is why it is a displacement the coord computes
rather than a radius the geom adds
([ADR 0026](adr/0026-breaking-a-mark-out.md)).

**A pie has no axis worth labelling**, so `theme.Grid(false, false)`,
`theme.AxisLines(false, false)` and `theme.Ticks(false, false)` are the three
switches that turn the furniture off. The third of them is new in v0.8, and
[ADR 0018](adr/0018-coordinate-systems.md) is why: "suppress the furniture" had
no home before the coord gave it one.

## D — needs a new aesthetic channel — **shipped in v0.9**

**Bubble** is `geom.SizeBy(col, scale.Size())`. The scale maps by **area**, not
radius — doubling a value multiplies the diameter by √2 — because a reader
compares bubbles by how much ink they occupy, and there is a test asserting that
for every pair in the domain rather than only for the extremes. The layer
contributes a third guide kind beside the legend and the colourbar, and
`layout`'s guide column was generalised once rather than extended twice:
`layout.Guide` carries what the solver needs of any kind, `GridResult.Guides` is
the boxes, and a fourth kind is a constant and two functions
([ADR 0027](adr/0027-size-channel-and-the-guide-column.md)).

**Two things fell out of it that were not asked for.** A sized layer draws
circles as subpaths of one path per colour rather than markers, because
`ir.Backend.Markers` carries one style per call — so a bubble chart is one
drawing call per colour, and a pointer lands on the bubble it is inside rather
than on the nearest centre. And the size scale's *range* belongs to the theme
rather than to the chart: `render` sets it while it collects the guides, because
panels are built concurrently and a geom writing a shared scale in `Build` would
be a data race.

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

## F — needs new stats — **shipped in v0.9**, except contour

`CONCEPT §8` promised `Bin`, `Density` and `Smooth` from v0.1 and `stat/` did not
carry them until v0.9. It does now, and each is a pure function with an `Append`
form and a determinism test, per CONTRIBUTING's rule for reductions.

| Chart | Stat | Mark |
|---|---|---|
| Histogram | `stat.Bin`, with `stat.Sturges` and `stat.FreedmanDiaconis` | `geom.Histogram` |
| Violin | `stat.KDE` + `stat.Silverman` | `geom.Violin` |
| Ridgeline | KDE + a categorical axis + a per-row offset | `geom.Ridgeline` |
| Hexbin | `stat.Hex` and `stat.BinHex`, a hex lattice beside `stat.Grid` | `geom.Hexbin` |
| Beeswarm | a deterministic 1-D dodge in the geom, and **no `math/rand`** | `geom.Beeswarm` |
| ECDF | `stat.ECDF` | `geom.ECDF` |
| Trend line | `stat.Loess` | `geom.Trend` |
| QQ | `stat.ECDF` against a theoretical quantile function | missing |
| Contour | `stat.Contour` | missing |

**`stat.Bin` changed meaning.** It is the 1-D histogram now, because that is what
"bin" means without a qualifier; the 2-D binner it used to name is
`stat.BinGrid`, beside `stat.BinHex`. The three are named after what they fill.

**Where a stat runs is the decision that took the argument**
([ADR 0028](adr/0028-distribution-stats.md)). ADR 0011 puts decimation in
`Build`, in device space, so that what a chart's axis reports does not depend on
how wide the chart is. A distribution stat is the same rule pointing the other
way — its output *is* what the axis describes — so it runs in `Train`. Two marks
are the exception because what they compute is a length on screen: a hexbin bins
over the plot rectangle, and a beeswarm places its marks against a marker
diameter.

**The beeswarm's offsets stayed out of `stat`.** They are defined in device units
against a marker width, so the function would take pixels and return pixels,
which is a geom wearing a stat's coat. What is genuinely numbers-in-numbers-out
there is the sorting, and `sort.Float64s` already exists.

**`stat` does no sorting.** `Quantile`, `ECDF` and `Loess` take ordered columns
and `Silverman` takes two spread measures, because sorting means a buffer and the
geoms already keep one — one per layer, for the reason `barGeom.gaps` is on the
layer rather than in the frame's pool.

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
4. ~~**Position adjustments**~~ — see 3.
5. ~~**`coord/`**~~ — shipped in v0.8
   ([ADR 0018](adr/0018-coordinate-systems.md)): C.
6. ~~**The size channel**~~ — shipped in v0.9
   ([ADR 0027](adr/0027-size-channel-and-the-guide-column.md)): D, and the guide
   column generalised.
7. ~~**The stat family**~~ — shipped in v0.9
   ([ADR 0028](adr/0028-distribution-stats.md)): F, less contour and QQ.
8. **Relational layouts** — E, the only bucket that shares nothing with the
   others and therefore the only one that can be moved without cost.

**Sankey deliberately sits last.** It is the single most-requested form in this
catalogue that benefits from none of the plumbing above: its own data shape, its
own solver, its own legend, its own hit-testing. Pulling it forward would delay
the four pieces that unlock everything else.
