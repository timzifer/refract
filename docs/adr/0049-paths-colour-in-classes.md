# 0049 — A path colours in classes, and the scale decides where the colour changes

**Status:** Accepted · **Date:** 2026-09-08

## Context

Since [ADR 0007](0007-per-mark-colour.md) a layer has been able to take each
mark's colour from a column: `geom.ColorBy(col, scale)` binds it, and
`scratch.colorsFor` resolves one colour per row. Every mark geom reads it —
scatter, bar, rect, text, beeswarm, error bar, arc, sankey, hexbin, violin.

`geom.Line`, `geom.Step` and `geom.Area` never did. They took one colour per
path, or one per `GroupBy` series, and ignored `colorScale` entirely: binding a
column to a line was a silent no-op, which is the worst of the three possible
answers. [ADR 0042](0042-colour-transforms-and-classes.md) sharpened the loss by
adding `scale.Threshold`, `Quantize` and `Quantile` — the scales whose entire
purpose is "this part is over the limit" — and leaving the mark that shows a
limit being crossed unable to say so.

Two charts wanted it, and they are not the same chart:

- a measurement against a threshold, where the reader has to see **when** the
  limit was passed;
- a series coloured by the state of the thing that produced it — a machine
  running, faulted, under maintenance — where the reader has to see **which
  state** each stretch was in.

## A stroke has one colour, so a path colours in stretches

`ir.Stroke` carries a colour and no stops, and [ADR 0007](0007-per-mark-colour.md)
already refused a per-vertex colour channel: adding one touches every backend
for the sake of two geoms. That decision stands here, and it settles more than
it looks like it does.

A mark loses nothing to it. `groupByColor` batches a thousand differently
coloured points into as many drawing calls as there are distinct colours,
because points do not touch. A path is not that: its colour changes *between*
two vertices, so the batching a scatter does — collect every red point, draw
them in one call — would connect points that are nowhere near each other. A
coloured path is therefore **contiguous runs**, one stroke each, and two
neighbouring runs share their boundary vertex so that the edge between them is
drawn exactly once.

The consequence is the interesting part: **a continuous ramp on a path is
refused**, `geom.ErrRampOnPath`. A path can say "this stretch is over the
limit"; it cannot say "this stretch is slightly further over it than that
one" — there is no colour it could stroke that with. The alternatives were both
worse than an error. Drawing one flat-coloured stroke per edge is a call per
row, on the geom whose decimation exists because a million rows must not become
a million calls. Quantising the ramp into some number of steps invents classes
the caller did not ask for and then hides that it did. Refusing says the true
thing and names the three scales that mean it: `scale.Threshold`,
`scale.Quantize`, `scale.Quantile` — or `geom.Scatter`, which really can paint a
continuum.

Turning a silent no-op into an error is a behaviour change, and it is a safe
one: no chart can be relying on the old behaviour, because the old behaviour was
that nothing happened.

## Where the colour changes follows from the scale, not from an option

Given a run-splitter, the whole design is one question: between two rows of
different colour, *where*?

There were two candidate answers, and the temptation was to add an option and
let the caller pick. That would have been wrong, because the two answers are not
preferences. They are what the two kinds of scale mean.

**A classed scale cuts a quantity at boundaries, and a boundary is a value.**
The line crosses it somewhere between the two rows, and that somewhere is the
content of the picture. A chart that turned red at the next measurement rather
than at the limit would be reporting a different time — off by however long the
sampling interval happens to be, which is a property of the logger and not of
the machine. So the crossing is interpolated and a vertex is inserted there. An
edge that jumps two classes at once gets a vertex per boundary, and the short
run between them is drawn: it is where the value was.

**A discrete scale paints categories, and a category is not a quantity to
cross.** Nothing was measured between two rows of a state column. The new colour
therefore begins at the row where the new state was first seen, and the edge
into it keeps the old one. Interpolating there would invent the moment the
machine changed — the one fact the data does not contain.

So there is no `geom.ColorSplit` option. `splitFor` reads the scale and the
answer follows. An option would have offered a wrong answer for each scale and
asked the caller to know which was which.

### The corner lands on the drawn threshold, not near it

Interpolating the *value* linearly and interpolating the *position* linearly are
the same thing only on a linear axis. Under `scale.Log` they are not, and the
reader is looking at the position: the corner has to sit on the gridline the
threshold is drawn at, or the picture contradicts itself. So when the colour
column is also a positional one — the common case, a threshold on the Y column —
the boundary is mapped through that axis and the parameter comes back from the
mapped space. Otherwise the value's own space is the only space there is, and it
is used.

The split runs on the columns the scales mapped into rather than on device
points, for the reason `stepColumns` already does it there: where the colour
changes is a statement about the data, and a coord that bends its edges would
otherwise be the thing deciding it.

### A staircase changes colour on its riser

A step only *moves* on its risers; its treads are a reading held. That makes the
corner vertex the whole question, and it takes a different value under each kind
of scale — for exactly the reasons above, applied to a shape that has two
vertices per row.

Under a discrete scale the corner takes the **new** value, so the riser is drawn
in the colour of the state it rises into: the machine changed at this x, and the
vertical *is* that change. Under a classed scale it takes the **old** one, so
that the riser — the only stretch where the value actually moves — is what the
crossing is interpolated along. The other way round would put a threshold
crossing half way along a tread, at a moment the reading was flat.

## What this does not do

**`geom.Area` stays one colour.** Its fill is a single closed path per segment,
and splitting it means closing a new path per run against a baseline or a lower
edge, with stacking on top. That is a bigger change than this one and it is not
made better by being rushed alongside it.

**A class cannot be switched off.** `Live.Hide` works per layer
([visibility.go](../../visibility.go)), and a class of a coloured line is not a
layer. A click on the classed colourbar reports the range it landed in — [ADR
0048](0048-clickable-colourbar-and-size-key.md) — so a caller who wants to
filter by class has what it needs to rebuild with a filtered source; making
hiding itself finer-grained is a separate decision.

**A curve joins at C0, not C1.** With `geom.Tension` the spline is fitted per
run, so the tangent turns at a colour boundary. The alternative is a spline
fitted across the boundary, which puts the corner somewhere other than where the
colour changed — a smoother picture of the wrong thing.

## `scale.Named` is why the state chart works

`scale.Qualitative` hands out palette entries in order of first appearance,
which [ADR 0012](0012-parallel-panels.md) requires to be the rule for
anything discovered from the data. For a state column that is exactly wrong: the
colour of `FAULT` would depend on which window of the stream is on screen, and a
reader knows a fault is red before consulting the legend.

`scale.Named` takes the mapping instead. Its enumerated categories come first
and sorted — a Go map has no order to preserve, and one that depended on map
iteration would be an order that depends on scheduling, which 0012 forbids. A
category the caller did not enumerate is still drawn, from a fallback palette,
because a state code nobody wrote down is a category and not a null: leaving it
undefined would put a hole in the line, and a hole in a line reads as missing
data.

It is a discrete scale like any other, so it contributes legend entries through
the path `config.legends` already had, and it works on every mark geom as well
as on the two path geoms this record is about.

## A chart of one layer painted from categories shows its legend

`Plot.showLegend` turns the legend on by default for more than one layer, or for
one layer that draws more than one series — `GroupBy`, or a relational layer
whose nodes are declared by an edge table. A layer painted per mark from a
*discrete* colour scale is the same case under a third name: its categories are
series in everything but the word, the colour is the only thing saying which
mark is which state, and nothing but the legend says what the colours are.
Before this it was left off, which made the state chart a picture of coloured
stretches with no key.

A continuous or classed scale is deliberately not this case. It contributes a
colourbar, and a colourbar names itself.
