# 0045 — Linked views are the host's; refract supplies the two ends of the wire

**Status:** Accepted · **Date:** 2026-09-08

## Context

"Hover a point in one chart, highlight the matching flow in another" is the
most-asked thing an interactive charting library does not do here.
[docs/v1-api-audit.md](../v1-api-audit.md) parked it as additive — `Select
EventKind`, `Event.Rows []int` — and
[docs/chart-types.md](../chart-types.md) named the missing piece as "an overlay
layer the chart itself owns … which linked brushing across panels needs before
anything else".

Two shapes were available.

**A linking engine.** Charts declare a shared selection; refract propagates it.
That is what a dashboard framework does, and it is a statement about *two*
charts — while refract's entire model, from `Plot` down through `render.Chart`,
is about one. It would need a chart registry, a selection that outlives a
frame, and an answer to whose selection it is when two charts disagree. Every
one of those is state refract does not have and would have to start keeping.

**Two ends of a wire.** The library says precisely what the pointer is on, and
gives a caller everything needed to act on another chart. The Go program in the
middle is the link.

The second is not a smaller version of the first. `interact`'s whole design is
that it *reads* — [ADR 0015](0015-hit-testing.md) builds the hit index by
watching a render rather than by asking anything — and `examples/web` already
states the principle: *"drawing the result is the page's business, not
refract's."*

## Decision

**refract supplies identification, selection and the ability to act without
losing the reader's place. It does not supply the link.**

The wire's two ends are [ADR 0043](0043-mark-identity.md)'s: `Event.Key`
identifies a row in terms another chart can understand, and `Index.Locate`
turns a row back into a place on screen. What this record adds is the rest of
what a host needs.

### Selection

- `interact.Select` is appended to the event kinds, so the ones that already
  mean something go on meaning it.
- `Event.Rows` carries the rows a selection covered. **One event per layer
  touched**, so a list of rows never has to say which layer's rows it holds: a
  rectangle over two crossing series fires twice, and a handler ignores the one
  it does not care about by its layer rather than by unpicking a mixed list.
- `Live.Select(rect)` is the read half of a brush. It reports and does not
  decide: what a selection *means* is the caller's.
- The rows are the positions the layer **reported**, not the ink it drew, so a
  half-covered bar is a question about where its value is rather than about
  where its corner is. It is the same position `Locate` hands back and
  `Hit.Row` resolves through.

### A drag is a mode, not a modifier

`Input.Drag(DragPans | DragSelects | DragZooms)`. A modifier key is a fact
about a keyboard and this package has never seen one: a browser reports shift
on its own events, a window on its own, and a touch screen has none. A surface
that wants shift-to-select reads its own event and sets the mode; the state
machine stays the one `Input` already was. `Input.Dragged()` is the rubber band
for a surface to draw.

### `Live.Rebuild` keeps the view

This is a **change to documented behaviour**, made deliberately. `Rebuild`
promised that it "forgets where the view was zoomed to", and that is wrong for
the case rebuilding now exists to serve: the caller is reacting to something
the reader did, and answering them by discarding their zoom is a chart that
fights back.

Restoring needs the new panels, and panels are what a render announces — so
`Rebuild` renders once into its own recording to find them. On a single-panel
or shared-axis chart the scales are the same objects and nothing was ever lost;
the free-scale facet is the case that was actually broken, because it builds a
fresh clone per panel.

`Live.View` and `Live.SetView` are the same capability spelled out, for a
caller who wants it across something wider. A caller who genuinely wants a
fresh start has `Live.Autoscale`.

### `Plot.SetLayers`

`Plot.Add` appended and nothing removed. A chart that gains a highlight layer
on every pointer move gains one layer per move, so the appending had to have a
counterpart. Building a fresh `Plot` each time is the alternative and a worse
one: it discards the scales, and with them the reader's zoom.

## Consequences

- **There is no `Live.Highlight` and no selection state inside refract.** The
  host owns which rows are emphasised. This is the whole decision, stated as
  the thing that is absent.
- **There is no `Grid.Live`.** `Grid` composes several different plots into one
  *document*; making it interactive means one hit index attributing a panel
  back to a plot it does not own. Several interactive charts are several
  `Live`s on several surfaces, wired by the host — the same answer as the
  linking one, for the same reason.
- **Nothing about a link appears in the JSON spec.** A specification carries no
  handlers today and gains none. A link is a program.
- The overlay layer bucket H asked for — a tooltip, a crosshair, a brush
  rectangle refract itself draws — is **still not here**. `Input.Dragged`
  hands the host the rectangle to draw meanwhile, and nothing here is wasted if
  an overlay lands later: it would draw the same rectangle from the same state.
- `examples/linked` records the thing that is easy to get wrong, and it is not
  about linking at all. A sankey's geometry is a property of its whole edge
  list, so highlighting some flows with `Faceter.Subset` lays out *three* edges
  and draws them beside the five they meant to mark. The highlight has to be a
  colour over every edge. A mark whose layout is a function of one row — a
  scatter, a bar, a rect — can be overdrawn with `Subset`. Which applies is a
  property of the mark.

## Revisit if

Enough programs write the same wiring that the shape of it is obvious rather
than guessed at. A helper that took "this key, that chart, these rows" would
then be sugar over a pattern with evidence behind it, which is a different
proposition from an engine designed before anyone had used the parts.

The overlay layer is the other one, and it is a record of its own: it needs a
seam in `render`, which is the only package that knows drawing order.
