package ir

// PathOp is a single path-construction verb.
type PathOp uint8

// The path verbs. The set is deliberately minimal: every curve a chart needs
// (rounded corners, tension-smoothed lines, arcs) is expressible as cubics,
// and both SVG and gg consume cubics natively.
const (
	OpMoveTo  PathOp = iota // 1 point
	OpLineTo                // 1 point
	OpCubicTo               // 3 points: control 1, control 2, end
	OpClose                 // 0 points
)

// Points reports how many points in Path.Pts the op consumes.
func (op PathOp) Points() int {
	switch op {
	case OpMoveTo, OpLineTo:
		return 1
	case OpCubicTo:
		return 3
	default:
		return 0
	}
}

// Path is a sequence of subpaths built from MoveTo/LineTo/CubicTo/Close.
//
// Ops and Pts are parallel: walking Ops and consuming Op.Points() entries from
// Pts per op reconstructs the path. Splitting them this way keeps a path two
// allocations regardless of length and lets callers reuse both slices via
// Reset.
type Path struct {
	Ops []PathOp
	Pts []Point
}

// MoveTo starts a new subpath at (x, y).
func (p *Path) MoveTo(x, y float32) *Path {
	p.Ops = append(p.Ops, OpMoveTo)
	p.Pts = append(p.Pts, Point{x, y})
	return p
}

// LineTo appends a straight segment to (x, y).
func (p *Path) LineTo(x, y float32) *Path {
	p.Ops = append(p.Ops, OpLineTo)
	p.Pts = append(p.Pts, Point{x, y})
	return p
}

// CubicTo appends a cubic Bézier segment with the given control points.
func (p *Path) CubicTo(c1x, c1y, c2x, c2y, x, y float32) *Path {
	p.Ops = append(p.Ops, OpCubicTo)
	p.Pts = append(p.Pts, Point{c1x, c1y}, Point{c2x, c2y}, Point{x, y})
	return p
}

// Close closes the current subpath.
func (p *Path) Close() *Path {
	p.Ops = append(p.Ops, OpClose)
	return p
}

// Rect appends a closed rectangular subpath.
func (p *Path) Rect(r Rect) *Path {
	return p.MoveTo(r.Min.X, r.Min.Y).
		LineTo(r.Max.X, r.Min.Y).
		LineTo(r.Max.X, r.Max.Y).
		LineTo(r.Min.X, r.Max.Y).
		Close()
}

// Circle appends a closed circular subpath centred on c.
//
// Four cubics, because [OpCubicTo] is the only curve the IR has and ADR 0002
// froze it there on the claim that every curve a chart needs is expressible as
// one. The control points sit kappa·r along the tangents, where kappa is
// 4(√2-1)/3 — the constant that makes the maximum radial error about one part
// in ten thousand, which is a hundredth of a pixel on a circle the width of a
// plot.
//
// A radius of zero or less appends nothing: a mark with no area is a mark that
// is not drawn, which is what a size channel's smallest value asks for.
func (p *Path) Circle(c Point, r float32) *Path {
	if !(r > 0) {
		return p
	}
	k := r * kappa
	return p.MoveTo(c.X, c.Y-r).
		CubicTo(c.X+k, c.Y-r, c.X+r, c.Y-k, c.X+r, c.Y).
		CubicTo(c.X+r, c.Y+k, c.X+k, c.Y+r, c.X, c.Y+r).
		CubicTo(c.X-k, c.Y+r, c.X-r, c.Y+k, c.X-r, c.Y).
		CubicTo(c.X-r, c.Y-k, c.X-k, c.Y-r, c.X, c.Y-r).
		Close()
}

// kappa is 4(√2-1)/3: how far along the tangent a cubic's control point goes
// to approximate a quarter circle.
const kappa = 0.5522847498307936

// Polyline appends pts as one open subpath. It is a no-op for fewer than two
// points.
func (p *Path) Polyline(pts []Point) *Path {
	if len(pts) < 2 {
		return p
	}
	p.MoveTo(pts[0].X, pts[0].Y)
	for _, q := range pts[1:] {
		p.LineTo(q.X, q.Y)
	}
	return p
}

// Grow reserves room for ops verbs and pts points, keeping whatever the path
// already holds.
//
// It exists for the one caller that knows in advance how much it will append: a
// layer emitting a shape per row. Appending a hundred thousand circles into an
// empty path walks the doubling ladder twenty times, and a path held in a pool
// that a garbage collection has emptied walks it again on the next frame — so a
// chart redrawn over a large table pays a cost that is logarithmic in its rows
// rather than none. Reserving once makes it two allocations, and none at all
// once the buffers are warm.
func (p *Path) Grow(ops, pts int) *Path {
	if n := len(p.Ops) + ops; n > cap(p.Ops) {
		next := make([]PathOp, len(p.Ops), n)
		copy(next, p.Ops)
		p.Ops = next
	}
	if n := len(p.Pts) + pts; n > cap(p.Pts) {
		next := make([]Point, len(p.Pts), n)
		copy(next, p.Pts)
		p.Pts = next
	}
	return p
}

// CircleOps and CirclePts are how much [Path.Circle] appends: one MoveTo, four
// CubicTo and a Close, and the thirteen points those consume. They are named so
// that a caller can reserve room for a known number of circles with [Path.Grow]
// rather than rediscovering the arithmetic.
const (
	CircleOps = 6
	CirclePts = 13
)

// Empty reports whether p contains no ops.
func (p *Path) Empty() bool { return len(p.Ops) == 0 }

// Reset clears p while keeping its capacity, so a caller can reuse it across
// frames without reallocating.
func (p *Path) Reset() {
	p.Ops = p.Ops[:0]
	p.Pts = p.Pts[:0]
}

// Bounds returns the control-point bounding box of p. For paths containing
// cubics this is conservative: it bounds the control polygon, not the exact
// curve. That is what layout and clip-culling need, and it is cheap.
func (p *Path) Bounds() Rect {
	if len(p.Pts) == 0 {
		return Rect{}
	}
	b := Rect{p.Pts[0], p.Pts[0]}
	for _, q := range p.Pts[1:] {
		b.Min.X = min(b.Min.X, q.X)
		b.Min.Y = min(b.Min.Y, q.Y)
		b.Max.X = max(b.Max.X, q.X)
		b.Max.Y = max(b.Max.Y, q.Y)
	}
	return b
}

// Walk calls fn for each op with the points that op consumes. The slice passed
// to fn aliases p.Pts and must not be retained.
func (p *Path) Walk(fn func(op PathOp, pts []Point)) {
	i := 0
	for _, op := range p.Ops {
		n := op.Points()
		fn(op, p.Pts[i:i+n])
		i += n
	}
}
