package render_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/theme"
)

// What v0.8 moved out of render and what it left in.
//
// The geometry of a grid line, an axis line and a tick label now comes from
// the coord. The *order* everything is drawn in did not move, and neither did
// the decisions that are the theme's: which grid lines are wanted, which
// labels would collide, whether this panel writes labels at all.

// A Cartesian panel's grid lines still reach the backend as two-point
// polylines. That is the property every golden file in the repository is
// written in terms of, and it is the one a coord could break silently by
// reporting a path where it used to report a run.
func TestACartesianGridIsStillPolylines(t *testing.T) {
	rec := draw(t, chart(line()))
	for _, c := range rec.Calls {
		if c.Op != "StrokePath" {
			continue
		}
		// The only stroked paths a plain line chart draws are the layer's own,
		// and it draws none: a polyline is a Polyline.
		t.Errorf("a Cartesian chart stroked a path where it used to draw a polyline: %v", c.Path.Ops)
	}
	if rec.Count("Polyline") == 0 {
		t.Error("the grid and the axes drew nothing")
	}
}

// The furniture pass reuses one buffer across the panels of a chart, and a
// panel with fewer ticks than the last must not inherit the last one's.
func TestFurnitureDoesNotLeakBetweenPanels(t *testing.T) {
	c := chart(line())
	// Two panels, the second with a domain that yields fewer ticks than the
	// first. Nothing may be drawn twice.
	c.Panels = []render.Panel{
		{Row: 0, Col: 0, X: c.X, Y: c.Y, Layers: c.Layers, ShowX: true, ShowY: true},
		{Row: 0, Col: 1, X: c.X, Y: c.Y, Layers: c.Layers, ShowX: true, ShowY: true},
	}
	c.Rows, c.Cols = 1, 2
	rec := draw(t, c)

	// Every polyline is either inside one panel's rectangle or on its gutter,
	// and no two panels share a rectangle — so a leaked shape would be a
	// duplicate. Count them instead of chasing geometry: the two panels are
	// identical, so an exact doubling is what correct looks like.
	seen := map[string]int{}
	for _, call := range rec.Calls {
		if call.Op != "Polyline" || len(call.Points) != 2 {
			continue
		}
		seen[pointsKey(call.Points)]++
	}
	for k, n := range seen {
		if n > 2 {
			t.Errorf("the run %s was drawn %d times; furniture leaked between panels", k, n)
		}
	}
}

func pointsKey(pts []ir.Point) string {
	var b strings.Builder
	for _, p := range pts {
		fmt.Fprintf(&b, "%.3f,%.3f;", p.X, p.Y)
	}
	return b.String()
}

// A theme that turns an axis's ticks off writes no tick labels, and reserves
// no gutter for the ones it did not write — which is what gives a pie with its
// furniture off the whole panel to fill.
func TestTicksOffWritesNoLabelsAndNoGutter(t *testing.T) {
	with := draw(t, chart(line()))
	c := chart(line())
	c.Theme = theme.Light.With(theme.Ticks(false, false))
	without := draw(t, c)

	if len(without.Texts()) != 0 {
		t.Errorf("a chart with its ticks off wrote %v", without.Texts())
	}
	if len(with.Texts()) == 0 {
		t.Fatal("a chart with its ticks on wrote no labels")
	}
	// The plot area is what the gutter was taken from, so it grows.
	if plotArea(without) <= plotArea(with) {
		t.Errorf("the plot area is %v with the ticks off and %v with them on; "+
			"the gutter was reserved anyway", plotArea(without), plotArea(with))
	}
}

// plotArea is the area of the clip a chart's data was drawn inside.
func plotArea(rec *irtest.Recorder) float32 {
	for _, c := range rec.Calls {
		if c.Op == "Push" && c.HasClip {
			return c.ClipRect.Dx() * c.ClipRect.Dy()
		}
	}
	return 0
}

// A polar chart clips to a disc rather than to a rectangle, and the coord is
// what says so.
func TestAPolarPanelClipsToADisc(t *testing.T) {
	c := chart(line())
	c.Coord = coord.Polar()
	rec := draw(t, c)
	for _, call := range rec.Calls {
		if call.Op != "Push" || !call.HasClip {
			continue
		}
		for _, op := range call.Path.Ops {
			if op == ir.OpCubicTo {
				return
			}
		}
		t.Fatalf("a polar panel clipped to %v", call.Path.Ops)
	}
	t.Fatal("no clip was pushed")
}
