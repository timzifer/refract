package render_test

import (
	"errors"
	"math"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// gridChart is a chart of n panels over real data, in the shape a facet
// produces: one scale object shared by every panel.
func gridChart(n int, shared bool) render.Chart {
	x, y := scale.Linear(scale.Nice()), scale.Linear(scale.Nice())
	c := render.Chart{
		Width: 900, Height: 600, DPR: 1, Theme: theme.Light,
		Rows: 1, Cols: n, ShowLegend: true,
	}
	for i := range n {
		xs := make([]float64, 400)
		ys := make([]float64, 400)
		for j := range xs {
			xs[j] = float64(j)
			ys[j] = math.Sin(float64(j)/20 + float64(i))
		}
		px, py := x, y
		if !shared {
			px, py = scale.Linear(scale.Nice()), scale.Linear(scale.Nice())
		}
		c.Panels = append(c.Panels, render.Panel{
			Row: 0, Col: i,
			X: px, Y: py,
			Layers: []geom.Geom{
				geom.Line(data.Float64Columns(map[string][]float64{"x": xs, "y": ys}),
					geom.X("x"), geom.Y("y"), geom.Label("v")),
				geom.HLine(0),
			},
			ShowX: true,
			ShowY: !shared || i == 0,
		})
	}
	return c
}

// The whole point of replaying recordings in panel order: a parallel render
// must emit exactly the calls a serial one does, in exactly the same sequence.
// If that ever stops being true the golden files only cover one of the two.
func TestParallelAndSerialDrawTheSameCalls(t *testing.T) {
	for _, shared := range []bool{true, false} {
		par := gridChart(4, shared)
		ser := gridChart(4, shared)
		ser.Serial = true

		a, b := irtest.New(), irtest.New()
		if err := render.Draw(a, par); err != nil {
			t.Fatalf("parallel Draw: %v", err)
		}
		if err := render.Draw(b, ser); err != nil {
			t.Fatalf("serial Draw: %v", err)
		}
		if a.String() != b.String() {
			t.Errorf("shared=%v: the two paths drew different pictures\nparallel:\n%s\nserial:\n%s", shared, a, b)
		}
		if len(a.Calls) != len(b.Calls) {
			t.Fatalf("shared=%v: %d calls in parallel, %d in serial", shared, len(a.Calls), len(b.Calls))
		}
		for i := range a.Calls {
			if !sameCall(a.Calls[i], b.Calls[i]) {
				t.Fatalf("shared=%v: call %d differs: %+v vs %+v", shared, i, a.Calls[i], b.Calls[i])
			}
		}
	}
}

func sameCall(a, b irtest.Call) bool {
	if a.Op != b.Op || len(a.Points) != len(b.Points) {
		return false
	}
	for i := range a.Points {
		if a.Points[i] != b.Points[i] {
			return false
		}
	}
	if (a.Path == nil) != (b.Path == nil) {
		return false
	}
	if a.Path != nil {
		if len(a.Path.Pts) != len(b.Path.Pts) {
			return false
		}
		for i := range a.Path.Pts {
			if a.Path.Pts[i] != b.Path.Pts[i] {
				return false
			}
		}
	}
	return a.Stroke.Color == b.Stroke.Color && a.Text.Text == b.Text.Text && a.Rect == b.Rect
}

// Panels sharing an axis share the scale object, and drawing writes its device
// range. A snapshot per goroutine is what removes the sharing; without it every
// panel but one draws its data where another panel's axis is.
func TestSharedScalesSurviveAParallelRender(t *testing.T) {
	c := gridChart(3, true)
	rec := irtest.New()
	if err := render.Draw(rec, c); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	// Each panel's data is inside its own clip; the polylines drawn under a
	// clip must lie within it.
	var clip ir.Rect
	inside := 0
	for _, call := range rec.Calls {
		if call.Op == "Push" && call.HasClip {
			clip = call.ClipRect
			continue
		}
		if call.Op == "Pop" {
			clip = ir.Rect{}
			continue
		}
		if call.Op != "Polyline" || clip.Empty() {
			continue
		}
		for _, p := range call.Points {
			if p.X < clip.Min.X-1 || p.X > clip.Max.X+1 {
				t.Fatalf("a mark at x=%v was drawn for the panel clipped to %v — the panels shared a range", p.X, clip)
			}
		}
		inside++
	}
	if inside == 0 {
		t.Fatal("no clipped marks were drawn at all")
	}
}

type failingGeom struct{ err error }

func (g failingGeom) Train(x, y scale.Scale) error           { return nil }
func (g failingGeom) Build(b ir.Backend, f geom.Frame) error { return g.err }
func (g failingGeom) Legend(geom.Frame) (geom.LegendEntry, bool) {
	return geom.LegendEntry{}, false
}

func TestAFailingPanelReportsItsError(t *testing.T) {
	want := errors.New("boom")
	c := gridChart(3, false)
	c.Panels[1].Layers = append(c.Panels[1].Layers, failingGeom{err: want})
	if err := render.Draw(irtest.New(), c); !errors.Is(err, want) {
		t.Errorf("Draw returned %v, want the layer's own error", err)
	}
}

func BenchmarkPanelsSerial(b *testing.B)   { benchmarkPanels(b, true) }
func BenchmarkPanelsParallel(b *testing.B) { benchmarkPanels(b, false) }

func benchmarkPanels(b *testing.B, serial bool) {
	c := gridChart(8, false)
	c.Serial = serial
	rec := irtest.New()
	b.ReportAllocs()
	for b.Loop() {
		rec.Calls = rec.Calls[:0]
		if err := render.Draw(rec, c); err != nil {
			b.Fatal(err)
		}
	}
}
