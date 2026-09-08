package render_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/interact"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func labelChart(avoid bool, panels int) render.Chart {
	c := render.Chart{Width: 800, Height: 400, DPR: 1, Theme: theme.Light, Rows: 1, Cols: panels}
	for range panels {
		p := render.Panel{X: scale.Linear(), Y: scale.Linear()}
		// Separate layers must use the same collision state within one panel.
		for i := range 10 {
			src := data.NewTable().Float64("x", []float64{0}).Float64("y", []float64{0}).String("label", []string{fmt.Sprintf("label%02d", i)})
			p.Layers = append(p.Layers, geom.Text(src, geom.X("x"), geom.Y("y"), geom.TextBy("label"), geom.AvoidOverlap(avoid)))
		}
		c.Panels = append(c.Panels, p)
	}
	return c
}

func labelCalls(r *irtest.Recorder) []ir.TextRun {
	var out []ir.TextRun
	for _, c := range r.Filter("Text") {
		if strings.HasPrefix(c.Text.Text, "label") {
			out = append(out, c.Text)
		}
	}
	return out
}

func TestLabelLayoutIsPanelLocalAndIndependentOfScheduling(t *testing.T) {
	c := labelChart(true, 2)
	a, b := irtest.New(), irtest.New()
	if err := render.Draw(a, c); err != nil {
		t.Fatal(err)
	}
	c.Serial = true
	if err := render.Draw(b, c); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Trace(), b.Trace()) {
		t.Fatal("parallel and serial labels differ")
	}
	if n := len(labelCalls(a)); n != 18 {
		t.Fatalf("drew %d labels, want 9 per panel", n)
	}
	c = labelChart(false, 1)
	r := irtest.New()
	if err := render.Draw(r, c); err != nil {
		t.Fatal(err)
	}
	if n := len(labelCalls(r)); n != 10 {
		t.Fatalf("default dropped %d labels", 10-n)
	}
}

func TestWatchingPlacedLabelsKeepsDrawingAndRows(t *testing.T) {
	c := labelChart(true, 1)
	a, b := irtest.New(), irtest.New()
	if err := render.Draw(a, c); err != nil {
		t.Fatal(err)
	}
	ix := interact.New().TrackRows(true)
	c.Observer, c.RowSink = ix, ix
	if err := render.Draw(ix.Watch(b), c); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Trace(), b.Trace()) {
		t.Fatal("observer changed label placement")
	}
	if ix.RowCount() != 9 {
		t.Fatalf("%d rows, want only visible labels", ix.RowCount())
	}
	for i, r := range labelCalls(b) {
		hit, ok := ix.At(ir.Point{X: r.At.X + 1, Y: r.At.Y - 1}, 0)
		if !ok || hit.Layer != i || hit.Row != 0 {
			t.Errorf("label %d: hit %v, %v", i, hit, ok)
		}
	}
}
