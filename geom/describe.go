package geom

import (
	"fmt"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Mark names what a layer draws. It is the one word that decides which
// constructor built a layer, and the hinge a serialized chart turns on.
type Mark string

// The marks. These are refract's own names for its layers; the JSON spec
// translates them into its own vocabulary rather than the other way round, so
// that this package stays ignorant of any wire format.
const (
	MarkLine    Mark = "line"
	MarkScatter Mark = "scatter"
	MarkBar     Mark = "bar"
	MarkArea    Mark = "area"
	MarkStep    Mark = "step"
	MarkBoxplot Mark = "boxplot"

	MarkHLine   Mark = "hline"
	MarkVLine   Mark = "vline"
	MarkHBand   Mark = "hband"
	MarkVBand   Mark = "vband"
	MarkSegment Mark = "segment"
	MarkRegion  Mark = "region"
	MarkNote    Mark = "note"
)

// Datum carries the values an annotation is placed by. A rule uses X0 or Y0
// alone, a band a pair on one axis, a segment and a region all four.
type Datum struct {
	X0, Y0, X1, Y1 float64
}

// Desc is a layer reduced to what configures it: its mark, its data, the
// columns it reads and the options it was given.
//
// It exists so that a chart can be written down and read back. A [Geom] is an
// interface with three methods and no way to ask it what it is, which is right
// for drawing and useless for serialization — so every layer in this package
// answers [Describer], and [FromDesc] turns the answer back into a layer that
// draws the same marks.
//
// A field left at its zero value means "not set", exactly as leaving the
// corresponding [Option] out does. The fields that have a non-zero default —
// BarWidth, Whisker, Outliers, Opacity, Extend — are filled in by [Describe]
// with the default the layer is actually using, so a Desc is complete rather
// than partial.
type Desc struct {
	// Mark is what the layer draws.
	Mark Mark
	// Source is the layer's data. It is nil for an annotation, which takes
	// values rather than columns.
	Source data.Source

	// X, Y and Y2 name the columns mapped to the axes. ColorCol and
	// ColorScale are [ColorBy]'s two halves.
	X, Y, Y2   string
	ColorCol   string
	ColorScale scale.ColorScale

	// Label names the layer in the legend.
	Label string

	// Datum places an annotation, and Text is a note's text. Both are unused
	// by a layer that has a Source.
	Datum Datum
	Text  string

	// The styling options, one field per [Option]. A nil Color or Fill means
	// the layer takes its colour from the palette.
	Color   *ir.Color
	Fill    *ir.Color
	Width   float32
	Dash    []float32
	DashSet bool
	Tension float64
	Missing Missing
	// Marker is the shape a scatter draws, and MarkerSet reports whether the
	// layer chose it. The pair is [Dash] and DashSet again, and for the same
	// reason: a circle is both the zero value and a shape somebody may have
	// asked for, and a theme's redundant encoding replaces the first but not
	// the second.
	Marker    ir.Marker
	MarkerSet bool
	Size      float32
	BarWidth  float64
	Baseline  float64
	Opacity   float64
	Steps     StepPos
	Whisker   float64
	Outliers  bool
	Decimate  Decimation
	Budget    int
	CellSize  float64
	FontSize  float64
	HAlign    ir.HAlign
	VAlign    ir.VAlign
	Rotation  float64
	Extend    bool
}

// Describer is implemented by a layer that can say what it is.
//
// It is an optional interface, like [Faceter] and [Guided]: a third-party geom
// that does not implement it still draws, and is simply not serializable. That
// is a better failure than a half-written spec — see
// [github.com/timzifer/refract/spec].
type Describer interface {
	// Describe returns the layer's configuration.
	Describe() Desc
}

// Describe reports g's configuration, or ok == false if g cannot describe
// itself.
func Describe(g Geom) (Desc, bool) {
	d, ok := g.(Describer)
	if !ok {
		return Desc{}, false
	}
	return d.Describe(), true
}

// ErrUnknownMark reports a Desc naming a mark this package does not have.
var ErrUnknownMark = fmt.Errorf("refract/geom: unknown mark")

// FromDesc builds the layer d describes.
//
// It is the inverse of [Describe] over every layer in this package: describing
// a layer and rebuilding it produces one that draws the same marks. A layer
// with data needs a Source; an annotation ignores one.
func FromDesc(d Desc) (Geom, error) {
	opts := d.options()
	switch d.Mark {
	case MarkHLine:
		return HLine(d.Datum.Y0, opts...), nil
	case MarkVLine:
		return VLine(d.Datum.X0, opts...), nil
	case MarkHBand:
		return HBand(d.Datum.Y0, d.Datum.Y1, opts...), nil
	case MarkVBand:
		return VBand(d.Datum.X0, d.Datum.X1, opts...), nil
	case MarkSegment:
		return Segment(d.Datum.X0, d.Datum.Y0, d.Datum.X1, d.Datum.Y1, opts...), nil
	case MarkRegion:
		return Region(d.Datum.X0, d.Datum.Y0, d.Datum.X1, d.Datum.Y1, opts...), nil
	case MarkNote:
		return Note(d.Datum.X0, d.Datum.Y0, d.Text, opts...), nil
	}

	if d.Source == nil {
		return nil, fmt.Errorf("refract/geom: a %s layer needs a data source", d.Mark)
	}
	switch d.Mark {
	case MarkLine:
		return Line(d.Source, opts...), nil
	case MarkScatter:
		return Scatter(d.Source, opts...), nil
	case MarkBar:
		return Bar(d.Source, opts...), nil
	case MarkArea:
		return Area(d.Source, opts...), nil
	case MarkStep:
		return Step(d.Source, opts...), nil
	case MarkBoxplot:
		return Boxplot(d.Source, opts...), nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownMark, d.Mark)
}

// options turns a Desc back into the option list that would have produced it.
//
// Every option is applied rather than only the ones that differ from a
// default: an option set to its default value is the default value, and
// filtering would only be a second place for the defaults to be written down.
func (d Desc) options() []Option {
	opts := []Option{
		OnMissing(d.Missing),
		Size(d.Size),
		Width(d.Width),
		Tension(d.Tension),
		BarWidth(d.BarWidth),
		Baseline(d.Baseline),
		Opacity(d.Opacity),
		Steps(d.Steps),
		Whisker(d.Whisker),
		Outliers(d.Outliers),
		Decimate(d.Decimate),
		Budget(d.Budget),
		DensityCells(d.CellSize),
		FontSize(d.FontSize),
		Align(d.HAlign, d.VAlign),
		Rotate(d.Rotation),
		Extend(d.Extend),
	}
	if d.X != "" {
		opts = append(opts, X(d.X))
	}
	if d.Y != "" {
		opts = append(opts, Y(d.Y))
	}
	if d.Y2 != "" {
		opts = append(opts, Y2(d.Y2))
	}
	if d.Label != "" {
		opts = append(opts, Label(d.Label))
	}
	if d.Color != nil {
		opts = append(opts, Color(*d.Color))
	}
	if d.Fill != nil {
		opts = append(opts, Fill(*d.Fill))
	}
	if d.DashSet {
		opts = append(opts, Dash(d.Dash...))
	}
	if d.MarkerSet {
		opts = append(opts, Shape(d.Marker))
	}
	if d.ColorCol != "" && d.ColorScale != nil {
		opts = append(opts, ColorBy(d.ColorCol, d.ColorScale))
	}
	return opts
}

// describe fills in everything a Desc takes from the shared option set. Each
// layer adds its mark, its source and — for an annotation — its values.
func (c config) describe(mark Mark) Desc {
	return Desc{
		Mark:       mark,
		X:          c.xcol,
		Y:          c.ycol,
		Y2:         c.y2col,
		ColorCol:   c.colorCol,
		ColorScale: c.colorScale,
		Label:      c.label,
		Color:      c.color,
		Fill:       c.fill,
		Width:      c.width,
		Dash:       c.dash,
		DashSet:    c.dashSet,
		Tension:    c.tension,
		Missing:    c.missing,
		Marker:     c.marker,
		MarkerSet:  c.markerSet,
		Size:       c.size,
		BarWidth:   c.barWidth,
		Baseline:   c.baseline,
		Opacity:    c.opacity,
		Steps:      c.steps,
		Whisker:    c.whisker,
		Outliers:   c.outliers,
		Decimate:   c.decimate,
		Budget:     c.budget,
		CellSize:   c.cellSize,
		FontSize:   c.fontSize,
		HAlign:     c.halign,
		VAlign:     c.valign,
		Rotation:   c.rotation,
		Extend:     c.extend,
	}
}

func (g *lineGeom) Describe() Desc {
	d := g.cfg.describe(MarkLine)
	d.Source = g.src
	return d
}

func (g *scatterGeom) Describe() Desc {
	d := g.cfg.describe(MarkScatter)
	d.Source = g.src
	return d
}

func (g *barGeom) Describe() Desc {
	d := g.cfg.describe(MarkBar)
	d.Source = g.src
	return d
}

func (g *areaGeom) Describe() Desc {
	d := g.cfg.describe(MarkArea)
	d.Source = g.src
	return d
}

func (g *stepGeom) Describe() Desc {
	d := g.cfg.describe(MarkStep)
	d.Source = g.src
	return d
}

func (g *boxGeom) Describe() Desc {
	d := g.cfg.describe(MarkBoxplot)
	d.Source = g.src
	return d
}

func (g *ruleGeom) Describe() Desc {
	if g.vertical {
		d := g.cfg.describe(MarkVLine)
		d.Datum.X0 = g.at
		return d
	}
	d := g.cfg.describe(MarkHLine)
	d.Datum.Y0 = g.at
	return d
}

func (g *bandGeom) Describe() Desc {
	if g.vertical {
		d := g.cfg.describe(MarkVBand)
		d.Datum.X0, d.Datum.X1 = g.lo, g.hi
		return d
	}
	d := g.cfg.describe(MarkHBand)
	d.Datum.Y0, d.Datum.Y1 = g.lo, g.hi
	return d
}

func (g *segmentGeom) Describe() Desc {
	d := g.cfg.describe(MarkSegment)
	d.Datum = Datum{X0: g.x0, Y0: g.y0, X1: g.x1, Y1: g.y1}
	return d
}

func (g *regionGeom) Describe() Desc {
	d := g.cfg.describe(MarkRegion)
	d.Datum = Datum{X0: g.x0, Y0: g.y0, X1: g.x1, Y1: g.y1}
	return d
}

func (g *noteGeom) Describe() Desc {
	d := g.cfg.describe(MarkNote)
	d.Datum = Datum{X0: g.x, Y0: g.y}
	d.Text = g.text
	return d
}

var (
	_ Describer = (*lineGeom)(nil)
	_ Describer = (*scatterGeom)(nil)
	_ Describer = (*barGeom)(nil)
	_ Describer = (*areaGeom)(nil)
	_ Describer = (*stepGeom)(nil)
	_ Describer = (*boxGeom)(nil)
	_ Describer = (*ruleGeom)(nil)
	_ Describer = (*bandGeom)(nil)
	_ Describer = (*segmentGeom)(nil)
	_ Describer = (*regionGeom)(nil)
	_ Describer = (*noteGeom)(nil)
)
