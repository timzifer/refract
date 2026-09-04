# 0015 — Hit-testing indexes the marks a render emitted, told apart by an observer

**Status:** Accepted · **Date:** 2026-09-04

## Context

v0.5 asks for "hover/click/zoom/pan with IR-level hit-testing". The zoom and
the pan are arithmetic on a scale; the hard half is the hit test, which has to
answer "which row of which layer of which panel is under this point" from
something.

There were three places that something could come from.

**Ask the geoms.** Add a `Pick(pt, frame) (row, bool)` to every layer. The
trouble is that a geom's positions are computed inside `Build`, after the
missing-data policy has split the series and after decimation has thrown most
of it away. A `Pick` would have to redo all of that, identically, in a second
implementation — and the day the two disagree is the day a tooltip points at a
row that is not on screen. Two implementations of one projection is a bug
waiting for a Tuesday.

**Widen the IR.** Give every drawing call a tag: a layer id, a row number.
That is the change [ADR 0007](0007-per-mark-colour.md) exists to refuse. It
would put an identity channel into an interface that every third-party backend
implements, for the benefit of a caller none of them have.

**Watch the render.** The marks a backend receives *are* what the reader can
see — after the policy, after decimation, in device space. What they lack is
only the knowledge of which layer emitted them, and refract knows that at the
moment it calls `Build`.

## Decision

**A hit index is built by observing a render: the backend is wrapped, and the
renderer announces which panel and which layer is drawing.**

- `render.Observer` has two methods, `Panel` and `Layer`. It draws nothing and
  cannot change what is drawn. `render.Chart.Observer` is nil for an ordinary
  render, so nothing else pays for this.
- `interact.Index` implements that interface and `Index.Watch(b)` returns an
  `ir.Backend` that forwards to `b` and indexes what it sees. There is a test
  that a watched render emits byte-identical calls to an unwatched one — which
  is what lets one set of golden files cover both.
- Only marks emitted inside a layer are indexed. The grid, the axes, the
  titles and the guides are furniture; a pointer landing on a grid line has not
  landed on anything a reader would ask about.
- A filled or stroked path is indexed **one mark per subpath**, not one per
  call. A layer draws all its bars in a single path — that is what
  `geom.groupByColor` batches to — so a mark per call would make the whole row
  of bars one shape, and pointing at the fourth bar would report whichever
  corner of whichever bar happened to be nearest.
- The data values come from inverting the panel's scales at the mark's
  position. That is exact for the position that was actually drawn, and it
  needs nothing from the geom.
- Marks are ranked by specificity and then by distance: a vertex the pointer is
  near beats a shape it is merely inside, which beats a label. Within a rank,
  the later mark wins, because it was drawn on top.
- A watched render is serial. The observer is told things in order, and two
  panels drawing at once have no order to be told in.

### Row identity, added afterwards

Values answer "what is here". They do not answer "which row is this", which is
what highlighting the matching row of a table beside the chart needs — and a
neighbouring row highlighted confidently is a wrong answer rather than a
missing one. So a second, opt-in channel carries it.

**A geom reports where each of its rows landed, separately from what it drew.**

- `geom.Rows` is a one-method interface; `geom.Frame.Rows` is nil for an
  ordinary render, and every geom checks it before doing the bookkeeping.
  `render.Chart.RowSink` is what turns it on; `Live.TrackRows` is the switch a
  caller reaches for.
- The report is of *marks*, not of the points of a drawing call. Those are not
  the same points: a smoothed line is a Bézier path whose control points are
  not measurements, a staircase draws two points per row, a bar is four corners
  around one value. Attributing rows to a call's points would attribute them to
  whichever encoding the geom happened to use.
- A hit resolves its row from the nearest position its own layer reported.
  Confining it to the layer is what stops two crossing series taking each
  other's rows.
- Rows are resolved through `data.Subset` before they are reported. Faceting
  cuts each layer down to its panel's rows with `data.Rows`, and a row number
  relative to a cut nobody holds is not an answer — so what comes out is a row
  of the table that was handed in.
- Not every mark has a row. A boxplot's box aggregates many, a density raster
  is not a mark, an interpolated point across a gap was never measured, and a
  third-party geom that does not implement `geom.Rows` reports none. All of
  those leave `Hit.Row` at -1 rather than guessing a nearby one.

It is opt-in because it keeps a position and a row number per mark. It turned
out **not** to cost per-frame allocations — the buffers are pooled like every
other per-frame buffer, and `BenchmarkWatchedFrame` against
`BenchmarkWatchedFrameRows` is pinned flat in the allocation gate — so the
thing being opted into is memory proportional to the marks on screen, not
speed.

## Consequences

- Hit-testing is correct by construction for every geom, including ones that
  do not exist yet: a third-party layer that emits IR is hit-testable without
  implementing anything.
- What the reader can point at is exactly what was drawn. On a decimated chart
  the hits are the rows that survived — which is right, because those are the
  rows on screen, and a tooltip for a row that was reduced away would be a
  tooltip for something invisible.
- A hit reports data values always, and a source row when row tracking is on.
  Decimation does not get in the way of either: LTTB and MinMax keep *real
  rows*, so a surviving mark is a measurement and the row it reports is the
  row that measurement came from.
- A watched render allocates: the index copies the points a layer draws. That
  is proportional to the marks on screen, not to the rows in the table — a
  decimated million-row line indexes a couple of thousand points — but it is
  not nothing, which is why an unwatched render still allocates nothing that
  grows with its data, and why the allocation gate measures one.
- The search is a linear scan with a bounding-box prefilter. A tree would have
  to be rebuilt every frame, and rebuilding it costs more than the scan it
  saves at the mark counts a chart actually draws.
- The event vocabulary is one `Event` struct rather than the per-kind types
  CONCEPT §13 sketched. Go has no sum types, and `On(kind, handler)` with a
  different handler signature per kind means `any` and a type assertion; one
  struct whose fields say which kinds set them keeps the registration
  type-safe.

## Revisit if

A geom appears whose marks are not where its rows are — a violin, a contour —
because the inversion that turns a mark's position back into a value assumes
they coincide. Such a geom would need to say what its marks mean, and that is
an optional interface rather than a change here.
