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

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
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
	// Coord decides what the interval a scale maps into means, and therefore
	// where a mapped pair lands. A nil Coord is [coord.Cartesian], so a Frame
	// built by code that never heard of coordinate systems draws the chart it
	// always drew — reach for [Frame.Coords] rather than the field.
	Coord coord.Coord
	// Theme supplies defaults the geom did not override.
	Theme theme.Theme
	// Index is the geom's position among the chart's layers, used to pick a
	// default colour from the palette.
	Index int

	// Rows, when non-nil, collects which source row is behind each mark the
	// geom draws. It is nil for an ordinary render and a geom must check it
	// before doing the bookkeeping — see [Rows] and [Frame.Marks].
	Rows Rows
}

// Coords is the frame's coordinate system, which is [coord.Cartesian] framed
// in the plot rectangle when it has none.
//
// A geom asks for it once at the top of Build and uses what comes back: the
// answer never changes within one frame, and the nil check is not worth
// repeating per row. Framing the fallback rather than handing back a bare
// Cartesian is what makes a Frame built by hand — in a test, or by a caller
// driving a geom directly — behave as it always did: the coord it gets can
// answer [coord.Coord.Extent], which is where a rule that spans the plot finds
// the far edge.
func (f Frame) Coords() coord.Coord {
	if f.Coord == nil {
		return coord.Cartesian().Frame(f.Area, nil, nil)
	}
	return f.Coord
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
//
// # Stability
//
// Geom is implemented outside this module, so it never gains a method: three
// is the whole contract. Everything else a layer can do is an optional
// interface beside it — [Faceter], [Guided], [Sized], [Legender], [Describer]
// — and a layer that implements none of them still draws. A layer defined
// outside this package reads the shared options through [Configure], takes
// its own through [Extra], and is written down and read back through
// [Describer] and [Register].
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
// API one namespace instead of six — including for a geom defined outside this
// package, which reads what an option set through [Configure] and adds a knob
// of its own through [Extra].
type Option func(*config)

type config struct {
	xcol, ycol string
	x2col      string
	y2col      string
	label      string
	textCol    string

	groupCol   string
	widthCol   string
	fromCol    string
	toCol      string
	idCol      string
	parentCol  string
	valCol     string
	padding    float64
	thickness  float64
	midCol     string
	errCol     string
	errXCol    string
	explode    float64
	explodeCol string
	stack      Stacking
	stackSet   bool
	dodge      bool
	dodgePad   float64
	order      Ordering

	sizeCol   string
	sizeScale scale.SizeScale

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
	caps       bool
	decimate   Decimation
	budget     int
	cellSize   float64

	bins      int
	binLo     float64
	binHi     float64
	bandwidth float64
	span      float64
	smooth    Smoothing
	overlap   float64

	closed    bool
	onY2      bool
	onX2      bool
	elide     bool
	dashSet   bool
	markerSet bool
	extend    bool
	fontSize  float64
	halign    ir.HAlign
	valign    ir.VAlign
	alignSet  bool
	rotation  float64

	// extra holds what a third-party option set — see [Extra]. It is nil for
	// every layer built from this package's own options.
	extra map[string]any
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

// Shape sets the marker shape for scatter geoms. Setting it explicitly opts
// the layer out of a theme's redundant-encoding ladder — see
// [github.com/timzifer/refract/theme.Redundant].
func Shape(m ir.Marker) Option {
	return func(c *config) { c.marker, c.markerSet = m, true }
}

// Size sets the marker diameter in device units.
func Size(s float32) Option { return func(c *config) { c.size = s } }

// BarWidth sets bar width as a fraction of the spacing between adjacent bars,
// in (0, 1]. The default is 0.8.
func BarWidth(f float64) Option { return func(c *config) { c.barWidth = f } }

// Baseline sets the value bars and areas grow from. The default is 0.
func Baseline(v float64) Option { return func(c *config) { c.baseline = v } }

// Y2 selects a second Y column, turning an area into a band between the two
// series rather than between one series and a baseline. It is how a confidence
// interval or a min/max envelope is drawn, and how a [Rect] is given a
// per-row baseline.
func Y2(col string) Option { return func(c *config) { c.y2col = col } }

// X2 selects a second X column, giving a [Rect] or a [Bar] its far edge on
// that axis: a gantt bar runs from a start column to an end column, a candle
// from open to close.
//
// A mark with no X2 spans its slot instead — a band scale's own bandwidth, or
// the closest spacing in the data narrowed by [BarWidth] — which is what a
// heatmap wants and what makes the cell the size of the category.
//
// Under [github.com/timzifer/refract/coord.Donut] the X axis is the radius, so
// the pair is a slice's inner and outer radius and both are dimensions of the
// data rather than a constant the coord chose:
//
//	geom.Bar(src, geom.X("floor"), geom.X2("reach"), geom.Y("share"),
//	    geom.GroupBy("browser"))
//
// The angular extent is still the stacked Y value, so such a layer reads three
// columns as three dimensions: how far round the slice goes, where it starts
// and where it stops.
//
// [Dodge] still divides the span between the layer's series, whether the span
// came from a column or from the slot: dodging is about sharing a mark's width,
// and a row that named its own edges named the width to share.
func X2(col string) Option { return func(c *config) { c.x2col = col } }

// From and To name the two ends of an edge, and [Value] what flows along it.
// The three are the whole encoding of a [Sankey] and an [Arc]: one row per
// edge, two names and a number.
//
//	geom.Sankey(src, geom.From("stage"), geom.To("next"), geom.Value("units"))
//
// The names are read as they are written — [github.com/timzifer/refract/data.Labels]
// spells a numeric or temporal column the same way a categorical axis does —
// and a node is created the first time a row mentions it. That first mention is
// what fixes the order of everything downstream: the columns a sankey stacks,
// the arcs a chord diagram goes round in, and which colour of the palette each
// node takes. It is the order of the table and never the order of a map, so a
// chart whose panels are built on separate goroutines draws what a serial one
// drew — see docs/adr/0012-parallel-panels.md.
//
// An edge naming a node only on one side still creates it: a stage nothing
// leaves is a sink, and dropping it would lose the end of the flow.
func From(col string) Option { return func(c *config) { c.fromCol = col } }

// To is [From]'s other end.
func To(col string) Option { return func(c *config) { c.toCol = col } }

// ID and Parent name a hierarchy: one row per node, the name it is known by and
// the name of the node above it. With [Value] they are the whole encoding of a
// [Treemap] and an [Icicle].
//
//	geom.Treemap(src, geom.ID("path"), geom.Parent("under"), geom.Value("bytes"))
//
// A row whose parent is empty, or names a node no row declares, is a root.
// There may be several, and they divide the whole between them.
//
// They are spelled apart from [From] and [To] on purpose, although a hierarchy
// is an edge table too: a hierarchy's edge runs from the child to its parent
// and a flow's runs from source to target, so a document that called both
// "from" and "to" would read a treemap as a flow. The columns are the same
// shape; what they mean is not.
func ID(col string) Option { return func(c *config) { c.idCol = col } }

// Parent is [ID]'s other end: the node this row hangs under.
func Parent(col string) Option { return func(c *config) { c.parentCol = col } }

// Value names the column carrying the magnitude of an edge or of a node: the
// units flowing along a link, the bytes in a directory, the weight of a chord.
//
// For a hierarchy it is usually the leaves that carry a number and the internal
// nodes that carry none, because an internal node's size is what is under it.
// A row that carries one anyway keeps it, and its share shows as the part of
// its box its children do not cover — which is how an unaccounted-for remainder
// becomes visible rather than invisible.
func Value(col string) Option { return func(c *config) { c.valCol = col } }

// Padding is the gap left between adjacent shapes of a layout that places its
// own: the space between a treemap's cells, between a sunburst's rings, between
// a sankey's nodes and between a chord diagram's arcs. It is a fraction of the
// plot, in [0, 1), and zero — the default — means the shapes touch.
//
// It is a gap rather than a stroke on purpose. A border drawn round a cell is
// ink a reader has to discount from the area they are being asked to compare,
// and under hit-testing a stroked outline sits *above* the shape it outlines,
// so a pointer on the border reports the border — see
// docs/adr/0015-hit-testing.md.
func Padding(f float64) Option { return func(c *config) { c.padding = f } }

// Thickness is how much of its slot a node fills, in (0, 1]: the width of a
// sankey's columns and the depth of a chord diagram's ring of arcs. Zero — the
// default — means the mark's own, which differs between them because a sankey's
// node is a landmark and a chord's is a rim.
//
// It is not [BarWidth], although it means something close, because BarWidth's
// default is 0.8 rather than zero: a mark could not tell a caller who asked for
// 0.8 from one who asked for nothing, and a default that cannot be told from a
// choice is the trap [Dash] and [Stack] each carry a companion flag to avoid.
func Thickness(f float64) Option { return func(c *config) { c.thickness = f } }

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

// Closed joins the last mark of a connected layer back to the first.
//
// It is what turns a [Line] over an angular axis into a radar contour and an
// [Area] over one into a filled one: five axes drawn as an open line leave a
// gap between the last and the first, which is a hole in a shape that has none.
// It applies to [Line], [Area] and [Step]; a layer whose marks are not
// connected ignores it.
//
// It is an option rather than something [coord.Polar] decides, because whether
// a series wraps is a fact about the series and not about the transform: a
// radar covers every axis and closes, and a polar time series spiralling
// through three revolutions does not.
func Closed(on bool) Option { return func(c *config) { c.closed = on } }

// ColorBy maps a column through a colour scale, giving every mark its own
// colour. It applies to [Scatter], [Bar] and [Rect]; geoms whose mark is one
// connected shape ignore it.
//
// Which guide the layer then contributes follows from the kind of scale it was
// handed. A continuous scale — [scale.Sequential], [scale.Diverging] — reads a
// numeric column and contributes a colourbar, because a single swatch cannot
// represent a continuum; see [ColorGuide]. A discrete one —
// [scale.Qualitative] — reads a column of categories and contributes one legend
// entry per category; see [Legender].
//
// The same scale also colours the series of a grouped layer, where the group
// label is the category. Sharing one across the layers of a chart, or across
// the panels of a facet, is what makes one colour mean one thing everywhere.
func ColorBy(col string, s scale.ColorScale) Option {
	return func(c *config) { c.colorCol, c.colorScale = col, s }
}

// TextBy names the column a [Text] layer reads its label from, one label per
// row. Any column will do: a text column is used as it is, and a numeric or
// temporal one is formatted the way a category name is, so an axis tick and a
// label for the same value are the same string.
//
// It is the channel that makes text data rather than annotation. [Note] places
// one literal string at one literal position; a layer that labels its rows
// reads them from the same [data.Source] every other mark reads, and needs no
// rebuild when the rows change.
func TextBy(col string) Option { return func(c *config) { c.textCol = col } }

// Elide lets a [Text] layer truncate a label that does not fit the box its row
// spans, ending it with an ellipsis.
//
// Without it the label is dropped instead, which is the default because a
// truncated label is a claim about a row that the row does not quite make: a
// column of "Wareneingang", "Warenausgang" and "Wartung" elides to three
// labels that are hard to tell apart, and no label at all is honest about it.
// Turn it on where the first few characters are enough to identify the row.
//
// It does nothing in point mode, where there is no box to overrun.
func Elide(on bool) Option { return func(c *config) { c.elide = on } }

// SizeBy maps a column through a size scale, giving every mark its own size.
// It applies to [Scatter]; geoms whose mark has a width the axes decide ignore
// it.
//
// It is the bubble chart, and it is a channel rather than a mark for the same
// reason a pie is not a geom: what changes is which column decides a mark's
// size, not what the mark is. The layer contributes a third guide kind beside
// the legend and the colourbar — a ladder of sample marks with the values they
// stand for — because a size is a continuum a single swatch cannot represent.
// See [scale.Size] for why the mapping is by area.
//
// A sized layer draws circles rather than markers, and that is a consequence of
// the IR rather than a preference: [ir.Backend.Markers] carries one style per
// call, so a layer whose size varied per row would be one drawing call per row.
// A circle per subpath of one path is one call per colour, and it gives a
// pointer the mark it is actually inside rather than the nearest centre.
func SizeBy(col string, s scale.SizeScale) Option {
	return func(c *config) { c.sizeCol, c.sizeScale = col, s }
}

// Bins sets how many bins a [Histogram] divides its column into. The default,
// 0, lets the layer choose: the Freedman-Diaconis rule where the data has an
// interquartile range to measure, and Sturges's rule where it does not.
func Bins(n int) Option { return func(c *config) { c.bins = n } }

// BinRange pins the interval a [Histogram] covers. The default is the extent of
// the data.
//
// Pinning it is what makes two histograms comparable: bins chosen from each
// column's own extent put the same value in different places, and a reader
// comparing two panels is then comparing the axes rather than the data.
func BinRange(lo, hi float64) Option {
	return func(c *config) { c.binLo, c.binHi = lo, hi }
}

// Bandwidth sets the kernel width a [Violin] or a [Ridgeline] estimates its
// density with, in the data's own units. The default, 0, lets
// [github.com/timzifer/refract/stat.Silverman] choose it from each group's own
// spread.
//
// It is the one number that decides what a density looks like, so pinning it is
// how several groups are made comparable: a bandwidth chosen per group means
// each group is smoothed by a different amount, and a difference in shape can
// then be a difference in sample size rather than in distribution.
func Bandwidth(bw float64) Option { return func(c *config) { c.bandwidth = bw } }

// Span sets the fraction of the rows one local fit of a [Trend] sees, in
// (0, 1]. The default is stat.DefaultSpan. It has no effect on a straight fit,
// which sees all of them.
func Span(f float64) Option {
	return func(c *config) {
		if f > 0 {
			c.span = f
		}
	}
}

// Smoothing is how a [Trend] fits its line.
type Smoothing uint8

// The smoothings. Loess is the default: a trend line's job is to show what the
// data is doing, and a straight line shows what a straight line would do.
const (
	// Loess fits a locally weighted line through the neighbours of each
	// abscissa. See stat.Loess.
	Loess Smoothing = iota

	// LinearFit is ordinary least squares over the whole column: one straight
	// line, which is the right answer when the claim being made is that the
	// relationship *is* linear.
	LinearFit
)

// Smooth sets how a [Trend] fits.
func Smooth(m Smoothing) Option { return func(c *config) { c.smooth = m } }

// Overlap sets how far a [Ridgeline]'s tallest ridge rises, in slots of its
// categorical axis. The default is 1.6.
//
// Overlapping is the point of the chart rather than a defect of it: the ridges
// are read against each other, and separating them into a grid of little
// densities is the small multiple this chart exists to compress. Values below 1
// keep each ridge inside its own slot.
func Overlap(f float64) Option {
	return func(c *config) {
		if f > 0 {
			c.overlap = f
		}
	}
}

// defaultOverlap is how far a ridge rises by default: a little over one and a
// half slots, which is enough that the ridges interleave and read as one
// distribution changing, and little enough that a tall one does not reach the
// label of the row two above it.
const defaultOverlap = 1.6

// Whisker sets how far a boxplot whisker reaches, as a multiple of the
// interquartile range. The default is 1.5, Tukey's original choice.
func Whisker(k float64) Option { return func(c *config) { c.whisker = k } }

// Outliers turns the individual points beyond the whiskers on or off. They are
// on by default: a boxplot that hides them is a boxplot that hides exactly the
// rows a reader opened the chart to find.
func Outliers(show bool) Option { return func(c *config) { c.outliers = show } }

// OnY2 binds this layer to the chart's secondary vertical axis, the one
// [github.com/timzifer/refract.Plot.Y2] sets, instead of to its primary one.
//
// It is the option a chart of two quantities in different units needs —
// revenue as bars against a left axis, margin as a percentage line against a
// right one — and it is on the *layer* because that is where the binding is: a
// scale does not know which marks read it, and a chart with two Y axes is one
// chart with two of them rather than two charts overlaid.
//
// A layer that asks for it in a chart with no secondary axis draws against the
// primary one, silently, for the reason [Explode] is silent under a Cartesian
// coord: an option every mark accepts must not make a chart's validity depend
// on something set somewhere else.
func OnY2() Option { return func(c *config) { c.onY2 = true } }

// OnX2 is [OnY2] turned a quarter turn: it binds this layer to the chart's
// secondary *horizontal* axis, the one [github.com/timzifer/refract.Plot.X2]
// sets, drawn along the top of the panel.
//
// It is the option two series measured over different extents of the same
// thing need — a run indexed by cycle against one indexed by elapsed time, or
// a spectrum read in wavelength against the same spectrum in wavenumber. The
// two are independent: a layer may name both, and then it reads the top axis
// and the right one.
func OnX2() Option { return func(c *config) { c.onX2 = true } }

// Mid selects the column an [ErrorBar] marks the measurement at, inside the
// interval it draws.
//
// It is the third number an interval needs and the reason this mark has a
// channel of its own: [Y] and [Y2] are the two ends, exactly as they are for
// the band an [Area] draws and the box a [Rect] draws, which leaves the
// measurement itself nowhere to go. A layer that names no Mid draws the
// interval alone, which is the honest picture when the interval is all that
// was measured — a min and a max are not evidence of a mean.
//
// The symmetric spelling needs no Mid: [ErrorBy] reads the measurement from
// [Y] and derives both ends from it, so the value is already named.
func Mid(col string) Option { return func(c *config) { c.midCol = col } }

// ErrorBy selects a column of half-widths, and makes an [ErrorBar] symmetric
// about its [Y] value: the interval runs from y−e to y+e.
//
// It is the spelling a table usually has. A mean and a standard deviation, or
// a mean and a margin of error, are two columns; turning them into a low and a
// high column first is arithmetic the caller should not have to do to draw a
// chart of what they measured.
//
// The measurement is marked, because with this spelling there always is one:
// the centre is the [Y] column, so a layer given a spread is a point with an
// interval around it rather than an interval alone.
func ErrorBy(col string) Option { return func(c *config) { c.errCol = col } }

// ErrorXBy is [ErrorBy] along the horizontal axis: the interval runs from x−e
// to x+e about the [X] value, and the mark lies on its side.
//
// Which axis an error bar runs along follows from the encoding and nothing
// else, exactly as a [Rect]'s edges do: naming [Y2] or [ErrorBy] puts the
// interval on the vertical axis, naming [X2] or this puts it on the
// horizontal one. That is what a chart of measurements against categories on
// the Y axis needs, and it is why there is no orientation option.
func ErrorXBy(col string) Option { return func(c *config) { c.errXCol = col } }

// Caps turns the crossbars at the ends of an [ErrorBar] on or off. They are on
// by default.
//
// Turning them off is the point-range look: a rule with a marker on it and
// nothing at the ends, which is what a chart with many intervals close
// together wants — caps that touch read as a grid. The cap is half as wide as
// a [Bar] of the same [BarWidth] would be, so an error bar drawn over a bar
// chart is narrower than the bar it annotates.
func Caps(show bool) Option { return func(c *config) { c.caps = show } }

// Align sets how a text annotation sits about its position. The default is
// the run's start on the point, on the baseline — except in a [Text] layer
// that labels a box, where a label with nothing said about it is centred in
// the box rather than hung off its left edge.
//
// That is why the layer records having been told: the start of the run and the
// baseline are both the zero value and an alignment somebody may have asked
// for, exactly as a circle is for [Shape].
func Align(h ir.HAlign, v ir.VAlign) Option {
	return func(c *config) { c.halign, c.valign, c.alignSet = h, v, true }
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
	c := config{
		barWidth: 0.8, whisker: 1.5, outliers: true, caps: true, opacity: -1, extend: true,
		span: stat.DefaultSpan, overlap: defaultOverlap,
	}
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

// dashFor resolves the dash pattern: the layer's own if it named one,
// otherwise the theme's redundant-encoding ladder — which is empty unless the
// theme asked for it, so this is nil for every ordinary chart.
func (c config) dashFor(f Frame) []float32 {
	if c.dashSet {
		return c.dash
	}
	return f.Theme.SeriesDash(f.Index)
}

// markerFor resolves the marker shape the same way. A layer that named a shape
// keeps it: redundant encoding is a default, and an explicit choice outranks
// a default even when the default is the more accessible one.
func (c config) markerFor(f Frame) ir.Marker {
	if c.markerSet {
		return c.marker
	}
	if m, ok := f.Theme.SeriesMarker(f.Index); ok {
		return m
	}
	return c.marker
}

func (c config) labelFor() string {
	if c.label != "" {
		return c.label
	}
	return c.ycol
}

// labelForX is labelFor for a layer whose subject is its X column: a
// histogram, an ECDF and a ridgeline all summarise one column, and it is on X.
// Falling back to the Y column would name them after an axis they compute.
func (c config) labelForX() string {
	if c.label != "" {
		return c.label
	}
	return c.xcol
}

// series is a resolved set of columns, already converted to float64.
//
// y2, c and sz are optional: y2 carries the second bound of a band, c the
// values a colour scale is read from, sz the values a size scale is read from.
// Each is nil when the geom was not given one.
//
// off and rows say where the elements came from, for the caller that asked to
// know — see [Rows]. A segment cut out of the source is contiguous, so off
// alone answers it; an interpolated one is not, and carries a row per element.
type series struct {
	x, y  []float64
	y2, c []float64
	sz    []float64
	off   int
	rows  []int

	// origin maps this series' rows onto the rows of the table its source was
	// cut from, when its source is a cut. Faceting makes one per panel, so
	// without this a faceted chart would report row numbers relative to a
	// table the caller never sees. See [data.Subset].
	origin []int
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
	s := series{x: xs, y: ys, origin: data.Origins(src)}
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
		v, err := colorColumn(src, c)
		if err != nil {
			return series{}, err
		}
		if len(v) != len(xs) {
			return series{}, fmt.Errorf("refract/geom: columns %q and %q differ in length (%d vs %d)", c.xcol, c.colorCol, len(xs), len(v))
		}
		s.c = v
	}
	if c.sizeCol != "" && c.sizeScale != nil {
		v, err := column(src, c.sizeCol, nil)
		if err != nil {
			return series{}, err
		}
		if len(v) != len(xs) {
			return series{}, fmt.Errorf("refract/geom: columns %q and %q differ in length (%d vs %d)", c.xcol, c.sizeCol, len(xs), len(v))
		}
		s.sz = v
	}
	dropNulls(src, &s, c.groupCol, c.sizeCol, c.widthCol)
	return s, nil
}

// resolveOne reads a layer that summarises a single column.
//
// A [Histogram] and an [ECDF] read X and compute what goes on Y, so [resolve]'s
// insistence on both columns would refuse them for not naming a column they do
// not have. The Y column of the series is the X column again rather than nil,
// so that every traversal written over a series — plottable, the missing-data
// policy, the group index — reads the observation on both axes and answers the
// one question there is: does this value have a position.
func resolveOne(src data.Source, c config, x scale.Scale) (series, error) {
	if src == nil {
		return series{}, errors.New("refract/geom: nil data source")
	}
	xs, err := column(src, c.xcol, x)
	if err != nil {
		return series{}, err
	}
	s := series{x: xs, y: xs, origin: data.Origins(src)}
	dropNulls(src, &s, c.groupCol)
	return s, nil
}

// colorColumn reads the column a colour scale paints from.
//
// A discrete scale reads categories, so a text column is encoded through it —
// the same arrangement a categorical *axis* has, where the scale assigns the
// numbers and the column is read in the scale's own space. A continuous scale
// reads numbers and a text column is an error: there is no position on a ramp
// for a name.
func colorColumn(src data.Source, c config) ([]float64, error) {
	if d, discrete := scale.Discrete(c.colorScale); discrete {
		// Whatever the column is stored as, it is read as one label per row —
		// [data.Labels] is the shared spelling of that, so a colour category
		// and an axis category for the same value are the same string.
		labels, ok := data.Labels(src, c.colorCol)
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrNoColumn, c.colorCol)
		}
		// A row with no category is not given one. NaN is the value every
		// colour scale answers with its undefined colour, so an absent
		// category reads as "not one of these" rather than as a category
		// named "" holding a slot in the palette.
		null, _ := data.NullMask(src, c.colorCol)
		out := make([]float64, len(labels))
		for i, l := range labels {
			if data.IsNull(null, i) {
				out[i] = math.NaN()
				continue
			}
			out[i] = d.Encode(l)
		}
		return out, nil
	}
	if _, text := src.StringColumn(c.colorCol); text {
		return nil, fmt.Errorf("%w: column %q holds category names and a colour ramp reads numbers; give it a scale.Qualitative",
			ErrCategorical, c.colorCol)
	}
	return column(src, c.colorCol, nil)
}

// column reads one column as float64 for the axis it feeds.
//
// A numeric column on a continuous scale is returned as it lies — that is the
// zero-copy path the data layer exists for. A time column becomes a number in
// the axis's own time space through [scale.ValueOf], so a time scale sees the
// same numeric domain as any other — nanoseconds since the scale's origin,
// which is the Unix epoch unless [scale.Origin] moved it. A text column is
// encoded through the scale, which must be categorical.
//
// A numeric or time column on a *categorical* scale is encoded too, one
// category per distinct formatted value: asking for an ordinal axis over
// numbers means asking for equally spaced slots rather than a numeric line.
func column(src data.Source, name string, s scale.Scale) ([]float64, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: no column selected (use geom.X/geom.Y)", ErrNoColumn)
	}
	cat, _ := s.(scale.Categorical)
	// A row the source calls absent becomes NaN, whatever the column is stored
	// as. That is the whole of the null rule on a position axis: no scale
	// places a NaN, so plottable rejects the row and [OnMissing] decides what
	// happens to it — the same three answers a numeric column's own NaN
	// already gets. Doing it here rather than per geom is what makes it true
	// of every mark, including the ones that do not exist yet. See
	// [data.Nulls].
	null, _ := data.NullMask(src, name)

	if v, ok := src.StringColumn(name); ok {
		if cat == nil {
			return nil, fmt.Errorf("%w: column %q holds category names; give that axis a scale.Ordinal", ErrCategorical, name)
		}
		return encode(cat, v, null, func(l string) string { return l }), nil
	}
	if v, ok := src.Float64Column(name); ok {
		if cat == nil {
			if !masks(v, null) {
				return v, nil
			}
			out := make([]float64, len(v))
			copy(out, v)
			for i := range out {
				if data.IsNull(null, i) {
					out[i] = math.NaN()
				}
			}
			return out, nil
		}
		return encode(cat, v, null, data.FormatNumber), nil
	}
	if t, ok := src.TimeColumn(name); ok {
		if cat != nil {
			return encode(cat, t, null, func(tv time.Time) string { return tv.Format(time.RFC3339) }), nil
		}
		out := make([]float64, len(t))
		for i, tv := range t {
			if data.IsNull(null, i) {
				out[i] = math.NaN()
				continue
			}
			out[i] = scale.ValueOf(s, tv)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrNoColumn, name)
}

// masks reports whether applying null to vs would change any of them.
//
// It is what keeps the zero-copy path in [data.Float64Columns] intact for the
// source that needs the mask least: an Arrow null is already NaN in a numeric
// column, so the mask agrees with the values and the caller's slice is handed
// on untouched. A source that says a row is absent while leaving a number in
// it — which is the whole point of the interface for text and time, and is
// permitted for numbers — forces the copy, and only then.
func masks(vs []float64, null []bool) bool {
	for i := range vs {
		if data.IsNull(null, i) && finite(vs[i]) {
			return true
		}
	}
	return false
}

// encode maps a column through a categorical scale, naming each value with
// label.
//
// A null row is not encoded at all. Registering it would give the scale a
// category for a value that is not there — "" for a text column, the zero
// time for a temporal one — and an ordinal axis would grow a band for it,
// between the categories somebody did measure.
func encode[T any](cat scale.Categorical, vs []T, null []bool, label func(T) string) []float64 {
	out := make([]float64, len(vs))
	for i, v := range vs {
		if data.IsNull(null, i) {
			out[i] = math.NaN()
			continue
		}
		out[i] = cat.Encode(label(v))
	}
	return out
}

// dropNulls marks a row missing when a column the layer reads *beside* its
// position has no value there.
//
// The position columns need none of this: [column] has already turned their
// nulls into NaN, and a NaN has no position. These are the others — the group
// column, the size column, the width column — where the value is absent but
// the row still has somewhere to go, and where drawing it would mean drawing a
// mark whose series, or whose size, nobody measured. The colour column is
// deliberately not among them: a colour scale has an answer for a value it
// cannot place ([scale.ColorUndefined]), so a row with an unknown category is
// still a reading and is painted in the undefined colour. That colour is
// transparent unless the scale was given one, so such a row is invisible by
// default — which is the answer every unmappable colour value already got,
// reached without a rule of its own.
//
// The copy is made on the first row it has to change and not before, because
// s.x and s.y may be the caller's own slices — the zero-copy path — and
// writing a NaN into those would edit the table rather than the chart.
func dropNulls(src data.Source, s *series, cols ...string) {
	owned := false
	for _, col := range cols {
		if col == "" {
			continue
		}
		null, ok := data.NullMask(src, col)
		if !ok {
			continue
		}
		for i := range s.x {
			if !data.IsNull(null, i) {
				continue
			}
			if !owned {
				s.detach()
				owned = true
			}
			s.x[i], s.y[i] = math.NaN(), math.NaN()
		}
	}
}

// detach replaces the series' position columns with copies it owns.
//
// x and y are the same slice for a layer that summarises one column — see
// [resolveOne] — so the aliasing is preserved rather than broken into two
// copies that happen to hold the same numbers.
func (s *series) detach() {
	alias := len(s.x) > 0 && len(s.y) == len(s.x) && &s.x[0] == &s.y[0]
	s.x = append([]float64(nil), s.x...)
	if alias {
		s.y = s.x
		return
	}
	s.y = append([]float64(nil), s.y...)
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
	return c.fillOf(c.colorFor(f), defaultOpacity)
}

// fillOf is fillFor over a colour the caller already resolved, which is what a
// grouped layer needs: the base colour is the series' own rather than the
// layer's.
func (c config) fillOf(base ir.Color, defaultOpacity float64) ir.Color {
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

// indexRun is a set of marks that share a colour, listed by their position in
// the layer's own arrays rather than by their points.
//
// It is [colorRun] for a mark that carries more than a position. A bubble has a
// centre *and* a diameter, and a run of points alone would lose which diameter
// belongs to which centre — so the run carries the indices and the caller reads
// both arrays through them.
type indexRun struct {
	color ir.Color
	idx   []int
}

// groupByColorAt batches marks by colour, in order of first appearance,
// reporting each run as a list of indices into cols.
func (sc *scratch) groupByColorAt(cols []ir.Color) []indexRun {
	if sc.at == nil {
		sc.at = make(map[ir.Color]int, 8)
	}
	clear(sc.at)
	n := 0
	for i, c := range cols {
		j, ok := sc.at[c]
		if !ok {
			j = n
			n++
			if j == len(sc.iruns) {
				sc.iruns = append(sc.iruns, indexRun{})
			}
			sc.iruns[j].color = c
			sc.iruns[j].idx = sc.iruns[j].idx[:0]
			sc.at[c] = j
		}
		sc.iruns[j].idx = append(sc.iruns[j].idx, i)
	}
	return sc.iruns[:n]
}

// rectRun is a set of cells that share a colour. It is [colorRun] for a mark
// that is a box rather than a point, and it exists for the same reason: the IR
// carries one style per drawing call, so a layer of a thousand differently
// coloured cells is a call per distinct colour rather than a call per cell.
type rectRun struct {
	color ir.Color
	rects []ir.Rect
	// offs is how far each of the run's cells is broken out of the middle of
	// the coord, index for index with rects. It is empty for a layer that is
	// not broken out, which is every layer that never asked — read it through
	// [offsetAt], which is why that tests the length rather than nil.
	offs []ir.Point
}

// groupByRect batches cells by colour, in order of first appearance. offs is
// the displacement of each cell, or nil.
func (sc *scratch) groupByRect(rects []ir.Rect, cols []ir.Color, offs []ir.Point) []rectRun {
	if sc.at == nil {
		sc.at = make(map[ir.Color]int, 8)
	}
	clear(sc.at)
	n := 0
	for i, r := range rects {
		c := cols[i]
		j, ok := sc.at[c]
		if !ok {
			j = n
			n++
			if j == len(sc.rruns) {
				sc.rruns = append(sc.rruns, rectRun{})
			}
			sc.rruns[j].color = c
			sc.rruns[j].rects = sc.rruns[j].rects[:0]
			sc.rruns[j].offs = sc.rruns[j].offs[:0]
			sc.at[c] = j
		}
		sc.rruns[j].rects = append(sc.rruns[j].rects, r)
		if offs != nil {
			sc.rruns[j].offs = append(sc.rruns[j].offs, offs[i])
		}
	}
	return sc.rruns[:n]
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

// trainSizes feeds the size column into the size scale, for the same reason
// [config.trainColors] exists: a size scale is not a Scale, it maps to ink
// rather than to a position.
func (c config) trainSizes(s series) {
	if c.sizeScale == nil || s.sz == nil {
		return
	}
	c.sizeScale.Train(s.sz...)
}

// sizing reports whether this layer takes each mark's size from a column.
func (c config) sizing(s series) bool { return c.sizeScale != nil && s.sz != nil }

// errLength reports two columns of a layer that disagree about how many rows
// there are.
func errLength(a, b string, na, nb int) error {
	return fmt.Errorf("refract/geom: columns %q and %q differ in length (%d vs %d)", a, b, na, nb)
}

// extent is the range of the finite values in a column.
func extent(vs []float64) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, v := range vs {
		if finite(v) {
			lo, hi, ok = math.Min(lo, v), math.Max(hi, v), true
		}
	}
	return lo, hi, ok
}

// smallestGap is the spacing between adjacent positions in data units: the
// smallest distance between distinct values. Using the smallest rather than
// the average means marks never overlap on irregularly spaced data.
//
// It sorts, so it needs somewhere to sort: buf is the caller's own buffer,
// which the plottable values are gathered into and which is handed back grown
// for the next call. That is the same bargain every buffer sized by the data
// makes here, and it is not decoration — a layer measures its slot on every
// Train, Train runs on every frame, and a copy of the column per frame is most
// of what a hundred-thousand-row frame would otherwise allocate. The buffer
// lives on the layer rather than in the frame's pool for the reason the group
// index does: Train runs outside a Build, where there is no scratch to take.
func smallestGap(buf, vs []float64) (float64, []float64) {
	xs := buf[:0]
	for _, v := range vs {
		if finite(v) {
			xs = append(xs, v)
		}
	}
	if len(xs) < 2 {
		return 1, xs
	}
	sort.Float64s(xs)
	gap := math.Inf(1)
	for i := 1; i < len(xs); i++ {
		if d := xs[i] - xs[i-1]; d > 0 && d < gap {
			gap = d
		}
	}
	if math.IsInf(gap, 0) {
		return 1, xs
	}
	return gap, xs
}

// markSpan returns the mapped edges of a mark centred on data position x —
// device coordinates under a Cartesian coord, and an angle or a radius under
// another one, which is the coord's business rather than the geom's.
//
// A band scale knows the width of a slot and is asked for it. On a continuous
// axis the width is halfWidth in data units on each side, mapped through the
// scale — so a bar on a log axis is narrower on its high side, as it must be.
func markSpan(f Frame, x, halfWidth float64) (float32, float32) {
	return slotOn(f.X, x, halfWidth)
}

// slotOn is markSpan against a named scale, which is what a mark with width on
// the *vertical* axis needs — a ridgeline's slot is a row rather than a column.
func slotOn(s scale.Scale, v, halfWidth float64) (float32, float32) {
	if band, ok := s.(scale.Band); ok {
		c, w := s.Map(v), band.Bandwidth()
		return c - w/2, c + w/2
	}
	a, b := s.Map(v-halfWidth), s.Map(v+halfWidth)
	if b < a {
		a, b = b, a
	}
	return a, b
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
