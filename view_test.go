package refract_test

// The reader's view survives a change to the plot. It is what makes "hover
// chart 1, add a highlight layer to chart 2" a thing a reader can live with
// rather than a thing that throws away wherever they had zoomed to.

import (
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

func viewTable() *data.Table {
	return refract.NewTable().
		Float64("x", []float64{0, 1, 2, 3, 4, 5}).
		Float64("y", []float64{10, 20, 30, 40, 50, 60}).
		String("g", []string{"a", "b", "a", "b", "a", "b"})
}

// half is the middle of a panel, which is a zoom that actually changes the
// domain. Zooming to the panel's whole area selects what is already shown and
// moves nothing, which would make every assertion below true for free.
func half(r ir.Rect) ir.Rect {
	dx := (r.Max.X - r.Min.X) / 4
	dy := (r.Max.Y - r.Min.Y) / 4
	return ir.Rect{
		Min: ir.Point{X: r.Min.X + dx, Y: r.Min.Y + dy},
		Max: ir.Point{X: r.Max.X - dx, Y: r.Max.Y - dy},
	}
}

func domainsOf(t *testing.T, l *refract.Live) [][2]float64 {
	t.Helper()
	var out [][2]float64
	for _, p := range l.Index().Panels() {
		lo, hi := p.X.Domain()
		out = append(out, [2]float64{lo, hi})
	}
	return out
}

func TestRebuildKeepsTheView(t *testing.T) {
	p := refract.New(refract.Size(600, 300))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Line(viewTable(), geom.X("x"), geom.Y("y")))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	// The reader zooms in.
	before := domainsOf(t, live)
	panel := live.Index().Panels()[0]
	if err := live.ZoomTo(half(panel.Area)); err != nil {
		t.Fatal(err)
	}
	zoomed := domainsOf(t, live)
	if zoomed[0] == before[0] {
		t.Fatal("the zoom changed nothing, so this test proves nothing")
	}

	// The host reacts by adding a layer, the way a linked highlight does.
	p.Add(geom.Scatter(viewTable(), geom.X("x"), geom.Y("y")))
	if err := live.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	if got := domainsOf(t, live); got[0] != zoomed[0] {
		t.Errorf("the view after a rebuild is %v, want %v", got[0], zoomed[0])
	}
}

// The case that was actually broken: a facet with free axes builds a fresh
// clone of the scale per panel, so the new panels have never heard of the
// zoom that was applied to the old ones.
func TestRebuildKeepsTheViewOnFreeFacetAxes(t *testing.T) {
	p := refract.New(refract.Size(600, 300))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Line(viewTable(), geom.X("x"), geom.Y("y")))
	p.Facet(facet.Wrap("g", facet.Free()))

	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	if n := len(live.Index().Panels()); n != 2 {
		t.Fatalf("panels = %d, want 2", n)
	}

	before := domainsOf(t, live)
	panel := live.Index().Panels()[0]
	if err := live.ZoomTo(half(panel.Area)); err != nil {
		t.Fatal(err)
	}
	zoomed := domainsOf(t, live)
	if zoomed[0] == before[0] {
		t.Fatal("the zoom changed nothing, so this test proves nothing")
	}

	p.Add(geom.Scatter(viewTable(), geom.X("x"), geom.Y("y")))
	if err := live.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	got := domainsOf(t, live)
	if len(got) != len(zoomed) {
		t.Fatalf("panels = %d after the rebuild, want %d", len(got), len(zoomed))
	}
	for i := range got {
		if got[i] != zoomed[i] {
			t.Errorf("panel %d: view is %v after the rebuild, want %v", i, got[i], zoomed[i])
		}
	}
}

// View and SetView bracket anything, not only a rebuild.
func TestViewRoundTrips(t *testing.T) {
	p := refract.New(refract.Size(600, 300))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Line(viewTable(), geom.X("x"), geom.Y("y")))

	rec := irtest.New()
	live, _ := p.Live(rec.Target())
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	v := live.View()
	if v.Empty() || v.Panels() != 1 {
		t.Fatalf("view describes %d panels, want 1", v.Panels())
	}
	before := domainsOf(t, live)

	panel := live.Index().Panels()[0]
	if err := live.ZoomTo(half(panel.Area)); err != nil {
		t.Fatal(err)
	}
	if got := domainsOf(t, live); got[0] == before[0] {
		t.Fatal("the zoom changed nothing, so this test proves nothing")
	}

	if err := live.SetView(v); err != nil {
		t.Fatal(err)
	}
	if got := domainsOf(t, live); got[0] != before[0] {
		t.Errorf("the restored view is %v, want %v", got[0], before[0])
	}
}

// A view from a chart of another shape describes nothing here, and is ignored
// rather than applied to whichever panels happen to line up.
func TestAViewOfTheWrongShapeIsIgnored(t *testing.T) {
	one := refract.New(refract.Size(600, 300))
	one.X(scale.Linear())
	one.Y(scale.Linear())
	one.Add(geom.Line(viewTable(), geom.X("x"), geom.Y("y")))
	l1, _ := one.Live(irtest.New().Target())
	defer l1.Close()
	if err := l1.Draw(); err != nil {
		t.Fatal(err)
	}
	if err := l1.ZoomTo(half(l1.Index().Panels()[0].Area)); err != nil {
		t.Fatal(err)
	}
	narrow := l1.View()

	two := refract.New(refract.Size(600, 300))
	two.X(scale.Linear())
	two.Y(scale.Linear())
	two.Add(geom.Line(viewTable(), geom.X("x"), geom.Y("y")))
	two.Facet(facet.Wrap("g"))
	l2, _ := two.Live(irtest.New().Target())
	defer l2.Close()
	if err := l2.Draw(); err != nil {
		t.Fatal(err)
	}
	before := domainsOf(t, l2)

	if err := l2.SetView(narrow); err != nil {
		t.Fatal(err)
	}
	got := domainsOf(t, l2)
	for i := range got {
		if got[i] != before[i] {
			t.Errorf("panel %d moved to %v under a view of the wrong shape", i, got[i])
		}
	}
}

// A chart that has not been drawn has no panels, so it is not looking
// anywhere yet.
func TestViewOfAnUndrawnChartIsEmpty(t *testing.T) {
	p := refract.New(refract.Size(600, 300))
	p.Add(geom.Line(viewTable(), geom.X("x"), geom.Y("y")))
	live, _ := p.Live(irtest.New().Target())
	defer live.Close()
	if !live.View().Empty() {
		t.Error("an undrawn chart reports a view")
	}
}
