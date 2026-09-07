package coord

import (
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Opposite is implemented by a coord that can place a second vertical axis on
// the far side of the panel from the first.
//
// It is an optional interface, for the reason every widening of this package
// since v0.8 has been one: [Coord] is implemented outside it and never gains a
// method ([CONCEPT §15](../CONCEPT.md#15-versioning--stability)). A coord that
// does not implement it draws no second axis, which is the honest answer for
// [Polar]: a ring has one radial axis and no far side to put another on, and a
// second radius drawn over the first would be two scales sharing one line.
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
	// It places **no grid lines**. Two ladders of horizontal rules at
	// different values are a moiré rather than a reading, and which of the two
	// scales a line belongs to is unanswerable by looking — so the grid stays
	// the primary axis's, and the second axis is a line, its ticks and its
	// labels.
	FurnitureY2(dst *Furniture, area ir.Rect, m Metrics, ticks []scale.Tick)
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

// FurnitureY2 implements [Opposite] for a framed Cartesian coord, which is the
// value a panel actually holds — [Coord.Frame] hands back the coord positioned
// in the panel, and the framed one has to answer everything the unframed one
// does.
func (f framedCartesian) FurnitureY2(dst *Furniture, area ir.Rect, m Metrics, ticks []scale.Tick) {
	f.cartesian.FurnitureY2(dst, area, m, ticks)
}

// OppositeFurniture fills dst with cd's second vertical axis, reporting
// whether cd has one to place.
//
// It is the type assertion written once, so that a caller asks the question
// rather than knowing which coords answer it.
func OppositeFurniture(cd Coord, dst *Furniture, area ir.Rect, m Metrics, ticks []scale.Tick) bool {
	o, ok := cd.(Opposite)
	if !ok {
		return false
	}
	o.FurnitureY2(dst, area, m, ticks)
	return true
}
