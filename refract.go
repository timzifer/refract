// Package refract turns one declarative chart specification into any output
// you need — SVG, PDF and a browser canvas today, raster through one more
// module, GPU and a native window through later ones — from the same model,
// with the same geometry.
//
// The core module is pure Go and depends on nothing but the standard library.
// Both vector emitters are built in and need no rendering engine and no font
// stack, so a server that wants a chart as SVG or a report generator that
// wants one as PDF links nothing native and nothing young. The browser backend
// is built in for the same reason: a canvas 2D context is reached through
// syscall/js. Raster output lives in a separate module,
// github.com/timzifer/refract/backend/gg, which is still CGO-free.
//
// # Shape of the API
//
// Build a plot, give it scales, add layers, render it to a target:
//
//	src := refract.Float64Columns(map[string][]float64{"t": times, "y": values})
//
//	p := refract.New(
//	    refract.Theme(theme.Dark),
//	    refract.Size(800, 500),
//	    refract.Title("Signal"),
//	)
//	p.X(scale.Time())
//	p.Y(scale.Linear(scale.Nice()))
//	p.Add(geom.Line(src, geom.X("t"), geom.Y("y"), geom.Color(palette.Blue)))
//
//	err := p.Render(refract.SVG("signal.svg"))
//
// Scales cover linear, time, log, symlog and ordinal/categorical axes; geoms
// cover lines, scatters, bars, areas, steps and boxplots. A mark's colour can
// come from the data through [scale.Sequential] or [scale.Diverging] and
// [geom.ColorBy], which contributes a colourbar beside the plot.
//
// # Annotations
//
// [geom.HLine], [geom.VLine], [geom.HBand], [geom.VBand], [geom.Segment],
// [geom.Region] and [geom.Note] add the marks that are not data — a threshold,
// a shaded window, a label pointing at what happened. They take values rather
// than a data source.
//
// # Many panels
//
// [Plot.Facet] splits one plot into small multiples, one panel per value of a
// column; [NewGrid] puts several different plots on one canvas. Both lay their
// panels out with the same solver, so the axes line up either way.
//
//	p.Facet(facet.Wrap("region", facet.Columns(3)))
//
// # Interaction
//
// [Plot.On] registers a handler for hover, click, zoom or pan, and [Plot.Live]
// draws the chart into a surface that can be redrawn, pointed at, panned and
// zoomed. Each redraw repaints only what changed, and a frame identical to the
// last is not painted at all. In a browser, [Live.Bind] wires a DOM element to
// all of it; see package backend/canvas.
//
//	p.On(refract.Hover, func(ev refract.Event) {
//	    if ev.Found {
//	        tooltip(ev.Series(), ev.Hit.X, ev.Hit.Y)
//	    }
//	})
//
//	live, err := p.Live(canvas.Element(el))
//
// # Live data
//
// [data.Stream] is a table a producer appends to from one goroutine while the
// renderer draws a frozen snapshot on another.
//
// # A chart as JSON
//
// A Plot marshals to a Vega-Lite-shaped document and reads back as the same
// chart — see [Plot.Spec], [ParseJSON] and package spec.
//
// # Status
//
// Pre-alpha. Every release below v1.0.0 may contain breaking changes without
// a deprecation cycle. See CONCEPT.md for the design and the roadmap.
package refract

import (
	"errors"
	"fmt"
	"io"

	"github.com/timzifer/refract/backend/pdf"
	"github.com/timzifer/refract/backend/svg"
	coordpkg "github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/mathtext"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/scale"
	themepkg "github.com/timzifer/refract/theme"
)

// Re-exports so that a simple chart needs one import.
type (
	// Source is a columnar data source. See package data.
	Source = data.Source
	// Target is a render destination. See package ir.
	Target = ir.Target
	// Backend is a renderer. See package ir.
	Backend = ir.Backend
)

// Float64Columns builds a Source over numeric columns, borrowing the slices.
// See [data.Float64Columns].
func Float64Columns(cols map[string][]float64) Source { return data.Float64Columns(cols) }

// NewTable returns an empty table that can mix numeric and time columns.
// See [data.NewTable].
func NewTable() *data.Table { return data.NewTable() }

// Plot is a chart specification: size, theme, scales and layers.
//
// A Plot is not safe for concurrent modification. Rendering the same Plot
// twice is supported and produces the same result, provided the underlying
// data has not changed.
type Plot struct {
	width, height int
	dpr           float64
	theme         themepkg.Theme

	title  string
	xTitle string
	yTitle string

	x, y   scale.Scale
	coord  coordpkg.Coord
	layers []geom.Geom

	facet *facet.Spec

	legend    bool
	legendSet bool

	// math typesets the notation in this plot's labels, and is nil for a plot
	// whose labels are text.
	math mathtext.Typesetter

	// desc is the accessible description, and descSet distinguishes "no
	// description" from one deliberately set to nothing.
	desc    ir.Description
	descSet bool

	// responsive scales the theme with the size the chart is actually drawn
	// at; refW and refH are the size it was designed at, which is the size
	// given at construction.
	responsive bool
	refW, refH int

	serial bool

	handlers map[EventKind][]func(Event)
}

// Option configures a Plot at construction.
type Option func(*Plot)

// Size sets the output size in device-independent pixels. The default is
// 800x500.
func Size(w, h int) Option {
	return func(p *Plot) {
		if w > 0 {
			p.width = w
		}
		if h > 0 {
			p.height = h
		}
	}
}

// DPR sets the device pixel ratio. Backends that rasterize multiply the pixel
// buffer by it; coordinates stay in device-independent units either way. The
// default is 1.
func DPR(r float64) Option {
	return func(p *Plot) {
		if r > 0 {
			p.dpr = r
		}
	}
}

// Responsive scales the theme with the size the chart is drawn at.
//
// A plot is designed at one size — the one [Size] gave it, or the default
// 800x500 — and a responsive one keeps its proportions when it is drawn at
// another: half the width and half the height means half-size type, half-width
// strokes, half the margins. Without it a chart shrunk to a third of its
// design size keeps 12pt labels, and they eat the plot area.
//
// It matters for a window, which the reader resizes, and for a browser canvas
// in a fluid layout. It does nothing at all to a chart rendered once at the
// size it was built with — the factor is exactly 1 — so turning it on cannot
// change an existing still.
//
// The factor is the smaller of the two ratios, so that a chart stretched wide
// scales to what still fits its height, and it is clamped to the range a chart
// stays legible over. Colours do not scale; see [theme.Scaled] for what does.
func Responsive(on bool) Option { return func(p *Plot) { p.responsive = on } }

// ResponsiveFrom is [Responsive] with the design size given explicitly rather
// than taken from [Size].
//
// It is what a still rendered at another size needs: a thumbnail of a chart
// designed at 800x500 is `Size(200, 125)` with `ResponsiveFrom(800, 500)`, and
// it comes out as the chart at a quarter of the size rather than as the chart
// with four times the type in it. A [Live] surface needs neither, because the
// size it was built with is already the design.
func ResponsiveFrom(w, h int) Option {
	return func(p *Plot) {
		if w > 0 && h > 0 {
			p.responsive, p.refW, p.refH = true, w, h
		}
	}
}

// Math typesets the notation in the chart's labels.
//
// It applies to every label the chart draws — the title, the axis titles, the
// tick labels, the legend, a facet's strip, a geom's own note — because a
// typesetter is installed by wrapping the backend rather than by being consulted
// at each place text is written.
//
//	p := refract.New(
//	    refract.Math(mathtext.TeX()),
//	    refract.YTitle(`flux density $F_\nu$ ($\mathrm{W\,m^{-2}\,Hz^{-1}}$)`),
//	)
//
// Passing nil turns it off, which is the default: a chart with no typesetter
// draws its labels exactly as they were written, and pays nothing for the
// notation it does not have. See package mathtext.
func Math(ts mathtext.Typesetter) Option { return func(p *Plot) { p.math = ts } }

// Coord sets the coordinate system: what the interval a scale maps into means.
//
// The default is [coord.Cartesian], the identity, where the interval is a
// distance along an edge of the plot. [coord.Polar] wraps one axis around a
// circle and reads the other as a radius, which is all a pie, a donut, a
// radar, a rose or a gauge is — the marks are the ones that were already
// there:
//
//	p := refract.New(refract.Coord(coord.Polar(coord.Theta(coord.FromY))))
//	p.X(scale.Linear())
//	p.Y(scale.Linear())
//	p.Add(geom.Bar(src, geom.X("one"), geom.Y("share"), geom.GroupBy("browser")))
//
// A coord belongs to the chart rather than to a panel, so the panels of a
// facet all share it.
func Coord(c coordpkg.Coord) Option { return func(p *Plot) { p.coord = c } }

// Theme sets the visual tokens. The default is [theme.Light].
func Theme(t themepkg.Theme) Option { return func(p *Plot) { p.theme = t } }

// Title sets the chart title.
func Title(s string) Option { return func(p *Plot) { p.title = s } }

// XTitle sets the horizontal axis title.
func XTitle(s string) Option { return func(p *Plot) { p.xTitle = s } }

// YTitle sets the vertical axis title.
func YTitle(s string) Option { return func(p *Plot) { p.yTitle = s } }

// Legend forces the legend on or off. By default a legend appears once a plot
// has more than one layer: one series does not need to be told apart from
// anything.
func Legend(show bool) Option {
	return func(p *Plot) { p.legend, p.legendSet = show, true }
}

// Parallel controls whether a multi-panel chart builds its panels
// concurrently. It is on by default and produces identical output either way:
// each panel is recorded on its own goroutine and the recordings are replayed
// in panel order.
//
// Turn it off to keep a render on one goroutine — inside a benchmark that is
// measuring something else, or in a process that has already committed its
// cores elsewhere. It has no effect on a chart with a single panel, which has
// nothing to overlap.
func Parallel(on bool) Option { return func(p *Plot) { p.serial = !on } }

// New creates a Plot.
func New(opts ...Option) *Plot {
	p := &Plot{
		width:  800,
		height: 500,
		dpr:    1,
		theme:  themepkg.Light,
	}
	for _, o := range opts {
		o(p)
	}
	// The size the plot was configured at is the size it was designed at, and
	// [Responsive] measures every later size against it — unless
	// [ResponsiveFrom] named a different design, which is the case where the
	// two are not the same thing.
	if p.refW == 0 || p.refH == 0 {
		p.refW, p.refH = p.width, p.height
	}
	return p
}

// themeFor returns the theme to draw at the given size: the plot's own, scaled
// to the size when the plot is responsive.
func (p *Plot) themeFor(w, h int) themepkg.Theme {
	f := p.sizeFactor(w, h)
	if f == 1 {
		return p.theme
	}
	return p.theme.With(themepkg.Scaled(f))
}

// sizeFactor is how much smaller or larger this drawing is than the one the
// plot was designed for.
//
// The smaller of the two ratios wins: a chart stretched to twice the width at
// the same height has no more room for type than it had, and scaling by the
// width would overflow it. The clamp is what stops a thumbnail from asking for
// a half-pixel stroke and a wall display for a 200pt tick label; past those
// bounds a chart wants a different design rather than the same one scaled.
func (p *Plot) sizeFactor(w, h int) float64 {
	if !p.responsive || p.refW <= 0 || p.refH <= 0 || w <= 0 || h <= 0 {
		return 1
	}
	f := min(float64(w)/float64(p.refW), float64(h)/float64(p.refH))
	return min(max(f, minResponsiveScale), maxResponsiveScale)
}

// The bounds [Plot.sizeFactor] clamps to. A quarter size keeps a 12pt label at
// 3pt, which is the smallest that is still text; four times over is a chart
// filling a wall.
const (
	minResponsiveScale = 0.25
	maxResponsiveScale = 4
)

// Size reports the size the plot is drawn at, in device-independent pixels.
// It is what [Size] set, or the default, and it is what a surface opening a
// window for this plot wants to know.
func (p *Plot) Size() (w, h int) { return p.width, p.height }

// X sets the horizontal scale. The default is [scale.Linear] with nicing.
func (p *Plot) X(s scale.Scale) *Plot { p.x = s; return p }

// Y sets the vertical scale. The default is [scale.Linear] with nicing.
func (p *Plot) Y(s scale.Scale) *Plot { p.y = s; return p }

// Add appends layers, drawn in the order given.
func (p *Plot) Add(gs ...geom.Geom) *Plot { p.layers = append(p.layers, gs...); return p }

// Facet splits the plot into small multiples, one panel per value of a
// column. See [facet.Wrap] and [facet.Grid].
//
//	p.Facet(facet.Wrap("region", facet.Columns(3)))
//
// Passing nil turns faceting back off.
func (p *Plot) Facet(s *facet.Spec) *Plot { p.facet = s; return p }

// ErrNoLayers reports a render of a plot with nothing in it. Rendering empty
// axes is a legitimate thing to want, so this is only returned when there is
// also no scale configured — that combination is always a mistake.
var ErrNoLayers = errors.New("refract: plot has no layers and no scales")

// Render draws the plot into t.
//
// It opens the target, lowers the chart into the backend it returns, flushes,
// and closes the target — so a file target has a complete file on disk when
// Render returns nil.
func (p *Plot) Render(t Target) (err error) {
	if t == nil {
		return errors.New("refract: nil render target")
	}
	if len(p.layers) == 0 && p.x == nil && p.y == nil {
		return ErrNoLayers
	}

	c, err := p.chart()
	if err != nil {
		return err
	}

	b, err := t.Open(p.width, p.height, p.dpr)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := t.Close(); err == nil {
			err = cerr
		}
	}()

	if err = render.Draw(b, c); err != nil {
		return err
	}
	return b.Flush()
}

// chart resolves the plot into what render draws: one panel, or the grid of
// panels a facet spec cuts it into.
func (p *Plot) chart() (render.Chart, error) {
	c := render.Chart{
		Width:       p.width,
		Height:      p.height,
		DPR:         p.dpr,
		Theme:       p.themeFor(p.width, p.height),
		Title:       p.title,
		XTitle:      p.xTitle,
		YTitle:      p.yTitle,
		X:           p.scaleX(),
		Y:           p.scaleY(),
		Coord:       p.coord,
		Layers:      p.layers,
		ShowLegend:  p.showLegend(),
		Description: p.Description(),
		Math:        p.math,
		Serial:      p.serial,
	}
	if p.facet == nil {
		return c, nil
	}

	panels, rows, cols, err := p.facet.Split(p.layers)
	if err != nil {
		return render.Chart{}, err
	}
	freeX, freeY := p.facet.FreeScales()
	c.Rows, c.Cols = rows, cols
	c.Layers = nil
	for _, fp := range panels {
		rp := render.Panel{
			Row:        fp.Row,
			Col:        fp.Col,
			Strip:      fp.Strip,
			RightStrip: fp.RightStrip,
			Layers:     fp.Layers,
			X:          c.X,
			Y:          c.Y,
			// A shared axis is written once, at the edge of the grid — which
			// is the last panel in the column, not the last row: a wrapped
			// facet whose final row is short would otherwise leave the
			// panels above the gap with no labels at all. A free axis is
			// written on every panel, because it is a different axis each
			// time and a reader who assumed otherwise would misread every
			// panel but one.
			ShowX: freeX || outermost(panels, fp, below),
			ShowY: freeY || outermost(panels, fp, leftOf),
		}
		if freeX {
			if rp.X, err = freeScale(c.X); err != nil {
				return render.Chart{}, err
			}
		}
		if freeY {
			if rp.Y, err = freeScale(c.Y); err != nil {
				return render.Chart{}, err
			}
		}
		c.Panels = append(c.Panels, rp)
	}
	return c, nil
}

// outermost reports whether no other panel lies past p in the given direction.
// Those are the panels that write a shared axis.
func outermost(panels []facet.Panel, p facet.Panel, beyond func(a, b facet.Panel) bool) bool {
	for _, q := range panels {
		if beyond(p, q) {
			return false
		}
	}
	return true
}

// below and leftOf are the two directions that matter. A shared X axis is
// written by the last panel in its column — not by the bottom row, because a
// wrapped facet whose final row is short would leave the panels above the gap
// unlabelled. A shared Y axis is written by the first panel in its row.
func below(a, b facet.Panel) bool  { return a.Col == b.Col && b.Row > a.Row }
func leftOf(a, b facet.Panel) bool { return a.Row == b.Row && b.Col < a.Col }

// freeScale copies a scale so that one panel's data cannot move another
// panel's axis.
func freeScale(s scale.Scale) (scale.Scale, error) {
	c, ok := s.(scale.Cloner)
	if !ok {
		return nil, fmt.Errorf("refract: %T cannot be given to a free facet axis: it does not implement scale.Cloner", s)
	}
	return c.Clone(), nil
}

func (p *Plot) scaleX() scale.Scale {
	if p.x == nil {
		p.x = scale.Linear(scale.Nice())
	}
	return p.x
}

func (p *Plot) scaleY() scale.Scale {
	if p.y == nil {
		p.y = scale.Linear(scale.Nice())
	}
	return p.y
}

// showLegend is the default rule: a legend appears once there is more than one
// thing to tell apart.
//
// That is more than one layer, or one layer that draws more than one series —
// a long table split by [geom.GroupBy] is N series inside one layer, and a
// chart of five unlabelled stacked bands is exactly the chart that needs the
// legend most. The question is asked of the configuration rather than of the
// trained layer, because it decides the layout and layout runs first.
func (p *Plot) showLegend() bool {
	if p.legendSet {
		return p.legend
	}
	if len(p.layers) > 1 {
		return true
	}
	for _, g := range p.layers {
		if d, ok := geom.Describe(g); ok && d.Group != "" {
			return true
		}
	}
	return false
}

// SVG returns a target writing an SVG document to the named file.
//
// This is the zero-dependency path: it uses the built-in emitter in
// backend/svg and links no rendering engine.
func SVG(path string, opts ...svg.Option) Target { return svg.File(path, opts...) }

// SVGWriter returns a target writing an SVG document to w.
func SVGWriter(w io.Writer, opts ...svg.Option) Target { return svg.Writer(w, opts...) }

// PDF returns a target writing a PDF document to the named file.
//
// Like [SVG], this is a zero-dependency path: the emitter is in backend/pdf
// and uses nothing but the standard library. The page is one PDF point per
// device-independent pixel, so a chart sized 800x500 is an 800x500pt page.
func PDF(path string, opts ...pdf.Option) Target { return pdf.File(path, opts...) }

// PDFWriter returns a target writing a PDF document to w.
func PDFWriter(w io.Writer, opts ...pdf.Option) Target { return pdf.Writer(w, opts...) }
