package coord

import (
	"math"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Polar wraps one axis around a circle and reads the other as a radius.
//
// It is what turns the marks that already exist into the family of charts that
// did not: a pie and a donut are a stacked [github.com/timzifer/refract/geom.Bar]
// with θ from the Y axis, a rose is the same bar with θ from an ordinal X, a
// radar is a [github.com/timzifer/refract/geom.Line] over one, and a gauge is a
// bar over a partial sweep. None of them is a new geom, which is the whole
// point of the stage.
//
// # Which axis is the angle
//
// By default X sweeps the circle and Y is the radius, which is what a rose,
// a wind rose and a radar want: the category or the direction goes round and
// the magnitude goes out. [Theta] swaps them, which is what a pie wants: the
// value goes round and the single slot goes out.
//
// # A pie
//
//	p := refract.New(refract.Coord(coord.Polar(coord.Theta(coord.FromY))))
//	p.X(scale.Linear())   // one slot, filling the radius
//	p.Y(scale.Linear())   // the stacked total, filling the circle
//	p.Add(geom.Bar(src, geom.X("one"), geom.Y("share"), geom.GroupBy("browser")))
//
// The ring closes into a full circle because a stacked Y domain ends at the
// total and starts at zero — so the angular scale must not be niced. A niced
// domain rounds the total up and leaves a wedge of nothing at twelve o'clock,
// which is why the recipe above spells [scale.Linear] without
// [scale.Nice]. [Hole] turns the pie into a donut.
//
// # Edges
//
// An edge between two marks is an arc by default, because that is the edge
// that is straight in data space and it is what a rose petal and a polar band
// need. A radar is the exception — its sides are chords, and drawing them as
// arcs bows them outwards — so a radar asks for [Chord].
func Polar(opts ...Option) Coord {
	p := &polar{
		theta:  FromX,
		sweep:  2 * math.Pi,
		radius: defaultRadius,
		edge:   arcEdges,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Option configures a coord.
type Option func(*polar)

// Axis names one of a panel's two scales.
type Axis uint8

// The axes, as [Theta] reads them.
const (
	// FromX sweeps the X scale around the circle and reads Y as the radius.
	FromX Axis = iota
	// FromY sweeps the Y scale around the circle and reads X as the radius.
	FromY
)

// Theta chooses which scale sweeps the circle. The default is [FromX].
func Theta(a Axis) Option { return func(p *polar) { p.theta = a } }

// Hole leaves the middle of the circle empty, as a fraction of the outer
// radius in [0, 1). It is what makes a donut out of a pie and a ring gauge out
// of a gauge, and it is an explicit annulus rather than a white circle painted
// over the middle: the hole is where the radial scale starts, so a mark never
// enters it and a pointer in it hits nothing.
func Hole(f float64) Option {
	return func(p *polar) {
		if f > 0 && f < 1 {
			p.hole = f
		}
	}
}

// Radius sets how much of the panel the circle fills, as a fraction of half
// its shorter side. The default leaves room outside the ring for the tick
// labels that go round it; a chart with none — a pie usually has none — can
// ask for the whole of it.
func Radius(f float64) Option {
	return func(p *polar) {
		if f > 0 && f <= 1 {
			p.radius = f
		}
	}
}

// Start turns the whole coord about its centre, in radians clockwise from
// twelve o'clock. The default is zero: the first slice of a pie and the first
// axis of a radar both begin straight up, which is where a reader looks first.
func Start(radians float64) Option { return func(p *polar) { p.start = radians } }

// Sweep sets how much of the circle the angular scale covers, in radians. The
// default is a full turn; a half turn is a gauge.
func Sweep(radians float64) Option {
	return func(p *polar) {
		if radians != 0 {
			p.sweep = radians
		}
	}
}

// Counterclockwise runs the angular scale the other way round. The default is
// clockwise, which is the direction a pie, a clock and a compass all read in.
func Counterclockwise(on bool) Option {
	return func(p *polar) {
		if on {
			p.ccw = true
		}
	}
}

// Chord draws an edge between two marks as the straight line between them
// rather than as the arc through data space. It is what a radar wants: the
// sides of a spider chart are chords, and an arc between two axes bows the
// outline outwards into something the data does not say.
func Chord() Option { return func(p *polar) { p.edge = chordEdges } }

// Arc is the default edge policy, spelled out for a chart that wants to say
// so. See [Chord] for the other one.
func Arc() Option { return func(p *polar) { p.edge = arcEdges } }

// defaultRadius leaves a tenth of the panel's shorter half-side outside the
// ring, which is about what a row of tick labels needs. [Radius] is how a
// chart with no labels round the outside takes it back.
const defaultRadius = 0.9

// The edge policies.
const (
	arcEdges = iota
	chordEdges
)

// polar is the coord itself, and — once [polar.Frame] has been called — the
// circle it was inscribed in.
//
// Frame returns a copy rather than moving the receiver, so the chart's own
// polar coord is never written to and two panels drawn on two goroutines
// cannot fight over one centre.
type polar struct {
	theta  Axis
	hole   float64
	radius float64
	start  float64
	sweep  float64
	ccw    bool
	edge   int

	// The circle, set by Frame.
	cx, cy float32
	r0, r1 float32
	framed bool
}

func (p *polar) Frame(area ir.Rect, x, y scale.Scale) Coord {
	q := *p
	q.cx = (area.Min.X + area.Max.X) / 2
	q.cy = (area.Min.Y + area.Max.Y) / 2
	outer := float32(math.Min(float64(area.Dx()), float64(area.Dy())) / 2 * q.radius)
	if outer < 0 {
		outer = 0
	}
	q.r0, q.r1 = outer*float32(q.hole), outer
	q.framed = true

	ang, rad := x, y
	if q.theta == FromY {
		ang, rad = y, x
	}
	if ang != nil {
		// A categorical angular axis is a ring of slots rather than a row of
		// them, and the difference is half a slot. Mapping the whole ordinal
		// domain onto the sweep would put the *edge* of the first category at
		// twelve o'clock and its centre a half-slot past it, so a radar's first
		// axis and a wind rose's north would both sit off the vertical. The
		// range is shifted back by half a slot instead, which puts category
		// zero at the start angle and the rest evenly round from there.
		//
		// A continuous angular scale is left alone: it has no slots, and where
		// its domain begins is where the reader asked it to begin.
		off := float32(0)
		if _, band := ang.(scale.Band); band {
			if lo, hi := ang.Domain(); hi > lo {
				off = float32(q.sweep / (2 * (hi - lo)))
			}
		}
		ang.SetRange(-off, float32(q.sweep)-off)
	}
	if rad != nil {
		rad.SetRange(q.r0, q.r1)
	}
	return &q
}

func (p *polar) Extent() (x0, x1, y0, y1 float32) {
	if p.theta == FromY {
		return p.r0, p.r1, 0, float32(p.sweep)
	}
	return 0, float32(p.sweep), p.r0, p.r1
}

// split reads a mapped pair as an angle and a radius, whichever way round this
// coord holds them.
func (p *polar) split(x, y float32) (theta, r float32) {
	if p.theta == FromY {
		return y, x
	}
	return x, y
}

// join is [polar.split] backwards: an angle and a radius written back into the
// pair the scales speak in.
func (p *polar) join(theta, r float32) (x, y float32) {
	if p.theta == FromY {
		return r, theta
	}
	return theta, r
}

// at places an angle and a radius on the canvas. Angle zero is twelve o'clock
// and grows clockwise, which is what [Start] and [Counterclockwise] move.
func (p *polar) at(theta, r float32) ir.Point {
	s, c := math.Sincos(p.angle(float64(theta)))
	return ir.Point{X: p.cx + r*float32(s), Y: p.cy - r*float32(c)}
}

// angle turns a mapped angular position into the canvas angle it draws at.
func (p *polar) angle(theta float64) float64 {
	if p.ccw {
		return p.start - theta
	}
	return p.start + theta
}

func (p *polar) Point(x, y float32) ir.Point {
	theta, r := p.split(x, y)
	return p.at(theta, r)
}

func (p *polar) Points(dst []ir.Point, xs, ys []float32) []ir.Point {
	for i := range xs {
		dst = append(dst, p.Point(xs[i], ys[i]))
	}
	return dst
}

func (p *polar) Straight() bool { return p.edge == chordEdges }

func (p *polar) Edge(path *ir.Path, from, to ir.Point) {
	if p.edge == chordEdges {
		path.LineTo(to.X, to.Y)
		return
	}
	a0, r0 := p.polarOf(from)
	a1, r1 := p.polarOf(to)
	p.spiral(path, a0, r0, shortWay(a0, a1), r1)
}

// polarOf reads a device point back as a canvas angle and a radius. It is the
// raw geometry rather than [polar.Invert]: an edge is drawn between two points
// the caller already placed, and asking the scales about them again would
// round twice.
func (p *polar) polarOf(pt ir.Point) (angle float64, r float64) {
	dx, dy := float64(pt.X-p.cx), float64(p.cy-pt.Y)
	return math.Atan2(dx, dy), math.Hypot(dx, dy)
}

// shortWay is the signed sweep from a0 to a1 that does not go the long way
// round. Two adjacent marks of a series are adjacent on the circle; an edge
// that took the other 340 degrees would draw a ring instead of a segment.
func shortWay(a0, a1 float64) float64 {
	d := math.Mod(a1-a0, 2*math.Pi)
	switch {
	case d > math.Pi:
		d -= 2 * math.Pi
	case d < -math.Pi:
		d += 2 * math.Pi
	}
	return d
}

func (p *polar) Area(path *ir.Path, x0, y0, x1, y1 float32) {
	t0, ra := p.split(x0, y0)
	t1, rb := p.split(x1, y1)
	a0, a1 := p.angle(float64(t0)), p.angle(float64(t1))
	p.sector(path, a0, a1-a0, float64(ra), float64(rb))
}

// sector appends one annular sector: the outer arc, the far radial edge, the
// inner arc back, and the near radial edge closed.
//
// A sector whose inner radius is zero is a wedge, and its inner arc collapses
// to the centre — a pie slice is that case, and drawing the degenerate arc
// would leave a hairline of stroke at the point where every slice meets.
func (p *polar) sector(path *ir.Path, a0, sweep, ra, rb float64) {
	if ra > rb {
		ra, rb = rb, ra
	}
	if rb <= 0 || sweep == 0 {
		return
	}
	// A sweep of a whole turn or more is the full ring. Clamping it keeps the
	// path closed on itself instead of overlapping, which the non-zero fill
	// rule would then read as a hole.
	if sweep > 2*math.Pi {
		sweep = 2 * math.Pi
	} else if sweep < -2*math.Pi {
		sweep = -2 * math.Pi
	}
	a1 := a0 + sweep

	start := p.onCircle(a0, rb)
	path.MoveTo(start.X, start.Y)
	p.spiral(path, a0, rb, sweep, rb)
	if ra <= 0 {
		path.LineTo(p.cx, p.cy)
		path.Close()
		return
	}
	inner := p.onCircle(a1, ra)
	path.LineTo(inner.X, inner.Y)
	p.spiral(path, a1, ra, -sweep, ra)
	path.Close()
}

// onCircle is [polar.at] for an angle that is already a canvas angle.
func (p *polar) onCircle(angle float64, r float64) ir.Point {
	s, c := math.Sincos(angle)
	return ir.Point{X: p.cx + float32(r*s), Y: p.cy - float32(r*c)}
}

// spiral appends the cubics of a sweep from (a0, r0) to (a0+sweep, r1),
// continuing from the path's current point.
//
// A sweep at a constant radius is a circular arc, and this is the exact
// construction for one: the control points sit a distance k = (4/3)·tan(φ/4)
// times the radius along the tangent, which at a quarter turn is the
// kappa in internal/markers. Longer sweeps are cut into quarter turns or less,
// because that identity degrades badly past one. A sweep whose two radii
// differ is the same construction with each end scaled by its own radius,
// which is the honest reading of an edge that is straight in data space when
// the radius is part of the data.
func (p *polar) spiral(path *ir.Path, a0, r0, sweep, r1 float64) {
	if sweep == 0 {
		end := p.onCircle(a0, r1)
		path.LineTo(end.X, end.Y)
		return
	}
	n := int(math.Ceil(math.Abs(sweep)/(math.Pi/2) - 1e-9))
	if n < 1 {
		n = 1
	}
	phi := sweep / float64(n)
	k := 4.0 / 3.0 * math.Tan(phi/4)
	for i := range n {
		t0 := float64(i) / float64(n)
		t1 := float64(i+1) / float64(n)
		b0, b1 := a0+sweep*t0, a0+sweep*t1
		ra := r0 + (r1-r0)*t0
		rb := r0 + (r1-r0)*t1
		s0, c0 := math.Sincos(b0)
		s1, c1 := math.Sincos(b1)
		// The point at angle b is (cx + r·sin b, cy − r·cos b), so the tangent
		// in the direction of increasing b is (cos b, sin b).
		p0 := ir.Point{X: p.cx + float32(ra*s0), Y: p.cy - float32(ra*c0)}
		p3 := ir.Point{X: p.cx + float32(rb*s1), Y: p.cy - float32(rb*c1)}
		c1x := p0.X + float32(k*ra*c0)
		c1y := p0.Y + float32(k*ra*s0)
		c2x := p3.X - float32(k*rb*c1)
		c2y := p3.Y - float32(k*rb*s1)
		path.CubicTo(c1x, c1y, c2x, c2y, p3.X, p3.Y)
	}
}

// Clip is the disc the panel's data lives in. The hole is not cut out of it:
// a clip is what keeps a mark inside the panel, and no mark reaches the middle
// anyway because the radial scale starts at the inner radius.
func (p *polar) Clip(path *ir.Path, area ir.Rect) {
	if !p.framed {
		path.Rect(area)
		return
	}
	start := p.onCircle(0, float64(p.r1))
	path.MoveTo(start.X, start.Y)
	p.spiral(path, 0, float64(p.r1), 2*math.Pi, float64(p.r1))
	path.Close()
}

func (p *polar) Invert(pt ir.Point) (x, y float32) {
	angle, r := p.polarOf(pt)
	theta := angle - p.start
	if p.ccw {
		theta = p.start - angle
	}
	// The scales run over one sweep, so a hit past the seam belongs at the
	// near end of it rather than at a negative angle nothing was drawn at.
	if p.sweep > 0 {
		theta = math.Mod(theta, 2*math.Pi)
		if theta < 0 {
			theta += 2 * math.Pi
		}
	}
	return p.join(float32(theta), float32(r))
}

// Decimates reports false: see [Coord.Decimates]. A bucket of equal angle is
// not a bucket of equal width, so a reduction defined over pixel columns would
// be measuring something other than what it was designed to measure — and
// nothing polar is a big-data chart, so nothing is lost by saying so.
func (p *polar) Decimates() bool { return false }

func (p *polar) Furniture(dst *Furniture, area ir.Rect, m Metrics, xTicks, yTicks []scale.Tick) {
	// Labels round a ring do not share a row, so the greedy overlap filter
	// that keeps a dense Cartesian axis readable must not run over them.
	dst.XLabelsShareARow = false

	if p.theta == FromY {
		p.radial(dst.x(), xTicks, m)
		p.angular(dst.y(), yTicks, m)
		return
	}
	p.angular(dst.x(), xTicks, m)
	p.radial(dst.y(), yTicks, m)
}

// angular fills the furniture of the axis that goes round: a spoke per tick
// from the hole to the rim, the rim itself as the axis line, and a label
// outside the rim at each tick's own angle.
func (p *polar) angular(s side, ticks []scale.Tick, m Metrics) {
	p.ring(s.axis, float64(p.r1))
	for _, t := range ticks {
		grid, tick := s.next()
		a := p.angle(float64(t.Pos))
		if !t.Minor {
			grid.line(p.onCircle(a, float64(p.r0)), p.onCircle(a, float64(p.r1)))
		}
		l := m.tickLen(t)
		if l > 0 {
			tick.line(p.onCircle(a, float64(p.r1)), p.onCircle(a, float64(p.r1)+float64(l)))
		}
		at := p.onCircle(a, float64(p.r1)+float64(m.labelGap()))
		h, v := radialAlign(a)
		// Every angle is inside the circle: a polar axis has no outside for a
		// tick to fall off, which is what [Furniture.InX] is otherwise for.
		s.mark(true, Label{At: at, H: h, V: v})
	}
}

// radial fills the furniture of the axis that goes out: a concentric ring per
// tick instead of a horizontal grid line, the starting spoke as the axis line,
// and the labels written along that spoke.
func (p *polar) radial(s side, ticks []scale.Tick, m Metrics) {
	s.axis.line(p.onCircle(p.start, float64(p.r0)), p.onCircle(p.start, float64(p.r1)))
	for _, t := range ticks {
		grid, tick := s.next()
		r := float64(t.Pos)
		in := inRange(t.Pos, p.r0, p.r1)
		if !in {
			s.mark(false, Label{})
			continue
		}
		if !t.Minor {
			p.ring(grid, r)
		}
		if l := m.tickLen(t); l > 0 {
			// A radial tick mark reaches across its own ring rather than out
			// of the panel: there is no edge here to hang it off.
			tick.line(p.onCircle(p.start, r), p.onCircle(p.start, r+float64(l)))
		}
		at := p.onCircle(p.start, r)
		s.mark(true, Label{
			At: ir.Point{X: at.X + m.LabelPad, Y: at.Y},
			H:  ir.AlignStart,
			V:  ir.AlignMiddle,
		})
	}
}

// ring makes s a full circle of radius r, as four cubics.
func (p *polar) ring(s *Shape, r float64) {
	if r <= 0 {
		return
	}
	start := p.onCircle(0, r)
	s.Path.MoveTo(start.X, start.Y)
	p.spiral(&s.Path, 0, r, 2*math.Pi, r)
	s.Path.Close()
}

// radialAlign is how a label outside the rim sits about its anchor: away from
// the circle, whichever way that is. A label at three o'clock starts at its
// anchor and a label at nine o'clock ends there, so neither runs back over the
// chart.
func radialAlign(angle float64) (ir.HAlign, ir.VAlign) {
	s, c := math.Sincos(angle)
	h := ir.AlignCenter
	switch {
	case s > alignEps:
		h = ir.AlignStart
	case s < -alignEps:
		h = ir.AlignEnd
	}
	v := ir.AlignMiddle
	switch {
	case c > alignEps:
		v = ir.AlignBottom
	case c < -alignEps:
		v = ir.AlignTop
	}
	return h, v
}

// alignEps is how far off the vertical or the horizontal a label has to be
// before it stops being centred on that axis. A label a degree past twelve
// o'clock reads as a label at twelve o'clock, and switching its alignment
// there would make two neighbouring ticks jump apart.
const alignEps = 0.05

var (
	_ Coord     = (*polar)(nil)
	_ Describer = (*polar)(nil)
)
