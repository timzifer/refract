# 0036 — An interval is a mark, and the encoding says which way it runs

**Status:** Accepted · **Date:** 2026-09-07

## Context

`docs/chart-types.md` sorted the missing charts by the machinery each needs,
and it was a good schedule for the reason it says: half the catalogue shares
four pieces of plumbing. What that sort could not surface is a gap that is not
a chart *type* at all.

Every chart of a mean, a forecast, a tolerance or a sampled quantity carries
two numbers per row: the value, and a claim about how well the value is known.
refract drew fifteen marks and had nowhere to put the second one. Not "no
convenient mark": the catalogue did not mention error bars, in any bucket, at
any milestone. They are neither a shape that needs a coordinate system nor a
statistic that needs a stat, so a document sorted by machinery had no line to
put them on.

The nearest things were both wrong:

- **`Area` with `Y2`** is the continuous version of the same statement, and it
  is right for a series. Over three categories it is a shape drawn through
  three vertices, which reads as a trend between things that have no order.
- **`Boxplot`** summarises a sample. A mean with a standard error is not a
  sample, and a box drawn from two numbers is a box claiming three it does not
  have.

For a scientific chart this is the difference between publishable and not. For
a business one — a forecast with a confidence band, a measurement against a
tolerance — it is the difference between a number and a number somebody can
act on.

## Decision

**`geom.ErrorBar` draws the interval a measurement is known to within: a rule
between two bounds, a crossbar at each end, and a marker at the measurement
when the row names one.**

### Two spellings, because tables come in two shapes

```go
geom.ErrorBar(src, geom.X("group"), geom.Y("lo"), geom.Y2("hi"), geom.Mid("mean"))
geom.ErrorBar(src, geom.X("group"), geom.Y("mean"), geom.ErrorBy("sd"))
```

The first reuses `Y`/`Y2` as the two ends, which is exactly what they are for
the band of an `Area` and the box of a `Rect`. The second reads a column of
half-widths, because that is what a table of means and standard deviations
already holds — and turning two columns into two other columns is arithmetic a
caller should not have to do to draw a chart of what they measured.

**`Mid` is the one new positional channel, and it had to be one.** An interval
needs three numbers and an axis offers two. Overloading `Y` to mean "the
measurement" in one spelling and "the low bound" in the other, decided by which
*other* options were present, is the kind of rule that is fine to read and
impossible to remember.

**The symmetric spelling marks the measurement; the bounds spelling does not
unless `Mid` names it.** With a spread there always is a measurement — the
centre is the `Y` column. With two bounds there may not be: a minimum and a
maximum are not evidence of a mean, and drawing a marker at their midpoint
would be inventing a reading.

### Which way it runs follows from the encoding

`Y2` or `ErrorBy` makes it vertical; `X2` or `ErrorXBy` makes it horizontal.
That is the rule `Rect` already follows about its edges, and it is why there is
no orientation option to set, forget, or contradict. A layer naming an interval
on both axes is a box rather than an interval and returns `ErrBothAxes`:
guessing which the caller meant would draw a different chart depending on the
order the options happened to be written in.

### The bounds are derived in `Train`

Because the axis has to describe them. An interval whose top runs off the top
of the plot is exactly the reading a reader opened the chart for. It is the
same boundary [ADR 0019](0019-position-adjustments.md) draws for a stacked
bar's totals and [ADR 0028](0028-distribution-stats.md) for a histogram's
counts: what the axis says must not depend on how wide the chart is, and what
will be drawn is what the axis has to cover.

### A cap is half a bar wide

The cap takes the same share of the slot a `Bar` of the same `BarWidth` would,
halved. So an error bar drawn over a bar chart is narrower than the bar it
annotates, by construction rather than by the caller tuning two numbers until
they look right — and a grouped layer given the same `Dodge` lines up over the
bars it belongs to.

### Every segment goes through the coord

The rule and both caps are strokes through `coord.Coord`, not pairs of device
points. Under a polar coord the rule follows the radius and the caps become
arcs, which makes a radial error bar the same mark rather than a second one —
the same argument `boxGeom.span` already makes about a median line.

## Consequences

- **The two spellings draw the same picture, and there is a test comparing
  them run for run.** They describe the same interval, so anything else would
  be a bug that only showed up in whichever spelling the reader did not use.
- **An absent half-width is a hole, not a zero-length interval.** A NaN spread
  gives NaN bounds, the row is unplottable, and `OnMissing` decides — the same
  answer every other mark gives. A *negative* half-width is the same interval
  read backwards and is drawn, because `|e|` is what the row means.
- **A layer with no interval is refused rather than drawn as a point.** An
  error bar without one is a layer that named the wrong mark, and a mark that
  quietly drew nothing would be found by nobody.
- **`docs/chart-types.md` grew a bucket G.** The catalogue sorted by machinery
  could not hold this, and could not hold the three gaps beside it — a tick
  label a document can choose ([ADR 0035](0035-label-format-and-locale.md)), a
  chart in a language, absence in a text column
  ([ADR 0034](0034-null-values.md)). None of them is a shape; all of them were
  the difference between a chart being drawable and being usable.
