# 0052 — The camera is a value, and turning it is the host's loop

**Status:** Planned · **Date:** 2026-09-08 · **Implementation:** not started

## Context

The third of the features [ADR 0050](0050-depth-without-a-third-axis.md) split
apart: a reader drags across the chart and the scene turns. It is the one
people picture when they say 3D, and it is last because it is worth nothing
without [0051](0051-three-dimensional-charts.md) and everything after it — a
projected scene is genuinely hard to read from one fixed angle, and a
projection that cannot be turned is a projection whose occlusions are permanent.

The question is not how to rotate a matrix. It is who owns the drag,
[CONCEPT §5](../../CONCEPT.md#5-non-goals) having said twice what refract is
not: "Not a GUI toolkit. It renders into a window; it does not own widgets or
the event loop."

The core already answers this twice, in opposite-looking ways that are the same
answer. `Live.Wheel`, `Live.PanBy` and `Live.ZoomTo` do own the arithmetic of
zooming and panning — because that arithmetic is *about the scales*, and the
scales are refract's. [ADR 0045](0045-linked-views.md) declines to own linked
views — because a link is a statement about two charts, and refract's model is
about one. The rule that produces both: **refract owns what is a statement
about the chart, and the host owns what is a statement about the session.**

## Decision

**A camera is an immutable value the caller holds. `three` turns pointer deltas
into cameras and cameras into frames; it installs no handler, opens no window
and runs no loop.**

```go
cam := three.LookAt(three.Azimuth(-0.6), three.Elevation(0.35))

live, _ := plot3.Live(target)     // mirrors refract.Live's shape
live.Camera(cam)
live.Draw()

// the host's event handler, wherever the host's events come from
live.Camera(three.Orbit(live.CameraValue(), dx, dy))
live.Draw()
```

`three.Orbit(cam, dx, dy) Camera` is a pure function: same inputs, same camera,
no state, testable without a surface. `Orbit` clamps elevation to just inside
the poles, because a camera looking straight down its own up-vector has no
up-vector and the scene would spin on its axis at the moment the reader least
expects it.

**A rotation is not an event kind.** `interact.EventKind` does not gain
`Orbit`. The comment on `Select` says why the question is even asked — "it is
last because the kinds before it are the ones a chart has always had" — and the
answer is that a camera is not a hit and the module does not need refract's
event vocabulary to receive two floats. The core is untouched by this record as
completely as it is by 0051.

### The camera is not data, so an orbit is not a transition

[ADR 0044](0044-transitions.md) put interpolation in data space, before the
scales, because that is the only place identity means anything. A camera has no
rows, no keys and no domain: interpolating between two of them is interpolating
a *view*, which is exactly the "under the IR" tweening 0044 refused for data —
and is trivially correct for a view, because a view has no identity to lose.

So easing a camera from one angle to another is a host-side loop over
`three.Slerp(a, b, t)` and the easings in `transition.go`, and it does not go
anywhere near `Live.Transition`. Animating the camera and animating the data are
different features that happen to both produce frames.

### Only the live backends can be turned

- **Canvas, window, GPU tier**: a frame loop exists; an orbit is a redraw.
- **SVG, PDF**: a camera is a parameter of the render, and a chart is written
  once at the angle it was asked for. There is no interactive SVG. Emitting a
  document with a script that re-projects the scene in the viewer means shipping
  a second renderer, in another language, inside a file
  ([ADR 0004](0004-svg-source-of-truth.md) makes the built-in emitter the only
  SVG path precisely so that there is one).

### A drag skips the damage diff

`Live.Draw` records a frame, calls `ir.Damage` to diff it against the last one,
and hands the rectangles to `ir.Partial` where the backend has it
([ADR 0016](0016-streaming-and-damage.md)). Under an orbit, **every drawing call
differs**, because every point moved. The diff would walk two whole recordings
to arrive at a result known before it started: the panel.

So the module's live loop damages the panel outright while a drag is in flight
and takes the diffing path when the camera is at rest — where it earns its keep
exactly as it does in 2D, because a changing dataset under a fixed camera is the
case damage was designed for.

### Per-frame cost is a gate, not an aspiration

`TestARenderDoesNotAllocatePerPoint` and `.github/scripts/allocgate.awk` bind
the core, and a module that redraws on every pointer move needs the same
discipline for the same reason. Three consequences, all decided now rather than
discovered in a profile:

- Projection writes into pooled `[]ir.Point`, as `coord.Points` does.
- The depth sort runs over a pooled key slice with `slices.SortFunc`, never
  `sort.Slice` — which takes a closure and a reflect-based swapper, and
  allocates per call on the hot path.
- A surface's back-to-front traversal allocates nothing at all: it is an
  iteration order over the grid, chosen from the sign of the view direction
  (0051), not a sort.

### Two orders that would flicker, and how each is settled

**Depth ties.** Two marks at equal depth may swap between frames if their order
is undefined, and a scene that shimmers while the reader holds the mouse still
is a bug in the picture. The tie-break is (layer, row), which is total and
independent of everything else — 0012's rule.

**Near-ties.** Two marks whose depths differ by a millionth swap legitimately as
the camera turns, and they should: that is what turning past each other looks
like. No hysteresis is added. Hysteresis would make the current frame depend on
which frames preceded it, and a chart whose picture depends on its history is a
chart that cannot be golden-tested.

### A chart that must be turned to be read is a chart some readers cannot read

[ADR 0024](0024-accessibility.md) says a chart says what it is in three
channels — a name, a description, and its data — and none of them is a camera.
Two things follow, and they are requirements rather than remarks:

- **The description is camera-independent.** It reports the three ranges and
  what is plotted, and does not change when the scene turns. A reader using it
  gets the same chart as a reader dragging it.
- **The default camera must be readable on its own.** A scene whose initial
  angle hides its own data behind itself is broken for everyone who cannot drag
  — and for every static export, which is most of them. `three` ships a default
  three-quarter view, and `live.Home()` returns to it, so a reader who has
  turned the scene into a mess is one call from the picture the author chose.

## What this does not do

- **No handlers, no window, no loop.** The host has an event source already;
  `Plot.Live` has always taken whatever it is.
- **No fly-through, no walk, no free camera.** Orbit, dolly and reset are the
  three verbs a chart needs. A camera that can be anywhere is a scene viewer.
- **No inertia, no momentum, no spring.** Those are properties of an
  interaction's *feel*, which belongs to the host's input layer, and a library
  that decided one would be deciding it for every host.
- **No perspective**, still. 0050 refused it for the reading and 0051 kept the
  refusal; being able to turn the scene is not an argument for distorting it.
- **No orbiting of a 2D chart.** `coord.Oblique`'s depth vector is a constant
  of the chart (0050) and turning it would be animating a decoration. A reader
  who wants to look at data from another angle wants 0051's chart.
