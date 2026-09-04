# 0001 — The gg backend is a nested module in this repository

**Status:** Accepted · **Date:** 2026-09-04 · **Closes:** CONCEPT.md §17.5

## Context

The headline promise is that `import "github.com/timzifer/refract"` pulls in
nothing but the standard library. That is what makes the lean case — a server
emitting SVG — cost nothing, and it is what keeps a young, fast-moving
dependency out of the graph of anyone who does not want it.

`CONCEPT.md` §16 sketched the raster/GPU backend as a separate module called
`refract-gg`, which reads as a second repository. That would work, but it means
the backend and its golden images cannot be committed anywhere until that
repository exists, and it splits one milestone across two release flows.

## Decision

Two modules, one repository:

- `github.com/timzifer/refract` — the core. `go.mod` has no `require` block.
- `github.com/timzifer/refract/backend/gg` — the gg adapter, with its own
  `go.mod` requiring `github.com/gogpu/gg` at an exact version.

A nested module is excluded from its parent's module graph, so the core's
dependency graph is unaffected by anything the backend needs. A `go.work` at the
repository root builds both together during development, and is committed for
that reason.

Releases are tagged separately: `v0.1.0` for the core, `backend/gg/v0.1.0` for
the backend.

## Consequences

- The stdlib-only promise is mechanically enforceable rather than aspirational.
  CI asserts it: `go list -deps ./...` on the core must not name a single
  non-stdlib package.
- Until the core is tagged, `backend/gg/go.mod` carries a
  `replace github.com/timzifer/refract => ../..`. A replace directive in a
  dependency is ignored by consumers, so it only affects builds of this
  repository. **It must be removed when `v0.1.0` is tagged**, and the require
  line left to resolve normally.
- The import path is `.../backend/gg`, not `refract-gg`. `CONCEPT.md` §13 and
  §16 have been updated to match.

## Revisit if

The backend grows a release cadence genuinely independent of the core — for
instance if it starts tracking gg's own weekly releases — at which point a
separate repository stops being overhead and starts being useful.
