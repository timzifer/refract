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
