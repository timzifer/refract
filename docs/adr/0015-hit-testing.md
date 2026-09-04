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
  position, not from a row number carried alongside. That is what keeps the
  index free of per-row bookkeeping, and it is exact for the position that was
  actually drawn.
- Marks are ranked by specificity and then by distance: a vertex the pointer is
  near beats a shape it is merely inside, which beats a label. Within a rank,
  the later mark wins, because it was drawn on top.
- A watched render is serial. The observer is told things in order, and two
  panels drawing at once have no order to be told in.

## Consequences

- Hit-testing is correct by construction for every geom, including ones that
  do not exist yet: a third-party layer that emits IR is hit-testable without
  implementing anything.
- What the reader can point at is exactly what was drawn. On a decimated chart
  the hits are the rows that survived — which is right, because those are the
  rows on screen, and a tooltip for a row that was reduced away would be a
  tooltip for something invisible.
- A hit reports data values rather than a row number. For a tooltip, an
  annotation or a readout that is the useful half; for "open the record behind
  this point", the caller has the x value and a lookup of its own to do. Adding
  row identity would mean carrying it through decimation, which is the
  bookkeeping this design avoids.
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
