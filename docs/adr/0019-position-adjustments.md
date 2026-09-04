# 0019 — Stacking is a position adjustment within a layer, derived in `Train`

**Status:** Accepted · **Date:** 2026-09-04

## Context

refract cannot stack. There is no `stack`, no `dodge`, no `fill`: a `Bar` grows
from a baseline to its value (`geom/bar.go:102`) and two `Bar` layers overplot
rather than sit on one another. That single gap blocks more chart types than any
other in the library — stacked bars, grouped bars, stacked areas, 100 % bars,
streamgraphs, funnels, pyramids, marimekko/mosaic, ridgelines — and it blocks
the first batch of [ADR 0018](0018-coordinate-systems.md), because a pie is a
stacked bar whose angular axis runs from zero to the total.

Two questions have to be answered together, because the wrong answer to either
makes the other unimplementable.

## Decision

### Stacking is *within* a layer, over a group column

One `Bar` layer plus `geom.GroupBy("series")` plus `geom.Stack(...)`, not three
`Bar` layers that discover each other.

Across-layer stacking would need a coordinator sitting above the geoms, and a
geom would have to be told about its siblings — which crosses the layer
discipline AGENTS.md draws, and puts a new stage between training and layout
that `render.Draw` has no place for. Within-layer stacking needs neither: a long
table with a group column is what streamgraph and marimekko require anyway,
since both are inherently one series family rather than several independent
layers.

This makes `geom.GroupBy` a prerequisite rather than a convenience, and it drags
in [ADR 0020](0020-discrete-colour-and-multi-entry-legends.md) with it: N groups
in one layer need N colours and N legend entries.

### The offsets are derived in `Train`; the geometry is built in `Build`

The split is exact, and both halves are forced.

**In `Train`, because the axis must describe what will be drawn.** `render.Draw`
trains every layer before it measures anything (`render/render.go:117-125`),
because tick labels need a domain and layout needs tick labels. A stacked bar
reaches the cumulative total, so the Y scale has to be trained on the totals, not
on the individual values — otherwise the tallest stack runs off the top of an
axis that describes rows nobody can see. There is precedent in the codebase and
it is exact: `geom.Boxplot` computes its summary inside `Train`
(`geom/boxplot.go:53`) and trains the scales on the derived quantiles rather than
the raw column.

**In `Build`, because that is where geometry lives.** Nothing about the *shape*
of a segment is computed early. This is the same boundary ADR 0011 draws for
decimation — reduce at draw time, train on everything — and it is drawn here for
the same reason: what a chart's axis says must not depend on how wide the chart
is.

So a stacking layer holds, after `Train`, a per-row `(lo, hi)` pair and an
ordered group list; `Build` maps those through the scales and the coord and emits
one path per group.

### The API

```go
geom.GroupBy(col string)        // the series column: also what ADR 0020 colours by
geom.Stack(geom.StackZero | geom.StackFill | geom.StackSilhouette | geom.StackWiggle)
geom.Dodge(padding float64)     // side by side inside the slot
geom.Order(geom.OrderAppearance | geom.OrderValue | geom.OrderInsideOut)
geom.WidthBy(col string)        // marimekko: the slot itself carries a value
```

`StackSilhouette` and `StackWiggle` are what make a streamgraph a streamgraph;
the offset computation (Byron–Wattenberg) is numbers in, numbers out, so it goes
to `stat.StackOffsets` and not into a geom — AGENTS.md: "`stat` knows about
numbers and nothing else."

Group order is **order of first appearance in the source table**, never map
iteration order. `geom.groupByColor` and `boxplot.summarise` already establish
that convention, and ADR 0012 is why it is not negotiable: a parallel render must
be byte-identical to a serial one, and an order that depends on hashing is an
order that depends on scheduling.

## Consequences

- `geom.config` gains the group, stack, order and width fields; `geom.Desc` gains
  them too, `config.describe` fills them, and `spec.writeMarkProps` decides which
  marks write them. AGENTS.md is explicit that forgetting the last one is how the
  JSON spec silently drops an option, and the round-trip test is what catches it.
- **Decimation of a stacked layer is all-or-nothing.** Reducing each group
  independently gives each its own keep-set, and the segment boundaries then no
  longer line up — a stacked area would tear along every seam. A stacked layer
  either reduces as a unit against one shared keep-set or does not reduce; bars
  already do not reduce at all (`geom/bar.go:92-94`), because dropping a bar drops
  a category rather than a pixel.
- **A hole is a hole, per the layer's own policy.** `scratch.plottable` is
  computed once and both the cumulative sum and the drawing traversal read the
  same answer. AGENTS.md warns about exactly this: "the traversals have to agree
  about where the holes are." A NaN that the sum skipped but the draw did not
  would shift every segment above it.
- Each segment is its own subpath, so ADR 0015's hit index names the segment
  under the pointer rather than the whole stack, and `f.Marks` reports the row
  behind each one.
- The group index and the label→index map live on `geom.scratch` and are reused
  and cleared, not allocated per row. This is the allocation gate's business, and
  the gate is not to be widened for it.

## Revisit if

- **Someone genuinely needs to stack two differently-shaped layers** — a bar
  under an area, say. That is the across-layer case this record declined, and it
  would need the coordinating stage it declined to build; it should be argued
  with a real chart in hand rather than in the abstract.
- **A stat wants to stack its own output.** A histogram of grouped data reaches
  the stack after binning rather than before, and the ordering of `stat` and
  adjustment would need writing down.
