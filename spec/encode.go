package spec

import (
	"fmt"
	"time"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
)

// Of writes a chart down.
//
// It fails rather than guesses. A layer or a scale that cannot describe itself
// — a third-party one that implements [geom.Describer] or [scale.Describer]
// nowhere — is an error, because a spec missing a layer is a spec that draws a
// different chart, and finding that out at read time is worse than finding it
// out here.
func Of(c Chart) (Spec, error) {
	s := Spec{
		Schema: Schema,
		Width:  c.Width,
		Height: c.Height,
		Title:  c.Title,
	}

	enc := &Encoding{}
	var err error
	if enc.X, err = axisChannel(c.X, c.XTitle); err != nil {
		return Spec{}, fmt.Errorf("refract/spec: x axis: %w", err)
	}
	if enc.Y, err = axisChannel(c.Y, c.YTitle); err != nil {
		return Spec{}, fmt.Errorf("refract/spec: y axis: %w", err)
	}
	if enc.X != nil || enc.Y != nil {
		s.Encoding = enc
	}

	if s.Coord, err = encodeCoord(c.Coord); err != nil {
		return Spec{}, err
	}

	shared, hoist := commonSource(c.Layers)
	if hoist {
		if s.Data, err = encodeData(shared); err != nil {
			return Spec{}, err
		}
	}
	axes := axisKinds{x: kindOf(c.X), y: kindOf(c.Y)}
	for i, g := range c.Layers {
		l, err := encodeLayer(g, hoist, axes)
		if err != nil {
			return Spec{}, fmt.Errorf("refract/spec: layer %d: %w", i, err)
		}
		s.Layer = append(s.Layer, l)
	}

	if c.Facet != nil {
		fd := c.Facet.Describe()
		if fd.Wrap {
			s.Facet = &Facet{Field: fd.Col, Type: "nominal"}
			s.Columns = fd.Columns
		} else {
			s.Facet = &Facet{
				Row:    &FacetField{Field: fd.Row, Type: "nominal"},
				Column: &FacetField{Field: fd.Col, Type: "nominal"},
			}
		}
		if fd.FreeX || fd.FreeY {
			s.Resolve = &Resolve{Scale: &ResolveScale{
				X: resolution(fd.FreeX),
				Y: resolution(fd.FreeY),
			}}
		}
	}

	cfg := &Config{Legend: c.Legend}
	if c.Theme.Name != "" {
		cfg.Theme = c.Theme.Name
	}
	if c.DPR != 0 && c.DPR != 1 {
		cfg.DevicePixelRatio = c.DPR
	}
	if *cfg != (Config{}) {
		s.Config = cfg
	}
	return s, nil
}

func resolution(free bool) string {
	if free {
		return Independent
	}
	return Shared
}

// axisChannel writes the plot's scale for one axis.
func axisChannel(s scale.Scale, title string) (*Channel, error) {
	if s == nil {
		if title == "" {
			return nil, nil
		}
		return &Channel{Title: title}, nil
	}
	d, ok := scale.Describe(s)
	if !ok {
		return nil, fmt.Errorf("%T cannot describe itself: it does not implement scale.Describer", s)
	}
	return &Channel{Type: channelType(d.Kind), Title: title, Scale: encodeScale(d)}, nil
}

// channelType is Vega-Lite's measurement type for a scale kind.
func channelType(k scale.Kind) string {
	switch k {
	case scale.KindTime:
		return "temporal"
	case scale.KindOrdinal:
		return "nominal"
	}
	return "quantitative"
}

func encodeScale(d scale.Desc) *Scale {
	out := &Scale{Nice: d.Nice, Zero: d.Zero}
	switch d.Kind {
	case scale.KindLinear:
		out.Type = "linear"
	case scale.KindLog:
		out.Type, out.Base = "log", d.Base
		out.MinorTicks = boolPtr(d.MinorTicks)
	case scale.KindSymLog:
		out.Type, out.Base, out.Constant = "symlog", d.Base, d.Threshold
		out.MinorTicks = boolPtr(d.MinorTicks)
	case scale.KindTime:
		out.Type, out.TimeZone = "time", d.Location
		if d.Origin != 0 {
			out.Origin = time.Unix(0, d.Origin).UTC().Format(timeLayout)
		}
	case scale.KindOrdinal:
		// "band" is Vega-Lite's name for a scale that gives each category a
		// slot of finite width, which is what this one does — see scale.Band.
		out.Type = "band"
		out.Padding = float64Ptr(d.Padding)
		for _, c := range d.Categories {
			out.Domain = append(out.Domain, c)
		}
	}
	if d.Fixed {
		switch d.Kind {
		case scale.KindTime:
			// The bounds are written as instants, so they are read back
			// against whatever origin the document carries rather than
			// against the one this axis happens to have.
			out.Domain = []any{
				time.Unix(0, d.Origin+int64(d.Min)).UTC().Format(timeLayout),
				time.Unix(0, d.Origin+int64(d.Max)).UTC().Format(timeLayout),
			}
		case scale.KindOrdinal:
			// The categories are already the domain.
		default:
			out.Domain = []any{d.Min, d.Max}
		}
	}
	return out
}

func encodeColorScale(cs scale.ColorScale) (*Scale, error) {
	d, ok := scale.DescribeColor(cs)
	if !ok {
		return nil, fmt.Errorf("%T cannot describe itself: it does not implement scale.ColorDescriber", cs)
	}
	out := &Scale{Type: string(d.Kind), Scheme: d.Ramp, Reverse: d.Reverse}
	for _, c := range d.Colors {
		out.Range = append(out.Range, colorHex(c))
	}
	if d.Fixed && d.Kind != scale.KindQualitative {
		// A discrete scale's domain is the labels it has been shown, and those
		// are the data rather than the scale — the same line an ordinal axis
		// draws between a fixed category set and a discovered one.
		out.Domain = []any{d.Min, d.Max}
	}
	if d.Kind == scale.KindDiverging {
		out.Center = float64Ptr(d.Center)
	}
	if d.Undefined.A != 0 {
		out.Undefined = colorHex(d.Undefined)
	}
	return out, nil
}

// colorChannelType is the measurement type of a colour encoding: a category
// for a discrete scale, a quantity for a ramp. Vega-Lite draws the same
// distinction with the same two words.
func colorChannelType(s *Scale) string {
	if s != nil && s.Type == string(scale.KindQualitative) {
		return "nominal"
	}
	return "quantitative"
}

// commonSource reports the source every layer with data draws from, and
// whether there is exactly one.
//
// A chart usually has one table behind all its layers, and writing it once is
// the difference between a readable document and the same million rows three
// times over.
func commonSource(layers []geom.Geom) (data.Source, bool) {
	var first data.Source
	for _, g := range layers {
		d, ok := geom.Describe(g)
		if !ok || d.Source == nil {
			continue
		}
		if first == nil {
			first = d.Source
			continue
		}
		if !sameSource(first, d.Source) {
			return nil, false
		}
	}
	return first, first != nil
}

func encodeLayer(g geom.Geom, hoisted bool, axes axisKinds) (Layer, error) {
	d, ok := geom.Describe(g)
	if !ok {
		return Layer{}, fmt.Errorf("%T cannot describe itself: it does not implement geom.Describer", g)
	}
	typ, orient, err := markType(d.Mark)
	if err != nil {
		return Layer{}, err
	}

	m := Mark{Type: typ, Orient: orient}
	if d.Color != nil {
		m.Color = colorHex(*d.Color)
	}
	writeMarkProps(&m, d)

	l := Layer{Name: d.Label, Mark: m}
	if l.Encoding, err = encodeLayerEncoding(d, axes); err != nil {
		return Layer{}, err
	}
	if d.Source != nil && !hoisted {
		if l.Data, err = encodeData(d.Source); err != nil {
			return Layer{}, err
		}
	}
	return l, nil
}

// writeMarkProps writes the properties the mark actually uses, and no others.
//
// A geom accepts every option and ignores the ones it has no use for, which is
// what keeps the option set one namespace instead of six — but a *document*
// listing a line's whisker extent and bar width is a document that reads as
// though those meant something. So the mark decides what is written, and the
// round trip is unaffected: an option the mark ignores draws nothing either
// way.
func writeMarkProps(m *Mark, d geom.Desc) {
	stroke := func() {
		m.StrokeWidth = d.Width
		if d.DashSet {
			m.StrokeDash = d.Dash
			if m.StrokeDash == nil {
				// An explicit geom.Dash() with no pattern means solid, and an
				// absent field means "not set". Those are different, so the
				// empty list has to survive.
				m.StrokeDash = []float32{}
			}
		}
	}
	fill := func() {
		if d.Fill != nil {
			m.Fill = colorHex(*d.Fill)
		}
		if d.Opacity >= 0 {
			m.Opacity = float64Ptr(d.Opacity)
		}
	}
	// The default policy and the default reduction are written only when they
	// are not the default: a document should say what was chosen, not repeat
	// what was not.
	rows := func() {
		if d.Missing != geom.Gap {
			m.Missing = missingName(d.Missing)
		}
		if d.Decimate != geom.AutoDecimation {
			m.Decimate = decimationName(d.Decimate)
		}
		m.Budget = d.Budget
	}
	// The adjustment is written by the marks that have one. The stack itself
	// goes on the positional channel, where Vega-Lite puts it and where this
	// function cannot reach; what is left here is the pair of choices that are
	// about the marks rather than about the axis.
	adjust := func() {
		if !d.Dodge {
			return
		}
		m.Dodge = float64Ptr(d.DodgePad)
	}
	group := func() {
		adjust()
		if d.Group != "" && d.Order != geom.OrderAppearance {
			m.Order = orderName(d.Order)
		}
	}

	// A contour that joins back to its start is a property of the connected
	// marks and of nothing else, so it is written by those three and not by
	// the six that would ignore it.
	loop := func() {
		m.Closed = d.Closed
	}

	switch d.Mark {
	case geom.MarkLine:
		stroke()
		rows()
		group()
		loop()
		if d.Tension > 0 {
			m.Interpolate, m.Tension = "cardinal", d.Tension
		}
	case geom.MarkStep:
		stroke()
		rows()
		group()
		loop()
		m.Interpolate = stepName(d.Steps)
	case geom.MarkScatter:
		stroke()
		fill()
		rows()
		group()
		m.Size = d.Size
		// The shape is written when the layer chose one. Left out, the mark is
		// a circle unless the theme's redundant encoding has an opinion — and
		// a document that spelled "circle" out would pin it and lose that.
		if d.MarkerSet {
			m.Shape = shapeName(d.Marker)
		}
		m.DensityCells = d.CellSize
	case geom.MarkBar:
		stroke()
		fill()
		rows()
		group()
		m.BarWidth, m.Origin = float64Ptr(d.BarWidth), d.Baseline
	case geom.MarkRect:
		stroke()
		fill()
		m.BarWidth = float64Ptr(d.BarWidth)
	case geom.MarkArea:
		stroke()
		fill()
		rows()
		group()
		loop()
		m.Origin = d.Baseline
		if d.Tension > 0 {
			m.Interpolate, m.Tension = "cardinal", d.Tension
		}
	case geom.MarkBoxplot:
		stroke()
		fill()
		rows()
		m.BarWidth = float64Ptr(d.BarWidth)
		m.Extent, m.Outliers = d.Whisker, boolPtr(d.Outliers)
	case geom.MarkHLine, geom.MarkVLine, geom.MarkSegment:
		stroke()
		m.Extend = boolPtr(d.Extend)
	case geom.MarkHBand, geom.MarkVBand, geom.MarkRegion:
		stroke()
		fill()
		m.Extend = boolPtr(d.Extend)
	case geom.MarkNote:
		m.Text, m.FontSize, m.Angle = d.Text, d.FontSize, degrees(d.Rotation)
		m.Align, m.Baseline = hAlignName(d.HAlign), vAlignName(d.VAlign)
		m.Extend = boolPtr(d.Extend)
	}
}

func encodeLayerEncoding(d geom.Desc, axes axisKinds) (*Encoding, error) {
	enc := &Encoding{}
	if d.Source != nil {
		if d.X != "" {
			enc.X = &Channel{Field: d.X}
		}
		if d.Y != "" {
			enc.Y = &Channel{Field: d.Y}
		}
		if d.X2 != "" {
			enc.X2 = &Channel{Field: d.X2}
		}
		if d.Y2 != "" {
			enc.Y2 = &Channel{Field: d.Y2}
		}
		if d.ColorCol != "" && d.ColorScale != nil {
			cs, err := encodeColorScale(d.ColorScale)
			if err != nil {
				return nil, err
			}
			enc.Color = &Channel{Field: d.ColorCol, Type: colorChannelType(cs), Scale: cs}
		}
		if d.Group != "" {
			enc.Detail = &Channel{Field: d.Group, Type: "nominal"}
		}
		if d.WidthCol != "" {
			enc.Width = &Channel{Field: d.WidthCol}
		}
		// The stack is a property of the axis the groups are stacked along,
		// which is the Y axis for every mark that has one — so it goes on the
		// Y channel, where Vega-Lite puts it and where a reader will look.
		if d.StackSet && enc.Y != nil {
			enc.Y.Stack = stackName(d.Stack)
		}
		if *enc == (Encoding{}) {
			return nil, nil
		}
		return enc, nil
	}

	// An annotation is placed by values rather than columns, which is what
	// Vega-Lite's `datum` is for.
	switch d.Mark {
	case geom.MarkHLine:
		enc.Y = axes.y.datum(d.Datum.Y0)
	case geom.MarkVLine:
		enc.X = axes.x.datum(d.Datum.X0)
	case geom.MarkHBand:
		enc.Y, enc.Y2 = axes.y.datum(d.Datum.Y0), axes.y.datum(d.Datum.Y1)
	case geom.MarkVBand:
		enc.X, enc.X2 = axes.x.datum(d.Datum.X0), axes.x.datum(d.Datum.X1)
	case geom.MarkSegment, geom.MarkRegion:
		enc.X, enc.Y = axes.x.datum(d.Datum.X0), axes.y.datum(d.Datum.Y0)
		enc.X2, enc.Y2 = axes.x.datum(d.Datum.X1), axes.y.datum(d.Datum.Y1)
	case geom.MarkNote:
		enc.X, enc.Y = axes.x.datum(d.Datum.X0), axes.y.datum(d.Datum.Y0)
	}
	if *enc == (Encoding{}) {
		return nil, nil
	}
	return enc, nil
}

// axisKinds remembers what each axis is, so that a value annotating a time
// axis is written as the timestamp it is rather than as a count of
// nanoseconds nobody can read.
type axisKinds struct{ x, y axisKind }

type axisKind scale.Kind

func (k axisKind) datum(v float64) *Channel {
	if scale.Kind(k) == scale.KindTime {
		return &Channel{Datum: scale.FromNanos(v).UTC().Format(timeLayout)}
	}
	return &Channel{Datum: v}
}

func kindOf(s scale.Scale) axisKind {
	d, ok := scale.Describe(s)
	if !ok {
		return axisKind(scale.KindLinear)
	}
	return axisKind(d.Kind)
}

func boolPtr(b bool) *bool          { return &b }
func float64Ptr(v float64) *float64 { return &v }
