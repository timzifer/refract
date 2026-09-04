# Contributing

## Layout

This is a two-module repository:

| Path | Module | Depends on |
|---|---|---|
| `.` | `github.com/timzifer/refract` | **nothing** — the standard library only |
| `backend/gg` | `github.com/timzifer/refract/backend/gg` | `gogpu/gg`, `x/image` |

The split is not cosmetic. A nested module is excluded from its parent's module
graph, which is what lets `import "github.com/timzifer/refract"` pull in zero
dependencies while the raster backend links GoGPU. CI enforces it — see
[ADR 0001](docs/adr/0001-module-layout.md).

`go.work` at the repository root is **committed**, so the two modules build
together with no setup. If your tooling ignores it, `go work init . ./backend/gg`
reproduces it.

> `backend/gg/go.mod` carries a temporary
> `replace github.com/timzifer/refract => ../..`, because the core is not tagged
> yet. Remove it when `v0.1.0` is tagged.

## Everyday commands

```sh
# build and test everything
go build ./... && go test ./...
cd backend/gg && go test ./... && cd ../..

# the checks CI runs
gofmt -l .                                   # must print nothing
go vet ./... && (cd backend/gg && go vet ./...)
CGO_ENABLED=0 go build ./...

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

Do not widen either tolerance to make a failure go away. Narrowing one is an
improvement; widening one hides the thing the test is for.

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

**A backend** implements `ir.Backend` and `ir.Target`. If it needs a dependency
the core must not have, it belongs in its own nested module. Keep its contact
with that dependency as small as you can, and say what you touched — see
[ADR 0006](docs/adr/0006-gg-coupling-surface.md).

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

## Style

Standard Go, `gofmt`-clean, doc comments on exported identifiers. Comments
should say *why*, not restate the code; the codebase's existing comments are the
guide. British or American spelling — just be consistent within a file.
