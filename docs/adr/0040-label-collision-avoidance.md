# 0040 — Label placement belongs to the panel layout

Status: accepted; implemented in v1.5.0.

ADR 0032 deferred neighbouring labels because a text geom owns rows, not the
positions of other layers. `geom.AvoidOverlap(true)` now requests placement
through the optional `geom.LabelAvoider` interface. `render` lends participating
layers one `Frame.Labels` per panel. Its implementation measures through the
active backend and calls `internal/layout.Labels`; no geom imports layout and
the frozen Backend and Geom interfaces gain no methods.

The placer tries the original anchor, above, below, right, left and four
diagonals, in that order. Candidate spacing comes from the aligned, rotated
font and ink bounds. Earlier layers and source rows win. A candidate must fit
inside the panel rectangle and clear earlier participating labels; otherwise
the label is omitted. A box label may only keep its original anchor: moving
one into its neighbour's box would change what the chart says.

The implementation is bounded and deterministic, with no random seed or
convergence loop. It retains accepted rectangles in a pooled buffer. Each panel
has independent state, and serial, parallel and watched renders draw the same
result. Rows are reported at the anchors actually drawn; omitted labels report
no row. A frame that opts out takes no layout state and keeps the old calls.

Only opted-in text layers participate, not points, paths, annotations or axes.
There are no leader lines. Polar and other coords retain their own clip;
rectangular containment alone cannot guarantee that text avoids that clip.
A caller driving a geom directly can supply a LabelPlacer; with nil Labels the
geom keeps its original placement. The spec writes `avoidOverlap` only on text.

Revisit when callouts need leader lines, a spatial index is justified by
measured label-heavy workloads, or obstacles beyond text are required.
