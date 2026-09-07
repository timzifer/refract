package refract_test

// The milestones after v1.1, end to end. What each of these tests is about is
// that the feature reaches a whole chart — the axes a plot holds, the tracks
// at its edges and the panels of a facet — rather than the one scale or the
// one layer it was set on.

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/backend/pdf"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/sfnttest"
	"github.com/timzifer/refract/scale"
)

// svgOf renders a plot and hands back the document.
func svgOf(t *testing.T, p *refract.Plot) string {
	t.Helper()
	var buf bytes.Buffer
	if err := p.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

// A chart is in one language. Setting it per scale means saying it once per
// axis and once per track, and forgetting it somewhere is a chart with two
// languages in it — so it is a plot option, and it reaches every axis the plot
// holds.
func TestALocaleReachesEveryAxisOfAPlot(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {1000.5, 2000.5, 3000.5, 4000.5},
	})
	p := refract.New(refract.Size(500, 300), refract.Locale(scale.LocaleDE))
	p.X(scale.Linear(scale.Nice(), scale.NumberFormat("#,.1")))
	p.Y(scale.Linear(scale.Nice(), scale.NumberFormat("#,.1")))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))

	doc := svgOf(t, p)
	if !strings.Contains(doc, "1.000,0") && !strings.Contains(doc, "2.000,0") {
		t.Errorf("no axis label is punctuated in German:\n%s", firstLabels(doc))
	}
	if strings.Contains(doc, "1,000.0") {
		t.Error("a label is still punctuated in English")
	}
}

// A track carries its own scale, and it is one of the axes a plot holds.
func TestALocaleReachesATracksOwnAxis(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {1, 2, 3, 4},
	})
	load := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"w": {1000, 2000, 3000, 4000},
	})
	p := refract.New(refract.Size(500, 300), refract.Locale(scale.LocaleDE))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	tr := p.Track(refract.Bottom,
		refract.TrackScale(scale.Linear(scale.Nice(), scale.NumberFormat("#,"))),
		refract.TrackAxis(true))
	tr.Add(geom.Line(load, geom.X("x"), geom.Y("w")))

	if doc := svgOf(t, p); !strings.Contains(doc, "2.000") {
		t.Errorf("the track's own axis is not in German:\n%s", firstLabels(doc))
	}
}

// A free facet axis is a clone made while the chart is described, so the walk
// has to reach it too — this is the case a per-scale option would silently
// miss.
func TestALocaleReachesAFreeFacetAxis(t *testing.T) {
	src := refract.NewTable().
		Float64("x", []float64{0, 1, 0, 1}).
		Float64("y", []float64{1000, 2000, 3000, 4000}).
		String("g", []string{"a", "a", "b", "b"})

	p := refract.New(refract.Size(600, 400), refract.Locale(scale.LocaleDE))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice(), scale.NumberFormat("#,")))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	p.Facet(facet.Wrap("g", facet.FreeY()))

	if doc := svgOf(t, p); !strings.Contains(doc, "2.000") && !strings.Contains(doc, "1.000") {
		t.Errorf("a free facet axis is not in German:\n%s", firstLabels(doc))
	}
}

// A time axis takes its month names from the locale, which is the half Go's
// own time package has no hook for.
func TestALocaleNamesTheMonthsOfATimeAxis(t *testing.T) {
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	var ts []time.Time
	var vs []float64
	for i := range 40 {
		ts = append(ts, from.AddDate(0, 0, i*3))
		vs = append(vs, float64(i))
	}
	src := refract.NewTable().Time("t", ts).Float64("v", vs)

	p := refract.New(refract.Size(600, 300), refract.Locale(scale.LocaleDE))
	p.X(scale.Time(scale.In(time.UTC)))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(src, geom.X("t"), geom.Y("v")))

	doc := svgOf(t, p)
	if !strings.Contains(doc, "Mär") && !strings.Contains(doc, "Apr") && !strings.Contains(doc, "Mai") {
		t.Errorf("no month name is German:\n%s", firstLabels(doc))
	}
}

// A chart that never mentions a locale draws what it always drew. This is the
// property every golden file in the repository rests on.
func TestAChartWithNoLocaleIsUnchanged(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {1000, 2000, 3000, 4000},
	})
	build := func(opts ...refract.Option) string {
		p := refract.New(append([]refract.Option{refract.Size(500, 300)}, opts...)...)
		p.X(scale.Linear(scale.Nice()))
		p.Y(scale.Linear(scale.Nice()))
		p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
		return svgOf(t, p)
	}
	if build() != build(refract.Locale(scale.English)) {
		t.Error("naming English changed the chart; it is the default and must cost nothing")
	}
}

// firstLabels pulls the text out of a document, for a failure message that
// says what the labels actually were.
func firstLabels(doc string) string {
	var out []string
	for _, part := range strings.Split(doc, "<text")[1:] {
		if i := strings.IndexByte(part, '>'); i >= 0 {
			part = part[i+1:]
			if j := strings.Index(part, "</text>"); j >= 0 {
				out = append(out, part[:j])
			}
		}
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return strings.Join(out, " | ")
}

// The interval mark, end to end: a chart of means with the intervals they are
// known to within, rendered through the whole pipeline.
func TestAnErrorBarRendersOverTheBarsItAnnotates(t *testing.T) {
	src := refract.NewTable().
		String("group", []string{"a", "b", "c"}).
		Float64("mean", []float64{10, 12, 11}).
		Float64("sd", []float64{1, 2, 0.5})

	p := refract.New(refract.Size(500, 300), refract.Title("Means"))
	p.X(scale.Ordinal())
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(geom.Bar(src, geom.X("group"), geom.Y("mean")))
	p.Add(geom.ErrorBar(src, geom.X("group"), geom.Y("mean"), geom.ErrorBy("sd")))

	doc := svgOf(t, p)
	// The tallest interval reaches 14, so the axis has to have a tick past it:
	// a chart whose error bar runs off the top is the failure the bounds are
	// trained for.
	if !strings.Contains(doc, ">14<") && !strings.Contains(doc, ">15<") {
		t.Errorf("the axis stops short of the widest interval:\n%s", firstLabels(doc))
	}
}

// A chart labelled in a language WinAnsi cannot hold, rendered to PDF with the
// font that can. It is the end of the road the locale started: the labels are
// in the reader's language, and the document can carry them.
func TestAPDFCarriesTheFontItsLabelsNeed(t *testing.T) {
	src := refract.NewTable().
		String("k", []string{"A", "B", "Ä"}).
		Float64("v", []float64{1, 2, 3})

	p := refract.New(refract.Size(400, 300), refract.Locale(scale.LocaleDE))
	p.X(scale.Ordinal())
	p.Y(scale.Linear(scale.Nice(), scale.NumberFormat("#,.1")))
	p.Add(geom.Bar(src, geom.X("k"), geom.Y("v")))

	font := sfnttest.Font(sfnttest.Cmap4(), sfnttest.NumGlyphs)
	var buf bytes.Buffer
	if err := p.Render(refract.PDFWriter(&buf, pdf.Uncompressed(), pdf.WithFont(font, nil, nil))); err != nil {
		t.Fatalf("Render: %v", err)
	}
	doc := buf.String()
	if !strings.Contains(doc, "/Encoding /Identity-H") {
		t.Error("the document does not carry a CID font")
	}
	if strings.Contains(doc, "/BaseFont /Helvetica") {
		t.Error("a label was still drawn in the base-14 face")
	}
	if !strings.Contains(doc, "/ToUnicode") {
		t.Error("the document's text cannot be selected, copied or read aloud")
	}
	if strings.Contains(doc, "(?)") {
		t.Error("a label still came out as the WinAnsi substitute")
	}
}
