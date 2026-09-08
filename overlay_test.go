package refract_test

// The overlay layer: what it draws, what it costs, and the two properties that
// make it usable rather than merely present — it is not hit-testable, and a
// chart without one draws exactly what it drew before there were any.

import (
	"strings"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

func overlayLive(t *testing.T, o refract.Overlay) (*refract.Live, *irtest.Recorder) {
	t.Helper()
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {10, 20, 15, 25},
	})
	p := refract.New(refract.Size(600, 300))
	p.X(scale.Linear(scale.Domain(0, 3)))
	p.Y(scale.Linear(scale.Domain(0, 30)))
	p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.Color(palette.Blue)))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	if o != nil {
		live.Overlay(o)
	}
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	return live, rec
}

func middleOf(t *testing.T, l *refract.Live) ir.Point {
	t.Helper()
	a := l.Index().Panels()[0].Area
	return ir.Point{X: (a.Min.X + a.Max.X) / 2, Y: (a.Min.Y + a.Max.Y) / 2}
}

// A chart with no overlay emits byte-identical calls to one from before there
// were overlays at all. It is the same test Index.Watch has, for the same
// reason: one set of golden files has to cover both.
func TestNoOverlayChangesNothing(t *testing.T) {
	_, plain := overlayLive(t, nil)
	_, alsoPlain := overlayLive(t, nil)
	if strings.Join(plain.Trace(), "\n") != strings.Join(alsoPlain.Trace(), "\n") {
		t.Fatal("two identical charts drew differently")
	}
	// And installing one that draws nothing is also invisible.
	_, empty := overlayLive(t, &refract.Crosshair{})
	if strings.Join(plain.Trace(), "\n") != strings.Join(empty.Trace(), "\n") {
		t.Error("an overlay that draws nothing changed the frame")
	}
}

// The property that makes an overlay usable: a pointer cannot hit it. A
// tooltip a pointer can hit is a tooltip that flickers, because hovering it
// moves the pointer off the thing it was about.
func TestAnOverlayIsNotHitTestable(t *testing.T) {
	bare, _ := overlayLive(t, nil)
	want := bare.Index().MarkCount()

	at := middleOf(t, bare)
	cross := &refract.Crosshair{At: at, Show: true, Panel: -1}
	live, _ := overlayLive(t, cross)
	if got := live.Index().MarkCount(); got != want {
		t.Errorf("an overlay added %d marks to the index, want none", got-want)
	}

	// And a pointer over the crosshair's own lines finds only what was there
	// before: the marks the layer drew, or nothing.
	if hit, ok := live.Index().At(ir.Point{X: at.X, Y: at.Y - 40}, 0); ok && hit.Kind != 0 {
		t.Errorf("a point on the crosshair hit %v", hit.Kind)
	}
}

func TestACrosshairDrawsTwoRules(t *testing.T) {
	bare, plain := overlayLive(t, nil)
	at := middleOf(t, bare)

	_, rec := overlayLive(t, &refract.Crosshair{At: at, Show: true, Panel: -1})
	if got, want := rec.Count("Polyline"), plain.Count("Polyline")+2; got != want {
		t.Errorf("polylines = %d, want %d — a crosshair is two rules", got, want)
	}

	_, one := overlayLive(t, &refract.Crosshair{At: at, Show: true, Panel: -1, NoHorizontal: true})
	if got, want := one.Count("Polyline"), plain.Count("Polyline")+1; got != want {
		t.Errorf("polylines = %d, want %d with the horizontal rule off", got, want)
	}
}

// A crosshair is clipped to its panel: a rule running into the margin would
// cross the axis it is being read against.
func TestACrosshairIsClippedToItsPanel(t *testing.T) {
	bare, plain := overlayLive(t, nil)
	at := middleOf(t, bare)
	_, rec := overlayLive(t, &refract.Crosshair{At: at, Show: true, Panel: -1})
	if got, want := rec.Count("Push"), plain.Count("Push")+1; got != want {
		t.Errorf("pushes = %d, want %d — the crosshair pushed no clip", got, want)
	}
	if rec.Count("Push") != rec.Count("Pop") {
		t.Errorf("%d pushes and %d pops", rec.Count("Push"), rec.Count("Pop"))
	}
}

// A point in no panel is not drawn, rather than drawn in whichever panel was
// nearest.
func TestACrosshairOutsideEveryPanelDrawsNothing(t *testing.T) {
	_, plain := overlayLive(t, nil)
	_, rec := overlayLive(t, &refract.Crosshair{
		At: ir.Point{X: 2, Y: 2}, Show: true, Panel: -1,
	})
	if got, want := rec.Count("Polyline"), plain.Count("Polyline"); got != want {
		t.Errorf("polylines = %d, want %d — a crosshair drew outside every panel", got, want)
	}
}

func TestHighlightRingsItsPoints(t *testing.T) {
	bare, plain := overlayLive(t, nil)
	at := middleOf(t, bare)
	_, rec := overlayLive(t, &refract.Highlight{
		At: []ir.Point{at, {X: at.X + 20, Y: at.Y}}, Panel: -1,
	})
	// Both rings are subpaths of one stroked path, which is what keeps a
	// highlight over a hundred marks one call rather than a hundred.
	if got, want := rec.Count("StrokePath"), plain.Count("StrokePath")+1; got != want {
		t.Errorf("stroked paths = %d, want %d", got, want)
	}
}

func TestBrushFillsAndOutlines(t *testing.T) {
	bare, plain := overlayLive(t, nil)
	a := bare.Index().Panels()[0].Area
	_, rec := overlayLive(t, &refract.Brush{Rect: ir.Rect{
		Min: ir.Point{X: a.Min.X + 10, Y: a.Min.Y + 10},
		Max: ir.Point{X: a.Min.X + 60, Y: a.Min.Y + 60},
	}})
	if got, want := rec.Count("FillPath"), plain.Count("FillPath")+1; got != want {
		t.Errorf("fills = %d, want %d", got, want)
	}
	if got, want := rec.Count("StrokePath"), plain.Count("StrokePath")+1; got != want {
		t.Errorf("strokes = %d, want %d", got, want)
	}
	// An empty rectangle is not a selection.
	_, none := overlayLive(t, &refract.Brush{})
	if got, want := none.Count("FillPath"), plain.Count("FillPath"); got != want {
		t.Errorf("an empty brush drew %d fills, want %d", got, want)
	}
}

func TestTooltipDrawsABoxAndItsLines(t *testing.T) {
	bare, plain := overlayLive(t, nil)
	at := middleOf(t, bare)
	_, rec := overlayLive(t, &refract.Tooltip{
		At: at, Lines: []string{"series: a", "x: 1.5", "y: 20"},
	})
	if got, want := rec.Count("Text"), plain.Count("Text")+3; got != want {
		t.Errorf("texts = %d, want %d — one per line", got, want)
	}
	if got, want := rec.Count("FillPath"), plain.Count("FillPath")+1; got != want {
		t.Errorf("fills = %d, want %d — the box", got, want)
	}
	for _, line := range []string{"series: a", "x: 1.5", "y: 20"} {
		if !slicesContain(rec.Texts(), line) {
			t.Errorf("the tooltip did not draw %q", line)
		}
	}
	// No lines is no tooltip.
	_, none := overlayLive(t, &refract.Tooltip{At: at})
	if got, want := none.Count("Text"), plain.Count("Text"); got != want {
		t.Errorf("an empty tooltip drew %d texts, want %d", got, want)
	}
}

// A tooltip near an edge flips rather than being clamped, because a box pushed
// back inside the canvas would sit over the point it is about.
func TestATooltipFlipsAtTheEdge(t *testing.T) {
	bare, _ := overlayLive(t, nil)
	a := bare.Index().Panels()[0].Area

	near := ir.Point{X: a.Min.X + 20, Y: a.Min.Y + 20}
	_, left := overlayLive(t, &refract.Tooltip{At: near, Lines: []string{"hello"}})

	far := ir.Point{X: a.Max.X - 2, Y: a.Max.Y - 2}
	_, right := overlayLive(t, &refract.Tooltip{At: far, Lines: []string{"hello"}})

	lx := textX(t, left, "hello")
	rx := textX(t, right, "hello")
	if rx >= far.X {
		t.Errorf("a tooltip at the right edge drew its text at x=%v, which is not left of its anchor %v", rx, far.X)
	}
	if lx <= near.X {
		t.Errorf("a tooltip away from the edge drew its text at x=%v, want right of its anchor %v", lx, near.X)
	}
}

// A frame that only moves an overlay is a damage rectangle. One that makes an
// overlay appear or disappear changes the call count, and is a full repaint —
// which is a real property worth knowing rather than a bug.
func TestMovingAnOverlayIsAPartialRepaint(t *testing.T) {
	cross := &refract.Crosshair{Show: true, Panel: -1}
	live, rec := overlayLive(t, cross)
	cross.At = middleOf(t, live)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	rec.Whole = nil
	for i := range 5 {
		cross.At.X += float32(3 * (i + 1))
		if err := live.Draw(); err != nil {
			t.Fatal(err)
		}
	}
	if len(rec.Whole) == 0 {
		t.Fatal("no frame reported what it repainted")
	}
	for i, whole := range rec.Whole {
		if whole {
			t.Errorf("frame %d repainted the whole canvas; a crosshair that only moved should be a damage rectangle", i)
		}
	}
}

// Several overlays, in drawing order, so a chart can have a crosshair and a
// tooltip at once.
func TestOverlaysDrawInOrder(t *testing.T) {
	bare, plain := overlayLive(t, nil)
	at := middleOf(t, bare)
	_, rec := overlayLive(t, refract.Overlays{
		&refract.Crosshair{At: at, Show: true, Panel: -1},
		nil, // skipped, so a caller may keep a fixed-length list
		&refract.Tooltip{At: at, Lines: []string{"one"}},
	})
	if got, want := rec.Count("Polyline"), plain.Count("Polyline")+2; got != want {
		t.Errorf("polylines = %d, want %d", got, want)
	}
	if got, want := rec.Count("Text"), plain.Count("Text")+1; got != want {
		t.Errorf("texts = %d, want %d", got, want)
	}
}

// Input.Move repaints when there is an overlay, which is what makes a
// crosshair follow the pointer on a real surface — Live.Move answers a
// question and deliberately does not draw.
func TestInputRedrawsForAnOverlay(t *testing.T) {
	cross := &refract.Crosshair{Show: true, Panel: -1}
	live, rec := overlayLive(t, cross)
	at := middleOf(t, live)

	in := live.Input()
	rec.Reset()
	cross.At = at
	if err := in.Move(float64(at.X), float64(at.Y)); err != nil {
		t.Fatal(err)
	}
	if len(rec.Trace()) == 0 {
		t.Error("a hover over a chart with an overlay painted nothing")
	}
}

func slicesContain(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// textX is where a text run carrying want was drawn.
func textX(t *testing.T, rec *irtest.Recorder, want string) float32 {
	t.Helper()
	for _, c := range rec.Filter("Text") {
		if c.Text.Text == want {
			return c.Text.At.X
		}
	}
	t.Fatalf("no text reading %q was drawn", want)
	return 0
}

// An overlay installed on the surface survives a rebuild, because a rebuild is
// something the caller asked for about the chart rather than about the pointer.
func TestAnOverlaySurvivesARebuild(t *testing.T) {
	cross := &refract.Crosshair{Show: true, Panel: -1}
	live, _ := overlayLive(t, cross)
	if err := live.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if live.CurrentOverlay() != refract.Overlay(cross) {
		t.Error("the rebuild dropped the surface's overlay")
	}
}

// The plot carries one too, so that Plot.Render and Live.Draw agree about what
// a chart is.
func TestAPlotCanCarryAnOverlay(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {1, 2}})
	p := refract.New(refract.Size(400, 300))
	p.X(scale.Linear(scale.Domain(0, 1)))
	p.Y(scale.Linear(scale.Domain(0, 2)))
	p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y")))

	var buf strings.Builder
	if err := p.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	plain := buf.Len()

	p.Overlay(&refract.Brush{Rect: ir.Rect{
		Min: ir.Point{X: 60, Y: 60}, Max: ir.Point{X: 160, Y: 160},
	}})
	buf.Reset()
	if err := p.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	if buf.Len() <= plain {
		t.Error("a plot with an overlay rendered no more than one without")
	}
}

// The picture. An overlay is drawn last and clipped by nothing, so what to
// look at in the diff is that the crosshair stops at the panel edge, the ring
// sits on a mark, the brush is translucent over the data, and the tooltip's box
// is the width of its text rather than of a guess at it.
func TestOverlayGolden(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3, 4},
		"y": {10, 24, 15, 28, 19},
	})
	p := refract.New(
		refract.Size(640, 320),
		refract.Title("Overlays"),
		refract.XTitle("x"),
		refract.YTitle("y"),
	)
	p.X(scale.Linear(scale.Domain(0, 4)))
	p.Y(scale.Linear(scale.Domain(0, 30)))
	p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.Color(palette.Blue), geom.Size(7)))

	// Rendered once to find out where the marks landed, so the overlay is
	// placed on the data rather than at a guess.
	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	panel := live.Index().Panels()[0]
	at := ir.Point{X: panel.X.Map(2), Y: panel.Y.Map(15)}

	p.Overlay(refract.Overlays{
		&refract.Brush{Rect: ir.Rect{
			Min: ir.Point{X: panel.X.Map(0.6), Y: panel.Y.Map(26)},
			Max: ir.Point{X: panel.X.Map(1.4), Y: panel.Y.Map(8)},
		}},
		&refract.Crosshair{At: at, Show: true, Panel: -1},
		&refract.Highlight{At: []ir.Point{at}, Panel: -1},
		&refract.Tooltip{At: at, Lines: []string{"x: 2", "y: 15"}},
	})
	golden(t, "overlays", p)
}
