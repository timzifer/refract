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

Still open, deliberately: **§17.7** (the third-party geom and backend extension
API). It freezes at v1.0, and freezing it well is what makes the "last plotting
library" claim survivable — so it stays open until the milestone that has to
answer it. 0018 and 0020 are both shaped by that deadline: the first widens
`geom.Frame` because it must be widened before the freeze, the second declines to
widen `Geom` because an optional interface does the same work without spending
the freeze.
