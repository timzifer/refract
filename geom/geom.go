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
	"sort"
	"time"

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
	y2col      string
	label      string

	color      *ir.Color
	width      float32
	dash       []float32
	tension    float64
	missing    Missing
	marker     ir.Marker
	size       float32
	barWidth   float64
	baseline   float64
	fill       *ir.Color
	opacity    float64
	steps      StepPos
	colorCol   string
	colorScale scale.ColorScale
	whisker    float64
	outliers   bool
	decimate   Decimation
	budget     int
	cellSize   float64

	dashSet  bool
	extend   bool
	fontSize float64
	halign   ir.HAlign
	valign   ir.VAlign
	rotation float64
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
func Dash(pattern ...float32) Option {
	return func(c *config) { c.dash, c.dashSet = pattern, true }
}

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

// Baseline sets the value bars and areas grow from. The default is 0.
func Baseline(v float64) Option { return func(c *config) { c.baseline = v } }

// Y2 selects a second Y column, turning an area into a band between the two
// series rather than between one series and a baseline. It is how a confidence
// interval or a min/max envelope is drawn.
func Y2(col string) Option { return func(c *config) { c.y2col = col } }

// Opacity scales the fill alpha, in [0, 1]. The default is 1 for an explicit
// [Fill] colour and 0.25 for an area that takes its colour from the palette —
// a filled band has to sit behind the lines it belongs to.
func Opacity(f float64) Option { return func(c *config) { c.opacity = f } }

// StepPos is where a step geom changes value between two rows.
type StepPos uint8

// The step positions. StepPost is the default: the value holds from each row
// until the next one, which is what a sampled signal or a state over time
// actually did.
const (
	StepPost StepPos = iota
	StepPre
	StepMid
)

// Steps sets where a [Step] geom changes value.
func Steps(where StepPos) Option { return func(c *config) { c.steps = where } }

// ColorBy maps a numeric column through a colour scale, giving every mark its
// own colour. It applies to [Scatter] and [Bar]; geoms whose mark is one
// connected shape ignore it.
//
// A layer coloured this way contributes a colourbar rather than a legend
// entry — a single swatch cannot represent a continuum. See [ColorGuide].
func ColorBy(col string, s scale.ColorScale) Option {
	return func(c *config) { c.colorCol, c.colorScale = col, s }
}

// Whisker sets how far a boxplot whisker reaches, as a multiple of the
// interquartile range. The default is 1.5, Tukey's original choice.
func Whisker(k float64) Option { return func(c *config) { c.whisker = k } }

// Outliers turns the individual points beyond the whiskers on or off. They are
// on by default: a boxplot that hides them is a boxplot that hides exactly the
// rows a reader opened the chart to find.
func Outliers(show bool) Option { return func(c *config) { c.outliers = show } }

// Align sets how a text annotation sits about its position. The default is
// the run's start on the point, on the baseline.
func Align(h ir.HAlign, v ir.VAlign) Option {
	return func(c *config) { c.halign, c.valign = h, v }
}

// FontSize sets the type size of a text annotation in device units. The
// default is the theme's label size.
func FontSize(pt float64) Option { return func(c *config) { c.fontSize = pt } }

// Rotate turns a text annotation about its anchor, in radians clockwise.
func Rotate(radians float64) Option { return func(c *config) { c.rotation = radians } }

// Extend controls whether an annotation widens the axis domain to include
// itself. It is on by default: a threshold line the chart does not reach is
// still worth seeing, because "we are nowhere near the limit" is the answer
// the reader came for. Turn it off for an annotation that should appear only
// when the data reaches it.
func Extend(on bool) Option { return func(c *config) { c.extend = on } }

func newConfig(opts []Option) config {
	c := config{barWidth: 0.8, whisker: 1.5, outliers: true, opacity: -1, extend: true}
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

// series is a resolved set of columns, already converted to float64.
//
// y2 and c are optional: y2 carries the second bound of a band, c the values a
// colour scale is read from. Both are nil when the geom was not given one.
type series struct {
	x, y  []float64
	y2, c []float64
}

// ErrNoColumn reports a column named by an option that the source does not
// have. It is returned rather than panicking because the column name usually
// comes from user input or a config file.
var ErrNoColumn = errors.New("refract/geom: column not found")

// ErrCategorical reports a text column mapped onto an axis that has no
// position for a name.
var ErrCategorical = errors.New("refract/geom: categorical column on a continuous scale")

// resolve reads the configured columns out of src.
//
// The scales are passed in because reading a column is not independent of the
// axis it feeds: a text column only has numbers at all once a categorical
// scale has assigned them, and doing that anywhere else would mean two layers
// on one axis disagreeing about which category is which.
func resolve(src data.Source, c config, x, y scale.Scale) (series, error) {
	if src == nil {
		return series{}, errors.New("refract/geom: nil data source")
	}
	xs, err := column(src, c.xcol, x)
	if err != nil {
		return series{}, err
	}
	ys, err := column(src, c.ycol, y)
	if err != nil {
		return series{}, err
	}
	if len(xs) != len(ys) {
		return series{}, fmt.Errorf("refract/geom: columns %q and %q differ in length (%d vs %d)", c.xcol, c.ycol, len(xs), len(ys))
	}
	s := series{x: xs, y: ys}
	if c.y2col != "" {
		v, err := column(src, c.y2col, y)
		if err != nil {
			return series{}, err
		}
		if len(v) != len(xs) {
			return series{}, fmt.Errorf("refract/geom: columns %q and %q differ in length (%d vs %d)", c.xcol, c.y2col, len(xs), len(v))
		}
		s.y2 = v
	}
	if c.colorCol != "" {
		if _, text := src.StringColumn(c.colorCol); text {
			return series{}, fmt.Errorf("%w: column %q holds category names and a colour scale reads numbers", ErrCategorical, c.colorCol)
		}
		v, err := column(src, c.colorCol, nil)
		if err != nil {
			return series{}, err
		}
		if len(v) != len(xs) {
			return series{}, fmt.Errorf("refract/geom: columns %q and %q differ in length (%d vs %d)", c.xcol, c.colorCol, len(xs), len(v))
		}
		s.c = v
	}
	return s, nil
}

// column reads one column as float64 for the axis it feeds.
//
// A numeric column on a continuous scale is returned as it lies — that is the
// zero-copy path the data layer exists for. A time column becomes Unix
// nanoseconds, so a time scale sees the same numeric domain as any other. A
// text column is encoded through the scale, which must be categorical.
//
// A numeric or time column on a *categorical* scale is encoded too, one
// category per distinct formatted value: asking for an ordinal axis over
// numbers means asking for equally spaced slots rather than a numeric line.
func column(src data.Source, name string, s scale.Scale) ([]float64, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: no column selected (use geom.X/geom.Y)", ErrNoColumn)
	}
	cat, _ := s.(scale.Categorical)

	if v, ok := src.StringColumn(name); ok {
		if cat == nil {
			return nil, fmt.Errorf("%w: column %q holds category names; give that axis a scale.Ordinal", ErrCategorical, name)
		}
		return encode(cat, v, func(l string) string { return l }), nil
	}
	if v, ok := src.Float64Column(name); ok {
		if cat == nil {
			return v, nil
		}
		return encode(cat, v, data.FormatNumber), nil
	}
	if t, ok := src.TimeColumn(name); ok {
		if cat != nil {
			return encode(cat, t, func(tv time.Time) string { return tv.Format(time.RFC3339) }), nil
		}
		out := make([]float64, len(t))
		for i, tv := range t {
			out[i] = scale.Nanos(tv)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrNoColumn, name)
}

// encode maps a column through a categorical scale, naming each value with
// label.
func encode[T any](cat scale.Categorical, vs []T, label func(T) string) []float64 {
	out := make([]float64, len(vs))
	for i, v := range vs {
		out[i] = cat.Encode(label(v))
	}
	return out
}

// finite reports whether v can be plotted.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// defined reports whether s has a position for v. A scale that excludes part
// of the real line — a log scale, an ordinal one — says so through
// [scale.Definite]; every other scale takes any finite value.
func defined(s scale.Scale, v float64) bool {
	if !finite(v) {
		return false
	}
	if d, ok := s.(scale.Definite); ok {
		return d.Defined(v)
	}
	return true
}

// trainColumn feeds a whole column into a scale in one call.
//
// [scale.Scale.Train] is documented to ignore NaN and infinities, and every
// scale here does — so filtering row by row here was doing the scale's job
// twice, and doing it through a variadic interface method allocated the
// argument slice once per row. On a million-row column that was a million
// allocations to establish two numbers.
func trainColumn(s scale.Scale, vs []float64) { s.Train(vs...) }

// plottable marks the rows a geom can draw: finite values that both scales
// have a position for.
//
// It is computed once per build rather than tested per use, because the
// answer feeds three different traversals — segmenting, interpolating and
// projecting — and they must agree on it exactly.
func (s series) plottable(x, y scale.Scale) []bool {
	ok := make([]bool, len(s.x))
	for i := range s.x {
		ok[i] = defined(x, s.x[i]) && defined(y, s.y[i])
		if ok[i] && s.y2 != nil {
			ok[i] = defined(y, s.y2[i])
		}
	}
	return ok
}

// checkMissing enforces the Error policy. It runs against the scales, so a
// negative reading on a log axis is caught by the same policy that catches a
// NaN — from the chart's point of view they are the same failure.
func (s series) checkMissing(c config, x, y scale.Scale) error {
	if c.missing != Error {
		return nil
	}
	for i, ok := range s.plottable(x, y) {
		if !ok {
			return fmt.Errorf("refract/geom: row %d has no position on the scales and OnMissing(Error) is set", i)
		}
	}
	return nil
}

// fillFor resolves the interior colour of a filled geom.
//
// An explicit [Fill] is taken at face value — a caller who names a colour has
// already decided how solid it should be. A fill inherited from the palette is
// faded to defaultOpacity instead, because a geom that fills the plot area has
// to sit behind the lines and points it belongs to rather than bury them.
func (c config) fillFor(f Frame, defaultOpacity float64) ir.Color {
	base := c.colorFor(f)
	op := defaultOpacity
	if c.fill != nil {
		base, op = *c.fill, 1
	}
	if c.opacity >= 0 {
		op = clamp01(c.opacity)
	}
	return ir.Fade(base, op)
}

// colorRun is a set of marks that share a colour.
type colorRun struct {
	color ir.Color
	pts   []ir.Point
}

// groupByColor batches marks by colour.
//
// The IR carries one style per drawing call and does not have a per-vertex
// colour channel; adding one would touch every backend for the sake of two
// geoms. Grouping instead costs one call per distinct colour, which for a
// continuous ramp over real data is far fewer calls than there are points, and
// for a handful of categories is a handful. See docs/adr/0007.
//
// Groups come out in order of first appearance, so a render is reproducible.
// Each group's point buffer is kept between frames — which is why the runs are
// held full-length in the scratch and only the used prefix is returned.
func (sc *scratch) groupByColor(pts []ir.Point, cols []ir.Color) []colorRun {
	if sc.at == nil {
		sc.at = make(map[ir.Color]int, 8)
	}
	clear(sc.at)
	n := 0
	for i, p := range pts {
		c := cols[i]
		j, ok := sc.at[c]
		if !ok {
			j = n
			n++
			if j == len(sc.runs) {
				sc.runs = append(sc.runs, colorRun{})
			}
			sc.runs[j].color = c
			sc.runs[j].pts = sc.runs[j].pts[:0]
			sc.at[c] = j
		}
		sc.runs[j].pts = append(sc.runs[j].pts, p)
	}
	return sc.runs[:n]
}

// trainColors feeds the colour column into the colour scale. It is separate
// from training the positional scales because a colour scale is not a Scale:
// it maps to paint, not to a position.
func (c config) trainColors(s series) {
	if c.colorScale == nil || s.c == nil {
		return
	}
	c.colorScale.Train(s.c...)
}

// colorsFor resolves the per-mark colours for the rows in idx, or nil if this
// layer is a single colour. It writes into the scratch's buffer, so a chart
// redrawn every frame recolours the same memory.
func (sc *scratch) colorsFor(c config, s series, idx []int) []ir.Color {
	if c.colorScale == nil || s.c == nil {
		return nil
	}
	sc.cols = grow(sc.cols, len(idx))
	for i, row := range idx {
		sc.cols[i] = c.colorScale.Color(s.c[row])
	}
	return sc.cols
}

// varying reports whether this layer paints each mark from a colour scale.
func (c config) varying(s series) bool { return c.colorScale != nil && s.c != nil }

// smallestGap is the spacing between adjacent positions in data units: the
// smallest distance between distinct values. Using the smallest rather than
// the average means marks never overlap on irregularly spaced data.
func smallestGap(vs []float64) float64 {
	xs := make([]float64, 0, len(vs))
	for _, v := range vs {
		if finite(v) {
			xs = append(xs, v)
		}
	}
	if len(xs) < 2 {
		return 1
	}
	sort.Float64s(xs)
	gap := math.Inf(1)
	for i := 1; i < len(xs); i++ {
		if d := xs[i] - xs[i-1]; d > 0 && d < gap {
			gap = d
		}
	}
	if math.IsInf(gap, 0) {
		return 1
	}
	return gap
}

// markSpan returns the device-space edges of a mark centred on data position
// x.
//
// A band scale knows the width of a slot and is asked for it. On a continuous
// axis the width is halfWidth in data units on each side, mapped through the
// scale — so a bar on a log axis is narrower on its high side, as it must be.
func markSpan(f Frame, x, halfWidth float64) (float32, float32) {
	if band, ok := f.X.(scale.Band); ok {
		c, w := f.X.Map(x), band.Bandwidth()
		return c - w/2, c + w/2
	}
	x0, x1 := f.X.Map(x-halfWidth), f.X.Map(x+halfWidth)
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	return x0, x1
}

// baselinePos maps the value a bar or area grows from.
//
// A log axis has no position for zero, which is the default baseline. Falling
// back to the bottom of the plot is the only honest answer: the bar still
// starts at the axis, and the axis still says where that is.
func baselinePos(f Frame, v float64) float32 {
	if defined(f.Y, v) {
		return f.Y.Map(v)
	}
	lo, hi := f.Y.Domain()
	if f.Y.Map(lo) > f.Y.Map(hi) {
		return f.Y.Map(lo)
	}
	return f.Y.Map(hi)
}
