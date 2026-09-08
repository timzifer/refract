package refract_test

// Guides are furniture, and one of them answers to a pointer.
//
// ADR 0015 said a pointer landing on a guide has not landed on anything a
// reader would ask about, and until render gained EndData the index did not
// even know when the data pass ended — so a legend's swatches were being
// recorded as *marks* of whichever layer was drawn last, which is the thing
// that rule was against.
//
// A legend is now indexed on purpose, and the rule still holds, because what
// it is indexed as is different: kind Guide, in panel -1, carrying the layer
// it stands for rather than a value read off an axis. ADR 0046's revisit
// clause is what these two tests are the boundary of.

import (
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/interact"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

func legendChart(t *testing.T, legend bool) *refract.Live {
	t.Helper()
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1, 2}, "y": {1, 2, 3}})
	p := refract.New(refract.Size(600, 300), refract.Legend(legend))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y"), geom.Label("a"), geom.Color(palette.Blue)))
	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	return live
}

// A legend contributes rows a pointer can find, and nothing that reads as data.
func TestAGuideIsIndexedAsAGuideAndNotAsAMark(t *testing.T) {
	bare := legendChart(t, false)
	withLegend := legendChart(t, true)

	if got, want := withLegend.Index().MarkCount(), bare.Index().MarkCount()+1; got != want {
		t.Errorf("marks = %d, want %d — one row for the one series", got, want)
	}

	// The one it added is a Guide. Everything else is what it always was.
	ix := withLegend.Index()
	area := ix.Panels()[0].Area
	var guides int
	for x := float32(2); x < 600; x += 2 {
		for y := float32(2); y < 300; y += 2 {
			pt := irPt(x, y)
			hit, ok := ix.At(pt, 0)
			if !ok {
				continue
			}
			if hit.Kind == interact.LegendRow {
				guides++
				if area.Contains(pt) {
					t.Fatalf("a guide was hit inside the panel at (%v,%v)", x, y)
				}
				if hit.Series != "a" {
					t.Errorf("the guide reports series %q, want %q", hit.Series, "a")
				}
				if hit.Layer != 0 {
					t.Errorf("the guide reports layer %d, want 0", hit.Layer)
				}
				if hit.Panel != -1 {
					t.Errorf("the guide reports panel %d, want -1 — a legend is the chart's", hit.Panel)
				}
			}
		}
	}
	if guides == 0 {
		t.Error("the legend is not reachable by a pointer")
	}
}

// Index.At is reachable without going through Live, which gates on the panel —
// so the index has to be right rather than merely unreachable.
func TestNothingOutsideAPanelIsHitAsData(t *testing.T) {
	live := legendChart(t, true)
	ix := live.Index()
	// Grown by the hit tolerance: a mark just inside the panel edge is meant
	// to be reachable from just outside it — twelve pixels is about a
	// fingertip — so the band around the panel is not what this is about.
	const tol = interact.DefaultTolerance
	area := ix.Panels()[0].Area
	near := ir.Rect{
		Min: irPt(area.Min.X-tol, area.Min.Y-tol),
		Max: irPt(area.Max.X+tol, area.Max.Y+tol),
	}
	for x := float32(2); x < 600; x += 3 {
		for y := float32(2); y < 300; y += 3 {
			pt := irPt(x, y)
			if near.Contains(pt) {
				continue
			}
			hit, ok := ix.At(pt, 0)
			if !ok {
				continue
			}
			// A legend row is reachable and is meant to be. Anything else out
			// here would be furniture indexed as data.
			if hit.Kind != interact.LegendRow {
				t.Fatalf("a point well outside every panel, at (%v,%v), hit layer %d as %v — furniture is being indexed as data",
					x, y, hit.Layer, hit.Kind)
			}
		}
	}
}

func irPt(x, y float32) ir.Point { return ir.Point{X: x, Y: y} }
