# 0028 — A distribution stat runs in `Train`, and decides one of its own axes

**Status:** Accepted · **Date:** 2026-09-05

## Context

`CONCEPT.md` §8 has promised `Bin`, `Density` and `Smooth` since v0.1. `stat/`
carried the decimation family and a two-dimensional density grid and none of the
three, so seven chart forms — histogram, violin, ridgeline, hexbin, beeswarm,
ECDF and trend line — had no spelling at all.

The reduction family already in `stat` answers a question about *drawing*: this
column has more rows than the plot has pixels, so which rows decide what the
reader sees. [ADR 0011](0011-decimation.md) put it in `Build`, in device space,
precisely because a pixel column is the unit it is about — and drew a hard line
saying that what a chart's axis reports must not depend on how wide the chart
is.

A distribution stat is not that. The rows are not too many; they are the wrong
shape. And its output is what the axis has to describe: a histogram's Y axis
holds counts that appear nowhere in the table, an ECDF's holds a fraction it
computed, a trend's fit reaches values no row has. Applying ADR 0011's rule
without thinking would put these in `Build` and leave every one of them drawing
off the top of its own axis.

## Decision

### The stat runs in `Train`, in data space

Every distribution stat is computed in `Geom.Train` and the scales are trained
on its *output*. That is not a contradiction of ADR 0011 but the same principle
applied: what a chart's axis says must not depend on how wide the chart is, and
here the thing the axis has to say is the summary.

It has a second consequence worth stating: a density estimated in device space
would change shape when the window was resized, and a histogram binned there
would have a different number of bars in every panel of a facet.

Two marks are deliberately the exception, and both for the same reason — what
they compute is a length on screen rather than a value:

- **`geom.Hexbin`** bins in `Build`, over the plot rectangle. A hexagon is only
  a hexagon if its six neighbours really are equidistant, and a lattice laid out
  in data space and mapped through the axes comes out stretched by whatever
  aspect ratio the panel happens to have. This is `stat.Grid`'s arrangement
  exactly, and it costs the same thing: the counts are not known when the guides
  are measured, so a hexbin has **no colourbar**. It shades from a faded version
  of its own colour to the full one and answers "how many" through a hit.
- **`geom.Beeswarm`** places its marks in `Build`, because a mark is moved aside
  until it clears its neighbours *by a marker's width*. What the axis says is
  unaffected: the axis was trained on the observations, and rearranging them
  does not move any of them on the value axis.

### The functions stay numbers in, numbers out

`stat` knows about numbers and nothing else. `Bin`, `KDE`, `ECDF`, `Loess` and
`Hex` take slices and return slices, each with an `Append` form that writes into
a caller-owned buffer, and each with a determinism test that runs it twice and
compares. Nothing reads a clock, a map iteration order or `math/rand`: ADR 0012
requires a parallel render to be byte-identical to a serial one, and a beeswarm
that jittered would end that silently.

Two functions take a **sorted** column — `Quantile`, `ECDF` — and `Loess` takes
a sorted, finite pair. That is a narrower contract than the rest of the package
and it is deliberate: sorting means a buffer, a chart redrawn every frame
already keeps one, and doing it inside the function would allocate a copy of the
column per frame to save the caller a loop. The geoms are those callers, and
each of them keeps its sorting buffer on the layer for the reason `barGeom.gaps`
does — `Train` runs outside a `Build`, where there is no scratch to take.

`stat.Bin` is the one-dimensional histogram, because that is what "bin" means
without a qualifier. The two-dimensional binners are named after what they fill:
`BinGrid` (the square lattice that was called `Bin` before v0.9) and `BinHex`.

### The nearest hexagon is measured in real distance, not in lattice units

`Hex.Cell` follows d3-hexbin's structure — round to the nearest row, round to
the nearest column of that row, and refine only in the band where two rows'
cells interlock — with one correction. Both work in lattice coordinates, where a
column step is 1 and a row step is 1; on screen a column step is √3·r and a row
step is 1.5·r, so comparing `px² + py²` there mixes two units and hands some
points to a cell that is not their nearest. The vertical term therefore carries
`(dy/dx)²`, which is exactly ¾ whatever the radius.
`TestEveryHexPointLandsInItsNearestCell` checks the defining property directly —
every point goes to the cell whose centre is nearest — and it fails without the
correction.

### Which axis a mark decides, and which channel it reads

| Mark | Reads | Decides | Composes with `GroupBy` |
|---|---|---|---|
| `Histogram` | X | Y (counts) | no — see below |
| `ECDF` | X | Y (fraction 0..1) | yes, one staircase per series |
| `Violin` | X (slot), Y (values) | the width across the slot | yes, side by side in the slot |
| `Ridgeline` | X (values), Y (rows) | the height out of the slot | the Y column *is* the grouping |
| `Beeswarm` | X (slot), Y (values) | the offset across the slot | yes, side by side in the slot |
| `Hexbin` | X, Y | the shade of each cell | no |
| `Trend` | X, Y | nothing; it adds a fit | yes, one fit per series |

**A histogram ignores `GroupBy` on purpose.** Two distributions drawn as two
overlapping histograms hide each other wherever they agree, which is exactly
where the comparison is. `Violin`, `Ridgeline` and `ECDF` are the three marks
that answer that question without overplotting, and each of them takes the
series column.

**A violin's widths are comparable across groups.** Every estimate integrates to
1 and they are all drawn against the widest of them, so a group with ten times
the rows is not ten times as fat. `geom.Bandwidth` pins the smoothing, which is
what makes two groups strictly comparable; left alone, each group is smoothed by
Silverman's robust rule from its own spread.

## Consequences

- `geom` gains seven marks, `Mark` gains seven names, and the JSON spec gains
  seven mark types with the numbers that configure them. Vega-Lite reaches most
  of this through transforms — `bin`, `density`, `loess`, `regression` — which
  refract runs inside the layer, so the numbers travel with the mark. That is
  [ADR 0014](0014-json-spec.md)'s rule: spell a difference out rather than
  disguise it.
- `stat.Bin` changed meaning. The 2-D binner it named is `stat.BinGrid`. Inside
  the v0.x window that is a rename with a compile error attached, which is the
  good kind.
- `geom.Ridgeline` **errors** when its Y axis cannot name categories, rather
  than drawing every ridge on top of the last. It is the first geom that refuses
  a scale outright; `ErrNotCategorical` is the sentinel.

## Alternatives rejected

**Running the stats in `Build`, like decimation.** Every one of the seven would
then draw outside an axis trained on something else. The rule ADR 0011 states is
about the axis, not about the phase, and following it correctly puts these in
`Train`.

**A `stat.Beeswarm` in the stat package.** The placement is defined in device
units against a marker diameter, so the function would take a length in pixels
and a slot width in pixels and hand back offsets in pixels — which is a geom
wearing a stat's coat. What is genuinely numbers-in-numbers-out here is the
sorting, and `sort.Float64s` already exists.

**A colourbar for the hexbin, by binning in data space so the counts are known
early.** The cells would then be hexagons in data space and something else on
screen, which is the one property the mark exists for.

**Deriving a histogram's bins with Freedman–Diaconis unconditionally.** It needs
an interquartile range, which needs a sort, and it has no answer at all for a
column of mostly one value. `stat.Sturges` needs only the count, so the layer
tries the better rule and falls back — and `FreedmanDiaconis` returns 0 rather
than a guess when it has nothing to say.
