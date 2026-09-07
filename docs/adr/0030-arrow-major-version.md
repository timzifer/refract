# 0030 — The Arrow adapter's major version is its upstream's, and its import path says so

**Status:** Accepted · **Date:** 2026-09-06

## Context

CONCEPT §15 has said since the v1 API audit that `arrow` "tracks its upstream:
its major version follows `apache/arrow-go`'s, which is the reason it is a
module of its own". The audit recorded the reason
([v1-api-audit.md](../v1-api-audit.md), the `arrow` row): the adapter's three
functions take Arrow's own types — an `arrow.Record`, an `arrow.Table` — and
every Arrow release line is a new major version of `arrow-go`, which is a new
import path and a new set of those types. An adapter for `arrow-go/v18`
cannot accept a `v19` record, so a refract
release that moved to `v19` would break every caller still on `v18`, and a
module whose version stayed at v1 while that happened would be lying about it.

What the sentence did not say is what it costs to keep. A Go module whose major
version is 2 or higher must carry that version in its import path — the
`/v2` rule — and `github.com/timzifer/refract/arrow` at `v18.0.0` is not a
version Go will resolve. So the decision in §15, taken as written, was one the
toolchain refused, and the release checklist would have found that out at the
tag.

## Decision

**The adapter lives at `github.com/timzifer/refract/arrow/v18`, in the
`arrow/v18` directory, and tags as `arrow/v18.x.y`.**

- The module path carries Arrow's major version, and the package is still
  named `arrow`. A caller writes
  `import "github.com/timzifer/refract/arrow/v18"` and uses `arrow.Source`,
  exactly as they import `arrow-go/v18` and use `arrow.Record`.
- The **major** version is upstream's and nothing else about the version is:
  the adapter's minor and patch numbers are its own, so `v18.1.0` is a
  release of the adapter, not a release of Arrow. The upstream pin stays an
  exact version in `go.mod`, per [ADR 0013](0013-arrow-adapter.md).
- When Arrow ships `v19`, the adapter for it is a second directory,
  `arrow/v19`, with its own `go.mod` and its own tag series. The `v18`
  directory stays for as long as it has callers, and a caller moves to the new
  one when they move their own Arrow dependency. That is the major-version
  subdirectory layout Go's module reference describes, and it is why a
  directory was chosen over a `/v18` suffix in `arrow/go.mod` alone: two
  majors can be built, tested and tagged side by side from one commit.
- The tag name is `arrow/v18.0.0`, not `arrow/v18/v18.0.0`: a module's
  subdirectory prefix for tagging excludes its major-version suffix, even when
  the module is in a major-version subdirectory.
- The adapter requires the core at a published tag, like every nested module
  ([CONTRIBUTING](../../CONTRIBUTING.md)). It does not require a particular
  core major: the core's own `/v2`, should it ever come, is a change to the
  adapter's `go.mod`, not to its version.

## Consequences

- `go.work`, CI, the README, CONCEPT and CONTRIBUTING all name `arrow/v18`.
  Nothing in the core module changes; the core has never imported the
  adapter, which is the point of it being nested.
- The version says which Arrow a program can hand the adapter, before the
  program is compiled. That is the guarantee the sentence in §15 was reaching
  for.
- A reader who sees `v18.0.0` on a library at `v1.0.0` may wonder. The package
  doc says why in one paragraph, and this record says the rest.

## Alternatives rejected

**Keep `arrow` at `v1.x` and move the upstream pin inside it.** A `v1.3.0`
that takes `arrow-go/v19` records where `v1.2.0` took `v18` ones is a breaking
change wearing a minor version. Go's own compatibility rule is the reason a
major version exists.

**Keep `arrow` at `v0.x` for good, like the GPU tier.** `v0` is Go's spelling
of "no promise", and the GPU tier makes none yet. The adapter's promise is
small and settled; what it cannot promise is which Arrow it adapts, and a
major version is the exact tool for that.

**A `/v18` suffix in `arrow/go.mod` with no directory move.** It works, and it
leaves nowhere for `v19` to go but a branch. A repository that builds every
module from one commit ([ADR 0001](0001-module-layout.md)) should not have one
module that has to be built from another.

## Revisit if

Arrow-go stops changing its major version with every release, or refract
gains a second upstream-typed adapter. If a pattern emerges — an adapter's
major is its upstream's — it belongs in CONCEPT §15 as a rule rather than in
one record.
