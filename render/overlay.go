package render

import (
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// Overlay paints over a finished chart: a crosshair, a tooltip, a brush
// rectangle, a ring around the mark another chart is pointing at.
//
// # Why it is not a layer
//
// A layer's positions come from its data, through the scales. An overlay's come
// from somewhere a layer has no access to — a pointer, a selection, or where
// the *previous* frame put something — and a layer that read any of those would
// be a layer whose data depended on its own last drawing. So this is a separate
// stage with a separate seam, drawn after everything else.
//
// It is also not announced to the [Observer], and therefore not hit-testable.
// That is deliberate and it is the property that makes an overlay usable: a
// tooltip a pointer can hit is a tooltip that flickers, because hovering it
// moves the pointer off whatever the tooltip was about.
//
// # What it is given
//
// The backend, and where the panels are. It is not clipped: a crosshair wants
// to be confined to its panel and a tooltip wants to overflow one, so an
// overlay that wants a clip pushes its own — [OverlayPanel.Area] is what to
// push. Drawing outside the canvas is the caller's business, as it is for a
// geom.
//
// # Cost
//
// A chart with no overlay pays nothing: [Chart.Overlay] is nil and this is not
// called. What an overlay costs when there is one is the calls it makes, plus
// one consideration that is easy to miss — an overlay that *appears* or
// *disappears* changes how many calls a frame has, which makes that frame not
// comparable with the last and therefore a full repaint. One that only moves is
// a damage rectangle like anything else. A crosshair following a pointer is the
// cheap case; one that blinks on and off at every panel boundary is two full
// repaints per crossing.
//
// An Overlay is implemented outside this package, so it never gains a method.
type Overlay interface {
	// DrawOverlay paints over the finished chart.
	DrawOverlay(b ir.Backend, f OverlayFrame)
}

// OverlayPanel is one panel of the frame an overlay is drawing over: where it
// is, and what places values in it.
type OverlayPanel struct {
	// Index is the panel's position in the chart, matching the index an
	// [Observer] was told and the one [interact.Hit] reports.
	Index int
	// Area is the panel's plot rectangle in device space.
	Area ir.Rect
	// X and Y are the panel's scales, ranged to Area. An overlay reads them
	// to turn a value into a place — a crosshair at a threshold, a band
	// between two dates — and must not modify them.
	X, Y scale.Scale
	// Y2 and X2 are the panel's secondary axes, or nil where it has none.
	Y2, X2 scale.Scale
	// Coord is the coordinate system the panel was drawn in, framed to Area.
	// It is what turns a mapped pair into a point, and is [coord.Cartesian]
	// for a chart that named no coord — reach for [OverlayPanel.Coords].
	Coord coord.Coord
}

// Coords is the panel's coordinate system, which is [coord.Cartesian] framed in
// the plot rectangle when it has none.
func (p OverlayPanel) Coords() coord.Coord {
	if p.Coord == nil {
		return coord.Cartesian().Frame(p.Area, nil, nil)
	}
	return p.Coord
}

// OverlayFrame is what an overlay is told about the chart it is drawing over.
type OverlayFrame struct {
	// Canvas is the whole drawing, in device space.
	Canvas ir.Rect
	// Panels are the chart's panels, in chart order.
	Panels []OverlayPanel
	// Theme is the chart's theme, so that an overlay drawn over a dark chart
	// is legible on it.
	Theme theme.Theme
}

// PanelAt reports which panel contains a device point, for an overlay deciding
// which one a pointer is in.
func (f OverlayFrame) PanelAt(pt ir.Point) (OverlayPanel, bool) {
	for _, p := range f.Panels {
		if p.Area.Contains(pt) {
			return p, true
		}
	}
	return OverlayPanel{}, false
}
