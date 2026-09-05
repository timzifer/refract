package spec

import (
	"fmt"
	"math"
	"time"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// Chart reads a spec back.
//
// `$schema` is not checked. It records the dialect a document was written in
// and is worth having in the file, but refusing to read a chart because of a
// version string would make the field a trap rather than a label; a document
// this package cannot understand fails on the part it cannot understand,
// naming it.
func (s Spec) Chart() (Chart, error) {
	c := Chart{Width: s.Width, Height: s.Height, Theme: theme.Light, DPR: 1}

	if s.Config != nil {
		if s.Config.Theme != "" {
			t, ok := theme.ByName(s.Config.Theme)
			if !ok {
				return Chart{}, fmt.Errorf("refract/spec: unknown theme %q", s.Config.Theme)
			}
			c.Theme = t
		}
		c.Legend = s.Config.Legend
		if s.Config.DevicePixelRatio > 0 {
			c.DPR = s.Config.DevicePixelRatio
		}
	}
	c.Title = s.Title

	var err error
	if s.Encoding != nil {
		if c.X, c.XTitle, err = axisScale(s.Encoding.X); err != nil {
			return Chart{}, fmt.Errorf("refract/spec: x axis: %w", err)
		}
		if c.Y, c.YTitle, err = axisScale(s.Encoding.Y); err != nil {
			return Chart{}, fmt.Errorf("refract/spec: y axis: %w", err)
		}
	}

	if c.Coord, err = decodeCoord(s.Coord); err != nil {
		return Chart{}, err
	}

	shared, err := decodeData(s.Data)
	if err != nil {
		return Chart{}, err
	}
	for i, l := range s.Layer {
		g, err := decodeLayer(l, shared)
		if err != nil {
			return Chart{}, fmt.Errorf("refract/spec: layer %d: %w", i, err)
		}
		c.Layers = append(c.Layers, g)
	}

	if s.Facet != nil {
		d := facet.Desc{Columns: s.Columns}
		switch {
		case s.Facet.Field != "":
			d.Col, d.Wrap = s.Facet.Field, true
		case s.Facet.Row != nil || s.Facet.Column != nil:
			if s.Facet.Row != nil {
				d.Row = s.Facet.Row.Field
			}
			if s.Facet.Column != nil {
				d.Col = s.Facet.Column.Field
			}
		}
		if s.Resolve != nil && s.Resolve.Scale != nil {
			d.FreeX = s.Resolve.Scale.X == Independent
			d.FreeY = s.Resolve.Scale.Y == Independent
		}
		c.Facet = facet.FromDesc(d)
	}
	return c, nil
}

func axisScale(ch *Channel) (scale.Scale, string, error) {
	if ch == nil {
		return nil, "", nil
	}
	if ch.Scale == nil {
		return nil, ch.Title, nil
	}
	d, err := decodeScale(*ch.Scale, ch.Type)
	if err != nil {
		return nil, "", err
	}
	s, err := scale.FromDesc(d)
	if err != nil {
		return nil, "", err
	}
	return s, ch.Title, nil
}

// decodeScale turns a document scale into a [scale.Desc]. The channel's
// measurement type is the fallback for a scale that named no type, which is
// what a hand-written Vega-Lite-style spec usually looks like.
func decodeScale(s Scale, channelType string) (scale.Desc, error) {
	typ := s.Type
	if typ == "" {
		switch channelType {
		case "temporal":
			typ = "time"
		case "nominal", "ordinal":
			typ = "band"
		default:
			typ = "linear"
		}
	}
	d := scale.Desc{Nice: s.Nice, Zero: s.Zero, Base: s.Base, Threshold: s.Constant}
	d.MinorTicks = true
	if s.MinorTicks != nil {
		d.MinorTicks = *s.MinorTicks
	}

	switch typ {
	case "linear":
		d.Kind = scale.KindLinear
	case "log":
		d.Kind = scale.KindLog
	case "symlog":
		d.Kind = scale.KindSymLog
	case "time", "utc":
		d.Kind, d.Location = scale.KindTime, s.TimeZone
		if typ == "utc" && d.Location == "" {
			d.Location = "UTC"
		}
		if s.Origin != "" {
			t, err := time.Parse(timeLayout, s.Origin)
			if err != nil {
				return scale.Desc{}, fmt.Errorf("scale origin %q is not a timestamp: %w", s.Origin, err)
			}
			d.Origin = t.UnixNano()
		}
	case "band", "point", "ordinal":
		d.Kind = scale.KindOrdinal
		d.Padding = 0.2
		if s.Padding != nil {
			d.Padding = *s.Padding
		}
		for _, v := range s.Domain {
			if name, ok := v.(string); ok {
				d.Categories = append(d.Categories, name)
			}
		}
		return d, nil
	default:
		return scale.Desc{}, fmt.Errorf("unknown scale type %q", typ)
	}

	if len(s.Domain) == 2 {
		lo, err := domainValue(s.Domain[0], d.Origin)
		if err != nil {
			return scale.Desc{}, err
		}
		hi, err := domainValue(s.Domain[1], d.Origin)
		if err != nil {
			return scale.Desc{}, err
		}
		d.Min, d.Max, d.Fixed = lo, hi, true
	} else if len(s.Domain) != 0 {
		return scale.Desc{}, fmt.Errorf("a %s domain needs two bounds, got %d", typ, len(s.Domain))
	}
	return d, nil
}

// domainValue reads one domain bound. A bound written as a timestamp is
// measured from origin, which is what makes the numbers on a rebased time axis
// mean the same thing after a round trip as before it — see [scale.Origin].
func domainValue(v any, origin int64) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case string:
		t, err := time.Parse(timeLayout, x)
		if err != nil {
			return 0, fmt.Errorf("%q is not a number or a timestamp", x)
		}
		return float64(t.UnixNano() - origin), nil
	}
	return 0, fmt.Errorf("%v is not a domain bound", v)
}

func decodeLayer(l Layer, shared data.Source) (geom.Geom, error) {
	mark, err := geomMark(l.Mark, l.Encoding)
	if err != nil {
		return nil, err
	}
	d := geom.Desc{
		Mark:      mark,
		Label:     l.Name,
		Size:      l.Mark.Size,
		Width:     l.Mark.StrokeWidth,
		Baseline:  l.Mark.Origin,
		BarWidth:  0.8,
		Whisker:   l.Mark.Extent,
		Outliers:  true,
		Missing:   missingPolicy(l.Mark.Missing),
		Decimate:  decimationMode(l.Mark.Decimate),
		Budget:    l.Mark.Budget,
		CellSize:  l.Mark.DensityCells,
		Text:      l.Mark.Text,
		FontSize:  l.Mark.FontSize,
		Rotation:  radians(l.Mark.Angle),
		Extend:    true,
		Opacity:   -1,
		Tension:   l.Mark.Tension,
		Marker:    markerShape(l.Mark.Shape),
		MarkerSet: l.Mark.Shape != "",
		Closed:    l.Mark.Closed,
		Steps:     stepPos(l.Mark.Interpolate),
		HAlign:    hAlignOf(l.Mark.Align),
		VAlign:    vAlignOf(l.Mark.Baseline),
	}
	if l.Mark.Extent == 0 {
		d.Whisker = 1.5
	}
	if l.Mark.BarWidth != nil {
		d.BarWidth = *l.Mark.BarWidth
	}
	if l.Mark.Outliers != nil {
		d.Outliers = *l.Mark.Outliers
	}
	if l.Mark.Extend != nil {
		d.Extend = *l.Mark.Extend
	}
	if l.Mark.Dodge != nil {
		d.Dodge, d.DodgePad = true, *l.Mark.Dodge
	}
	d.Order = ordering(l.Mark.Order)
	if l.Mark.Opacity != nil {
		d.Opacity = *l.Mark.Opacity
	}
	if l.Mark.Color != "" {
		c, err := parseColor(l.Mark.Color)
		if err != nil {
			return nil, err
		}
		d.Color = &c
	}
	if l.Mark.Fill != "" {
		c, err := parseColor(l.Mark.Fill)
		if err != nil {
			return nil, err
		}
		d.Fill = &c
	}
	if l.Mark.StrokeDash != nil {
		d.Dash, d.DashSet = l.Mark.StrokeDash, true
	}

	if err := decodeLayerEncoding(&d, l.Encoding); err != nil {
		return nil, err
	}

	if d.X != "" || d.Y != "" {
		if d.Source, err = decodeData(l.Data); err != nil {
			return nil, err
		}
		if d.Source == nil {
			d.Source = shared
		}
	}
	return geom.FromDesc(d)
}

func decodeLayerEncoding(d *geom.Desc, enc *Encoding) error {
	if enc == nil {
		return nil
	}
	d.X, d.Y, d.Y2 = fieldOf(enc.X), fieldOf(enc.Y), fieldOf(enc.Y2)
	d.X2 = fieldOf(enc.X2)
	d.Group, d.WidthCol = fieldOf(enc.Detail), fieldOf(enc.Width)
	// The stack rides the channel it adjusts. A document that names none
	// leaves the mark's own default in place, which is why this is a pair
	// rather than a value — see [geom.Desc].
	for _, ch := range [...]*Channel{enc.Y, enc.X} {
		if ch == nil || ch.Stack == "" {
			continue
		}
		s, ok := stacking(ch.Stack)
		if !ok {
			return fmt.Errorf("unknown stack %q", ch.Stack)
		}
		d.Stack, d.StackSet = s, true
		break
	}
	d.Datum = geom.Datum{
		X0: datumOf(enc.X), Y0: datumOf(enc.Y),
		X1: datumOf(enc.X2), Y1: datumOf(enc.Y2),
	}
	// An hband and a vband carry their pair on one axis, where the encoder put
	// them: y/y2 and x/x2. A rule carries one value, and a segment and a
	// region all four, so the mapping above already holds for those.
	switch d.Mark {
	case geom.MarkHBand:
		d.Datum.Y1 = datumOf(enc.Y2)
	case geom.MarkVBand:
		d.Datum.X1 = datumOf(enc.X2)
	}

	if enc.Color != nil && enc.Color.Field != "" {
		if enc.Color.Scale == nil {
			return fmt.Errorf("the colour encoding on %q needs a scale", enc.Color.Field)
		}
		cs, err := decodeColorScale(*enc.Color.Scale)
		if err != nil {
			return err
		}
		d.ColorCol, d.ColorScale = enc.Color.Field, cs
	}
	return nil
}

func fieldOf(ch *Channel) string {
	if ch == nil {
		return ""
	}
	return ch.Field
}

// datumOf reads an annotation's value. A timestamp string becomes the Unix
// nanoseconds a time scale maps, which is what the encoder wrote it from.
func datumOf(ch *Channel) float64 {
	if ch == nil || ch.Datum == nil {
		return 0
	}
	v := asNumber(ch.Datum)
	if math.IsNaN(v) {
		return 0
	}
	return v
}

func decodeColorScale(s Scale) (scale.ColorScale, error) {
	d := scale.ColorDesc{Kind: scale.KindSequential, Ramp: s.Scheme, Reverse: s.Reverse}
	switch s.Type {
	case string(scale.KindDiverging):
		d.Kind = scale.KindDiverging
	case string(scale.KindQualitative), "ordinal", "nominal":
		// "ordinal" is what Vega-Lite calls a scale from categories to a
		// discrete range, so a hand-written document that says it means this.
		d.Kind = scale.KindQualitative
	}
	if s.Center != nil {
		d.Center = *s.Center
	}
	for _, hex := range s.Range {
		c, err := parseColor(hex)
		if err != nil {
			return nil, err
		}
		d.Colors = append(d.Colors, c)
	}
	if s.Undefined != "" {
		c, err := parseColor(s.Undefined)
		if err != nil {
			return nil, err
		}
		d.Undefined = c
	} else {
		d.Undefined = ir.Transparent
	}
	if len(s.Domain) == 2 {
		lo, hi := asNumber(s.Domain[0]), asNumber(s.Domain[1])
		if math.IsNaN(lo) || math.IsNaN(hi) {
			return nil, fmt.Errorf("refract/spec: a colour domain needs two numbers")
		}
		d.Min, d.Max, d.Fixed = lo, hi, true
	}
	return scale.ColorFromDesc(d)
}
