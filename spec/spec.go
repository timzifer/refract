// Package spec writes a chart down as JSON and reads it back.
//
// The document is Vega-Lite-shaped: `data.values`, `mark`, `encoding.x.field`,
// `scale.type`, `facet` and `resolve` mean what they mean in Vega-Lite, and a
// person who knows that vocabulary can read a refract spec without a manual.
// It is not a Vega-Lite subset, and it does not claim to be one — refract has
// marks Vega-Lite has no name for and Vega-Lite has transforms refract does
// not run. See docs/adr/0014-json-spec.md for what that choice buys and costs.
//
// What is guaranteed is the round trip through refract:
//
//	s, err := spec.Of(chart)      // chart -> document
//	c, err := s.Chart()           // document -> chart
//
// draws the same marks in the same places. The one thing that cannot survive
// is a Go function: a custom tick formatter or a custom colour ramp has no
// JSON, and a scale carrying one says so through [scale.Desc.Formatted].
//
// # Shape
//
//	{
//	  "$schema": "https://github.com/timzifer/refract/spec/v1",
//	  "width": 800, "height": 500,
//	  "title": "Signal",
//	  "data": {"values": [{"t": 0, "y": 1}], "format": {"parse": {"t": "number", "y": "number"}}},
//	  "encoding": {
//	    "x": {"type": "quantitative", "scale": {"type": "linear", "nice": true}},
//	    "y": {"type": "quantitative", "scale": {"type": "linear", "nice": true}}
//	  },
//	  "layer": [{"mark": {"type": "line"}, "encoding": {"x": {"field": "t"}, "y": {"field": "y"}}}]
//	}
//
// The top-level `encoding` carries the plot's scales and axis titles, which is
// where a layered Vega-Lite spec puts the encodings its layers share. Each
// layer's own `encoding` carries the columns it reads. Data is hoisted to the
// top level when every layer draws from the same source and written per layer
// when they do not.
package spec

import (
	"encoding/json"
	"fmt"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// Schema is the value written to `$schema`. It names the dialect and the
// version of it; nothing fetches it, and a Vega-Lite consumer that checks the
// field will refuse the document, which is the honest outcome.
//
// # Stability
//
// The dialect is a v1 stability surface. Within v1 a field is only ever added,
// never removed, renamed or given a different meaning, and a reader ignores a
// field it does not know — so a document written by any v1.x reads in any
// other, and a document written by a v0.x refract reads too. The version in
// the string moves only with the major version of the module, because that
// is the only time the meaning of a field may change.
//
// Reading is not gated on it: refusing a chart because of a version string
// would make the field a trap rather than a label. A document this package
// cannot understand fails on the part it cannot understand, naming it.
//
// Before v1 the string moved with the dialect: v0.9 added the distribution
// marks and the size channel, v0.8 the top-level `coord`, v0.7 the series
// channel (`detail`), the position adjustments and the data-driven rect, and
// v0.6 a time scale's `origin`.
const Schema = "https://github.com/timzifer/refract/spec/v1"

// Chart is the part of a plot that survives being written down: everything
// [Of] reads and everything [Spec.Chart] returns.
//
// It exists so that this package does not import the root one — a spec is
// built out of the model packages, and the root package assembles a Plot from
// what comes back.
type Chart struct {
	Width, Height int
	DPR           float64
	Theme         theme.Theme
	Title         string
	XTitle        string
	YTitle        string
	// Y2Title and X2Title label the secondary axes, and Y2 and X2 are their
	// scales. All four are zero for a chart with one axis in that direction.
	Y2Title string
	X2Title string
	X, Y    scale.Scale
	Y2, X2  scale.Scale
	Coord   coord.Coord
	Layers  []geom.Geom
	Facet   *facet.Spec

	// Tracks are the bands at the plot's edges, in the order they were added.
	Tracks []Track

	// Legend forces the legend on or off. Nil leaves the default, which shows
	// one as soon as a plot has more than one layer.
	Legend *bool
}

// Track is a band at an edge of the plot area, written down.
//
// It carries one scale of its own and its own layers, and shares the chart's
// other scale — which is why only one is written here: a track that named a
// different scale for the axis it runs along would not be a track.
type Track struct {
	// Edge is "bottom", "top", "left" or "right".
	Edge string
	// Size is how thick the band is in device-independent pixels, and Fraction
	// its thickness as a share of the canvas. Exactly one is non-zero.
	Size, Fraction float32
	// Scale is the band's own scale: the one it does not share with the plot.
	Scale scale.Scale
	// Layers are the band's marks.
	Layers []geom.Geom
	// Axis reports whether the band writes its tick labels, and Grid whether
	// it draws grid lines.
	Axis, Grid bool
}

// Spec is a chart as a JSON document.
type Spec struct {
	Schema   string     `json:"$schema,omitempty"`
	Width    int        `json:"width,omitempty"`
	Height   int        `json:"height,omitempty"`
	Title    string     `json:"title,omitempty"`
	Data     *Data      `json:"data,omitempty"`
	Encoding *Encoding  `json:"encoding,omitempty"`
	Coord    *Coord     `json:"coord,omitempty"`
	Layer    []Layer    `json:"layer,omitempty"`
	Facet    *Facet     `json:"facet,omitempty"`
	Tracks   []TrackDoc `json:"tracks,omitempty"`
	Columns  int        `json:"columns,omitempty"`
	Resolve  *Resolve   `json:"resolve,omitempty"`
	Config   *Config    `json:"config,omitempty"`
}

// Layer is one set of marks.
type Layer struct {
	// Name is the layer's legend label, when it was given one.
	Name     string    `json:"name,omitempty"`
	Mark     Mark      `json:"mark"`
	Data     *Data     `json:"data,omitempty"`
	Encoding *Encoding `json:"encoding,omitempty"`
}

// Mark is what a layer draws, and how.
//
// Type and the properties above the line are Vega-Lite's, spelled as
// Vega-Lite spells them. The properties below it are refract's own: they name
// behaviour Vega-Lite has no equivalent for, so borrowing a Vega-Lite name for
// them would be the misleading option.
type Mark struct {
	Type        string    `json:"type"`
	Color       string    `json:"color,omitempty"`
	Fill        string    `json:"fill,omitempty"`
	Opacity     *float64  `json:"opacity,omitempty"`
	Size        float32   `json:"size,omitempty"`
	Shape       string    `json:"shape,omitempty"`
	StrokeWidth float32   `json:"strokeWidth,omitempty"`
	StrokeDash  []float32 `json:"strokeDash,omitempty"`
	Interpolate string    `json:"interpolate,omitempty"`
	Tension     float64   `json:"tension,omitempty"`
	Orient      string    `json:"orient,omitempty"`
	Extent      float64   `json:"extent,omitempty"`
	Outliers    *bool     `json:"outliers,omitempty"`
	Text        string    `json:"text,omitempty"`
	Align       string    `json:"align,omitempty"`
	Baseline    string    `json:"baseline,omitempty"`
	FontSize    float64   `json:"fontSize,omitempty"`
	Angle       float64   `json:"angle,omitempty"`

	// Caps is whether an error bar carries a crossbar at each end. It is
	// refract's own, it is a pointer because the default is true rather than
	// false, and a document that omits it gets the caps — writing `false`
	// is the point-range look.
	Caps *bool `json:"caps,omitempty"`

	// XAxis and YAxis name the scales this layer's values are read against:
	// "x2" and "y2" for the chart's secondary axes, and empty for its primary
	// ones. They are two fields rather than one because the two directions are
	// independent — a layer may read the top axis and the right one — and a
	// single field would have to spell a set.
	//
	// They are refract's own: Vega-Lite reaches a second axis by layering two
	// specs with independent resolves, which is a different picture and a
	// different set of scales.
	XAxis string `json:"xAxis,omitempty"`
	YAxis string `json:"yAxis,omitempty"`

	// Elide is whether a text layer truncates a label too wide for the box its
	// row spans rather than dropping it. It is refract's own: Vega-Lite has no
	// equivalent, so no name is borrowed for it.
	Elide bool `json:"elide,omitempty"`

	// Origin is the value bars and areas grow from — Vega-Lite reaches the
	// same place through a scale's `zero`, which is a different thing.
	Origin float64 `json:"origin,omitempty"`
	// BarWidth is the fraction of the slot a bar fills.
	BarWidth *float64 `json:"barWidth,omitempty"`
	// Missing is the NaN policy: "gap", "interpolate" or "error".
	Missing string `json:"missing,omitempty"`
	// Decimate is the reduction: "auto", "none", "lttb", "minmax" or
	// "density".
	Decimate string `json:"decimate,omitempty"`
	// Budget caps how many marks survive a reduction.
	Budget int `json:"budget,omitempty"`
	// DensityCells is the cell size of a density raster, in device units.
	DensityCells float64 `json:"densityCells,omitempty"`
	// Extend reports whether an annotation widens the axis to include itself.
	Extend *bool `json:"extend,omitempty"`
	// Dodge places the groups of a slot side by side rather than stacking
	// them, leaving this fraction of each share blank. It is a pointer because
	// zero padding is a dodge with the bars touching, which is a different
	// thing from no dodge at all. Vega-Lite reaches the same place with an
	// `xOffset` channel and its own scale, which is more machinery than one
	// number.
	Dodge *float64 `json:"dodge,omitempty"`
	// Order is the order the groups are stacked and listed in: "appearance",
	// "value" or "inside-out".
	Order string `json:"order,omitempty"`
	// Explode breaks the mark out of the middle of the coord, as a fraction of
	// its outer radius: a slice pulled out of a donut. It is refract's own —
	// Vega-Lite has no coordinate stage and therefore no middle to move away
	// from — and the per-row form is the `explode` channel.
	Explode float64 `json:"explode,omitempty"`
	// Closed joins a connected mark's last point back to its first, which is
	// what makes a radar a contour. Vega-Lite has no equivalent, so no name is
	// borrowed for it.
	Closed bool `json:"closed,omitempty"`

	// The distribution marks' own properties. Vega-Lite reaches most of this
	// through transforms — `bin`, `density`, `loess`, `regression` — which
	// refract runs inside the layer, so the numbers that configure them travel
	// with the mark. See docs/adr/0014-json-spec.md on spelling a difference out
	// rather than disguising it.
	//
	// Bins is how many bins a histogram divides its column into, and BinStart
	// and BinEnd pin the interval it covers.
	Bins     int      `json:"bins,omitempty"`
	BinStart *float64 `json:"binStart,omitempty"`
	BinEnd   *float64 `json:"binEnd,omitempty"`
	// Bandwidth is the kernel width a violin or a ridgeline estimates with, in
	// the data's own units. Vega-Lite's density transform spells it the same
	// way.
	Bandwidth float64 `json:"bandwidth,omitempty"`
	// Span is the fraction of the rows one local fit of a trend sees, and
	// Method how it fits: "loess" or "linear". Vega-Lite's loess transform
	// spells the first the same way.
	Span   float64 `json:"span,omitempty"`
	Method string  `json:"method,omitempty"`
	// Overlap is how far a ridgeline's tallest ridge rises, in slots of its
	// categorical axis.
	Overlap float64 `json:"overlap,omitempty"`

	// Extra is what a mark this package does not define carries as its own
	// properties. They are written beside the fields above, at the same level
	// of the mark object, and read back into [geom.Desc.Extra] for the builder
	// the mark was registered with — see [geom.Register] and [geom.Extra]. A
	// key that names one of the fields above is an error, not an override.
	Extra map[string]any `json:"-"`
}

// Coord is the coordinate system the chart is drawn in.
//
// It is refract's own field, named plainly rather than smuggled through a
// borrowed one. Vega-Lite has no coordinate stage: it reaches a pie with an
// `arc` mark, and a document that wrote one would round-trip into a mark
// refract cannot rebuild — so a pie here is `mark: "bar"` with
// `coord: {"type": "polar"}`, which is exactly what it is. See
// docs/adr/0014-json-spec.md for why a difference is spelled out rather than
// disguised.
//
// The field is absent for a Cartesian chart, which is every chart written
// before there was a coord to write.
type Coord struct {
	// Type is "cartesian", "polar" or "smith".
	Type string `json:"type"`
	// Theta is the axis a polar coord sweeps around the circle: "x" or "y".
	Theta string `json:"theta,omitempty"`
	// Hole is the inner radius as a fraction of the outer one: a donut.
	Hole float64 `json:"hole,omitempty"`
	// Radius is how much of the panel's shorter half-side the circle fills.
	Radius float64 `json:"radius,omitempty"`
	// Start is where the angular scale begins, in radians clockwise from
	// twelve o'clock, and Sweep how much of the circle it covers. Sweep is a
	// pointer because a chart that never asked writes no field, while one that
	// asked for a full turn should keep saying so.
	Start float64  `json:"start,omitempty"`
	Sweep *float64 `json:"sweep,omitempty"`
	// Counterclockwise reverses the direction the angular scale runs in.
	Counterclockwise bool `json:"counterclockwise,omitempty"`
	// Admittance mirrors a Smith chart through its centre — Γ ↦ −Γ — so that
	// the pair reads as a conductance and a susceptance. It is the Y chart.
	Admittance bool `json:"admittance,omitempty"`
	// Edge is how an edge between two marks is drawn: "arc" or "chord".
	//
	// An absent field is the coord's own default, which is not the same answer
	// for both: an arc under a polar coord, because that is what a rose petal's
	// side is, and a chord under a Smith one, because that is what a measured
	// locus is. So a document that names a type and nothing else draws what the
	// constructor of that type draws.
	Edge string `json:"edge,omitempty"`
}

// The coord edge policies.
const (
	EdgeArc   = "arc"
	EdgeChord = "chord"
)

// The angular axes.
const (
	ThetaX = "x"
	ThetaY = "y"
)

// Encoding maps channels onto columns, values and scales.
type Encoding struct {
	X     *Channel `json:"x,omitempty"`
	Y     *Channel `json:"y,omitempty"`
	X2    *Channel `json:"x2,omitempty"`
	Y2    *Channel `json:"y2,omitempty"`
	Color *Channel `json:"color,omitempty"`

	// Detail is the series column: the channel that splits a layer into groups
	// without saying anything about how they look. It is Vega-Lite's own name
	// for exactly that, and a layer that also colours by the column carries it
	// in both places, because the two say different things — one is what makes
	// the series, the other is what paints them.
	Detail *Channel `json:"detail,omitempty"`

	// Width is refract's: the column a bar takes its width from. Vega-Lite has
	// no equivalent channel, so no name is borrowed for it.
	Width *Channel `json:"width,omitempty"`

	// Explode is refract's too: the column each mark's break-out is read from,
	// which is how one slice leaves a donut and the rest stay in it. The
	// constant form is the mark's own `explode` property.
	Explode *Channel `json:"explode,omitempty"`

	// Text is the column a text layer reads its labels from. Vega-Lite has the
	// same channel with the same name, and it is what tells a text mark with
	// data apart from a note placed at literal values — the way a field tells
	// a rect apart from a region.
	Text *Channel `json:"text,omitempty"`

	// YSecondary and XSecondary are the chart's secondary axes: the scales a
	// layer whose mark names `"yAxis": "y2"` or `"xAxis": "x2"` is drawn
	// against, and the titles written down the chart's right-hand side and
	// along its top. They are only ever set on the top-level encoding — a
	// layer has one channel per direction, and which axis it reads is the
	// mark's business rather than the channel's.
	//
	// They are not spelled `x2` and `y2` because those names are already the
	// *layer* channels for the far end of a band, and two things called y2 in
	// one document is how a reader ends up with a chart that draws neither.
	YSecondary *Channel `json:"ySecondary,omitempty"`
	XSecondary *Channel `json:"xSecondary,omitempty"`

	// Size is the column a mark takes its size from — the bubble chart's third
	// dimension. Vega-Lite has the same channel with the same name; what is
	// refract's is that the scale behind it is read as an *area*, which the
	// scale's `type: "size"` says.
	Size *Channel `json:"size,omitempty"`

	// Mid is the column an error bar marks its measurement at, inside the
	// interval its positional channels describe. Error and ErrorX are the
	// symmetric spelling: a column of half-widths about the value on that
	// axis. All three are refract's, so no Vega-Lite name is borrowed —
	// Vega-Lite reaches the same picture with an `errorbar` mark and an
	// aggregate transform, which refract does not have because a stat runs in
	// the layer.
	Mid    *Channel `json:"mid,omitempty"`
	Error  *Channel `json:"error,omitempty"`
	ErrorX *Channel `json:"errorX,omitempty"`
}

// Channel is one encoding: a column, or a literal value, and the scale behind
// it.
type Channel struct {
	Field string `json:"field,omitempty"`
	Type  string `json:"type,omitempty"`
	// Datum is the literal value an annotation is placed at: a number, or a
	// timestamp string on a temporal axis. Vega-Lite's `datum` is the same
	// shape and there for the same reason.
	Datum any    `json:"datum,omitempty"`
	Title string `json:"title,omitempty"`
	Scale *Scale `json:"scale,omitempty"`

	// Stack is the position adjustment applied along this channel: "zero",
	// "normalize", "center", "wiggle", or "none" for groups drawn from a
	// common baseline. Vega-Lite puts it here too, and spells the first three
	// the same way; "wiggle" is Vega's name for the streamgraph offset and
	// "none" is refract's spelling of Vega-Lite's `null`, which is a JSON null
	// rather than an absent field and would read as "not set" here.
	Stack string `json:"stack,omitempty"`
}

// Scale is a positional or colour scale.
type Scale struct {
	Type   string `json:"type,omitempty"`
	Domain []any  `json:"domain,omitempty"`
	Nice   bool   `json:"nice,omitempty"`
	Zero   bool   `json:"zero,omitempty"`
	// TickValues pins a linear axis's tick positions. Vega-Lite spells this
	// `axis.values`; refract has no axis object on a channel, and `values` on a
	// scale would read as a domain, so it is named for what it pins.
	TickValues []float64 `json:"tickValues,omitempty"`
	Base       float64   `json:"base,omitempty"`
	// Constant is Vega-Lite's name for a symlog's linear threshold.
	Constant float64  `json:"constant,omitempty"`
	Padding  *float64 `json:"padding,omitempty"`
	Scheme   string   `json:"scheme,omitempty"`
	Range    []string `json:"range,omitempty"`
	Reverse  bool     `json:"reverse,omitempty"`

	// SizeRange is the diameters a size scale's domain maps onto, in device
	// units, when the chart pinned them rather than leaving them to the theme.
	// Vega-Lite writes a size scale's range as a plain `range` of two numbers;
	// this is a separate field because `range` here is already the list of
	// colours a colour scale carries.
	SizeRange []float32 `json:"sizeRange,omitempty"`
	// SizeZero is the value a size scale gives its smallest mark, when it is
	// not zero. Anchoring anywhere but zero stops the drawing being a
	// proportion, so it is written out rather than assumed.
	SizeZero *float64 `json:"sizeZero,omitempty"`

	// Format is how this axis writes its tick labels, and Locale the language
	// it writes them in.
	//
	// A numeric scale reads Format as a number format —
	// [github.com/timzifer/refract/scale.NumberFormat] gives the grammar,
	// which is "#" for the number with an optional group comma, decimals,
	// style letter, and any literal text around it: "€ #,.2". A time scale
	// reads it as a Go reference layout, which is
	// [github.com/timzifer/refract/scale.TimeLayout]. One field rather than
	// two because the scale's own type already says which of the two an axis
	// is, and Vega-Lite spells both of its equivalents `format` as well —
	// there on the axis, which refract has no object for.
	//
	// Locale is a name, resolved against what the reading process registered
	// through [github.com/timzifer/refract/scale.RegisterLocale]. A name
	// nothing registered draws in English and is written back out unchanged,
	// so a document does not lose what it asked for by passing through a
	// process that cannot honour it.
	Format string `json:"format,omitempty"`
	Locale string `json:"locale,omitempty"`

	// MinorTicks, Center, Undefined, TimeZone and Origin are refract's.
	MinorTicks *bool    `json:"minorTicks,omitempty"`
	Center     *float64 `json:"center,omitempty"`
	Undefined  string   `json:"undefined,omitempty"`
	TimeZone   string   `json:"timeZone,omitempty"`

	// Origin is the instant a time scale measures its domain from, written as
	// a timestamp. It is refract's own — Vega-Lite has no equivalent because
	// it has no float64 domain to run out of precision — and it is spelled out
	// rather than dropped because it says what the numbers on that axis mean.
	// See [github.com/timzifer/refract/scale.Origin].
	Origin string `json:"origin,omitempty"`
}

// Data is a table written inline.
//
// Values is one object per row; a missing or null field is a NaN, which is the
// same thing the missing-data policy already handles. Format.Parse gives every
// column's type, so a table reads back as the columns it was — Vega-Lite has
// the same field for the same reason.
type Data struct {
	Values []map[string]any `json:"values"`
	Format *Format          `json:"format,omitempty"`
}

// Format carries the column types.
type Format struct {
	// Parse maps a column name to "number", "date" or "string".
	Parse map[string]string `json:"parse,omitempty"`
}

// The column types Format.Parse uses.
const (
	ParseNumber = "number"
	ParseDate   = "date"
	ParseString = "string"
)

// Facet describes small multiples: a field to wrap on, or a row and a column
// field to cross.
type Facet struct {
	Field  string      `json:"field,omitempty"`
	Type   string      `json:"type,omitempty"`
	Row    *FacetField `json:"row,omitempty"`
	Column *FacetField `json:"column,omitempty"`
}

// TrackDoc is one track in a document.
//
// A track's marks are layers like any other, and its own scale is an axis
// channel like any other; what is particular to a track is which edge it is on
// and how much room it takes.
type TrackDoc struct {
	Edge     string   `json:"edge,omitempty"`
	Size     float32  `json:"size,omitempty"`
	Fraction float32  `json:"fraction,omitempty"`
	Scale    *Channel `json:"scale,omitempty"`
	Layer    []Layer  `json:"layer,omitempty"`
	NoAxis   bool     `json:"noAxis,omitempty"`
	Grid     bool     `json:"grid,omitempty"`
}

// FacetField is one axis of a facet grid.
type FacetField struct {
	Field string `json:"field,omitempty"`
	Type  string `json:"type,omitempty"`
}

// Resolve says whether panels share their scales. It carries Vega-Lite's
// "independent" and "shared".
type Resolve struct {
	Scale *ResolveScale `json:"scale,omitempty"`
}

// ResolveScale is the per-axis resolution.
type ResolveScale struct {
	X string `json:"x,omitempty"`
	Y string `json:"y,omitempty"`
}

// The scale resolutions.
const (
	Independent = "independent"
	Shared      = "shared"
)

// Config carries the choices that are refract's rather than the chart's.
type Config struct {
	// Theme names a theme registered with [theme.Register].
	Theme string `json:"theme,omitempty"`
	// Legend forces the legend on or off.
	Legend *bool `json:"legend,omitempty"`
	// DevicePixelRatio is the backend's pixel ratio.
	DevicePixelRatio float64 `json:"devicePixelRatio,omitempty"`
}

// Marshal writes s as indented JSON.
//
// Indented rather than compact: a spec is a thing people read and edit, and
// the compact form of a chart over a hundred rows is one very long line. Use
// [encoding/json] directly for the compact form.
func (s Spec) Marshal() ([]byte, error) { return json.MarshalIndent(s, "", "  ") }

// Parse reads a spec from JSON.
func Parse(b []byte) (Spec, error) {
	var s Spec
	if err := json.Unmarshal(b, &s); err != nil {
		return Spec{}, fmt.Errorf("refract/spec: %w", err)
	}
	return s, nil
}
