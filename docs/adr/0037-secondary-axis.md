# 0037 — A second axis is a scale on the chart and a binding on the layer

**Status:** Accepted · **Date:** 2026-09-07 · **Amended:** 2026-09-07 (see
[Amendment](#amendment-the-horizontal-direction))

## Context

`refract.Plot` had exactly one X scale and one Y scale, and a layer had no way
to name a different one. That is the right default and it was the only option:
the two-scale chart — revenue as bars against a left axis, margin as a
percentage line against a right one — could not be drawn at all.

It is worth saying why the workarounds do not work, because two of them look
like they should:

- **Normalising into the primary axis's units.** Plot margin × 10 000 and label
  the ticks with a formatter that divides again. The chart draws; the *axis*
  then reads in units nobody measured, a zoom scales the wrong thing, and a
  tooltip reports 1 400 for a 14 % margin.
- **A track** ([ADR 0031](0031-tracks.md)). A track is a band *beside* the
  panel on a scale it shares along one direction. Two quantities plotted
  against each other in the same rectangle is the opposite arrangement.
- **A grid of two plots** ([ADR 0010](0010-panel-layout.md)). Two panels
  stacked share an X axis and are two pictures. The whole point of the two-axis
  chart is that the reader compares the *shapes* against a common horizontal
  extent, in one rectangle.

The reason it is not simply "add a field" is that the axis reaches five places
that were written for one: training, layout, the furniture, hit-testing, and
the document.

## Decision

**The scale is on the chart and the binding is on the layer.** Stated for the
vertical direction, which is the one it was written for; the horizontal one is
the same sentence and is covered by the amendment below.

```go
p.Y(scale.Linear(scale.Zero()))
p.Y2(scale.Linear(scale.NumberFormat("#.0%")))
p.Add(geom.Bar(src, geom.X("m"), geom.Y("revenue")))
p.Add(geom.Line(src, geom.X("m"), geom.Y("margin"), geom.OnY2()))
```

A scale does not know which marks read it — nothing in refract's scales ever
has — so the binding has to be somewhere else, and the layer is where it
belongs: a chart with two Y axes is one chart with two of them rather than two
charts overlaid. Putting the binding on the scale would also make a scale
shared between panels mean different things in different ones, which is the
trap [ADR 0031](0031-tracks.md) is about from the other side.

### render asks the layer through `Describe`

`render.Panel.axisOf(g)` reads `geom.OnSecondaryY(g)`, which asks the layer's
`Describer`. Not a method on `Geom`: that interface is implemented outside the
package and never gains one ([CONCEPT §15](../../CONCEPT.md#15-versioning--stability)).
Using the description rather than a new optional interface has a second
benefit — the binding is already part of what a layer says about itself, so the
chart and the document agree by construction rather than by two mechanisms
staying in step.

A layer that cannot describe itself reads the primary axis, which is what every
layer written before there were two of them means.

### The furniture comes from the coord, through an optional interface

`coord.Opposite` places the second axis's line, ticks and labels on the far
side of the panel. Cartesian implements it; **`Polar` deliberately does not** —
a ring has one radial axis and no far side to put another on, and a second
radius drawn over the first would be two scales sharing one line. A coord that
does not implement it draws no second axis, and the chart is otherwise
unchanged.

It is written out rather than derived from the first axis by reflection.
Reflection would have to know which of the label's two alignments and which of
the tick's two endpoints to flip, and getting one wrong produces labels inside
the plot — a bug that looks like a theme.

**The second axis draws no grid lines.** Two ladders of horizontal rules at
different values are a moiré rather than a reading, and which of the two scales
a given line belongs to is unanswerable by looking. The grid stays the primary
axis's, and there is a test asserting the count does not change.

### The layout gains a right gutter, and that is all it gains

`layout.Panel.Y2Labels` sizes a per-column right gutter exactly as `YLabels`
sizes the left one, and `Grid.Y2Title` reserves a band outside it. A column
whose panels have no second axis gets a gutter of zero, so a grid without one
is laid out precisely as it was — which is the property every golden file in
the repository rests on.

One thing had to move with it: the guide column is anchored past the last
column's right gutter rather than on the panel edge. The gutter is already
subtracted from the available width, so an anchor that ignored it would put a
legend on top of the axis labels.

### A hit is read back through the scale that placed the mark

`render.LayerAxes` is an optional interface beside `Observer`: an observer that
implements it is told which vertical scale the layer about to be drawn reads.
`interact.Index` implements it and records the scale per mark, so
`Index.At` inverts through the axis the mark was placed by.

This is the half that would have been easy to skip and expensive to leave out.
An index that inverted every mark through the panel's Y would report `4200` for
a point on a chart whose right axis reads `12 %` — a number that is not wrong
by a rounding but by a unit, on the feature whose entire purpose is that the
two units are different.

### The document carries `ySecondary` and `axis: "y2"`

The scale travels on the top-level encoding as `ySecondary`, **not** as `y2`:
that name is already the layer channel for the far end of a band, and two
things called `y2` in one document is how a reader ends up with a chart that
draws neither. The binding travels on the mark, because a mark always has one
and a layer's Y channel does not — a histogram computes its Y and names no
column for it.

## Consequences

- **A free facet axis frees both.** One panel's data must not move another
  panel's axis, and that is as true of the second as of the first; a shared
  second axis under a free first one would be half a free facet, which is not a
  reading.
- **A layer that asks for an axis the chart does not have draws against the
  primary one, silently.** That is the line [ADR 0026](0026-breaking-a-mark-out.md)
  draws for a break-out under a Cartesian coord, for the same reason: an option
  every mark accepts must not make a chart's validity depend on something set
  somewhere else.
- **A chart with a second axis and no layer on it still draws the axis.** An
  axis somebody asked for is a statement about the chart even where nothing
  reaches it yet — a live chart whose second series has not arrived is the
  case.
- **The parallel path snapshots it.** Panels share one scale object per axis,
  and setting its device range from two goroutines is the same write race the
  first axis has, answered the same way.
- **What was still one thing per chart: the X axis.** A secondary *horizontal*
  axis is the same machinery turned a quarter turn and was not in the first
  version of this record, because nothing had asked for it. Something did; see
  the amendment below.

## Amendment: the horizontal direction

The consequence above gave two reasons for leaving the second X axis out. One
was "nothing has asked for it", and that expired the day somebody did. The
other was an argument and deserves an answer rather than a quiet reversal:

> a second X axis is usually a second *time* base, which is a different feature
> (two domains over one extent) wearing this one's clothes.

**That was half right, and the half it got wrong is the half that mattered.**
There genuinely is a different feature nearby: two *layers* whose X columns are
unrelated measurements sharing one rectangle — a run indexed by cycle beside a
run indexed by elapsed time — and that is what `Plot.X2` plus `geom.OnX2`
gives, exactly as `Y2` gives it vertically. What the argument missed is the
*more* common shape, which needs no second feature at all: **one reading with
two rulers under it**. An oven curve the operator counts in cycles and the
engineer counts in minutes is one series, one set of marks, and two ladders
describing the same extent. That falls out of this machinery for free, because
an axis with no layer bound to it is still drawn — which this record had
already decided, for a different reason, one consequence up.

So the generalisation is the feature, and the "different feature" it was held
back for turns out to be a *third* thing that neither this nor the guess
describes: two domains over one extent that must stay in a fixed relation, so
that zooming one rescales the other. That one is still not here, and now has a
name.

### What generalising cost

Less than the first direction did, which is the argument for having built the
first one as machinery rather than as a special case:

- `coord.Opposite` gained `FurnitureX2` beside `FurnitureY2` — one interface,
  because they are one capability ("this coord has edges opposite its axes")
  and a coord that can answer for one direction can answer for the other.
  Cartesian implements both; Polar still implements neither.
- `layout` gained a per-row **top gutter**, which is the right gutter turned a
  quarter turn, and a title band above the panels and below the chart title.
  A row whose panels have no second axis gets a gutter of zero, so every
  existing figure is unchanged — the same property the right gutter has.
- `render.LayerAxes` became `LayerAxes(x, y)` rather than `LayerY(y)`. It was
  added in the same unreleased batch as this record, so widening it costs
  nothing; had it shipped, the second direction would have needed an interface
  of its own, which is an argument for naming an optional interface after the
  *capability* rather than after the first use of it.
- `Panel.axisOf` became `Panel.axesOf`, returning both scales. The two
  directions are independent by construction: a layer may name `OnX2` and
  `OnY2` together and then reads the top axis and the right one.
- The document carries `xAxis` and `yAxis` on the mark rather than one `axis`,
  because a single field would have to spell a *set* once a layer can be on
  both.

### The strip moved, and that is load-bearing

A facet's strip is drawn above its panel. With a second horizontal axis there
are now two things above the panel, and the order is: panel, then its own
axis's labels, then the strip naming it. Same order as the right-hand side and
the same reason — the axis belongs to the panel and the strip names the panel.
A strip drawn between the panel and its own tick labels would read as though it
named the axis.
