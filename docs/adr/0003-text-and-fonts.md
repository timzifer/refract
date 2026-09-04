# 0003 — Text measurement and the font strategy

**Status:** Accepted · **Date:** 2026-09-04 · **Closes:** CONCEPT.md §17.4

## Context

refract owns text *placement* and nothing else: shaping belongs to the active
backend (§5, §9). But layout cannot place anything without knowing how wide it
is, and it has to know before a single primitive is drawn — margins, tick
spacing and label-collision decisions all depend on it.

That produces one hard requirement and one hard constraint:

- **Requirement:** every backend must answer `Measure`, using the same font
  stack it will draw with.
- **Constraint:** the core module is stdlib-only, so it cannot link a shaper —
  and `gogpu/gg` ships no default font, so the raster backend has nothing to
  measure with either until one is supplied.

## Decision

**The `Measure` seam is the whole of refract's text problem.** It is the only
text capability `ir.Backend` requires beyond drawing.

**The core carries a metrics reader, not a shaper.** `internal/fontmetrics`
parses `head`, `hhea`, `hmtx` and `cmap` (formats 4 and 12) with nothing but the
standard library — enough for advance widths and vertical metrics, and nothing
more. No outlines, no `GSUB`/`GPOS`, no rasterization.

**The core carries a metrics table, not a font file.** With no font configured,
`fontmetrics.Builtin` measures against a ~200-byte table of Helvetica advance
widths, which is what `font-family: sans-serif` resolves to closely enough on
every mainstream platform. Embedding a real TTF would put a quarter of a
megabyte into the graph of every consumer, including those who only emit SVG.
Users who want exact SVG metrics pass their own font to `svg.WithFont`.

**The gg backend embeds Go Regular and Go Bold** (`golang.org/x/image/font/gofont`,
BSD-3-Clause), so raster output works with no configuration, and measures with
the very face it draws with.

## Consequences

**"Identical output across backends" is bounded, not absolute — and the bound is
now measured.** `backend/gg/parity_test.go` renders the same chart through both
backends with the same font file and asserts how far apart they are:

- gg hints glyph advances to whole pixels; refract's `hmtx` reader sums
  unrounded advances. Divergence is at most about half a pixel per glyph.
- The two read vertical metrics from different tables. For Go Regular the font
  box height differs by about 15%.
- The resulting plot rectangle agrees to within a few pixels, not exactly.

This is stricter than the "text metrics are approximately identical" caveat in
`CONCEPT.md` §9 — and it corrects it. Geometry is identical *given identical
metrics*; the metrics are not identical, so the layout differs slightly. The
tolerances in that test are where the real number lives. Tightening them is an
improvement; loosening them silently is a regression.

Without a supplied font the SVG path is further off still, because the built-in
table approximates a different face entirely. That is the intended trade: zero
dependencies, correct-looking charts, exactness available on request.

## Revisit if

Someone needs pixel-exact agreement between SVG and raster. The route would be
to measure the SVG path with a real font by default — which costs an embedded
font in the core, and is a different trade, not a free improvement.
