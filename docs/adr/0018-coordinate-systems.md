# 0018 — Coordinates are a stage between the scales and the IR

**Status:** Accepted · **Date:** 2026-09-04

## Context

[CONCEPT §8](../../CONCEPT.md) has promised a coordinate stage since v0.1:
"Cartesian for v1; the transform is a **pluggable stage** so polar/geographic
slot in later without touching geoms." §16 then admits what the tree actually
holds: "Every package below exists except `coord/`."

What exists instead is a hard-coded Cartesian mapping in every geom. `geom.Frame`
carries `Area ir.Rect` and `X, Y scale.Scale` and nothing else; a geom calls
`f.X.Map(v)` and writes the `float32` it gets back straight into an `ir.Point`
(`geom/scatter.go:69`, `geom/bar.go:102`). `render.Draw` draws a Cartesian grid
and a pair of Cartesian axes for every panel, unconditionally
(`render/render.go:156-161`). Nothing in the codebase names a coordinate system,
so nothing can choose a different one.

The consequence is a whole family of charts that cannot be drawn: pie, donut,
radar, rose/coxcomb, wind rose, gauge, polar boxplot. §14 files them under
"Beyond v1.0" alongside geographic projections, as though they were the same
size of problem.

**They are not, and the schedule is the reason this is being decided now rather
than later.** §17.7 is the one open decision left, and it says the extension API
— the interfaces a third party implements — freezes at v1.0. `geom.Frame` and
the `Geom` interface are that API. A `Frame` that says a chart is a rectangle
with an X scale and a Y scale, frozen, is a library that is Cartesian forever;
"polar later" then means "polar only by breaking the v1.0 promise". The
coordinate stage lands before the freeze or it does not land.

## Decision

**A coordinate system is a stage between the scales and the IR. Scales keep
mapping a data value into an interval; the coord decides what that interval
means and turns a pair of them into a device point.**

The seam is cheap because `scale.Scale` is already agnostic about it.
`SetRange(lo, hi)` sets an interval and `Map(v) float32` places a value in it
(`scale/scale.go:31-54`). Nothing there is Cartesian — what is Cartesian is only
that `render` currently passes the panel rectangle's edges as the interval. A
polar coord passes `[0, 2π]` for the angular scale and an inner/outer radius for
the radial one, then converts the pair. **`scale.Scale` does not change**, and
that is what makes this affordable before v1.0.

```go
// Package coord maps scaled positions into device space.
type Coord interface {
	// Frame gives the coord the panel rectangle and this panel's scales, and
	// lets it choose the interval each scale maps into. Cartesian sets the
	// rectangle's edges; Polar sets [0,2π] and a radius interval.
	Frame(area ir.Rect, x, y scale.Scale)

	// Point turns one mapped pair into a device point.
	Point(x, y float32) ir.Point
	// Points is the batch form, and the one a geom on the hot path calls.
	Points(dst []ir.Point, xs, ys []float32) []ir.Point

	// Edge appends the device path of an edge that is straight in data space.
	// Cartesian appends one LineTo; Polar appends the cubics of an arc.
	Edge(p *ir.Path, from, to ir.Point)
	// Area appends the device path of a data-space rectangle. Cartesian
	// appends four corners; Polar appends an annular sector.
	Area(p *ir.Path, x0, y0, x1, y1 float32)

	// Clip is the path a panel's data is clipped to: a rectangle, or a disc.
	Clip(area ir.Rect) *ir.Path
	// Invert is what a tooltip needs: a device point back to a mapped pair.
	Invert(pt ir.Point) (x, y float32)
	// Furniture reports the geometry of grid lines, axis lines and tick label
	// anchors. It returns shapes; render still strokes them.
	Furniture(area ir.Rect, xTicks, yTicks []scale.Tick) Furniture
}
```

Four properties are binding, and each is binding for a reason the codebase has
already paid for once.

**1. `Cartesian` is the identity, and the default.** Its `Point` returns
`ir.Point{x, y}`, its `Edge` appends one `LineTo`, its `Area` four corners. So
migrating a geom is mechanical — `ir.Point{X: f.X.Map(vx), Y: f.Y.Map(vy)}`
becomes `f.Coord.Point(f.X.Map(vx), f.Y.Map(vy))` — and **the golden files
already in the repository are the proof that it changed nothing**. A refactor
whose correctness argument is already committed is a different risk from one
that needs a new one. A zero `Coord` on a `Frame` means Cartesian, so code that
never heard of this still compiles and still draws the same chart.

**2. A coord reports geometry; it does not paint.** `Furniture` hands back paths
and label anchors, and `render.drawGrid` / `render.drawAxes`
(`render/render.go:350,402`) stroke them in the order they always did. AGENTS.md
is unambiguous: "`render` is the only package that knows the drawing order of a
chart." A coord that drew its own rings would be a second drawing order, and
ADR 0010 exists because a second path for "the simple case" is how golden-file
coverage silently halves.

**3. The per-point call has a batch form, and geoms use it.** AGENTS.md records
what a variadic interface method per row costs: `scale.Scale.Train(vs ...float64)`
could not be proved non-escaping, so the argument slice went to the heap once per
row, a million times on a million-row column. `Points(dst, xs, ys)` exists so
that the same shape does not reappear here, and `Point` is for the handful of
callers that place one thing. `TestARenderDoesNotAllocatePerPoint` and
`.github/scripts/allocgate.awk` are the gates; neither may be widened for this.

**4. Decimation stays where ADR 0011 put it, and polar does not get it.**
`stat.LTTB` chooses the row forming the largest triangle over a *pixel column*,
and ADR 0011 pins the reduction to device space precisely because equal steps in
data are not equal steps on screen. Under a polar transform a bucket of equal
angle is not a bucket of equal width, so the pixel-column argument does not hold
and a reduction that ran after the transform would be measuring the wrong thing.
The resolution is not to redefine the reduction: **a polar coord reports that it
does not decimate.** Nothing polar is a big-data chart — a pie has a dozen
slices, a rose three dozen petals, a radar twenty axes — so this costs nothing
real, and it keeps ADR 0011's guarantee exactly as written for the Cartesian path
where it earns its keep.

### The first batch: pie, donut, radar

An interface is a claim until something is drawn through it. These two are the
milestone's evidence, and neither is a new geom:

- **Pie and donut** are `geom.Bar` with a stacking adjustment
  ([ADR 0019](0019-position-adjustments.md)), in `coord.Polar`, taking θ from the
  Y axis. The ring closes into a full circle because the stacked Y domain ends at
  the total; the hole is the inner radius `Frame` was given. A slice is an
  annular sector: two radial edges and two arcs. **An arc is cubics** — `ir.Path`
  has `MoveTo`, `LineTo`, `CubicTo`, `Close` and nothing else, and ADR 0002 froze
  that set — so `internal/markers` is the precedent, not the exception: its
  `kappa` is the quarter-circle constant already. Sweeps are split so no segment
  exceeds a quarter turn, and the control-point rule generalises it:
  `k = (4/3)·tan(φ/4)`, which at `φ = π/2` is `kappa` — exactly in real
  arithmetic, and to within about three ulps once `math.Tan` has evaluated it.
  So the test that pins the two together compares them at a tolerance rather
  than with `==`, for the same reason AGENTS.md forbids comparing device
  coordinates exactly: an identity that holds on paper is still float
  arithmetic by the time it reaches a path.
  Slice angles are computed from a running total, never by accumulating deltas,
  so the last slice ends exactly where the first began and there is no seam.
- **Radar** is `geom.Line`, or `geom.Area` for the filled form, over an ordinal
  angular scale — which already exists and already satisfies `scale.Categorical`
  and `scale.Band` (ADR 0008). Two things it forces into the interface: the
  contour must **close** back to the first axis, and a radar edge is a **chord,
  not an arc**. So `Edge` is a policy and not merely an implementation —
  `coord.Polar(coord.Chord)` against the arc default that pie and rose need.
  Without that distinction a radar comes out with bowed sides.

Both need two things this ADR does not supply, and that is deliberate: the
stacking adjustment (ADR 0019) and the discrete colour scale with a multi-entry
legend ([ADR 0020](0020-discrete-colour-and-multi-entry-legends.md)) — a pie has
N slices inside *one* layer, and `render.legendEntries` collects exactly one
entry per layer today.

### The alternative, and why it was not taken

The honest case against this record is strong enough to write down.

**Self-contained polar geoms.** `geom.Pie` computes its own wedges inside
`f.Area`, ignores the scales, and the chart suppresses its furniture. It touches
no existing geom, no `ir.Rect`-shaped mark, no decimation path, and no
allocation gate — six polar charts would be six files instead of a change spread
across all of `geom/`. Its sharpest argument is a counting argument: **the
transform is the least valuable fifth of what a pie chart needs.** Stacking,
groups, discrete colour, multi-entry legends and furniture suppression are
required either way; only the transform is unique to this decision, and only this
decision pays for it by rewriting how every geom emits a point. Rectangles make
that concrete: a bar collects `[]ir.Rect` into the pooled `scratch.rects`
(`geom/bar.go:106`) and a boxplot fills one outright (`geom/boxplot.go:108`),
and a rectangle is not a shape that survives a polar transform.

It was not taken for two reasons the counting argument does not reach. The first
is §17.7 again: self-contained geoms leave `geom.Frame` exactly as it is, and
freezing it at v1.0 is the outcome this record exists to avoid. The second is
that "suppress the furniture" has no home — the levers available are
`theme.ShowGridX` and `theme.ShowAxisLineX` (`theme/theme.go:52-55`), global
switches for a per-panel question, and a geom painting over furniture it dislikes
would put drawing order in two packages. AGENTS.md names this situation
precisely: "If a change needs to cross one of these lines, that is a signal the
seam is in the wrong place — say so rather than routing around it."

The seam is in the wrong place. This record moves it rather than routing around
it, and pays the migration cost once, before the API freeze makes it unpayable.

## Consequences

| Affected | What changes |
|---|---|
| `geom.Frame` | one field, `Coord`. Zero value means Cartesian, so existing construction sites compile unchanged |
| every geom's `Build` | mechanical rewrite onto `f.Coord.*`. `Bar` and `Boxplot` cost the most: they fill `ir.Rect` directly (`geom/bar.go:106`, `geom/boxplot.go:108`), and a rectangle becomes a coord-built path |
| `render.Draw` | `drawGrid` and `drawAxes` consume `Furniture` instead of computing it. Drawing order is unchanged |
| `layout` | nothing, deliberately. A polar coord inscribes itself in whatever rectangle the solver gives it. Gutter rules that understand a radial axis are a later milestone, not this one — ADR 0010's single solver is untouched |
| `facet` | nothing. A coord belongs to a chart, not to a panel |
| `spec` | a refract-owned top-level `"coord"` field, plus `coord.Desc` / `coord.FromDesc` in the shape of `scale.Desc`. ADR 0014 requires that what refract has and Vega-Lite does not be named plainly rather than smuggled through a borrowed name — so a pie is written as `mark: "bar"` with `coord: {"type": "polar"}`, not as an `arc` mark that would round-trip into something refract cannot rebuild |
| `interact` | hit-testing is unaffected: a slice is a subpath and ADR 0015 indexes subpaths. What does change is `Index.At`, which inverts a hit through `p.X.Invert` / `p.Y.Invert` (`interact/interact.go:263-265`) and would report nonsense for a wedge. `Coord.Invert` is what makes a tooltip name a value instead of a pixel |
| `stat` | nothing. Reductions stay generic over `float32`/`float64` and stay Cartesian, per property 4 |
| allocation gates | the real exposure. Property 3 is the mitigation, and the gates are the check |

The IR does not change. That is the load-bearing consequence: ADR 0002 froze the
primitive set for the v0.1 cycle on the argument that every curve a chart needs is
expressible as cubics, and a polar coordinate system is the first serious test of
that claim. It passes.

## Revisit if

- **Geographic coordinates arrive.** A projection is a transform on every point
  with no linear interval underneath it, and it will want `Frame` to hand a coord
  the data domain rather than a mapped position. That is a wider seam than this
  one, and it should be argued on its own evidence rather than smuggled in as a
  third `Coord` implementation.
- **A polar chart turns out to be a big-data chart.** Property 4 assumes none is.
  A radial time series over a million rows would reopen the decimation question,
  and the answer would be a second reduction space, not a widened ADR 0011.
- **The batch form is not enough.** If `Points` still shows up in an allocation
  profile, the answer is a concrete `Cartesian` fast path that skips the interface
  entirely on the default route — not a looser gate.
