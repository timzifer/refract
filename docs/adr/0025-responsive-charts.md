# 0025 — A chart follows its surface by scaling its theme

**Status:** Accepted · **Date:** 2026-09-04

## Context

CONCEPT §14 lists "responsiveness (line widths/font sizes on resize — leans on
gg device scale)" in v0.6. Two things are entangled in that sentence and only
one of them is what a reader means.

**Device scale** is already handled and is not this. A chart is laid out in
device-independent pixels and a backend multiplies by the device pixel ratio;
gg owns that mapping, and a retina display gets a sharper chart of the same
size. Nothing about the design changes.

**Resizing** is the real question, and it appears the moment a chart lives in
something the reader can drag: a window, a canvas in a fluid layout. A plot is
designed at a size — 800×500, or whatever `Size` said — and drawn at another. A
chart that keeps 12pt labels at a third of its design size has labels eating the
plot area; one that stretches its type with the width has 30pt ticks on a wide
dashboard.

## Decision

**`refract.Responsive(true)` scales the theme by how much smaller or larger the
drawing is than the design, and `Live.Resize` is how a surface says so.**

- `theme.Scaled(f)` multiplies every *length* in a theme: type sizes, stroke
  widths, spacings, the marker diameter, the margin, the dash patterns. Colours
  are not lengths and are left alone. It is a theme option because a theme is
  where lengths live, and it is separate from `theme.Density`, which moves the
  spacings and deliberately leaves the text alone — that is a decision about how
  tightly a chart is packed, and this is a decision about how big the drawing is.
- The factor is the smaller of the two size ratios, so a chart stretched wide
  scales to what still fits its height, and it is clamped: past a quarter and
  four times over, a chart wants a different design rather than the same one
  scaled.
- `ResponsiveFrom(w, h)` names the design size explicitly, for the one case a
  surface does not cover: a still rendered at another size. A thumbnail of an
  800x500 chart is `Size(200, 125)` with `ResponsiveFrom(800, 500)`.
- It is **off by default**, and at the design size the factor is exactly 1. A
  still rendered once at the size it was built with cannot change, which is what
  makes this safe to add to a library whose output is compared against golden
  files.
- `Live.Resize(w, h)` lays the chart out again at the new size, keeping whatever
  the scales were zoomed or panned to — a reader who dragged a view into place
  has not asked to leave it — and redraws. The size lives on the `Live` rather
  than on the `Plot`: a window being dragged wider is not an edit to the chart
  specification.
- `Live.Rescale(dpr)` is the same call for a device pixel ratio: a window
  dragged onto another display wants more pixels behind the same chart, and the
  chart does not change size for it. It goes through the same `ir.Resizer`,
  which takes both.
- `ir.Resizer` is the optional backend interface that tells a surface its new
  size. A document target does not implement it and does not need to; a canvas
  resizes its backing store, and the raster surface behind a window reallocates
  its buffer. A backend that cannot resize is redrawn at the new logical size
  into the surface it has, which is the best available answer.

## Consequences

- A window and a browser canvas get the same behaviour from the same code, and
  a test can drive it without either: `Live.Resize` is an ordinary call.
- Because the theme is scaled rather than the canvas, everything follows —
  tick counts are chosen from the axis length, labels are measured at their new
  size, and the legend takes the room it now needs. A chart drawn at half size
  is the chart, not a photograph of it.
- A resized frame shares no coordinate with the last one, so there is no damage
  to compute and the frame is repainted whole. That is correct rather than
  unfortunate: every mark moved.
- Two lengths do not scale, on purpose: the device pixel ratio, which is the
  backend's, and the hit-test tolerance, which is a property of the pointer
  rather than of the drawing.

## Revisit if

charts start needing to *reflow* rather than scale — a legend that moves under
the plot when the canvas is narrow, ticks that thin out rather than shrink.
That is a layout decision per breakpoint rather than one factor, and it belongs
with `layout` and [ADR 0010](0010-panel-layout.md) rather than with the theme.
