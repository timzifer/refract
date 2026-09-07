# 0035 — A tick label is described rather than computed, and the description carries a language

**Status:** Accepted · **Date:** 2026-09-07

## Context

Two holes with one shape, and `scale.Desc` already admitted the first of them:

```go
// A tick formatter is a Go function. Format, LogFormat, SymLogFormat and
// TimeFormat therefore have no place in a Desc, and a scale carrying one says
// so through Formatted.
```

`Formatted bool` is an honest field and a complete answer to the wrong
question. It says the document lost something; it does not let the document
hold anything. So a chart authored as JSON — a dashboard's config file, which
is the form [ADR 0014](0014-json-spec.md) exists to serve — could not say that
an axis reads `1,234,567`, `12.5 %` or `€ 1.234,50`. Not "could not say it
conveniently": could not say it at all. The one channel a chart's author most
often wants to set was reachable only by recompiling.

The second hole was never written down, because nothing in the code looked
like a decision:

- `scale/time.go` carries a ladder of layouts — `"Jan 2 15:04"`, `"Jan 2006"` —
  and `time.Format` renders them with Go's own English tables. There is no hook
  past those tables; the package does not have one.
- `data.FormatNumber` and `formatterFor` reach `strconv`, which writes `.` for
  a decimal point and nothing for thousands.

Neither was a choice a caller could make, and for a German or French reader the
first is foreign and the second is *wrong*: `1.234` reads as one and a bit.

## Decision

**The declarative spelling of a label is a string, and it lives beside the
function rather than replacing it. A locale is a plot-level option that reaches
every axis.**

### `NumberFormat` is a spec, and the grammar is small on purpose

```
spec  := prefix "#" [","] ["." digits] [style] suffix
style := "%" | "k" | "e"
```

A string rather than a struct of options, because a string is what survives the
round trip as one field and what a person editing a configuration file can
type. A grammar of refract's own rather than d3-format's, because the subset
worth supporting is five features wide and a familiar-looking notation that
implements a third of what it looks like is worse than a small one that is
documented in full.

`#` marks where the number goes, which is what tells a prefix from a suffix and
is why `"€ #,.2"` needs no escaping rules. The one place the grammar can
surprise is that a style letter counts as one only where the body ends — `"#k"`
is SI and `"# kg"` is a unit — and that is pinned from both sides by a test.

**A spec that names no precision keeps the axis's own.** The number of decimals
that tells adjacent ticks apart is a property of the tick *step*, not of any
one value, and it is what keeps a column of labels aligned; a caller who adds a
currency prefix must not lose it. So `autoFor` returns a *description* of the
axis's choice and the two meet in `numberFormat.label` — rather than the spec
replacing the formatter, which is the shape that would have lost it.

### A Go function outranks a spec, and both are written down

`Format` still wins where both are set: a caller who wrote a formatter has
answered the question, and a spec is what a document can carry rather than a
better answer. But `Desc` now carries *both* — `Formatted` and `Format` — for a
reason worth stating: a Desc that dropped the spec because a function was
present would silently change the chart the day somebody deleted the Go code.

### An invalid spec fails on the side it came from

A spec written in Go source is a literal, so a malformed one panics — the line
`data.Table` already draws about a ragged table. A spec read from a document is
input, so `FromDesc` returns `ErrFormat`. One parser, two callers, and the
difference is which of them can have a bug.

### The locale is a plot option, reaching scales through an optional interface

`scale.Scale` is implemented outside the package and never gains a method
([CONCEPT §15](../../CONCEPT.md#15-versioning--stability)), so the language
arrives through `scale.Localizer` the way a zoom arrives through `scale.Zoomer`
— and `refract.Locale` walks the scales the chart description holds and calls
it. That walk is one place rather than four call sites that can drift, and it
is what reaches a track's own scale and a *free facet axis's clone*, neither of
which the caller ever holds.

**An ordinal scale deliberately does not implement it.** Its labels are the
caller's own categories, and translating those would be inventing data.

**A locale is data, and the ones that ship are the ones refract could get
right.** Eight languages, and `RegisterLocale` for everything else — the same
bargain [ADR 0029](0029-extension-model.md) makes for a third party's scale
kind: the document carries a name, the process carries the tables. Shipping a
table refract cannot check would be worse than shipping none, because a chart
with a misspelled month is a chart nobody can trust the rest of.

**A locale name nothing registered draws in English and is written back out
unchanged.** A document that travels through a process without the tables is
still a chart, and refusing to draw it would be a worse answer; losing the name
would be worse still, because the next process along has the tables.

### Go's time package has no hook, so the layout is split

`time.Format` carries its own English month and weekday tables and there is no
way to reach past them. `localTime` therefore splits a layout on the four
tokens that *name* something — `January`, `Jan`, `Monday`, `Mon` — fills those
from the locale, and hands every other run to `Format` unchanged. Numbers,
padding, zones and fractional seconds are still Go's own, which is what keeps a
layout meaning what its documentation says it means. The long forms are matched
first, or `January` reads as `Jan` followed by a literal `uary`.

## Consequences

- **A chart that never mentions a locale draws exactly what it drew.** English
  is the default, `punctuate` with English's separators is the identity on
  `strconv`'s output, and a time scale with no locale takes `time.Format`
  itself rather than the splitting path. Every golden file in the repository is
  unchanged, and there is a test asserting that naming English changes nothing.
- **`scale.Labeller` fell out of it, and is worth having on its own.** A tick
  label is not the only place a chart writes a number: a tooltip, a data table
  and an accessible description write the same values, and a chart whose axis
  says `1.234,5 €` while its tooltip says `1234.5` has been localised in one
  place. `LabelOf` writes a value the way its axis would, precision included.
- **A percentage moves the axis's precision with it.** An axis whose step is a
  quarter needs two decimals to tell its ticks apart and its percentages need
  none, so the `%` style shifts the derived precision by two. Keeping it would
  have written `50.00 %` on an axis that only ever has quarters.
- **The document carries one `format` field, not two.** A numeric scale reads
  it as a number format and a time scale as a layout, because the scale's own
  type already says which it is — and Vega-Lite spells both of its equivalents
  `format` as well, on the axis object refract does not have.
