# 0023 — Notation is typeset by a pluggable typesetter, installed by wrapping the backend

**Status:** Accepted · **Date:** 2026-09-04

## Context

CONCEPT §14 lists "math typesetting for labels (optional, pluggable)" in v0.6.
An axis reading `σ²/√n` is not a string with some characters in it: a
superscript is smaller and raised, a fraction is two things stacked with a rule
between them, a radical has a bar over what it covers. Unicode gets a few of
those approximately right — `²` exists, `ⁿ⁺¹` does not — and the rest not at
all.

Two questions. Where does the typesetting happen, and how does a chart use one.

**Where.** [ADR 0003](0003-text-and-fonts.md) settled the text seam: the backend
shapes, refract places, and `Measure` is the whole of the interface between them.
A typesetter that shaped glyphs would reopen that decision and would need a font
file, which the core does not carry. A typesetter that only *places* needs
nothing but `Measure` — it asks how wide each piece is and decides where the
pieces go.

**How.** A chart draws labels from a dozen places: the title, two axis titles,
every tick, every legend entry, a facet's strip, a geom's own note. Threading a
typesetter through all of them means a dozen call sites that can each forget it,
and third-party geoms that never had it. Worse, *layout* measures labels too —
the margin under an axis is the height of its tick labels — so a typesetter
consulted only at drawing time would size the chart for the markup and draw the
notation.

## Decision

**A `mathtext.Typesetter` returns positioned runs and rules, and `render.Draw`
installs one by wrapping the backend.**

- `Typesetter.Typeset(src, font, measurer) (Layout, ok)` takes the *whole label*
  and decides for itself what in it is notation — the built-in one reads TeX's
  `$…$`. A label with nothing to typeset returns `ok == false` and is drawn as
  ordinary text, which is the common case and stays free.
- A `Layout` is text runs with their own fonts and positions, plus rules: filled
  rectangles, because a fraction bar is a thin filled box and needs no line
  width, cap or join decided elsewhere. It reports width, ascent and descent,
  which is exactly `ir.TextMetrics`, so a measured label is a measured label
  whether or not it turned out to be notation.
- `render` wraps the backend in a `mathBackend` whose `Text` draws the layout
  and whose `Measure` returns the layout's metrics. Every label in the chart
  goes through the backend — including the ones layout measures and the ones a
  geom that does not exist yet will draw — so wrapping it once is the whole
  integration, and no call site knows this exists.
- A chart with no typesetter is not wrapped at all: `refract.Math(nil)` is the
  default and costs nothing.
- **A typesetter never fails a render.** Notation it cannot parse comes back as
  `ok == false`, so the label is drawn exactly as it was written — which is what
  a reader needs in order to see what is wrong with it.
- `mathtext.TeX` is the one that ships: superscripts and subscripts, groups,
  `\frac`, `\sqrt`, `\mathrm`, the TeX spacing commands and a table of the
  symbols a chart label actually reaches for. A single letter is a variable and
  is set italic; a run of letters is a name and is upright.
- `mathtext.Plainer` is the optional second half: the same notation written as
  readable text, for the accessible description
  ([ADR 0024](0024-accessibility.md)). A screen reader asked to announce
  `$\frac{\sigma^2}{\sqrt{n}}$` says "dollar backslash frac".

## Consequences

- Notation works in every label a chart has, at no cost to a chart that has
  none, and the margins are sized by what will actually be drawn.
- A wrapper hides the optional interfaces of what it wraps. `render.Draw` keeps
  the unwrapped backend for the one question that is asked of the backend rather
  than of the drawing — whether it can carry a description — and `ir.Recorder`
  and `interact`'s probe both forward `Describe` for the same reason. This is a
  hazard of the arrangement and is written down where the wrappers are.
- A typeset label is several `Text` calls and some fills rather than one `Text`
  call. Hit-testing indexes what was drawn, so a pointer over a fraction finds
  its pieces; damage compares two recordings call for call, so a chart whose
  notation did not change still compares equal.
- The built-in subset is deliberately small: no matrices, no integrals with
  limits, no growing delimiters, none of the hundreds of macros a document class
  defines. A label needing those wants a typesetting engine, and the interface
  is where one plugs in.
- Italic depends on the backend having an italic face. The SVG and PDF emitters
  do; a raster backend given one font draws the upright face, which is a legible
  chart rather than a failed render. `mathtext.Italic(false)` turns the
  distinction off for a chart whose font stack cannot make it.

## Revisit if

a label needs to wrap, or notation needs to sit inside a paragraph. Both are
paragraph layout, which CONCEPT §5 puts out of scope for the whole project — and
if that changes, the shape of `Layout` (one line, one baseline) is the first
thing that has to change with it.
