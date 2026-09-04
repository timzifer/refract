package refract

import (
	"encoding/json"

	"github.com/timzifer/refract/spec"
)

// Spec writes the plot down as a document that can be marshalled to JSON and
// read back with [FromSpec].
//
// It fails on a layer or a scale that cannot describe itself rather than
// writing a document that draws a different chart — see
// [github.com/timzifer/refract/spec] for what survives the trip and what
// cannot.
func (p *Plot) Spec() (spec.Spec, error) {
	c := spec.Chart{
		Width:  p.width,
		Height: p.height,
		DPR:    p.dpr,
		Theme:  p.theme,
		Title:  p.title,
		XTitle: p.xTitle,
		YTitle: p.yTitle,
		X:      p.scaleX(),
		Y:      p.scaleY(),
		Layers: p.layers,
		Facet:  p.facet,
	}
	if p.legendSet {
		legend := p.legend
		c.Legend = &legend
	}
	return spec.Of(c)
}

// FromSpec builds a plot from a document.
func FromSpec(s spec.Spec) (*Plot, error) {
	c, err := s.Chart()
	if err != nil {
		return nil, err
	}
	p := New(Size(c.Width, c.Height), DPR(c.DPR), Theme(c.Theme),
		Title(c.Title), XTitle(c.XTitle), YTitle(c.YTitle))
	if c.X != nil {
		p.X(c.X)
	}
	if c.Y != nil {
		p.Y(c.Y)
	}
	p.Add(c.Layers...)
	p.Facet(c.Facet)
	if c.Legend != nil {
		p.legend, p.legendSet = *c.Legend, true
	}
	return p, nil
}

// MarshalJSON writes the plot as a spec document, so that a Plot can be handed
// straight to [encoding/json].
//
// Called directly it returns the indented form [spec.Spec.Marshal] produces: a
// chart is a thing people read and edit, and the compact form of one over a
// hundred rows is a single very long line. Called through [json.Marshal] it
// comes back compact, because that is what json.Marshal does to anything a
// Marshaler returns.
func (p *Plot) MarshalJSON() ([]byte, error) {
	s, err := p.Spec()
	if err != nil {
		return nil, err
	}
	return s.Marshal()
}

// UnmarshalJSON replaces the plot's contents with the document in b.
func (p *Plot) UnmarshalJSON(b []byte) error {
	s, err := spec.Parse(b)
	if err != nil {
		return err
	}
	q, err := FromSpec(s)
	if err != nil {
		return err
	}
	*p = *q
	return nil
}

// ParseJSON builds a plot from a spec document.
//
//	p, err := refract.ParseJSON(b)
//	if err == nil {
//	    err = p.Render(refract.SVG("chart.svg"))
//	}
//
// It is the whole web workflow in two calls: a browser or a config file hands
// over a chart, and the same model that a Go program builds by hand draws it.
func ParseJSON(b []byte) (*Plot, error) {
	s, err := spec.Parse(b)
	if err != nil {
		return nil, err
	}
	return FromSpec(s)
}

var (
	_ json.Marshaler   = (*Plot)(nil)
	_ json.Unmarshaler = (*Plot)(nil)
)
