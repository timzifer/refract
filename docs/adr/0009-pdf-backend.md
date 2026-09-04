# 0009 — PDF is a built-in emitter, not a gg recording

**Status:** Accepted · **Date:** 2026-09-04 · **Revisits:** [0004](0004-svg-source-of-truth.md), [0006](0006-gg-coupling-surface.md)

## Context

[CONCEPT.md §14](../../CONCEPT.md#14-roadmap--milestones) puts PDF in v0.3 and
names the route: gg's recording API, played back through
[`gg-pdf`](https://github.com/gogpu/gg-pdf). [ADR 0006](0006-gg-coupling-surface.md)
excluded `gg/recording` from the adapter and said so explicitly — "neither is
needed until animation (post-1.0) and PDF (v0.3)". [ADR 0004](0004-svg-source-of-truth.md)
said the same from the other side: when PDF brings recording in, whether SVG
should follow it there is worth reopening.

So the plan was to take the dependency. Before doing that, the route was
tried.

`gg-pdf` v0.2.1 records fine and produces a valid PDF. It draws no geometry.
Its `FillPath` and `StrokePath` hand the path to `gxpdf`'s
`creator.Surface`, and in `gxpdf` v0.4.0 — the version `gg-pdf` requires — that
method is a stub:

```go
func (s *Surface) FillPath(path *Path) error {
	...
	// TODO: Implement actual PDF content stream generation
	// For now, this validates and prepares for rendering
	return nil
}
```

The same stub is still there in `gxpdf` v0.9.4, the newest release, so bumping
the dependency does not fix it. A chart played back through this path comes out
as a page containing its tick labels and nothing else: no axes, no grid, no
data. That was reproduced end to end before this record was written.

## Decision

**refract emits PDF itself, in the core module, using only the standard
library.** `backend/pdf` is the second built-in emitter, alongside
`backend/svg`, and `refract.PDF(path)` is symmetric with `refract.SVG(path)`.

`gg/recording` stays out of `backend/gg`, so ADR 0006's import rule is
unchanged rather than relaxed.

Two things made this the cheap option rather than the expensive one:

- **PDF's imaging model is the IR's.** Paths of lines and cubics, a fill rule,
  a stroke with cap/join/dash, a clip, an affine transform, an axial gradient,
  a text run with a matrix. Every IR primitive has one operator. The emitter is
  about the same size as the SVG one.
- **The core already carries Helvetica's metrics.** `internal/fontmetrics`
  ships the base-14 Helvetica advance table as its fallback face, because SVG
  output needs to measure text it does not shape. PDF's base-14 Helvetica is
  that exact font. So this backend measures with the font it draws with — the
  only backend other than gg for which that is true, and unlike gg it needs no
  font file to do it.

The `gg-pdf` route would have measured with the Go font and drawn in Helvetica,
because `gg-pdf` hardcodes Helvetica for every run. Even with the path stubs
filled in, every centred label would have been centred on the wrong width.

## Consequences

- PDF output costs no dependency. "Give me a vector chart in a report" links
  the same stdlib-only core as "give me an SVG".
- There are now two vector emitters in the core to keep in step. They share
  their marker geometry through `internal/markers` and their number formatting
  policy, and the golden tests cover both, but a third would be one too many —
  at that point the shared spine should be extracted rather than copied again.
- PDF is one page, no embedded fonts, and no transparency groups. Alpha is a
  graphics-state constant, which is what a faded area fill needs; a gradient
  whose stops disagree about alpha is painted at the strongest of them.
- Text outside WinAnsi cannot be encoded and becomes `?`. Embedding a font
  would fix it and is a bigger change than this milestone.
- §17.2 stays closed the way [0004](0004-svg-source-of-truth.md) closed it, and
  for a stronger reason than before: both vector formats are now built-in, so
  there is no gg vector path to unify with.

## Revisit if

`gxpdf` implements content-stream generation and `gg-pdf` starts drawing
geometry — at which point the question becomes whether one recording-based
backend is worth replacing two working emitters, and the answer is probably
still no. Sooner: if someone needs a font other than Helvetica, or a
multi-page document, both of which are additions here rather than reasons to
change route.
