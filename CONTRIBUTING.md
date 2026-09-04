# Contributing

## Layout

This is a three-module repository:

| Path | Module | Depends on |
|---|---|---|
| `.` | `github.com/timzifer/refract` | **nothing** — the standard library only |
| `backend/gg` | `github.com/timzifer/refract/backend/gg` | `gogpu/gg`, `x/image` |
| `arrow` | `github.com/timzifer/refract/arrow` | `apache/arrow-go` |

Both vector backends — `backend/svg` and `backend/pdf` — are in the core
module, so SVG and PDF cost no dependency at all
([ADR 0009](docs/adr/0009-pdf-backend.md)).

The split is not cosmetic. A nested module is excluded from its parent's module
graph, which is what lets `import "github.com/timzifer/refract"` pull in zero
dependencies while the raster backend links GoGPU and the adapter links Arrow.
CI enforces it — see [ADR 0001](docs/adr/0001-module-layout.md) and
[ADR 0013](docs/adr/0013-arrow-adapter.md).

`go.work` at the repository root is **committed**, so the three modules build
together with no setup. If your tooling ignores it,
`go work init . ./arrow ./backend/gg` reproduces it.

> Each nested module requires the core at a published tag, not through a
> `replace` directive. The workspace still overrides that for local
> development, so a change to the core is picked up immediately — but the
> module as published always resolves to a real release. Bumping those require
> lines is part of tagging a release, and needs the core's tag to exist first.

## Everyday commands

```sh
# build and test everything
go build ./... && go test ./...
(cd backend/gg && go test ./...)
(cd arrow && go test ./...)

# the checks CI runs
gofmt -l .                                   # must print nothing
go vet ./... && (cd backend/gg && go vet ./...) && (cd arrow && go vet ./...)
CGO_ENABLED=0 go build ./...

# the benchmarks, and the gate on what they measure
go test -run='^$' -bench=. -benchtime=10x ./... | awk -f .github/scripts/allocgate.awk

# the core must stay dependency-free
go list -deps ./... | grep -v '^github.com/timzifer/refract' | grep '\.'
# must print nothing

# cross-compilation, which is half the point of being cgo-free
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...
CGO_ENABLED=0 GOOS=js    GOARCH=wasm  go build ./...
```

## Golden files

Two sets, both compared with a tolerance narrow enough that only a real change
to a chart trips them.

**Golden SVG** (`testdata/golden/`) pins what the core renders:

```sh
go test . -update
```

**Golden PNG** (`backend/gg/testdata/golden/`) pins what the raster backend
renders:

```sh
cd backend/gg && go test . -update
```

Regenerate deliberately, and **read the diff before committing it**. A golden
file that changes without an intended reason is the test doing its job.

### Why the comparison has a tolerance

Neither set is compared byte for byte, and cannot be. Go may contract `a*b + c`
into a fused multiply-add, and arm64 does while amd64 does not — so the same
chart prints a coordinate one float32 unit in the last place apart on an Apple
Silicon Mac and on an x86 runner. That is enough to change a rounded third
decimal and fail a byte comparison on output that is visually identical.

So SVG is compared with `internal/svgdiff`: everything that is not a number must
match exactly — element and attribute order, ids, colours, path verbs, text —
and coordinates may differ by a hundredth of a pixel. PNG is compared with a
small per-channel tolerance. Both are orders of magnitude below anything visible
and orders of magnitude above the noise they exist to absorb.

A figure that embeds a raster — a density chart is an `<image>` whose href is a
base64 PNG — has that payload compared as an image rather than as the deflate
stream it is written as. Deflate output belongs to the standard library, and two
Go releases produce two streams for identical pixels; pinning it would be a
golden test of `compress/flate`. The vector half, including where the raster is
drawn and how large it is, is still compared exactly. See
`backend/gg/cmd/gallery/embedded.go`.

Do not widen either tolerance to make a failure go away. Narrowing one is an
improvement; widening one hides the thing the test is for.

The same reasoning applies inside tests: device coordinates are float32 and
arm64 contracts `a*b + c` into an FMA where amd64 does not, so comparing them
with `==` fails on one architecture and passes on the other. `geom`'s annotation
tests use `sameRect`/`samePoint` at `svgdiff.DefaultTolerance` instead.

## Documentation figures

Every chart in the README and the docs is generated, never hand-made:

```sh
go run ./backend/gg/cmd/gallery            # regenerate docs/images/
go run ./backend/gg/cmd/gallery -check     # verify they match the code
```

The `-check` form also runs as an ordinary test
(`go test ./backend/gg/cmd/gallery`) and in CI, so a stale figure fails the
build. When it does, run the generator and commit the result. On `main`, CI
regenerates and commits them for you.

## Adding things

**A geom** goes in `geom/`, implements `geom.Geom`, and emits IR — it must never
reference a backend. Reuse the shared `geom.Option` set rather than inventing
per-geom options; an option a geom has no use for is accepted and ignored.

**A scale** goes in `scale/`, implements `scale.Scale`, and owns both its
mapping and its tick generation. That pairing is why a time axis can label
itself in calendar units without anything above it knowing. If it cannot place
every finite value, implement `scale.Definite` too — that is what keeps a NaN
coordinate out of a backend. If it positions categories in slots, implement
`scale.Categorical` and `scale.Band`; see
[ADR 0008](docs/adr/0008-categorical-axes.md).

**A backend** implements `ir.Backend` and `ir.Target`. Whatever a drawing call
hands it — a point slice, a path, an image — is lent for that call only: refract
draws from pooled buffers, so a backend that keeps one will render the next
frame's data. Copy what you need to keep; `ir.Recorder` is the worked example.
If it needs a dependency the core must not have, it belongs in its own nested
module. Keep its contact
with that dependency as small as you can, and say what you touched — see
[ADR 0006](docs/adr/0006-gg-coupling-surface.md). Marker outlines come from
`internal/markers` so that a diamond is the same diamond everywhere; the gg
backend is a separate module and cannot import it, so it carries a copy and
says so.

**A reduction** goes in `stat/`, takes plain slices and returns row numbers —
never a `Source`, never IR. It is generic over `float32` and `float64` so that a
geom can run it on projected device coordinates, which is where the reduction
belongs ([ADR 0011](docs/adr/0011-decimation.md)). Give it an `Append` form as
well: the plain one is what reads well in documentation, the `Append` one is
what a chart redrawn every frame calls.

Wiring one into a geom means adding a case to `geom.config.reduction`, not a new
option namespace — `geom.Decimate` and `geom.Budget` are shared like every other
option. Reduce in `Build`, never in `Train`.

**A theme** is `theme.Tokens` plus `theme.Build`, not fifty literal fields.
Register it by name if it should be reachable from a config file. Reach for
`Theme.With` before copying the struct.

**An annotation** goes in `geom/annotate.go`, takes values rather than a
`data.Source`, and must *not* implement `geom.Faceter` — a layer with no rows is
furniture and belongs on every panel of a facet.

Anything that changes the IR or the `Backend` interface needs an ADR. So does
anything that answers one of the open questions in
[CONCEPT.md §17](CONCEPT.md#17-open-decisions).

## Tests

Model-layer code is tested against `internal/irtest.Recorder`, an `ir.Backend`
that records calls. Assert on the primitives that were emitted, not on scraped
SVG or on pixels — a geom test that parses XML is testing the wrong layer.

Write the test that would have caught the bug.
`TestLinearFractionalStepKeepsItsDecimal` in `scale/` exists because a step of
2.5 once rounded its labels to whole numbers, producing an evenly spaced axis
that read 0, 2, 5, 8, 10.

### Benchmarks and the allocation gate

There is one claim in this repository that is a number rather than a picture:
what a frame costs in allocations does not grow with the data it draws. It is
checked twice, because the two ways of measuring it catch different mistakes.

**As a test.** `TestARenderDoesNotAllocatePerPoint` renders the same chart over
a thousand rows and over a million and fails if the second allocates more than
a handful of times more than the first. Its siblings cover a faceted chart, a
layer coloured from a column, and an absolute per-frame budget. They are built
out under `-race`, which allocates on refract's behalf — the race job and the
ordinary test job are separate, so the gate still runs on every commit.

**As a benchmark.** `.github/scripts/allocgate.awk` reads `go test -bench`
output and enforces the same property from the numbers a benchmark actually
reports, plus `BenchmarkLTTB` and `BenchmarkMinMax` being allocation-free —
which is the whole reason `stat`'s `Append` forms have that shape:

```sh
go test -run='^$' -bench=. -benchtime=10x ./... | awk -f .github/scripts/allocgate.awk
```

That is exactly what the `Benchmarks and the allocation gate` CI job runs, over
all three modules. It gates allocation counts and never times: a shared runner
has nothing reliable to say about times, and a gate that flakes is a gate people
learn to ignore.

When either fails, find the culprit rather than raising the bar:

```sh
go test -run=XXX -bench=Frame -memprofile=mem.out .
go tool pprof -sample_index=alloc_objects -top mem.out
```

The usual causes are a new buffer that should have come from `geom.scratch`, and
a variadic interface method being called once per row.

A new benchmark goes wherever the thing it measures lives, and needs no
registration — the CI job runs `-bench=.` across every package, which also means
a benchmark that stops compiling fails the build rather than quietly rotting.

## Style

Standard Go, `gofmt`-clean, doc comments on exported identifiers. Comments
should say *why*, not restate the code; the codebase's existing comments are the
guide. British or American spelling — just be consistent within a file.
