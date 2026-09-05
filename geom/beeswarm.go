package geom

import (
	"math"
	"slices"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Beeswarm draws one marker per row and moves it aside until it stops
// overlapping its neighbours: every observation of a distribution, none of them
// hidden.
//
// It is the distribution plot that shows the rows themselves. A boxplot
// summarises and a violin smooths; both answer "what does this look like" and
// neither answers "how many are there, and where exactly". A swarm answers both
// — and it is honest about small samples in the way a density estimate is not,
// because a group of six observations draws six marks rather than a curve.
//
// The X column is the grouping key, exactly as it is for [Boxplot] and
// [Violin], so a swarm wants many rows per X value and usually a
// [scale.Ordinal] axis. Given [GroupBy] it draws one swarm per series within
// each slot, side by side.
//
// # Why the offsets are computed when it draws
//
// A mark is moved aside until it clears its neighbours *by a marker's width*,
// which is a length in device units — so the arrangement depends on how wide
// the panel is and on how large the markers are, and it therefore belongs in
// Build. That is the same line ADR 0011 draws for decimation, and it has the
// same consequence: what the axis says does not change when the swarm
// rearranges, because the axis was trained on the observations.
//
// A swarm that will not fit its slot is packed as tightly as the slot allows
// rather than spilling into the one beside it: a mark in the wrong slot is a
// mark attributed to the wrong category, which is worse than two marks touching.
func Beeswarm(src data.Source, opts ...Option) Geom {
	return &swarmGeom{src: src, cfg: newConfig(opts)}
}

type swarmGeom struct {
	src   data.Source
	cfg   config
	s     series
	gs    groups
	slots []float64
	at    []float64
	gap   float64
	gaps  []float64
	err   error
}

func (g *swarmGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	trainColumn(x, g.s.x)
	trainColumn(y, g.s.y)
	g.cfg.trainColors(g.s)
	if g.err = g.gs.train(g.src, g.s, g.cfg, x, y, NoStack); g.err != nil {
		return g.err
	}
	g.slots, g.at = distinctPositions(g.at, g.s.x, g.s.plottable(x, y))
	g.gap, g.gaps = smallestGap(g.gaps, g.slots)

	if _, band := x.(scale.Band); band || len(g.slots) == 0 {
		return nil
	}
	// A swarm has width, so the outermost ones would be clipped in half by a
	// domain that stops at the data.
	half := g.slot() / 2 * g.widthFraction()
	if half > 0 {
		x.Train(g.slots[0]-half, g.slots[len(g.slots)-1]+half)
	}
	return nil
}

func (g *swarmGeom) widthFraction() float64 {
	if g.cfg.barWidth <= 0 || g.cfg.barWidth > 1 {
		return 0.8
	}
	return g.cfg.barWidth
}

// slot is the spacing between adjacent swarms, measured once per Train — see
// [violinGeom.slot] for why it is not measured while drawing.
func (g *swarmGeom) slot() float64 { return g.gap }

func (g *swarmGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	sc := acquire(f)
	defer sc.release()

	diameter := pick(g.cfg.size, f.Theme.MarkerSize)
	half := g.slot() * g.widthFraction() / 2
	cd := f.Coords()
	ok := sc.plottable(g.s, f.X, f.Y)

	// The rows of every swarm are collected first, so the layer emits one
	// drawing call per colour rather than one per slot.
	sc.kx, sc.ky = grow(sc.kx, len(g.s.x))[:0], grow(sc.ky, len(g.s.x))[:0]
	rows := sc.rows[:0]

	for _, pos := range g.slots {
		for _, grp := range g.seriesOf() {
			x0, x1 := markSpan(f, pos, half)
			if g.gs.grouped() {
				x0, x1 = dodgeSpan(x0, x1, g.gs.rank[grp], g.gs.count(), g.cfg.dodgePad)
			}
			sc.kx, sc.ky, rows = g.swarm(sc, f, ok, pos, grp, (x0+x1)/2, (x1-x0)/2, diameter, sc.kx, sc.ky, rows)
		}
	}
	sc.rows = rows
	pts := cd.Points(grow(sc.pts, len(rows))[:0], sc.kx, sc.ky)
	sc.pts = pts
	if len(pts) == 0 {
		return nil
	}
	f.Marks(pts, sc.sourceRows(g.s, rows))

	style := ir.MarkerStyle{Size: diameter}
	if g.gs.grouped() {
		// One call per series, so a swarm of several groups is a handful of
		// calls rather than one per mark.
		for _, run := range sc.groupByColorAt(g.seriesColors(sc, f, rows)) {
			if run.color.A == 0 {
				continue
			}
			st := style
			st.Fill = run.color
			g.emit(b, sc, pts, run.idx, g.cfg.groupMarker(f, g.gs.groupOf(rows[run.idx[0]])), st)
		}
		return nil
	}
	if cols := sc.colorsFor(g.cfg, g.s, rows); cols != nil {
		for _, run := range sc.groupByColorAt(cols) {
			if run.color.A == 0 {
				continue
			}
			st := style
			st.Fill = run.color
			g.emit(b, sc, pts, run.idx, g.cfg.markerFor(f), st)
		}
		return nil
	}
	style.Fill = g.cfg.colorFor(f)
	b.Markers(g.cfg.markerFor(f), pts, style)
	return nil
}

// emit draws the marks named by idx, gathered into a contiguous run.
func (g *swarmGeom) emit(b ir.Backend, sc *scratch, pts []ir.Point, idx []int, marker ir.Marker, style ir.MarkerStyle) {
	sc.edge = grow(sc.edge, len(idx))[:0]
	for _, i := range idx {
		sc.edge = append(sc.edge, pts[i])
	}
	b.Markers(marker, sc.edge, style)
}

// seriesColors resolves the colour of each collected mark from its series.
func (g *swarmGeom) seriesColors(sc *scratch, f Frame, rows []int) []ir.Color {
	sc.cols = grow(sc.cols, len(rows))
	for i, r := range rows {
		sc.cols[i] = g.cfg.groupColor(f, &g.gs, g.gs.groupOf(r))
	}
	return sc.cols[:len(rows)]
}

func (g *swarmGeom) seriesOf() []int {
	if !g.gs.grouped() {
		return oneSeries
	}
	return g.gs.order
}

// oneSeries is the group list of a layer that has none. It is a package
// variable rather than a literal so that the ungrouped path allocates nothing.
var oneSeries = []int{0}

// swarm places one slot's marks and appends them to the mapped columns.
//
// The algorithm is the classic one and it is deliberately deterministic: sort
// the values, then place each mark at the offset nearest the centre line that
// clears every mark already placed within a diameter of it on the value axis.
// Nothing here consults a random number — a swarm that jittered would draw a
// different picture every frame, and a parallel render would stop being
// identical to a serial one.
func (g *swarmGeom) swarm(sc *scratch, f Frame, ok []bool, pos float64, grp int, mid, reach, diameter float32, xs, ys []float32, rows []int) ([]float32, []float32, []int) {
	// The rows of this slot and this series, sorted by value so that
	// neighbours on the axis are neighbours in the scan.
	sc.keep = sc.keep[:0]
	for _, i := range g.rowsOfSeries(sc, grp) {
		if ok[i] && g.s.x[i] == pos {
			sc.keep = append(sc.keep, i)
		}
	}
	if len(sc.keep) == 0 {
		return xs, ys, rows
	}
	vals := g.s.y
	slices.SortStableFunc(sc.keep, func(a, b int) int {
		switch {
		case vals[a] < vals[b]:
			return -1
		case vals[a] > vals[b]:
			return 1
		}
		return 0
	})

	start := len(rows)
	for _, i := range sc.keep {
		y := f.Y.Map(vals[i])
		xs = append(xs, mid+g.offsetFor(xs[start:], ys[start:], mid, reach, y, diameter))
		ys = append(ys, y)
		rows = append(rows, i)
	}
	return xs, ys, rows
}

// rowsOfSeries lists the rows of one series of this layer.
func (g *swarmGeom) rowsOfSeries(sc *scratch, grp int) []int {
	if g.gs.grouped() {
		return g.gs.rows[grp]
	}
	return sc.marksIn(len(g.s.x))
}

// offsetFor is how far aside a mark at device position y has to sit to clear
// the marks already placed in this swarm.
//
// Candidates are tried outwards from the centre in half-diameter steps,
// alternating sides, and the first one that clears everything wins — so a swarm
// grows symmetrically about its slot and the densest part of the distribution
// is the widest. The search stops at the slot's own edge: a mark that will not
// fit is placed at the edge rather than in the neighbouring category.
func (g *swarmGeom) offsetFor(xs, ys []float32, mid, reach, y, diameter float32) float32 {
	step := diameter / 2
	if step <= 0 {
		return 0
	}
	for d := float32(0); d <= reach; d += step {
		for _, off := range [2]float32{d, -d} {
			if free(xs, ys, mid+off, y, diameter) {
				return off
			}
			if d == 0 {
				break // +0 and -0 are the same candidate
			}
		}
	}
	// Nothing cleared inside the slot. The edge is the honest place for it: it
	// is still this category's observation.
	if free(xs, ys, mid+reach, y, diameter) {
		return reach
	}
	return -reach
}

// free reports whether a mark at (x, y) clears every mark already placed.
//
// Only the marks within a diameter on the value axis can collide, and the list
// is in ascending value order, so the scan walks back from the end and stops as
// soon as it is out of reach. That is what keeps a swarm of ten thousand marks
// linear in the marks rather than quadratic.
func free(xs, ys []float32, x, y, diameter float32) bool {
	for i := len(ys) - 1; i >= 0; i-- {
		if abs32(ys[i]-y) >= diameter {
			return true
		}
		if math.Hypot(float64(xs[i]-x), float64(ys[i]-y)) < float64(diameter) {
			return false
		}
	}
	return true
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func (g *swarmGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.legends(f, &g.gs, g.s, SwatchMarker))
}

func (g *swarmGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil || g.cfg.varying(g.s) {
		return LegendEntry{}, false
	}
	return LegendEntry{
		Label:  g.cfg.labelFor(),
		Color:  g.cfg.colorFor(f),
		Kind:   SwatchMarker,
		Marker: g.cfg.markerFor(f),
	}, true
}

func (g *swarmGeom) ColorGuide() (ColorGuide, bool) {
	return g.cfg.colorGuide(g.s, g.err)
}

func (g *swarmGeom) Source() data.Source { return g.src }
func (g *swarmGeom) Subset(rows []int) Geom {
	return &swarmGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *swarmGeom) Describe() Desc {
	d := g.cfg.describe(MarkBeeswarm)
	d.Source = g.src
	return d
}

var (
	_ Describer = (*swarmGeom)(nil)
	_ Faceter   = (*swarmGeom)(nil)
	_ Guided    = (*swarmGeom)(nil)
	_ Legender  = (*swarmGeom)(nil)
)
