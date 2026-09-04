package a11y_test

import (
	"strings"
	"testing"
	"time"

	"github.com/timzifer/refract/a11y"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
)

func table() data.Source {
	return data.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {10, 30, 20, 40},
	})
}

func TestASummaryNamesWhatIsPlotted(t *testing.T) {
	c := a11y.Chart{
		Title:  "Throughput",
		XTitle: "time",
		YTitle: "requests",
		X:      scale.Linear(),
		Y:      scale.Linear(),
		Layers: []geom.Geom{geom.Line(table(), geom.X("x"), geom.Y("y"), geom.Label("north"))},
	}
	s := a11y.Describe(c)

	if s.Title != "Throughput" {
		t.Errorf("title %q, want the chart's own", s.Title)
	}
	for _, want := range []string{"1 layer", "line", "north", "4 rows", "x from 0 to 3", "y from 10 to 40"} {
		if !strings.Contains(s.Detail, want) {
			t.Errorf("the description does not mention %q:\n%s", want, s.Detail)
		}
	}
	if len(s.Series) != 1 {
		t.Fatalf("described %d series, want 1", len(s.Series))
	}
	if got := s.Series[0]; got.Rows != 4 || !got.XRange.Ok || got.YRange.Max != 40 {
		t.Errorf("series described as %+v", got)
	}
}

// A chart with no title still has to be announced as something, because "graphic"
// is what a screen reader says otherwise.
func TestAnUntitledChartIsNamedAfterWhatItDraws(t *testing.T) {
	s := a11y.Describe(a11y.Chart{
		X:      scale.Linear(),
		Y:      scale.Linear(),
		Layers: []geom.Geom{geom.Scatter(table(), geom.X("x"), geom.Y("y"))},
	})
	if s.Title != "Scatter chart of y against x" {
		t.Errorf("generated title %q", s.Title)
	}
}

func TestAnEmptyChartSaysSo(t *testing.T) {
	s := a11y.Describe(a11y.Chart{X: scale.Linear(), Y: scale.Linear()})
	if s.Title == "" || s.Detail == "" {
		t.Errorf("an empty chart described as %q / %q", s.Title, s.Detail)
	}
}

func TestATimeAxisIsReadAsTimestamps(t *testing.T) {
	start := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	src := data.NewTable().
		Time("t", []time.Time{start, start.Add(time.Hour)}).
		Float64("y", []float64{1, 2})

	s := a11y.Describe(a11y.Chart{
		X:      scale.Time(),
		Y:      scale.Linear(),
		Layers: []geom.Geom{geom.Line(src, geom.X("t"), geom.Y("y"))},
	})
	if !strings.Contains(s.Detail, "2026-03-01T12:00:00Z") {
		t.Errorf("a time axis was not read as a timestamp:\n%s", s.Detail)
	}
}

func TestAnnotationsAreDescribedByTheirValues(t *testing.T) {
	s := a11y.Describe(a11y.Chart{
		X:      scale.Linear(),
		Y:      scale.Linear(),
		Layers: []geom.Geom{geom.HLine(25, geom.Label("limit"))},
	})
	if !strings.Contains(s.Detail, "limit") {
		t.Errorf("the annotation is not in the description:\n%s", s.Detail)
	}
}

func TestAFacetedChartSaysWhatItIsSplitBy(t *testing.T) {
	s := a11y.Describe(a11y.Chart{
		X:      scale.Linear(),
		Y:      scale.Linear(),
		Facet:  "region",
		Layers: []geom.Geom{geom.Line(table(), geom.X("x"), geom.Y("y"))},
	})
	if !strings.Contains(s.Detail, "one panel per value of region") {
		t.Errorf("the facet is not in the description:\n%s", s.Detail)
	}
}

func TestTheTableCarriesEveryRow(t *testing.T) {
	html, err := a11y.Table(a11y.Chart{
		Layers: []geom.Geom{geom.Line(table(), geom.X("x"), geom.Y("y"), geom.Label("north"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(html, "<tr>"); n != 5 {
		t.Errorf("the table has %d rows, want a header and four of data:\n%s", n, html)
	}
	for _, want := range []string{"<caption>north</caption>", `<th scope="col">x</th>`, "<td>40</td>"} {
		if !strings.Contains(html, want) {
			t.Errorf("the table is missing %q:\n%s", want, html)
		}
	}
}

// The table's content is the caller's data, and a column named after a script
// tag is a column name rather than a script.
func TestTheTableEscapesWhatItIsGiven(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{"<script>": {1}, "y": {2}})
	html, err := a11y.Table(a11y.Chart{
		Layers: []geom.Geom{geom.Line(src, geom.X("<script>"), geom.Y("y"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>") {
		t.Errorf("a column name was written unescaped:\n%s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Errorf("the escaped column name is missing:\n%s", html)
	}
}

func TestTheTableReadsEveryColumnType(t *testing.T) {
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	src := data.NewTable().
		Time("t", []time.Time{start}).
		Float64("y", []float64{1.5}).
		String("region", []string{"north"})

	html, err := a11y.Table(a11y.Chart{
		Layers: []geom.Geom{geom.Scatter(src, geom.X("t"), geom.Y("y"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "2026-03-01T00:00:00Z") || !strings.Contains(html, "<td>1.5</td>") {
		t.Errorf("the table did not read its columns:\n%s", html)
	}
}

func TestAnAnnotationIsListedRatherThanTabulated(t *testing.T) {
	html, err := a11y.Table(a11y.Chart{
		Layers: []geom.Geom{geom.HLine(25, geom.Label("limit"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<table>") {
		t.Errorf("an annotation was given a table:\n%s", html)
	}
	if !strings.Contains(html, "y 25") {
		t.Errorf("the annotation's value is missing:\n%s", html)
	}
	// A horizontal rule has no x, and reporting one as "x 0" would be a number
	// the chart does not contain.
	if strings.Contains(html, "x 0") {
		t.Errorf("a horizontal rule was reported as having an x:\n%s", html)
	}
}
