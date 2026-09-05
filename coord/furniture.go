package coord

import "github.com/timzifer/refract/ir"

// Shape is one piece of furniture: a straight run of points, or a path when
// the coord bends it.
//
// The two forms are not interchangeable and the difference is deliberate. A
// Cartesian grid line is two points and reaches the backend as a Polyline,
// exactly as it did before there was a coord to ask — which is why every
// golden file in the repository still matches. A polar ring is cubics and
// reaches it as a stroked path.
type Shape struct {
	// Pts is a straight run, empty when the shape is curved.
	Pts []ir.Point
	// Path is a curve, empty when the shape is straight. It is a value rather
	// than a pointer so that its buffers survive [Furniture.Reset].
	Path ir.Path
}

// Empty reports whether s holds nothing to draw.
func (s *Shape) Empty() bool { return len(s.Pts) < 2 && s.Path.Empty() }

// line makes s the straight segment from a to b, reusing its buffer.
func (s *Shape) line(a, b ir.Point) { s.Pts = append(s.Pts[:0], a, b) }

// reset empties s without giving up its memory.
func (s *Shape) reset() {
	s.Pts = s.Pts[:0]
	s.Path.Reset()
}

// Label is where one tick label sits and how it is aligned about that point.
type Label struct {
	At ir.Point
	H  ir.HAlign
	V  ir.VAlign
	// Rotation turns the label about its anchor, in radians clockwise. It is
	// zero for every label a Cartesian axis writes.
	Rotation float64
}

// Furniture is the geometry of one panel's grid lines, axis lines, tick marks
// and tick labels. A coord fills it; render strokes it.
//
// Every per-tick slice is parallel to the tick list it came from, so index i
// is tick i whether or not that tick is drawn. That is what lets render keep
// the decisions that are its own — which grid lines the theme asked for, which
// labels would collide, whether this panel writes labels at all — while the
// coord answers only where things go.
//
// It is filled rather than returned so that a chart redrawn every frame does
// not allocate its furniture again: [Furniture.Reset] keeps every buffer.
type Furniture struct {
	// AxisX and AxisY are the two axis lines.
	AxisX, AxisY Shape

	// GridX is one shape per X tick and GridY one per Y tick, in tick order.
	// A tick with no grid line — a minor one, or one outside the panel — has
	// an empty shape.
	GridX, GridY []Shape

	// TickX and TickY are the tick marks, in tick order, empty where there is
	// none.
	TickX, TickY []Shape

	// LabelX and LabelY are where the tick labels go, in tick order.
	LabelX, LabelY []Label

	// InX and InY report, per tick, whether the tick falls inside the panel at
	// all. A Cartesian coord culls a tick that a float32 mapping put a hair
	// outside the plot rectangle; a polar one has nothing to fall off.
	InX, InY []bool

	// XLabelsShareARow reports whether the X tick labels sit along one
	// horizontal line and can therefore run into each other. render drops the
	// ones that would overlap when they do — and must not when they do not:
	// two labels on opposite sides of a ring can share an x and still be a
	// finger apart.
	XLabelsShareARow bool
}

// Reset empties f while keeping every buffer it has grown, including those of
// the shapes past its current length.
func (f *Furniture) Reset() {
	f.AxisX.reset()
	f.AxisY.reset()
	f.GridX, f.GridY = resetShapes(f.GridX), resetShapes(f.GridY)
	f.TickX, f.TickY = resetShapes(f.TickX), resetShapes(f.TickY)
	f.LabelX, f.LabelY = f.LabelX[:0], f.LabelY[:0]
	f.InX, f.InY = f.InX[:0], f.InY[:0]
	f.XLabelsShareARow = false
}

// resetShapes empties every shape the slice has ever held — not merely the
// ones inside its current length — and truncates it. Reaching past the length
// is the point: [Furniture.nextX] hands those shapes back out on the next
// frame, and a shape still holding last frame's points would draw them again.
func resetShapes(s []Shape) []Shape {
	full := s[:cap(s)]
	for i := range full {
		full[i].reset()
	}
	return s[:0]
}

// side is one axis's worth of furniture. A coord fills X and Y with the same
// loop written once, which is the only reason it exists.
type side struct {
	axis  *Shape
	grid  *[]Shape
	tick  *[]Shape
	label *[]Label
	in    *[]bool
}

func (f *Furniture) x() side {
	return side{&f.AxisX, &f.GridX, &f.TickX, &f.LabelX, &f.InX}
}

func (f *Furniture) y() side {
	return side{&f.AxisY, &f.GridY, &f.TickY, &f.LabelY, &f.InY}
}

// next lengthens the side by one tick and hands back that tick's grid line and
// tick mark to fill in. Both come back with whatever buffers the last frame
// left them, and empty.
func (s side) next() (grid, tick *Shape) {
	*s.grid, *s.tick = growShapes(*s.grid), growShapes(*s.tick)
	return &(*s.grid)[len(*s.grid)-1], &(*s.tick)[len(*s.tick)-1]
}

// mark records whether the tick just added falls inside the panel, and where
// its label goes.
func (s side) mark(in bool, l Label) {
	*s.in = append(*s.in, in)
	*s.label = append(*s.label, l)
}

// growShapes lengthens s by one, reusing the shape that was there before Reset
// truncated the slice rather than appending a zero one over it.
func growShapes(s []Shape) []Shape {
	if len(s) < cap(s) {
		return s[:len(s)+1]
	}
	return append(s, Shape{})
}
