# 0029 — A third party's geom, scale or coord is a first-class citizen of the spec

**Status:** Accepted · **Date:** 2026-09-05

## Context

`CONCEPT.md` §17 item 7 was the last open decision: the interfaces third parties
implement, which §14 promises to freeze at v1.0 as a "stable
registration/extension model for third-party geoms and backends". The
[v1 API audit](../v1-api-audit.md) found the backend half of that promise kept
— `ir.Backend` has not changed since v0.1 and grows through optional interfaces
— and the geom half not:

- `geom.Option` is `func(*config)` over an unexported struct with no exported
  accessor. A geom outside the package could accept `...geom.Option` and then
  read none of them — not `X`, not `Color`, not `Label`. The package doc's
  promise of "one namespace instead of six" and `CONTRIBUTING.md`'s instruction
  to "reuse the shared option set" were true only inside the package.
- `geom.FromDesc`, `scale.FromDesc`, `scale.ColorFromDesc` and `coord.FromDesc`
  were closed switches. A third-party mark could implement `geom.Describer` and
  still not read back from a document; `spec` refused a mark type it did not
  know. `theme.Register` and `palette.RegisterRamp` already showed the open
  shape, and nothing else followed it.
- `spec.Mark` had no place for a property this module did not define, so even a
  registered mark could carry only the shared options through a document.

Freezing the interfaces without closing this would have frozen a library whose
own marks are serialisable and whose users' marks are not.

## Decision

### The configuration a third party reads is the `Desc` it already writes

```go
// geom
func Configure(opts ...Option) Desc
func (d Desc) Options() []Option
```

`Configure` applies the shared options to the package defaults and returns the
result as a `Desc` with no `Mark` and no `Source`. A geom outside the package
keeps that `Desc` as its configuration, reads `d.X`, `d.Color`, `d.Missing`
from it, and returns it — with `Mark` and `Source` filled in — from its own
`Describe`. `Options` is the inverse, and what a registered builder hands to its
constructor.

The audit sketched a new `Config` type with accessor methods. `Desc` was chosen
instead because it already exists, is already public, and is already what a
layer is: a second exported form of the same fifty fields would be a second
place for them to drift. Its fields are frozen by the same rule every struct
is — added, never removed — so returning it costs nothing the spec had not
already committed to.

### One escape hatch, not a second option type

```go
func Extra(key string, v any) Option     // sets Desc.Extra[key]
```

A third-party mark that needs a knob of its own — a lollipop's stem width, a
waffle's cell count — takes it through `Extra` and reads it from `Desc.Extra`,
so that its constructor reads exactly like a built-in one. Every built-in geom
accepts and ignores it, as it does any option it has no use for. The value is
whatever `encoding/json` can write, because it is about to be written.

### Registries replace the switches

```go
geom.Register(m Mark, build func(Desc) (Geom, error))
scale.Register(k Kind, build func(Desc) (Scale, error))
scale.RegisterColor(k ColorKind, build func(ColorDesc) (ColorScale, error))
coord.Register(t Type, build func(Desc) (Coord, error))
```

Each `FromDesc` tries its own switch, then the registry, then reports the
sentinel it always did (`ErrUnknownMark`, `ErrUnknownKind`, and a new
`coord.ErrUnknownType`). Registering a name the package owns panics: shadowing
a built-in would change what every existing document means. Registering a name
twice replaces the builder, which is what a test wants. The registries are maps
behind a mutex that nothing iterates — [ADR 0012](0012-parallel-panels.md)'s
rule that registration order never changes output holds by construction.

`SizeScale` gets no registry: `SizeDesc` has no kind field, because there is one
size scale. If a second arrives, a `Kind` field on `SizeDesc` and a
`RegisterSize` are additive.

### The spec carries a registered mark's own properties on the mark object

`spec.Mark` gains `Extra map[string]any` with a custom `MarshalJSON` and
`UnmarshalJSON`: the keys are written beside the fields this package defines,
at the same level of the mark object, and read back into `geom.Desc.Extra`. A
key that names a field this package owns is an error, not an override — a
document with two meanings for one key has no meaning. A registered mark's
`type` is the name its `Describer` reports; the shared properties a mark most
plausibly honours are written for it, and the rest travel in `Extra`.

```json
{"mark": {"type": "lollipop", "strokeWidth": 2, "stem": 3},
 "encoding": {"x": {"field": "t"}, "y": {"field": "count"}}}
```

A registered scale kind or coord type travels under its own name and
round-trips the fields of `scale.Desc` and `coord.Desc`. Those two carry no
`Extra` yet; a scale or coord whose configuration does not fit is the case
that adds one, additively, when it exists.

### The growth rule is written where it applies

Every interface a third party implements — `data.Source`, `scale.Scale`,
`scale.ColorScale`, `scale.SizeScale`, `coord.Coord`, `geom.Geom`, `geom.Rows`,
`ir.Backend`, `ir.Target`, `render.Observer`, `mathtext.Typesetter` — now says
in its doc comment that it never gains a method and names the optional
interfaces beside it. The rule is not new; `Resizer`, `Definite`, `Legender` and
`Subset` were already written to it. What is new is that the first request to
widen one of them will find the answer already written.

## Consequences

- A geom, a scale or a coord defined outside this module is read from and
  written to a JSON document exactly as a built-in one is, provided it
  implements the `Describer` of its package and registers a builder.
- `geom.Desc` gains `Extra`; `spec.Mark` gains `Extra`. Both are additive.
- `coord.Option` is renamed `coord.PolarOption`: it was a polar option wearing
  the package's generic name, and a coordinate system added later needs the
  name free. No call site spelled the type.
- `layout` moves to `internal/layout`. It had one importer and no conceivable
  third-party caller, and freezing it publicly would have made every refinement
  of the solver a breaking change for nobody's benefit. `backend/gg`'s parity
  test still reaches it: the `internal` rule is about import paths, and a
  nested module under `github.com/timzifer/refract/` is inside the tree.
- `mathtext.Symbols` becomes `RegisterSymbol` and `Symbol` over an unexported
  table behind a lock. An exported mutable map read on every typeset was a data
  race waiting for a caller who added to it at runtime, and once frozen there
  would have been no way to make it safe without breaking that caller.
- `spec.Schema` is `…/spec/v1` and its comment states the dialect's policy:
  within v1 a field is only ever added, a reader ignores what it does not know,
  and the version in the string moves only with the module's major version.
- §17 item 7 is closed. The remaining v1 work is the tag.

## Alternatives rejected

**A `Config` type with one accessor per option.** Cleaner in the abstract —
the representation stays private — and forty-two methods that duplicate the
fields of a struct that is already public. `Desc` is that struct.

**A second variadic list for a third party's own options.** `Foo(src, geomOpts,
fooOpts)` reads unlike every built-in constructor, and `...any` loses the type
checking the option types exist for. One `Extra` keeps the shape and costs a
type assertion on the reading side.

**Registering the built-ins too, so `FromDesc` is nothing but a lookup.** It
would make the package's own marks depend on `init` order for no gain, and it
would make "is this built in" a registry question rather than a compile-time
one. The switch stays for what the package owns; the registry is for what it
does not.

**Leaving `layout` public with a doc comment excluding it from the promise.**
Go has no mechanism for that. `internal/` is the honest spelling.

## Revisit if

A registered scale or coord needs to carry configuration the shared `Desc`
cannot hold. The answer is an `Extra` on `scale.Desc` and `coord.Desc`, mirrored
in `spec.Scale` and `spec.Coord`, which is additive on every side.
