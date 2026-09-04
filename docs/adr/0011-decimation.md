# 0011 — Decimation happens at draw time, in device space, by default

**Status:** Accepted · **Date:** 2026-09-04

## Context

v0.4 is the big-data milestone. A column with a million rows drawn into an
eight-hundred-pixel plot emits a million line segments that land on eight
hundred columns of pixels: slower to build, slower to rasterize, megabytes of
SVG, and not one pixel different from the reduced version. Every plotting
library that scales has an answer to this; the questions are *where* the
reduction happens, *what coordinates* it reasons in, and *who asks for it*.

Three things were genuinely open.

**Where.** Reducing in the data layer — a `Source` that hands back fewer rows —
is the simplest to implement and wrong: the scales would then be trained on the
subset, so the axis would report what survived rather than what was measured. A
chart whose axis shrinks when it is drawn smaller is lying about the data.

**What coordinates.** Data space is what every published LTTB implementation
uses. But "how many rows are there per pixel column" is a question about
pixels, and on a log axis equal steps in data are not equal steps on screen: a
reduction bucketing by value would spend most of its budget on the top decade
and starve the bottom one.

**Who asks.** Making it opt-in is safe and useless — the people who most need
it are the ones who have not read far enough to know it exists. Making it
mandatory removes the ability to produce an exact vector chart, which is the
reason to use a vector format.

## Decision

**Reduction happens in `Build`, on projected device coordinates, and the
default is automatic per geom and dataset size.**

- `Train` sees every row. The domain is the data's, always. A reduced chart and
  an unreduced one have identical axes — there is a test.
- `Build` projects the rows it can draw into device space, reduces there, and
  emits the survivors. A pixel column is the unit, on every axis type.
- `geom.Decimate` overrides the choice and `geom.NoDecimation` turns it off;
  `geom.Budget` overrides how many marks survive.

The algorithms live in `stat/`, take plain slices, and are generic over
`float32` and `float64` so that the device-space path costs no conversion:

- **`stat.LTTB`** for a line. Largest-triangle-three-buckets keeps the rows that
  carry the shape and, importantly, keeps *real rows*: every vertex the reader
  sees is a measurement that was taken.
- **`stat.MinMax`** for a staircase and for a band. Four rows per column —
  entry, minimum, maximum, exit — which is visually lossless for a signal,
  where LTTB can weigh a one-sample spike against its neighbours and lose it.
- **`stat.Grid`** for a point cloud. Counting per cell and drawing the counts
  through the IR's existing `Image` primitive, which has been there since v0.1
  for exactly this.

The automatic choice is: a line reduces with LTTB past four rows per pixel
column; a step and a band reduce with MinMax past the same threshold; a scatter
becomes a density raster once its markers would cover the plot area four times
over; a bar and a boxplot never reduce, because those marks already aggregate.
A layer whose colour comes from a column is never reduced automatically — it
draws one fact per mark, and counting them into cells answers a different
question.

## Consequences

- A chart over a million rows is about a hundred kilobytes of SVG instead of
  tens of megabytes, with no option set. The default is the useful one.
- The reduction is per segment, after the missing-data policy has split the
  series, so a gap is still a gap and a reduced line never bridges one.
- LTTB and MinMax both assume rows are ordered along x. That is not a new
  condition: a line geom already connects consecutive rows, so a series it can
  draw is a series they can bucket.
- A density raster is an image, so it does not scale with a PDF reader's zoom
  the way a mark does. That is the honest trade — the alternative at that
  density is a million markers, which no reader has ever zoomed into
  usefully — and `geom.Decimate(geom.NoDecimation)` is there for the person who
  wants the marks anyway.
- `stat` is a new core package and, like everything else in the core, depends
  on nothing.
- `stat`'s `Append` forms are allocation-free, and CI keeps them that way:
  `BenchmarkLTTB` and `BenchmarkMinMax` are pinned at zero allocations per
  operation by `.github/scripts/allocgate.awk`. That is not a performance
  target, it is the reason those functions have that shape.
- The allocation pass that came with this milestone tightened the `ir.Backend`
  contract: a point slice, a path or an image handed to a drawing call is
  **lent for that call**, because it comes from a pool and the next call may
  write over it. Nothing shipped here retained one, so nothing broke — but a
  third-party backend that did would now show the next frame's data, and the
  interface says so.

## Revisit if

A geom arrives whose marks are neither a path, a staircase, a band nor
independent points — a hexbin, a contour — because the auto table above is
written in terms of those four shapes and a fifth would need its own row rather
than the nearest fit.
