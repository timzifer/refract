package geom

import (
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
)

// Breaking a mark out of the middle of the coord: what a donut slice pulled
// out of its ring is doing, and the one thing about a pie that a stacked bar
// in a polar coord could not already say.
//
// It is a displacement of the mark rather than a change to its extent, and
// that distinction is the whole of it. A slice moved outwards keeps the angle,
// the inner radius and the outer radius the data gave it — so it still reads
// against the axis it was measured on, and the ring it left behind still shows
// where it came from. Growing the radius instead would move the ink and change
// the reading, which is what makes an exploded pie a lie in most of the tools
// that draw one. See docs/adr/0026-breaking-a-mark-out.md.

// Explode breaks a layer's marks out of the middle of the coord, by the given
// fraction of its outer radius.
//
// It is what pulls a slice out of a donut, and a tenth is about as far as one
// goes before the ring stops reading as a ring:
//
//	geom.Bar(src, geom.X("all"), geom.Y("share"), geom.GroupBy("browser"),
//	    geom.Explode(0.08))
//
// Every mark of the layer moves, which is rarely what a chart means — one
// slice is usually the point. [ExplodeBy] is that, per row.
//
// A coord with no middle to move away from ignores it: see
// [github.com/timzifer/refract/coord.Exploder], which [coord.Cartesian]
// deliberately does not implement.
func Explode(f float64) Option { return func(c *config) { c.explode = f } }

// ExplodeBy reads the break-out per row from a column of fractions, so that
// one slice leaves the ring and the rest stay in it:
//
//	geom.Bar(src, geom.X("all"), geom.Y("share"), geom.GroupBy("browser"),
//	    geom.ExplodeBy("pull"))
//
// A row whose value is zero or missing does not move. The column wins over a
// constant [Explode] where it has a value, which is how a layer says "this far,
// except here".
func ExplodeBy(col string) Option { return func(c *config) { c.explodeCol = col } }

// breakOut is a layer's break-out resolved for one Build: the coord that can
// answer how far a mark moves, and how far this layer asked each of them to.
//
// The type assertion is made once here rather than per mark, exactly as a geom
// resolves [github.com/timzifer/refract/scale.Band] once — and a layer that
// asked for nothing, or one drawn in a coord with no middle, resolves to the
// zero value, whose [breakOut.on] is false and which costs the drawing path a
// comparison.
type breakOut struct {
	cd  coord.Exploder
	by  float64
	col []float64
}

// breaking resolves c's break-out against the coord the layer is drawn in. col
// is the per-row column, or nil.
func (c config) breaking(cd coord.Coord, col []float64) breakOut {
	if c.explode == 0 && col == nil {
		return breakOut{}
	}
	e, ok := cd.(coord.Exploder)
	if !ok {
		return breakOut{}
	}
	return breakOut{cd: e, by: c.explode, col: col}
}

// on reports whether anything is broken out at all.
func (b breakOut) on() bool { return b.cd != nil }

// at is the device displacement of the mark whose mapped extent is r, drawn
// for source row i.
func (b breakOut) at(r ir.Rect, i int) ir.Point {
	by := b.by
	if b.col != nil {
		if v := b.col[i]; finite(v) {
			by = v
		} else {
			by = 0
		}
	}
	if by == 0 {
		return ir.Point{}
	}
	dx, dy := b.cd.Explode(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y, by)
	return ir.Point{X: dx, Y: dy}
}

// trainBreakOut reads the per-row break-out column, checked against the length
// of the layer's own columns. It is nil for a layer that named none, which is
// every layer written before there was one.
func (c config) trainBreakOut(src data.Source, n int) ([]float64, error) {
	if c.explodeCol == "" {
		return nil, nil
	}
	v, err := column(src, c.explodeCol, nil)
	if err != nil {
		return nil, err
	}
	if len(v) != n {
		return nil, errLength(c.xcol, c.explodeCol, n, len(v))
	}
	return v, nil
}

// offsets collects the displacement of each mark a layer is about to draw,
// index for index with the rects it collected. It answers nil when nothing is
// broken out, which is what keeps the drawing loops of every other chart on
// the path they were on.
//
// The buffer comes from the scratch for the reason every buffer here does: it
// is sized by the data, and a chart redrawn every frame must not pay for its
// data twice.
func (sc *scratch) offsets(b breakOut, rects []ir.Rect, rows []int) []ir.Point {
	if !b.on() {
		return nil
	}
	offs := sc.offs[:0]
	for i, r := range rects {
		offs = append(offs, b.at(r, rows[i]))
	}
	sc.offs = offs
	return offs
}

// areaAt appends one data-space rectangle through the coord, displaced by d.
//
// The mark is built where the data says and then moved, rather than the coord
// being asked for a second transform: a broken-out slice is the same annular
// sector it always was, somewhere else. Moving the points the call appended
// costs no allocation and no second path.
func areaAt(p *ir.Path, cd coord.Coord, r ir.Rect, d ir.Point) {
	n := len(p.Pts)
	area(p, cd, r)
	if d.X == 0 && d.Y == 0 {
		return
	}
	for i := n; i < len(p.Pts); i++ {
		p.Pts[i].X += d.X
		p.Pts[i].Y += d.Y
	}
}

// offsetAt is offs[i] where a layer has displacements and the origin where it
// has none, so that a drawing loop reads the same either way.
//
// The test is the length rather than nil: a batch's displacement buffer comes
// back from the pool emptied rather than cleared away, so a layer that breaks
// nothing out is handed a non-nil slice with nothing in it.
func offsetAt(offs []ir.Point, i int) ir.Point {
	if i >= len(offs) {
		return ir.Point{}
	}
	return offs[i]
}
