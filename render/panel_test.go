package render_test

import (
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func panelLine(label string) geom.Geom {
	src := data.Float64Columns(map[string][]float64{"x": {0, 1, 2}, "y": {0, 2, 1}})
	return geom.Line(src, geom.X("x"), geom.Y("y"), geom.Label(label))
}

func twoPanels(sharedScales bool) render.Chart {
	x, y := scale.Linear(scale.Nice()), scale.Linear(scale.Nice())
	c := render.Chart{
		Width: 800, Height: 500, DPR: 1, Theme: theme.Light,
		Rows: 1, Cols: 2,
	}
	for i := range 2 {
		px, py := x, y
		if !sharedScales {
			px, py = scale.Linear(scale.Nice()), scale.Linear(scale.Nice())
		}
		c.Panels = append(c.Panels, render.Panel{
			Row: 0, Col: i,
			Strip:  []string{"left", "right"}[i],
			X:      px,
			Y:      py,
			Layers: []geom.Geom{panelLine("v")},
			ShowX:  true,
			ShowY:  !sharedScales || i == 0,
		})
	}
	return c
}

// Each panel's data goes inside its own clip, or one panel's line runs across
// its neighbour.
func TestEachPanelClipsItsOwnData(t *testing.T) {
	rec := draw(t, twoPanels(true))
	var clips []ir.Rect
	for _, c := range rec.Calls {
		if c.Op == "Push" && c.HasClip {
			clips = append(clips, c.ClipRect)
		}
	}
	if len(clips) != 2 {
		t.Fatalf("pushed %d clipped groups, want one per panel", len(clips))
	}
	if clips[0].Max.X > clips[1].Min.X {
		t.Errorf("the panels' clips overlap: %v and %v", clips[0], clips[1])
	}
	if rec.MaxDepth != 1 {
		t.Errorf("clip stack reached depth %d, want 1", rec.MaxDepth)
	}
}

func TestStripsAreDrawn(t *testing.T) {
	rec := draw(t, twoPanels(true))
	for _, want := range []string{"left", "right"} {
		if !hasText(rec, want) {
			t.Errorf("strip %q was not drawn: %v", want, texts(rec))
		}
	}
}

// A panel that shares its axis with the panel beside it leaves the tick labels
// to the edge of the grid, but still gets the ticks themselves.
func TestASharedAxisIsLabelledOnce(t *testing.T) {
	one := yTickLabels(draw(t, twoPanels(true)))
	both := yTickLabels(draw(t, twoPanels(false)))
	if one == 0 {
		t.Fatal("the shared Y axis was not labelled at all")
	}
	if one*2 != both {
		t.Errorf("a shared Y axis wrote %d labels and a free one %d; want half as many", one, both)
	}
}

func TestAFreeAxisIsLabelledOnEveryPanel(t *testing.T) {
	rec := draw(t, twoPanels(false))
	// Both panels hold the same data, so each writes the same labels; what
	// matters is that both wrote them.
	seen := map[float32]int{}
	for _, c := range rec.Filter("Text") {
		if c.Text.H == ir.AlignEnd && c.Text.V == ir.AlignMiddle {
			seen[c.Text.At.X]++
		}
	}
	if len(seen) != 2 {
		t.Errorf("Y labels were written at %d distinct x positions, want one per panel", len(seen))
	}
}

// yTickLabels counts the runs written the way a Y tick label is: right
// aligned against the axis, centred on the tick.
func yTickLabels(rec *irtest.Recorder) int {
	n := 0
	for _, c := range rec.Filter("Text") {
		if c.Text.H == ir.AlignEnd && c.Text.V == ir.AlignMiddle {
			n++
		}
	}
	return n
}

// Panels sharing a scale object share one range, so the second pass has to set
// it again before drawing — otherwise every panel but the last draws its data
// where the last panel's axis is.
func TestSharedScalesAreRangedPerPanel(t *testing.T) {
	c := twoPanels(true)
	rec := draw(t, c)

	var clips []ir.Rect
	polylinesIn := map[int]int{}
	for _, call := range rec.Calls {
		if call.Op == "Push" && call.HasClip {
			clips = append(clips, call.ClipRect)
		}
	}
	for _, call := range rec.Filter("Polyline") {
		for i, clip := range clips {
			inside := true
			for _, p := range call.Points {
				if p.X < clip.Min.X-0.5 || p.X > clip.Max.X+0.5 {
					inside = false
					break
				}
			}
			if inside {
				polylinesIn[i]++
			}
		}
	}
	for i := range clips {
		if polylinesIn[i] == 0 {
			t.Errorf("panel %d has no geometry inside it", i)
		}
	}
}

// The legend describes the layers, and faceted panels carry the same ones. A
// legend that grew with the facet would be a list of the same series repeated.
func TestTheLegendIsNotRepeatedPerPanel(t *testing.T) {
	c := twoPanels(true)
	c.ShowLegend = true
	rec := draw(t, c)
	got := 0
	for _, s := range texts(rec) {
		if s == "v" {
			got++
		}
	}
	if got != 1 {
		t.Errorf("the legend row appears %d times, want 1", got)
	}
}

func TestPanelErrorsPropagate(t *testing.T) {
	c := twoPanels(true)
	c.Panels[1].Layers = []geom.Geom{geom.Line(
		data.Float64Columns(map[string][]float64{"x": {0}}), geom.X("x"), geom.Y("nope"))}
	rec := irtest.New()
	if err := render.Draw(rec, c); err == nil {
		t.Error("a panel's error did not reach the caller")
	}
}

// A single-panel chart is a one-by-one grid, so the same code draws both.
func TestASinglePanelChartStillWorks(t *testing.T) {
	rec := draw(t, chart(line()))
	if rec.Count("Polyline") == 0 {
		t.Error("nothing was drawn")
	}
	if rec.MaxDepth != 1 {
		t.Errorf("clip depth %d, want 1", rec.MaxDepth)
	}
}
