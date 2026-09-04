// Package a11y makes a chart readable by something other than an eye.
//
// A chart is a picture, and a picture is where the information stops for a
// reader using a screen reader, a reader on a monochrome printout, and any
// program that would rather have the numbers. This package produces the two
// things that fix that: a description in words, and the data as a table.
//
// # Where it sits
//
// It reads the model and is read by nobody in it — the same arrangement
// package spec has, and for the same reason. A geom that knew what a screen
// reader was would be a geom in the wrong package, so a layer says what it is
// through [geom.Desc] and this package decides what that means in words.
//
// # What refract does with it
//
// [github.com/timzifer/refract.Plot.Describe] attaches a summary to a plot, and
// every backend that can carry words then writes it: an SVG gets a <title> and
// a <desc> and the role that makes a screen reader read them, a PDF gets a
// document title, a browser canvas gets an aria-label.
// [github.com/timzifer/refract.Plot.DataTable] writes the table.
//
// The third channel — not colour alone — is a theme decision rather than a
// document one, and lives in [github.com/timzifer/refract/theme.Redundant].
package a11y

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
)

// Chart is what a plot tells this package about itself.
//
// It is deliberately the model rather than the picture: the layers, the scales
// and the titles, with no rectangle anywhere. A description of where the marks
// landed on a canvas would be a description of a canvas.
type Chart struct {
	Title          string
	XTitle, YTitle string
	X, Y           scale.Scale
	Layers         []geom.Geom

	// Facet names the column a faceted chart is split by, if any. The panels
	// themselves are not described one by one: "one panel per region" is the
	// fact a reader needs, and thirty near-identical paragraphs is not.
	Facet string
}

// Summary is a chart in words.
type Summary struct {
	// Title is a short label: the chart's own title, or a sentence naming what
	// it plots when it has none.
	Title string
	// Detail is the long reading: what each layer plots, over what range, and
	// how many rows there are.
	Detail string
	// Series is the same thing before it was turned into prose, for a caller
	// writing its own.
	Series []Series
}

// Series is one layer, reduced to what can be said about it.
type Series struct {
	// Label names the layer, and Mark is what it draws.
	Label string
	Mark  geom.Mark
	// X and Y name the columns, and Rows is how many there are.
	X, Y string
	Rows int
	// XRange and YRange are the extremes of the plotted columns. Ok is false
	// for a layer with no numbers in it — an annotation, or a column of
	// nothing but missing values.
	XRange, YRange Range
	// Time reports whether the corresponding axis is temporal, which decides
	// whether a bound reads as a number or as an instant.
	XTime, YTime bool
}

// Range is the extent of a column. Ok is false when there was nothing finite
// in it to measure.
type Range struct {
	Min, Max float64
	Ok       bool
}

// Describe reads a chart and says what it shows.
//
// It costs one pass over each plotted column, which is why refract does not do
// it on every render: describing a million rows is cheap compared with drawing
// them and is not free, and a chart nobody is describing should not pay for it.
//
// A layer that cannot describe itself — a third-party geom implementing no
// [geom.Describer] — is named by its position rather than skipped, because a
// description that quietly omits a series is worse than one that admits to it.
func Describe(c Chart) Summary {
	s := Summary{Title: c.Title}
	for i, g := range c.Layers {
		s.Series = append(s.Series, describeLayer(i, g, c))
	}
	if s.Title == "" {
		s.Title = generatedTitle(s.Series)
	}
	s.Detail = detail(c, s.Series)
	return s
}

func describeLayer(i int, g geom.Geom, c Chart) Series {
	d, ok := geom.Describe(g)
	if !ok {
		return Series{Label: fmt.Sprintf("layer %d", i+1)}
	}
	out := Series{Label: d.Label, Mark: d.Mark, X: d.X, Y: d.Y}
	if out.Label == "" {
		out.Label = d.Y
	}
	if out.Label == "" {
		out.Label = string(d.Mark)
	}
	out.XTime, out.YTime = isTime(c.X), isTime(c.Y)
	if d.Source == nil {
		// An annotation carries values rather than columns, and its extent is
		// the values it was given.
		out.Rows = 1
		out.XRange = datumRange(d.Datum.X0, d.Datum.X1)
		out.YRange = datumRange(d.Datum.Y0, d.Datum.Y1)
		return out
	}
	out.Rows = d.Source.Len()
	out.XRange = columnRange(d.Source, d.X)
	out.YRange = columnRange(d.Source, d.Y)
	if r := columnRange(d.Source, d.Y2); r.Ok {
		out.YRange = merge(out.YRange, r)
	}
	return out
}

// columnRange measures a column, whatever type it is stored as. A time column
// is measured in Unix nanoseconds, which is what a bound is formatted back
// from — the scale's own origin does not enter into it, because this reads the
// table rather than the axis.
func columnRange(src data.Source, name string) Range {
	if src == nil || name == "" {
		return Range{}
	}
	if v, ok := src.Float64Column(name); ok {
		return rangeOf(v)
	}
	if v, ok := src.TimeColumn(name); ok {
		out := Range{}
		for _, t := range v {
			out = extend(out, scale.Nanos(t))
		}
		return out
	}
	return Range{}
}

func rangeOf(vs []float64) Range {
	out := Range{}
	for _, v := range vs {
		out = extend(out, v)
	}
	return out
}

func extend(r Range, v float64) Range {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return r
	}
	if !r.Ok {
		return Range{Min: v, Max: v, Ok: true}
	}
	r.Min, r.Max = math.Min(r.Min, v), math.Max(r.Max, v)
	return r
}

func merge(a, b Range) Range {
	if !a.Ok {
		return b
	}
	if !b.Ok {
		return a
	}
	return Range{Min: math.Min(a.Min, b.Min), Max: math.Max(a.Max, b.Max), Ok: true}
}

func datumRange(a, b float64) Range {
	return merge(extend(Range{}, a), extend(Range{}, b))
}

func isTime(s scale.Scale) bool {
	d, ok := scale.Describe(s)
	return ok && d.Kind == scale.KindTime
}

// generatedTitle names a chart that was not given a title.
func generatedTitle(series []Series) string {
	if len(series) == 0 {
		return "Empty chart"
	}
	kinds := map[geom.Mark]bool{}
	for _, s := range series {
		kinds[s.Mark] = true
	}
	first := series[0]
	what := "Chart"
	if len(kinds) == 1 && first.Mark != "" {
		what = strings.ToUpper(string(first.Mark)[:1]) + string(first.Mark)[1:] + " chart"
	}
	switch {
	case first.Y != "" && first.X != "":
		return fmt.Sprintf("%s of %s against %s", what, first.Y, first.X)
	case first.Y != "":
		return fmt.Sprintf("%s of %s", what, first.Y)
	}
	return what
}

// detail writes the long description.
//
// The order is the order a reader needs it in: what the chart is, what its axes
// are, then one sentence per layer. A screen reader reads it start to finish
// and cannot skim, so the sentence that says whether this chart is worth
// listening to comes first.
func detail(c Chart, series []Series) string {
	var b strings.Builder
	if len(series) == 0 {
		return "A chart with no layers."
	}

	fmt.Fprintf(&b, "%s with %s.", plural(len(series), "layer", "layers"), listMarks(series))
	if c.XTitle != "" || c.YTitle != "" {
		b.WriteString(" Axes: ")
		b.WriteString(axisPhrase(c.XTitle, c.YTitle))
		b.WriteString(".")
	}
	if c.Facet != "" {
		fmt.Fprintf(&b, " Split into one panel per value of %s.", c.Facet)
	}
	for _, s := range series {
		b.WriteString(" ")
		b.WriteString(sentence(s))
	}
	return b.String()
}

func sentence(s Series) string {
	var b strings.Builder
	b.WriteString(s.Label)
	if s.Mark != "" {
		fmt.Fprintf(&b, ", a %s", s.Mark)
	}
	if s.Rows > 0 {
		fmt.Fprintf(&b, " of %s", plural(s.Rows, "row", "rows"))
	}
	if s.XRange.Ok {
		fmt.Fprintf(&b, ", %s from %s to %s", nameOr(s.X, "x"),
			format(s.XRange.Min, s.XTime), format(s.XRange.Max, s.XTime))
	}
	if s.YRange.Ok {
		fmt.Fprintf(&b, ", %s from %s to %s", nameOr(s.Y, "y"),
			format(s.YRange.Min, s.YTime), format(s.YRange.Max, s.YTime))
	}
	b.WriteString(".")
	return b.String()
}

func nameOr(name, fallback string) string {
	if name == "" {
		return fallback
	}
	return name
}

func axisPhrase(x, y string) string {
	switch {
	case x != "" && y != "":
		return x + " horizontally, " + y + " vertically"
	case x != "":
		return x + " horizontally"
	default:
		return y + " vertically"
	}
}

func listMarks(series []Series) string {
	names := make([]string, 0, len(series))
	seen := map[geom.Mark]bool{}
	for _, s := range series {
		if s.Mark == "" || seen[s.Mark] {
			continue
		}
		seen[s.Mark] = true
		names = append(names, string(s.Mark))
	}
	switch len(names) {
	case 0:
		return "no described marks"
	case 1:
		return names[0] + " marks"
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1] + " marks"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// format writes a bound the way a reader would say it: an instant as a
// timestamp, a number with enough digits to be worth reading and no more.
func format(v float64, isTime bool) string {
	if isTime {
		return scale.FromNanos(v).UTC().Format(time.RFC3339)
	}
	return data.FormatNumber(v)
}
