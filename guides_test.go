package refract_test

// Guides are furniture. ADR 0015 says so — "the grid, the axes, the titles and
// the guides are furniture; a pointer landing on a grid line has not landed on
// anything a reader would ask about" — and until render gained EndData the
// index did not know when the data pass ended, so a legend's swatches were
// recorded as marks of whichever layer was drawn last.

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

func TestAGuideIsNotIndexed(t *testing.T) {
	without := legendChart(t, false).Index().MarkCount()
	with := legendChart(t, true).Index().MarkCount()
	if with != without {
		t.Errorf("a legend added %d marks to the index; a swatch is furniture, not a mark", with-without)
	}
}

// Index.At is reachable without going through Live, which gates on the panel —
// so the index has to be right rather than merely unreachable.
func TestNothingOutsideAPanelIsHit(t *testing.T) {
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
			if hit, ok := ix.At(pt, 0); ok {
				t.Fatalf("a point well outside every panel, at (%v,%v), hit layer %d (%v) — furniture is being indexed",
					x, y, hit.Layer, hit.Kind)
			}
		}
	}
}

func irPt(x, y float32) ir.Point { return ir.Point{X: x, Y: y} }
