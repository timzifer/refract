package geom

import (
	"errors"
	"math"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// ErrorBar draws the interval a measurement is known to within: a rule between
// two bounds, a crossbar at each end, and a marker at the measurement itself
// when the row names one.
//
// It is the mark refract drew fourteen others before: every chart of a mean, a
// forecast, a tolerance or a sampled quantity has one number and a claim about
// how well that number is known, and until this the second half had nowhere to
// go. A band through [Area] is the continuous version of the same statement
// and is right for a series; this is right for the categories and the handful
// of points where a band would be a shape drawn through three vertices.
//
// # The two spellings
//
// The interval's ends are two columns, exactly as the band of an [Area] and
// the box of a [Rect] are:
//
//	geom.ErrorBar(src, geom.X("group"), geom.Y("lo"), geom.Y2("hi"), geom.Mid("mean"))
//
// or one column of half-widths about the value, which is what a table of means
// and standard deviations already holds:
//
//	geom.ErrorBar(src, geom.X("group"), geom.Y("mean"), geom.ErrorBy("sd"))
//
// The second marks the measurement without being asked, because with that
// spelling there always is one — the centre is the [Y] column. The first marks
// it only when [Mid] names it: a minimum and a maximum are not evidence of a
// mean, and drawing one would be inventing a reading.
//
// # Which way it runs
//
// Along the axis whose two ends the encoding names, and nothing else decides
// it: [Y2] or [ErrorBy] makes it vertical, [X2] or [ErrorXBy] makes it
// horizontal. That is the rule [Rect] already follows about its edges, and it
// is why there is no orientation option to forget. A layer that names both
// pairs is a box rather than an interval, and says so rather than guessing.
//
// # What it composes with
//
// [GroupBy] gives one colour per series and [Dodge] gives each series its own
// share of the slot, so a grouped error bar lines up over the grouped bars it
// annotates — the bar layer and this one take the same [BarWidth] and the same
// [Dodge], and the caps come out half as wide as the bars by construction.
// [ColorBy] paints each row through a scale. [Caps] turns the crossbars off
// for the point-range look.
//
// Under a polar coord the rule follows the radius and the caps become arcs,
// because every span goes through the coord rather than being drawn as two
// device points — which is what makes a radial error bar the same mark rather
// than a second one.
func ErrorBar(src data.Source, opts ...Option) Geom {
	return &errorGeom{src: src, cfg: newConfig(opts)}
}

type errorGeom struct {
	src data.Source
	cfg config
	s   series

	// lo and hi are the interval's ends in the space its axis is measured in,
	// derived once in Train: from the two columns, or from the value and the
	// spread. mid is the measurement, and is nil for a layer that names none.
	lo, hi []float64
	mid    []float64

	// vertical is which axis the interval runs along, decided by the encoding.
	vertical bool

	// half is how far a cap reaches on each side of the rule, in the units of
	// the axis the mark sits across. It is measured once per Train out of a
	// buffer the layer keeps, never per row and never per frame: smallestGap
	// sorts, and asking it per row is what made a bar layer quadratic once
	// already.
	half float64

	gs   groups
	gaps []float64 // the buffer the slot is measured out of
	err  error
}

// ErrBothAxes reports a layer that named an interval on both axes. It is
// returned rather than drawn as a box because a mark that guessed which of the
// two the caller meant would draw a different chart depending on the order the
// options happened to be written in.
var ErrBothAxes = errors.New("refract/geom: an error bar names an interval on both axes; it runs along one")

// ErrNoInterval reports an error bar with no interval to draw. An interval is
// the mark, so a layer without one is a layer that named the wrong geom rather
// than one with nothing to say.
var ErrNoInterval = errors.New("refract/geom: an error bar needs an interval: give it geom.Y2/geom.X2 or geom.ErrorBy/geom.ErrorXBy")

func (g *errorGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	horizontal := g.cfg.x2col != "" || g.cfg.errXCol != ""
	vertical := g.cfg.y2col != "" || g.cfg.errCol != ""
	if horizontal && vertical {
		g.err = ErrBothAxes
		return g.err
	}
	g.vertical = !horizontal

	along, value := y, g.s.y
	if !g.vertical {
		along, value = x, g.s.x
	}
	if g.err = g.bounds(along, value); g.err != nil {
		return g.err
	}
	if g.err = g.measurement(along, value); g.err != nil {
		return g.err
	}

	trainColumn(x, g.s.x)
	trainColumn(y, g.s.y)
	trainColumn(along, g.lo)
	trainColumn(along, g.hi)
	if g.mid != nil {
		trainColumn(along, g.mid)
	}
	g.cfg.trainColors(g.s)

	// A cap has width the domain does not know about, so the outermost ones
	// would be clipped in half by an axis that stops at the data. A band scale
	// reserves its slot already; a continuous one is widened, exactly as a
	// bar's axis is.
	across, at := x, g.s.x
	if !g.vertical {
		across, at = y, g.s.y
	}
	g.half = g.halfSlot(across, at)
	widen(across, at, g.half)

	// The series index is what paints and dodges the rows; nothing here is
	// stacked, because two intervals added together are not an interval.
	g.err = g.gs.train(g.src, g.s, g.cfg, x, y, NoStack)
	return g.err
}

// bounds fills lo and hi from whichever spelling the layer used.
//
// They are derived in Train rather than in Build because the axis has to
// describe them: an interval whose top runs off the plot is exactly the
// reading a reader opened the chart for. It is the same rule a stacked bar's
// totals follow — see docs/adr/0019-position-adjustments.md.
func (g *errorGeom) bounds(along scale.Scale, value []float64) error {
	n := len(g.s.x)
	g.lo, g.hi = grow(g.lo, n), grow(g.hi, n)

	spread, second := g.cfg.errCol, g.cfg.y2col
	if !g.vertical {
		spread, second = g.cfg.errXCol, g.cfg.x2col
	}

	switch {
	case spread != "":
		e, err := column(g.src, spread, nil)
		if err != nil {
			return err
		}
		if len(e) != n {
			return errLength(g.cfg.xcol, spread, n, len(e))
		}
		for i := range n {
			// A negative half-width is the same interval read backwards, and
			// a NaN one is a row with no interval — which is a missing value,
			// so the bounds are NaN and the row is a hole like any other.
			w := math.Abs(e[i])
			g.lo[i], g.hi[i] = value[i]-w, value[i]+w
		}
	case second != "":
		hi, err := column(g.src, second, along)
		if err != nil {
			return err
		}
		if len(hi) != n {
			return errLength(g.cfg.xcol, second, n, len(hi))
		}
		for i := range n {
			g.lo[i], g.hi[i] = value[i], hi[i]
			if g.hi[i] < g.lo[i] {
				g.lo[i], g.hi[i] = g.hi[i], g.lo[i]
			}
		}
	default:
		return ErrNoInterval
	}
	return nil
}

// measurement fills mid, or leaves it nil for a layer that names none.
func (g *errorGeom) measurement(along scale.Scale, value []float64) error {
	if g.cfg.midCol != "" {
		v, err := column(g.src, g.cfg.midCol, along)
		if err != nil {
			return err
		}
		if len(v) != len(g.s.x) {
			return errLength(g.cfg.xcol, g.cfg.midCol, len(g.s.x), len(v))
		}
		g.mid = v
		return nil
	}
	// The symmetric spelling names the measurement already: it is the value
	// the interval is centred on, so the mark is a point with an interval
	// around it rather than an interval alone.
	if g.cfg.errCol != "" && g.vertical || g.cfg.errXCol != "" && !g.vertical {
		g.mid = value
	}
	return nil
}

// halfSlot is how far the caps reach on each side of the rule, in the units of
// the axis the mark sits across.
//
// Half a bar's width, so an error bar drawn over a bar chart is narrower than
// the bar it annotates — which is what makes the two readable together, and
// what a reader expects from every chart that has ever paired them.
func (g *errorGeom) halfSlot(across scale.Scale, at []float64) float64 {
	if _, band := across.(scale.Band); band {
		// A band scale reserves the slot itself and answers in device units,
		// so there is no data-space half-width to compute — [errorGeom.span]
		// reads the bandwidth instead. Inventing a data unit for a categorical
		// axis is the thing an ordinal scale exists not to have.
		return 0
	}
	gap, buf := smallestGap(g.gaps, at)
	g.gaps = buf
	return gap * g.widthFraction() / 4
}

func (g *errorGeom) widthFraction() float64 {
	if g.cfg.barWidth <= 0 || g.cfg.barWidth > 1 {
		return 0.8
	}
	return g.cfg.barWidth
}

func (g *errorGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	stroke := ir.Stroke{
		Color: g.cfg.colorFor(f),
		Width: pick(g.cfg.width, 1),
		Cap:   ir.CapButt,
		Dash:  g.cfg.dashFor(f),
	}
	if !stroke.Visible() && !g.gs.grouped() && !g.cfg.varying(g.s) {
		return nil
	}

	sc := acquire(f)
	defer sc.release()

	cd := f.Coords()
	along, across := f.Y, f.X
	at := g.s.x
	if !g.vertical {
		along, across = f.X, f.Y
		at = g.s.y
	}

	// Which rows can be drawn is decided once, and every traversal below reads
	// that answer: the rule, the caps and the marker have to agree about where
	// the holes are, or a cap would appear at the end of a rule that was not
	// drawn.
	ok := sc.plottable(g.s, f.X, f.Y)
	rows := sc.rows[:0]
	for i := range g.s.x {
		if !ok[i] || !defined(along, g.lo[i]) || !defined(along, g.hi[i]) {
			continue
		}
		rows = append(rows, i)
	}
	sc.rows = rows
	if len(rows) == 0 {
		return nil
	}

	cols := sc.colorsFor(g.cfg, g.s, rows)
	var run ir.Path
	for j, i := range rows {
		st := stroke
		if cols != nil {
			if cols[j].A == 0 {
				continue
			}
			st.Color = cols[j]
		}
		c0, c1 := g.span(across, at, i)
		mid := (c0 + c1) / 2
		lo, hi := along.Map(g.lo[i]), along.Map(g.hi[i])

		g.rule(b, cd, &run, mid, lo, hi, st)
		if !g.cfg.caps {
			continue
		}
		g.cap(b, cd, &run, c0, c1, lo, st)
		g.cap(b, cd, &run, c0, c1, hi, st)
	}

	g.marks(b, f, cd, sc, rows, along, across, at, stroke, cols)
	return nil
}

// span is the interval's extent across its own axis: the slot the row sits in,
// narrowed by the width fraction and divided by [Dodge] where a grouped layer
// asked for it.
func (g *errorGeom) span(across scale.Scale, at []float64, i int) (float32, float32) {
	c0, c1 := slotOn(across, at[i], g.half)
	if band, isBand := across.(scale.Band); isBand {
		// The band gave the whole slot; the cap takes the same share of it a
		// bar would, halved.
		mid, w := across.Map(at[i]), float32(float64(band.Bandwidth())*g.widthFraction()/4)
		c0, c1 = mid-w, mid+w
	}
	if g.cfg.dodge {
		c0, c1 = dodgeSpan(c0, c1, g.gs.slotIndex(i), g.gs.count(), g.cfg.dodgePad)
	}
	return c0, c1
}

// rule strokes the interval itself, and cap one of its crossbars. Both go
// through the coord: a run at a constant position on one axis is a straight
// line only where the coord says so, and under a polar coord a cap is an arc.
func (g *errorGeom) rule(b ir.Backend, cd coord.Coord, p *ir.Path, at, lo, hi float32, st ir.Stroke) {
	g.stroke(b, cd, p, at, lo, at, hi, st)
}

func (g *errorGeom) cap(b ir.Backend, cd coord.Coord, p *ir.Path, c0, c1, at float32, st ir.Stroke) {
	g.stroke(b, cd, p, c0, at, c1, at, st)
}

// stroke draws one segment in the mark's own orientation, swapping the pair
// for a horizontal layer so that the two directions are one code path rather
// than two that can disagree about a cap's width.
func (g *errorGeom) stroke(b ir.Backend, cd coord.Coord, p *ir.Path, a0, b0, a1, b1 float32, st ir.Stroke) {
	from, to := ir.Point{X: a0, Y: b0}, ir.Point{X: a1, Y: b1}
	if !g.vertical {
		from = ir.Point{X: b0, Y: a0}
		to = ir.Point{X: b1, Y: a1}
	}
	strokeRun(b, cd, p, []ir.Point{cd.Point(from.X, from.Y), cd.Point(to.X, to.Y)}, st, false)
}

// marks draws the measurement and reports the rows.
//
// The row is reported at the measurement where there is one and at the middle
// of the interval where there is not — which is where a reader points when
// they mean "this interval", and is the same choice a stacked bar segment
// makes for the same reason: neither end is the value.
func (g *errorGeom) marks(b ir.Backend, f Frame, cd coord.Coord, sc *scratch, rows []int,
	along, across scale.Scale, at []float64, stroke ir.Stroke, cols []ir.Color,
) {
	pts := sc.pts[:0]
	for _, i := range rows {
		c0, c1 := g.span(across, at, i)
		mid := (c0 + c1) / 2
		v := (along.Map(g.lo[i]) + along.Map(g.hi[i])) / 2
		if g.mid != nil {
			if !defined(along, g.mid[i]) {
				// The interval is still a reading. A marker at a value the
				// axis has no place for would be a mark at the edge of the
				// plot claiming to be the measurement.
				continue
			}
			v = along.Map(g.mid[i])
		}
		a, c := mid, v
		if !g.vertical {
			a, c = v, mid
		}
		pts = append(pts, cd.Point(a, c))
	}
	sc.pts = pts
	if len(pts) == 0 {
		return
	}
	if f.tracking() {
		f.Marks(pts, sc.sourceRows(g.s, rows))
	}
	if g.mid == nil {
		return
	}
	style := ir.MarkerStyle{Size: pick(g.cfg.size, f.Theme.MarkerSize), Fill: stroke.Color}
	marker := g.cfg.markerFor(f)
	if cols == nil {
		b.Markers(marker, pts, style)
		return
	}
	for _, r := range sc.groupByColor(pts, cols) {
		if r.color.A == 0 {
			continue
		}
		st := style
		st.Fill = r.color
		b.Markers(marker, r.pts, st)
	}
}

func (g *errorGeom) ColorGuide() (ColorGuide, bool) {
	return g.cfg.colorGuide(g.s, g.err)
}

func (g *errorGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.legends(f, &g.gs, g.s, SwatchLine))
}

func (g *errorGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil || g.cfg.varying(g.s) {
		return LegendEntry{}, false
	}
	return LegendEntry{
		Label: g.cfg.labelFor(),
		Color: g.cfg.colorFor(f),
		Kind:  SwatchLine,
		Dash:  g.cfg.dashFor(f),
	}, true
}

func (g *errorGeom) Source() data.Source { return g.src }
func (g *errorGeom) Subset(rows []int) Geom {
	return &errorGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *errorGeom) Describe() Desc {
	d := g.cfg.describe(MarkErrorBar)
	d.Source = g.src
	return d
}

var (
	_ Describer = (*errorGeom)(nil)
	_ Faceter   = (*errorGeom)(nil)
	_ Guided    = (*errorGeom)(nil)
	_ Legender  = (*errorGeom)(nil)
)
