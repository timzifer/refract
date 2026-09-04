# 0020 — Discrete colour is a scale; a layer may contribute many legend entries

**Status:** Accepted · **Date:** 2026-09-04

## Context

Two gaps that look unrelated turn out to be the same gap, and both stand between
refract and every chart in the first batch of
[ADR 0018](0018-coordinate-systems.md).

**Colour from a category has no scale.** `scale/color.go` offers `Sequential`
and `Diverging` — both continuous — and `geom.ColorBy(col, s)` binds a column to
one of them. A qualitative palette exists (`palette.Qualitative`, carried on
every theme as `theme.Palette`) but nothing binds a *column* to it. A layer
picks one colour by its own index (`Frame.Index`), which is right for "this
layer is the blue one" and useless for "this slice is Europe".

**A layer contributes exactly one legend entry.** `Geom.Legend(f) (LegendEntry,
bool)` is a method on the `Geom` interface itself (`geom/geom.go:83`), and
`render.legendEntries` calls it once per layer and dedupes by label
(`render/render.go:286-303`). A pie has N slices inside one layer. So does a
stacked bar, a treemap, a waffle, a sankey.

The constraint that shapes the answer: **§17.7 is still open on purpose.** The
extension API freezes at v1.0, and `Geom` is the middle of it. Adding a second
method to `Geom` now would break every implementation for the sake of a feature
most geoms have no use for, and would spend the freeze before the evidence for it
exists.

## Decision

### A discrete colour scale, shaped like `scale.Categorical`

```go
// scale
type DiscreteColorScale interface {
	Encode(label string) int          // registers on first sight, in order
	ColorOf(label string) ir.Color
	Labels() []string                 // in legend order
}

func Qualitative(p palette.Qualitative, opts ...ColorOption) DiscreteColorScale
```

This is deliberately the shape of `scale.Categorical` (`scale/scale.go:76-83`),
which is the existing precedent for a categorical thing riding an interface built
for numbers, and which ADR 0008 already argued through. `palette.Qualitative.At`
wraps, so a domain larger than the palette repeats rather than fails — the same
behaviour a layer index already gets.

Order is **order of first appearance in the source table**, matching
`geom.groupByColor` and `boxplot.summarise`. ADR 0012 requires it: a parallel
render must be byte-identical to a serial one, and map iteration order is not.

`geom.ColorBy(col, s)` accepts it alongside the continuous scales; which guide a
layer contributes then follows from which kind of scale it was handed — a ramp
gets a colourbar, a qualitative scale gets legend entries.

### Many legend entries, through an optional interface

```go
// geom
type Legender interface {
	Legends(f Frame) []LegendEntry
}
```

`Geom` is untouched. `render.legendEntries` prefers `Legends` where a layer
implements it and falls back to `Legend` where it does not, so every existing
geom and every third-party geom keeps working with no stub — which is exactly the
argument `geom.Guided` (`geom/guide.go:37-46`) already made for colourbars, and
which AGENTS.md records as how this codebase extends a type without breaking
everyone who implements it.

Dedupe-by-label stays: faceted panels repeat their layers, and one legend is the
point. `layout.Panels` already sizes the legend box from the label list
(`LegendLabels`), so more entries need no layout change.

## Consequences

- A pie's legend names its slices, a stacked bar's names its series, and neither
  needed a change to the `Geom` interface. That is the whole point: v1.0 can
  freeze `Geom` at three methods with twenty geoms of evidence behind it.
- `spec` gains a discrete branch on the colour channel — `scale.ColorKind`
  currently spans `sequential` and `diverging` — plus `scale.ColorDesc` fields for
  the palette name. `palette.RegisterRamp` / `RampByName` already exist for ramps
  (`palette/registry.go`); qualitative palettes need the same registry treatment
  or a spec cannot name one it did not inline.
- A layer that colours per mark still batches by colour: `geom.groupByColor`
  emits one drawing call per distinct colour, and ADR 0007 is what stops a
  per-vertex colour channel appearing in the IR instead. A discrete scale changes
  where the colour comes from, not how many calls it costs.
- The label→colour lookup is a reused, cleared scratch map, not an allocation per
  row. Same gate, same rule as everywhere else.
- One drawing call per mark is what hit-testing wants anyway: ADR 0015 indexes
  one mark per subpath, so a slice, a segment or a tile that is its own subpath is
  pointable, and one merged path would make a whole pie a single shape.

## Revisit if

- **A layer needs both a colourbar and a legend** — coloured by a ramp, shaped by
  a category. `Guided` and `Legender` are independent optional interfaces, so
  nothing forbids it; what is unwritten is how `layout` orders two guides from one
  layer, and that should be settled when a chart needs it.
- **A size guide arrives** (a bubble chart's key). It is a third guide kind, and
  the guide column in `layout` would be better generalised once than extended
  three times.
