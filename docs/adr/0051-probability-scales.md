# 0051 — A probability scale warps the axis, where a QQ plot warps the sample

**Status:** Proposed · **Date:** 2026-09-08 · **Implemented:** —

## Context

**Probability paper** is an axis warped so that one distribution's cumulative
function plots as a straight line. It is a reading instrument: the reader draws
a line through the points, and the line's slope and intercept *are* the fitted
parameters.

Four kinds of it are in daily professional use, and each names a field.

| Paper | Axis transform | Field |
|---|---|---|
| Normal | Φ⁻¹(p) — the probit | metrology, quality, psychometrics |
| Weibull | ln(−ln(1−p)) against ln t | **reliability engineering** |
| Gumbel / extreme value | −ln(−ln p) | hydrology, flood return periods, structural loads |
| Logit | ln(p/(1−p)) | epidemiology, dose–response, log-odds |

Weibull analysis is the load-bearing one. It is the standard method for
lifetime data — the slope is the shape parameter β, which says whether the
failures are infant mortality, random or wear-out, and the line crosses 63.2 %
at the characteristic life η. The field has dedicated commercial software for
it. It has no general-purpose plotting library at all: R gets there through
`scales::probability_trans` if you know the incantation, matplotlib needs the
third-party `probscale`, and Go has nothing.

### The one refract already has, and why it is not this

[ADR 0041](0041-qq-plots.md) shipped `geom.QQ`, and the two objects are easy to
confuse because they show the same thing. They are opposites:

- **A QQ plot warps the sample.** The observations are plotted against
  theoretical quantiles, and the axis stays linear. The X ladder is therefore
  labelled in z-scores — −3, −2, −1, 0, 1 — which nobody reads as a
  probability.
- **Probability paper warps the axis.** The observations are plotted against
  their own cumulative probability, and the *axis* is warped. The ladder is
  labelled 0.1 %, 1 %, 10 %, 50 %, 90 %, 99 %, 99.9 %, which is what the reader
  came for.

The distinction is not cosmetic, because a mark and a scale compose with
different things. `geom.QQ` is one mark and does one job. A probability scale
is available to every mark there is: a scatter of failure times, an ECDF, a
line, a track, a secondary axis, a facet — and to charts nobody has thought of,
which is the general argument for putting a capability in the scale layer when
it will go there honestly.

## Decision

**A probability scale is a monotone link applied to a probability, and it is
one implementation with four named links.**

```go
p.Y(scale.Probability(scale.Probit, scale.ProbabilityDomain(0.001, 0.999)))
```

`scale.Probit`, `scale.Logit`, `scale.CLogLog` and `scale.Gumbel` are the four,
and a caller may pass a link of their own.

### Why one type and not four

`scale`'s naming rule says each constructor has its own option type, "because
the choices differ: a log axis has a base and a linear one does not". Here they
do not differ. All four links share a domain of (0, 1) exclusive, share the same
tick ladder, share the same `Definite` bound and share every line of the
mapping except one function call. Four types would be four copies of one thing
with the rule cited as the reason, which is the rule being followed off a cliff.

The shape chosen is the one [ADR 0041](0041-qq-plots.md) already chose for the
same question one layer down: `stat.QQ` takes a theoretical quantile function
and `nil` selects the normal. A link here is the same kind of parameter, named
the same way, and **serialised the same way** — a named link round-trips
through `spec` as a string, and a Go function does not and says so. That rule
is now three records old ([0041](0041-qq-plots.md),
[0049](0049-locus-annotations.md), this one) and is worth stating plainly: *a
named member of a small closed family is written down; an arbitrary Go function
is not, and the escape hatch is to materialise its output as data.*

### What it inherits, and what it has to choose

**`Definite` covers the ends.** A probability scale cannot place 0 or 1
anywhere on an axis, which is precisely the relationship a log scale has with
zero and negatives. `scale.Definite` exists for that, `geom` reads it in one
place, and every missing-data policy already downstream of a NaN applies with
nothing added. `TestALogAxisTreatsANonPositiveValueAsMissing` is the test this
one is written next to.

**The ticks are a convention, not a search.** The printed ladder is
0.1 / 1 / 5 / 10 / 20 / 30 / 50 / 70 / 80 / 90 / 95 / 99 / 99.9, symmetric
about the median and crowding both tails — no tick-choosing algorithm produces
it, in the same way none produces a Smith chart's 0.2 / 0.5 / 1 / 2 / 5. The
default is that ladder, clipped to the domain, thinned the way a log scale
thins decades when the axis is short. `scale.TickValues` from v1.2 already
pins something else, and this is the second axis family to need it, which is
mild evidence it was the right thing to add.

**`Nice` is meaningless here** and is not offered; there is no round number to
round to. `ProbabilityFormat` defaults to per cent, because the ladder is read
in per cent, and `scale.NumberFormat` and `scale.Locale` from v1.3 apply
unchanged.

### The charts cost no marks

This is the return on putting it in the scale layer, and it is the whole
argument in three lines:

- **`geom.ECDF` on a probit Y is a normal probability plot.**
- **`geom.ECDF` on a `CLogLog` Y against a log X is a Weibull plot.**
- **`geom.ECDF` on a Gumbel Y is extreme-value paper.**

No new mark, no new stat, no new coord. It is the same move as an icicle under
a polar coord being a sunburst, made in the scale layer instead of the
coordinate one, and it is the reason this record is short.

The one genuine wrinkle is the **plotting position**. An ECDF is a step
function that reaches 1, and 1 is off the end of every one of these axes. Under
`Definite` that last step is simply unplottable and the existing policy drops
it, which leaves a correct chart. It is not, however, what a reliability
engineer wants: the convention there is a median rank, Benard's
(i − 0.3)/(n + 0.4), which never reaches 1 by construction. `stat.MedianRank`
is numbers in and numbers out and belongs beside `stat.ECDF`; whether
`geom.ECDF` gains a plotting-position option or the caller draws a `Scatter`
over the ranks is the one open question in this record, and it is a small one.

### The scale does not fit the line

A Weibull plot's whole purpose is the fitted line, and this record does not
draw it — for [ADR 0041](0041-qq-plots.md)'s reason, quoted because it decides
this too:

> A reference line would assert parameters, so callers add one explicitly when
> they know them.

A scale that fitted a distribution to the data would be asserting the answer
the chart exists to let a reader find. `stat.WeibullFit` returning β and η by
rank regression is a plausible companion and a separate decision; the line it
would justify is `geom.Segment` or `geom.Trend` today.

## Consequences

| | |
|---|---|
| `geom`, `coord`, `render`, `ir`, `layout` | unchanged |
| `scale` | one type, four link values, one option type, `Definite`, a `Desc` field and a registry entry |
| `spec` | a `"probability"` scale type and a `link` string |
| `stat` | possibly `MedianRank`, per the wrinkle above |
| `interact` | unchanged; a hover inverts through the scale and reports a probability |
| Charts unlocked | Weibull probability plot, normal and lognormal probability plots, Gumbel and extreme-value paper, hazard plots, a log-odds axis on any mark |

## Not in scope

- **Fitting**, per the argument above.
- **Confidence bounds on a probability plot.** Beta-binomial rank bounds are a
  real part of Weibull practice and are a stat plus `geom.Area` with `Y2`,
  which already draws a band. They are a separate record if they are anything.
- **Censored data.** Suspensions change the plotting positions, not the axis.
  That is [ADR 0053](0053-statistical-instruments.md)'s territory, where
  censoring is the central question rather than a footnote.
- **A probability *colour* scale.** [ADR 0042](0042-colour-transforms-and-classes.md)
  settled how a ramp compresses its domain; nothing here asks to reopen it.

## Revisit if

- A second scale family wants a closed set of named transforms. Two is a
  coincidence and three is a pattern, and the pattern would be worth naming as
  a shared "transform" concept rather than being spelled out per family.
- Median ranks turn out to be wanted by more than one mark, which would make
  the plotting position an option on the stat rather than on the geom.
