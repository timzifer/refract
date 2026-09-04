package gg_test

import (
	"bytes"
	"math"
	"regexp"
	"strconv"
	"testing"

	ggbackend "github.com/timzifer/refract/backend/gg"
	"github.com/timzifer/refract/backend/svg"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/layout"
	"github.com/timzifer/refract/theme"
	"golang.org/x/image/font/gofont/goregular"
)

// The "one model, many backends" claim rests on one thing: layout asks each
// backend to measure the text it will actually draw, and the two answers agree
// closely enough that the same chart lays out the same way. These tests hold
// that to account, and pin how much divergence is actually there.
//
// They do not assert equality, because equality is not true even when both
// backends are handed the same font file:
//
//   - gg hints glyph advances to whole pixels; refract's own hmtx reader sums
//     unrounded advances. On chart-sized strings that is up to a few percent.
//   - The two read vertical metrics from different tables, so the font box
//     height differs by about 15% for Go Regular.
//
// The consequence is bounded and known: the plot rectangle the two lay out for
// the same chart agrees to within a few pixels, not exactly. That is the real
// version of CONCEPT.md §9's "one accepted imprecision", and these tolerances
// are where it is written down. Tightening them is a real improvement; loosening
// them silently is a regression.
const (
	// maxAdvanceDriftPerGlyph bounds the width disagreement, in device pixels
	// per glyph. Rounding an advance to a whole pixel can move it by at most
	// half a pixel, so this is that bound plus a little slack — and stating it
	// per glyph rather than as a percentage is what makes it meaningful for a
	// four-character tick label as well as for a long axis title.
	maxAdvanceDriftPerGlyph = 0.6
	// maxHeightDrift bounds the font box height disagreement.
	maxHeightDrift = 0.20
	// maxLayoutDrift bounds the resulting plot-rectangle disagreement, in
	// device pixels.
	maxLayoutDrift = 6.0
)

func measurers(t *testing.T) (svgB, ggB ir.Backend) {
	t.Helper()

	var buf bytes.Buffer
	st := svg.Writer(&buf, svg.WithFont(goregular.TTF))
	svgB, err := st.Open(600, 400, 1)
	if err != nil {
		t.Fatalf("opening the SVG backend: %v", err)
	}

	gt := ggbackend.Writer(&bytes.Buffer{}, ggbackend.FormatPNG)
	ggB, err = gt.Open(600, 400, 1)
	if err != nil {
		t.Fatalf("opening the gg backend: %v", err)
	}
	return svgB, ggB
}

func TestMeasurementAgreesAcrossBackendsWithinKnownDrift(t *testing.T) {
	svgB, ggB := measurers(t)

	texts := []string{"0", "10.0", "-2.5", "09:15", "amplitude", "Signal", "Mar 14 2026"}
	sizes := []float64{11, 12, 16, 24}

	for _, size := range sizes {
		for _, text := range texts {
			run := ir.TextRun{Text: text, Font: ir.FontRef{Size: size}}
			a := svgB.Measure(run)
			b := ggB.Measure(run)

			budget := maxAdvanceDriftPerGlyph * float64(len([]rune(text)))
			if d := math.Abs(float64(a.Advance - b.Advance)); d > budget {
				t.Errorf("advance for %q at %vpx: svg %v, gg %v (%.2fpx apart, budget %.2fpx)",
					text, size, a.Advance, b.Advance, d, budget)
			}
			if rel := relDiff(a.Height(), b.Height()); rel > maxHeightDrift {
				t.Errorf("font box height at %vpx: svg %v, gg %v (%.2f%% apart)",
					size, a.Height(), b.Height(), rel*100)
			}
		}
	}
}

func TestPlotAreaAgreesAcrossBackendsWithinKnownDrift(t *testing.T) {
	// Same chart description, same font, two backends. Everything the geoms
	// draw is positioned relative to the plot rectangle, so this is the number
	// that decides whether the two outputs look like the same chart.
	svgB, ggB := measurers(t)

	c := layout.Chart{
		Canvas:       ir.R(0, 0, 640, 400),
		Theme:        theme.Light,
		Title:        "Signal",
		XTitle:       "time",
		YTitle:       "amplitude",
		XLabels:      []string{"09:00", "09:15", "09:30", "09:45"},
		YLabels:      []string{"-1.0", "-0.5", "0.0", "0.5", "1.0"},
		LegendLabels: []string{"measured", "modelled"},
	}

	a := layout.Compute(c, svgB)
	b := layout.Compute(c, ggB)

	for _, cmp := range []struct {
		name   string
		x, y   float32
		x2, y2 float32
	}{
		{"plot min", a.Plot.Min.X, a.Plot.Min.Y, b.Plot.Min.X, b.Plot.Min.Y},
		{"plot max", a.Plot.Max.X, a.Plot.Max.Y, b.Plot.Max.X, b.Plot.Max.Y},
		{"legend min", a.Legend.Min.X, a.Legend.Min.Y, b.Legend.Min.X, b.Legend.Min.Y},
	} {
		if math.Abs(float64(cmp.x-cmp.x2)) > maxLayoutDrift || math.Abs(float64(cmp.y-cmp.y2)) > maxLayoutDrift {
			t.Errorf("%s differs: svg (%v,%v), gg (%v,%v)", cmp.name, cmp.x, cmp.y, cmp.x2, cmp.y2)
		}
	}
}

// TestRenderedClipRectMatchesTheLayout closes the loop: the plot rectangle the
// layout computes for the gg backend is the rectangle the SVG document
// actually clips to, when both are given the same font.
func TestRenderedClipRectMatchesTheLayout(t *testing.T) {
	p := plot(640, 400)

	var buf bytes.Buffer
	if err := p.Render(refractSVG(&buf)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := clipRect(t, buf.String())

	_, ggB := measurers(t)
	// The chart the helper builds has a title and no axis titles or legend.
	want := layout.Compute(layout.Chart{
		Canvas:  ir.R(0, 0, 640, 400),
		Theme:   theme.Light,
		Title:   "t",
		XLabels: []string{"0.0", "0.5", "1.0", "1.5", "2.0"},
		YLabels: []string{"0.0", "0.5", "1.0", "1.5", "2.0"},
	}, ggB).Plot

	if math.Abs(float64(got.Min.X-want.Min.X)) > maxLayoutDrift || math.Abs(float64(got.Max.Y-want.Max.Y)) > maxLayoutDrift {
		t.Errorf("the SVG clips to %v but the gg layout computes %v", got, want)
	}
}

var clipRe = regexp.MustCompile(`<clipPath id="c1"><path d="M([\d.-]+),([\d.-]+) L([\d.-]+),([\d.-]+) L([\d.-]+),([\d.-]+)`)

func clipRect(t *testing.T, doc string) ir.Rect {
	t.Helper()
	m := clipRe.FindStringSubmatch(doc)
	if m == nil {
		t.Fatal("no clip path found in the SVG document")
	}
	num := func(s string) float32 {
		v, err := strconv.ParseFloat(s, 32)
		if err != nil {
			t.Fatalf("bad number %q: %v", s, err)
		}
		return float32(v)
	}
	return ir.R(num(m[1]), num(m[2]), num(m[5]), num(m[6]))
}

func relDiff(a, b float32) float64 {
	if a == b {
		return 0
	}
	denom := math.Max(math.Abs(float64(a)), math.Abs(float64(b)))
	if denom == 0 {
		return 0
	}
	return math.Abs(float64(a-b)) / denom
}
