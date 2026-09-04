// Package refract turns one declarative chart specification into any output
// you need — SVG today, raster, PDF, GPU and browser through additional
// backends — from the same model, with the same geometry.
//
// The core module is pure Go and depends on nothing but the standard library.
// The built-in SVG backend needs no rendering engine and no font stack, so a
// server that only wants a chart as SVG links nothing native and nothing
// young. Raster output lives in a separate module,
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
// [geom.ColorBy].
//
// # Status
//
// Pre-alpha. Every release below v1.0.0 may contain breaking changes without
// a deprecation cycle. See CONCEPT.md for the design and the roadmap.
package refract

import (
	"errors"
	"io"

	"github.com/timzifer/refract/backend/svg"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
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
	layers []geom.Geom

	legend    bool
	legendSet bool
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
	return p
}

// X sets the horizontal scale. The default is [scale.Linear] with nicing.
func (p *Plot) X(s scale.Scale) *Plot { p.x = s; return p }

// Y sets the vertical scale. The default is [scale.Linear] with nicing.
func (p *Plot) Y(s scale.Scale) *Plot { p.y = s; return p }

// Add appends layers, drawn in the order given.
func (p *Plot) Add(gs ...geom.Geom) *Plot { p.layers = append(p.layers, gs...); return p }

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

	c := render.Chart{
		Width:      p.width,
		Height:     p.height,
		DPR:        p.dpr,
		Theme:      p.theme,
		Title:      p.title,
		XTitle:     p.xTitle,
		YTitle:     p.yTitle,
		X:          p.scaleX(),
		Y:          p.scaleY(),
		Layers:     p.layers,
		ShowLegend: p.showLegend(),
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

func (p *Plot) showLegend() bool {
	if p.legendSet {
		return p.legend
	}
	return len(p.layers) > 1
}

// SVG returns a target writing an SVG document to the named file.
//
// This is the zero-dependency path: it uses the built-in emitter in
// backend/svg and links no rendering engine.
func SVG(path string, opts ...svg.Option) Target { return svg.File(path, opts...) }

// SVGWriter returns a target writing an SVG document to w.
func SVGWriter(w io.Writer, opts ...svg.Option) Target { return svg.Writer(w, opts...) }
