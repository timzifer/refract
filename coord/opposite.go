package coord

import (
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Opposite is implemented by a coord that can place a second axis on the far
// side of the panel from the first: a vertical one down the right-hand edge, a
// horizontal one along the top.
//
// It is an optional interface, for the reason every widening of this package
// since v0.8 has been one: [Coord] is implemented outside it and never gains a
// method ([CONCEPT §15](../CONCEPT.md#15-versioning--stability)). A coord that
// does not implement it draws no second axis, which is the honest answer for
// [Polar]: a ring has one angular axis and one radial one and no far side to
// put another on, and a second one drawn over the first would be two scales
// sharing one line.
//
// The two methods are one interface rather than two because they are one
// capability — "this coord has edges opposite its axes" — and a coord that can
// answer for one direction can answer for the other. Cartesian implements
// both; Polar implements neither.
//
// It answers the *furniture* only. Whether a chart has a second axis, which
// layers read it and whether the panel writes its labels are all decisions
// render already owns, exactly as they are for the first one — a coord reports
// where things go and does not draw. See
// [ADR 0036](../docs/adr/0036-secondary-axis.md).
type Opposite interface {
	// FurnitureY2 fills dst with the axis line, tick marks and tick label
	// positions of a second vertical axis, opposite the one [Coord.Furniture]
	// places.
	//
	// It fills the Y side of dst and nothing else, so a caller keeps one
	// Furniture per axis rather than one with two of everything in it. The
	// slices are parallel to ticks, as they are everywhere in this package.
	//
	// It places **no grid lines**. Two ladders of rules at different values
	// are a moiré rather than a reading, and which of the two scales a line
	// belongs to is unanswerable by looking — so the grid stays the primary
	// axis's, and the second axis is a line, its ticks and its labels.
	FurnitureY2(dst *Furniture, area ir.Rect, m Metrics, ticks []scale.Tick)

	// FurnitureX2 is FurnitureY2 turned a quarter turn: a second horizontal
	// axis along the panel's top edge, filling the X side of dst.
	FurnitureX2(dst *Furniture, area ir.Rect, m Metrics, ticks []scale.Tick)
}

// FurnitureY2 implements [Opposite] for a Cartesian coord: the mirror image of
// the Y axis in [cartesian.Furniture], reaching right instead of left and
// left-aligning its labels instead of right-aligning them.
//
// It is written out rather than derived from the first axis by reflection,
// because a reflection would have to know which of the label's alignments and
// which of the tick's endpoints to flip — and getting one of those wrong
// produces labels inside the plot, which is a bug that looks like a theme.
func (cartesian) FurnitureY2(dst *Furniture, area ir.Rect, m Metrics, ticks []scale.Tick) {
	y := dst.y()
	y.axis.line(ir.Point{X: area.Max.X, Y: area.Min.Y}, ir.Point{X: area.Max.X, Y: area.Max.Y})
	for _, t := range ticks {
		_, tick := y.next()
		if !inRange(t.Pos, area.Min.Y, area.Max.Y) {
			y.mark(false, Label{})
			continue
		}
		if l := m.tickLen(t); l > 0 {
			tick.line(ir.Point{X: area.Max.X, Y: t.Pos}, ir.Point{X: area.Max.X + l, Y: t.Pos})
		}
		y.mark(true, Label{
			At: ir.Point{X: area.Max.X + m.labelGap(), Y: t.Pos},
			H:  ir.AlignStart,
			V:  ir.AlignMiddle,
		})
	}
}

// FurnitureX2 implements [Opposite] for a Cartesian coord: the mirror image of
// the X axis in [cartesian.Furniture], reaching up instead of down and hanging
// its labels above the ticks instead of below them.
//
// It reports [Furniture.XLabelsShareARow], because they do: labels along the
// top of a panel collide with each other exactly as those along the bottom do,
// and render drops the ones that would overlap on the same evidence.
func (cartesian) FurnitureX2(dst *Furniture, area ir.Rect, m Metrics, ticks []scale.Tick) {
	dst.XLabelsShareARow = true

	x := dst.x()
	x.axis.line(ir.Point{X: area.Min.X, Y: area.Min.Y}, ir.Point{X: area.Max.X, Y: area.Min.Y})
	for _, t := range ticks {
		_, tick := x.next()
		if !inRange(t.Pos, area.Min.X, area.Max.X) {
			x.mark(false, Label{})
			continue
		}
		if l := m.tickLen(t); l > 0 {
			tick.line(ir.Point{X: t.Pos, Y: area.Min.Y - l}, ir.Point{X: t.Pos, Y: area.Min.Y})
		}
		x.mark(true, Label{
			At: ir.Point{X: t.Pos, Y: area.Min.Y - m.labelGap()},
			H:  ir.AlignCenter,
			V:  ir.AlignBottom,
		})
	}
}

// The framed Cartesian coord is the value a panel actually holds —
// [Coord.Frame] hands back the coord positioned in the panel — so it has to
// answer everything the unframed one does.
func (f framedCartesian) FurnitureY2(dst *Furniture, area ir.Rect, m Metrics, ticks []scale.Tick) {
	f.cartesian.FurnitureY2(dst, area, m, ticks)
}

func (f framedCartesian) FurnitureX2(dst *Furniture, area ir.Rect, m Metrics, ticks []scale.Tick) {
	f.cartesian.FurnitureX2(dst, area, m, ticks)
}

// OppositeFurniture fills dst with cd's second axis in one direction,
// reporting whether cd has one to place.
//
// It is the type assertion written once, so that a caller asks the question
// rather than knowing which coords answer it. vertical picks which of the two
// edges is meant: the right-hand one, or the top.
func OppositeFurniture(cd Coord, dst *Furniture, area ir.Rect, m Metrics, ticks []scale.Tick, vertical bool) bool {
	o, ok := cd.(Opposite)
	if !ok {
		return false
	}
	if vertical {
		o.FurnitureY2(dst, area, m, ticks)
	} else {
		o.FurnitureX2(dst, area, m, ticks)
	}
	return true
}
