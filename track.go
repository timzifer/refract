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
// Which edge a track is on decides which of the plot's scales it shares.
// Bottom and Top are grid rows and share the plot's X; Left and Right are grid
// columns and share its Y. That is the same statement twice, turned a quarter
// turn: a track shares the axis it runs along, and brings its own for the one
// it is thick in.
const (
	// Bottom puts the track below the panel. Several bottom tracks stack
	// downwards in the order they were added, and the last of them carries
	// the shared X axis.
	Bottom Edge = iota
	// Top puts the track above the panel, below the chart title. Several top
	// tracks stack upwards: the first added sits nearest the panel.
	Top
	// Left puts the track beside the panel, to its left. Several left tracks
	// stack leftwards — the first added sits nearest the panel — and the
	// outermost carries the shared Y axis.
	Left
	// Right puts the track beside the panel, to its right. Several right
	// tracks stack rightwards: the first added sits nearest the panel.
	Right
)

// String names the edge. It is what a spec document writes down.
func (e Edge) String() string {
	switch e {
	case Bottom:
		return "bottom"
	case Top:
		return "top"
	case Left:
		return "left"
	case Right:
		return "right"
	}
	return "unknown edge"
}

// vertical reports whether the edge is a column rather than a row — a track
// that shares the plot's Y and is thick in X.
func (e Edge) vertical() bool { return e == Left || e == Right }

// Track is a band at an edge of the plot area, on one of the plot's own scales.
//
// It is what a chart needs when part of what it shows is not on the panel's
// other axis at all: a gantt strip of machine states under a speed trace, a rug
// of event times, a ribbon of shifts, a key or a marginal distribution beside
// the panel. The track shares the axis it runs along — the same scale object,
// so a zoom is one zoom rather than two that agree — and carries a scale of its
// own across it, of whatever kind suits it. Putting an ordinal band under a
// linear panel is the case this exists for.
//
//	p := refract.New()
//	p.X(scale.Time()).Y(scale.Linear())
//	p.Add(geom.Line(src, geom.X("t"), geom.Y("speed")))
//
//	p.Track(refract.Bottom, refract.TrackSize(48)).
//		Add(geom.Rect(states, geom.X("start"), geom.X2("end"), geom.Y("row"),
//			geom.ColorBy("state", palette)))
//
// A track's size comes out of the panel, not out of the panel's domain: the
// axis the track does not share is identical with the track and without it.
// That is the difference between a track and the lane of negative values it
// replaces, where the lane was in the domain and `scale.Zero` stopped meaning
// what it said.
//
// The axis it *does* share is trained by both, because it is one axis and not
// two that agree — a rug of event times widens the time axis to cover the
// events, which is the whole reason to draw them against it.
type Track struct {
	edge     Edge
	size     float32
	fraction float32
	own      scale.Scale
	layers   []geom.Geom
	axis     bool
	axisSet  bool
	grid     bool
}

// TrackOption configures a track.
type TrackOption func(*Track)

// TrackSize sets how thick the band is, in device-independent pixels: the
// height of a [Bottom] or [Top] track, the width of a [Left] or [Right] one.
// It is the unit a lane count is naturally counted in — three lanes of sixteen
// pixels — and it is taken out of the panel, so the panel is what shrinks.
//
// It replaces any [TrackFraction]. The default is [DefaultTrackSize].
func TrackSize(px float32) TrackOption {
	return func(t *Track) { t.size, t.fraction = px, 0 }
}

// TrackFraction sets the band's thickness as a share of the canvas, in (0, 1)
// — of its height for a [Bottom] or [Top] track, of its width for a [Left] or
// [Right] one. It is what a [Responsive] chart wants: a strip given 48 pixels
// keeps them as the canvas shrinks around it, and eventually there is nothing
// left to keep them out of.
//
// It replaces any [TrackSize].
func TrackFraction(f float32) TrackOption {
	return func(t *Track) { t.fraction, t.size = f, 0 }
}

// TrackScale sets the track's own scale: the one it does not share. That is the
// vertical scale of a [Bottom] or [Top] track and the horizontal scale of a
// [Left] or [Right] one — in both cases the axis the band is thick in.
//
// The default is [scale.Ordinal], because a track's rows are usually lanes with
// names rather than a quantity. The scale is the track's alone: nothing here
// touches the scale the track shares with the panel.
func TrackScale(s scale.Scale) TrackOption {
	return func(t *Track) { t.own = s }
}

// TrackAxis decides whether the track writes the tick labels of its own scale —
// the lane names, on an ordinal one. The default is true: a lane nobody can
// name is a coloured stripe.
//
// They are written along the same edge the panel's are, and share the panel's
// gutter there, which is what keeps a track's edge and the panel's edge in the
// same place.
func TrackAxis(show bool) TrackOption {
	return func(t *Track) { t.axis, t.axisSet = show, true }
}

// TrackGrid decides whether the track draws grid lines. The default is false:
// a grid line through a gantt bar is a rule drawn across a solid shape.
func TrackGrid(show bool) TrackOption {
	return func(t *Track) { t.grid = show }
}

// DefaultTrackSize is the thickness of a track that was given none, in
// device-independent pixels. It is about three lanes' worth.
const DefaultTrackSize = 48

// Track attaches a band to an edge of the plot area and returns it, so that
// layers can be added to it.
//
//	p.Track(refract.Bottom, refract.TrackSize(48)).Add(states)
//
// Tracks at one edge stack in the order they were added, outwards from the
// panel. Bands on two edges at once are fine — a strip below and a key beside
// leave the corner between them empty — and a track with a [Plot.Facet] is
// [ErrTrackWithFacet]: a facet already owns the grid the track would need a
// row or a column of.
func (p *Plot) Track(e Edge, opts ...TrackOption) *Track {
	t := &Track{edge: e, size: DefaultTrackSize}
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

// ownScale is the track's own scale, defaulting to an ordinal one.
//
// It is created on first use rather than at construction so that a track built
// and never rendered costs nothing, matching [Plot.scaleX].
func (t *Track) ownScale() scale.Scale {
	if t.own == nil {
		t.own = scale.Ordinal()
	}
	return t.own
}

// showAxis reports whether the track writes its lane names.
func (t *Track) showAxis() bool {
	if t.axisSet {
		return t.axis
	}
	return true
}

// sizeIn resolves the track's thickness in device units against a canvas.
//
// A fraction is resolved here, where the canvas is known, and against the
// dimension the band is actually thick in, so that nothing downstream of this
// ever sees anything but device units — the layout solver included, which is
// what keeps a fixed track's arithmetic one subtraction.
func (t *Track) sizeIn(canvasW, canvasH int) float32 {
	if t.fraction <= 0 {
		return t.size
	}
	if t.edge.vertical() {
		return t.fraction * float32(canvasW)
	}
	return t.fraction * float32(canvasH)
}

// tracked turns a single-panel chart into the grid its tracks need: the panel,
// with a row for every band above and below it and a column for every band
// beside it.
//
// The panel and every track are given the *same* scale object for the axis
// they share — X down a column, Y across a row. That is not an optimisation
// and not a convenience: it is the whole mechanism. A zoom reaches a scale
// through [scale.Zoomer.SetDomain] and there is only one scale to reach, so
// the panel and its tracks move together by construction rather than by two
// handlers agreeing. Cloning it here would compile, would look tidier, and
// would silently break every acceptance criterion this feature has.
func (p *Plot) tracked(c render.Chart) render.Chart {
	var top, bottom, left, right int
	for _, t := range p.tracks {
		switch t.edge {
		case Top:
			top++
		case Left:
			left++
		case Right:
			right++
		default:
			bottom++
		}
	}

	rows := top + 1 + bottom
	cols := left + 1 + right
	c.Rows, c.Cols = rows, cols
	c.RowHeights = make([]float32, rows)
	c.ColWidths = make([]float32, cols)
	c.Layers = nil

	// The panel sits past whatever is above it and to the left of it. A cell
	// where a row of bands crosses a column of them holds nothing: the corner
	// between a strip below and a key beside is a hole in the grid, which the
	// solver already understands because a wrapped facet leaves them too.
	panelRow, panelCol := top, left

	// The shared axes are written at the outside of the grid — the X labels by
	// the bottom-most row, the Y labels by the left-most column — so that a
	// panel's own numbers are never written into the middle of the chart,
	// under or beside a band rather than at the axis they belong to.
	lastRow := rows - 1

	// The data panel is panel zero and the tracks follow it in the order they
	// were added. That order is the chart's public shape — it is what a Hit
	// names and what an Observer is told — so it is fixed here rather than
	// falling out of which edges happen to be in use: a chart that grew a top
	// track must not renumber the bottom one that a tooltip already knew.
	c.Panels = append(c.Panels, render.Panel{
		Row:    panelRow,
		Col:    panelCol,
		X:      c.X,
		Y:      c.Y,
		Y2:     c.Y2,
		X2:     c.X2,
		Coord:  p.coord,
		Layers: p.layers,
		ShowX:  panelRow == lastRow,
		ShowY:  panelCol == 0,
		// The second axis belongs to the data panel and is written at the
		// right-hand edge of the grid, which is the last column — a right
		// track puts a band there instead, and the axis then sits between the
		// panel and the band rather than outside it.
		ShowY2: panelCol == cols-1,
		// The second horizontal axis is written at the top of the grid, which
		// is the mirror of where the first is written at the bottom — a top
		// track puts a band there instead, and the axis then sits between the
		// panel and the band.
		ShowX2: panelRow == 0,
	})

	// A band nearest the panel is the first one added at its edge, so the
	// added order runs outwards and the row and column numbers run away from
	// the panel.
	var nTop, nBottom, nLeft, nRight int
	for _, t := range p.tracks {
		row, col := panelRow, panelCol
		switch t.edge {
		case Top:
			row = panelRow - 1 - nTop
			nTop++
		case Left:
			col = panelCol - 1 - nLeft
			nLeft++
		case Right:
			col = panelCol + 1 + nRight
			nRight++
		default:
			row = panelRow + 1 + nBottom
			nBottom++
		}
		c.Panels = append(c.Panels, t.panel(c, row, col, row == lastRow, col == 0))
		if t.edge.vertical() {
			c.ColWidths[col] = t.sizeIn(c.Width, c.Height)
		} else {
			c.RowHeights[row] = t.sizeIn(c.Width, c.Height)
		}
	}
	return c
}

// panel is the render panel one track is drawn as.
//
// atBottom and atLeft say whether this cell is at the outside of the grid on
// each axis, which is where a shared axis is written. On the axis the track
// shares, that decides whether it writes the panel's numbers; on the axis it
// owns, its lane names are written wherever the track is, because they name
// that band and nothing else.
//
// A track is Cartesian whatever the chart is: it is a band along an axis, and
// a band bent around a polar chart's angle is not a band.
func (t *Track) panel(c render.Chart, row, col int, atBottom, atLeft bool) render.Panel {
	p := render.Panel{
		Row:      row,
		Col:      col,
		Coord:    coordpkg.Cartesian(),
		Layers:   t.layers,
		HideGrid: !t.grid,
	}
	if t.edge.vertical() {
		// A band beside the panel runs along Y and is thick in X.
		p.X, p.Y = t.ownScale(), c.Y
		p.ShowX, p.ShowY = t.showAxis(), atLeft
		return p
	}
	p.X, p.Y = c.X, t.ownScale()
	p.ShowX, p.ShowY = atBottom, t.showAxis()
	return p
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
			Size:     t.size,
			Fraction: t.fraction,
			Scale:    t.ownScale(),
			Layers:   t.layers,
			Axis:     t.showAxis(),
			Grid:     t.grid,
		})
	}
	return out
}

// edgeNamed is the edge a document names. An unknown name is Bottom, which is
// the edge a track is on when nobody said: a document from a later version
// naming an edge this one does not have draws a chart that is wrong in a place
// the reader can see, rather than failing to draw at all.
func edgeNamed(s string) Edge {
	for _, e := range []Edge{Top, Left, Right} {
		if s == e.String() {
			return e
		}
	}
	return Bottom
}
