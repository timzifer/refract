# 0014 — The JSON spec is Vega-Lite-shaped, not a Vega-Lite subset

**Status:** Accepted · **Date:** 2026-09-04 · **Closes:** CONCEPT §17.3

## Context

v0.5 is the milestone that lets a chart be written down. CONCEPT §14 asks for
"full-spec JSON serialization (Vega-Lite-compatible where feasible)", and §17.3
records the question that phrase leaves open: *strict round-trippable subset, or
"inspired by"?* It has been open since v0.1 on purpose — deciding it before
there was a spec to decide about would have been guessing.

The three candidates, and what each costs.

**A strict Vega-Lite subset.** refract would emit documents a Vega-Lite renderer
can draw, and read the ones it can express. The appeal is obvious: an ecosystem
of editors, viewers and validators for free. The cost is that the subset is
small and shrinking. Vega-Lite has no symlog threshold, no per-layer decimation,
no missing-data policy, no band annotation, no step position, no colourbar
placement — and refract has no transforms, no selections, no aggregate
pipelines, no `repeat`, no `vconcat`. Everything refract has that Vega-Lite does
not would have to be dropped from the format or smuggled through a `params`
bag. A spec that cannot express `geom.Decimate` is a spec that draws a
different chart, and finding that out at read time is worse than not having the
format.

**A refract-native format.** Name everything ourselves, round-trip perfectly,
owe nothing to anyone. The cost is that every reader starts from zero, and that
a format with one implementation tends to acquire whatever the implementation
happens to do.

**Vega-Lite's vocabulary, refract's semantics.** Borrow the names and the
structure wherever the concept exists in both, and be explicit where it does
not.

## Decision

**The document is Vega-Lite-shaped. It is not a Vega-Lite document, it does not
claim to be one, and its `$schema` says so.**

Concretely:

- Where Vega-Lite has the concept, the document uses Vega-Lite's name and
  Vega-Lite's structure: `data.values`, `data.format.parse`, `mark.type`,
  `encoding.x.field`, `encoding.x.datum`, `encoding.y2`, `scale.type`,
  `scale.domain`, `scale.scheme`, `facet`, `columns`, `resolve.scale.x:
  independent`. A layered spec's shared encodings sit at the top level, where a
  layered Vega-Lite spec puts them. A person who knows Vega-Lite can read a
  refract spec without a manual, which is most of what the compatibility was
  ever worth.
- Where the concept is close but not identical, the nearest true statement is
  made rather than a new word invented: a step is a `line` with
  `interpolate: step-after`, a smoothed line is `interpolate: cardinal` with a
  `tension`, a threshold is a `rule` with an `orient`, a shaded window is a
  `rect` with one, a note is a `text` mark. These are Vega-Lite spellings for
  Vega-Lite-ish things.
- Where refract has something Vega-Lite does not, the property is refract's and
  is named plainly: `mark.missing`, `mark.decimate`, `mark.budget`,
  `mark.densityCells`, `mark.extend`, `mark.barWidth`, `mark.origin`,
  `scale.minorTicks`, `scale.center`, `scale.timeZone`, `config.theme`. No
  Vega-Lite name is borrowed for something that is not what Vega-Lite means by
  it.
- `$schema` is `https://github.com/timzifer/refract/spec/v0.6` (it was `v0.5`
  when this record was written; the dialect moves when it gains a field, and
  v0.6 added a time scale's `origin`). It identifies
  the dialect; nothing fetches it. A Vega-Lite consumer that checks the field
  will refuse the document, which is the honest outcome — the alternative is a
  document that claims to be Vega-Lite and renders as something else.

**What is guaranteed is the round trip through refract.** `spec.Of` then
`Spec.Chart` produces a chart that draws the same marks in the same places, and
there is a test per mark and per scale that renders both and compares the
primitives. Anything that cannot round-trip is an error at write time, not a
surprise at read time: a layer or a scale that cannot describe itself stops
`spec.Of` and names the interface it is missing.

**Serialization is built on description, not on reflection.** `geom.Desc`,
`scale.Desc`, `scale.ColorDesc` and `facet.Desc` are what a layer, a scale and a
facet say about themselves; `spec` translates between those and JSON, and knows
nothing about any geom's internals. That is what keeps the format out of the
model packages and the model out of the format.

## Consequences

- The spec covers every geom, every scale, every annotation and every facet
  refract has, because the format was allowed to grow past Vega-Lite where
  refract has.
- A Go function cannot be written down. A custom tick formatter and an
  unregistered colour ramp are the two places this bites: the formatter is
  lost and `scale.Desc.Formatted` says so, while an unregistered ramp is
  written out colour by colour rather than by name, because losing it would be
  worse than spelling it out.
- Colour ramps are now registered by name (`palette.RegisterRamp`), using the
  scheme names Vega and matplotlib use for the same ramps. `"viridis"` means
  the same thing in a refract spec and in a Vega-Lite one.
- `geom.Describer` and `scale.Describer` are optional interfaces, which means a
  third-party geom that does not implement them still draws and simply cannot
  be serialized. That is the right failure: the alternative is a spec missing a
  layer.
- Reading is lenient where writing is strict. A hand-written document with no
  `$schema`, no parse map and no scale types reads: types are inferred from the
  encodings and the values, exactly as Vega-Lite infers them. A document
  refract wrote always carries the parse map, so the inference only ever runs
  on something a person typed.
- `Plot` implements `json.Marshaler` and `json.Unmarshaler`, so a chart is a
  value `encoding/json` already knows how to handle.

## Revisit if

Vega-Lite grows the concepts this document had to name itself — a decimation
hint and a missing-data policy above all — because then the honest thing is to
adopt its names for them, which is a breaking change to the format and needs
saying out loud.
