package geom

import (
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// The two density marks. Both draw a kernel density estimate of one column,
// and they differ in what the other axis is: a violin puts the density across
// a slot on the cross axis and mirrors it, a ridgeline lets it rise out of its
// slot and overlap the next one.
//
// Both do their estimating in Train, in data space, for the reason ADR 0011
// gives for decimation: what a chart's axis says must not depend on how wide
// the chart is. A density estimated in device space would change shape when the
// window was resized.

// Violin draws the distribution of the Y column within each distinct X value,
// as a kernel density estimate mirrored about the slot's centre.
//
// It is the boxplot's answer to the boxplot's own weakness. A box says where
// the quartiles are and nothing about the shape between them, so two very
// different distributions — one hump, or two — draw the same box. A violin
// draws the shape.
//
// The X column is the grouping key, exactly as it is for [Boxplot], so a violin
// wants many rows per X value and usually a [scale.Ordinal] axis. Given
// [GroupBy] as well it draws one violin per series within each slot, side by
// side: that is the comparison a grouped boxplot makes, with the shapes left in.
//
// # What the widths mean
//
// Every estimate integrates to 1, and they are all drawn against the widest of
// them — so the violins are compared by shape and not by sample size, and one
// group with ten times the rows is not ten times as fat. [Bandwidth] pins the
// smoothing, which is what makes two groups strictly comparable; left alone,
// each group is smoothed by [stat.Silverman]'s rule from its own spread.
func Violin(src data.Source, opts ...Option) Geom {
	return &violinGeom{src: src, cfg: newConfig(opts)}
}

// violinCell is one estimated distribution: which slot it sits in, which series
// it belongs to, and the density curve itself.
type violinCell struct {
	at    float64 // the slot's X position
	group int     // the series, or 0 for an ungrouped layer
	curve []stat.Point
}

type violinGeom struct {
	src     data.Source
	cfg     config
	s       series
	gs      groups
	cells   []violinCell
	peak    float64   // the widest density in the layer
	vals    []float64 // the buffer one cell's values are gathered and sorted in
	allRows []int     // the identity row list, for an ungrouped layer
	slots   []float64 // the distinct slot positions, ascending
	at      []float64 // the buffer they are found in
	gap     float64   // the closest spacing between slots, measured in Train
	gaps    []float64
	err     error
}

func (g *violinGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	// The series index is needed before the cells are built, because a cell is
	// a slot *and* a series.
	if g.err = g.gs.train(g.src, g.s, g.cfg, x, y, NoStack); g.err != nil {
		return g.err
	}
	g.estimate(x, y)
	g.measureSlot()

	for _, c := range g.cells {
		x.Train(c.at)
		for _, p := range c.curve {
			y.Train(p.X)
		}
	}
	if _, band := x.(scale.Band); band {
		return nil
	}
	// Violins have width, so the outermost ones would be clipped in half by a
	// domain that stops at the data.
	half := g.slot() / 2 * g.widthFraction()
	if half > 0 && len(g.cells) > 0 {
		x.Train(g.cells[0].at-half, g.cells[len(g.cells)-1].at+half)
	}
	return nil
}

// estimate builds one density per (slot, series), in slot order and then in
// stacking order.
//
// Order of first appearance decides nothing here and that is deliberate: the
// slots are sorted by position because a violin sits at a place on an axis, and
// the series within a slot follow the layer's own group order so that [Order]
// moves them the way it moves a dodged bar.
func (g *violinGeom) estimate(x, y scale.Scale) {
	ok := g.s.plottable(x, y)
	g.slots, g.at = distinctPositions(g.at, g.s.x, ok)

	n := len(g.slots) * max(g.gs.count(), 1)
	g.cells = growCells(g.cells, n)[:0]
	g.peak = 0

	for _, pos := range g.slots {
		for _, grp := range g.groupOrder() {
			g.vals = g.vals[:0]
			for _, i := range g.rowsOf(grp) {
				if ok[i] && g.s.x[i] == pos {
					g.vals = append(g.vals, g.s.y[i])
				}
			}
			if len(g.vals) < 2 {
				// One observation is a point, not a distribution; drawing a
				// kernel around it would claim a spread that was never measured.
				continue
			}
			sort.Float64s(g.vals)

			cell := violinCell{at: pos, group: grp}
			if i := len(g.cells); i < cap(g.cells) {
				cell.curve = g.cells[:i+1][i].curve
			}
			cell.curve = stat.AppendKDE(cell.curve, g.vals, g.bandwidth(g.vals),
				g.vals[0], g.vals[len(g.vals)-1], 0)
			for _, p := range cell.curve {
				g.peak = math.Max(g.peak, p.Y)
			}
			g.cells = append(g.cells, cell)
		}
	}
}

// rowsOf lists the rows of one series, or every row for a layer with no groups.
// Walking the group's own rows rather than the whole table is what keeps a
// violin over many series linear in the table rather than quadratic in it —
// the same argument [groups.split] already makes.
func (g *violinGeom) rowsOf(grp int) []int {
	if g.gs.grouped() {
		return g.gs.rows[grp]
	}
	g.allRows = grow(g.allRows, len(g.s.x))
	for i := range g.s.x {
		g.allRows[i] = i
	}
	return g.allRows
}

// groupOrder is the series to walk, or a single unnamed one for a layer with no
// groups.
func (g *violinGeom) groupOrder() []int {
	if !g.gs.grouped() {
		return []int{0}
	}
	return g.gs.order
}

// bandwidth resolves the kernel width for one sorted sample: the layer's own if
// it named one, and Silverman's robust rule otherwise.
func (g *violinGeom) bandwidth(sorted []float64) float64 {
	if g.cfg.bandwidth > 0 {
		return g.cfg.bandwidth
	}
	iqr := stat.Quantile(sorted, 0.75) - stat.Quantile(sorted, 0.25)
	return stat.Silverman(stat.StdDev(sorted), iqr, len(sorted))
}

func (g *violinGeom) widthFraction() float64 {
	if g.cfg.barWidth <= 0 || g.cfg.barWidth > 1 {
		return 0.8
	}
	return g.cfg.barWidth
}

// slot is the spacing between adjacent violins in data units, measured once
// per Train.
//
// Once per Train and not once per Build: measuring it sorts, and Build runs on
// whichever goroutine a panel was given — so a layer that measured it while
// drawing would be writing its own sort buffer from two panels at once. It is
// the same reason [barGeom.gap] exists.
func (g *violinGeom) slot() float64 { return g.gap }

func (g *violinGeom) measureSlot() {
	g.gap, g.gaps = smallestGap(g.gaps, g.slots)
}

func (g *violinGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	if len(g.cells) == 0 || g.peak <= 0 {
		return nil
	}
	sc := acquire(f)
	defer sc.release()

	cd := f.Coords()
	half := g.slot() * g.widthFraction() / 2

	for _, cell := range g.cells {
		col := g.cfg.colorFor(f)
		if g.gs.grouped() {
			col = g.cfg.groupColor(f, &g.gs, cell.group)
		}
		x0, x1 := markSpan(f, cell.at, half)
		if g.gs.grouped() {
			x0, x1 = dodgeSpan(x0, x1, g.gs.rank[cell.group], g.gs.count(), g.cfg.dodgePad)
		}
		g.outline(b, f, sc, cd, cell, x0, x1, col)
	}
	return nil
}

// outline draws one violin: up the right flank and back down the left one, as a
// single closed shape.
//
// The two flanks are one subpath rather than two, because a violin is one mark:
// a pointer inside it has landed on this distribution, and two half outlines
// would be two shapes with nothing between them.
func (g *violinGeom) outline(b ir.Backend, f Frame, sc *scratch, cd coord.Coord, cell violinCell, x0, x1 float32, col ir.Color) {
	n := len(cell.curve)
	if n < 2 {
		return
	}
	mid, reach := (x0+x1)/2, (x1-x0)/2

	sc.kx, sc.ky = grow(sc.kx, 2*n)[:0], grow(sc.ky, 2*n)[:0]
	for i := range n {
		p := cell.curve[i]
		w := reach * float32(p.Y/g.peak)
		sc.kx = append(sc.kx, mid+w)
		sc.ky = append(sc.ky, f.Y.Map(p.X))
	}
	for i := n - 1; i >= 0; i-- {
		p := cell.curve[i]
		w := reach * float32(p.Y/g.peak)
		sc.kx = append(sc.kx, mid-w)
		sc.ky = append(sc.ky, f.Y.Map(p.X))
	}
	pts := cd.Points(grow(sc.pts, 2*n)[:0], sc.kx, sc.ky)
	sc.pts = pts

	sc.fill.Reset()
	appendEdges(&sc.fill, cd, pts, true)
	cd.Edge(&sc.fill, pts[len(pts)-1], pts[0])
	sc.fill.Close()

	if fill := g.cfg.fillOf(col, violinFillOpacity); fill.A != 0 {
		b.FillPath(&sc.fill, ir.Solid(fill), ir.NonZero)
	}
	stroke := ir.Stroke{Color: col, Width: pick(g.cfg.width, 1), Join: ir.JoinRound}
	if stroke.Visible() {
		b.StrokePath(&sc.fill, stroke)
	}
}

// violinFillOpacity is how much of the layer's colour a violin's interior
// keeps. A violin is read from its outline — that is where the shape is — so
// the fill only has to separate it from the grid behind it.
const violinFillOpacity = 0.35

func (g *violinGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.legends(f, &g.gs, g.s, SwatchBox))
}

func (g *violinGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil {
		return LegendEntry{}, false
	}
	return LegendEntry{Label: g.cfg.labelFor(), Color: g.cfg.colorFor(f), Kind: SwatchBox}, true
}

func (g *violinGeom) Source() data.Source { return g.src }
func (g *violinGeom) Subset(rows []int) Geom {
	return &violinGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *violinGeom) Describe() Desc {
	d := g.cfg.describe(MarkViolin)
	d.Source = g.src
	return d
}

// Ridgeline draws the distribution of the X column within each category of the
// Y axis, as a density curve rising out of that category's slot.
//
// It is the chart for "how did this distribution change" — one row per month,
// per cohort, per station — and it works because the ridges overlap: a grid of
// twenty little densities is twenty comparisons a reader has to carry between
// panels, and twenty overlapping ridges is one picture. [Overlap] sets how far
// they rise.
//
// The Y column names the rows and wants a [scale.Ordinal] axis; the X column
// holds the observations. That is the transpose of [Violin], and it is why the
// two are separate marks rather than one with a flag: a violin is a mark with a
// width and a ridgeline is a mark with a height, and neither reads as the other
// rotated.
//
// The ridges are drawn from the top of the axis down, so that each one is
// painted over the one it rises into and the front of the picture is the row
// nearest the reader.
func Ridgeline(src data.Source, opts ...Option) Geom {
	return &ridgeGeom{src: src, cfg: newConfig(opts)}
}

// ridge is one row of the chart: where it sits on the categorical axis, the
// density of its observations, and what colour it is drawn in.
type ridge struct {
	at    float64
	row   int // a source row of this category, for the colour scale
	curve []stat.Point
}

type ridgeGeom struct {
	src    data.Source
	cfg    config
	s      series
	ridges []ridge
	peak   float64
	vals   []float64
	slots  []float64
	at     []float64
	gap    float64
	gaps   []float64
	err    error
}

// ErrNotCategorical reports an axis that has to name categories and cannot.
var ErrNotCategorical = fmt.Errorf("refract/geom: this mark needs a categorical axis")

func (g *ridgeGeom) Train(x, y scale.Scale) error {
	if _, ok := y.(scale.Categorical); !ok {
		g.err = fmt.Errorf("%w: a ridgeline names its rows on the Y axis; give it a scale.Ordinal", ErrNotCategorical)
		return g.err
	}
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	g.estimate(x, y)
	g.gap, g.gaps = smallestGap(g.gaps, g.slots)

	for _, r := range g.ridges {
		y.Train(r.at)
		for _, p := range r.curve {
			x.Train(p.X)
		}
	}
	return nil
}

func (g *ridgeGeom) estimate(x, y scale.Scale) {
	ok := g.s.plottable(x, y)
	// The rows are the distinct Y values, in axis order rather than in table
	// order: a ridgeline is read down its axis.
	g.slots, g.at = distinctPositions(g.at, g.s.y, ok)

	g.ridges = growRidges(g.ridges, len(g.slots))[:0]
	g.peak = 0
	for _, pos := range g.slots {
		g.vals = g.vals[:0]
		row := -1
		for i := range g.s.y {
			if ok[i] && g.s.y[i] == pos {
				g.vals = append(g.vals, g.s.x[i])
				if row < 0 {
					row = i
				}
			}
		}
		if len(g.vals) < 2 {
			continue
		}
		sort.Float64s(g.vals)

		r := ridge{at: pos, row: row}
		if i := len(g.ridges); i < cap(g.ridges) {
			r.curve = g.ridges[:i+1][i].curve
		}
		iqr := stat.Quantile(g.vals, 0.75) - stat.Quantile(g.vals, 0.25)
		bw := g.cfg.bandwidth
		if bw <= 0 {
			bw = stat.Silverman(stat.StdDev(g.vals), iqr, len(g.vals))
		}
		r.curve = stat.AppendKDE(r.curve, g.vals, bw, g.vals[0], g.vals[len(g.vals)-1], 0)
		for _, p := range r.curve {
			g.peak = math.Max(g.peak, p.Y)
		}
		g.ridges = append(g.ridges, r)
	}
}

// slot is the spacing between adjacent rows, measured once per Train — see
// [violinGeom.slot] for why it is not measured while drawing.
func (g *ridgeGeom) slot() float64 { return g.gap }

func (g *ridgeGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	if len(g.ridges) == 0 || g.peak <= 0 {
		return nil
	}
	sc := acquire(f)
	defer sc.release()

	cd := f.Coords()

	// How far the tallest ridge rises, in device units. A slot's height is the
	// band's own where the axis has one, and the closest spacing in the data
	// where it does not.
	lo, hi := slotOn(f.Y, g.ridges[0].at, g.slot()/2)
	rise := float64(hi-lo) * g.cfg.overlap

	// Painted from the top of the axis down, so a ridge is covered by the one
	// below it rather than by the one it rises into.
	order := sc.marksIn(len(g.ridges))
	slices.SortStableFunc(order, func(a, c int) int {
		pa, pc := f.Y.Map(g.ridges[a].at), f.Y.Map(g.ridges[c].at)
		switch {
		case pa < pc:
			return -1
		case pa > pc:
			return 1
		}
		return 0
	})

	for _, i := range order {
		g.outline(b, f, sc, cd, g.ridges[i], float32(rise))
	}
	return nil
}

func (g *ridgeGeom) outline(b ir.Backend, f Frame, sc *scratch, cd coord.Coord, r ridge, rise float32) {
	n := len(r.curve)
	if n < 2 {
		return
	}
	base := f.Y.Map(r.at)
	// The ridge rises towards the top of the panel, which is whichever
	// direction the Y axis's own high end is: a device Y axis runs downwards,
	// so that is normally negative.
	up := float32(-1)
	if lo, hi := f.Y.Domain(); f.Y.Map(hi) > f.Y.Map(lo) {
		up = 1
	}

	sc.kx, sc.ky = grow(sc.kx, n+2)[:0], grow(sc.ky, n+2)[:0]
	sc.kx = append(sc.kx, f.X.Map(r.curve[0].X))
	sc.ky = append(sc.ky, base)
	for i := range n {
		p := r.curve[i]
		sc.kx = append(sc.kx, f.X.Map(p.X))
		sc.ky = append(sc.ky, base+up*rise*float32(p.Y/g.peak))
	}
	sc.kx = append(sc.kx, f.X.Map(r.curve[n-1].X))
	sc.ky = append(sc.ky, base)

	pts := cd.Points(grow(sc.pts, n+2)[:0], sc.kx, sc.ky)
	sc.pts = pts

	col := g.cfg.colorFor(f)
	if g.cfg.varying(g.s) && r.row >= 0 {
		col = g.cfg.colorScale.Color(g.s.c[r.row])
	}

	sc.fill.Reset()
	appendEdges(&sc.fill, cd, pts, true)
	sc.fill.Close()
	if fill := g.cfg.fillOf(col, ridgeFillOpacity); fill.A != 0 {
		b.FillPath(&sc.fill, ir.Solid(fill), ir.NonZero)
	}

	// The crest is stroked on its own, without the baseline the fill closed
	// over: a line along every row's slot would draw a second grid.
	stroke := ir.Stroke{Color: col, Width: pick(g.cfg.width, f.Theme.LineWidth), Join: ir.JoinRound, Cap: ir.CapRound}
	if stroke.Visible() {
		sc.line.Reset()
		appendEdges(&sc.line, cd, pts[1:len(pts)-1], true)
		b.StrokePath(&sc.line, stroke)
	}
}

// ridgeFillOpacity is how much of the layer's colour a ridge's interior keeps.
// The ridges overlap by construction, so the fill has to be light enough that
// the crest behind it still reads.
const ridgeFillOpacity = 0.55

func (g *ridgeGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil {
		return LegendEntry{}, false
	}
	return LegendEntry{Label: g.cfg.labelForX(), Color: g.cfg.colorFor(f), Kind: SwatchBox}, true
}

func (g *ridgeGeom) Source() data.Source { return g.src }
func (g *ridgeGeom) Subset(rows []int) Geom {
	return &ridgeGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *ridgeGeom) Describe() Desc {
	d := g.cfg.describe(MarkRidgeline)
	d.Source = g.src
	return d
}

// distinctPositions lists the distinct plottable values of a column in
// ascending order, into buf, and returns both the list and the buffer it is a
// prefix of.
//
// Ascending and not order of first appearance: these are positions on an axis
// rather than categories being registered, and a slot is where it is. The
// caller keeps the buffer, so a chart redrawn every frame does not allocate one.
func distinctPositions(buf, vs []float64, ok []bool) (slots, out []float64) {
	buf = buf[:0]
	for i, v := range vs {
		if i < len(ok) && !ok[i] {
			continue
		}
		buf = append(buf, v)
	}
	sort.Float64s(buf)
	n := 0
	for i, v := range buf {
		if i == 0 || v != buf[n-1] {
			buf[n] = v
			n++
		}
	}
	return buf[:n], buf
}

func growCells(buf []violinCell, n int) []violinCell {
	if cap(buf) >= n {
		return buf[:n]
	}
	return append(buf[:cap(buf)], make([]violinCell, n-cap(buf))...)
}

func growRidges(buf []ridge, n int) []ridge {
	if cap(buf) >= n {
		return buf[:n]
	}
	return append(buf[:cap(buf)], make([]ridge, n-cap(buf))...)
}

var (
	_ Describer = (*violinGeom)(nil)
	_ Faceter   = (*violinGeom)(nil)
	_ Legender  = (*violinGeom)(nil)
	_ Describer = (*ridgeGeom)(nil)
	_ Faceter   = (*ridgeGeom)(nil)
)
