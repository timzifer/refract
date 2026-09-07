package refract

import (
	coordpkg "github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/spec"
)

// Edge names a side of the plot area.
type Edge int

// The edges a track can be attached to.
//
// Bottom and Top are grid rows, which is why they are what v0.10 carries: a
// track takes its height out of the panel and shares the panel's X. Left and
// Right are the mirror image — grid columns sharing the panel's Y — and are
// not implemented yet rather than being renumbered later.
const (
	// Bottom puts the track below the panel. Several bottom tracks stack
	// downwards in the order they were added, and the last of them carries
	// the shared X axis.
	Bottom Edge = iota
	// Top puts the track above the panel, below the chart title. Several top
	// tracks stack upwards: the first added sits nearest the panel.
	Top
)

// String names the edge, for an error message.
func (e Edge) String() string {
	switch e {
	case Bottom:
		return "bottom"
	case Top:
		return "top"
	}
	return "unknown edge"
}

// Track is a band at an edge of the plot area, on the plot's own X scale.
//
// It is what a chart needs when part of what it shows is not on the panel's
// Y axis at all: a gantt strip of machine states under a speed trace, a rug
// of event times, a ribbon of shift boundaries. The track shares the panel's X
// — the same scale object, so a zoom is one zoom rather than two that agree —
// and carries a Y scale of its own, of whatever kind suits it. Putting an
// ordinal band under a linear panel is the case this exists for.
//
//	p := refract.New()
//	p.X(scale.Time()).Y(scale.Linear())
//	p.Add(geom.Line(src, geom.X("t"), geom.Y("speed")))
//
//	p.Track(refract.Bottom, refract.TrackHeight(48)).
//		Add(geom.Rect(states, geom.X("start"), geom.X2("end"), geom.Y("row"),
//			geom.ColorBy("state", palette)))
//
// A track's height comes out of the panel, not out of the panel's domain: the
// panel's Y domain is identical with the track and without it. That is the
// difference between a track and the lane of negative values it replaces.
type Track struct {
	edge     Edge
	height   float32
	fraction float32
	y        scale.Scale
	layers   []geom.Geom
	axis     bool
	axisSet  bool
	grid     bool
}

// TrackOption configures a track.
type TrackOption func(*Track)

// TrackHeight sets the track's height in device-independent pixels. It is the
// unit a lane count is naturally counted in — three lanes of sixteen pixels —
// and it is taken out of the panel, so the panel is what shrinks.
//
// It replaces any [TrackFraction]. The default is [DefaultTrackHeight].
func TrackHeight(px float32) TrackOption {
	return func(t *Track) { t.height, t.fraction = px, 0 }
}

// TrackFraction sets the track's height as a share of the canvas, in (0, 1).
// It is what a [Responsive] chart wants: a strip given 48 pixels keeps them as
// the canvas shrinks around it, and eventually there is nothing left to keep
// them out of.
//
// It replaces any [TrackHeight].
func TrackFraction(f float32) TrackOption {
	return func(t *Track) { t.fraction, t.height = f, 0 }
}

// TrackScale sets the track's own vertical scale. The default is
// [scale.Ordinal], because a track's rows are usually lanes with names rather
// than a quantity.
//
// The scale is the track's alone. Nothing here touches the plot's Y.
func TrackScale(s scale.Scale) TrackOption {
	return func(t *Track) { t.y = s }
}

// TrackAxis decides whether the track writes its own tick labels — the lane
// names, on an ordinal scale. The default is true: a lane nobody can name is a
// coloured stripe.
//
// The labels share the panel's left gutter, which is what keeps the track's
// left edge and the panel's left edge in the same place.
func TrackAxis(show bool) TrackOption {
	return func(t *Track) { t.axis, t.axisSet = show, true }
}

// TrackGrid decides whether the track draws grid lines. The default is false:
// a grid line through a gantt bar is a rule drawn across a solid shape.
func TrackGrid(show bool) TrackOption {
	return func(t *Track) { t.grid = show }
}

// DefaultTrackHeight is the height of a track that was given none, in
// device-independent pixels. It is about three lanes' worth.
const DefaultTrackHeight = 48

// Track attaches a band to an edge of the plot area and returns it, so that
// layers can be added to it.
//
//	p.Track(refract.Bottom, refract.TrackHeight(48)).Add(states)
//
// Tracks at one edge stack in the order they were added. A track and a
// [Plot.Facet] on the same plot is [ErrTrackWithFacet]: a facet already owns
// the grid the track would need a row of.
func (p *Plot) Track(e Edge, opts ...TrackOption) *Track {
	t := &Track{edge: e, height: DefaultTrackHeight}
	for _, o := range opts {
		o(t)
	}
	p.tracks = append(p.tracks, t)
	return t
}

// Tracks reports the tracks attached to the plot, in the order they were
// added. It is what a caller rebuilding a plot from another one needs.
func (p *Plot) Tracks() []*Track { return p.tracks }

// Add appends layers to the track, drawn in the order given.
func (t *Track) Add(gs ...geom.Geom) *Track { t.layers = append(t.layers, gs...); return t }

// Edge reports which side of the panel the track is attached to.
func (t *Track) Edge() Edge { return t.edge }

// scaleY is the track's vertical scale, defaulting to an ordinal one.
//
// It is created on first use rather than at construction so that a track built
// and never rendered costs nothing, matching [Plot.scaleX].
func (t *Track) scaleY() scale.Scale {
	if t.y == nil {
		t.y = scale.Ordinal()
	}
	return t.y
}

// showAxis reports whether the track writes its lane names.
func (t *Track) showAxis() bool {
	if t.axisSet {
		return t.axis
	}
	return true
}

// heightIn resolves the track's height in device units against a canvas of the
// given height.
//
// A fraction is resolved here, where the canvas is known, so that nothing
// downstream of this ever sees anything but device units — the layout solver
// included, which is what keeps a fixed row's arithmetic one subtraction.
func (t *Track) heightIn(canvasH int) float32 {
	if t.fraction > 0 {
		return t.fraction * float32(canvasH)
	}
	return t.height
}

// tracked turns a single-panel chart into the one-column grid its tracks
// need: the top tracks, then the panel, then the bottom ones.
//
// The panel and every track are given the *same* X scale object. That is not
// an optimisation and not a convenience — it is the whole mechanism. A zoom
// reaches a scale through [scale.Zoomer.SetDomain] and there is only one scale
// to reach, so the panel and its tracks move together by construction rather
// than by two handlers agreeing. Cloning X here would compile, would look
// tidier, and would silently break every acceptance criterion this feature
// has.
func (p *Plot) tracked(c render.Chart) render.Chart {
	var top, bottom []*Track
	for _, t := range p.tracks {
		if t.edge == Top {
			top = append(top, t)
		} else {
			bottom = append(bottom, t)
		}
	}

	rows := len(top) + 1 + len(bottom)
	c.Rows, c.Cols = rows, 1
	c.RowHeights = make([]float32, rows)
	c.Layers = nil

	// The bottom-most row carries the shared X axis, wherever it is: the
	// panel's own X labels would otherwise be written into the middle of the
	// chart, under a band rather than under the axis they belong to.
	last := rows - 1

	// A top track nearest the panel is the first one added, so the added order
	// runs upwards and the row numbers run down from the panel.
	panelRow := len(top)
	for i, t := range top {
		row := panelRow - 1 - i
		c.Panels = append(c.Panels, t.panel(c, row, last))
		c.RowHeights[row] = t.heightIn(c.Height)
	}
	c.Panels = append(c.Panels, render.Panel{
		Row:    panelRow,
		X:      c.X,
		Y:      c.Y,
		Coord:  p.coord,
		Layers: p.layers,
		ShowX:  panelRow == last,
		ShowY:  true,
	})
	for i, t := range bottom {
		row := panelRow + 1 + i
		c.Panels = append(c.Panels, t.panel(c, row, last))
		c.RowHeights[row] = t.heightIn(c.Height)
	}
	return c
}

// panel is the render panel one track is drawn as.
//
// A track is Cartesian whatever the chart is: it is a band along an axis, and
// a band bent around a polar chart's angle is not a band.
func (t *Track) panel(c render.Chart, row, last int) render.Panel {
	return render.Panel{
		Row:      row,
		X:        c.X,
		Y:        t.scaleY(),
		Coord:    coordpkg.Cartesian(),
		Layers:   t.layers,
		ShowX:    row == last,
		ShowY:    t.showAxis(),
		HideGrid: !t.grid,
	}
}

// trackSpecs writes the plot's tracks down for [Plot.Spec].
//
// The scale is resolved here rather than left nil, so that a document says
// which kind of axis the band has instead of leaving the reader to know the
// default.
func (p *Plot) trackSpecs() []spec.Track {
	if len(p.tracks) == 0 {
		return nil
	}
	out := make([]spec.Track, 0, len(p.tracks))
	for _, t := range p.tracks {
		out = append(out, spec.Track{
			Edge:     t.edge.String(),
			Height:   t.height,
			Fraction: t.fraction,
			Y:        t.scaleY(),
			Layers:   t.layers,
			Axis:     t.showAxis(),
			Grid:     t.grid,
		})
	}
	return out
}

// edgeNamed is the edge a document names. An unknown name is Bottom, which is
// the edge a track is on when nobody said: a document from a later version
// naming an edge this one does not have draws a chart that is wrong in the
// place the reader can see, rather than failing to draw at all.
func edgeNamed(s string) Edge {
	if s == Top.String() {
		return Top
	}
	return Bottom
}
