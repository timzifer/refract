package refract_test

import (
	"errors"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

func points() refract.Source {
	return refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3, 4},
		"y": {0, 4, 1, 9, 2},
	})
}

func livePlot(t *testing.T) (*refract.Plot, *refract.Live, *irtest.Recorder) {
	t.Helper()
	p := refract.New(refract.Size(400, 300))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Scatter(points(), geom.X("x"), geom.Y("y"), geom.Label("s")))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	t.Cleanup(func() { live.Close() })
	if err := live.Draw(); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	return p, live, rec
}

func TestLiveNeedsATargetAndSomethingToDraw(t *testing.T) {
	if _, err := refract.New().Live(nil); err == nil {
		t.Error("a nil target was accepted")
	}
	if _, err := refract.New().Live(irtest.New().Target()); !errors.Is(err, refract.ErrNoLayers) {
		t.Errorf("an empty plot gave %v, want ErrNoLayers — the same answer Render gives", err)
	}
}

func TestDrawAfterCloseIsAnError(t *testing.T) {
	_, live, _ := livePlot(t)
	if err := live.Close(); err != nil {
		t.Fatal(err)
	}
	if err := live.Draw(); err == nil {
		t.Error("Draw on a closed Live succeeded")
	}
	if err := live.Close(); err != nil {
		t.Errorf("closing twice: %v", err)
	}
}

func TestClickReportsWhatIsUnderIt(t *testing.T) {
	p, live, _ := livePlot(t)
	var seen []refract.Event
	p.On(refract.Click, func(ev refract.Event) { seen = append(seen, ev) })

	panel := live.Index().Panels()[0]
	ev := live.Click(float64(panel.X.Map(1)), float64(panel.Y.Map(4)))
	if !ev.Found {
		t.Error("the click found nothing at a marker")
	}
	if ev.Panel != 0 {
		t.Errorf("panel = %d, want 0", ev.Panel)
	}

	// A click in the margins is still a click, and reports no panel and no hit.
	ev = live.Click(2, 2)
	if ev.Panel != -1 || ev.Found {
		t.Errorf("a click outside every panel reports panel %d found=%v, want -1 and false", ev.Panel, ev.Found)
	}
	if len(seen) != 2 {
		t.Errorf("the handler fired %d times, want one per click", len(seen))
	}
}

func TestLeaveFiresOnce(t *testing.T) {
	p, live, _ := livePlot(t)
	var leaves int
	p.On(refract.Leave, func(refract.Event) { leaves++ })

	panel := live.Index().Panels()[0]
	live.Move(float64(panel.X.Map(1)), float64(panel.Y.Map(4)))
	if got := live.Leave(); got.Kind != refract.Leave {
		t.Errorf("Leave fired %v", got.Kind)
	}
	if leaves != 1 {
		t.Errorf("the handler fired %d times", leaves)
	}
}

func TestZoomToARectangle(t *testing.T) {
	p, live, _ := livePlot(t)
	var zooms int
	p.On(refract.Zoom, func(ev refract.Event) {
		zooms++
		if ev.Rect.Empty() {
			t.Error("a rubber-band zoom reported no rectangle")
		}
	})

	panel := live.Index().Panels()[0]
	before0, before1 := panel.X.Domain()
	sel := ir.Rect{
		Min: ir.Point{X: panel.X.Map(1), Y: panel.Y.Map(6)},
		Max: ir.Point{X: panel.X.Map(3), Y: panel.Y.Map(2)},
	}
	if err := live.ZoomTo(sel); err != nil {
		t.Fatal(err)
	}
	lo, hi := panel.X.Domain()
	if lo < before0 || hi > before1 || hi-lo >= before1-before0 {
		t.Errorf("the domain went from %v..%v to %v..%v, want the selection", before0, before1, lo, hi)
	}
	if got := panel.Y; func() bool { l, h := got.Domain(); return h-l >= 9 }() {
		lo, hi := got.Domain()
		t.Errorf("the y domain is %v..%v, want the selection", lo, hi)
	}
	if zooms != 1 {
		t.Errorf("the handler fired %d times", zooms)
	}

	// An empty selection is a click, not a zoom, and changes nothing.
	if err := live.ZoomTo(ir.Rect{}); err != nil {
		t.Fatal(err)
	}
	if zooms != 1 {
		t.Error("an empty selection fired a zoom")
	}
}

func TestWheelRejectsANonPositiveFactor(t *testing.T) {
	_, live, _ := livePlot(t)
	if err := live.Wheel(100, 100, 0); err == nil {
		t.Error("a zoom factor of zero was accepted")
	}
}

func TestRebuildPicksUpANewLayer(t *testing.T) {
	p, live, _ := livePlot(t)
	before := live.Index().MarkCount()

	p.Add(geom.Scatter(points(), geom.X("x"), geom.Y("y"), geom.Shape(ir.MarkerSquare)))
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	if live.Index().MarkCount() != before {
		t.Error("a layer added after Live was drawn without a Rebuild")
	}

	if err := live.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	if live.Index().MarkCount() <= before {
		t.Errorf("after Rebuild the index holds %d marks, want more than %d", live.Index().MarkCount(), before)
	}
}

func TestAnUnregisteredHandlerKindIsIgnored(t *testing.T) {
	p := refract.New(refract.Size(200, 200))
	p.Add(geom.Scatter(points(), geom.X("x"), geom.Y("y")))
	p.On(refract.Hover, nil) // a nil handler is dropped rather than panicking

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	live.Move(100, 100) // no handler for Hover: nothing happens, nothing panics
}

func TestSpecFailsOnALayerItCannotWriteDown(t *testing.T) {
	p := refract.New()
	p.Add(opaqueLayer{})
	if _, err := p.Spec(); err == nil {
		t.Fatal("a layer that cannot describe itself was written down")
	}
	if _, err := p.MarshalJSON(); err == nil {
		t.Error("MarshalJSON succeeded on the same plot")
	}
}

func TestParseJSONRejectsRubbish(t *testing.T) {
	if _, err := refract.ParseJSON([]byte("not json")); err == nil {
		t.Error("invalid JSON was accepted")
	}
	if _, err := refract.ParseJSON([]byte(`{"layer":[{"mark":{"type":"violin"}}]}`)); err == nil {
		t.Error("an unknown mark was accepted")
	}
	var p refract.Plot
	if err := p.UnmarshalJSON([]byte("{")); err == nil {
		t.Error("UnmarshalJSON accepted invalid JSON")
	}
}

type opaqueLayer struct{}

func (opaqueLayer) Train(scale.Scale, scale.Scale) error       { return nil }
func (opaqueLayer) Build(ir.Backend, geom.Frame) error         { return nil }
func (opaqueLayer) Legend(geom.Frame) (geom.LegendEntry, bool) { return geom.LegendEntry{}, false }
