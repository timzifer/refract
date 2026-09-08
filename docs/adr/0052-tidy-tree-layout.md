# 0052 — A tidy tree is a bounded deterministic layout, and it is not a force simulation

**Status:** Proposed · **Date:** 2026-09-08 · **Implemented:** —

## Context

[ADR 0039](0039-relational-layouts.md) shipped four relational marks and
declined one:

> **A node-link / force layout.** It is in bucket E, and it is the one member
> that cannot be a pure function of its input in any bounded sweep count that
> also looks good — a force simulation's whole method is to run until it
> settles.

and named what would reopen it:

> A node-link layout arrives with a bounded, deterministic solver. The node
> table and the five channels here are what it would read.

That reasoning is correct and this record does not weaken it. What this record
observes is that it was applied to a category one size too large. It binds
**force** layouts. It does not bind **tree** layouts, and a tree layout is
where most of the charts in that category actually live.

### The layout the argument does not reach

The Reingold–Tilford tidy tree, in the linear-time form Buchheim, Jünger and
Leipert published, is:

- **O(n)** — one post-order pass to place subtrees and one pre-order pass to
  apply the accumulated shifts;
- **deterministic** — no randomness, no annealing, no tie-breaking by map
  order;
- **bounded** — it does not iterate towards anything, so there is no sweep
  count to choose and no "run until it settles";
- **a pure function of its input** — a node table in, two coordinates per node
  out.

That is `stat.Squarify`'s shape exactly, and `stat.Squarify` is already in the
package. It also has published correctness properties that make good test
oracles: no two subtrees overlap, a parent sits centred over its children, and
two isomorphic subtrees are drawn identically wherever they appear.

### What it draws

| Chart | What it is |
|---|---|
| Dendrogram | a tree whose node heights are merge distances — the output of every hierarchical clustering there is |
| Phylogram / cladogram | the same, with branch lengths — the standard instrument of phylogenetics |
| Org chart, decision tree, file tree, dependency tree | a tree whose heights are its depth |
| Radial dendrogram | the same mark under `coord.Polar()` |
| **Clustered heatmap** | `geom.Rect` with a dendrogram in a `Plot.Track` on two edges |

The last one is the point. It is the single most-published figure shape in
bioinformatics, it needs a rectangle mark, a colour ramp, an ordinal axis and a
band at a panel's edge on a shared axis, and refract has had all four since
v0.10 and cannot draw the figure because the band has nothing to put in it.

## Decision

**`geom.Tree` is a fifth relational mark, laying a tidy tree out in the unit
square, and the coordinate stage decides what it looks like.**

It reads the hierarchy channels [ADR 0039](0039-relational-layouts.md) already
defined — `geom.ID`, `geom.Parent`, `geom.Value` — and adds none. That is not a
coincidence: 0039's revisit clause predicted the node table would be what a
node-link layout reads, and this is the prediction coming true.

### It fills the unit square, so the radial version is free

Every layout in bucket E fills the unit square — a span across, a height out —
and this one keeps the convention: the breadth axis carries the leaf order and
the depth axis carries the height. So:

- under `coord.Cartesian`, a dendrogram or an org chart;
- under `coord.Polar()`, a **radial dendrogram**, with the root at the hub;
- with `geom.Baseline(1)`, the root at the rim instead.

This is the third time the move pays: an icicle under a polar coord is a
sunburst, an arc diagram with its rail at the rim is a chord diagram, and a
tidy tree under a polar coord is a radial dendrogram. None of the three is a
mark of its own, and this record adds one mark and gets two charts, exactly as
0039 got six charts from four.

### The height comes from a column or from the depth

Two conventions, and the channels tell them apart with no option:

- **`geom.Value` present** — the node's height is that column. This is a
  dendrogram's merge distance and a phylogram's branch length, and the depth
  axis is continuous and trains on it.
- **`geom.Value` absent** — the height is the node's depth from
  `stat.Depth`, which already exists. This is an org chart, and the axis is
  the integers.

`stat.Rollup` and `stat.Partition` are not used here; a tree's geometry is its
structure, not its subtree totals, which is the difference between this mark
and the icicle beside it.

### An edge is an elbow by default and a segment on request

The classic dendrogram bracket is orthogonal — out from the child, across, in
to the parent — and a phylogram's edge is the straight segment. Both are
drawn as data-space points and go through the coord, so under a polar coord
the bracket's cross-piece becomes the arc a radial dendrogram is printed with,
with no polar-specific code. It is the same shape of policy `coord.Chord` and
`coord.SmithArc` are, made in the mark because the choice is about what the
edge *means* rather than about what the coordinate system does to it.

### Leaf order is the table's, and this record does not reorder

Leaf order comes from first appearance in the source table, never from map
iteration — ADR 0012's rule, and the one 0039 tested for its sankey's nodes.
Reordering leaves to reduce crossings or to rotate a clustering's branches is
refused for 0039's reason, quoted because it is unchanged:

> a sort is where a layout stops being a pure function of its input and starts
> depending on how a tie was broken.

A caller who wants a particular leaf order — an optimal leaf ordering out of
their clustering library, say — sorts their own rows, which is what
`geom.Order` is for.

## Consequences

| | |
|---|---|
| `render`, `ir`, `coord`, `scale`, `layout` | unchanged |
| `stat` | `Tidy` and `AppendTidy`, one bounded pure function with a determinism test and the three published properties as its oracle |
| `geom` | one mark, reusing 0039's channels, its cycle and duplicate-node validation, its legend behaviour, its hit test and its refusal of an ordinal axis (`ErrNotContinuous`) |
| `spec` | a `"tree"` mark and an edge-shape field |
| `docs/chart-types.md` | bucket E's "Node-link — missing" becomes "force-directed node-link — missing", which is a more honest line |
| Charts unlocked | dendrogram, phylogram, radial dendrogram, org and decision trees, and the clustered heatmap that has been one missing band away since v0.10 |

The honest cost: the Buchheim contour-and-thread mechanism is about 150 lines
and is famously easy to get subtly wrong. It is *fiddly-known* rather than
open-ended — there is a published algorithm, published invariants and a
reference implementation to test against — which is the opposite of the force
layout's problem and is why this record can be written and that one cannot.

## Not in scope

- **Force-directed layouts and general graphs.** 0039's refusal stands
  verbatim. A tree has a parent per node and no cycles, and every property
  above depends on that. This record narrows the missing category; it does not
  empty it.
- **Crossing minimisation and optimal leaf ordering**, per the argument above.
- **Edge bundling**, which is a smoothing over a layout rather than a layout.
- **Computing the clustering.** A dendrogram's merge heights come from a
  hierarchical clustering, and clustering is an analysis rather than a
  reduction for drawing. `data.Source` is the boundary: the caller's columns
  are `(id, parent, height)` however they were produced.
- **Venn and UpSet.** Untouched by this. UpSet is a matrix chart and is
  probably a `Grid` question rather than a relational one, which is worth
  saying because it makes it cheaper than its neighbour in the same sentence
  of 0039.

## Revisit if

- A DAG turns up — a dependency graph that is nearly a tree. Layered
  (Sugiyama) layout is bounded too, but its crossing-reduction phase is a
  heuristic sort, which is the thing this record and 0039 both refuse. It
  would need its own answer to that, not an option here.
- A force layout arrives with a bounded solver after all. 0039's clause is
  still the live one; this record does not consume it.
