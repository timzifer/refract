# 0032 — Text is a mark that reads a column, and it labels the box a row spans

**Status:** Accepted · **Date:** 2026-09-07

This record was written after the code it describes, which is the wrong order —
[the index](README.md) says a decision that opens after v1.0 gets a record
before it gets code. The decisions below were real and were argued; they were
argued in a commit message and in `AGENTS.md`, which is not where a reader
looks. Writing it down late is better than leaving it in a log.

## Context

`geom.Note(x, y, text)` places one literal string at one literal position. It
is an annotation, and it is right for what it is: a threshold named "budget", a
peak named "peak".

It was the only way text got into a chart, so labelling *rows* went through it
too — a loop appending one `Note` per row. That works, and what it costs is not
a layer count:

- `Plot.Add` appends and nothing removes. A chart whose rows change has to be
  rebuilt from scratch, and a rebuild takes the reader's zoom and the interact
  index with it. Every other mark reads a `data.Source` and needs no rebuild
  when the numbers change; text was the exception, and it was the exception in
  exactly the charts that change — a live one.
- A note knows nothing about the mark it is labelling. It cannot tell that its
  string is wider than the bar it sits in, so it overruns into the neighbour,
  and a label that overruns reads as naming the neighbour.
- A note cannot know what colour it is drawn on. A layer painted through
  `ColorBy` gives each row its own fill, and a fixed ink is unreadable on some
  fraction of any qualitative palette. The caller can reach back into the colour
  scale and resolve the row's fill themselves, which is asking every caller to
  reimplement the layer's own arithmetic.

The case that forced it: a shopfloor terminal, locked against panning because on
a touchscreen every touch is a swipe. Locked means no hover, no hover means no
tooltip, and a coloured bar with no text in it is just a coloured bar. It is the
same chart [ADR 0031](0031-tracks.md) gave a lane to — a track puts the state
strip below the speed trace, and this puts the state's name inside the bar.

## Decision

**Text is a mark. `geom.Text` reads its labels from a column through `TextBy`,
and when the row spans a box it labels the box.**

### The box is the one `Rect` would draw

Which anchor a layer uses follows from the encoding, and from nothing else:

- Naming neither `X2` nor `Y2` is **point mode**: the anchor is the mapped
  point, laid out by `Align` exactly as a note is.
- Naming either is **box mode**: the anchor is the middle of the box the row
  spans, and that box is computed by `spanOn` — the same function `Rect` uses,
  including its rule that an edge the row does not name is the slot the axis
  implies.

That second sentence is the decision. It means one option list describes the
rectangles and labels them:

```go
opts := []geom.Option{geom.X("start"), geom.X2("end"), geom.Y("lo"), geom.Y2("hi"),
    geom.ColorBy("state", pal)}
p.Add(geom.Rect(src, opts...))
p.Add(geom.Text(src, append(opts, geom.TextBy("label"))...))
```

The alternative — a text layer with a geometry of its own — would agree with
`Rect` on the common cases and disagree on the slot rule, on `BarWidth`, and on
a band scale's bandwidth. Two marks that agree about where a box is only most of
the time is worse than one that cannot know.

### The anchor is the middle of the *visible* box

The mapped box is clamped to `coord.Extent` before its middle is taken, so a bar
half scrolled off the left edge carries its label in the middle of what is left
rather than off-screen with the box's true centre. On a locked chart that is the
difference between a readable strip and one whose left-most bar is anonymous.

The clamp is against the coord's extent rather than `Frame.Area` because that is
the interval the scales map into, which is what the coord answers for — under a
polar coord it is an angle and a radius rather than two edges.

### A label is measured, and one that does not fit is dropped

Every run goes through `ir.Backend.Measure` with the font it will be drawn in.
That is the only backend call a geom makes while it builds, and it is already
sanctioned: `render.syncMeasurer` exists to serialise exactly these calls onto
one font stack, because panels build on separate goroutines and two of them
disagreeing about how wide a label is would draw different text in the same box.

The room a label has is measured as the chord between the box's mapped edges
rather than as its width in the space the scales map into. Under Cartesian the
two are the same; under polar the box's width is an *angle* while what a label
needs is a length, and the chord is the honest answer because it never claims
room the ink has not got.

**A label that does not fit is dropped. `Elide` truncates instead, and is
opt-in.** A truncated label is a claim about a row that the row does not quite
make: a column of "Wareneingang", "Warenausgang" and "Wartung" elides to three
labels that cannot be told apart, and no label at all is honest about that where
three identical ones are not. Where the first characters do identify the row,
the option says so.

This is also why measuring cannot move to `Train`. What a label has to fit is a
width in pixels, and no geom knows that until it is building —
[ADR 0011](0011-decimation.md) draws this line from the other side, and it is
the same line: what depends on how wide the chart is happens in `Build`.

### The ink follows the fill

A layer given `ColorBy` resolves each row's fill through `scratch.colorsFor` —
the same call `Rect` makes, so the two agree by construction without either
reaching into the other — and picks the more readable of two inks against it.

The two are **the theme's own**: its label colour and the colour behind the
chart, rather than black and white. A dark theme's labels are then drawn in its
own colours, and a chart still looks like one thing. `geom.Color` overrides the
whole mechanism, because a caller who names a colour has answered the question.

The comparison needs a luminance, and `palette.Luminance` is where it went: how
light a colour is is a fact about the colour, not about the mark. It is WCAG's
definition over the linear-light decode `palette.Lerp` already blends through —
averaging the encoded bytes reads a saturated yellow as dark, which is the same
error the ramp's midpoint test pins at 188 rather than 128.

## Consequences

- **A text layer contributes no colour guide.** It implements `Describer` and
  `Faceter` — it holds rows, so faceting must be able to cut it — and
  deliberately not `Guided`. The layer whose boxes it labels carries the same
  `ColorBy` and has already contributed the guide; a second would be the same
  colourbar twice. It contributes no legend entry either, for the reason an
  annotation does not: what it draws is the reading, not a series to be named.
- **A dropped label is not a reported mark.** Row identity is collected for the
  labels that were actually drawn, so a pointer cannot land on a label the box
  had no room for.
- **The elision cache is what keeps the allocation gate flat.** Cutting a label
  builds a string, which is the one place a per-row allocation could hide behind
  work that has to happen anyway. `textGeom.remember` hands back last frame's
  string when the cut has not moved, so a chart redrawn at the same size builds
  none. It lives on the layer rather than in the frame's pool because it has to
  survive a `Build` — the argument `barGeom.gaps` already makes.
  `BenchmarkLabelled1k` against `BenchmarkLabelled10k` is gated flat, and
  `TestALabelledRenderDoesNotAllocatePerPoint` pins the same thing from the test
  side.
- **`geom.Align` gained `AlignSet`**, joining `DashSet` and `MarkerSet`. The
  start of a run on the baseline is the zero value *and* an alignment somebody
  may have asked for, and a text layer centres a label in its box when nobody
  has. Without the flag a round trip through the spec turns that default into a
  pinned left edge and the chart silently changes — which is what the existing
  per-mark round-trip test caught, on the first run.
- **A note and a text layer are both `"text"` in a document.** `spec.geomMark`
  asks `hasField`, exactly as it does for a rect against a region: a mark with a
  `text` *field* is data, one placed at literal values is an annotation. The
  label travels on the encoding under Vega-Lite's own channel name, and adding a
  channel to `spec.Encoding` means adding it to `hasField` too — a layer encoded
  only by that channel would otherwise read back as an annotation.
- **Neighbouring labels are not moved apart.** A box too narrow drops its label
  already, which is the case this mark exists for. A general de-overlap pass
  moves ink away from the row it belongs to, which is a different decision.
- Text is one run: no wrapping, no multi-line. Paragraph layout is out of the IR
  by design (CONCEPT.md §5), and a label that needs two lines is a label that
  needs a shorter column.

## Alternatives

**A `Label` option on `Rect` and `Bar`.** The obvious shape, and the one that
looks like less API. It was rejected on draw order first: a mark that drew its
own text would draw each label immediately after its own box, so a wide label
would be painted over by the next row's fill. Text has to come after *every*
box, and a layer is how this library expresses "after". It would also have had
to be added to every mark that could carry a label, each with its own anchor
rule — while one text layer already labels a rect, a bar, a point, and anything
later that spans a box.

**Sugar that expands to one `Note` per row.** It removes the loop from the
caller and none of the cost: the layers are still one per row, the chart still
has to be rebuilt when the rows change, and nothing measures anything. The
lifecycle problem is the problem.

**Truncating by default, with an option to drop.** The defaults are the whole
argument here, and this one is backwards for the reason given above: elision
that cannot be told apart is worse than absence, and a caller who knows their
labels are distinguishable by their first characters is the one who should say
so.

**Resolving the fill in the caller.** Possible — the colour scale is public —
and it asks every caller to reimplement `colorsFor`, get the
discrete/continuous distinction right, and keep it in step with the mark they
are labelling. The layer already has the answer.

**Drawing the ellipsis as a second run, to avoid building a string.** It makes
truncation allocation-free without a cache, and it puts an anchoring problem on
the hot path — the second run has to start exactly where the first ends, in a
font the geom does not shape. A remembered string costs one entry per row and
is correct by construction.

## Revisit if

Labels on *neighbouring* rows need to be moved apart. A scatter with names
beside its points is the case, and point mode has no box to drop against, so it
draws whatever overlaps today. That is a layout pass over a layer's own marks,
and it needs an answer to what moves, how far, and what happens when nothing
fits; it is not an extension of this record's fit rule, which is about one
label and one box.
