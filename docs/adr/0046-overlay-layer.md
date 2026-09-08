# 0046 — The chart owns an overlay, drawn last and announced to nobody

**Status:** Accepted · **Date:** 2026-09-08

## Context

[docs/chart-types.md](../chart-types.md) has carried the same remainder since
bucket H shipped:

> What is left of the bucket is an **overlay layer the chart itself owns** — a
> tooltip, a crosshair, a brush rectangle — which `interact` cannot draw
> because it only reads.

[ADR 0045](0045-linked-views.md) narrowed it but did not close it: a host can
now identify a row, select a region and act on another chart, and it draws its
own feedback. Drawing it is the part that was still missing, and "the host
draws it" is a real answer for a browser — where there is a DOM to put a
`<div>` in — and no answer at all for a native window, a PNG, or a chart
exported with the reader's crosshair where they left it.

Three places it could go.

**A geom.** A layer that draws a crosshair. It cannot work: a layer's positions
come from its data through the scales, and a crosshair's come from a pointer. A
geom that read a pointer would be a geom whose data depended on the frame
before it, and the facet machinery would try to cut it by a column it does not
have.

**A backend.** Let the surface draw over what it was given. That is the
inversion [ADR 0017](0017-browser-backend.md) refuses in the other direction: a
backend consumes IR and must not know what a panel or a scale is, and every
backend would need its own implementation of the same crosshair.

**A stage in `render`.** `render` is already the only package that knows drawing
order, which is exactly what "over everything else" is a statement about.

## Decision

**`render.Chart.Overlay` is a stage after the guides, given the backend and
where the panels are.**

- `render.Overlay` has one method, `DrawOverlay(b ir.Backend, f OverlayFrame)`.
  It is implemented outside `render`, so it never gains one.
- `OverlayFrame` carries the canvas, the theme, and an `OverlayPanel` per panel
  — its area, its scales already ranged, and the coord the paint pass framed.
  The coords are collected during that pass rather than recomputed: ranging a
  panel generates its ticks, and doing it twice would allocate a tick list per
  frame for nobody to read.
- Nil is the default and costs nothing. A chart with no overlay emits
  byte-identical calls to one from before there were any, which is the same
  property `Index.Watch` has and for the same reason: one set of golden files
  covers both.

### It is not clipped

A crosshair wants to be confined to its panel; a tooltip wants to overflow one.
So an overlay that wants a clip pushes its own, and `OverlayPanel.Area` is what
to push. Choosing for it would have made one of the two impossible.

### It is not announced to the `Observer`, and therefore not hit-testable

This is the property that makes an overlay usable rather than merely present. A
tooltip a pointer can hit is a tooltip that flickers: hovering it moves the
pointer off whatever the tooltip was about, which dismisses it, which moves the
pointer back on, at about thirty hertz.

Getting there needed a bug fixed first. `Observer` has no way to *close* a
layer — `Layer` opens one and the next `Panel` opens another — so after the
final layer of the final panel, everything drawn afterwards was attributed to
it. The guides already were: a legend's swatches were indexed as shapes
belonging to whichever layer happened to be drawn last, contradicting
[ADR 0015](0015-hit-testing.md)'s own rule that "the grid, the axes, the titles
and the guides are furniture".

It was invisible because `Live.Move` does not hit-test a point outside every
panel, so the only reachable path never asked. `Index.At` is public and does
ask, and an overlay draws *inside* a panel — so the same latent bug would have
been a visible one.

`render.EndData` is the fix: an optional interface beside `Observer`, in the
shape `LayerAxes` already established, called once after the last layer.
`interact.Index` implements it by closing the layer.
`TestAGuideIsIndexedAsAGuideAndNotAsAMark` and
`TestNothingOutsideAPanelIsHitAsData` pin both halves — under those names since
[ADR 0047](0047-clickable-legend.md), which made a legend row findable on
purpose and under a kind of its own. The rule they enforce is unchanged: no
piece of furniture is indexed as data.

### The built-ins are structs whose zero value draws nothing

`Crosshair`, `Highlight`, `Brush` and `Tooltip` live in the root package,
because they need `theme` and are policy rather than seam. Each takes its
colours from the theme when it is not told, so an overlay over a dark chart is
legible without being configured. `Overlays` composes them in drawing order.

They are *pointers* a caller keeps and mutates — a crosshair's position, a
tooltip's lines — so installing one and moving it per event is the shape, and
there is nothing to re-install.

`Tooltip` is the only one that measures, and it measures through the backend
that is about to draw it. That is what `ir.Backend.Measure` is for: a box sized
from an estimate is a box that clips its own text in whatever font the surface
actually has. It flips at the canvas edge rather than clamping, because a box
pushed back inside would sit over the point it is about.

### `Input.Move` repaints when there is an overlay

`Live.Move` answers a question and deliberately does not draw — that is the
split the whole interactive API is built on. But the handler it fires is where
a crosshair's position gets set, and nothing else was going to repaint, so a
crosshair would lag a frame behind the pointer or not appear at all.

`Input` is the surface driver rather than the question-answerer, and it already
draws for a pan. So it draws for a hover too, but only when an overlay is
installed. A frame identical to the last is still not painted, so a chart
without one pays nothing.

### The plot carries one as well as the Live

Otherwise `Plot.Render` and `Live.Draw` would draw different pictures of the
same model, and exporting what a reader is looking at would be impossible from
the model alone. `Live.Overlay` wins where both are set, because the Live is
the thing with a pointer over it, and it survives a `Rebuild` for the same
reason.

## Consequences

- **An overlay that appears or disappears is a full repaint.** It changes how
  many calls a frame has, which makes that frame not comparable with the last —
  `ir.Damage` reports `ok == false` and the whole canvas is repainted. One that
  only *moves* is a damage rectangle like anything else, and
  `TestMovingAnOverlayIsAPartialRepaint` pins that. A crosshair following a
  pointer is the cheap case; one blinking on and off at every panel boundary is
  two full repaints per crossing. That is a real property and the fix, if
  anyone needs one, is to keep it shown and move it off-panel rather than to
  toggle it.
- **An overlay is not in the JSON spec.** A spec document is a chart, and where
  a pointer is is not a fact about a chart. `Plot.Overlay` is a Go-level
  attachment like `Plot.On`, and neither round-trips.
- **The overlay draws over the guides**, including the legend. That is what
  "last" means, and a tooltip that appeared behind a legend would be a tooltip
  nobody could read.
- **A third-party overlay is a first-class one.** `Overlay` is one method and
  everything the built-ins use is exported, so a caller wanting a scrubber, a
  range band or a magnifier writes one and installs it the same way.
- Nothing about hit-testing changed for a chart that has no overlay, except
  that a legend swatch is no longer indexed *as a mark* — which is the fix. It
  is indexed as a `Guide` since [ADR 0047](0047-clickable-legend.md), which is
  a different thing and reachable only by asking for it.

## Revisit if

~~An overlay wants to be hit-testable after all — a legend whose entries can be
clicked to hide a series is the case, and it is a real one.~~ **Answered by
[ADR 0047](0047-clickable-legend.md).** It went the way this record predicted:
the guides are announced, through an optional `render.LegendEntry` beside
`Observer`, and indexed under a kind of their own so that a hit on a swatch is
distinguishable from a hit on the thing the swatch stands for. The overlay
itself is still not hit-testable and there is still no reason for it to be.

An overlay also cannot currently read the rows a layer drew — it is given
scales and areas, not marks. A tooltip that wanted to snap to the nearest point
gets that from `interact.Index` on the caller's side, which is where the hit
test already lives; if that turns out to be awkward often enough, handing the
index to the overlay is the smaller of the two changes.
