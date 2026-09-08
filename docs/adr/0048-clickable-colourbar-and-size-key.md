# 0048 — A colourbar and a size key report a quantity, because neither is a series

**Status:** Accepted · **Date:** 2026-09-08

## Context

[ADR 0047](0047-clickable-legend.md) made a legend row findable and closed with
the case it had not taken:

> A colourbar or a size key wanting to be clickable is the other. Neither maps
> to a layer the way a legend row does — a colourbar row is a *value*, not a
> series — so the announcement would need a vocabulary for what was clicked,
> and that is its own record.

This is that vocabulary, and the whole difficulty is in one question: what does
clicking a colourbar *mean*?

A legend row is easy because it stands for a discrete thing that already
exists — a layer — so "clicked" resolves to "that layer". A colourbar stands
for a continuum. Clicking one could plausibly mean a threshold, a range, a
filter, or nothing at all, and refract cannot know which.

## Decision

**A guide hit reports a quantity. What the quantity means is the caller's.**

That is the same answer as [ADR 0045](0045-linked-views.md) and ADR 0047, and
it is load-bearing here rather than a habit: a band that always filtered would
be wrong for a chart where it should select, annotate, drill down, or do
nothing.

### Three kinds, not one with a discriminator

`interact.Kind` gains `Colorbar` and `SizeKey` beside `LegendRow`, so a handler
branches with a `switch` on the field it already has:

```go
switch ev.Hit.Kind {
case refract.LegendRow: live.Toggle(ev.Hit.Layer)
case refract.Colorbar:  filter(ev.Hit.Lo, ev.Hit.Hi)
case refract.SizeKey:   pick(ev.Hit.Value)
}
```

`Kind.Guides()` is the umbrella predicate, for the places that care only that
something is furniture — `Live.Move`'s margin exception is the one in the
library.

**`interact.Guide` was renamed to `interact.LegendRow`** in the process. It
shipped one commit earlier meaning specifically a legend row, and once there
were three of them the general name on the specific thing was actively
misleading. `refract.Legend` was already the option that asks a plot for one,
which is why the root spelling carries the `Row`.

### A classed bar reports bands; a continuous one reports itself

A band is a discrete thing a reader can mean — *the rows between these two
numbers* — so `drawClassedBar` announces one per band with its class index and
interval. A continuous ramp has nothing discrete to enumerate, so it announces
once for the whole bar and the value is read back by inverting the ramp at the
point asked about.

**The inversion is through the ramp, not through the axis beside it.** The two
disagree wherever the ramp is compressed — a log ramp, a diverging one centred
off zero — and [ADR 0042](0042-colour-transforms-and-classes.md) already
settled that the ramp is the thing the reader is looking at.
`scale.ColorValueOf` is the inverse and already existed for the gradient
sampling; the index keeps the `ColorScale` on the mark and asks it at hit time,
which is what a hit in a panel already does with the panel's scales.

### A band reports its midpoint, not a position within itself

The first implementation inverted a position inside the band and reported
values that varied across it. That is precision the scale threw away on
purpose: a band is one colour standing for one interval, and there is no
gradient inside it to read. So `Hit.Value` for a band is the middle of what it
covers — a representative — and `Hit.Lo` and `Hit.Hi` are the truth a filter is
written against.

### `Hit` gains four fields

`Value`, `Class`, `Lo`, `Hi`. `Class` is −1 for anything that is not a band,
including a continuous bar, and `Index.At` sets it explicitly the way it
already sets `Row: -1` — the zero value of an `int` is a legitimate class index
and would otherwise be a lie.

A guide fills in none of `X`, `Y` or `Row`: it belongs to the chart rather than
to a panel, and inverting its position through a panel's scales would report a
value from a place no value was drawn. `Layer` is −1 for a colourbar and a size
key, because neither maps to one — which is the fact this whole record starts
from.

## Consequences

- **Two more optional interfaces beside `Observer`**: `render.ColorbarEntry`
  and `render.SizeKeyEntry`, in the shape `LayerAxes`, `EndData` and
  `LegendEntry` already established. Observer still never gains a method, and
  an observer implementing none of them sees what it saw before.
- **`sizeSample` gained the value it stands for.** It carried only a label and
  a diameter, and a filter written against `"1.2k"` is a filter against a
  string.
- **A size key row's target spans the key**, so the gap between a sample and
  its label is part of it — the same rule a legend row follows.
- **`Index.MarkCount` grows** by one per band, one per continuous bar and one
  per size-key row. A chart with a colourbar has a handful more marks; none of
  them is indexed as data, which `TestNoGuideReportsAPanelValueOrARow` pins.
- **A colourbar has no hidden state**, so there is no `Live.Toggle` for it.
  Hiding *a range of values* is a filter over rows, which is `Plot.SetLayers`
  and a `data.Rows` — a different statement about the chart, and the caller's
  to make.
- Nothing about the JSON spec changed, for the reason nothing did in 0046 or
  0047: where a reader clicked is not a fact about a chart.

## Revisit if

Someone wants to drag a *range* on a continuous bar — press at one value,
release at another — which is the natural gesture and the one this does not
give. It is a brush, and the brush machinery already exists
([ADR 0045](0045-linked-views.md)); what is missing is `Input` knowing that a
drag which starts on a colourbar is a different drag from one that starts on a
panel. That is a mode question rather than a vocabulary one, and it should be
answered when someone has written the awkward version by hand.

An axis wanting to be clickable is the other. A tick is furniture too, and
"click a tick to filter to that category" is a real interaction — but an axis
is drawn per panel rather than once per chart, so a hit would have to carry
which panel *and* which axis, and that is a third vocabulary rather than a
fourth kind.
