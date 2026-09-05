# 0027 — A size channel maps by area, and the guide column is generalised once

**Status:** Accepted · **Date:** 2026-09-05

## Context

`geom.Size` has been a scalar since v0.1: it sets the diameter of every marker
in a layer. The bubble chart wants that number to come from a column, which is
the third aesthetic channel after position and colour and the last one v1.0's
API freeze can still afford to add.

Three things had to be decided, and none of them is "add a field".

**What a size means.** A reader compares two circles by how much ink is in
them. Map a value to a *radius* and a doubled value gets four times the ink and
reads as four times the quantity — the error Bertin named and every chart
library that ships `r = f(v)` reproduces.

**How a layer draws marks of differing size.** `ir.Backend.Markers` carries one
`MarkerStyle` per call. A layer whose size varies per row would therefore emit
one drawing call per row, which is exactly the cost
[ADR 0007](0007-per-mark-colour.md) refused to pay for colour.

**Where the key goes.** A size channel needs a guide, and it is neither a legend
swatch nor a ramp: the only honest key for a size is the mark itself, drawn at
several sizes. Before v0.9 `layout` carried a `LegendLabels []string` beside a
`Colorbars []Colorbar`, measured by two loops, placed into two result fields.
Adding a third kind the same way would mean three of everything, and a fourth
would mean four.

## Decision

### The scale interpolates area and reports a diameter

`scale.SizeScale` is a third interface beside `Scale` and `ColorScale`, for the
reason `ColorScale` is one: it answers a different question, from a different
column, and it places nothing on an axis.

`scale.Size()` maps a value onto an area and hands back the diameter that has
it. With the defaults — a domain anchored at zero, a range from a diameter of
zero — that is exactly `d(v) = D·√(v/vmax)`, so doubling a value multiplies the
diameter by √2. There is a test that asserts it for every pair in the domain,
not only for the extremes.

The domain deliberately starts at zero rather than at the smallest observation.
A channel whose domain was the data's own extent would draw the smallest of
three near-equal values as a dot and the largest at full size, which says
something about the sample and nothing about the values. `scale.SizeZero` moves
the anchor for a caller who wants the other trade, and `scale.SizeRange` puts a
floor under the smallest mark — both of them documented as what they are, which
is a decision to stop being a proportion.

The *range* is the theme's rather than the chart's. `render` sets it, once per
scale, while it collects the guides — a geom cannot, because panels are built
concurrently ([ADR 0012](0012-parallel-panels.md)) and a scale two of them wrote
to would be a data race, and `Geom.Train` has no theme to read it from. A range
the caller pins with `scale.SizeRange` outranks it, which is the same line
`scale.Zoomer`'s `pinned` already draws between a domain the data decides and
one the caller did.

### A bubble is a shape, not a marker

A layer given `geom.SizeBy` draws circles as subpaths of one path per colour,
through the new `ir.Path.Circle`. The IR gains nothing: ADR 0002 froze the
primitive set on the claim that every curve a chart needs is expressible as
cubics, and a circle is four of them.

Two consequences are load-bearing. One call per *colour* rather than per row is
the same bargain `geom.groupByColor` already makes. And one subpath per bubble
is what lets a pointer land on the bubble it is actually inside, because
hit-testing indexes a mark per subpath ([ADR 0015](0015-hit-testing.md)) — a
bubble chart is the case where bounding boxes overlap almost completely, which
is the same problem a pie's wedges had in v0.8.

The bubbles are drawn largest first, by a stable sort, so a small bubble inside a
large one is still visible and still pointable at, and so two bubbles of one
size keep the order the table gave them.

A layer that varies its size does not vary its *shape*: `geom.Shape` is one of
the options it ignores. A marker ladder is a redundant encoding for telling
series apart, and a bubble cloud is one series.

### The guide column is a list of guides

`layout.Guide` carries what the solver needs of any guide — a kind, a title, the
labels it writes, and (for a size key) the diameters of its samples — and
`layout.Grid.Guides` is a list of them. `GridResult.Guides` is the boxes,
parallel and in the same order. `measureGuides` is one function over one list
with a switch on the kind, and `placeGuides` stacks them without knowing what
any of them is.

That is the generalisation the catalogue asked for, and it is done once. A
fourth kind is a `GuideKind` constant, a measuring function and a drawing
function; nothing between `geom` and `render` grows a field.

`render` keeps the ordered list on its own side as `guide`, with what it needs
to *draw* one, and collects it in a fixed order: the legend, then the
colourbars, then the size keys. That order is the one a reader scans in — what
the series are, then what the colours mean, then what the sizes mean — and
fixing it here is what makes a chart with all three lay out the same way every
time.

Two size guides merge exactly when they would be drawn identically, which is
`SizeGuide.Key` and is the argument `ColorGuide.Key` already makes. The colour
is deliberately *not* part of that key: two layers reading one column through
one scale answer "how big is how much" identically whatever they are painted in,
so they get one key rather than two saying the same thing.

## Consequences

- `layout.Chart.LegendLabels`, `layout.Chart.Colorbars`, `layout.Result.Legend`
  and `layout.Result.Colorbars` are gone, along with their `Grid`/`GridResult`
  twins. This is a breaking change inside the v0.x window and is the whole point
  of making it now rather than after the freeze.
- `theme.Theme` gains `BubbleSize` and `SizeKeyCount`. The first scales with
  `theme.Scaled`, like `MarkerSize`; neither is touched by `theme.Density`,
  which moves spacings and leaves marks alone.
- The JSON spec gains `encoding.size` — Vega-Lite's own channel name — with a
  scale whose `type` is `"size"`, because the mapping is by area and a document
  that left the type off would read as a plain linear range. A range the theme
  chose is *not* written down: it is a property of how the chart was drawn, and
  pinning it would stop a spec drawn at another size from scaling its bubbles.

## Alternatives rejected

**A `geom.Bubble` mark.** It would be `Scatter` with one channel resolved
differently, and it would need its own decimation, its own grouping, its own
legend. The channel is the difference; the mark is not.

**Widening `ir.Backend.Markers` with a per-mark size.** The same request ADR
0007 refused for colour, with the same cost — every backend, for one geom — and
without the hit-testing gain that drawing shapes brings.

**Quantising sizes into a handful of buckets so that `Markers` could batch.** It
draws a chart whose sizes are not the data's, to save a change the IR did not
need.

**Keeping `Legend` and `Colorbars` and adding `SizeKeys`.** Three of everything,
and a fourth kind would make it four. The catalogue predicted this one
([docs/chart-types.md](../chart-types.md) §D) before the code needed it.
