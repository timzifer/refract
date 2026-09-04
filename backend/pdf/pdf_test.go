package pdf_test

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/timzifer/refract/backend/pdf"
	"github.com/timzifer/refract/ir"
)

// render draws with fn and returns the whole document.
func render(t *testing.T, w, h int, fn func(ir.Backend), opts ...pdf.Option) string {
	t.Helper()
	var buf bytes.Buffer
	opts = append([]pdf.Option{pdf.Uncompressed()}, opts...)
	tg := pdf.Writer(&buf, opts...)
	b, err := tg.Open(w, h, 1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	fn(b)
	if err := b.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := tg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return buf.String()
}

// content returns the page's content stream. It is found by its leading Y
// flip rather than by position, because an image writes a stream of its own
// and does it first — and that one is raw bytes, not text to scan through.
func content(t *testing.T, doc string) string {
	t.Helper()
	const marker = "stream\n1 0 0 -1 0 "
	i := strings.Index(doc, marker)
	if i < 0 {
		t.Fatal("no content stream in the document")
	}
	body := doc[i+len("stream\n"):]
	j := strings.Index(body, "\nendstream")
	if j < 0 {
		t.Fatal("unterminated content stream")
	}
	return body[:j]
}

func TestDocumentStructure(t *testing.T) {
	doc := render(t, 800, 500, func(b ir.Backend) {
		b.Polyline([]ir.Point{{X: 0, Y: 0}, {X: 10, Y: 10}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})
	})
	for _, want := range []string{
		"%PDF-1.7\n",
		"/Type /Catalog",
		"/Type /Pages",
		"/Type /Page ",
		"/MediaBox [0 0 800 500]",
		"xref\n",
		"trailer\n",
		"startxref\n",
		"%%EOF\n",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("document is missing %q", want)
		}
	}
}

// Every object offset in the cross reference table has to name the byte the
// object actually starts at, or a reader rejects the file.
func TestCrossReferenceOffsetsPointAtTheirObjects(t *testing.T) {
	doc := render(t, 100, 100, func(b ir.Backend) {
		b.Text(ir.TextRun{Text: "x", Font: ir.FontRef{Size: 10}, Color: ir.RGB(0, 0, 0)})
	})
	i := strings.Index(doc, "xref\n")
	if i < 0 {
		t.Fatal("no xref table")
	}
	lines := strings.Split(doc[i:], "\n")
	entry := regexp.MustCompile(`^(\d{10}) 00000 n $`)
	n := 0
	// lines[0] is "xref", lines[1] the subsection header and lines[2] the free
	// entry for object zero; the numbered objects start after those.
	for _, l := range lines[3:] {
		m := entry.FindStringSubmatch(l)
		if m == nil {
			break
		}
		n++
		var off int
		for _, c := range m[1] {
			off = off*10 + int(c-'0')
		}
		want := itoa(n) + " 0 obj"
		if !strings.HasPrefix(doc[off:], want) {
			t.Errorf("object %d: offset %d starts %q, want %q", n, off, doc[off:off+len(want)], want)
		}
	}
	if n == 0 {
		t.Fatal("the xref table has no entries")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// The page flips Y once, at the top of the content stream, so that every
// coordinate below it is refract's own top-left space.
func TestPageFlipsYOnce(t *testing.T) {
	c := content(t, render(t, 300, 200, func(b ir.Backend) {
		var p ir.Path
		p.Rect(ir.R(0, 0, 10, 10))
		b.FillPath(&p, ir.Solid(ir.RGB(1, 2, 3)), ir.NonZero)
	}))
	if !strings.HasPrefix(c, "1 0 0 -1 0 200 cm\n") {
		t.Errorf("content starts %q", firstLine(c))
	}
	if strings.Count(c, "0 -1 0 200 cm") != 1 {
		t.Errorf("the flip appears more than once:\n%s", c)
	}
	// The rectangle's own coordinates are written unchanged.
	if !strings.Contains(c, "0 0 m\n10 0 l\n10 10 l\n0 10 l\nh\nf\n") {
		t.Errorf("rectangle path not emitted verbatim:\n%s", c)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Text is placed with a matrix that undoes the page flip for the glyphs alone,
// and the anchoring offset is applied in text space so that rotation carries
// it round with the run.
func TestTextMatrixUndoesTheFlip(t *testing.T) {
	c := content(t, render(t, 300, 200, func(b ir.Backend) {
		b.Text(ir.TextRun{
			Text: "Hi", Font: ir.FontRef{Size: 10},
			At: ir.Point{X: 50, Y: 60}, Color: ir.RGB(0, 0, 0),
		})
	}))
	if !strings.Contains(c, "1 0 0 -1 50 60 Tm") {
		t.Errorf("text matrix missing:\n%s", c)
	}
	if !strings.Contains(c, "(Hi) Tj") {
		t.Errorf("text not emitted:\n%s", c)
	}
	if !strings.Contains(c, "/Helvetica") == false && !strings.Contains(c, "Tf") {
		t.Errorf("no font selected:\n%s", c)
	}
}

func TestTextAlignmentOffsetsAreInTextSpace(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  ir.TextRun
		want string
	}{
		{"start-baseline", ir.TextRun{}, ""},
		{"centred", ir.TextRun{H: ir.AlignCenter}, "Td"},
		{"top", ir.TextRun{V: ir.AlignTop}, "Td"},
	} {
		run := tc.run
		run.Text, run.Color, run.Font = "Hg", ir.RGB(0, 0, 0), ir.FontRef{Size: 10}
		c := content(t, render(t, 100, 100, func(b ir.Backend) { b.Text(run) }))
		if got := strings.Contains(c, "Td"); got != (tc.want != "") {
			t.Errorf("%s: Td present = %v, want %v:\n%s", tc.name, got, tc.want != "", c)
		}
	}

	// A top-aligned run drops by the ascent, and text space has Y up, so the
	// offset is negative.
	c := content(t, render(t, 100, 100, func(b ir.Backend) {
		b.Text(ir.TextRun{Text: "Hg", Font: ir.FontRef{Size: 10}, V: ir.AlignTop, Color: ir.RGB(0, 0, 0)})
	}))
	if !strings.Contains(c, "0 -7.18 Td") {
		t.Errorf("top alignment offset wrong:\n%s", c)
	}
}

func TestRotatedTextComposesIntoTheTextMatrix(t *testing.T) {
	c := content(t, render(t, 100, 100, func(b ir.Backend) {
		b.Text(ir.TextRun{
			Text: "y", Font: ir.FontRef{Size: 10}, At: ir.Point{X: 20, Y: 30},
			Rotation: -1.5707963267948966, Color: ir.RGB(0, 0, 0),
		})
	}))
	// cos(-pi/2) is 0 and sin is -1, giving [0 -1 -1 0].
	if !strings.Contains(c, "0 -1 -1 0 20 30 Tm") {
		t.Errorf("rotated text matrix wrong:\n%s", c)
	}
}

func TestBoldAndItalicPickABaseFont(t *testing.T) {
	doc := render(t, 100, 100, func(b ir.Backend) {
		b.Text(ir.TextRun{Text: "a", Font: ir.FontRef{Size: 10, Weight: 700}, Color: ir.RGB(0, 0, 0)})
		b.Text(ir.TextRun{Text: "b", Font: ir.FontRef{Size: 10, Italic: true}, Color: ir.RGB(0, 0, 0)})
		b.Text(ir.TextRun{Text: "c", Font: ir.FontRef{Size: 10, Weight: 700, Italic: true}, Color: ir.RGB(0, 0, 0)})
		b.Text(ir.TextRun{Text: "d", Font: ir.FontRef{Size: 10}, Color: ir.RGB(0, 0, 0)})
	})
	for _, want := range []string{
		"/BaseFont /Helvetica-Bold ",
		"/BaseFont /Helvetica-Oblique ",
		"/BaseFont /Helvetica-BoldOblique ",
		"/BaseFont /Helvetica ",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// A font object is written once however many runs use it.
func TestFontsAreInterned(t *testing.T) {
	doc := render(t, 100, 100, func(b ir.Backend) {
		for range 5 {
			b.Text(ir.TextRun{Text: "a", Font: ir.FontRef{Size: 10}, Color: ir.RGB(0, 0, 0)})
		}
	})
	if got := strings.Count(doc, "/BaseFont /Helvetica "); got != 1 {
		t.Errorf("wrote the font object %d times, want 1", got)
	}
}

// WinAnsi covers Latin-1 and the usual punctuation; anything else becomes a
// visible "?" rather than vanishing behind a label that was measured wider.
func TestStringEncoding(t *testing.T) {
	c := content(t, render(t, 100, 100, func(b ir.Backend) {
		b.Text(ir.TextRun{Text: `a(b)c\d`, Font: ir.FontRef{Size: 10}, Color: ir.RGB(0, 0, 0)})
		b.Text(ir.TextRun{Text: "µ€—", Font: ir.FontRef{Size: 10}, Color: ir.RGB(0, 0, 0)})
		b.Text(ir.TextRun{Text: "日本", Font: ir.FontRef{Size: 10}, Color: ir.RGB(0, 0, 0)})
	}))
	if !strings.Contains(c, `(a\(b\)c\\d) Tj`) {
		t.Errorf("delimiters not escaped:\n%s", c)
	}
	if !strings.Contains(c, "(\xB5\x80\x97) Tj") {
		t.Errorf("WinAnsi high range wrong:\n%s", c)
	}
	if !strings.Contains(c, "(??) Tj") {
		t.Errorf("unrepresentable runes should become ?:\n%s", c)
	}
}

func TestStrokeStyleIsEmitted(t *testing.T) {
	c := content(t, render(t, 100, 100, func(b ir.Backend) {
		b.Polyline([]ir.Point{{}, {X: 5, Y: 5}}, ir.Stroke{
			Color: ir.RGB(255, 0, 0), Width: 2,
			Cap: ir.CapRound, Join: ir.JoinBevel, MiterLimit: 8,
			Dash: []float32{4, 3}, DashOffset: 1,
		})
	}))
	for _, want := range []string{"1 0 0 RG", "2 w", "1 J", "2 j", "8 M", "[4 3] 1 d", "S"} {
		if !strings.Contains(c, want) {
			t.Errorf("missing %q:\n%s", want, c)
		}
	}
}

func TestEvenOddFillRule(t *testing.T) {
	c := content(t, render(t, 100, 100, func(b ir.Backend) {
		var p ir.Path
		p.Rect(ir.R(0, 0, 5, 5))
		b.FillPath(&p, ir.Solid(ir.RGB(0, 0, 0)), ir.EvenOdd)
	}))
	if !strings.Contains(c, "\nf*\n") {
		t.Errorf("even-odd fill not emitted:\n%s", c)
	}
}

// Alpha is a graphics-state constant in PDF, so a faded fill costs one
// interned ExtGState and no per-operation state.
func TestAlphaBecomesAnExtGState(t *testing.T) {
	doc := render(t, 100, 100, func(b ir.Backend) {
		var p ir.Path
		p.Rect(ir.R(0, 0, 5, 5))
		b.FillPath(&p, ir.Solid(ir.RGBA(0, 0, 255, 64)), ir.NonZero)
		b.FillPath(&p, ir.Solid(ir.RGBA(0, 255, 0, 64)), ir.NonZero)
	})
	if got := strings.Count(doc, "/Type /ExtGState"); got != 1 {
		t.Errorf("wrote %d graphics states, want 1", got)
	}
	if !strings.Contains(doc, "/ca 0.251") {
		t.Errorf("fill alpha not written:\n%s", doc)
	}
	if !strings.Contains(doc, "/ExtGState <<") {
		t.Error("the resource dictionary has no /ExtGState")
	}
}

func TestOpaqueDrawingNeedsNoGraphicsState(t *testing.T) {
	doc := render(t, 100, 100, func(b ir.Backend) {
		var p ir.Path
		p.Rect(ir.R(0, 0, 5, 5))
		b.FillPath(&p, ir.Solid(ir.RGB(0, 0, 0)), ir.NonZero)
	})
	if strings.Contains(doc, "ExtGState") {
		t.Errorf("an opaque chart should not need a graphics state:\n%s", doc)
	}
}

// A gradient is painted with `sh` through the path as a clip, which is how PDF
// spells a filled gradient.
func TestGradientBecomesAnAxialShading(t *testing.T) {
	doc := render(t, 100, 100, func(b ir.Backend) {
		var p ir.Path
		p.Rect(ir.R(0, 0, 20, 60))
		b.FillPath(&p, ir.Fill{
			Start: ir.Point{X: 0, Y: 60},
			End:   ir.Point{X: 0, Y: 0},
			Stops: []ir.GradientStop{
				{Offset: 0, Color: ir.RGB(0, 0, 0)},
				{Offset: 0.5, Color: ir.RGB(255, 0, 0)},
				{Offset: 1, Color: ir.RGB(255, 255, 255)},
			},
		}, ir.NonZero)
	})
	for _, want := range []string{
		"/ShadingType 2",
		"/ColorSpace /DeviceRGB",
		"/Coords [0 60 0 0]",
		"/FunctionType 3",
		"/Bounds [0.5]",
		"/FunctionType 2",
		"/Shading <<",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %q:\n%s", want, doc)
		}
	}
	c := content(t, doc)
	if !strings.Contains(c, "W n\n/Sh") || !strings.Contains(c, " sh\n") {
		t.Errorf("shading not painted through a clip:\n%s", c)
	}
}

// Two stops need no stitching function: the ramp is one exponential segment.
func TestTwoStopGradientSkipsTheStitch(t *testing.T) {
	doc := render(t, 100, 100, func(b ir.Backend) {
		var p ir.Path
		p.Rect(ir.R(0, 0, 20, 20))
		b.FillPath(&p, ir.Fill{
			End: ir.Point{X: 20},
			Stops: []ir.GradientStop{
				{Offset: 0, Color: ir.RGB(0, 0, 0)},
				{Offset: 1, Color: ir.RGB(255, 255, 255)},
			},
		}, ir.NonZero)
	})
	if strings.Contains(doc, "/FunctionType 3") {
		t.Errorf("a two-stop ramp should not be stitched:\n%s", doc)
	}
}

func TestClipAndTransform(t *testing.T) {
	c := content(t, render(t, 100, 100, func(b ir.Backend) {
		var clip ir.Path
		clip.Rect(ir.R(1, 2, 3, 4))
		b.Push(&clip, ir.Translate(5, 6))
		b.Polyline([]ir.Point{{}, {X: 1, Y: 1}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})
		b.Pop()
	}))
	if !strings.Contains(c, "1 0 0 1 5 6 cm") {
		t.Errorf("transform not emitted:\n%s", c)
	}
	if !strings.Contains(c, "W n") {
		t.Errorf("clip not emitted:\n%s", c)
	}
	if strings.Count(c, "q\n") < 2 || strings.Count(c, "Q\n") < 2 {
		t.Errorf("state stack unbalanced:\n%s", c)
	}
}

func TestPopWithoutPushIsAnError(t *testing.T) {
	tg := pdf.Writer(&bytes.Buffer{})
	b, err := tg.Open(10, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	b.Pop()
	if err := b.Flush(); err == nil {
		t.Error("Flush accepted an unmatched Pop")
	}
}

func TestUnclosedPushIsAnError(t *testing.T) {
	tg := pdf.Writer(&bytes.Buffer{})
	b, err := tg.Open(10, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	b.Push(nil, ir.Identity)
	if err := b.Flush(); err == nil {
		t.Error("Flush accepted an unclosed Push")
	}
}

func TestMarkersFillAndStroke(t *testing.T) {
	c := content(t, render(t, 100, 100, func(b ir.Backend) {
		b.Markers(ir.MarkerCircle, []ir.Point{{X: 10, Y: 10}, {X: 20, Y: 20}}, ir.MarkerStyle{
			Size: 6, Fill: ir.RGB(0, 0, 255),
			Stroke: ir.Stroke{Color: ir.RGB(255, 255, 255), Width: 1},
		})
	}))
	if got := strings.Count(c, "\nB\n"); got != 2 {
		t.Errorf("painted %d markers with fill+stroke, want 2:\n%s", got, c)
	}
	if got := strings.Count(c, " c\n"); got != 8 {
		t.Errorf("emitted %d curve segments, want 8 (two circles):\n%s", got, c)
	}
}

func TestMarkersFillOnly(t *testing.T) {
	c := content(t, render(t, 100, 100, func(b ir.Backend) {
		b.Markers(ir.MarkerSquare, []ir.Point{{X: 10, Y: 10}}, ir.MarkerStyle{
			Size: 6, Fill: ir.RGB(0, 0, 255),
		})
	}))
	if strings.Contains(c, "\nB\n") || !strings.Contains(c, "\nf\n") {
		t.Errorf("a marker with no stroke should be filled only:\n%s", c)
	}
}

func TestImageBecomesAnXObject(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	img.Set(1, 1, color.NRGBA{B: 255, A: 128})
	doc := render(t, 100, 100, func(b ir.Backend) {
		b.Image(img, ir.R(10, 20, 30, 40))
	})
	for _, want := range []string{
		"/Subtype /Image",
		"/Width 2 /Height 2",
		"/ColorSpace /DeviceRGB",
		"/SMask ",
		"/XObject <<",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %q", want)
		}
	}
	// The placement matrix flips back, so the image's first row lands at the
	// top of the destination rectangle.
	if !strings.Contains(content(t, doc), "20 0 0 -20 10 40 cm") {
		t.Errorf("image placement matrix wrong:\n%s", content(t, doc))
	}
}

func TestOpaqueImageHasNoSoftMask(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	doc := render(t, 100, 100, func(b ir.Backend) { b.Image(img, ir.R(0, 0, 1, 1)) })
	if strings.Contains(doc, "/SMask") {
		t.Error("an opaque image should carry no soft mask")
	}
}

// The metrics table this backend measures with is Helvetica's own, which is
// the font it draws with — so unlike every other backend, measurement here is
// exact rather than approximate.
func TestMeasureIsHelvetica(t *testing.T) {
	tg := pdf.Writer(&bytes.Buffer{})
	b, err := tg.Open(100, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	m := b.Measure(ir.TextRun{Text: "Hi", Font: ir.FontRef{Size: 1000}})
	// H is 722/1000 em and i is 222/1000 in Helvetica.
	if m.Advance != 944 {
		t.Errorf("Advance = %v, want 944", m.Advance)
	}
	if m.Ascent != 718 || m.Descent != 207 {
		t.Errorf("Ascent/Descent = %v/%v, want 718/207", m.Ascent, m.Descent)
	}
	if m.Ink != ir.R(0, -718, 944, 207) {
		t.Errorf("Ink = %v", m.Ink)
	}
}

func TestMeasureDefaultsAZeroSize(t *testing.T) {
	tg := pdf.Writer(&bytes.Buffer{})
	b, _ := tg.Open(100, 100, 1)
	if got := b.Measure(ir.TextRun{Text: "x"}).Advance; got <= 0 {
		t.Errorf("Advance = %v with no font size set", got)
	}
}

// Golden files and CI diffs both depend on two runs of the same chart being
// the same bytes.
func TestOutputIsDeterministic(t *testing.T) {
	draw := func(b ir.Backend) {
		var p ir.Path
		p.Rect(ir.R(1, 2, 3, 4))
		b.FillPath(&p, ir.Solid(ir.RGBA(1, 2, 3, 128)), ir.NonZero)
		b.Text(ir.TextRun{Text: "a", Font: ir.FontRef{Size: 10}, Color: ir.RGB(0, 0, 0)})
		b.Text(ir.TextRun{Text: "b", Font: ir.FontRef{Size: 10, Weight: 700}, Color: ir.RGB(0, 0, 0)})
		b.Markers(ir.MarkerPlus, []ir.Point{{X: 5, Y: 5}}, ir.MarkerStyle{Size: 4, Fill: ir.RGB(9, 9, 9)})
	}
	a := render(t, 120, 90, draw)
	c := render(t, 120, 90, draw)
	if a != c {
		t.Error("two renders of the same chart differ")
	}
}

func TestMetadataGoesInTheInfoDictionary(t *testing.T) {
	doc := render(t, 100, 100, func(b ir.Backend) {
		b.Polyline([]ir.Point{{}, {X: 1, Y: 1}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})
	}, pdf.Title("T"), pdf.Author("A"), pdf.Subject("S"))
	for _, want := range []string{"/Title (T)", "/Author (A)", "/Subject (S)", "/Producer (refract)", "/Info "} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestNoMetadataMeansNoInfoDictionary(t *testing.T) {
	doc := render(t, 100, 100, func(b ir.Backend) {
		b.Polyline([]ir.Point{{}, {X: 1, Y: 1}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})
	})
	if strings.Contains(doc, "/Info ") {
		t.Error("an /Info reference was written with no metadata to put in it")
	}
}

// Compression is the default; the uncompressed form exists so a human can read
// what was emitted.
func TestCompressionIsTheDefault(t *testing.T) {
	var buf bytes.Buffer
	tg := pdf.Writer(&buf)
	b, err := tg.Open(100, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	b.Polyline([]ir.Point{{}, {X: 1, Y: 1}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := tg.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "/Filter /FlateDecode") {
		t.Error("the content stream was not deflated")
	}
}

func TestFileTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chart.pdf")
	tg := pdf.File(path)
	b, err := tg.Open(200, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	b.Polyline([]ir.Point{{}, {X: 1, Y: 1}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := tg.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(got, []byte("%PDF-1.7")) {
		t.Error("the file does not start with a PDF header")
	}
}

func TestOpenRejectsAnEmptyCanvas(t *testing.T) {
	if _, err := pdf.Writer(&bytes.Buffer{}).Open(0, 10, 1); err == nil {
		t.Error("Open accepted a zero width")
	}
}

func TestCloseBeforeFlushIsAnError(t *testing.T) {
	tg := pdf.Writer(&bytes.Buffer{})
	if _, err := tg.Open(10, 10, 1); err != nil {
		t.Fatal(err)
	}
	if err := tg.Close(); err == nil {
		t.Error("Close before Flush should report that nothing was assembled")
	}
}

// Nothing invisible should reach the content stream: it is the cheapest
// possible optimisation and it keeps the output readable.
func TestInvisibleDrawingIsSkipped(t *testing.T) {
	c := content(t, render(t, 100, 100, func(b ir.Backend) {
		b.Polyline([]ir.Point{{}, {X: 1}}, ir.Stroke{Color: ir.Transparent, Width: 1})
		b.Polyline([]ir.Point{{}}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})
		b.StrokePath(&ir.Path{}, ir.Stroke{Color: ir.RGB(0, 0, 0), Width: 1})
		b.FillPath(&ir.Path{}, ir.Solid(ir.RGB(0, 0, 0)), ir.NonZero)
		b.Text(ir.TextRun{Text: "", Font: ir.FontRef{Size: 10}, Color: ir.RGB(0, 0, 0)})
		b.Text(ir.TextRun{Text: "x", Font: ir.FontRef{Size: 10}, Color: ir.Transparent})
		b.Markers(ir.MarkerCircle, nil, ir.MarkerStyle{Size: 4, Fill: ir.RGB(0, 0, 0)})
		b.Markers(ir.MarkerCircle, []ir.Point{{}}, ir.MarkerStyle{Size: 0, Fill: ir.RGB(0, 0, 0)})
		b.Markers(ir.MarkerCircle, []ir.Point{{}}, ir.MarkerStyle{Size: 4})
		b.Image(nil, ir.R(0, 0, 1, 1))
		b.Image(image.NewNRGBA(image.Rect(0, 0, 1, 1)), ir.Rect{})
	}))
	if strings.TrimSpace(strings.TrimPrefix(c, "1 0 0 -1 0 100 cm\n")) != "" {
		t.Errorf("invisible drawing reached the stream:\n%s", c)
	}
}
