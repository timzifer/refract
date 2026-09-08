# 0047 — A legend is furniture that answers to a pointer; hiding is the chart's, toggling is the caller's

**Status:** Accepted · **Date:** 2026-09-08

## Context

[ADR 0046](0046-overlay-layer.md) closed with the case it had deliberately not
taken:

> An overlay wants to be hit-testable after all — a legend whose entries can be
> clicked to hide a series is the case, and it is a real one. […] a clickable
> legend is furniture that answers to a pointer, which means the *guides* would
> need to be announced, with their own kind, so that a hit on one is
> distinguishable from a hit on a mark.

This record is that change. Two questions, and they are separable: how a
pointer finds a legend row, and what "hidden" means.

## Decision, part one: the legend is announced, with its own kind

`render.LegendEntry` is an optional interface beside `Observer`, in the shape
`LayerAxes` and `EndData` already established. It reports each row: the layer
it stands for, its label, the rectangle it occupies, and whether that layer is
currently hidden.

`interact.Index` indexes those as marks of a new kind, `Guide`, and that
separation is the whole point. [ADR 0015](0015-hit-testing.md)'s rule — a
pointer landing on a guide has not landed on anything a reader would ask about
— still holds for *data*: a hit on a swatch must not be confusable with a hit
on the thing the swatch stands for, or a tooltip would describe a row that is
not under the pointer. So a `Guide` hit reports panel −1, no `X`, no `Y` and no
`Row`; what it carries is `Layer`, `Series` and `Hidden`.

The row's rectangle spans the legend's width rather than hugging the swatch, so
the gap between a swatch and its label is part of the same target. A reader
aiming at a word should not have to hit the word.

**`Live.Move` searches the margins for a guide and for nothing else.** A hover
outside every panel is over no data and reports no hit — that gate is what
stops a mark near the panel edge being reported from outside it, where it would
mean something different. A legend lives in the margins and is the one piece of
furniture out there worth finding, so it is the one exception, and it is spelled
as one.

`Live.Click` needed no change: it never gated on the panel.

## Decision, part two: hidden is a property of the chart, not of the layer

`render.Chart.Hidden []bool`, indexed by layer.

The alternative was a `geom` option — `geom.Hidden(true)` — and it is wrong
twice. A geom that knew whether it was being shown would be carrying a fact
about a *reader* rather than about its data, and `geom.Geom` cannot gain a
method to ask it, so `render` would have to go through `Describe`, which a
third-party mark need not implement. Visibility is a statement about the chart,
and the chart is where it goes.

A hidden layer:

- **is not drawn**, and is not announced to the `Observer` — so a pointer where
  it used to be finds whatever is behind it rather than a mark nobody can see;
- **still trains its scales**;
- **still has its legend row**, dimmed to a third opacity.

### The axes deliberately do not move

This is the decision most likely to be questioned, so the reason is here rather
than in a comment. A toggle is a reading aid — *let me see this one without
that one on top* — and an axis that rescaled every time a row was clicked would
make the two readings incomparable, which is the thing the toggle was for. It
is the same argument `examples/stream` makes for pinning a live chart's axis
and that [ADR 0044](0044-transitions.md) makes for `Transition.Rescale` being
off by default.

A caller who wants the axes to follow what is left is making a different
statement — *this series is not part of this chart* — and makes it with
`Plot.SetLayers` and `Live.Rebuild`, which genuinely removes the layer.

### The row is dimmed, not dropped

A row that vanished would take with it the only way of getting the series back.
It would also change the legend's length, which moves every row below it —
so the reader's second click would land on a different series than the one they
were aiming at when they made the first.

## Decision, part three: refract does not wire the click

There is no `Live.LegendToggle(true)` mode. The mechanism is `Live.Toggle`,
`Live.Hide`, `Live.IsHidden` and `Live.ShowAll`, and the wiring is four lines
in the caller:

```go
p.On(refract.Click, func(ev refract.Event) {
	if ev.Hit.Kind == refract.Guide {
		live.Toggle(ev.Hit.Layer)
	}
})
```

That is the same answer this library gives everywhere a pointer means something
— [ADR 0045](0045-linked-views.md) most explicitly — and it is not dogma here.
A legend that always toggled would be wrong for a chart whose legend *selects*
rather than filters, one where clicking a series opens something else, or one
where only one series may be shown at a time. Four lines is the price of all
three of those being possible.

## Consequences

- **A layer contributing several legend rows toggles as one.** A pie, a stack
  and a waffle name themselves for each of their rows, and turning any of them
  off turns the layer off — the rows are one drawing and there is no way to
  draw a third of it. A caller who wants them independent splits the layer,
  which is what makes them independent in the data too.
- **`Index.MarkCount` grows by one per legend row**, and a chart with a legend
  now has reachable marks outside its panels. `TestNothingOutsideAPanelIsHitAsData`
  is the boundary: out there, a `Guide` is findable and nothing else is.
- **Hiding is per surface.** `Plot.chart()` copies the slice, so two `Live`s
  over one plot are two readers and one of them putting a series away is not
  the other one doing it. `Plot.HideLayer` is the model-level statement, for
  the same reason `Plot.Overlay` exists: otherwise `Plot.Render` and
  `Live.Draw` would disagree about what the chart is.
- **Hidden state survives a `Rebuild`**, by index. Adding a layer keeps the
  earlier ones hidden; *inserting* one in the middle shifts what the indices
  mean, which is worth avoiding while anything is hidden.
- **A hidden layer still costs its `Train`.** That is what keeps the axes still,
  and it means hiding a series does not make a slow chart fast. Removing the
  layer does.
- Nothing about the JSON spec changed. Whether a reader has put a series away
  is not a fact about a chart, the way where their pointer is is not.

## Revisit if

Someone wants the axes to rescale on toggle *and* the layer to stay in the
legend. Both are reasonable and they conflict; the way out would be for
`Hidden` to distinguish "not drawn" from "not counted", which is a second flag
and a second set of consequences rather than a change to this one.

A colourbar or a size key wanting to be clickable is the other. Neither maps to
a layer the way a legend row does — a colourbar row is a *value*, not a series
— so the announcement would need a vocabulary for what was clicked, and that is
its own record.
