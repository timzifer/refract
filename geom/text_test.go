package geom_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// gantt is the shape this mark exists for: a row with two edges on one axis, a
// slot on the other, a label and a state to colour it by.
func gantt() *data.Table {
	return data.NewTable().
		Float64("start", []float64{0, 4}).
		Float64("end", []float64{4, 10}).
		Float64("lo", []float64{0, 5}).
		Float64("hi", []float64{5, 10}).
		String("label", []string{"a", "b"}).
		String("state", []string{"run", "stop"})
}

// oneRow is the same shape with a single wide box and a long label, which is
// where the fit decisions are visible.
func oneRow(label string) *data.Table {
	return data.NewTable().
		Float64("start", []float64{0}).
		Float64("end", []float64{10}).
		Float64("lo", []float64{0}).
		Float64("hi", []float64{1}).
		String("label", []string{label})
}

// textFrame trains one layer into a pair of scales over an area, the way
// render does.
func textFrame(t *testing.T, g geom.Geom, area ir.Rect) geom.Frame {
	t.Helper()
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	x.SetRange(area.Min.X, area.Max.X)
	y.SetRange(area.Max.Y, area.Min.Y)
	return geom.Frame{Area: area, X: x, Y: y, Theme: theme.Light}
}

func boxed(src data.Source, opts ...geom.Option) geom.Geom {
	return geom.Text(src, append([]geom.Option{
		geom.X("start"), geom.X2("end"), geom.Y("lo"), geom.Y2("hi"), geom.TextBy("label"),
	}, opts...)...)
}

type rowsFunc func(at []ir.Point, rows []int)

func (f rowsFunc) Marks(at []ir.Point, rows []int) { f(at, rows) }

func TestALabelSitsInTheMiddleOfItsBox(t *testing.T) {
	g := boxed(gantt())
	f := textFrame(t, g, ir.R(0, 0, 100, 100))
	runs := draw(t, f, g).Filter("Text")
	if len(runs) != 2 {
		t.Fatalf("emitted %d runs, want 2", len(runs))
	}
	// The first row spans x 0..4 of a 0..10 domain and y 0..5, so its middle is
	// at x = 20 and — the Y range running downwards — y = 75.
	if !samePoint(runs[0].Text.At, ir.Point{X: 20, Y: 75}) {
		t.Errorf("label at %v, want the middle of its box at (20, 75)", runs[0].Text.At)
	}
	if runs[0].Text.H != ir.AlignCenter || runs[0].Text.V != ir.AlignMiddle {
		t.Errorf("alignment %v/%v, want centred in the box", runs[0].Text.H, runs[0].Text.V)
	}
}

// The case the mark exists for: a bar scrolled half off the edge still carries
// its label, in the middle of what is left of it rather than off-screen with
// the box's true centre.
func TestALabelFollowsTheVisiblePartOfItsBox(t *testing.T) {
	g := boxed(gantt())
	f := textFrame(t, g, ir.R(0, 0, 100, 100))

	// Pan the view so the first box, 0..4, is half outside on the left: a
	// domain of 2..10 leaves 2..4 of it showing, whose middle is 3.
	f.X.(scale.Zoomer).SetDomain(2, 10)
	runs := draw(t, f, g).Filter("Text")
	if len(runs) == 0 {
		t.Fatal("the half-visible box lost its label")
	}
	// 2..4 of a 2..10 domain across 100px is 0..25, so the visible middle is at
	// 12.5 — not the 6.25 the box's true centre would map to.
	if !samePoint(runs[0].Text.At, ir.Point{X: 12.5, Y: 75}) {
		t.Errorf("label at %v, want the middle of the visible part at (12.5, 75)", runs[0].Text.At)
	}
}

func TestALabelTooWideForItsBoxIsDropped(t *testing.T) {
	// A plot four pixels wide has no room for twenty characters.
	narrow := boxed(oneRow("Wareneingang Halle 3"))
	f := textFrame(t, narrow, ir.R(0, 0, 4, 100))
	if runs := draw(t, f, narrow).Filter("Text"); len(runs) != 0 {
		t.Errorf("drew %q in a box with no room for it", runs[0].Text.Text)
	}

	// The same layer with room draws the whole label.
	wide := boxed(oneRow("Wareneingang Halle 3"))
	f = textFrame(t, wide, ir.R(0, 0, 400, 100))
	runs := draw(t, f, wide).Filter("Text")
	if len(runs) != 1 || runs[0].Text.Text != "Wareneingang Halle 3" {
		t.Errorf("drew %v, want the whole label", runs)
	}
}

func TestElideTruncatesInsteadOfDropping(t *testing.T) {
	const label = "Wareneingang Halle 3"
	g := boxed(oneRow(label), geom.Elide(true))
	f := textFrame(t, g, ir.R(0, 0, 60, 100))

	runs := draw(t, f, g).Filter("Text")
	if len(runs) != 1 {
		t.Fatalf("emitted %d runs, want 1 truncated one", len(runs))
	}
	got := runs[0].Text.Text
	if !strings.HasSuffix(got, "…") {
		t.Errorf("drew %q, want it to end in an ellipsis", got)
	}
	if !strings.HasPrefix(label, strings.TrimSuffix(got, "…")) {
		t.Errorf("drew %q, which is not a prefix of the label", got)
	}
	// And what it drew has to fit, or eliding bought nothing.
	w := irtest.New().Measure(ir.TextRun{Text: got, Font: theme.Light.Font(theme.Light.LabelSize)}).Advance
	if w > 60 {
		t.Errorf("the elided label is %v wide, in a box of 60", w)
	}
}

// A box with room for nothing at all drops the label even when eliding: an
// ellipsis on its own names no row.
func TestElideStillDropsWhatCannotBeCutToFit(t *testing.T) {
	g := boxed(oneRow("Wareneingang"), geom.Elide(true))
	f := textFrame(t, g, ir.R(0, 0, 3, 100))
	if runs := draw(t, f, g).Filter("Text"); len(runs) != 0 {
		t.Errorf("drew %q in a box three pixels wide", runs[0].Text.Text)
	}
}

// A cut lands on a rune boundary: half a character is not a shorter label, and
// a backend handed one would draw a replacement glyph.
func TestAnElidedLabelIsCutBetweenRunes(t *testing.T) {
	g := boxed(oneRow("Wareneingangsprüfung Süd"), geom.Elide(true))
	for w := float32(8); w < 120; w += 3 {
		f := textFrame(t, g, ir.R(0, 0, w, 100))
		for _, c := range draw(t, f, g).Filter("Text") {
			if !utf8.ValidString(c.Text.Text) {
				t.Fatalf("at %v wide the label was cut mid-rune: %q", w, c.Text.Text)
			}
		}
	}
}

// Point mode is the other half of the mark: no box, so no fit check and
// nothing dropped, and the anchor is laid out about the point as a note is.
func TestPointModePlacesTheLabelOnItsRow(t *testing.T) {
	tbl := data.NewTable().
		Float64("t", []float64{0, 10}).
		Float64("v", []float64{0, 10}).
		String("name", []string{"a long name that would never fit a box", "b"})
	g := geom.Text(tbl, geom.X("t"), geom.Y("v"), geom.TextBy("name"))
	f := textFrame(t, g, ir.R(0, 0, 100, 100))

	runs := draw(t, f, g).Filter("Text")
	if len(runs) != 2 {
		t.Fatalf("emitted %d runs, want 2 — point mode drops nothing", len(runs))
	}
	if !samePoint(runs[0].Text.At, ir.Point{X: 0, Y: 100}) {
		t.Errorf("label at %v, want its row's point at (0, 100)", runs[0].Text.At)
	}
	if runs[0].Text.H != ir.AlignStart || runs[0].Text.V != ir.AlignBaseline {
		t.Error("point mode should hang the label off the point, as a note does")
	}
}

func TestAlignIsHonouredWhereTheLayerWasTold(t *testing.T) {
	g := boxed(gantt(), geom.Align(ir.AlignStart, ir.AlignTop))
	f := textFrame(t, g, ir.R(0, 0, 100, 100))
	runs := draw(t, f, g).Filter("Text")
	if runs[0].Text.H != ir.AlignStart || runs[0].Text.V != ir.AlignTop {
		t.Errorf("alignment %v/%v, want what the layer was told", runs[0].Text.H, runs[0].Text.V)
	}
}

func TestInkFollowsTheFillItIsDrawnOn(t *testing.T) {
	pal := scale.Qualitative(palette.Qualitative{palette.White, palette.Black})
	g := boxed(gantt(), geom.ColorBy("state", pal))
	f := textFrame(t, g, ir.R(0, 0, 400, 100))

	runs := draw(t, f, g).Filter("Text")
	if len(runs) != 2 {
		t.Fatalf("emitted %d runs, want 2", len(runs))
	}
	onWhite := palette.Luminance(runs[0].Text.Color)
	onBlack := palette.Luminance(runs[1].Text.Color)
	if onWhite >= onBlack {
		t.Errorf("ink on white is %v and on black %v: it does not follow the fill", onWhite, onBlack)
	}
}

func TestAnExplicitColourBeatsTheContrast(t *testing.T) {
	pal := scale.Qualitative(palette.Qualitative{palette.White, palette.Black})
	g := boxed(gantt(), geom.ColorBy("state", pal), geom.Color(palette.Red))
	f := textFrame(t, g, ir.R(0, 0, 400, 100))
	for _, c := range draw(t, f, g).Filter("Text") {
		if c.Text.Color != palette.Red {
			t.Errorf("ink is %v, want the colour the layer was given", c.Text.Color)
		}
	}
}

// A row with no position is a row with nothing to label — the same hole every
// other mark leaves, decided by the same test.
func TestARowWithNoPositionDrawsNoLabel(t *testing.T) {
	tbl := data.NewTable().
		Float64("t", []float64{0, 1}).
		Float64("v", []float64{0, 10}).
		String("name", []string{"zero", "ten"})
	g := geom.Text(tbl, geom.X("t"), geom.Y("v"), geom.TextBy("name"))
	x, y := scale.Linear(), scale.Log()
	if err := g.Train(x, y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	x.SetRange(0, 100)
	y.SetRange(100, 0)
	f := geom.Frame{Area: ir.R(0, 0, 100, 100), X: x, Y: y, Theme: theme.Light}

	r := irtest.New()
	if err := g.Build(r, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, c := range r.Filter("Text") {
		if c.Text.Text == "zero" {
			t.Error("a log scale has no position for zero, but its label was drawn")
		}
	}
}

// An empty label is not a label. Drawing it would put an invisible mark in the
// row list and an empty run in front of a backend.
func TestAnEmptyLabelDrawsNothing(t *testing.T) {
	tbl := data.NewTable().
		Float64("t", []float64{0, 1}).
		Float64("v", []float64{0, 1}).
		String("name", []string{"", "b"})
	g := geom.Text(tbl, geom.X("t"), geom.Y("v"), geom.TextBy("name"))
	f := textFrame(t, g, ir.R(0, 0, 100, 100))
	if runs := draw(t, f, g).Filter("Text"); len(runs) != 1 {
		t.Fatalf("emitted %d runs, want 1", len(runs))
	}
}

func TestAMissingLabelColumnIsAnError(t *testing.T) {
	for _, g := range []geom.Geom{
		geom.Text(gantt(), geom.X("start"), geom.Y("lo"), geom.TextBy("nope")),
		geom.Text(gantt(), geom.X("start"), geom.Y("lo")),
	} {
		if err := g.Train(scale.Linear(), scale.Linear()); err == nil {
			t.Error("a layer with nothing to say built without complaint")
		}
	}
}

// Any column will do, and a number is spelled the way a category name is — so
// a label and an axis tick for the same value are the same string.
func TestALabelColumnMayBeNumeric(t *testing.T) {
	tbl := data.NewTable().
		Float64("t", []float64{0, 1}).
		Float64("v", []float64{2.5, 7})
	g := geom.Text(tbl, geom.X("t"), geom.Y("v"), geom.TextBy("v"))
	f := textFrame(t, g, ir.R(0, 0, 100, 100))
	got := draw(t, f, g).Texts()
	if len(got) != 2 || got[0] != data.FormatNumber(2.5) {
		t.Errorf("drew %q, want the numbers formatted as category names", got)
	}
}

func TestATextLayerContributesNoLegendEntry(t *testing.T) {
	g := geom.Text(gantt(), geom.X("start"), geom.Y("lo"), geom.TextBy("label"),
		geom.Label("named"))
	f := textFrame(t, g, ir.R(0, 0, 100, 100))
	if _, ok := g.Legend(f); ok {
		t.Error("a text layer contributed a legend entry")
	}
}

// It holds rows, so faceting has to be able to cut it — and a row it reports
// has to be a row of the caller's table rather than of the cut.
func TestATextLayerFacetsAndReportsRowsInTheCallersTable(t *testing.T) {
	g, ok := boxed(gantt()).(geom.Faceter)
	if !ok {
		t.Fatal("a text layer holds rows and must implement geom.Faceter")
	}
	sub := g.Subset([]int{1})
	f := textFrame(t, sub, ir.R(0, 0, 400, 100))

	var got []int
	f.Rows = rowsFunc(func(at []ir.Point, rows []int) { got = append(got, rows...) })
	if err := sub.Build(irtest.New(), f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("reported rows %v, want the caller's row 1", got)
	}
}

// A dropped label is not a mark a pointer can land on, so it must not be
// reported as one.
func TestADroppedLabelIsNotReportedAsAMark(t *testing.T) {
	g := boxed(oneRow("Wareneingang Halle 3"))
	f := textFrame(t, g, ir.R(0, 0, 4, 100))

	n := 0
	f.Rows = rowsFunc(func(at []ir.Point, rows []int) { n += len(rows) })
	if err := g.Build(irtest.New(), f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if n != 0 {
		t.Errorf("reported %d marks for a label that was never drawn", n)
	}
}

// The mark a row is reported at is the label's own anchor, so a tooltip names
// the row whose text the pointer is on.
func TestAReportedMarkIsWhereTheLabelIs(t *testing.T) {
	g := boxed(gantt())
	f := textFrame(t, g, ir.R(0, 0, 400, 100))

	var at []ir.Point
	f.Rows = rowsFunc(func(pts []ir.Point, rows []int) { at = append(at, pts...) })
	r := irtest.New()
	if err := g.Build(r, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	runs := r.Filter("Text")
	if len(at) != len(runs) {
		t.Fatalf("reported %d marks for %d labels", len(at), len(runs))
	}
	for i := range runs {
		if !samePoint(at[i], runs[i].Text.At) {
			t.Errorf("mark %d reported at %v, label drawn at %v", i, at[i], runs[i].Text.At)
		}
	}
}

// A text layer must be describable and must read back as itself, which is what
// keeps it a first-class citizen of the document rather than a special case.
func TestATextLayerDescribesItself(t *testing.T) {
	g := boxed(gantt(), geom.Elide(true))
	d, ok := geom.Describe(g)
	if !ok {
		t.Fatal("a text layer is not describable")
	}
	if d.Mark != geom.MarkText || d.TextCol != "label" || !d.Elide {
		t.Errorf("described as %+v, want a text mark labelled from \"label\" that elides", d)
	}
	back, err := geom.FromDesc(d)
	if err != nil {
		t.Fatalf("FromDesc: %v", err)
	}
	if _, ok := geom.Describe(back); !ok {
		t.Error("what came back is not a text layer")
	}
}
