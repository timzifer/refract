# 0031 — A track is a panel with a fixed row height, on the panel's own scale

**Status:** Accepted · **Date:** 2026-09-07

## Context

A chart of a machine's last n minutes wants the measured speed, its target and
its threshold as lines, and the machine's state and the running work order as
gantt strips — all against one time axis, because the whole point is to read
that the speed dropped *while* the machine was in a state.

Nothing expressed that. `facet` splits one plot by a column and gives every
panel the same geoms and the same axis kinds; `FreeY` frees the domain, not the
kind, and here the two halves want a rect against a line and an ordinal axis
against a linear one.

What people did instead was put the strips in the main panel as negative Y
values and hide the negative tick labels with `scale.Format`. It looks close to
right, and every one of its costs is structural:

- the lane is in the Y domain, so `scale.Zero()` no longer means what it says
  and a niced axis rounds around geometry rather than around data;
- the lane's height has to be recomputed every frame from the current data
  maximum, because a rectangle names its edges in data space and there is no
  way to say "ten per cent of the axis";
- grid lines are drawn through the lane;
- a Y zoom scales the lane along with the data;
- and it only works at all because the data happens to be non-negative. A chart
  with negative values has nowhere to put the lane.

The alternative shape — two plots linked on a domain — was considered and is
**not** what shipped as the primary answer. See *Alternatives*.

## Decision

**A track is another panel of the same chart, in its own grid row, sharing the
panel's X scale object, with a height the layout solver is told rather than
derives.**

Three things follow from that sentence, and all three are the point:

1. **Zoom is shared by construction, not by agreement.** Zoom state lives on
   the scale object — `scale.Zoomer.SetDomain` pins a domain in place — and
   `render.Panel`s sharing an axis share that object, as ADR 0010 already
   established. `Plot.tracked` hands the track `c.X` itself. There is one scale
   to zoom, so there is nothing to keep in step, and no pair of handlers that
   could drift. **Cloning X there would compile and would silently break the
   feature**; the code says so where it happens.
2. **The panel's Y domain is untouched.** The track has a Y scale of its own,
   of whatever kind suits it — `scale.Ordinal` by default, because a track's
   rows are lanes with names. A track is not data on the panel's axis, and now
   nothing makes the panel's axis think it is.
3. **Hit-testing, parallel rendering and the widget need no new code.**
   `interact.Index.Panel` is already per-panel and `Hit` already carries
   `Panel`, so a tooltip names a bar in a track the way it names a point in the
   panel. A track makes a one-panel chart a two-panel one, so it takes the
   concurrent path, which `scale.Snapshotter` already covers. And a track lives
   inside one `Plot`, so `fyne-refract`'s widget — which wraps exactly one
   Plot and one Live — did not change at all.

### What this does to ADR 0010

ADR 0010 decided that **every panel is the same size**, and it was right to.
Its own *Revisit if* names this shape exactly: "a marginal-distribution plot
where a narrow strip sits beside a square one".

This is that revisit, answered **narrowly**. `layout.Grid` gains
`RowHeights []float32`: a row named there is that tall, and every other row
takes an equal share of what is left. That is not the general size-per-panel
solver ADR 0010 warns about and declines — a row is fixed or it is not, and
nothing in this can make two flexible rows differ from each other. The panels
that are panels are still all the same size.

There is still one solver, and `layout.Compute` is still `Panels` over a
one-by-one grid. A grid that fixes no row computes what it computed before, and
the arithmetic is written to make that bit-exact rather than merely close: the
share is one division of the same quantity, and the total handed to the guide
column is a multiplication rather than a sum, because a float32 sum of n equal
terms is not always their product and every golden file in the repository would
have moved by an ulp. The evidence is that none of the eighteen did.

## Alternatives

**Linking two plots on a domain.** The other shape of the same need: a `Plot`
above a `Plot`, `refract.Link(LinkX, a, b)`. It was rejected as the *primary*
answer for three reasons, in increasing order of weight:

- Linking has to cover the domain, the interaction *and* the layout — two panel
  rectangles need identical left and right insets, or the two time axes are
  drawn at different scales and the whole point is lost. That is a cross-plot
  layout protocol; a chart with tracks needs none.
- `refract.Grid` has no `Live`. A linked pair would be static, or would need a
  second interaction path.
- `fyne-refract`'s widget holds one Plot. Two linked plots would need a
  `chart.Sync(a, b)` forwarding gestures between widgets — a whole API whose
  only job is to rebuild what one shared scale object already gives.

It is not gone, and it needed almost nothing, because `Grid` already routes
stacked plots through the one solver and already takes the plots' scale
*objects*. Two `GridOption`s finish it: `GridRowHeights` for the short row and
`GridSharedX` for writing the tick labels once, under the bottom row. Handing
both plots the same `scale.Time()` shares their domain, their nicing and their
framing. **No `refract.Link` was added**: it would be a name for what passing
one scale to both already does, and a name for that would imply it does more.

**Left and right tracks** are the mirror image — grid columns with fixed
widths, sharing the panel's Y — and would carry colour keys and marginal
distributions. `Edge` is numbered so they can be added, and `ColWidths` is
deliberately *not* on `layout.Grid` yet: an unused field is a promise.

## Consequences

- `render.Chart` gains `RowHeights`, and `render.Panel` gains `HideGrid`. A
  track hides its grid by default: a grid line through a gantt bar is a rule
  drawn across a solid shape. A banded scale already draws no grid lines of its
  own, so this is about the *shared* axis's lines crossing the lanes.
- A track's lane names share the panel's left gutter, because the gutter is
  per column and there is one column. That is what keeps the track's left edge
  and the panel's left edge in the same place — for free, and by the same
  mechanism that aligns a facet.
- The X tick labels are written by the bottom-most row, so a bottom track
  carries the shared time axis and the panel above writes none. `measurePanels`
  already reserved no gutter for a panel with `ShowX: false`, so this needed no
  new code either.
- **A track's lanes cannot be zoomed, and that is not an oversight.**
  `zoomAxis` and `panAxis` no-op on a scale that is not a `scale.Zoomer`, and
  `scale.Ordinal` deliberately is not one — half a category is not a view of
  anything. So a wheel over a track scrolls time and leaves the lanes alone.
  This is free today and there is a test pinning it, because it would be lost
  silently if an ordinal scale ever gained `SetDomain`.
- **A track and a facet together are `ErrTrackWithFacet`.** A facet owns the
  grid's rows and columns — it decides how many there are and what each means —
  and a track needs a row of that grid. A band spanning a facet is a different
  feature with its own questions, so the combination is refused rather than
  guessed at.
- A track is Cartesian whatever coord the chart is in. A band bent around a
  polar chart's angle is not a band.

## Revisit if

Left and right tracks arrive, which need `ColWidths` and the same argument
transposed — or a track is wanted on a faceted chart, which is the case this
deliberately refuses and which would have to answer what a band spanning
several panels with different domains actually shows.
