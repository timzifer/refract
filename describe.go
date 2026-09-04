package refract

import (
	"io"

	"github.com/timzifer/refract/a11y"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/mathtext"
)

// Description attaches an accessible description to the chart.
//
// title is the short label — an SVG's <title>, a PDF's document title, a
// canvas element's aria-label. detail is the long reading, which an SVG puts in
// a <desc> and a screen reader reads after the title. Either may be empty.
//
// A chart with a [Title] already has a short label and needs no option to get
// one: the title is written into the output as a matter of course, because a
// picture with no accessible name is the one thing every accessibility
// guideline agrees about. This option is for saying something *other* than the
// title, and for the paragraph a title cannot hold.
//
// [Plot.Describe] writes both from the data instead, which is what to reach for
// when the chart is built by a program rather than by a person.
func Description(title, detail string) Option {
	return func(p *Plot) {
		p.desc = ir.Description{Title: title, Detail: detail}
		p.descSet = true
	}
}

// Describe reads the chart's own data and attaches a description of it, then
// returns what it wrote. See [a11y.Describe] for the shape of the summary and
// [a11y.Chart] for what it is derived from.
//
//	p.Describe()
//	p.Render(refract.SVG("chart.svg")) // carries <title> and <desc>
//
// It is a method that does work rather than an option that sets a flag,
// because the work is a pass over every plotted column: a chart that nobody
// asked to describe should not pay for one on every render, and a chart that
// did should pay for it once rather than per frame. Call it again after the
// data changes.
//
// A description written by [Description] is replaced by this, and calling
// Describe on a plot that has been given one is how a caller says the data has
// moved on.
func (p *Plot) Describe() a11y.Summary {
	s := a11y.Describe(p.a11yChart())
	p.desc = ir.Description{Title: s.Title, Detail: s.Detail}
	p.descSet = true
	return s
}

// Description reports the description the chart currently carries: the one
// [Description] or [Plot.Describe] set, or the chart's title alone.
func (p *Plot) Description() ir.Description {
	if p.descSet {
		return p.desc
	}
	return ir.Description{Title: p.title}
}

// DataTable writes the chart's data to w as an HTML table — the fallback for a
// reader who cannot see the picture, and the honest answer to what is in it.
// See [a11y.WriteTable].
func (p *Plot) DataTable(w io.Writer) error { return a11y.WriteTable(w, p.a11yChart()) }

// a11yChart is the plot as the a11y package wants to read it.
//
// The titles are put through the typesetter's plain form first, when there is
// one: a description is read aloud, and notation read aloud is markup read
// aloud. A chart with no typesetter has no notation in its titles, so nothing
// happens to them.
func (p *Plot) a11yChart() a11y.Chart {
	c := a11y.Chart{
		Title:  p.plain(p.title),
		XTitle: p.plain(p.xTitle),
		YTitle: p.plain(p.yTitle),
		X:      p.scaleX(),
		Y:      p.scaleY(),
		Layers: p.layers,
	}
	if p.facet != nil {
		d := p.facet.Describe()
		c.Facet = d.Col
		if d.Row != "" && d.Col != "" {
			c.Facet = d.Row + " and " + d.Col
		} else if d.Col == "" {
			c.Facet = d.Row
		}
	}
	return c
}

// plain writes a label as readable text. See [mathtext.PlainOf].
func (p *Plot) plain(label string) string {
	if p.math == nil {
		return label
	}
	return mathtext.PlainOf(p.math, label)
}
