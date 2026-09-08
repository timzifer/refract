# Architecture decision records

Short records of decisions that were genuinely open, why they went the way they
did, and what would make them worth revisiting.

They exist because `CONCEPT.md` §17 lists open decisions, and a design document
that never records how its own questions were answered stops being trustworthy.
Each record below closes one of those items or pins something the code now
depends on.

| # | Decision | Status | Closes |
|---|---|---|---|
| [0001](0001-module-layout.md) | The gg backend is a nested module in this repository | Accepted | §17.5 |
| [0002](0002-ir-and-backend.md) | The v0.1 IR and `Backend` interface | Accepted | — |
| [0003](0003-text-and-fonts.md) | Text measurement and the font strategy | Accepted | §17.4 |
| [0004](0004-svg-source-of-truth.md) | The built-in emitter is the only SVG path in v0.1 | Accepted | §17.2 |
| [0005](0005-go-version.md) | Go 1.25 is the minimum | Accepted | §17.6 |
| [0006](0006-gg-coupling-surface.md) | How much of gg the adapter is allowed to touch | Accepted | §17.1 |
| [0007](0007-per-mark-colour.md) | Colour varies per mark without changing the IR | Accepted | — |
| [0008](0008-categorical-axes.md) | Categorical axes ride the numeric `Scale` interface | Accepted | — |
| [0009](0009-pdf-backend.md) | PDF is a built-in emitter, not a gg recording | Accepted | — |
| [0010](0010-panel-layout.md) | One constraint solver for every chart shape | Accepted | — |
| [0011](0011-decimation.md) | Decimation at draw time, in device space, by default | Accepted | — |
| [0012](0012-parallel-panels.md) | Panels build in parallel by recording, replayed in order | Accepted | — |
| [0013](0013-arrow-adapter.md) | The Arrow adapter is its own module, and borrows only where it can | Accepted | — |
| [0014](0014-json-spec.md) | The JSON spec is Vega-Lite-shaped, not a Vega-Lite subset | Accepted | §17.3 |
| [0015](0015-hit-testing.md) | Hit-testing indexes what a render emitted, told apart by an observer | Accepted | — |
| [0016](0016-streaming-and-damage.md) | Streaming is a snapshot and a swap; damage is a diff of two recordings | Accepted | — |
| [0017](0017-browser-backend.md) | The browser backend is canvas 2D, in the core, and not gg | Accepted | — |
| [0018](0018-coordinate-systems.md) | Coordinates are a stage between the scales and the IR | Accepted | — |
| [0019](0019-position-adjustments.md) | Stacking is a position adjustment within a layer, derived in `Train` | Accepted | — |
| [0020](0020-discrete-colour-and-multi-entry-legends.md) | Discrete colour is a scale; a layer may contribute many legend entries | Accepted | — |
| [0021](0021-native-window.md) | The native window rasterizes on the CPU and presents one texture | Accepted | — |
| [0022](0022-gpu-tier.md) | The GPU tier is opted into by importing a module | Accepted | — |
| [0023](0023-math-typesetting.md) | Notation is typeset by a pluggable typesetter, installed by wrapping the backend | Accepted | — |
| [0024](0024-accessibility.md) | A chart says what it is in three channels: a name, a description, and its data | Accepted | — |
| [0025](0025-responsive-charts.md) | A chart follows its surface by scaling its theme | Accepted | — |
| [0026](0026-breaking-a-mark-out.md) | A mark is broken out by displacing it, and the coord answers how far | Accepted | — |
| [0027](0027-size-channel-and-the-guide-column.md) | A size channel maps by area, and the guide column is generalised once | Accepted | — |
| [0028](0028-distribution-stats.md) | A distribution stat runs in `Train`, and decides one of its own axes | Accepted | — |
| [0029](0029-extension-model.md) | A third party's geom, scale or coord is a first-class citizen of the spec | Accepted | §17.7 |
| [0030](0030-arrow-major-version.md) | The Arrow adapter's major version is its upstream's, and its import path says so | Accepted | — |
| [0031](0031-tracks.md) | A track is a panel with a fixed extent, on a scale it shares | Accepted | — |
| [0032](0032-text-as-a-mark.md) | Text is a mark that reads a column, and it labels the box a row spans | Accepted | — |
| [0033](0033-smith-charts.md) | A Smith chart is the polar-shaped coord seam, over normalised impedance | Accepted | — |
| [0034](0034-null-values.md) | A null is a missing value, and a column says so beside its values | Accepted | — |
| [0035](0035-label-format-and-locale.md) | A tick label is described rather than computed, and the description carries a language | Accepted | — |
| [0036](0036-error-bars.md) | An interval is a mark, and the encoding says which way it runs | Accepted | — |
| [0037](0037-secondary-axis.md) | A second axis is a scale on the chart and a binding on the layer | Accepted, amended | — |
| [0038](0038-embedded-fonts.md) | A PDF carries the font its labels need, subset to the glyphs it drew | Accepted | — |
| [0039](0039-relational-layouts.md) | A relational layout is a stat in the unit square, and the coord decides what it looks like | Accepted | — |
| [0040](0040-label-collision-avoidance.md) | Participating text layers share a deterministic panel-local label layout | Accepted | — |
| [0041](0041-qq-plots.md) | QQ plots rank the sample before it is drawn | Accepted | — |
| [0042](0042-colour-transforms-and-classes.md) | A colour ramp may compress its domain or cut it into classes | Accepted | — |
| [0043](0043-mark-identity.md) | Mark identity is a column the caller names, resolved outside the geom | Accepted | — |
| [0044](0044-transitions.md) | A transition is a keyed join blended in data space, driven by the host's clock | Accepted | — |
| [0045](0045-linked-views.md) | Linked views are the host's; refract supplies the two ends of the wire | Accepted | — |
| [0046](0046-overlay-layer.md) | The chart owns an overlay, drawn last and announced to nobody | Accepted | — |
| [0047](0047-clickable-legend.md) | A legend answers to a pointer; hiding is the chart's, toggling the caller's | Accepted | — |
| [0048](0048-clickable-colourbar-and-size-key.md) | A colourbar and a size key report a quantity, because neither is a series | Accepted | — |
| [0049](0049-locus-annotations.md) | A locus is an annotation, and the coord draws it | Proposed | — |
| [0050](0050-barycentric-coord.md) | A ternary chart is a barycentric coord, and its third grid family is not furniture | Proposed | — |
| [0051](0051-probability-scales.md) | A probability scale warps the axis, where a QQ plot warps the sample | Proposed | — |
| [0052](0052-tidy-tree-layout.md) | A tidy tree is a bounded deterministic layout, and it is not a force simulation | Proposed | — |
| [0053](0053-statistical-instruments.md) | A domain reduction belongs in `stat` when its output is the chart | Proposed | — |

Nothing in §17 is open any more. **§17.7**, the third-party geom and backend
extension API, was the last, and it was held open on purpose until the
milestone that had to answer it: freezing it well is what makes the "last
plotting library" claim survivable. 0018 and 0020 were both shaped by that
deadline — the first widens `geom.Frame` because it had to be widened before
the freeze, the second declines to widen `Geom` because an optional interface
does the same work without spending it — and 0029 is the answer. A decision
that opens after v1.0 gets a record here before it gets code.

## Proposed records

**0049 to 0053 are proposed, not accepted, and no code implements them.** They
are written down because the alternative is worse: each one answers a question
an earlier record left open — 0033's "Revisit if" for the first two, 0039's for
the fourth, 0041's serialisation rule for the third — and a question answered in
a conversation and not in the repository gets answered again, differently, later.

They are also deliberately written as a set, because four of the five lean on
each other. 0049 introduces the mark 0050 keeps as its escape hatch for a
ternary chart's third grid family and 0053 uses for a funnel plot's contours;
0051 and 0053 both cite 0041's rule that a named member of a closed family
serialises and an arbitrary Go function does not; 0052 narrows a category 0039
refused rather than reopening it. Reading any one of them alone will make it
look more expensive than it is.

A proposed record becomes accepted when it is implemented, or is deleted with a
sentence saying what it got wrong. Neither is urgent: nothing in v1.7 depends on
any of them, and each is additive by construction.
