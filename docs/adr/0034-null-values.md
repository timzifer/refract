# 0034 — A null is a missing value, and a column says so beside its values

**Status:** Accepted · **Date:** 2026-09-07

## Context

refract has had one answer for missing data since v0.1: NaN. A geom asks
`scale.Definite` whether a scale has a position for a value, an unplottable row
is gapped, interpolated or rejected by `geom.OnMissing`, and every traversal
over a series reads one precomputed `plottable` answer so that they agree about
where the holes are ([ADR 0008](0008-categorical-axes.md),
[ADR 0011](0011-decimation.md)).

That answer covers a third of the data layer. `data.Source` offers three column
kinds, and only one of them has a NaN:

| Column | A missing value reads back as | What the chart did with it |
|---|---|---|
| `Float64Column` | `NaN` | gapped, interpolated or rejected — correct |
| `StringColumn` | `""` | a band of its own on an ordinal axis, a series of its own in a legend, a panel of its own in a facet |
| `TimeColumn` | the zero time | the year 1 — a domain of three hours stretched across two millennia |

The Arrow adapter documented this and was right to: it has a validity bitmap
and nowhere to put it, so `arrow/v18/columns.go` wrote the type's own stand-in
value and said so in its package doc. The stand-ins are not wrong as values —
there is nothing else a `[]time.Time` can hold — but they are indistinguishable
from measurements. An empty string is a string somebody may have measured; the
zero time is an instant.

Both failures are silent. The chart renders, the axis is drawn, and what it
describes is a value nobody recorded. The temporal one is the loud version
because a stretched domain is visible; the categorical one is worse, because a
band labelled with nothing looks like a rendering bug and is not.

## Decision

**Absence is stated beside the values, through an optional interface, and it
means one thing everywhere it is read: the value is missing.**

### `data.Nulls` is an optional interface

```go
type Nulls interface {
    Nulls(name string) (null []bool, ok bool)
}
```

`data.Source`'s own documentation already promised this shape — "a fourth
column kind arrives as an optional interface beside it, the way `Subset` did"
— and the growth rule in [CONCEPT §15](../../CONCEPT.md#15-versioning--stability)
requires it: `data.Source` is implemented outside this module and never gains a
method.

**A column with no nulls answers `ok == false`.** That is not an optimisation
detail, it is what the interface is for: a reader decides between the borrowed
column and a copy on that answer, and a source that said "here is a mask of all
false" would make every reader copy a column to change none of it. The
zero-copy promise in `data.Float64Columns` is unchanged, and
`TestAFloat64ColumnIsBorrowed` still holds.

### One rule: a null is a missing value

It is read the same way in every channel, which is what keeps it explicable:

- **On a position axis** the row has no place. `geom.column` writes NaN
  whatever the column is stored as, and everything downstream is the machinery
  that already existed — `plottable`, `OnMissing`, the segmenting and
  interpolating traversals, the stacked layer's `groups.ok`. No mark was
  changed to know about nulls, including the ones that do not exist yet.
- **On a categorical axis** the row is not encoded, so the scale never
  registers a category for it. This is the half that could not be expressed as
  a value: a band for `""` sits between the categories somebody measured, and
  no downstream policy can tell it from one.
- **In a group or facet column** the row belongs to no series and no panel.
  `data.GroupBy` skips it and `geom.groups.train` registers no key for it, so
  there is no series called `""` in the legend spending a colour of the
  palette.
- **On the colour channel** the row takes the scale's undefined colour. This is
  the one exception to "not drawn", and it is not a special case: a colour
  scale already has an answer for a value it cannot place
  (`scale.ColorUndefined`), a row with a measurement and an unknown category is
  still a reading, and NaN reaches that answer without a rule of its own. The
  default undefined colour is transparent, so such a row is invisible unless
  the scale was given a colour — which is what every unmappable colour value
  already got.

### The mask is applied where the column is read, not where it is drawn

`geom.column` is the one place, for the reason `data.Labels` is one place: two
readers disagreeing about what counts as absent is the same class of bug as two
readers disagreeing about what counts as one category.

The copy is made on the first value it has to change and not before. `masks`
scans the mask against the values first, so a numeric column whose nulls are
already NaN — which is every Arrow column, since the adapter writes NaN into
them — is handed on rather than copied. A source that marks a row while leaving
a number in it forces the copy, and only then. `series.detach` does the same
for the positions when a *neighbouring* column's null takes a row away, because
`s.x` may be the caller's own slice and a chart may not write into the table it
was handed.

## Consequences

- **A cut carries its mask.** `data.Rows` gathers the mask with the same row
  numbers it gathers the column with — a mask cut differently would mark
  whichever rows happened to land in those slots, which is worse than losing
  it, because it is wrong rather than absent. A cut that left every null behind
  reports none, so a facet panel does not pay for its parent's nulls.
- **`arrow.Materialize` keeps the mask.** It is the call whose purpose is to
  preserve the data, so it is the last place a mask may be dropped.
- **A genuine `""` is still a category and the zero time is still an instant.**
  Without the mask there is no way to say otherwise, and with it there is no
  need to guess: the distinction is what the interface exists to make, and
  there is a test for each direction — dropping every empty string would pass
  the null tests and be a different, wrong feature.
- **Nothing changes for a source that does not implement it.** No mask means no
  nulls beyond the NaNs already in the numbers, which is exactly what every
  `data.Source` written before this record meant.
- **A numeric column may carry a mask too, and the two spellings agree.** A NaN
  and a marked row are both missing and neither outranks the other, so a caller
  who has a mask does not have to rewrite their numbers to use it.
