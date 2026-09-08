# 0044 — A transition is a keyed join blended in data space, driven by the host's clock

**Status:** Accepted · **Date:** 2026-09-08

## Context

[CONCEPT §14](../../CONCEPT.md#14-roadmap--milestones) closes its animation
entry with a sentence that is a design decision rather than an observation:

> The step is a key concept, not an interpolation layer.

With [ADR 0043](0043-mark-identity.md) there is a join key, so the question is
what to do with it — and specifically *where* two states are blended.

**Under the IR.** Interpolate the drawing calls: two paths, one halfway
between. `ir.Damage` already walks two recordings call for call, so the
machinery looks close. It is not. Damage answers "did this change", which needs
no identity; tweening answers "what did *this* become", which needs one per
path — and putting it there is the identity channel ADR 0015 and
[ADR 0007](0007-per-mark-colour.md) refuse. It would also be wrong per mark: a
bar is four corners, a smoothed line is control points that are not
measurements, a sankey band is a curve whose shape is a property of the whole
edge list. Halfway between two such paths is not halfway between two readings.

**In the geoms.** Give each mark a `Tween`. That is one interpolation per geom,
each of which has to agree with the geom's own projection — the second
implementation ADR 0015 rejected `Pick` for, arriving by another door.

**In the data.** Blend the numbers, then draw the result the way anything else
is drawn.

## Decision

**Interpolation happens in data space, before the scales. A step is a named
state of a table; between two steps is a keyed join and a numeric blend.**

- `data.Align` is the join: one entry per distinct key, saying which row of
  each table carries it. Key order is first appearance in the start state, then
  the keys only the end state has — never map iteration order, per
  [ADR 0012](0012-parallel-panels.md).
- `data.Tween` is the blend, and it is **one `Source` whose contents change**
  rather than a `Source` per frame. Columns are allocated once at the joined
  row count and rewritten in place. That is exactly the arrangement
  `data.Stream` has, for the same reason: the layer is built once, and a frame
  of an animation costs a frame.
- `refract.Transition` drives it. Every geom, coord and backend works
  unchanged, because none of them can tell that the numbers came from a blend.

### The row set is the union, and it is fixed

A key in either state is a row of the blend for the whole of it. An entering
row exists at `f == 0`; an exiting one still exists at `f == 1`.

This is the load-bearing part. It keeps the *structure* of the frame identical
between one frame and the next, which is the condition `ir.Damage` needs to
report the two comparable — and a blend whose row count changed mid-flight
would make every frame a full repaint while the picture stayed perfectly
correct. `TestATransitionKeepsItsFramesComparable` asserts on what the backend
was actually told to repaint rather than on a proxy for it.

### refract owns no clock

`Transition.At(f)` is the whole primitive: a fraction in, a frame out. It reads
no clock, starts no goroutine and schedules nothing.

That is what makes a transition a pure function of a number, and therefore
something a golden file can be taken of — `testdata/golden/transition-t50.svg`
is a real frame of a real animation rather than an approximation of one.
`Advance(now)` is sugar over it, and the loop belongs to whatever is already
running one: `requestAnimationFrame` in a browser, `window.Handler.Frame`
natively, nothing at all in a test. `Advance` reports whether it is still
running, so a window that asks for another frame only while it is stops asking
exactly once and stays idle — which is the property
[ADR 0021](0021-native-window.md) exists to protect.

### The axes stay put unless asked

`Transition.Rescale` is off by default. An axis that rescales itself every
frame is one a reader cannot compare two frames of, which is the same reason
`examples/stream` pins its Y axis and says so.

There is a second reason it cannot be the default. Learning where an axis
*ends* means releasing it and letting the render train it — and `Zoomer.Autoscale`
releases a domain fixed at construction too, so a chart that had deliberately
pinned its axis would silently lose it. Turning `Rescale` on is a caller saying
the axis does move, which is the only reading under which that is acceptable.

While it is on the domains are pinned between the two ends, and the pin is
doing a second job: a `Nice` scale re-rounds whatever it is trained on, so an
unpinned axis would relabel itself mid-move — and a changed tick *count* is a
structural change, `ir.Damage` reports the frames incomparable, and every frame
is a full repaint again.

### It refuses at construction

A column one state has and the other does not, a column numeric on one side and
textual on the other, a missing key: `data.NewTween` returns an error. This is
where the design departs from `ir.Damage`'s `ok == false`, and it should.
Damage answers mid-frame because it has no choice; a transition is built before
any frame runs, so there is somewhere honest to put the failure. Snapping to
the target from inside a render loop is a silent one.

## Consequences

- **Strings do not interpolate.** A string is a name rather than a quantity;
  half of "ingest" is not a node. Anything a geom decides from a string column
  therefore decides it abruptly: a bar's slot on a categorical axis, a discrete
  colour class, `GroupBy` membership. `data.Hold` gives a *numeric* column the
  same abruptness, for a number that is really a name.
- **A scale-type change is not a path.** Linear to log has no halfway.
- **A `data.Stream` has no identities to join on.** Under a `Window` ring the
  row numbers slide between snapshots, which is the whole reason a key exists.
  A stream animates by being redrawn, which already works; the two mechanisms
  are mutually exclusive and neither wants the other.
- **Enter and exit do not fade.** Opacity is a property of a layer, not of a
  row, and a per-row opacity channel is the IR change ADR 0007 refuses.
  `EnterFrom` and `ExitTo` put the answer in data space instead, so a bar grows
  out of its baseline. The default is to hold — a new row appears in place —
  because that needs no configuration and is never wrong.
- **A duplicate key takes its first row.** Refusing the whole transition over
  one would be refusing to animate a chart that draws perfectly well. What it
  costs is that the second row does not move, which is visible.
- An animated chart that is also being hovered renders serially, because an
  `Observer` or a `RowSink` takes the serial path (`render/parallel.go`). That
  is a consequence of watching a render, not of animating one.
- A frame of a transition allocates what a frame allocates:
  `BenchmarkTransitionFrame1k` and `…100k` are both 80, and the gate compares
  them.

## Revisit if

Someone wants more than two states. A timeline is a list of two-state
transitions and building one out of the pair is a host-side loop, so the pair
shipped first deliberately — but a keyframe API that owned the sequence would
be a real addition rather than sugar, and it would be argued on the evidence of
people actually writing that loop.

A transition has **no spec representation**, and that is not an oversight
either: a spec document is a chart, and an animation is a sequence of charts. A
`steps` array would be a second document type wearing the first one's schema.
Nothing about [ADR 0014](0014-json-spec.md)'s freeze blocks adding one later.
