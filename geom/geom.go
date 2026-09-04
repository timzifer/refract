// Package geom holds the visual marks a chart is made of.
//
// A geom knows what its shape is and never how it is rendered: it reads
// columns, asks the scales where values go, and emits IR. It has no reference
// to a backend, no knowledge of SVG or raster, and no opinion about layout
// beyond the rectangle it is given.
package geom

import (
	"errors"
	"fmt"
	"math"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// Missing is the policy for NaN and infinite values in a column.
type Missing uint8

// The missing-data policies. Gap is the default: a hole in the data should
// look like a hole, not like a straight line someone might read as real.
const (
	Gap Missing = iota
	Interpolate
	Error
)

// Frame is everything a geom needs to turn data into IR.
type Frame struct {
	// Area is the plot rectangle in device space. Scales already map into it.
	Area ir.Rect
	// X and Y are the trained, ranged scales.
	X, Y scale.Scale
	// Theme supplies defaults the geom did not override.
	Theme theme.Theme
	// Index is the geom's position among the chart's layers, used to pick a
	// default colour from the palette.
	Index int
}

// SwatchKind is how a legend entry draws its sample.
type SwatchKind uint8

// The swatch kinds.
const (
	SwatchLine SwatchKind = iota
	SwatchMarker
	SwatchBox
)

// LegendEntry is a geom's contribution to the legend.
type LegendEntry struct {
	Label  string
	Color  ir.Color
	Kind   SwatchKind
	Marker ir.Marker
	Dash   []float32
	Width  float32
}

// Geom is a layer of marks.
type Geom interface {
	// Train feeds the geom's data into the scales so they can establish their
	// domains. It runs before layout, because layout needs tick labels and
	// tick labels need a domain.
	Train(x, y scale.Scale) error

	// Build emits the geom's marks into b.
	Build(b ir.Backend, f Frame) error

	// Legend returns the entry this geom contributes, or ok == false if it
	// should not appear in the legend.
	Legend(f Frame) (LegendEntry, bool)
}

// Option configures a geom. Options are shared across geom constructors: an
// option a given geom has no use for is accepted and ignored, which keeps the
// API one namespace instead of six.
type Option func(*config)

type config struct {
	xcol, ycol string
	label      string

	color    *ir.Color
	width    float32
	dash     []float32
	tension  float64
	missing  Missing
	marker   ir.Marker
	size     float32
	barWidth float64
	baseline float64
	fill     *ir.Color
}

// X selects the column mapped to the horizontal axis.
func X(col string) Option { return func(c *config) { c.xcol = col } }

// Y selects the column mapped to the vertical axis.
func Y(col string) Option { return func(c *config) { c.ycol = col } }

// Color sets the mark colour, overriding the palette.
func Color(col ir.Color) Option { return func(c *config) { c.color = &col } }

// Fill sets a fill colour separately from the stroke colour, for geoms that
// have both.
func Fill(col ir.Color) Option { return func(c *config) { c.fill = &col } }

// Width sets the stroke width in device units.
func Width(w float32) Option { return func(c *config) { c.width = w } }

// Dash sets a dash pattern in device units. Dashing a series is redundant
// encoding: it keeps the chart readable in greyscale and for colourblind
// readers.
func Dash(pattern ...float32) Option { return func(c *config) { c.dash = pattern } }

// Tension smooths a line. 0 (the default) is a plain polyline; values in
// (0, 1] progressively round the corners using a Catmull-Rom spline.
func Tension(t float64) Option { return func(c *config) { c.tension = t } }

// OnMissing sets the NaN/Inf policy.
func OnMissing(m Missing) Option { return func(c *config) { c.missing = m } }

// Label names the series in the legend. It defaults to the Y column's name.
func Label(s string) Option { return func(c *config) { c.label = s } }

// Shape sets the marker shape for scatter geoms.
func Shape(m ir.Marker) Option { return func(c *config) { c.marker = m } }

// Size sets the marker diameter in device units.
func Size(s float32) Option { return func(c *config) { c.size = s } }

// BarWidth sets bar width as a fraction of the spacing between adjacent bars,
// in (0, 1]. The default is 0.8.
func BarWidth(f float64) Option { return func(c *config) { c.barWidth = f } }

// Baseline sets the value bars grow from. The default is 0.
func Baseline(v float64) Option { return func(c *config) { c.baseline = v } }

func newConfig(opts []Option) config {
	c := config{barWidth: 0.8}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// colorFor resolves the mark colour: explicit if set, otherwise the palette
// entry for this layer.
func (c config) colorFor(f Frame) ir.Color {
	if c.color != nil {
		return *c.color
	}
	pal := f.Theme.Palette
	if len(pal) == 0 {
		pal = theme.Light.Palette
	}
	return pal.At(f.Index)
}

func (c config) labelFor() string {
	if c.label != "" {
		return c.label
	}
	return c.ycol
}

// series is a resolved pair of columns, already converted to float64.
type series struct {
	x, y []float64
}

// ErrNoColumn reports a column named by an option that the source does not
// have. It is returned rather than panicking because the column name usually
// comes from user input or a config file.
var ErrNoColumn = errors.New("refract/geom: column not found")

// resolve reads the configured columns out of src.
func resolve(src data.Source, c config) (series, error) {
	if src == nil {
		return series{}, errors.New("refract/geom: nil data source")
	}
	x, err := column(src, c.xcol)
	if err != nil {
		return series{}, err
	}
	y, err := column(src, c.ycol)
	if err != nil {
		return series{}, err
	}
	if len(x) != len(y) {
		return series{}, fmt.Errorf("refract/geom: columns %q and %q differ in length (%d vs %d)", c.xcol, c.ycol, len(x), len(y))
	}
	return series{x: x, y: y}, nil
}

// column reads one column as float64, converting a time column to Unix
// nanoseconds so that a time scale sees the same numeric domain as any other.
func column(src data.Source, name string) ([]float64, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: no column selected (use geom.X/geom.Y)", ErrNoColumn)
	}
	if v, ok := src.Float64Column(name); ok {
		return v, nil
	}
	if t, ok := src.TimeColumn(name); ok {
		out := make([]float64, len(t))
		for i, tv := range t {
			out[i] = scale.Nanos(tv)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrNoColumn, name)
}

// finite reports whether v can be plotted.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// trainFinite feeds only plottable values into a scale, so one NaN does not
// blow the domain out to the whole real line.
func trainFinite(s scale.Scale, vs []float64) {
	for _, v := range vs {
		if finite(v) {
			s.Train(v)
		}
	}
}

// checkMissing enforces the Error policy.
func (s series) checkMissing(c config) error {
	if c.missing != Error {
		return nil
	}
	for i := range s.x {
		if !finite(s.x[i]) || !finite(s.y[i]) {
			return fmt.Errorf("refract/geom: missing value at row %d and OnMissing(Error) is set", i)
		}
	}
	return nil
}
