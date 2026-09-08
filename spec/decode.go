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
		if c.Y2, c.Y2Title, err = axisScale(s.Encoding.YSecondary); err != nil {
			return Chart{}, fmt.Errorf("refract/spec: secondary y axis: %w", err)
		}
		if c.X2, c.X2Title, err = axisScale(s.Encoding.XSecondary); err != nil {
			return Chart{}, fmt.Errorf("refract/spec: secondary x axis: %w", err)
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

	for i, d := range s.Tracks {
		tr, err := decodeTrack(d, shared)
		if err != nil {
			return Chart{}, fmt.Errorf("refract/spec: track %d: %w", i, err)
		}
		c.Tracks = append(c.Tracks, tr)
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
	d := scale.Desc{Nice: s.Nice, Zero: s.Zero, Base: s.Base, Threshold: s.Constant, Locale: s.Locale}
	d.MinorTicks = true
	if s.MinorTicks != nil {
		d.MinorTicks = *s.MinorTicks
	}

	// The one format field is read as whichever of the two the scale's type
	// makes it; see [Scale.Format].
	if typ == "time" || typ == "utc" {
		d.Layout = s.Format
	} else {
		d.Format = s.Format
	}

	switch typ {
	case "linear":
		d.Kind, d.TickValues = scale.KindLinear, s.TickValues
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
		// A type this package did not define. scale.FromDesc knows whether
		// anyone registered it, and says so if nobody did; what is read here
		// is the shared part of the description, which is all a document has.
		d.Kind = scale.Kind(typ)
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
		Caps:      true,
		Missing:   missingPolicy(l.Mark.Missing),
		Decimate:  decimationMode(l.Mark.Decimate),
		Budget:    l.Mark.Budget,
		CellSize:  l.Mark.DensityCells,
		Explode:   l.Mark.Explode,
		Text:      l.Mark.Text,
		Elide:     l.Mark.Elide,
		FontSize:  l.Mark.FontSize,
		Rotation:  radians(l.Mark.Angle),
		Extend:    true,
		Opacity:   -1,
		Tension:   l.Mark.Tension,
		Marker:    markerShape(l.Mark.Shape),
		MarkerSet: l.Mark.Shape != "",
		Closed:    l.Mark.Closed,
		OnY2:      l.Mark.YAxis == axisSecondaryY,
		OnX2:      l.Mark.XAxis == axisSecondaryX,
		Steps:     stepPos(l.Mark.Interpolate),
		HAlign:    hAlignOf(l.Mark.Align),
		VAlign:    vAlignOf(l.Mark.Baseline),
		AlignSet:  l.Mark.Align != "" || l.Mark.Baseline != "",
		Bins:      l.Mark.Bins,
		Bandwidth: l.Mark.Bandwidth,
		Span:      l.Mark.Span,
		Smooth:    smoothing(l.Mark.Method),
		Overlap:   l.Mark.Overlap,
		Padding:   l.Mark.Padding,
		Thickness: l.Mark.Thickness,
		Extra:     l.Mark.Extra,

		AvoidOverlap: l.Mark.AvoidOverlap,
	}
	if l.Mark.BinStart != nil && l.Mark.BinEnd != nil {
		d.BinLo, d.BinHi = *l.Mark.BinStart, *l.Mark.BinEnd
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
	if l.Mark.Caps != nil {
		d.Caps = *l.Mark.Caps
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

	// A layer with data is one that named a column. The relational marks name
	// none of the positional ones — their columns are an edge table — so the
	// test has to ask about theirs too, or a sankey decodes with no source and
	// geom.FromDesc refuses it. It is spelled out rather than deferred to
	// hasField, which would newly hand a source to a layer encoded only by
	// colour.
	if d.X != "" || d.Y != "" || d.From != "" || d.ID != "" {
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
	d.Key = fieldOf(enc.Key)
	d.ExplodeCol = fieldOf(enc.Explode)
	d.MidCol, d.ErrorCol, d.ErrorXCol = fieldOf(enc.Mid), fieldOf(enc.Error), fieldOf(enc.ErrorX)
	d.From, d.To = fieldOf(enc.From), fieldOf(enc.To)
	d.ID, d.ParentCol, d.ValueCol = fieldOf(enc.ID), fieldOf(enc.Parent), fieldOf(enc.Value)
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

	if enc.Text != nil && enc.Text.Field != "" {
		d.TextCol = enc.Text.Field
	}
	if enc.Size != nil && enc.Size.Field != "" {
		ss, err := decodeSizeScale(enc.Size.Scale)
		if err != nil {
			return err
		}
		d.SizeCol, d.SizeScale = enc.Size.Field, ss
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

// decodeSizeScale reads a size channel's scale. A channel with no scale still
// gets one: a size encoding without a scale is a size encoding, and the
// defaults are the ones that make the area proportional to the value.
func decodeSizeScale(s *Scale) (scale.SizeScale, error) {
	var d scale.SizeDesc
	if s != nil {
		if len(s.Domain) == 2 {
			lo, hi := asNumber(s.Domain[0]), asNumber(s.Domain[1])
			if math.IsNaN(lo) || math.IsNaN(hi) {
				return nil, fmt.Errorf("refract/spec: a size domain needs two numbers")
			}
			d.Min, d.Max, d.Fixed = lo, hi, true
		} else if len(s.Domain) != 0 {
			return nil, fmt.Errorf("refract/spec: a size domain needs two bounds, got %d", len(s.Domain))
		}
		if len(s.SizeRange) == 2 {
			d.MinSize, d.MaxSize, d.RangeSet = s.SizeRange[0], s.SizeRange[1], true
		} else if len(s.SizeRange) != 0 {
			return nil, fmt.Errorf("refract/spec: a size range needs two diameters, got %d", len(s.SizeRange))
		}
		if s.SizeZero != nil {
			d.Zero, d.ZeroSet = *s.SizeZero, true
		}
	}
	return scale.SizeFromDesc(d)
}

func decodeColorScale(s Scale) (scale.ColorScale, error) {
	d := scale.ColorDesc{Kind: scale.KindSequential, Ramp: s.Scheme, Reverse: s.Reverse}
	switch s.Type {
	case string(scale.KindDiverging):
		d.Kind = scale.KindDiverging
	case string(scale.KindThreshold):
		d.Kind = scale.KindThreshold
	case string(scale.KindQuantize):
		d.Kind = scale.KindQuantize
	case string(scale.KindQuantile):
		d.Kind = scale.KindQuantile
	case string(scale.KindNamed):
		d.Kind = scale.KindNamed
	case string(scale.KindQualitative), "ordinal", "nominal":
		// "ordinal" is what Vega-Lite calls a scale from categories to a
		// discrete range, so a hand-written document that says it means this.
		d.Kind = scale.KindQualitative
	case "", string(scale.KindSequential):
	case string(scale.TransformLog), string(scale.TransformSymLog):
		// Vega-Lite spells a colour scale's transform as its type, so a
		// hand-written document that says "log" means a sequential ramp run
		// logarithmically. An explicit transform below still wins.
		d.Transform = scale.ColorTransform(s.Type)
	default:
		// A kind this package did not define; scale.ColorFromDesc decides
		// whether anyone registered it.
		d.Kind = scale.ColorKind(s.Type)
	}
	if s.Transform != "" {
		d.Transform = scale.ColorTransform(s.Transform)
	}
	d.Base, d.Constant = s.Base, s.Constant
	d.Breaks, d.Classes = s.Breaks, s.Classes
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
	if d.Kind == scale.KindNamed {
		// A named scale's domain is its categories, not the two ends of an
		// interval, so it is read as labels and the numeric branch below is
		// not reached — a two-category scale would otherwise be decoded as a
		// pinned domain of two NaNs.
		for _, v := range s.Domain {
			label, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("refract/spec: a named colour scale's domain holds category names")
			}
			d.Labels = append(d.Labels, label)
		}
		for _, hex := range s.Fallback {
			c, err := parseColor(hex)
			if err != nil {
				return nil, err
			}
			d.Fallback = append(d.Fallback, c)
		}
		return scale.ColorFromDesc(d)
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

// decodeTrack reads one band back.
//
// A track's layers may name the chart's shared data source, so the hoisted
// source is handed down exactly as it is for the chart's own layers.
func decodeTrack(d TrackDoc, shared data.Source) (Track, error) {
	t := Track{
		Edge:     d.Edge,
		Size:     d.Size,
		Fraction: d.Fraction,
		Axis:     !d.NoAxis,
		Grid:     d.Grid,
	}
	var err error
	if t.Scale, _, err = axisScale(d.Scale); err != nil {
		return Track{}, fmt.Errorf("scale: %w", err)
	}
	for i, l := range d.Layer {
		g, err := decodeLayer(l, shared)
		if err != nil {
			return Track{}, fmt.Errorf("layer %d: %w", i, err)
		}
		t.Layers = append(t.Layers, g)
	}
	return t, nil
}
