// Package refract turns one declarative chart specification into any output
// you need — SVG, PDF and a browser canvas today, raster through one more
// module, GPU and a native window through later ones — from the same model,
// with the same geometry.
//
// # This library is now github.com/timzifer/figure
//
// Development continues under a new name and a new import path. v1.8.0 is the
// last release here; it is v1.7.0 plus this notice, and nothing else changed.
//
//	go get github.com/timzifer/figure
//
// Everything under this path keeps working and keeps its tags. It receives no
// fixes and no features.
//
// Migrating is an import-path change and a compiler pass. Beyond the path,
// what moved is the shape of the seams a caller implements — Geom.Train,
// Observer.Panel and Layer, Coord.Frame and Furniture, Target.Open,
// Typesetter.Typeset, Rows.Marks and Scale.Ticks each take one growable struct
// now instead of positional arguments — and data.Source, which answers one
// Column call rather than three typed ones. The release notes for figure
// v0.8.0 carry the table, and the compiler finds every site:
//
//	https://github.com/timzifer/figure/releases/tag/v0.8.0
//
// The reason for the rename is written down rather than left to guess: the old
// name argued a thesis about prisms that the library had outgrown, and the
// three seams a third dimension breaks were cheaper to correct under a new
// import path than under a major version that would have carried a /v2 suffix
// for the rest of the library's life.
//
// # What it is
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
// cover lines, scatters, bars, areas, steps, boxplots and rects. A mark's
// colour can come from the data through [scale.Sequential] or [scale.Diverging]
// and [geom.ColorBy], which contributes a colourbar beside the plot, and its
// size through [scale.Size] and [geom.SizeBy], which contributes a key of
// sample marks — the bubble chart.
//
// # Distributions
//
// [geom.Histogram], [geom.Violin], [geom.Ridgeline], [geom.Hexbin],
// [geom.Beeswarm], [geom.ECDF] and [geom.Trend] summarise a column rather than
// plotting it. Each is a pure function in package stat with a determinism test,
// and each trains its axis on the summary: a histogram's Y axis holds counts
// that are nowhere in the data.
//
//	p.X(scale.Ordinal())
//	p.Add(geom.Violin(src, geom.X("service"), geom.Y("latency"),
//	    geom.GroupBy("region")))
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
// Closed. v1.8.0 is the last release under this path; the API is frozen where
// v1.0.0 froze it and stays that way, because nothing further will be built
// here. See github.com/timzifer/figure for the library that continues.
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

	title   string
	xTitle  string
	yTitle  string
	y2Title string
	x2Title string

	x, y   scale.Scale
	y2, x2 scale.Scale
	coord  coordpkg.Coord
	layers []geom.Geom

	// locale is the language every axis of this plot writes its labels in,
	// and nil for English. It is a plot-level option rather than a scale one
	// because a chart is in one language: setting it per scale means saying
	// it once per axis and once per track, and forgetting it somewhere is a
	// chart with two languages in it. See [Locale].
	locale *scale.Locale

	facet *facet.Spec

	// tracks are the bands at the panel's edges, in the order they were
	// added. See [Plot.Track].
	tracks []*Track

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

	overlay render.Overlay
	hidden  []bool

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
//	p := refract.New(refract.Coord(coord.Donut(0.45)))
//	p.X(scale.Linear())
//	p.Y(scale.Linear())
//	p.Add(geom.Bar(src, geom.X("one"), geom.Y("share"), geom.GroupBy("browser")))
//
// [coord.Pie] and [coord.Donut] are that recipe named; neither scale is niced,
// because a pie's ring closes on the stacked total. A slice can also name its
// own inner and outer radius with [geom.X] and [geom.X2] and be broken out of
// the ring with [geom.ExplodeBy], neither of which is a new mark.
//
// [coord.Smith] is the third one, and the same idea over a different map: it
// reads the pair as a normalised impedance and carries it through
// Γ = (z − 1)/(z + 1) onto the unit disc, which is the Smith chart. Its grid is
// the two axes' own ticks — a circle per resistance, an arc per reactance — so
// again the mark is one that was already there:
//
//	p := refract.New(refract.Coord(coord.Smith()))
//	p.X(scale.Linear(scale.Domain(0, 50), scale.TickValues(0, 0.2, 0.5, 1, 2, 5)))
//	p.Y(scale.Linear(scale.Domain(-50, 50), scale.TickValues(-5, -1, -0.5, 0.5, 1, 5)))
//	p.Add(geom.Line(sweep, geom.X("r"), geom.Y("x")))
//
// A coord belongs to the chart rather than to a panel, so the panels of a
// facet all share it.
func Coord(c coordpkg.Coord) Option { return func(p *Plot) { p.coord = c } }

// Locale sets the language every axis of this plot writes its tick labels in:
// the decimal and group separators of a number, the percent sign, and the
// month and weekday names of a time axis.
//
// It reaches the scales through [scale.Localizer], which every scale in the
// scale package implements except the ordinal one — an ordinal axis labels its
// ticks with the caller's own categories, and translating those would be
// inventing data. A scale from somewhere else that does not implement it is
// left alone rather than refused.
//
// It is a plot option rather than a scale one because a chart is in one
// language: setting it per scale means saying it once per axis and once per
// track, and forgetting it somewhere is a chart with two languages in it.
//
// The default is [scale.English], which is what every chart drew before this
// option existed.
//
//	p := refract.New(refract.Locale(scale.LocaleDE))
//	p.X(scale.Time()).Y(scale.Linear(scale.NumberFormat("#,.1")))
//	// → "1.234,5" on the Y axis and "Mär 2026" on the X one
func Locale(l *scale.Locale) Option { return func(p *Plot) { p.locale = l } }

// Theme sets the visual tokens. The default is [theme.Light].
func Theme(t themepkg.Theme) Option { return func(p *Plot) { p.theme = t } }

// Title sets the chart title.
func Title(s string) Option { return func(p *Plot) { p.title = s } }

// XTitle sets the horizontal axis title.
func XTitle(s string) Option { return func(p *Plot) { p.xTitle = s } }

// YTitle sets the vertical axis title.
func YTitle(s string) Option { return func(p *Plot) { p.yTitle = s } }

// Y2Title sets the title of the secondary vertical axis, written down the
// chart's right-hand side. It is ignored by a chart with no [Plot.Y2].
func Y2Title(s string) Option { return func(p *Plot) { p.y2Title = s } }

// X2Title sets the title of the secondary horizontal axis, written along the
// chart's top. It is ignored by a chart with no [Plot.X2].
func X2Title(s string) Option { return func(p *Plot) { p.x2Title = s } }

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

// Y2 sets the chart's secondary vertical axis: a second scale, drawn down the
// right-hand side, read by the layers that asked for it with [geom.OnY2].
//
// It is the chart of two quantities in different units — revenue as bars
// against the left axis, margin as a percentage line against the right — and
// it is one chart with two axes rather than two charts overlaid, which is why
// the binding is on the layer and the scale is on the plot.
//
// The second axis draws **no grid lines**. Two ladders of horizontal rules at
// different values are a moiré rather than a reading, and which of the two a
// line belongs to is unanswerable by looking; the grid stays the primary
// axis's. See [ADR 0037](docs/adr/0037-secondary-axis.md).
//
// A chart with no layer on it draws the axis anyway, because an axis somebody
// asked for is a statement about the chart even where nothing reaches it yet —
// a live chart whose second series has not arrived is the case.
func (p *Plot) Y2(s scale.Scale) *Plot { p.y2 = s; return p }

// X2 sets the chart's secondary horizontal axis: a second scale, drawn along
// the top, read by the layers that asked for it with [geom.OnX2].
//
// It is [Plot.Y2] turned a quarter turn and everything said there holds,
// including that the second axis draws no grid lines. What it is *for* is
// different: two series measured over different extents of the same thing —
// a run indexed by cycle beside one indexed by elapsed time, a spectrum read
// in wavelength against the same spectrum in wavenumber, a backlog by date
// against a backlog by sprint.
//
// The two directions are independent. A layer may name [geom.OnX2] and
// [geom.OnY2] together, and then it reads the top axis and the right one.
func (p *Plot) X2(s scale.Scale) *Plot { p.x2 = s; return p }

// Add appends layers, drawn in the order given.
func (p *Plot) Add(gs ...geom.Geom) *Plot { p.layers = append(p.layers, gs...); return p }

// SetLayers replaces the plot's layers with the ones given, drawn in the order
// given. Passing none leaves a plot with no layers, which [Plot.Render]
// refuses with [ErrNoLayers].
//
// It is [Plot.Add]'s counterpart and exists for the same caller: one reacting
// to something the reader did. A chart that gains a highlight layer on every
// hover and can never lose one accumulates a layer per pointer move, so a plot
// that can be added to has to be a plot that can be set. Building a fresh Plot
// each time is the alternative and a worse one — it discards the scales, and
// with them the zoom the reader established.
//
// Layers already drawn are unaffected until the chart is resolved again:
// [Live.Rebuild] is what picks this up, and it keeps the view.
//
// The slice is copied, so the caller may reuse it.
func (p *Plot) SetLayers(gs ...geom.Geom) *Plot {
	p.layers = append(p.layers[:0:0], gs...)
	return p
}

// Layers reports the plot's layers, in drawing order. The slice is a copy; the
// layers in it are not.
//
// It is what a caller reaching for [Plot.SetLayers] needs first: keeping the
// ones that were there and replacing the rest means being able to see them.
func (p *Plot) Layers() []geom.Geom { return append([]geom.Geom(nil), p.layers...) }

// Overlay installs something to paint over the finished chart — a crosshair, a
// tooltip, a brush rectangle. Passing nil removes it. See [Overlay].
//
// It is on the plot as well as on [Live] so that the two agree about what a
// chart is: an overlay that only existed on a live surface would make
// [Plot.Render] and [Live.Draw] draw different pictures of the same model, and
// exporting what a reader is looking at — the chart with its crosshair where
// they left it — would be impossible from the model alone.
//
// [Live.Overlay] overrides this for one surface. A plot that names one and a
// Live that names another draws the Live's, because the Live is the thing with
// a pointer over it.
func (p *Plot) Overlay(o Overlay) *Plot { p.overlay = o; return p }

// HideLayer turns a layer off, or back on, by its index among the plot's
// layers. A hidden layer is not drawn, still trains its scales, and still
// appears in the legend, dimmed.
//
// It is on the plot as well as on [Live] for the reason [Plot.Overlay] is:
// otherwise [Plot.Render] and [Live.Draw] would disagree about what a chart
// is, and exporting a chart with a series put away would be impossible from
// the model alone. [Live.Hide] is the one a legend click calls, and it starts
// from whatever the plot said.
//
// An index outside the plot's layers is ignored.
func (p *Plot) HideLayer(layer int, hide bool) *Plot {
	if layer < 0 || layer >= len(p.layers) {
		return p
	}
	for len(p.hidden) <= layer {
		p.hidden = append(p.hidden, false)
	}
	p.hidden[layer] = hide
	return p
}

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

// ErrTrackWithFacet reports a plot that has both a track and a facet.
//
// A facet owns the grid's rows and columns — it is what decides how many there
// are and what each one means — and a track needs a row of that grid to live
// in. A band spanning a facet is a different feature with different questions
// to answer, so this is refused rather than guessed at.
var ErrTrackWithFacet = errors.New("refract: a plot cannot have both a track and a facet")

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
// chart builds the render description and puts every axis it holds into the
// plot's language.
//
// Localising here rather than in [Plot.X] is what makes the option reach a
// scale the plot was given afterwards, a track's own scale and a free facet
// axis's clone alike — they are all in the description by the time this runs,
// and the walk is one place rather than four call sites that can drift.
//
// It is safe against the parallel path because it happens before it: the
// panels are built after this returns, and a locale is read from then on and
// never written.
func (p *Plot) chart() (render.Chart, error) {
	c, err := p.describe()
	if err != nil {
		return render.Chart{}, err
	}
	if p.locale != nil {
		scale.Localize(c.X, p.locale)
		scale.Localize(c.Y, p.locale)
		scale.Localize(c.Y2, p.locale)
		scale.Localize(c.X2, p.locale)
		for i := range c.Panels {
			scale.Localize(c.Panels[i].X, p.locale)
			scale.Localize(c.Panels[i].Y, p.locale)
		}
	}
	return c, nil
}

func (p *Plot) describe() (render.Chart, error) {
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
		Y2:          p.y2,
		X2:          p.x2,
		Y2Title:     p.y2Title,
		X2Title:     p.x2Title,
		Coord:       p.coord,
		Layers:      p.layers,
		ShowLegend:  p.showLegend(),
		Description: p.Description(),
		Math:        p.math,
		Serial:      p.serial,
		Overlay:     p.overlay,
		// Copied: a Live mutates its chart's slice when a reader hides a
		// series, and that must not reach back into the plot every other Live
		// of it is built from.
		Hidden: append([]bool(nil), p.hidden...),
	}
	if len(p.tracks) > 0 {
		if p.facet != nil {
			return render.Chart{}, ErrTrackWithFacet
		}
		return p.tracked(c), nil
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
			Y2:         c.Y2,
			X2:         c.X2,
			// A shared axis is written once, at the edge of the grid — which
			// is the last panel in the column, not the last row: a wrapped
			// facet whose final row is short would otherwise leave the
			// panels above the gap with no labels at all. A free axis is
			// written on every panel, because it is a different axis each
			// time and a reader who assumed otherwise would misread every
			// panel but one.
			ShowX: freeX || outermost(panels, fp, below),
			ShowY: freeY || outermost(panels, fp, leftOf),
			// The second axis is written at the *right* edge of the grid,
			// which is the mirror of where the first one is written and the
			// same rule: a shared axis belongs at the outside, and a free one
			// is a different axis in every panel and has to be written in each.
			ShowY2: c.Y2 != nil && (freeY || outermost(panels, fp, rightOf)),
			ShowX2: c.X2 != nil && (freeX || outermost(panels, fp, above)),
		}
		if freeX {
			if rp.X, err = freeScale(c.X); err != nil {
				return render.Chart{}, err
			}
			if c.X2 != nil {
				if rp.X2, err = freeScale(c.X2); err != nil {
					return render.Chart{}, err
				}
			}
		}
		if freeY {
			if rp.Y, err = freeScale(c.Y); err != nil {
				return render.Chart{}, err
			}
			if c.Y2 != nil {
				// A free Y axis frees both of them. One panel's data must not
				// move another panel's axis, and that is as true of the second
				// as of the first — a shared second axis under a free first
				// one would be half a free facet, which is not a reading.
				if rp.Y2, err = freeScale(c.Y2); err != nil {
					return render.Chart{}, err
				}
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

// below, above, leftOf and rightOf are the four directions that matter. A shared X axis is
// written by the last panel in its column — not by the bottom row, because a
// wrapped facet whose final row is short would leave the panels above the gap
// unlabelled. A shared Y axis is written by the first panel in its row.
func below(a, b facet.Panel) bool   { return a.Col == b.Col && b.Row > a.Row }
func above(a, b facet.Panel) bool   { return a.Col == b.Col && b.Row < a.Row }
func leftOf(a, b facet.Panel) bool  { return a.Row == b.Row && b.Col < a.Col }
func rightOf(a, b facet.Panel) bool { return a.Row == b.Row && b.Col > a.Col }

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
		d, ok := geom.Describe(g)
		if !ok {
			continue
		}
		// A relational layer is the same case under a different name: its
		// nodes are its series, and nothing declares them but the edge table,
		// so a chart of one sankey needs the legend for exactly the reason a
		// chart of one stack does.
		if d.Group != "" || d.From != "" || d.ID != "" {
			return true
		}
		// And so is a layer painted per mark from a *discrete* colour scale.
		// Its categories are series in everything but name — the colours are
		// the only thing that says which mark is which state — and nothing
		// but the legend names them. A continuous or classed scale is not
		// this case: it contributes a colourbar, which names itself.
		if d.ColorCol != "" && d.ColorScale != nil {
			if _, discrete := scale.Discrete(d.ColorScale); discrete {
				return true
			}
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
