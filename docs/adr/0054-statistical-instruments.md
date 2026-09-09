# 0054 — A domain reduction belongs in `stat` when its output is the chart

**Status:** Proposed · **Date:** 2026-09-08 · **Implemented:** —

## Context

`docs/chart-types.md` sorts the missing charts by the machinery each needs, and
every bucket in it so far has been missing a *shape*: a rectangle, a position
adjustment, a coordinate system, a channel, a layout. Bucket H was the other
kind — things a chart says that no mark draws.

There is a third kind, and it has been invisible because it looks like nothing
is missing. These are charts whose **shape refract already draws** and whose
**arithmetic it does not have**:

| Chart | Marks it needs | All of which exist since |
|---|---|---|
| Kaplan–Meier survival curve | `Step`, `Area` with `Y2`, a `Track` | v0.10 |
| SPC control chart (X̄-R, I-MR, p, np, c, u) | `Line`, `Scatter`, `HLine` | v0.1 |
| ACF / PACF correlogram | `Bar`, `HLine` | v0.1 |
| ROC and precision–recall curve | `Line` | v0.1 |
| Lorenz curve | `Line`, `Segment` | v0.1 |

Nobody draws these in Go. Nobody draws most of them anywhere outside a
domain-specific package — R's `survminer`, Python's `lifelines`, `qcc`,
`statsmodels` — because a general plotting library has no reason to hold the
arithmetic and a domain package has no reason to hold a plotting library.
refract has an unusual position here: it already holds `Bin`, `KDE`, `Loess`,
`ECDF`, `Hex`, `QQ`, `Squarify`, `Sankey` and `Chord`, all under one rule.

The rule is CONTRIBUTING's: a reduction in `stat` is a pure function, numbers
in and numbers out, with an `Append` form and a determinism test, and none of
them ever sees a string. The question this record answers is not whether these
five are pure — they are — but **where the line is**, because "put the field's
arithmetic in `stat`" has no natural end and a plotting library that acquires a
statistics department has lost.

## Decision

### The admission test

**A reduction belongs in `stat` when its output is the chart's geometry, and
there is no reading of it that is not the chart.**

An ECDF is that: the staircase is the estimator. A KDE is that. A loess fit is
that. Squarify is that. By the same test:

- A **hierarchical clustering** is not, and [ADR 0053](0053-tidy-tree-layout.md)
  says so — its output is a tree you then use for other things, and drawing it
  is one of them.
- A **regression model** is not. `stat.Loess` is in because a loess curve has
  no life off the chart; a fitted GLM has coefficients, standard errors and a
  summary table, and none of that is geometry.
- A **meta-analysis** is not, per the same test, and that decides the forest
  plot below.

Applying it admits five and declines three.

### The five

**`stat.KaplanMeier`** — times and event indicators in, a step function and
Greenwood's variance out. The estimator *is* the curve; there is no other
object. It gets a mark, `geom.Survival`, which runs it in `Train` for
[ADR 0028](0028-distribution-stats.md)'s reason — its output is what the axis
describes — reading X as the time and a `geom.Event` channel as the indicator,
exactly as `geom.ECDF` reads its column. The confidence band and the censoring
ticks are opt-ins on the same layer, the way `geom.Outliers` is on a boxplot.
The numbers-at-risk table underneath is **not** the mark's: a table under the
panel on the shared time axis is a `Plot.Track`, a track is a panel, and a mark
cannot make one ([ADR 0031](0031-tracks.md)). The caller adds it, and that is
the correct division rather than a shortfall.

**`stat.ControlLimits` and the run rules** — a centre line, an upper and a
lower limit for each chart in the family, and the Nelson / Western Electric
rules as a pass returning the indices of the points they flag.

This one gets **no mark**, and the reason is domain-correct rather than
economical. Control limits are computed from a *baseline* period and then
frozen; new observations are judged against limits derived from data that is
not on the chart. That is the whole method — phase I establishes the limits,
phase II watches against them. A mark that recomputed its limits from the
points it was handed would be wrong for the principal use, silently, and would
be right only for the exploratory case. So the caller computes the limits once
and draws three `HLine`s, which is explicit about which data they came from.

The run rules return a **selection**, not geometry — a set of row indices —
which is a shape v1.7 already has a home for: flagged points are a
`geom.ColorBy` over a derived column, or a selection handed to the linked-views
machinery of [ADR 0045](0045-linked-views.md). No new channel.

The limits themselves got cheaper to show while this record was being written.
[ADR 0049](0049-paths-colour-in-classes.md) taught `Line` and `Step` to colour
in classes, and it interpolates the crossing rather than starting the new colour
at the next row — so `ColorBy` over `scale.Threshold` puts an out-of-limit run's
colour change on the limit, which is the reading the chart exists for. That is a
second customer for 0049 arriving before its first release, and it is why this
record needs no drawing machinery at all.

**`stat.ACF` and `stat.PACF`** — a column in, correlations per lag and a
confidence bound out. Stat only, no mark: the drawing convention genuinely
varies between bars, stems and points, and a mark would have to pick one
without there being a reading that distinguishes them.

**`stat.ROC`** — scores and labels in, the curve and its area out. It is two
ECDFs read against each other and sits beside `stat.ECDF` in the same file.
Precision–recall is the same walk with the other two ratios.

**`stat.Lorenz`** — cumulative share against cumulative population, with the
Gini coefficient as the area. ECDF family again.

All five take **sorted input and do not sort**, per `stat`'s existing rule and
for its existing reason: sorting means a buffer, and the geoms already keep
one.

### The three declined, and what they are instead

**A forest plot** is `geom.ErrorBar` plus `geom.Text` plus a `Track` for the
study labels, and it can be drawn today. What it also has is a pooled estimate,
and pooling is a meta-analysis — fixed or random effects, a heterogeneity
statistic, a choice of estimator — which fails the admission test on every
clause. It is an **example**, and a good one, because it demonstrates four
pieces of v1.3 and v0.10 machinery in one figure.

**A funnel plot** is a scatter of effect against standard error with triangular
pseudo-confidence contours. The scatter is a scatter; the contours are curves
defined by a formula in the plane, which is a `geom.Locus`
([ADR 0050](0050-locus-annotations.md)) and needs nothing from this record.

**A Pareto chart** is bars sorted descending with the cumulative percentage on
the secondary axis, and the secondary axis shipped in v1.3
([ADR 0037](0037-secondary-axis.md)). It is a recipe, and so is
**Bland–Altman**: a scatter and three reference lines.

Naming these is part of the decision. A catalogue that lists a chart as missing
when it is three lines of composition is the wish list `docs/chart-types.md`
was written to stop being.

## Consequences

| | |
|---|---|
| `render`, `ir`, `coord`, `scale`, `layout` | unchanged |
| `stat` | five reductions plus their `Append` forms, determinism tests, and no dependency — `math` only, as ever |
| `geom` | one mark (`geom.Survival`), one channel (`geom.Event`) |
| `spec` | a `"survival"` mark; the rest is composition and needs no vocabulary |
| `docs` | three recipes and two examples, which is most of the work |
| Charts unlocked | survival curves with a risk table, the SPC family, correlograms, ROC and PR curves, Lorenz curves, and — by composition — forest, funnel, Pareto and Bland–Altman plots |

This is the cheapest bucket in the catalogue and the one with the widest reach,
because it adds almost no architecture. That is also the argument for doing it
late rather than early: nothing else waits on it.

## Not in scope

- **Fitting, modelling and inference.** No regression beyond the loess that
  ships, no hypothesis tests, no distribution fitting. The admission test is
  the boundary and it is meant to be enforced.
- **Recomputed control limits**, per the argument above.
- **Censoring beyond right-censoring** in the survival estimator. Left- and
  interval-censoring change the estimator, not the chart, and would be argued
  on their own evidence.
- **A `stat` that sees a string.** Unchanged. Group labels are interned in the
  geom, where the order is decided.

## Revisit if

- A sixth reduction is proposed and the admission test does not settle it
  cleanly. The test is the thing this record is really for, and a case it
  cannot decide is evidence the test is wrong rather than that the case is.
- The run rules' selection turns out to want to be a first-class channel
  rather than a derived column. That is a question for
  [ADR 0045](0045-linked-views.md)'s machinery, not for `stat`.
