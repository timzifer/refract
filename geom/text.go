package geom

import (
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// Text draws one label per row, read from a column.
//
// It is what [Note] is not: a note places one literal string at one literal
// position, so labelling rows with it costs a layer per row — and since a plot
// only ever gains layers, a chart whose rows change has to be rebuilt from
// scratch, which takes the reader's zoom with it. A text layer reads the same
// [data.Source] every other mark reads and needs no rebuild when the rows
// change.
//
// The label column is [TextBy], and any column will do: a text column is used
// as it is, a numeric or temporal one is formatted the way a category name is.
//
// Where the label goes follows from which channels the layer names.
//
// Naming neither [X2] nor [Y2] puts the label at the row's point, laid out by
// [Align] exactly as a note is — the name beside a scatter point, the value
// above a bar:
//
//	geom.Text(src, geom.X("t"), geom.Y("v"), geom.TextBy("name"),
//	    geom.Align(ir.AlignCenter, ir.AlignBottom))
//
// Naming either of them puts the label in the middle of the box the row spans,
// and that box is exactly the one [Rect] would draw for the same options — an
// edge the row does not name is the slot the axis implies, as it is there. So
// one set of options describes the rectangles and labels them:
//
//	opts := []geom.Option{geom.X("start"), geom.X2("end"), geom.Y("lo"), geom.Y2("hi"),
//	    geom.ColorBy("state", pal)}
//	p.Add(geom.Rect(src, opts...))
//	p.Add(geom.Text(src, append(opts, geom.TextBy("label"))...))
//
// Two things follow from the layer knowing the box, and both are why this is a
// mark rather than a recipe over [Note].
//
// A label is measured with the font it will be drawn in, and one that overruns
// its box is dropped — an overrunning label reads as belonging to the
// neighbour. [Elide] truncates it instead. And the middle of the box is the
// middle of its *visible* part: a bar half scrolled off the edge carries its
// label in the middle of what is left rather than off-screen with the box's
// true centre.
//
// A layer given [ColorBy] takes each label's ink from the fill that scale
// gives the row, dark on light and light on dark, so that a qualitative
// palette does not leave half its categories unreadable. [Color] overrides it.
//
// AvoidOverlap opts into the renderer's panel-local label layout. Point labels
// may move; box labels remain anchored to their own box and are dropped if they
// collide. Without that option, neighbouring labels are not moved apart.
func Text(src data.Source, opts ...Option) Geom {
	return &textGeom{src: src, cfg: newConfig(opts)}
}

type textGeom struct {
	src    data.Source
	cfg    config
	s      series
	x2     []float64
	labels []string
	gaps   []float64 // the buffer a slot-sized edge is measured out of

	// cut and elided cache the truncation: a chart redrawn every frame must
	// not build a string per row, so a row whose label is cut where it was cut
	// last frame reuses the string. They live on the layer rather than in the
	// frame's pool because they have to survive a Build — the same reason
	// barGeom.gaps does.
	cut    []int
	elided []string

	err error
}

// errNoText reports a text column the source does not have. A layer that says
// nothing is a layer with nothing to draw, so an absent column is an error
// rather than a silent no-op.
func errNoText(col string) error {
	if col == "" {
		return fmt.Errorf("%w: no label column selected (use geom.TextBy)", ErrNoColumn)
	}
	return fmt.Errorf("%w: %q", ErrNoColumn, col)
}

// ellipsis ends a truncated label. One character rather than three dots: it is
// what a font has a glyph for, and it costs a third of the width the box did
// not have in the first place.
const ellipsis = "…"

func (g *textGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if g.cfg.x2col != "" {
		g.x2, g.err = column(g.src, g.cfg.x2col, x)
		if g.err != nil {
			return g.err
		}
		if len(g.x2) != len(g.s.x) {
			g.err = errLength(g.cfg.xcol, g.cfg.x2col, len(g.s.x), len(g.x2))
			return g.err
		}
	}
	var ok bool
	if g.labels, ok = data.Labels(g.src, g.cfg.textCol); !ok {
		g.err = errNoText(g.cfg.textCol)
		return g.err
	}
	if len(g.labels) != len(g.s.x) {
		g.err = errLength(g.cfg.xcol, g.cfg.textCol, len(g.s.x), len(g.labels))
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	trainColumn(x, g.s.x)
	trainColumn(y, g.s.y)
	if g.x2 != nil {
		trainColumn(x, g.x2)
	}
	if g.s.y2 != nil {
		trainColumn(y, g.s.y2)
	}
	g.cfg.trainColors(g.s)

	// No widen. A mark with width pads its axis so the outermost one is not
	// clipped in half; a label's ink is not a width in data space, and the
	// layer whose boxes these are has already padded the axis for both.
	return nil
}

// boxed reports whether this layer labels a box rather than a point. It is the
// encoding that decides: a row that names a far edge on either axis spans
// something, and a row that names neither is at a place.
func (g *textGeom) boxed() bool { return g.x2 != nil || g.s.y2 != nil }

func (g *textGeom) halfWidth(vs []float64) float64 {
	gap, buf := smallestGap(g.gaps, vs)
	g.gaps = buf
	frac := g.cfg.barWidth
	if frac <= 0 || frac > 1 {
		frac = 1
	}
	return gap * frac / 2
}

func (g *textGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	size := g.cfg.fontSize
	if size <= 0 {
		size = f.Theme.LabelSize
	}
	run := ir.TextRun{Font: f.Theme.Font(size), Rotation: g.cfg.rotation}

	sc := acquire(f)
	defer sc.release()

	cd := f.Coords()
	ok := sc.plottable(g.s, f.X, f.Y)
	boxed := g.boxed()
	run.H, run.V = g.align(boxed)

	var halfX, halfY float64
	if boxed {
		halfX, halfY = g.halfWidth(g.s.x), g.halfWidth(g.s.y)
	}
	x0e, x1e, y0e, y1e := cd.Extent()

	// The rows worth drawing, and where each one's label goes. A text layer
	// aggregates nothing and decimates nothing: a label dropped to save a
	// pixel is a row nobody can read, which is the opposite of the point.
	rows := sc.rows[:0]
	pts := sc.pts[:0]
	room := sc.dx[:0]
	for i := range g.s.x {
		if !ok[i] || g.labels[i] == "" {
			continue
		}
		if g.x2 != nil && !defined(f.X, g.x2[i]) {
			continue
		}
		var at ir.Point
		w := float32(math.Inf(1))
		if boxed {
			x0, x1 := spanOn(f.X, g.s.x, g.x2, i, halfX, true)
			y0, y1 := spanOn(f.Y, g.s.y, g.s.y2, i, halfY, false)

			// The middle of the visible part of the box rather than the middle
			// of the box: a bar half scrolled off the edge carries its label in
			// what is left of it. The clamp is against the interval each scale
			// maps into rather than against the plot rectangle, because that is
			// what the coord answers for — under a polar coord it is an angle
			// and a radius rather than two edges.
			x0, x1 = clamp(x0, x0e, x1e), clamp(x1, x0e, x1e)
			y0, y1 = clamp(y0, y0e, y1e), clamp(y1, y0e, y1e)
			ym := (y0 + y1) / 2
			at = cd.Point((x0+x1)/2, ym)

			// How much room there is, measured on screen rather than in the
			// space the scales map into. The two are the same under Cartesian,
			// and under a polar coord the box's width is an angle while what a
			// label needs is a length: the chord between the edges is the
			// honest answer, because it never claims room the ink has not got.
			p0, p1 := cd.Point(x0, ym), cd.Point(x1, ym)
			w = float32(math.Hypot(float64(p1.X-p0.X), float64(p1.Y-p0.Y)))
		} else {
			at = cd.Point(f.X.Map(g.s.x[i]), f.Y.Map(g.s.y[i]))
		}
		rows = append(rows, i)
		pts = append(pts, at)
		room = append(room, w)
	}
	sc.rows, sc.pts, sc.dx = rows, pts, room
	if len(rows) == 0 {
		return nil
	}

	cols := sc.colorsFor(g.cfg, g.s, rows)
	ink := g.ink(f)

	// One Text call per label. Colour batching does not apply here: the IR
	// carries one style per drawing call and a run is not a path, so a layer
	// of N labels is N calls whatever their colours are.
	drawn := sc.keep[:0]
	for i, row := range rows {
		text, fits := g.fit(b, run.Font, g.labels[row], room[i], row)
		if !fits {
			continue
		}
		run.Color = ink
		if cols != nil {
			run.Color = contrast(f.Theme, cols[i])
		}
		if g.cfg.color != nil {
			run.Color = *g.cfg.color
		}
		if run.Color.A == 0 {
			continue
		}
		run.Text, run.At = text, pts[i]
		if g.cfg.avoidLabels && f.Labels != nil {
			at, keep := f.Labels.PlaceLabel(run, !boxed)
			if !keep {
				continue
			}
			run.At, pts[i] = at, at
		}
		b.Text(run)
		drawn = append(drawn, i)
	}
	sc.keep = drawn

	// The rows behind the labels that were drawn, and only those: a label the
	// box had no room for is not a mark a pointer can land on.
	if f.tracking() {
		for i, e := range drawn {
			pts[i], rows[i] = pts[e], rows[e]
		}
		f.Marks(pts[:len(drawn)], sc.sourceRows(g.s, rows[:len(drawn)]))
	}
	return nil
}

// align is how a label sits about its anchor.
//
// A layer that was told nothing centres a label in its box and hangs it off
// its point, which is why the config records having been told: the start of a
// run on the baseline is a zero value as well as an alignment.
func (g *textGeom) align(boxed bool) (ir.HAlign, ir.VAlign) {
	if g.cfg.alignSet || !boxed {
		return g.cfg.halign, g.cfg.valign
	}
	return ir.AlignCenter, ir.AlignMiddle
}

// fit measures a label against the room it has and reports what to draw.
//
// A label that does not fit is dropped rather than allowed to overrun, because
// an overrunning label reads as belonging to the neighbouring row. With
// [Elide] it is truncated instead, and where it was cut is remembered: a chart
// redrawn at the same size cuts every label where it cut it last frame, so a
// steady-state frame builds no strings at all. That is the difference between
// a live chart that allocates per row and one that does not.
func (g *textGeom) fit(m ir.Measurer, font ir.FontRef, s string, room float32, row int) (string, bool) {
	if math.IsInf(float64(room), 1) {
		return s, true
	}
	if room <= 0 {
		return "", false
	}
	if m.Measure(ir.TextRun{Text: s, Font: font}).Advance <= room {
		return s, true
	}
	if !g.cfg.elide {
		return "", false
	}
	room -= m.Measure(ir.TextRun{Text: ellipsis, Font: font}).Advance
	if room <= 0 {
		return "", false
	}

	// The longest prefix that still leaves room for the ellipsis, found by
	// halving rather than by walking: a run's advance does not shrink as the
	// run grows, so the boundary can be searched for, and a long label in a
	// narrow box is exactly where a per-character walk would be felt. The cut
	// is on a rune boundary — half a character is not a shorter label.
	lo, hi := 0, len(s)
	for lo < hi {
		mid := lo + (hi-lo+1)/2
		for mid > lo && !utf8.RuneStart(s[mid]) {
			mid--
		}
		if mid == lo {
			break
		}
		if m.Measure(ir.TextRun{Text: s[:mid], Font: font}).Advance <= room {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return "", false
	}
	return g.remember(row, lo), true
}

// remember hands back this row's label cut at n bytes, out of the layer's own
// cache when the cut has not moved since the last frame.
func (g *textGeom) remember(row, n int) string {
	if len(g.cut) != len(g.labels) {
		g.cut = grow(g.cut, len(g.labels))
		g.elided = grow(g.elided, len(g.labels))
		for i := range g.cut {
			g.cut[i], g.elided[i] = -1, ""
		}
	}
	if g.cut[row] == n {
		return g.elided[row]
	}
	out := g.labels[row][:n] + ellipsis
	g.cut[row], g.elided[row] = n, out
	return out
}

// ink is the colour of a layer whose labels are not coloured per row.
func (g *textGeom) ink(f Frame) ir.Color {
	if g.cfg.color != nil {
		return *g.cfg.color
	}
	// A uniform fill is a background to read against exactly as a scale's is,
	// so a label inside one follows it too.
	if g.cfg.fill != nil {
		return contrast(f.Theme, *g.cfg.fill)
	}
	if f.Theme.LabelColor.A != 0 {
		return f.Theme.LabelColor
	}
	return theme.Light.LabelColor
}

// contrast picks the more readable of the theme's two inks against a fill.
//
// The pair is the theme's own — the colour it labels with and the colour
// behind the chart — rather than black and white, so that a dark theme's
// labels are drawn in its own colours and a chart still looks like one thing.
// The comparison is on luminance, which is why [palette.Luminance] is where it
// is: a fill comes from a colour scale, and how light a colour is is a fact
// about the colour rather than about the mark.
func contrast(th theme.Theme, bg ir.Color) ir.Color {
	label, back := th.LabelColor, th.Background
	if label.A == 0 {
		label = theme.Light.LabelColor
	}
	if back.A == 0 {
		back = theme.Light.Background
	}
	l := palette.Luminance(bg)
	if math.Abs(palette.Luminance(back)-l) > math.Abs(palette.Luminance(label)-l) {
		return back
	}
	return label
}

// clamp confines v to an interval given in either order, because the interval
// a scale maps into runs downwards on a vertical axis.
func clamp(v, lo, hi float32) float32 {
	if lo > hi {
		lo, hi = hi, lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// A text layer contributes no legend entry, for the reason an annotation does
// not: what it draws is the reading rather than a series to be named.
func (g *textGeom) Legend(f Frame) (LegendEntry, bool) { return LegendEntry{}, false }

func (g *textGeom) Describe() Desc {
	d := g.cfg.describe(MarkText)
	d.Source = g.src
	return d
}

func (g *textGeom) Source() data.Source { return g.src }

// AvoidsLabels reports whether this layer requests panel-local label layout.
func (g *textGeom) AvoidsLabels() bool { return g.cfg.avoidLabels }

func (g *textGeom) Subset(rows []int) Geom {
	return &textGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

// A text layer holds rows, so it is faceted by splitting them. It contributes
// no colour guide: the layer whose boxes it labels carries the same ColorBy
// and has already contributed one, and a second would be the same guide twice.
var (
	_ Describer = (*textGeom)(nil)
	_ Faceter   = (*textGeom)(nil)
)
