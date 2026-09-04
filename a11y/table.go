package a11y

import (
	"fmt"
	"html"
	"io"
	"math"
	"strings"
	"time"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
)

// WriteTable writes the chart's data as an HTML table.
//
// This is the fallback a picture cannot be: the numbers themselves, in reading
// order, in a form a screen reader navigates cell by cell and a spreadsheet
// opens. It is what the accessibility guidance means by a data table
// alternative, and it is also the honest answer to "what is actually in this
// chart".
//
// One table per layer that has data, each with a caption naming the layer and a
// column per field it reads — x, y, a second y where a band has one, and the
// column a colour is taken from. A layer with no data source is an annotation:
// it is listed with its values rather than given a table of one row.
//
// The output is a fragment, not a document: one or more <table> elements with
// no wrapper, so it drops into a page beside the chart. Everything written is
// escaped, including column names and category labels, which come from the
// caller's data rather than from refract.
func WriteTable(w io.Writer, c Chart) error {
	tw := &tableWriter{w: w}
	if len(c.Layers) == 0 {
		tw.printf("<p>%s</p>\n", esc("This chart has no layers."))
		return tw.err
	}
	for i, g := range c.Layers {
		d, ok := geom.Describe(g)
		if !ok {
			tw.printf("<p>%s</p>\n", esc(fmt.Sprintf("Layer %d cannot describe itself.", i+1)))
			continue
		}
		if d.Source == nil {
			tw.annotation(d)
			continue
		}
		tw.table(labelOf(d, i), d)
		if tw.err != nil {
			return tw.err
		}
	}
	return tw.err
}

// Table returns what [WriteTable] writes.
func Table(c Chart) (string, error) {
	var b strings.Builder
	if err := WriteTable(&b, c); err != nil {
		return "", err
	}
	return b.String(), nil
}

func labelOf(d geom.Desc, i int) string {
	switch {
	case d.Label != "":
		return d.Label
	case d.Y != "":
		return d.Y
	case d.Mark != "":
		return string(d.Mark)
	}
	return fmt.Sprintf("layer %d", i+1)
}

type tableWriter struct {
	w   io.Writer
	err error
}

func (t *tableWriter) printf(format string, args ...any) {
	if t.err != nil {
		return
	}
	_, t.err = fmt.Fprintf(t.w, format, args...)
}

func (t *tableWriter) annotation(d geom.Desc) {
	// Which of the four values an annotation carries are meaningful depends on
	// the mark and cannot be read off the values: zero is a perfectly good
	// place to put a threshold, so an unused field looks exactly like a used
	// one that happens to be there.
	var parts []string
	for _, f := range []struct {
		name string
		v    float64
	}{{"x", d.Datum.X0}, {"x1", d.Datum.X1}, {"y", d.Datum.Y0}, {"y1", d.Datum.Y1}} {
		if usesDatum(d.Mark, f.name) {
			parts = append(parts, fmt.Sprintf("%s %s", f.name, data.FormatNumber(f.v)))
		}
	}
	text := d.Text
	if text == "" {
		text = string(d.Mark)
	}
	t.printf("<p>%s</p>\n", esc(fmt.Sprintf("Annotation: %s at %s.", text, strings.Join(parts, ", "))))
}

// usesDatum reports which of an annotation's four values the mark actually
// reads, so that a horizontal rule is not reported as sitting at "x 0".
func usesDatum(m geom.Mark, field string) bool {
	switch m {
	case geom.MarkHLine:
		return field == "y"
	case geom.MarkVLine:
		return field == "x"
	case geom.MarkHBand:
		return field == "y" || field == "y1"
	case geom.MarkVBand:
		return field == "x" || field == "x1"
	case geom.MarkSegment, geom.MarkRegion:
		return true
	case geom.MarkNote:
		return field == "x" || field == "y"
	}
	return false
}

func (t *tableWriter) table(label string, d geom.Desc) {
	cols := fields(d)
	if len(cols) == 0 {
		return
	}
	src := d.Source
	n := src.Len()

	t.printf("<table>\n<caption>%s</caption>\n<thead>\n<tr>", esc(label))
	for _, col := range cols {
		t.printf("<th scope=\"col\">%s</th>", esc(col))
	}
	t.printf("</tr>\n</thead>\n<tbody>\n")

	readers := make([]func(int) string, len(cols))
	for i, col := range cols {
		readers[i] = reader(src, col)
	}
	for row := range n {
		t.printf("<tr>")
		for _, read := range readers {
			t.printf("<td>%s</td>", esc(read(row)))
		}
		t.printf("</tr>\n")
		if t.err != nil {
			return
		}
	}
	t.printf("</tbody>\n</table>\n")
}

// fields lists the columns a layer reads, in the order a reader wants them and
// without repeating one that fills two roles.
func fields(d geom.Desc) []string {
	var out []string
	seen := map[string]bool{}
	for _, name := range []string{d.X, d.Y, d.Y2, d.ColorCol} {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// reader returns a per-row formatter for one column, resolved once rather than
// per cell: a table of a hundred thousand rows should not look its column type
// up a hundred thousand times.
func reader(src data.Source, name string) func(int) string {
	if v, ok := src.Float64Column(name); ok {
		return func(i int) string {
			if i >= len(v) {
				return ""
			}
			if math.IsNaN(v[i]) {
				return ""
			}
			return data.FormatNumber(v[i])
		}
	}
	if v, ok := src.TimeColumn(name); ok {
		return func(i int) string {
			if i >= len(v) {
				return ""
			}
			return v[i].UTC().Format(time.RFC3339)
		}
	}
	if v, ok := src.StringColumn(name); ok {
		return func(i int) string {
			if i >= len(v) {
				return ""
			}
			return v[i]
		}
	}
	return func(int) string { return "" }
}

func esc(s string) string { return html.EscapeString(s) }
