// Package coord maps scaled positions into device space.
//
// A scale maps a data value into an interval; a coord decides what that
// interval means. [Cartesian] says it is a distance along an edge of the panel
// and is the identity, so every geom draws what it always drew. [Polar] says
// one of the two intervals is an angle and the other a radius, and the same
// geoms then draw a pie, a donut, a radar, a rose or a gauge.
//
// That is the whole of it: the coord is a stage between the scales and the IR,
// and neither end changes. [github.com/timzifer/refract/scale.Scale] is
// untouched — what used to be Cartesian was only that render passed the panel
// rectangle's edges as the interval — and the IR is untouched too, because an
// arc is cubics and [github.com/timzifer/refract/ir.Path] has always had
// those. See docs/adr/0018-coordinate-systems.md.
//
// # What a coord does not do
//
// It does not paint. [Coord.Furniture] reports where a panel's grid lines,
// axis lines and tick labels go and render strokes them, because render is the
// only package that knows the drawing order of a chart. A coord that drew its
// own rings would be a second drawing order.
//
// It does not own a panel either. A coord belongs to a chart, and
// [Coord.Frame] hands back the coord positioned in one panel's rectangle
// rather than moving the receiver into it — panels are built concurrently, and
// a coord that remembered which panel it was in would be a data race.
package coord

import (
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Coord turns a pair of mapped positions into a device point, and reports the
// geometry of everything else that depends on what the pair means.
//
// Implementing one is how a coordinate system is added. The set of methods is
// wider than a transform because a coordinate system is wider than a
// transform: what a straight edge is, what a data-space rectangle is, what a
// panel clips to, and where a grid line runs are all answers only the coord
// has.
//
// # Stability
//
// Coord is implemented outside this module, so it never gains a method. What a
// coordinate system can additionally do — break a mark out of its middle, say
// what it is — is an optional interface beside it, as [Exploder] and
// [Describer] are, and a projection's own needs arrive the same way. A coord
// this package does not define is read back through [Register].
type Coord interface {
	// Frame gives the coord a panel rectangle and that panel's scales, sets
	// the interval each scale maps into, and returns the coord positioned in
	// the rectangle. Cartesian sets the rectangle's edges; Polar sets an angle
	// range and a radius range.
	//
	// The receiver is not modified: the returned value is what the panel's
	// geoms are handed, so two panels drawn on two goroutines never share one
	// position.
	Frame(area ir.Rect, x, y scale.Scale) Coord

	// Extent reports the interval each scale maps into — what Frame chose.
	// A mark that spans a whole axis needs it: the far end of a rule is where
	// the scale ends, and under a polar coord that is not where the rectangle
	// does.
	Extent() (x0, x1, y0, y1 float32)

	// Point turns one mapped pair into a device point.
	Point(x, y float32) ir.Point

	// Points is the batch form, and the one a geom on the hot path calls. It
	// appends into dst, which the caller owns and reuses between frames.
	//
	// It exists so that a per-row interface call does not reappear here: a
	// variadic or per-row method on this interface is the shape that cost a
	// million allocations on a million-row column once already.
	Points(dst []ir.Point, xs, ys []float32) []ir.Point

	// Straight reports whether an edge that is straight in data space is also
	// straight on screen. It is true for Cartesian and for a polar coord asked
	// for chords; it is false for one drawing arcs, where a geom has to build
	// a path instead of a polyline.
	Straight() bool

	// Edge appends the device path of an edge that is straight in data space,
	// continuing from p's current point. Cartesian appends one LineTo; Polar
	// appends the cubics of an arc.
	Edge(p *ir.Path, from, to ir.Point)

	// Area appends the closed device path of a data-space rectangle, given as
	// two mapped pairs. Cartesian appends four corners; Polar appends an
	// annular sector.
	Area(p *ir.Path, x0, y0, x1, y1 float32)

	// Clip appends the path a panel's data is clipped to: the rectangle, or
	// the disc inscribed in it.
	Clip(p *ir.Path, area ir.Rect)

	// Invert turns a device point back into a mapped pair, which is what a
	// tooltip needs before it asks the scales what the values were.
	Invert(pt ir.Point) (x, y float32)

	// Furniture reports the geometry of the grid lines, the axis lines and the
	// tick labels of one panel, one entry per tick in tick order. It fills dst
	// rather than returning a value so that a chart redrawn every frame does
	// not pay for its furniture again; render resets and reuses one.
	Furniture(dst *Furniture, area ir.Rect, m Metrics, xTicks, yTicks []scale.Tick)

	// Decimates reports whether a reduction defined over pixel columns still
	// measures what it was defined to measure under this coord.
	//
	// It is true for Cartesian, where [github.com/timzifer/refract/stat.LTTB]
	// and MinMax bucket by a column of screen and that is exactly the unit
	// they were designed in. It is false for Polar, where a bucket of equal
	// angle is not a bucket of equal width — and nothing polar is a big-data
	// chart, so saying no costs nothing. See docs/adr/0011-decimation.md.
	Decimates() bool
}

// Describer is implemented by a coord that can write itself down, so that a
// chart survives the round trip through the JSON spec. It sits beside
// [github.com/timzifer/refract/scale.Describer] and is optional for the same
// reason: a coord nobody serialises does not have to know what JSON is.
type Describer interface {
	Describe() Desc
}

// Exploder is implemented by a coord with a middle for a mark to be moved away
// from: what a slice broken out of a donut is doing.
//
// It is an optional interface, like [Describer], and [Cartesian] deliberately
// does not implement it. A rectangle on a Cartesian panel has no direction to
// be broken out in — every bar would move the same way, which is a translation
// of the layer rather than a reading of it — so a layer asking to break its
// marks out under a coord that has no middle draws exactly what it drew, and
// nothing is silently invented.
//
// The displacement is answered rather than applied, because a coord does not
// draw: the geom that built the mark moves the path it built. A geom resolves
// the interface once per Build rather than per mark, exactly as it does
// [github.com/timzifer/refract/scale.Band]. See
// docs/adr/0026-breaking-a-mark-out.md.
type Exploder interface {
	// Explode reports the device displacement of a mark whose extent in the
	// space the scales map into is the given pair of mapped positions, when it
	// is broken out by the fraction by of the coord's outer radius.
	Explode(x0, y0, x1, y1 float32, by float64) (dx, dy float32)
}

// Metrics are the theme lengths a coord needs in order to place furniture.
// They are passed in rather than read because a coord must not know what a
// theme is.
type Metrics struct {
	// TickLen is how far a major tick mark reaches out of the axis, and
	// MinorTickLen the same for a minor one.
	TickLen, MinorTickLen float32
	// LabelPad is the gap between the end of a full-length tick mark and the
	// label beyond it.
	LabelPad float32
}

// tickLen is how far tick t's mark reaches.
func (m Metrics) tickLen(t scale.Tick) float32 {
	if t.Minor {
		return m.MinorTickLen
	}
	return m.TickLen
}

// labelGap is how far a label sits from the axis: past a full-length tick
// mark, whether or not this tick's own mark is that long, so that a row of
// labels lines up.
func (m Metrics) labelGap() float32 { return m.TickLen + m.LabelPad }

// Cartesian is the identity coord, and the default.
//
// Its Point returns the pair it was given, its Edge is one LineTo and its Area
// is four corners — so a chart that never heard of this package draws exactly
// what it drew before there was one, and the golden files in the repository
// are the proof.
func Cartesian() Coord { return cartesian{} }

// cartesian is an empty struct so that putting one in a Coord interface value
// allocates nothing.
type cartesian struct{}

func (cartesian) Frame(area ir.Rect, x, y scale.Scale) Coord {
	if x != nil {
		x.SetRange(area.Min.X, area.Max.X)
	}
	if y != nil {
		// Y is flipped: larger values are higher on screen.
		y.SetRange(area.Max.Y, area.Min.Y)
	}
	return framedCartesian{area: area}
}

// Extent is empty until [cartesian.Frame] has been called: an unframed coord
// has no rectangle to report. Nothing asks — [github.com/timzifer/refract/geom.Frame.Coords]
// frames the fallback before handing it out — and answering zeros is better
// than answering a rectangle that was never chosen.
func (cartesian) Extent() (x0, x1, y0, y1 float32) { return 0, 0, 0, 0 }

func (cartesian) Point(x, y float32) ir.Point { return ir.Point{X: x, Y: y} }

func (cartesian) Points(dst []ir.Point, xs, ys []float32) []ir.Point {
	for i := range xs {
		dst = append(dst, ir.Point{X: xs[i], Y: ys[i]})
	}
	return dst
}

func (cartesian) Straight() bool { return true }

func (cartesian) Edge(p *ir.Path, _, to ir.Point) { p.LineTo(to.X, to.Y) }

// Area appends the same four corners [ir.Path.Rect] does, in the same order.
// That identity is not decoration: it is why every rect, bar and box in the
// golden files came out unchanged when they were rewritten onto this call.
func (cartesian) Area(p *ir.Path, x0, y0, x1, y1 float32) {
	p.MoveTo(x0, y0).LineTo(x1, y0).LineTo(x1, y1).LineTo(x0, y1).Close()
}

func (cartesian) Clip(p *ir.Path, area ir.Rect) { p.Rect(area) }

func (cartesian) Invert(pt ir.Point) (float32, float32) { return pt.X, pt.Y }

func (cartesian) Decimates() bool { return true }

func (cartesian) Describe() Desc { return Desc{Type: TypeCartesian} }

func (cartesian) Furniture(dst *Furniture, area ir.Rect, m Metrics, xTicks, yTicks []scale.Tick) {
	dst.XLabelsShareARow = true

	// The X axis runs along the bottom edge, its ticks hang below it and its
	// labels sit below those, centred.
	x := dst.x()
	x.axis.line(ir.Point{X: area.Min.X, Y: area.Max.Y}, ir.Point{X: area.Max.X, Y: area.Max.Y})
	for _, t := range xTicks {
		grid, tick := x.next()
		in := inRange(t.Pos, area.Min.X, area.Max.X)
		if !in {
			x.mark(false, Label{})
			continue
		}
		if !t.Minor {
			grid.line(ir.Point{X: t.Pos, Y: area.Min.Y}, ir.Point{X: t.Pos, Y: area.Max.Y})
		}
		if l := m.tickLen(t); l > 0 {
			tick.line(ir.Point{X: t.Pos, Y: area.Max.Y}, ir.Point{X: t.Pos, Y: area.Max.Y + l})
		}
		x.mark(true, Label{
			At: ir.Point{X: t.Pos, Y: area.Max.Y + m.labelGap()},
			H:  ir.AlignCenter,
			V:  ir.AlignTop,
		})
	}

	// The Y axis runs up the left edge, its ticks reach out to the left and
	// its labels are right-aligned against them.
	y := dst.y()
	y.axis.line(ir.Point{X: area.Min.X, Y: area.Min.Y}, ir.Point{X: area.Min.X, Y: area.Max.Y})
	for _, t := range yTicks {
		grid, tick := y.next()
		in := inRange(t.Pos, area.Min.Y, area.Max.Y)
		if !in {
			y.mark(false, Label{})
			continue
		}
		if !t.Minor {
			grid.line(ir.Point{X: area.Min.X, Y: t.Pos}, ir.Point{X: area.Max.X, Y: t.Pos})
		}
		if l := m.tickLen(t); l > 0 {
			tick.line(ir.Point{X: area.Min.X - l, Y: t.Pos}, ir.Point{X: area.Min.X, Y: t.Pos})
		}
		y.mark(true, Label{
			At: ir.Point{X: area.Min.X - m.labelGap(), Y: t.Pos},
			H:  ir.AlignEnd,
			V:  ir.AlignMiddle,
		})
	}
}

// framedCartesian is a Cartesian coord that remembers which rectangle it was
// framed in, which is the one thing [Coord.Extent] needs and the one thing an
// unframed Cartesian cannot answer. Everything else is inherited, because a
// Cartesian coord has no other state and the identity does not depend on where
// it is.
type framedCartesian struct {
	cartesian
	area ir.Rect
}

func (f framedCartesian) Extent() (x0, x1, y0, y1 float32) {
	return f.area.Min.X, f.area.Max.X, f.area.Max.Y, f.area.Min.Y
}

// cullEps is the tolerance for "is this tick inside the panel".
//
// A tick position comes out of a float32 mapping, so one sitting exactly on an
// edge can land a hair outside it. Half a pixel is far below anything visible
// and far above the rounding error.
const cullEps = 0.5

func inRange(v, lo, hi float32) bool {
	if lo > hi {
		lo, hi = hi, lo
	}
	return v >= lo-cullEps && v <= hi+cullEps
}

var (
	_ Coord     = cartesian{}
	_ Coord     = framedCartesian{}
	_ Describer = cartesian{}
	_ Describer = framedCartesian{}
)
