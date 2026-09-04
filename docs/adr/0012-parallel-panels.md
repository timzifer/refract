# 0012 — Panels are built in parallel by recording, and replayed in order

**Status:** Accepted · **Date:** 2026-09-04

## Context

CONCEPT §11 promises parallel subplots: "independent subplots build IR on
separate goroutines, composited at the end." The panels genuinely are
independent — each has its own rectangle, its own scales and its own rows — so
this is the part of a render that parallelises cleanly.

Two things stand in the way, and neither is incidental.

**The backend is not parallel and must not become so.** `ir.Backend` is an
immediate-mode sink with a transform and clip stack ([ADR 0002](0002-ir-and-backend.md)).
Two goroutines drawing into one would interleave `Push`/`Pop` pairs and emit
paths inside the wrong clip. Making the interface concurrency-safe would push
that requirement onto every backend author, including third parties, for the
benefit of one caller.

**Panels sharing an axis share the scale object.** That is deliberate — it is
what makes a shared axis shared rather than merely similar
([ADR 0010](0010-panel-layout.md)) — and drawing sets that object's device
range. It is also worse than it looks: `linear.Map` memoises its nicing, so
even *reading* a scale is a write.

## Decision

**Each panel draws into an `ir.Recorder` on its own goroutine, and the
recordings are replayed into the real backend in panel order.**

- `ir.Recorder` is an `ir.Backend` that stores calls in flat arenas — one for
  points, one for path verbs, one for path points — so a recording is a handful
  of allocations regardless of how much was drawn, and a pooled recorder reused
  across frames is none.
- Replay is **in panel order, never in completion order**. A parallel render
  therefore emits exactly the calls a serial one does, in exactly the same
  sequence. That is what lets one set of golden files cover both paths; there
  is a test asserting the two produce byte-identical SVG.
- `scale.Snapshotter` is a new optional interface returning an independent copy
  of a scale *including* its trained domain. Each goroutine takes a snapshot,
  so the sharing is gone without anything a panel draws changing. It is
  deliberately not `Cloner`, which returns an *untrained* copy for a free facet
  axis: the two exist for opposite reasons.
- `Measure` is the one backend call a geom may make while building, so the
  recorder forwards it through a mutex onto the real backend. Giving each
  goroutine its own font stack would be the alternative, and it would let two
  panels disagree about how wide a label is.
- The path is taken only when there is more than one panel, more than one
  processor, and every panel's scales can snapshot themselves. Any of those
  missing, and serial is not a fallback but the right answer.

## Consequences

- A nine-panel facet over half a million rows renders about 15% faster on four
  cores. That is Amdahl rather than a disappointment: training the scales and
  splitting the rows still happen serially, and they are a large share of the
  work.
- The parallel path costs one recording buffer per panel. They are pooled, so
  the cost is paid once per process rather than once per frame.
- A geom must be safe to `Build` concurrently with itself, because faceting
  replicates annotation layers across panels. Every geom here is: `Build` reads
  the state `Train` produced and writes nothing.
- A layer that rasterizes lends pooled pixels for the length of the call, the
  same way it lends a point slice, so the recorder copies an image rather than
  keeping the reference. Copying is not an optimisation to remove later; it is
  what makes the borrow legal.
- `refract.Parallel(false)` and `refract.GridParallel(false)` turn it off, for
  a process that has already committed its cores elsewhere.

## Revisit if

Training becomes the bottleneck. Splitting rows and training scales are still
serial, and parallelising *those* means either per-goroutine partial domains
merged at the end, or a lock on every `Train` call — a different decision with
a different shape, and one worth making on evidence rather than in advance.
