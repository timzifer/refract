# 0043 — Mark identity is a column the caller names, resolved outside the geom

**Status:** Accepted · **Date:** 2026-09-08

## Context

[CONCEPT §14](../../CONCEPT.md#14-roadmap--milestones) has listed animation as
blocked on one sentence since v0.5:

> Tweening needs mark identity across frames — a join key — and the nearest
> thing that exists is `geom.Rows`, which holds only within one frame.

`geom.Rows` reports which *source row* is behind each mark. A row is an index
into a table as it stands for one frame. Append to the table, filter it, or
window a stream and the numbers renumber: row 7 in frame N and row 7 in frame
N+1 are the same measurement only if nothing happened in between, and the
whole point of an animation is that something did.

The same gap shows up without any animation at all. A hover in one chart that
should highlight something in another needs a name for the thing under the
pointer that the *other* chart also knows, and the two charts are drawn from
two different tables. A row number is meaningless across that boundary.

Three places the identity could come from.

**Invent one.** Hash the row's values, or number the marks as they are drawn.
Both are identities nobody asked for: a hash changes when any value changes,
which is exactly when a tween most needs the row to stay the same thing, and a
draw order is not a fact about the data.

**Widen the IR.** Tag every drawing call with a layer id and a row number.
That is the change [ADR 0015](0015-hit-testing.md) already refused, on
[ADR 0007](0007-per-mark-colour.md)'s grounds: it puts an identity channel into
an interface every third-party backend implements, for the benefit of a caller
none of them have.

**Let the caller say.** The data usually already carries the answer — an id, a
name, a stage, a symbol. It is the one identity that survives the table being
rebuilt, because it is a value rather than a position.

## Decision

**A layer's identity is a column the caller names, and it is read from outside
the layer.**

- `geom.KeyBy(col)` names it. Nothing in `geom` reads it: a layer draws exactly
  what it drew before, and the column need not be one the mark plots.
- The key for a hit is read from the layer's own `data.Source` at the row
  `geom.Rows` already reports. `geom.Faceter` already exposes both halves of
  what that needs — `Source()` and `Subset(rows)` — and about twenty geoms
  implement it, so **nothing had to be implemented to make this true**.
  `geom.Sourced` is the read half spelled on its own, for a caller that will
  never split a layer.
- `geom.Geom` gains no method. `Desc` gains a field, which is how a
  configuration has always been written down and read back.
- The IR gains nothing. ADR 0015 and ADR 0007 both stand.

### The key lands on `Event`, not on `Hit`

`interact` imports `coord`, `ir` and `scale` and nothing else. Reading a key
means reading the data and knowing which geom drew a mark, which is the root
package's knowledge, not a spatial index's — so a `Hit.Key` would either drag
`data` into `interact` or be a field that is empty when the `Hit` comes from
`Index.At` and full when it comes from `Live`. A field that lies half the time
is worse than no field.

`Live.fire` fills `Event.Key` in, where the plot's layers and their sources are
both in scope.

### It reads the plot's layer, not the panel's

On a faceted chart those are different objects. A panel holds a `Faceter`
`Subset` copy whose `Source` is the *cut* — one panel's worth of rows — while
`Hit.Row` has already been resolved back through `data.Subset` to the table the
caller handed in. Reading the cut with a handed-in row number indexes the wrong
table, and does it silently and with a plausible answer.

`TestKeyIsReadFromTheHandedInTable` is that bug written down.

### One cell, not the column

`data.Labels` spells a whole numeric or temporal column to answer "what does
this row say here" — the right shape for faceting, which asks once for every
row, and the wrong shape for a pointer, which asks about one row on every move.
`data.Label` answers one cell. `BenchmarkHoverKeyed` pins the hover path at
zero allocations.

### The reverse lookup

`interact.Index` already stored where every row landed and had no way to be
asked. `Index.Locate(panel, layer, row)` is the inverse of `Index.At`, and it
is the far end of the wire between two charts: the first says which row, the
second says where that row is on screen. `RowsOf` and `RowsIn` are the same
data by layer and by rectangle; `RowsIn` is what a brush reads.

All three are linear scans with a caller-owned `dst`, for the reason ADR 0015
gives for the forward search: a tree would have to be rebuilt every frame and
costs more than the scan it saves.

### The document spells it `key`

Vega-Lite already has a `key` channel and already defines it for exactly this:
the field that says which datum is which when a view's data is updated. Taking
the name is what [ADR 0014](0014-json-spec.md)'s "Vega-Lite-shaped" rule asks
for; coining one would have been the misleading option.

## Consequences

- A layer that names no key has none. That is three different situations with
  the same answer — no key column, no row tracking, no row behind the mark —
  and `Hit.Row` tells them apart when it matters.
- Keys are not checked for uniqueness. A duplicate is a caller saying two rows
  are the same thing, and `Panel`, `Layer` and `Row` are still there beside it.
- The key is a `string` because that is what a name is, and because
  `data.Label` already had to exist for a numeric column to be spelled the same
  way a facet panel key and a categorical tick are.
- Nothing pays for it. `Desc.Key` is a string field no render reads;
  `Event.Key` is resolved once per event and only when there is a row and a
  column; the reverse lookups are over data the index already kept.
- **Linked views stay the host's.** `Event.Key` and `Index.Locate` are the two
  ends of a wire and refract does not run anything through it. A link is a
  statement about two charts and refract's model is one chart; the mapping from
  a key in one to rows in another is a lookup, a query, or a rule that no chart
  specification could have carried. `examples/linked` is what that looks like.

## Revisit if

A geom appears whose marks are not one per row, and which still wants an
identity — a boxplot's box, a density raster. Those report no row today and
therefore no key, which is right: the identity of an aggregate is not the
identity of anything that went into it. Such a geom would have to say what its
marks *are*, which is an optional interface beside `Rows` rather than a change
here.
