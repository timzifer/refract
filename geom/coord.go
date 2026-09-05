package geom

import (
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/ir"
)

// What a geom has to do differently once the stage under it is pluggable.
//
// A geom still asks the scales where a value goes and still emits IR; what
// changes is the step between. A mapped pair is no longer a device point, an
// edge between two marks is no longer a straight line, and a rectangle in data
// space is no longer a rectangle on screen. These are the three helpers that
// answer those, written once so that every geom answers them the same way.
//
// Under [coord.Cartesian] each of them is what the code did before there was a
// coord to ask: Points copies the pair through, an edge is a LineTo, and a data
// rectangle is four corners in the order [ir.Path.Rect] writes them. That is
// what makes the golden files the proof of this refactor rather than its
// casualty.

// strokeRun strokes a run of marks: a polyline where the coord draws straight
// edges, and a path of the coord's own edges where it does not.
//
// The polyline is not an optimisation to be tidied away. A Cartesian chart's
// line, grid and whisker have reached the backend as Polyline calls since v0.1,
// and every golden file, every damage rectangle and every hit-testing bound in
// the repository is written in terms of them.
func strokeRun(b ir.Backend, cd coord.Coord, p *ir.Path, pts []ir.Point, st ir.Stroke, closed bool) {
	if len(pts) < 2 {
		return
	}
	if cd.Straight() && !closed {
		b.Polyline(pts, st)
		return
	}
	p.Reset()
	appendEdges(p, cd, pts, true)
	if closed {
		closeLoop(p, cd, pts)
	}
	b.StrokePath(p, st)
}

// closeLoop joins the last mark of a run back to the first, which is the edge
// that makes a radar a contour rather than an open line with a gap in it.
func closeLoop(p *ir.Path, cd coord.Coord, pts []ir.Point) {
	if len(pts) < 3 {
		return
	}
	cd.Edge(p, pts[len(pts)-1], pts[0])
	p.Close()
}

// appendEdges appends the marks to p as the coord's own edges, starting a new
// subpath when move is set.
func appendEdges(p *ir.Path, cd coord.Coord, pts []ir.Point, move bool) {
	if len(pts) == 0 {
		return
	}
	if move {
		p.MoveTo(pts[0].X, pts[0].Y)
	} else {
		p.LineTo(pts[0].X, pts[0].Y)
	}
	for i := 1; i < len(pts); i++ {
		cd.Edge(p, pts[i-1], pts[i])
	}
}

// area appends one data-space rectangle to p through the coord: four corners
// under Cartesian, an annular sector under Polar.
func area(p *ir.Path, cd coord.Coord, r ir.Rect) {
	cd.Area(p, r.Min.X, r.Min.Y, r.Max.X, r.Max.Y)
}

// ordered puts the two ends of an axis interval in ascending order.
//
// [coord.Coord.Extent] reports the interval each scale maps into, and a
// Cartesian Y scale maps into a *descending* one so that larger values are
// higher on screen. A mark that crosses the whole plot has no opinion about
// which end it starts at, so it starts at the smaller one — which is what
// keeps a vertical rule running top to bottom, exactly as it has since v0.3.
func ordered(a, b float32) (float32, float32) {
	if b < a {
		return b, a
	}
	return a, b
}
