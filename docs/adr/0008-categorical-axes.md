# 0008 — Categorical axes ride the numeric `Scale` interface

**Status:** Accepted · **Date:** 2026-09-04 · **Closes:** —

## Context

v0.2 adds ordinal scales, so a bar chart can plot `[]string` region names
against sales. `scale.Scale` is entirely numeric: `Train(vs ...float64)`,
`Map(v float64) float32`, `Invert(pos float32) float64`. A category name is not
a float64, and a bar on a categorical axis needs a *width* that no continuous
scale has.

Three shapes were available:

1. **A second interface.** `CategoricalScale` alongside `Scale`, with geoms and
   `render` branching on which one an axis holds.
2. **Generics.** `Scale[T]`, instantiated at `float64` and `string`.
3. **One numeric interface, with the category index as the domain value.**

## Decision

**Option 3, with three small optional interfaces for what the numeric one cannot
express.**

An ordinal scale's numeric domain is the category *index*. `Map(2)` is the centre
of the third slot. Everything above the scale — `render`, `layout`, the tick
pipeline, every geom's projection loop — stays numeric and needs no branch.

Three optional interfaces carry the rest, each implemented by the scales that
have something to say and ignored by the rest:

- **`scale.Categorical`** — `Encode(label string) float64` and `Labels()`. A geom
  reading a text column encodes it through the axis, which is what makes two
  layers on one axis agree about which category is which. Encoding anywhere else
  would let them disagree.
- **`scale.Band`** — `Bandwidth() float32`. A bar or a boxplot on a band axis
  takes its width from the scale instead of inferring one from the spacing of the
  data. `render` also uses it to suppress the X grid: a grid line at a band's
  centre is drawn straight through the mark it is meant to help read.
- **`scale.Definite`** — `Defined(v float64) bool`. A value with no position:
  a category outside a fixed set, or — the reason this interface is not
  ordinal-specific — zero on a log axis. Geoms treat such a row as missing under
  the layer's own `OnMissing` policy, so a negative reading on a log chart is a
  gap rather than a NaN handed to a backend.

## Consequences

- **No branch in the hot path.** A geom's projection loop is the same code for
  every scale.
- **Generics stay out of the model layer.** `Scale[T]` would infect `Frame`,
  `Chart`, `layout` and every geom signature with a type parameter that only two
  scales would ever vary.
- **A numeric column on an ordinal axis becomes categories**, one per distinct
  formatted value. Asking for an ordinal axis over numbers is asking for equal
  slots; a chart that wanted a numeric line would have asked for `scale.Linear`.
- **`Definite` is how a log axis and a fixed category set share one policy.** They
  are the same failure from the chart's point of view — a row with nowhere to go —
  and having one answer for both is why `OnMissing(Error)` catches a zero on a
  log axis without a special case.
- The optional interfaces are duck-typed, so a third-party scale that implements
  none of them still works, and one that implements `Band` gets bar sizing for
  free. That matters for §17.7 (the extension API), which this record does not
  close: it shows one workable shape, and the freeze still has to happen at v1.0.

## Revisit if

A coordinate system other than Cartesian arrives (polar, geographic), which is
the first thing that would make "the scale maps a float to a float" too small a
contract — or when §17.7 is settled and the extension surface is written down
properly.
