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

Still open, deliberately: **§17.3** (how faithful the JSON spec is to Vega-Lite)
and **§17.7** (the third-party geom and backend extension API). Both belong to
milestones that have not started, and deciding them now would be guessing.
