package coord

import (
	"math"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Smith reads a panel's two axes as a complex impedance and places it on the
// unit disc, which is the chart every RF, microwave and antenna engineer works
// on and no general-purpose plotting library draws.
//
// X is the normalised resistance r = R/Z₀ and Y the normalised reactance
// x = X/Z₀. The coord maps the pair through the reflection coefficient
//
//	Γ = (z − 1) / (z + 1),  z = r + jx
//
// which carries the whole right half-plane — every passive impedance there is,
// including the infinite ones — into a disc a finger wide. That is the entire
// trick of the chart: a quantity with no bounds gets a picture with edges.
//
// # Why this is a coord and not a mark
//
// A Smith chart's grid is two families of curves — a circle for each constant
// resistance, an arc for each constant reactance — and under Γ they are exactly
// the images of the two axes' own grid lines. So the grid is not drawn by
// anything: it is what the panel's X and Y ticks look like once the coord has
// had them, in the same way a polar coord turns a Y tick into a ring. Nothing
// in render changes, and no geom knows this chart exists.
//
//	p := refract.New(refract.Coord(coord.Smith()))
//	p.X(scale.Linear(scale.Domain(0, 20), scale.TickValues(0, 0.2, 0.5, 1, 2, 5)))
//	p.Y(scale.Linear(scale.Domain(-20, 20), scale.TickValues(-5, -2, -1, -0.5, -0.2, 0.2, 0.5, 1, 2, 5)))
//	p.Add(geom.Line(sweep, geom.X("r"), geom.Y("x")))
//
// [scale.TickValues] is what asks for the grid a paper chart is printed with;
// an axis left to choose its own ticks draws a perfectly correct Smith chart
// with unfamiliar circles on it. The domains are pinned rather than trained
// because the chart's extent is the whole disc whatever the data does — a
// near-open reflection is an r in the thousands, and an axis that autoscaled to
// it would put every tick in the last pixel before the rim.
//
// # What the interval means
//
// [Frame] gives each scale the range its own domain already is, so a linear
// scale's Map is the identity and the pair reaching [Smith.Point] is the
// impedance itself. That is this coord's answer to the question every coord
// answers — Cartesian says the interval is a distance along an edge, Polar says
// it is an angle and a radius, and Smith says it is the impedance — and it is
// why an axis here wants a linear scale. A log or symlog axis under this coord
// is not broken, it is a different chart, and the coord does not guess which
// one was meant.
//
// A consequence worth knowing: a zoom moves nothing. [scale.Zoomer.SetDomain]
// changes the domain, the next Frame changes the range to match, and every
// point stays where it was. That is right — the disc is always the whole
// picture — but it means a Smith panel is not pannable, only relabelled.
//
// # A measured sweep
//
// A vector network analyser reports S₁₁ as a reflection coefficient rather than
// as an impedance. [SmithZ] is the conversion, so a measured sweep is one line
// at the call site.
//
// # The admittance chart
//
// [SmithAdmittance] turns the disc through half a turn and reads the pair as a
// conductance and a susceptance instead. Everything above still holds with y
// for z and the short circuit for the open; the grid is the same two families,
// mirrored.
func Smith(opts ...SmithOption) Coord {
	s := &smith{radius: defaultRadius}
	for _, o := range opts {
		o(s)
	}
	return s
}

// SmithOption configures a Smith coord. It is named for the coord it configures
// rather than for the package, exactly as [PolarOption] is, so that two coords
// can have options of the same shape without colliding.
type SmithOption func(*smith)

// SmithRadius sets how much of the panel the disc fills, as a fraction of half
// its shorter side. The default leaves room outside the rim for the reactance
// labels that go round it.
func SmithRadius(f float64) SmithOption {
	return func(s *smith) {
		if f > 0 && f <= 1 {
			s.radius = f
		}
	}
}

// SmithAdmittance draws the admittance chart, whose two columns are a
// normalised admittance — conductance g on X and susceptance b on Y — rather
// than an impedance.
//
// It is one sign. y = 1/z gives Γ_y = −Γ_z, so the Y chart is the Z chart
// turned through half a turn, and the grid that bunched towards an open now
// bunches towards a short. Because the turn is applied to the picture and not
// to the data, a load plotted as its admittance here lands on exactly the point
// its impedance lands on over there: the same physical reflection, read against
// the other grid. That is what makes the option useful rather than decorative,
// and it is what a shunt element wants, because a shunt admittance adds where a
// shunt impedance does not.
//
//	y := 1 / z                                     // in the caller's own numbers
//	g, b := coord.SmithZ(-re, -im)                 // or straight from a sweep
func SmithAdmittance(on bool) SmithOption { return func(s *smith) { s.admittance = on } }

// SmithArc draws an edge between two marks as the true image of the straight
// line between their impedances, which is an arc.
//
// It is what a swept component traces: sweeping a series inductance draws the
// constant-resistance circle exactly, and four points along it joined by chords
// draw a quadrilateral instead. Use it for a locus computed from a component
// value; leave the default for a measured frequency sweep, where the samples
// are what is known and the line between two of them is a convention.
func SmithArc() SmithOption { return func(s *smith) { s.arc = true } }

// SmithChord is the default edge policy, spelled out for a chart that wants to
// say so: an edge between two marks is the straight line between them on
// screen, which is what every instrument draws for a measured trace. See
// [SmithArc] for the other one.
func SmithChord() SmithOption { return func(s *smith) { s.arc = false } }

// SmithZ converts a reflection coefficient into the normalised impedance a
// Smith panel plots, which is the inverse of the map the coord applies:
//
//	z = (1 + Γ) / (1 − Γ)
//
// It lives here rather than in a data package because it is the same three
// lines of arithmetic [smith.Invert] already needs, and having one copy is what
// keeps a tooltip and a column agreeing. A Γ of exactly 1 is an open circuit
// and has no finite impedance; both results are +Inf, which every scale treats
// as missing.
//
//	r, x := coord.SmithZ(re, im)    // the impedance chart's pair
//	g, b := coord.SmithZ(-re, -im)  // the admittance chart's, from the same Γ
func SmithZ(re, im float64) (r, x float64) {
	d := (1-re)*(1-re) + im*im
	if d == 0 {
		return math.Inf(1), math.Inf(1)
	}
	return (1 - re*re - im*im) / d, 2 * im / d
}

// smith is the coord itself and — once [smith.Frame] has been called — the disc
// it was inscribed in. Frame returns a copy rather than moving the receiver, so
// two panels drawn on two goroutines never share one centre.
type smith struct {
	radius     float64
	admittance bool
	arc        bool

	// The disc and the domains, set by Frame.
	cx, cy         float32
	rad            float32
	x0, x1, y0, y1 float32
	framed         bool
}

func (s *smith) Frame(area ir.Rect, x, y scale.Scale) Coord {
	q := *s
	q.cx = (area.Min.X + area.Max.X) / 2
	q.cy = (area.Min.Y + area.Max.Y) / 2
	q.rad = float32(math.Min(float64(area.Dx()), float64(area.Dy())) / 2 * q.radius)
	if q.rad < 0 {
		q.rad = 0
	}
	q.x0, q.x1 = identityRange(x)
	q.y0, q.y1 = identityRange(y)
	q.framed = true
	return &q
}

// identityRange gives a scale the range its own domain already is and reports
// what that was. A linear scale's Map is then the identity, which is how the
// impedance itself reaches Point — see [Smith].
func identityRange(sc scale.Scale) (lo, hi float32) {
	if sc == nil {
		return 0, 0
	}
	dlo, dhi := sc.Domain()
	lo, hi = float32(dlo), float32(dhi)
	sc.SetRange(lo, hi)
	return lo, hi
}

func (s *smith) Extent() (x0, x1, y0, y1 float32) { return s.x0, s.x1, s.y0, s.y1 }

// gamma is the reflection coefficient of a normalised impedance. z = −1 is the
// map's pole and has no image; NaN is what every scale and every backend
// already treats as nothing to draw.
func gamma(r, x float64) (re, im float64) {
	d := (r+1)*(r+1) + x*x
	if d == 0 {
		return math.NaN(), math.NaN()
	}
	return (r*r + x*x - 1) / d, 2 * x / d
}

// place puts a point of the Γ plane on the canvas. Γ's imaginary axis points up
// and the canvas's Y axis points down, which is the one sign flip here that is
// not the admittance one.
func (s *smith) place(re, im float64) ir.Point {
	if s.admittance {
		re, im = -re, -im
	}
	r := float64(s.rad)
	return ir.Point{X: s.cx + float32(r*re), Y: s.cy - float32(r*im)}
}

func (s *smith) Point(x, y float32) ir.Point {
	return s.place(gamma(float64(x), float64(y)))
}

func (s *smith) Points(dst []ir.Point, xs, ys []float32) []ir.Point {
	for i := range xs {
		dst = append(dst, s.Point(xs[i], ys[i]))
	}
	return dst
}

func (s *smith) Invert(pt ir.Point) (x, y float32) {
	re, im := s.gammaOf(pt)
	r, ix := SmithZ(re, im)
	return float32(r), float32(ix)
}

// gammaOf reads a device point back as a point of the Γ plane. It is the raw
// geometry rather than [smith.Invert]: an edge is drawn between two points the
// caller already placed, and a construction over them works in Γ.
func (s *smith) gammaOf(pt ir.Point) (re, im float64) {
	if s.rad == 0 {
		return 0, 0
	}
	r := float64(s.rad)
	re, im = float64(pt.X-s.cx)/r, float64(s.cy-pt.Y)/r
	if s.admittance {
		re, im = -re, -im
	}
	return re, im
}

func (s *smith) Straight() bool { return !s.arc }

// Edge appends the image of the straight line between two impedances.
//
// A Möbius map takes a line to a circle, and three points settle which circle:
// the two ends, and the image of the line's own point at infinity — which is
// Γ = 1 for every line, because every line runs out to an impedance of infinite
// magnitude and that is an open circuit. Of the two arcs those ends cut that
// circle into, the edge is the one that does *not* pass through Γ = 1, because
// a segment between two finite impedances does not pass through infinity.
//
// A line through the map's pole at z = −1 has a straight line for its image and
// no circle through the three points; the construction reports that and the
// edge is drawn as the chord it is.
func (s *smith) Edge(p *ir.Path, from, to ir.Point) {
	if !s.arc {
		p.LineTo(to.X, to.Y)
		return
	}
	s.arcBetween(p, from, to)
}

// arcBetween appends the Möbius image of the segment between two device points,
// falling back to a chord where that image is a straight line.
func (s *smith) arcBetween(p *ir.Path, from, to ir.Point) {
	// gammaOf undoes the half turn an admittance chart applies and gammaArc
	// puts it back, so the whole construction runs in the Γ the pair maps to
	// directly — where the image of every line's own point at infinity is 1,
	// whether that infinity is an open circuit or a short one.
	fre, fim := s.gammaOf(from)
	tre, tim := s.gammaOf(to)
	cx, cy, r, ok := circleThrough(fre, fim, tre, tim, 1, 0)
	if !ok {
		p.LineTo(to.X, to.Y)
		return
	}
	start := math.Atan2(fim-cy, fre-cx)
	end := math.Atan2(tim-cy, tre-cx)
	sweep := arcAvoiding(start, end, math.Atan2(-cy, 1-cx))
	s.gammaArc(p, cx, cy, r, start, sweep)
}

// gammaArc appends the arc of a circle given in the Γ plane: centre (cx, cy),
// radius r, starting at angle a0 and turning by sweep, both measured from the
// positive real axis and growing the way Γ's imaginary axis points.
//
// [arcTo] measures from twelve o'clock and turns the other way, which is the
// convention a pie is written in; a = π/2 − θ and a sweep of the opposite sign
// is that conversion, done here rather than by giving the shared helper a
// second convention to be wrong about.
func (s *smith) gammaArc(p *ir.Path, cx, cy, r, a0, sweep float64) {
	c := s.place(cx, cy)
	if s.admittance {
		a0 += math.Pi
	}
	arcTo(p, c.X, c.Y, math.Pi/2-a0, r*float64(s.rad), -sweep, r*float64(s.rad))
}

// circleThrough is the circle through three points, or ok == false when they
// are collinear and there is none.
func circleThrough(ax, ay, bx, by, cx, cy float64) (ox, oy, r float64, ok bool) {
	d := 2 * (ax*(by-cy) + bx*(cy-ay) + cx*(ay-by))
	if d == 0 || math.IsNaN(d) || math.IsInf(d, 0) {
		return 0, 0, 0, false
	}
	a2, b2, c2 := ax*ax+ay*ay, bx*bx+by*by, cx*cx+cy*cy
	ox = (a2*(by-cy) + b2*(cy-ay) + c2*(ay-by)) / d
	oy = (a2*(cx-bx) + b2*(ax-cx) + c2*(bx-ax)) / d
	r = math.Hypot(ax-ox, ay-oy)
	if r == 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		return 0, 0, 0, false
	}
	return ox, oy, r, true
}

// arcAvoiding is the sweep from a0 to a1 that does not pass through avoid. Two
// arcs of a circle join any two points; naming the third one the arc must miss
// is what chooses between them.
func arcAvoiding(a0, a1, avoid float64) float64 {
	ccw := turn(a1 - a0)
	if turn(avoid-a0) < ccw {
		return ccw - 2*math.Pi
	}
	return ccw
}

// turn normalises an angle into [0, 2π).
func turn(a float64) float64 {
	a = math.Mod(a, 2*math.Pi)
	if a < 0 {
		a += 2 * math.Pi
	}
	return a
}

// Area appends the closed image of a data-space rectangle: a tolerance box on a
// Smith chart, whose four sides are the images of four straight lines and are
// therefore four arcs. It is what a [github.com/timzifer/refract/geom.Rect] and
// a Region draw here, and it uses the true image whatever the edge policy is —
// a shape's outline is a claim about the region it encloses, and a chord would
// enclose the wrong one.
func (s *smith) Area(p *ir.Path, x0, y0, x1, y1 float32) {
	corners := [4]ir.Point{
		s.Point(x0, y0), s.Point(x1, y0), s.Point(x1, y1), s.Point(x0, y1),
	}
	p.MoveTo(corners[0].X, corners[0].Y)
	for i := 1; i < len(corners); i++ {
		s.arcBetween(p, corners[i-1], corners[i])
	}
	s.arcBetween(p, corners[3], corners[0])
	p.Close()
}

// Clip is the disc the panel's data lives in. Every passive impedance is inside
// it by construction, so this clips off the active ones — a reflection of more
// than unity — rather than clipping data the chart meant to show.
func (s *smith) Clip(p *ir.Path, area ir.Rect) {
	if !s.framed {
		p.Rect(area)
		return
	}
	p.Circle(ir.Point{X: s.cx, Y: s.cy}, s.rad)
}

// Decimates reports false: see [Coord.Decimates]. A bucket of equal width on
// screen is not a bucket of equal anything in impedance here — the map crowds
// the whole of the far half-plane into the last few pixels before the rim — so
// a reduction defined over pixel columns would be measuring something other
// than what it was designed to measure. Nothing on a Smith chart is a big-data
// chart; a sweep is a few thousand points at most.
func (s *smith) Decimates() bool { return false }

func (s *smith) Furniture(dst *Furniture, area ir.Rect, m Metrics, xTicks, yTicks []scale.Tick) {
	// The resistance labels really do sit along one horizontal line — the real
	// axis — so the greedy overlap filter that keeps a dense axis readable
	// should run over them. This is the one place a Smith panel differs from a
	// polar one, which sets it false.
	dst.XLabelsShareARow = true

	s.resistance(dst.x(), xTicks, m)
	s.reactance(dst.y(), yTicks, m)
}

// resistance fills the furniture of the axis that reads as R: a circle per
// tick, all of them internally tangent at Γ = 1, the real axis as the axis
// line, and the labels written along it where each circle crosses it — which is
// where a paper Smith chart prints its numbers.
func (s *smith) resistance(side side, ticks []scale.Tick, m Metrics) {
	side.axis.line(s.place(-1, 0), s.place(1, 0))
	for _, t := range ticks {
		grid, tick := side.next()
		r := float64(t.Pos)
		if r < 0 || math.IsNaN(r) || math.IsInf(r, 0) {
			// A negative resistance is outside the disc: the chart has no
			// place for it, which is what InX reports.
			side.mark(false, Label{})
			continue
		}
		// The circle of constant r is centred at r/(1+r) with radius 1/(1+r).
		// At r = 0 that is the rim itself, which the reactance axis already
		// draws — one circle, not two coats of ink on it.
		if !t.Minor && r > 0 {
			s.circle(&grid.Path, r/(1+r), 0, 1/(1+r))
		}
		// The circle meets the real axis at Γ = (r−1)/(r+1), its leftmost
		// point, and the label hangs below that.
		at := s.place((r-1)/(r+1), 0)
		if l := m.tickLen(t); l > 0 {
			tick.line(at, ir.Point{X: at.X, Y: at.Y + l})
		}
		side.mark(true, Label{
			At: ir.Point{X: at.X, Y: at.Y + m.labelGap()},
			H:  ir.AlignCenter,
			V:  ir.AlignTop,
		})
	}
}

// reactance fills the furniture of the axis that reads as X: an arc per tick
// running from Γ = 1 round to the rim, the rim itself as the axis line, and a
// label outside the rim where the arc lands.
func (s *smith) reactance(side side, ticks []scale.Tick, m Metrics) {
	s.circle(&side.axis.Path, 0, 0, 1)
	for _, t := range ticks {
		grid, tick := side.next()
		x := float64(t.Pos)
		if math.Abs(x) < reactanceEps || math.IsNaN(x) || math.IsInf(x, 0) {
			// A reactance of zero is the real axis, which the resistance axis
			// already draws, and its label would land on top of the one r = 0
			// writes at the same point of the rim.
			side.mark(false, Label{})
			continue
		}
		// The arc of constant x is centred at (1, 1/x) with radius 1/|x|, and
		// runs from Γ = 1 — where every one of them meets, because a reactance
		// of any size is still an open circuit once the resistance is infinite
		// — out to the rim.
		re, im := gamma(0, x)
		if !t.Minor {
			s.reactanceArc(&grid.Path, x, re, im)
		}
		s.rimMark(side, tick, m, t, re, im)
	}
}

// rimMark places one reactance tick's mark and label just outside the rim,
// aligned away from the disc so that neither runs back over the chart.
func (s *smith) rimMark(side side, tick *Shape, m Metrics, t scale.Tick, re, im float64) {
	at := s.place(re, im)
	dx, dy := float64(at.X-s.cx), float64(at.Y-s.cy)
	if d := math.Hypot(dx, dy); d > 0 {
		dx, dy = dx/d, dy/d
	}
	if l := m.tickLen(t); l > 0 {
		tick.line(at, ir.Point{X: at.X + float32(dx)*l, Y: at.Y + float32(dy)*l})
	}
	gap := m.labelGap()
	// radialAlign reads an angle in the pie convention: zero at twelve o'clock,
	// growing clockwise, so that the point at angle a is (sin a, −cos a).
	h, v := radialAlign(math.Atan2(dx, -dy))
	side.mark(true, Label{
		At: ir.Point{X: at.X + float32(dx)*gap, Y: at.Y + float32(dy)*gap},
		H:  h,
		V:  v,
	})
}

// reactanceEps is how near zero a reactance has to be before it is read as the
// real axis. The arc's radius is 1/|x|, so a reactance any smaller than this
// asks for a circle wider than the whole float32 canvas to draw a curve nobody
// could tell from the diameter.
const reactanceEps = 1e-6

// circle appends a whole circle of the Γ plane. It is [ir.Path.Circle] — the
// four cubics every round mark in the library is made of, at the exact kappa —
// rather than [arcTo] closed on itself, because a whole circle is the case that
// construction was frozen for. The two agree to about three ulp, which is the
// tolerance ADR 0018 already records for them.
func (s *smith) circle(p *ir.Path, cx, cy, r float64) {
	p.Circle(s.place(cx, cy), float32(r*float64(s.rad)))
}

// reactanceArc appends the constant-reactance arc through the rim point
// (re, im), which is the part of the circle centred at (1, 1/x) that lies
// inside the disc.
//
// Which part that is cannot be found by taking the shorter way round. The arc
// grows from nothing at a reactance of zero to very nearly a half turn as the
// reactance grows without bound, so the two candidates come arbitrarily close
// to equal and a comparison against a half turn decides an arc the wrong way
// on the strength of a rounding error. The sign of the reactance decides it
// instead, and decides it exactly: an inductive arc always turns one way out of
// the open circuit and a capacitive one always turns the other.
func (s *smith) reactanceArc(p *ir.Path, x, re, im float64) {
	cy := 1 / x
	r := math.Abs(cy)
	// Γ = 1 lies directly below the centre for a positive reactance and
	// directly above it for a negative one, so the arc leaves it at a quarter
	// turn either side of straight up.
	from := math.Atan2(-cy, 0)
	sweep := turn(math.Atan2(im-cy, re-1) - from)
	if x > 0 {
		sweep -= 2 * math.Pi
	}
	open := s.place(1, 0)
	p.MoveTo(open.X, open.Y)
	s.gammaArc(p, 1, cy, r, from, sweep)
}

func (s *smith) Describe() Desc {
	return Desc{Type: TypeSmith, Radius: s.radius, Admittance: s.admittance, Arc: s.arc}
}

var (
	_ Coord     = (*smith)(nil)
	_ Describer = (*smith)(nil)
)
