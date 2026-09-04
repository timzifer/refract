# 0013 — The Arrow adapter is its own module, and borrows only where it can

**Status:** Accepted · **Date:** 2026-09-04

## Context

CONCEPT §7 has always said "Arrow adapter as a separate module (`refract/arrow`)
so the core never links Apache Arrow", and v0.4 is where it lands. What was left
open is what the adapter actually promises about memory.

Arrow's whole value here is that it is already columnar, already contiguous,
already the shape refract's data layer wants. `data.Source` is documented to
return read-only views and to borrow rather than copy where it can, so the
temptation is to promise zero copy across the board.

That promise cannot be kept honestly. An Arrow `int32` column is not a
`[]float64` and never will be without a copy. A timestamp is an integer plus a
unit in the schema. A dictionary-encoded string column is two arrays. And a
float64 column with nulls is *nearly* right — the values are there — but the
nulls have to become NaN somewhere, and Arrow's buffer is not this package's
memory to write into.

## Decision

**A separate module, and one borrow: a `float64` column with no nulls.**

- `github.com/timzifer/refract/arrow` is a nested module with its own `go.mod`,
  pinning `apache/arrow-go` to an exact version — the same arrangement, for the
  same reason, as `backend/gg` ([ADR 0001](0001-module-layout.md),
  [ADR 0006](0006-gg-coupling-surface.md)). The core's dependency graph is
  unchanged and CI still asserts it is stdlib-only.
- `Float64Column` on a null-free `array.Float64` returns Arrow's own buffer.
  That is the case where the two libraries agree exactly, and it is the case a
  numeric pipeline actually produces.
- Everything else converts once, on first use, and caches. A record with forty
  columns and a chart that plots two pays for two.
- **A null becomes NaN** in a numeric column, the zero time in a temporal one,
  the empty string in a categorical one. NaN is the value refract's
  missing-data policies are written against, so an Arrow null is gapped,
  interpolated or rejected by the same `geom.OnMissing` setting that handles a
  NaN from anywhere else — rather than by a second, parallel notion of missing.
- Dictionary-encoded strings are decoded rather than rejected: dictionary
  encoding is how Arrow spells "categorical", and a categorical column is
  exactly what an ordinal axis or a facet is asking for.
- `Materialize` copies everything into a `data.Table`, for a caller who wants
  to release the record.

## Consequences

- Importing `refract` still links nothing. Importing `refract/arrow` links
  Arrow, which is the caller's choice and their existing dependency.
- The lazy cache means a `Source` is not safe for concurrent *first* use.
  That matches `data.Rows`, which faceting already relies on, and column
  resolution happens during `Train` — before any panel draws. `Materialize`
  is the way out for a caller who wants no caveat at all.
- The adapter borrows the record without retaining or releasing it. Arrow's
  memory is reference-counted and the caller owns that decision; a library that
  called `Retain` behind the caller's back would be worse than one that says
  "keep it alive".
- A column type the adapter does not know reports `ok == false` from the typed
  accessor, which is exactly what `data.Source` already means by "not numeric".
  No error type, no partial source.

## Revisit if

Arrow's `float16`, decimal or extension types start showing up in charts.
`float16` in particular would widen fine; it was left out because nothing that
plots produces it yet, and an untested conversion is worse than an absent one.
