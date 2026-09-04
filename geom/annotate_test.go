package geom_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/internal/svgdiff"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// Device coordinates come out of a float32 mapping, and arm64 and amd64
// disagree about the last bit of one: Go may contract `a*b + c` into a fused
// multiply-add, and one architecture does while the other does not. Mapping 8
// onto a 0..10 domain across a hundred pixels lands on 20 exactly on amd64 and
// on 19.999998 on arm64 — visually the same place, a different float32.
//
// So an annotation's geometry is compared the way the golden files are: with a
// hundredth of a pixel of slack, which is two orders of magnitude below
// anything visible and four above the disagreement it exists to absorb. See
// internal/svgdiff for the full reasoning.
const coordEps = svgdiff.DefaultTolerance

func sameRect(a, b ir.Rect) bool {
	return samePoint(a.Min, b.Min) && samePoint(a.Max, b.Max)
}

func samePoint(a, b ir.Point) bool {
	return math.Abs(float64(a.X-b.X)) <= coordEps && math.Abs(float64(a.Y-b.Y)) <= coordEps
}

// The slack has to be wide enough for the disagreement and narrow enough to
// catch a real move. A test that compared device coordinates exactly is what
// turned CI red on macOS; one that compared them loosely would have caught
// nothing.
func TestTheCoordinateSlackIsTheRightWidth(t *testing.T) {
	// One float32 unit in the last place near 20, which is what an FMA
	// contraction costs: 20 came back as 19.999998 on arm64.
	if !sameRect(ir.R(0, 19.999998, 100, 80), ir.R(0, 20, 100, 80)) {
		t.Error("the slack is too narrow to absorb a float32 ulp")
	}
	// A tenth of a pixel is not noise; something moved.
	if sameRect(ir.R(0, 20.1, 100, 80), ir.R(0, 20, 100, 80)) {
		t.Error("the slack is wide enough to hide a real change")
	}
	if samePoint(ir.Point{X: 0, Y: 20.1}, ir.Point{X: 0, Y: 20}) {
		t.Error("the slack is wide enough to hide a real change")
	}
}

// annotFrame trains every layer into one pair of scales and returns the frame,
// the way render does — an annotation's position depends on what the data
// layers put in the domain.
func annotFrame(t *testing.T, gs ...geom.Geom) geom.Frame {
	t.Helper()
	x := scale.Linear()
	y := scale.Linear()
	for _, g := range gs {
		if err := g.Train(x, y); err != nil {
			t.Fatalf("Train: %v", err)
		}
	}
	x.SetRange(0, 100)
	y.SetRange(100, 0)
	return geom.Frame{
		Area:  ir.R(0, 0, 100, 100),
		X:     x,
		Y:     y,
		Theme: theme.Light,
	}
}

func draw(t *testing.T, f geom.Frame, g geom.Geom) *irtest.Recorder {
	t.Helper()
	r := irtest.New()
	if err := g.Build(r, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return r
}

func TestHLineSpansThePlot(t *testing.T) {
	line := geom.HLine(5)
	f := annotFrame(t, geom.Line(src(map[string][]float64{"x": {0, 10}, "y": {0, 10}}),
		geom.X("x"), geom.Y("y")), line)
	r := draw(t, f, line)

	calls := r.Filter("Polyline")
	if len(calls) != 1 {
		t.Fatalf("emitted %d polylines, want 1", len(calls))
	}
	pts := calls[0].Points
	if pts[0].X != 0 || pts[1].X != 100 {
		t.Errorf("line runs from %v to %v, want the full width", pts[0], pts[1])
	}
	if pts[0].Y != pts[1].Y {
		t.Error("a horizontal rule is not horizontal")
	}
	if pts[0].Y != 50 {
		t.Errorf("line at y = %v, want 50", pts[0].Y)
	}
}

func TestVLineSpansThePlot(t *testing.T) {
	line := geom.VLine(5)
	f := annotFrame(t, geom.Line(src(map[string][]float64{"x": {0, 10}, "y": {0, 10}}),
		geom.X("x"), geom.Y("y")), line)
	pts := draw(t, f, line).Filter("Polyline")[0].Points
	if pts[0].Y != 0 || pts[1].Y != 100 {
		t.Errorf("line runs from %v to %v, want the full height", pts[0], pts[1])
	}
	if pts[0].X != 50 {
		t.Errorf("line at x = %v, want 50", pts[0].X)
	}
}

// A reference line the data does not reach is the interesting case: the axis
// has to grow to show it, or the chart hides the answer.
func TestAnnotationExtendsTheDomain(t *testing.T) {
	x := scale.Linear()
	y := scale.Linear()
	g := geom.Line(src(map[string][]float64{"x": {0, 1}, "y": {0, 1}}), geom.X("x"), geom.Y("y"))
	if err := g.Train(x, y); err != nil {
		t.Fatal(err)
	}
	if err := geom.HLine(9).Train(x, y); err != nil {
		t.Fatal(err)
	}
	if _, hi := y.Domain(); hi < 9 {
		t.Errorf("Y domain tops out at %v, want at least 9", hi)
	}
}

func TestExtendOffLeavesTheDomainAlone(t *testing.T) {
	x := scale.Linear()
	y := scale.Linear()
	g := geom.Line(src(map[string][]float64{"x": {0, 1}, "y": {0, 1}}), geom.X("x"), geom.Y("y"))
	if err := g.Train(x, y); err != nil {
		t.Fatal(err)
	}
	for _, a := range []geom.Geom{
		geom.HLine(9, geom.Extend(false)),
		geom.VLine(9, geom.Extend(false)),
		geom.HBand(8, 9, geom.Extend(false)),
		geom.VBand(8, 9, geom.Extend(false)),
		geom.Segment(8, 8, 9, 9, geom.Extend(false)),
		geom.Region(8, 8, 9, 9, geom.Extend(false)),
		geom.Note(9, 9, "x", geom.Extend(false)),
	} {
		if err := a.Train(x, y); err != nil {
			t.Fatal(err)
		}
	}
	if _, hi := y.Domain(); hi > 1 {
		t.Errorf("Y domain reaches %v, want it left at the data", hi)
	}
}

func TestBandsFillTheirStrip(t *testing.T) {
	band := geom.HBand(2, 8)
	f := annotFrame(t, geom.Line(src(map[string][]float64{"x": {0, 10}, "y": {0, 10}}),
		geom.X("x"), geom.Y("y")), band)
	calls := draw(t, f, band).Filter("FillPath")
	if len(calls) != 1 {
		t.Fatalf("emitted %d fills, want 1", len(calls))
	}
	got := calls[0].Path.Bounds()
	if !sameRect(got, ir.R(0, 20, 100, 80)) {
		t.Errorf("band covers %v, want the strip from y=20 to y=80 across the plot", got)
	}
	if a := calls[0].Fill.Color.A; a == 0 || a == 255 {
		t.Errorf("band alpha = %d, want it faded but visible", a)
	}
}

func TestVBandFillsItsColumn(t *testing.T) {
	band := geom.VBand(8, 2) // reversed on purpose: order should not matter
	f := annotFrame(t, geom.Line(src(map[string][]float64{"x": {0, 10}, "y": {0, 10}}),
		geom.X("x"), geom.Y("y")), band)
	got := draw(t, f, band).Filter("FillPath")[0].Path.Bounds()
	if !sameRect(got, ir.R(20, 0, 80, 100)) {
		t.Errorf("band covers %v, want x=20..80 across the height", got)
	}
}

// A window of zero width has no area. Drawing a hairline would report it as
// having extent, which is the one thing it does not have.
func TestDegenerateBandDrawsNothing(t *testing.T) {
	band := geom.HBand(5, 5)
	f := annotFrame(t, geom.Line(src(map[string][]float64{"x": {0, 10}, "y": {0, 10}}),
		geom.X("x"), geom.Y("y")), band)
	if n := len(draw(t, f, band).Calls); n != 0 {
		t.Errorf("emitted %d calls for a zero-width band", n)
	}
}

func TestSegmentConnectsTwoPoints(t *testing.T) {
	seg := geom.Segment(0, 0, 10, 10)
	f := annotFrame(t, seg)
	pts := draw(t, f, seg).Filter("Polyline")[0].Points
	if !samePoint(pts[0], ir.Point{X: 0, Y: 100}) || !samePoint(pts[1], ir.Point{X: 100, Y: 0}) {
		t.Errorf("segment runs %v -> %v", pts[0], pts[1])
	}
}

func TestRegionFillsItsRectangle(t *testing.T) {
	reg := geom.Region(2, 2, 8, 8)
	f := annotFrame(t, geom.Line(src(map[string][]float64{"x": {0, 10}, "y": {0, 10}}),
		geom.X("x"), geom.Y("y")), reg)
	r := draw(t, f, reg)
	if n := r.Count("FillPath"); n != 1 {
		t.Fatalf("emitted %d fills, want 1", n)
	}
	if n := r.Count("StrokePath"); n != 0 {
		t.Errorf("emitted %d outlines for a region with no explicit colour, want 0", n)
	}
	if got := r.Filter("FillPath")[0].Path.Bounds(); !sameRect(got, ir.R(20, 20, 80, 80)) {
		t.Errorf("region covers %v", got)
	}
}

// Naming a colour means the region is the point rather than the backdrop, so
// it gets an outline as well.
func TestNamedRegionIsOutlined(t *testing.T) {
	reg := geom.Region(2, 2, 8, 8, geom.Color(palette.Red))
	f := annotFrame(t, geom.Line(src(map[string][]float64{"x": {0, 10}, "y": {0, 10}}),
		geom.X("x"), geom.Y("y")), reg)
	r := draw(t, f, reg)
	if r.Count("StrokePath") != 1 {
		t.Error("a region with an explicit colour should be outlined")
	}
	if got := r.Filter("StrokePath")[0].Stroke.Color; got != palette.Red {
		t.Errorf("outline colour = %v, want the named colour", got)
	}
}

func TestNotePlacesText(t *testing.T) {
	note := geom.Note(5, 5, "here", geom.Align(ir.AlignCenter, ir.AlignBottom), geom.FontSize(9))
	f := annotFrame(t, geom.Line(src(map[string][]float64{"x": {0, 10}, "y": {0, 10}}),
		geom.X("x"), geom.Y("y")), note)
	calls := draw(t, f, note).Filter("Text")
	if len(calls) != 1 {
		t.Fatalf("emitted %d text runs, want 1", len(calls))
	}
	run := calls[0].Text
	if run.Text != "here" {
		t.Errorf("text = %q", run.Text)
	}
	if !samePoint(run.At, ir.Point{X: 50, Y: 50}) {
		t.Errorf("placed at %v, want (50,50)", run.At)
	}
	if run.H != ir.AlignCenter || run.V != ir.AlignBottom {
		t.Errorf("alignment = %v/%v", run.H, run.V)
	}
	if run.Font.Size != 9 {
		t.Errorf("font size = %v, want 9", run.Font.Size)
	}
}

func TestNoteDefaultsToTheThemeLabelSize(t *testing.T) {
	note := geom.Note(5, 5, "here")
	f := annotFrame(t, note)
	if got := draw(t, f, note).Filter("Text")[0].Text.Font.Size; got != theme.Light.LabelSize {
		t.Errorf("font size = %v, want the theme's label size", got)
	}
}

func TestEmptyNoteDrawsNothing(t *testing.T) {
	note := geom.Note(5, 5, "")
	f := annotFrame(t, note)
	if n := len(draw(t, f, note).Calls); n != 0 {
		t.Errorf("emitted %d calls for an empty note", n)
	}
}

// An annotation is not a series: it takes the theme's annotation ink rather
// than the next palette colour, or a threshold would look like data.
func TestAnnotationsDoNotTakeAPaletteColour(t *testing.T) {
	line := geom.HLine(5)
	f := annotFrame(t, line)
	f.Index = 1
	got := draw(t, f, line).Filter("Polyline")[0].Stroke
	if got.Color != theme.Light.AnnotationColor {
		t.Errorf("colour = %v, want the theme's annotation ink", got.Color)
	}
	if len(got.Dash) == 0 {
		t.Error("the default annotation is not dashed")
	}
}

func TestDashOverridesTheThemeDefault(t *testing.T) {
	solid := geom.HLine(5, geom.Dash())
	dashed := geom.HLine(5, geom.Dash(2, 2))
	f := annotFrame(t, solid, dashed)
	if got := draw(t, f, solid).Filter("Polyline")[0].Stroke.Dash; len(got) != 0 {
		t.Errorf("Dash() with no pattern left %v", got)
	}
	if got := draw(t, f, dashed).Filter("Polyline")[0].Stroke.Dash; len(got) != 2 {
		t.Errorf("Dash(2,2) gave %v", got)
	}
}

func TestAnnotationsAppearInTheLegendOnlyWhenLabelled(t *testing.T) {
	f := annotFrame(t, geom.HLine(5))
	for _, g := range []geom.Geom{
		geom.HLine(5), geom.VLine(5), geom.HBand(1, 2), geom.VBand(1, 2),
		geom.Segment(0, 0, 1, 1), geom.Region(0, 0, 1, 1), geom.Note(1, 1, "x"),
	} {
		if _, ok := g.Legend(f); ok {
			t.Errorf("%T contributed a legend entry with no label", g)
		}
	}
	for _, g := range []geom.Geom{
		geom.HLine(5, geom.Label("limit")),
		geom.HBand(1, 2, geom.Label("window")),
		geom.Segment(0, 0, 1, 1, geom.Label("trend")),
		geom.Region(0, 0, 1, 1, geom.Label("area")),
	} {
		e, ok := g.Legend(f)
		if !ok {
			t.Errorf("%T contributed no entry despite a label", g)
			continue
		}
		if e.Label == "" {
			t.Errorf("%T contributed an empty label", g)
		}
	}
	// A note names a place, not a series; it never joins the legend.
	if _, ok := geom.Note(1, 1, "x", geom.Label("n")).Legend(f); ok {
		t.Error("a labelled note joined the legend")
	}
}

// A log axis has no position for zero, so an annotation there is skipped by
// the same rule that skips a data point there.
func TestAnnotationOffAnUndefinedScalePositionIsSkipped(t *testing.T) {
	x := scale.Linear()
	y := scale.Log()
	g := geom.Line(src(map[string][]float64{"x": {1, 10}, "y": {1, 100}}), geom.X("x"), geom.Y("y"))
	if err := g.Train(x, y); err != nil {
		t.Fatal(err)
	}
	x.SetRange(0, 100)
	y.SetRange(100, 0)
	f := geom.Frame{Area: ir.R(0, 0, 100, 100), X: x, Y: y, Theme: theme.Light}

	for _, a := range []geom.Geom{
		geom.HLine(0, geom.Extend(false)),
		geom.HBand(-1, 0, geom.Extend(false)),
		geom.Segment(1, 0, 2, 1, geom.Extend(false)),
		geom.Region(1, 0, 2, 1, geom.Extend(false)),
		geom.Note(1, 0, "x", geom.Extend(false)),
	} {
		if n := len(draw(t, f, a).Calls); n != 0 {
			t.Errorf("%T emitted %d calls for a value the scale cannot place", a, n)
		}
	}
}

func TestInvisibleAnnotationsDrawNothing(t *testing.T) {
	f := annotFrame(t, geom.HLine(5))
	for _, g := range []geom.Geom{
		geom.HLine(5, geom.Color(ir.Transparent)),
		geom.VLine(5, geom.Color(ir.Transparent)),
		geom.HBand(1, 2, geom.Opacity(0), geom.Fill(ir.Transparent)),
		geom.Segment(0, 0, 1, 1, geom.Color(ir.Transparent)),
		geom.Note(1, 1, "x", geom.Color(ir.Transparent)),
	} {
		if n := len(draw(t, f, g).Calls); n != 0 {
			t.Errorf("%T emitted %d calls while invisible", g, n)
		}
	}
}
