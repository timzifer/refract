package svg_test

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/timzifer/refract/backend/svg"
	"github.com/timzifer/refract/ir"
)

// open returns a backend drawing into buf, plus a finish func that flushes and
// closes it and returns the document.
func open(t *testing.T, opts ...svg.Option) (ir.Backend, func() string) {
	t.Helper()
	var buf bytes.Buffer
	target := svg.Writer(&buf, opts...)
	b, err := target.Open(200, 100, 1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return b, func() string {
		if err := b.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		if err := target.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		return buf.String()
	}
}

func TestDocumentSkeleton(t *testing.T) {
	_, finish := open(t)
	got := finish()

	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`xmlns="http://www.w3.org/2000/svg"`,
		`width="200"`,
		`height="100"`,
		`viewBox="0 0 200 100"`,
		`</svg>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("document is missing %s:\n%s", want, got)
		}
	}
}

func TestPolyline(t *testing.T) {
	b, finish := open(t)
	b.Polyline([]ir.Point{{X: 0, Y: 0}, {X: 10.5, Y: 20}}, ir.Stroke{
		Color: ir.RGB(255, 0, 0), Width: 2, Cap: ir.CapRound, Dash: []float32{4, 2},
	})
	got := finish()

	for _, want := range []string{
		`<polyline fill="none" points="0,0 10.5,20"`,
		`stroke="#ff0000"`,
		`stroke-width="2"`,
		`stroke-linecap="round"`,
		`stroke-dasharray="4 2"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
}

func TestInvisibleDrawingIsSkipped(t *testing.T) {
	b, finish := open(t)
	b.Polyline([]ir.Point{{}, {X: 1}}, ir.Stroke{Color: ir.Transparent, Width: 2})
	b.Polyline([]ir.Point{{}, {X: 1}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 0})
	b.Polyline([]ir.Point{{}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})

	var p ir.Path
	p.Rect(ir.R(0, 0, 1, 1))
	b.FillPath(&p, ir.Solid(ir.Transparent), ir.NonZero)
	b.Text(ir.TextRun{Text: "", Color: ir.RGB(0, 0, 0)})

	if got := finish(); strings.Contains(got, "<polyline") || strings.Contains(got, "<path") || strings.Contains(got, "<text") {
		t.Fatalf("invisible primitives emitted markup:\n%s", got)
	}
}

func TestPathAndFillRule(t *testing.T) {
	b, finish := open(t)
	var p ir.Path
	p.MoveTo(0, 0).LineTo(10, 0).CubicTo(10, 5, 5, 10, 0, 10).Close()
	b.FillPath(&p, ir.Solid(ir.RGBA(0, 0, 255, 128)), ir.EvenOdd)
	got := finish()

	if !strings.Contains(got, `d="M0,0 L10,0 C10,5 5,10 0,10 Z"`) {
		t.Errorf("path data is wrong:\n%s", got)
	}
	if !strings.Contains(got, `fill-rule="evenodd"`) {
		t.Errorf("fill rule missing:\n%s", got)
	}
	if !strings.Contains(got, `fill-opacity="0.5"`) {
		t.Errorf("alpha must become fill-opacity:\n%s", got)
	}
}

func TestTextEscapesAndAnchors(t *testing.T) {
	b, finish := open(t)
	b.Text(ir.TextRun{
		Text:  `a & b < c "d"`,
		Font:  ir.FontRef{Size: 11},
		At:    ir.Point{X: 5, Y: 6},
		H:     ir.AlignCenter,
		V:     ir.AlignMiddle,
		Color: ir.RGB(0, 0, 0),
	})
	got := finish()

	if !strings.Contains(got, `a &amp; b &lt; c &quot;d&quot;`) {
		t.Errorf("text was not escaped:\n%s", got)
	}
	if !strings.Contains(got, `text-anchor="middle"`) || !strings.Contains(got, `dominant-baseline="central"`) {
		t.Errorf("alignment attributes missing:\n%s", got)
	}
	if !strings.Contains(got, `font-family="sans-serif"`) {
		t.Errorf("default family missing:\n%s", got)
	}
}

func TestTextRotation(t *testing.T) {
	b, finish := open(t)
	b.Text(ir.TextRun{
		Text: "y", Font: ir.FontRef{Size: 10},
		At: ir.Point{X: 20, Y: 30}, Rotation: -math.Pi / 2, Color: ir.RGB(0, 0, 0),
	})
	if got := finish(); !strings.Contains(got, `transform="rotate(-90 20 30)"`) {
		t.Errorf("rotation must be emitted in degrees about the anchor:\n%s", got)
	}
}

func TestMarkersShareOneSymbol(t *testing.T) {
	b, finish := open(t)
	pts := []ir.Point{{X: 1, Y: 1}, {X: 2, Y: 2}, {X: 3, Y: 3}}
	b.Markers(ir.MarkerCircle, pts, ir.MarkerStyle{Size: 6, Fill: ir.RGB(0, 128, 0)})
	got := finish()

	if n := strings.Count(got, "<use "); n != 3 {
		t.Errorf("got %d <use> elements, want 3", n)
	}
	if n := strings.Count(got, `<path id="m1"`); n != 1 {
		t.Errorf("the marker shape must be defined exactly once, got %d", n)
	}
}

func TestClipAndTransformGroup(t *testing.T) {
	b, finish := open(t)
	var clip ir.Path
	clip.Rect(ir.R(0, 0, 50, 50))
	b.Push(&clip, ir.Translate(5, 5))
	b.Polyline([]ir.Point{{}, {X: 1, Y: 1}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})
	b.Pop()
	got := finish()

	if !strings.Contains(got, `<clipPath id="c1">`) {
		t.Errorf("clip path not defined:\n%s", got)
	}
	if !strings.Contains(got, `clip-path="url(#c1)"`) {
		t.Errorf("clip not referenced:\n%s", got)
	}
	if !strings.Contains(got, `transform="matrix(1 0 0 1 5 5)"`) {
		t.Errorf("transform not emitted in SVG matrix order:\n%s", got)
	}
	if !strings.Contains(got, "</g>") {
		t.Errorf("group not closed:\n%s", got)
	}
}

func TestUnbalancedPushIsAnError(t *testing.T) {
	var buf bytes.Buffer
	target := svg.Writer(&buf)
	b, err := target.Open(10, 10, 1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	b.Push(nil, ir.Identity)
	if err := b.Flush(); err == nil {
		t.Fatal("Flush must refuse to close a document with an open group")
	}
}

func TestPopWithoutPushIsAnError(t *testing.T) {
	b, _ := open(t)
	b.Pop()
	if err := b.Flush(); err == nil {
		t.Fatal("an unmatched Pop must surface as an error, not be swallowed")
	}
}

func TestImageIsEmbedded(t *testing.T) {
	b, finish := open(t)
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	b.Image(img, ir.R(0, 0, 20, 20))
	got := finish()

	if !strings.Contains(got, `href="data:image/png;base64,`) {
		t.Errorf("image not embedded as a data URI:\n%s", got)
	}
	if !strings.Contains(got, `preserveAspectRatio="none"`) {
		t.Errorf("the blit must fill its destination rectangle exactly:\n%s", got)
	}
}

func TestNumbersAreTrimmedAndStable(t *testing.T) {
	b, finish := open(t)
	b.Polyline([]ir.Point{{X: 1.0, Y: -0.0001}, {X: 2.5, Y: 3.0}}, ir.Stroke{
		Color: ir.RGB(0, 0, 0), Width: 1,
	})
	got := finish()
	if !strings.Contains(got, `points="1,0 2.5,3"`) {
		t.Errorf("numbers were not trimmed to their shortest fixed form:\n%s", got)
	}
	if strings.Contains(got, "-0 ") || strings.Contains(got, ",-0") {
		t.Errorf("negative zero leaked into the output:\n%s", got)
	}
}

func TestOutputIsDeterministic(t *testing.T) {
	draw := func() string {
		b, finish := open(t)
		var p ir.Path
		p.Rect(ir.R(1, 2, 3, 4))
		b.FillPath(&p, ir.Solid(ir.RGB(1, 2, 3)), ir.NonZero)
		b.Markers(ir.MarkerSquare, []ir.Point{{X: 1}}, ir.MarkerStyle{Size: 4, Fill: ir.RGB(9, 9, 9)})
		b.Text(ir.TextRun{Text: "hi", Font: ir.FontRef{Size: 10}, Color: ir.RGB(0, 0, 0)})
		return finish()
	}
	if a, b := draw(), draw(); a != b {
		t.Fatal("two identical renders produced different documents")
	}
}

func TestMeasureUsesTheBuiltinTableByDefault(t *testing.T) {
	b, _ := open(t)
	m := b.Measure(ir.TextRun{Text: "Hello", Font: ir.FontRef{Size: 12}})
	if m.Advance <= 0 || m.Ascent <= 0 || m.Descent <= 0 {
		t.Fatalf("implausible metrics: %+v", m)
	}
	wide := b.Measure(ir.TextRun{Text: "Hello Hello", Font: ir.FontRef{Size: 12}})
	if wide.Advance <= m.Advance {
		t.Error("a longer string must measure wider")
	}
	big := b.Measure(ir.TextRun{Text: "Hello", Font: ir.FontRef{Size: 24}})
	if math.Abs(float64(big.Advance-2*m.Advance)) > 1e-3 {
		t.Errorf("doubling the size gave %v, want %v", big.Advance, 2*m.Advance)
	}
}

func TestBadFontIsReportedOnOpen(t *testing.T) {
	var buf bytes.Buffer
	target := svg.Writer(&buf, svg.WithFont([]byte("not a font")))
	if _, err := target.Open(10, 10, 1); err == nil {
		t.Fatal("a malformed font must fail at Open, not silently fall back")
	}
}

func TestFontFamilyOverride(t *testing.T) {
	b, finish := open(t, svg.WithFontFamily("Inter, sans-serif"))
	b.Text(ir.TextRun{Text: "x", Font: ir.FontRef{Size: 10}, Color: ir.RGB(0, 0, 0)})
	if got := finish(); !strings.Contains(got, `font-family="Inter, sans-serif"`) {
		t.Errorf("family override not applied:\n%s", got)
	}
}

func TestPrettyDiffersOnlyInWhitespace(t *testing.T) {
	render := func(opts ...svg.Option) string {
		b, finish := open(t, opts...)
		var p ir.Path
		p.Rect(ir.R(0, 0, 5, 5))
		b.FillPath(&p, ir.Solid(ir.RGB(1, 2, 3)), ir.NonZero)
		return finish()
	}
	compact := render()
	pretty := render(svg.Pretty())
	if strings.ReplaceAll(pretty, "\n", "") != compact {
		t.Fatal("pretty output is not the compact output plus newlines")
	}
}
