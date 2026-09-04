# 0016 — Streaming is a snapshot and a swap; damage is a diff of two recordings

**Status:** Accepted · **Date:** 2026-09-04

## Context

CONCEPT §7 and §11 have promised streaming since v0.1: "a `StreamSource` with a
snapshot model — produce on one goroutine, render a consistent snapshot on
another", and "the renderer reads an immutable snapshot (copy-on-swap); gg's
damage tracking then repaints only dirty regions." v0.4 explicitly left it out,
because the interesting half is the repaint and that needs a surface to repaint.

Two questions, and they are separable.

**What does a renderer read?** A `Source` is read column by column, over
several calls. A source being appended to between two of them is a source that
disagrees with itself: a chart drawn from one would have more timestamps than
values, and a geom would return a length error or, worse, plot a shifted
series. Locking each column read would make every existing chart pay for a
feature it does not use, and would still not give one frame a consistent view.

**What is repainted?** gg's damage tracking was the plan of record, and it is
the wrong layer: gg is one backend of four, and the browser backend added in
this milestone is not it. Something above the backends has to work out *where*
a frame changed, or every backend works it out separately.

## Decision

**`data.Stream` is not a `Source`. The only way to draw one is to freeze it.**

- `Stream.Append` may be called from any goroutine. `Stream.Snapshot` copies
  the live rows into a buffer the renderer is not reading and swaps the two.
  `Stream.Source` returns a stable `Source` that reads whichever buffer the
  last snapshot filled — so a layer is built once, before any row has arrived,
  and the frame it draws is the frame that was frozen.
- Two buffers, reused. A snapshot is one copy per frame, not per column read
  and not per row, and after the first frame at each size it allocates nothing.
  The lifetime is the price: a snapshot is valid until the next Snapshot, and
  Snapshot belongs between frames rather than during one.
- A `Window` makes the stream a stream rather than a log. Appends are O(1) into
  a ring; the snapshot unwraps it into row order, which is the order a line has
  to be drawn in.
- Columns are numeric only. A timestamp is its Unix nanoseconds — which is the
  domain `scale.Time` already maps — so a time series needs no second column
  type and no per-row allocation to carry one.

**Damage is computed in the IR, by comparing two recordings.**

- `ir.Damage(prev, next, dst)` walks two `ir.Recorder`s call for call and
  returns the rectangles in which they differ, in device space, with
  overlapping rectangles merged.
- It reports `ok == false` when the two are not comparable call for call — a
  different number of calls, a different kind at an index, a transform or clip
  that moved. That is a chart whose *structure* changed rather than its data,
  and a full repaint is the honest answer rather than a list of rectangles that
  describes half of it.
- `ir.Partial` is the one-method optional interface a backend implements to act
  on it. A backend that cannot repaint part of a frame — a file emitter, which
  writes a whole document or none — simply does not implement it and gets the
  whole frame, as before.
- `Plot.Live` ties the two together: record the frame, diff it against the
  last, tell the backend what changed, replay. A frame identical to the last is
  not painted at all.

## Consequences

- The producer and the renderer share no mutable state, and there is a test
  that runs both at once for `-race` to look at.
- The unit of damage is a drawing call. A line is one call, so moving one of
  its points repaints the line's bounding box — which spans the plot. What it
  does not repaint is the title, the axes, the tick labels and the margins,
  which is most of the canvas. Sub-call damage would mean diffing point ranges
  inside a polyline, and a line's ink is spread along its length anyway.
- A chart whose axes rescale every frame repaints more, because relabelled
  ticks are changed calls and a changed tick *count* is a structural change.
  That is a real property and it points at a real practice: pin the axes of a
  live chart. An axis that rescales every frame is also one a reader cannot
  compare two frames of.
- Past `maxDamageRects` regions the list collapses to its own bounding box.
  Damage tracking that produces five hundred rectangles has found a full
  repaint the slow way.
- `ir.Recorder` gained `Bounds`, and `Stroke`, `Fill` and `MarkerStyle` gained
  a `same` comparison. A `Stroke` holds a dash slice, so `==` does not compile
  on it; that is the field a hand-rolled comparison would have forgotten.
- gg's own damage tracking is not used and does not need to be. A gg-backed
  surface can implement `ir.Partial` on top of it later; the rectangles are
  device-space and mean the same thing on either side.

## Revisit if

A geom appears that redraws a large call for a small change — a raster whose
pixels shift by one — because the call-level unit is what makes that a full
repaint of the raster. The fix would be damage the geom declares rather than
damage inferred from the calls, which is a wider interface than this milestone
needed.
