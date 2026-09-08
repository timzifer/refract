# Chart types: what exists, what is missing, and what each one costs

refract draws twenty data-bearing marks today — `Line`, `Scatter`, `Bar`,
`Area`, `Step`, `Boxplot`, `Rect`, `Text`, `ErrorBar`, `Histogram`, `Violin`,
`Ridgeline`, `Hexbin`, `Beeswarm`, `ECDF`, `Trend`, `Treemap`, `Icicle`,
`Sankey` and `Arc`, plus the annotations in `geom/annotate.go`. This document is the catalogue of what it does not draw
yet, sorted **by the machinery each form needs** rather than by how popular it
is. Sorted that way the list stops being a wish list and becomes a schedule:
half of these charts share four pieces of plumbing, and once those exist the
charts themselves are small.

The milestone column follows [CONCEPT §14](../CONCEPT.md). Nothing here is a
commitment to draw every form as a named constructor; several are recipes over a
mark that does not exist yet, and the catalogue says which.

**Buckets A through H have shipped. I through M are planned and have records but
no code** — [ADR 0049](adr/0049-locus-annotations.md) through
[ADR 0053](adr/0053-statistical-instruments.md). They are written down early
because three of them answer a question an earlier record explicitly left open,
and a question answered in a conversation rather than in the repository gets
answered again, differently, later. Each is additive and nothing in v1.7 waits
on any of them.

## The plumbing, and what it unlocks

| Piece | Status | Unlocks |
|---|---|---|
| A data-driven rectangle mark (`geom.Rect`) | **shipped in v0.7** | heatmap, gantt, candlestick, waterfall, bullet, waffle, calendar |
| Groups in one layer (`geom.GroupBy`) + discrete colour (`scale.Qualitative`) | **shipped in v0.7** — [ADR 0020](adr/0020-discrete-colour-and-multi-entry-legends.md) | every multi-series form; prerequisite for stacking |
| Multi-entry legends (`geom.Legender`) | **shipped in v0.7** — [ADR 0020](adr/0020-discrete-colour-and-multi-entry-legends.md) | pie, stacks, treemap, waffle, sankey |
| Position adjustments (stack / dodge / fill / wiggle) | **shipped in v0.7** — [ADR 0019](adr/0019-position-adjustments.md) | stacked and grouped bars, stacked area, streamgraph, funnel, marimekko, ridgeline, **and pie** |
| Coordinate systems (`coord.Polar`) | **shipped in v0.8** — [ADR 0018](adr/0018-coordinate-systems.md) | pie, donut, radar, rose, wind rose, gauge |
| A band at a panel's edge on a shared axis (`Plot.Track`) | **shipped in v0.10** — [ADR 0031](adr/0031-tracks.md) | gantt strip under a trace, rug plot, event ribbon, sparkline gutter, shift bands; beside it, colour keys and marginal distributions |
| A size channel (`geom.SizeBy` + a size scale) | **shipped in v0.9** — [ADR 0027](adr/0027-size-channel-and-the-guide-column.md) | bubble |
| Distribution stats (`Bin`, KDE, hexbin, ECDF, loess) | **shipped in v0.9** — [ADR 0028](adr/0028-distribution-stats.md) | histogram, violin, hexbin, ridgeline, beeswarm, smoothing |
| A Smith coordinate system (`coord.Smith`) + pinned ticks (`scale.TickValues`) | **shipped in v1.2** — [ADR 0033](adr/0033-smith-charts.md) | Smith chart, admittance (Y) chart, matching-network locus, impedance region |
| Relational layouts (squarify, sankey, chord) | **shipped in v1.4** — [ADR 0039](adr/0039-relational-layouts.md) | treemap, icicle, sunburst, flame graph, sankey, alluvial, chord, arc diagram |
| A locus: a family of curves given by a formula (`geom.Locus`) | **planned** — [ADR 0049](adr/0049-locus-annotations.md) | Nichols, VSWR circles, constant-Q arcs, the ZY overlay, Hall chart, funnel-plot contours |
| A barycentric coord (`coord.Ternary`) | **planned** — [ADR 0050](adr/0050-barycentric-coord.md) | ternary plots, QFL and QAP diagrams, the soil texture triangle, phase and flammability diagrams, Piper |
| A probability scale (`scale.Probability`) | **planned** — [ADR 0051](adr/0051-probability-scales.md) | Weibull, normal and Gumbel probability paper, hazard plots, a log-odds axis |
| A deterministic tree layout (`stat.Tidy`) | **planned** — [ADR 0052](adr/0052-tidy-tree-layout.md) | dendrogram, phylogram, radial dendrogram, org and decision trees, clustered heatmap |
| Domain reductions in `stat` | **planned** — [ADR 0053](adr/0053-statistical-instruments.md) | survival curves, the SPC family, correlograms, ROC and PR curves, Lorenz |

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

## E — needs a relational layout — **shipped in v1.4**, except force-directed node-link and Venn

Sankey/alluvial, chord, arc diagram, node-link, treemap, sunburst/icicle,
Venn/UpSet.

The last bucket, and the one `CONCEPT §14` called the only family that shares no
machinery with the rest. Three of the four things it was said to need — its own
data shape, its own solver, its own legend, its own hit-testing — turned out to
be two: the legend and the hit test were already general enough, which is what
[ADR 0039](adr/0039-relational-layouts.md) records.

| Chart | Mark | Coord |
|---|---|---|
| Treemap | `geom.Treemap` | Cartesian |
| Icicle, flame graph | `geom.Icicle` | Cartesian |
| **Sunburst** | `geom.Icicle` | `coord.Polar()` |
| Sankey, alluvial | `geom.Sankey` | Cartesian |
| Arc diagram | `geom.Arc` | Cartesian |
| **Chord diagram** | `geom.Arc` | `coord.Polar()` + `geom.Baseline(1)` |
| Node-link, force-directed | missing | — |
| Node-link, tree-shaped | `geom.Tree` — planned, [ADR 0052](adr/0052-tidy-tree-layout.md) | Cartesian |
| **Radial dendrogram** | `geom.Tree` — planned | `coord.Polar()` |
| Venn / UpSet | missing | — |

**Four marks, six charts.** Every layout here fills the unit square — a span
across, a height out — and the coordinate stage decides what that looks like.
That is the v0.8 move made twice: an icicle wrapped round a circle is a
sunburst, and an arc diagram with its rail at the rim is a chord diagram.
Neither is a mark of its own, for the same reason a pie is not a second
implementation of a bar.

**The data layer did not change.** An edge list is `StringColumn("from")`,
`StringColumn("to")`, `Float64Column("value")` — three columns, exactly what
`data.Source` already returns. A hierarchy is `(id, parent, value)`, a
self-referential edge table, equally columnar. What was added is five *channels*
— `geom.From`, `geom.To`, `geom.ID`, `geom.Parent`, `geom.Value` — and the two
pairs are spelled apart because a hierarchy's edge runs from the child to its
parent and a flow's from source to target.

**The layout algorithms are in `stat`.** `Depth`, `Rollup` and `Partition` for a
hierarchy, `Squarify` for a treemap's packing, and `Sankey` and `Chord` as
structs with a `Reset`, the way `Hex` is, because a flow layout keeps a cursor
per node and a chart redrawn every frame should reuse it. All of it is numbers
in, numbers out — none of them ever sees a string, because interning a name is
where the order is decided and that belongs in the geom.

Both of ADR 0012's properties hold and are tested: node and link order comes
from first appearance in the source table, never from map iteration, and the
sankey's relaxation runs `stat.SankeySweeps` sweeps rather than to convergence.

**What is still missing, and why.** A *force-directed* node-link layout is a
simulation whose whole method is to run until it settles — so it cannot be a
pure function of its input at a bounded sweep count that also looks good, and
ADR 0012 has to be answered on its own terms before it lands. Venn is a
circle-packing optimiser, and UpSet is a matrix chart rather than a relational
layout at all.

**That sentence was one size too large, and bucket L is the correction.** It
binds force layouts and not tree layouts: Reingold–Tilford, in Buchheim's
linear-time form, is O(n), deterministic and bounded, which is `stat.Squarify`'s
shape exactly. See [ADR 0052](adr/0052-tidy-tree-layout.md).

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
| QQ | `stat.QQ` with a theoretical quantile function | `geom.QQ` for normal quantiles; unreleased |
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

## G — needs a Smith coordinate system — **shipped in v1.2**

See [ADR 0033](adr/0033-smith-charts.md). A Smith chart is a conformal map of
the impedance half-plane onto the unit disc, Γ = (z−1)/(z+1). It is the one form
in this catalogue that no general-purpose library draws, and it needed exactly
one piece of plumbing — a coord — because everything else it wants shipped
between v0.1 and v0.9.

| Chart | Recipe |
|---|---|
| Smith chart | `Line` over two columns holding r = R/Z₀ and x = X/Z₀, in `coord.Smith` |
| Measured sweep (S₁₁) | the same, with `coord.SmithZ` converting Γ into the pair — see `examples/smith` |
| Matching-network locus | `Line` + `coord.SmithArc`, whose steps are straight in impedance and therefore arcs on the disc |
| Admittance (Y) chart | `coord.SmithAdmittance(true)`, with the columns holding g and b |
| An impedance tolerance region | `Rect` or a `Region` annotation, whose cell is curvilinear here |

**Three things worth knowing.** The columns are an **impedance**, not a
reflection coefficient, and that is forced rather than chosen: `render` takes a
grid line's geometry from the coord and its label from the scale's own tick, so
a coord may draw one grid line per tick and label nothing the scale did not — so
the two tick families have to be the two things the reader wants labelled, r and
x. Both axes are **linear** and their domains are **pinned**, because the
chart's extent is the whole disc whatever the data does. And an edge is a
**chord** by default: a line between two measured samples asserting a linear
sweep in impedance is an assertion the instrument did not make.

**Not drawn, and for one reason.** Constant-|Γ| (VSWR) circles, constant-Q arcs
and a combined ZY overlay are each a third grid family, and there are two tick
lists. That is the same constraint that chose the data model, and the two would
be reopened together.

**Bucket I is the answer, and it is not the one ADR 0033 predicted.** All three
are curves given by a formula in impedance space rather than grid lines given by
a tick, so they are annotations, and the Smith coord draws them without knowing
they exist. See [ADR 0049](adr/0049-locus-annotations.md).

## H — what is not a chart type

The forms above are shapes. This bucket is the other kind of gap: things a
chart says that no mark draws, and that were missing for long enough to be
worth naming as a class. They share no machinery with each other either, but
each of them is small, and each of them was reachable only by giving up
something else.

| Gap | Status | What it was |
|---|---|---|
| Two quantities in different units | **shipped** — [ADR 0037](adr/0037-secondary-axis.md) | `Plot.Y2` and `geom.OnY2`. A plot had one Y scale and a layer no way to name another, so revenue-and-margin — bars against the left axis, a percentage against the right — could not be drawn at all; normalising into the primary axis's units draws it and makes the axis, the zoom and the tooltip all read in units nobody measured. |
| One reading with two rulers | **shipped** — [ADR 0037](adr/0037-secondary-axis.md#amendment-the-horizontal-direction) | `Plot.X2` and `geom.OnX2`, the same machinery a quarter turn round. An oven curve the operator counts in cycles and the engineer counts in minutes is one series and two ladders; an axis with no layer on it is still drawn, so that shape needs no second layer at all. |
| An interval around a measurement | **shipped** — [ADR 0036](adr/0036-error-bars.md) | `geom.ErrorBar`. Every chart of a mean, a forecast or a tolerance has one number and a claim about how well it is known, and the second half had nowhere to go: a band through `Area` is the continuous version and is wrong for three categories. |
| A tick label a document can choose | **shipped** — [ADR 0035](adr/0035-label-format-and-locale.md) | `scale.NumberFormat` and `scale.TimeLayout`. `scale.Format` takes a Go function, so a chart authored as JSON could not set a thousands separator, a currency or a decimal place at all. |
| A chart in a language | **shipped** — [ADR 0035](adr/0035-label-format-and-locale.md) | `scale.Locale` and `refract.Locale`. The time ladder rendered through Go's English tables and `strconv` writes a decimal point; for a German reader the second is not foreign but wrong. |
| A PDF in a script WinAnsi cannot hold | **shipped** — [ADR 0038](adr/0038-embedded-fonts.md) | `pdf.WithFont`. The PDF emitter named the base-14 Helvetica and encoded WinAnsi, so every rune outside Latin-1 became `?` — Greek, Cyrillic, Hebrew, Thai and every CJK script, in the format people send to customers. |
| Absence in a text or temporal column | **shipped** — [ADR 0034](adr/0034-null-values.md) | `data.Nulls`. A null read back as `""` was a band of its own on an ordinal axis and one read back as the zero time stretched a domain across two millennia. |

## I — needs a locus — **planned**, [ADR 0049](adr/0049-locus-annotations.md)

A **locus** is a family of curves given by a formula rather than by data: the
set of points in the plane where some derived quantity is constant. `geom.HLine`
is the degenerate member of the family and has been there since v0.1.

The bucket exists because four charts wanted the same thing and each was
individually too small to build machinery for. It is an annotation and not
furniture — `render` still walks two tick lists and still labels nothing a
scale did not write — and because a locus is defined in **data space**, the
coordinate stage draws it. That is the v0.8 move again: a VSWR circle is not
implemented as a circle, it is implemented as the set of impedances whose
reflection has a given magnitude, and `coord.Smith` makes it a circle.

| Chart | The family | Coord |
|---|---|---|
| **Nichols diagram** | closed-loop magnitude and phase, `stat.NicholsM` / `stat.NicholsN` | Cartesian — the response itself is `Line` and needs nothing |
| VSWR circles | constant \|Γ\| | `coord.Smith` |
| Constant-Q arcs | \|x\| = Q·r | `coord.Smith` |
| ZY overlay | the impedance families read through y = 1/z | `coord.Smith` |
| Hall chart | the same two circle families as Nichols, before the log-polar step | Cartesian or `coord.Polar` |
| Funnel plot contours | pseudo-confidence limits in (effect, standard error) | Cartesian |
| Psychrometric, Mollier | constant enthalpy, wet-bulb, relative humidity | Cartesian |

**The Nichols diagram is the one to build it for.** MATLAB's Control System
Toolbox draws it and `python-control` draws it; outside those two the form does
not exist, and Go has nothing. The arithmetic is smaller than the picture
suggests: both contour families are circles in the complex L-plane, and the
chart is that plane in log-polar view.

## J — needs a barycentric coord — **planned**, [ADR 0050](adr/0050-barycentric-coord.md)

Three components that sum to a constant, read as one point in a triangle. It is
the second-most-common coordinate system in the physical sciences after polar
and the tooling for it is thin everywhere: Plotly has it, R needs `ggtern`,
Python needs `python-ternary`, D3 and Vega-Lite have nothing, **Go has nothing
at all**.

`coord.Ternary` reads X and Y as two components and derives the third, so the
constraint holds by construction and `scale.Scale` is untouched. The map is
**affine**, which makes it the cheapest coord in the package — `Straight()` is
true, an edge is a `LineTo`, `Area` is four transformed corners, `Invert` is a
2×2 matrix.

| Chart | Recipe |
|---|---|
| Ternary scatter, line, path | `Scatter` or `Line` in `coord.Ternary()` |
| Ternary density | `Rect` over binned compositions — a parallelogram per cell, correctly |
| QFL, QAP, soil texture triangle | the same, with the field's own corner labels |
| Phase and flammability diagrams | the same, plus `Region` and `geom.Locus` for the boundaries |
| Probability simplex | the same, over three class probabilities |
| Piper diagram | two ternary panels and one Cartesian panel in a `Grid`, plus the projection arithmetic |

**The third grid family is the interesting part.** Three labelled ladders, two
tick lists. The constant-c lines are drawn as a second subpath inside the X
ticks' own shapes, so they cost `Furniture` nothing; their *labels* are what is
missing, and that is deliberately left as the case that would reopen ADR 0033's
seam — once, together with a projection's graticule, rather than twice.

## K — needs a probability scale — **planned**, [ADR 0051](adr/0051-probability-scales.md)

Probability paper: an axis warped so that one distribution's cumulative
function plots as a straight line, and the line's slope and intercept are the
fitted parameters. `geom.QQ` from v1.5 is the same information the other way
round — it warps the sample and leaves the axis linear, so its ladder is
labelled in z-scores; this warps the axis, so the ladder is labelled in
percentages, which is what the reader came for.

**Every chart in this bucket is `geom.ECDF` on a warped axis. There is no new
mark.**

| Chart | Axis | Field |
|---|---|---|
| Normal probability plot | `scale.Probit` on Y | metrology, quality, psychometrics |
| **Weibull plot** | `scale.CLogLog` on Y, log on X | reliability engineering |
| Gumbel / extreme-value paper | `scale.Gumbel` on Y | hydrology, structural loads |
| Lognormal probability plot | `scale.Probit` on Y, log on X | particle sizing, dose–response |
| Log-odds axis | `scale.Logit` | epidemiology |

Weibull analysis is the load-bearing one: the slope is the shape parameter β,
which says whether failures are infant mortality, random or wear-out. The field
has dedicated commercial software and no general-purpose library.

## L — needs a deterministic tree layout — **planned**, [ADR 0052](adr/0052-tidy-tree-layout.md)

Bucket E declined node-link layouts because a force simulation's whole method
is to run until it settles. A **tidy tree** is not a force simulation:
Reingold–Tilford, in Buchheim's linear-time form, is O(n), deterministic,
bounded and a pure function of its input — `stat.Squarify`'s shape exactly.

`geom.Tree` reads bucket E's own channels — `ID`, `Parent`, `Value` — and adds
none, which is what [ADR 0039](adr/0039-relational-layouts.md)'s revisit clause
predicted a node-link layout would read.

| Chart | Mark | Coord |
|---|---|---|
| Dendrogram, phylogram, cladogram | `geom.Tree` with `Value` as the merge height | Cartesian |
| Org chart, decision tree, file tree | `geom.Tree` with the depth as the height | Cartesian |
| **Radial dendrogram** | `geom.Tree` | `coord.Polar()` |
| **Clustered heatmap** | `Rect` plus a `geom.Tree` in a `Plot.Track` on two edges | Cartesian |

The last one is the reason to build it. It is the most-published figure shape
in bioinformatics, it needs a rectangle, a colour ramp, an ordinal axis and a
band at a panel's edge — all four of which shipped by v0.10 — and it has been
one missing band's worth of content away ever since.

## M — needs a domain reduction — **planned**, [ADR 0053](adr/0053-statistical-instruments.md)

The other buckets are missing a shape. This one is missing only **arithmetic**:
every chart in it is drawable with marks that shipped by v0.10, and none of them
can be drawn because refract does not hold the numbers.

The admission rule, which is what the record is really for: *a reduction belongs
in `stat` when its output is the chart's geometry, and there is no reading of it
that is not the chart.*

| Chart | Stat | Mark |
|---|---|---|
| Kaplan–Meier survival curve | `stat.KaplanMeier` + Greenwood | `geom.Survival`, with the risk table as a `Track` |
| SPC: X̄-R, I-MR, p, np, c, u | `stat.ControlLimits` + the Nelson rules | `Line`, `Scatter`, `HLine` — **no mark**, because limits come from a baseline period and not from the plotted points |
| Correlogram | `stat.ACF`, `stat.PACF` | `Bar` + `HLine` |
| ROC, precision–recall | `stat.ROC` | `Line` |
| Lorenz curve, Gini | `stat.Lorenz` | `Line` + `Segment` |

**Four more are recipes and are named here so the catalogue stops calling them
missing.** A **forest plot** is `ErrorBar` + `Text` + a `Track` (the pooled
estimate is a meta-analysis and is the caller's); a **funnel plot** is a scatter
plus bucket I's contours; a **Pareto chart** is sorted bars with the cumulative
percentage on the secondary axis v1.3 shipped; **Bland–Altman** is a scatter and
three reference lines.

## Already possible today

Worth saying plainly, because they look like gaps and are not: a **band /
uncertainty ribbon** is `Area` with `Y2` (and the discrete version of the same
statement is `ErrorBar`); a **step chart** is `Step`; a
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
8. ~~**A Smith coord**~~ — shipped in v1.2
   ([ADR 0033](adr/0033-smith-charts.md)): G, on the seam v0.8 already cut.
9. ~~**Bucket H**~~ — shipped: the gaps that are not chart types at all. It is
   listed last in this order and first in nothing, because sorting by
   machinery is what makes a schedule and these have none — each is small,
   independent, and was blocking a whole class of charts from being *usable*
   rather than from being drawn. The bucket is now empty: the last of it
   was an **overlay layer the chart itself owns** — a tooltip, a crosshair, a
   brush rectangle — which `interact` cannot draw because it only reads, and
   which shipped in v1.8 as a stage in `render`, the one package that knows
   drawing order ([ADR 0046](adr/0046-overlay-layer.md)). It is drawn last and
   announced to no observer, so what it draws is not hit-testable: a tooltip a
   pointer can hit is a tooltip that flickers. **Label collision
   avoidance** is implemented for the next release through
   `geom.AvoidOverlap(true)` ([ADR 0040](adr/0040-label-collision-avoidance.md)).
10. ~~**Relational layouts**~~ — E, shipped in v1.4
   ([ADR 0039](adr/0039-relational-layouts.md)): the only bucket that shared
   nothing with the others, and therefore the only one that could be moved
   without cost.

The five that follow are planned. They are listed in the order their records
argue for, which is again a dependency order rather than a preference — the
first is the only one anything else waits on.

11. **A locus** — I ([ADR 0049](adr/0049-locus-annotations.md)). First, because
   it is the only one of the five with a dependent: bucket J keeps it as the
   escape hatch for a fourth grid family, and bucket M's funnel plot is a
   scatter plus one. It also closes three lines ADR 0033 left open, which no
   other work will close.
12. **A barycentric coord** — J ([ADR 0050](adr/0050-barycentric-coord.md)). The
   widest genuine gap in the general-purpose world with a real user base, on the
   seam v0.8 already cut, and the cheapest coord in the package because the map
   is affine.
13. **A probability scale** — K ([ADR 0051](adr/0051-probability-scales.md)).
   The smallest diff in this list and the one with the rarest output: five
   charts and no new mark, because every one of them is `geom.ECDF` on a warped
   axis.
14. **A tree layout** — L ([ADR 0052](adr/0052-tidy-tree-layout.md)). One mark,
   four charts, and it makes bucket E's "node-link is missing" an honest
   sentence instead of an over-broad one.
15. **Domain reductions** — M ([ADR 0053](adr/0053-statistical-instruments.md)).
   Last, and deliberately: it is the widest reach in the catalogue and the least
   architecture, so nothing waits on it and it costs nothing to defer. Most of
   the work in it is documentation.

**Sankey deliberately sat last, and the order was right.** It was the single
most-requested form in this catalogue that benefits from none of the plumbing
above, and pulling it forward would have delayed the four pieces that unlock
everything else. What the wait bought is visible in the diff: the multi-entry
legend of v0.7, the coordinate stage of v0.8 and the subpath-per-mark hit test
of v0.5 were all already general enough, so two of the four things this bucket
was said to need turned out to need nothing at all — and the two recipes,
sunburst and chord, cost no code whatsoever.
