# 0024 — A chart says what it is in three channels: a name, a description, and its data

**Status:** Accepted · **Date:** 2026-09-04

## Context

CONCEPT §14 lists v0.6's accessibility work as "redundant encoding
(patterns/dashes), SVG `title`/`desc`/ARIA, data-table fallback". Three separate
things, and they fail for three separate readers: someone using a screen reader
gets nothing from a `<svg>` full of `<path>` elements, someone reading a
greyscale printout cannot tell two palette entries apart, and someone who wants
the numbers has to squint at a picture of them.

The design questions are where each belongs, and what it costs a chart that does
not ask for it.

Describing a chart means reading its data: how many rows, over what range. That
is a pass over every plotted column. A render must not pay for it — CONCEPT §11
promises a frame's cost does not grow with the data, and `TestARenderDoesNotAllocatePerPoint`
enforces it.

## Decision

**Three channels, in three places, each opted into where it costs something.**

### A name, always

`render.Chart` carries an `ir.Description`, and `ir.Semantics` is the optional
backend interface that receives one before any drawing. A chart's title fills it
in with no work at all, so:

- the SVG emitter writes `<title>`, `role="img"` and `aria-labelledby`, which is
  the difference between "graphic" and "Throughput" for a screen reader;
- the PDF emitter puts it in the document information dictionary;
- the canvas backend sets `role` and `aria-label` on the element, because a
  `<canvas>` is a hole in the accessibility tree and its attributes are the only
  thing that can be said about it;
- a raster backend implements nothing, because a PNG has nowhere to put words.

A chart with no title gets no ARIA rather than an empty `<title>`, which a
screen reader announces as an unnamed graphic.

### A description, when asked

`Plot.Describe` reads the chart's own data and attaches a longer reading: what
each layer plots, over what range, how many rows. It is a **method that does
work** rather than an option that sets a flag, because the work is that pass
over the columns: a chart nobody asked to describe does not pay for one, and a
chart that did pays once rather than per frame. `refract.Description` sets one
by hand instead.

The prose is built in `a11y`, a package that reads the model and is read by
nobody in it — the arrangement `spec` has, for the same reason. A geom that knew
what a screen reader was would be a geom in the wrong package, so a layer says
what it is through `geom.Desc` and `a11y` decides what that means in words.

Notation in a title is put through the typesetter's plain form first
([ADR 0023](0023-math-typesetting.md)): a description is read aloud, and
notation read aloud is markup read aloud.

### The data, on request

`Plot.DataTable` writes the chart's rows as an HTML table — one per layer, with
a caption naming it and a column per field it reads. It is the fallback a
picture cannot be, and it is also the honest answer to "what is actually in this
chart". Everything written is escaped: column names and category labels come
from the caller's data.

### Redundant encoding, as a theme

`theme.Redundant(true)` fills in `SeriesDashes` and `SeriesMarkers`, and a layer
that named neither `geom.Dash` nor `geom.Shape` takes the entry at its own index
— the same way it takes its colour from the palette. Both ladders start with
what a chart already draws, so the first layer is unchanged and the difference
appears from the second onwards.

It is a theme decision rather than a document one because it is about how the
chart looks, and because "our house style is dashed" is a theme in the same
sense that "our house colours are these" is.

## Consequences

- Every chart with a title gained an accessible name, which changed every golden
  file with a title in it. That is a deliberate change to the output and the
  goldens were regenerated with the diff read, per the rule in AGENTS.md.
- The description has to survive being recorded and replayed: an interactive
  chart draws through `ir.Recorder`, and a watched one through `interact`'s
  probe. Both carry `Describe` now — a wrapper that dropped it would cost the
  accessible name to exactly the charts that most need one.
- `geom.Desc` gained `MarkerSet` beside the existing `DashSet`. A circle is both
  the zero value and a shape somebody may have asked for, and redundant encoding
  replaces the first but not the second; without the flag a round trip through
  the JSON spec would turn an unset shape into a pinned one.
- What is *not* here: per-mark `<title>` elements, ARIA descriptions of
  individual data points, and a tab order through the marks. A chart with ten
  thousand points has no useful reading as ten thousand elements, and the table
  is the better answer to the same question. Reduced-motion and contrast
  preferences are the host application's to read and a theme's to express.
- A description is a snapshot. Data that changes after `Describe` leaves the
  description stale, which is why it is a method the caller can call again
  rather than something a render silently recomputes.

## Revisit if

a screen reader's chart support gets good enough that structure — a group per
layer, a label per mark — is read usefully rather than as noise. The IR would
have to carry identity for that, which is what [ADR 0007](0007-per-mark-colour.md)
exists to refuse, so the case would have to be strong.
