# 0010 — One constraint solver for every chart shape

**Status:** Accepted · **Date:** 2026-09-04

## Context

v0.3 adds two things that look different and are not: faceting, which splits
one plot into small multiples, and subplots, which put several plots on one
canvas. Both need panels whose axes line up.

`layout.Compute` already existed and did one panel well: measure the real text
with the real backend, subtract the furniture, hand back a rectangle. The
obvious move was to leave it alone and add a second function for grids.

That would have been two implementations of the same arithmetic, and the golden
files would only have covered one of them. The first divergence would have been
silent: a facet of one panel drawn a pixel differently from the same chart
unfaceted, noticed by nobody until someone diffed two figures.

## Decision

**`layout.Panels` solves the grid, and `layout.Compute` is `Panels` over a
one-by-one grid.** There is one implementation. A chart with no facet and no
subplots goes through exactly the same code as a three-by-two facet.

The solver is a constraint problem rather than a loop because the sizes depend
on each other:

- Each column has one left gutter, the widest Y tick label in it. Each row has
  one bottom gutter. That is what makes panels in a column line up when they
  have scales of their own.
- Every panel is the same size. Panel width falls out of the canvas minus the
  gutters, the strips and the gaps — it is not chosen, it is what is left.
- The guides' height depends on the panel region's height; the panel region's
  width depends on the guides' width. Those are resolved in that order, which
  works because nothing on the right can change the height.

`render.Chart` gained a `Panels` field and `render.Draw` resolves a
single-panel chart into a one-panel grid, for the same reason.

## Consequences

- The golden files test the grid solver on every commit, because every chart
  goes through it. Rewriting `Compute` in terms of `Panels` changed no golden
  file by a single digit, which is the evidence that the two paths agree.
- Panels sharing an axis share the scale *object*, which is what makes the axis
  shared rather than merely similar — and means the device range has to be set
  per panel, twice, once for the furniture pass and once for the data pass.
  There is a test for it; a shared scale left ranged to the last panel would
  draw every other panel's data in the wrong place.
- A free axis needs a copy of the scale, so `scale.Cloner` exists. It is an
  optional interface, like `Definite` and `Band`: a scale that cannot be copied
  can still be shared, and a facet that needs a copy says so with an error
  rather than quietly sharing one.
- The single-panel path pays for machinery it does not use — a one-element
  gutter slice, a strip band of zero height. That cost is a few allocations per
  render and it buys the guarantee that there is nothing to diverge.

## Revisit if

A layout arrives that a grid of equal panels cannot express — a dashboard with
panels of deliberately different sizes, or a marginal-distribution plot where a
narrow strip sits beside a square one. That is a real constraint solver rather
than a wider version of this one, and it should be judged on its own.
