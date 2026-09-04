// Package interact turns a rendered chart into something a pointer can ask
// questions of.
//
// It has two halves. [Index] is the spatial index: it watches a render, notes
// where every data mark landed and which layer of which panel drew it, and
// answers "what is under this point". [Event] and its kinds are the vocabulary
// pointer input is reported in; the root package's Live wires the two together
// — turning a browser's pointer, wheel and drag into hits, zooms and pans —
// and redraws.
//
// # Why it watches rather than asks
//
// A geom emits primitives and forgets them. Asking a layer afterwards where
// its rows ended up would mean every geom carrying a second, parallel
// implementation of its own projection — and the two would disagree, on some
// axis type, eventually. So the index takes the marks the render actually
// emitted, which is the definition of what the reader can see, and turns a
// device position back into data through the same scales that put it there.
//
// # What it costs
//
// Only a watched render pays: [Index.Watch] copies the points a layer draws,
// which is memory proportional to the marks on screen. That is the reduced
// count rather than the row count — a line over a million rows draws a couple
// of thousand marks after decimation — but it is not nothing, which is why an
// unwatched render still allocates nothing that grows with its data.
package interact

import (
	"image"
	"math"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Kind is what sort of mark a hit landed on.
type Kind uint8

// The mark kinds. They differ in how a hit is decided: a vertex is hit by
// being near it, an area by being inside it.
const (
	// Vertex is a point a layer drew: a line's sample, a scatter's marker.
	Vertex Kind = iota
	// Area is a filled shape: a bar, a band, a boxplot's box.
	Area
	// Label is text a layer drew.
	Label
)

// Hit is what the pointer found.
type Hit struct {
	// Panel is which panel the mark is in, and Layer which layer of it.
	Panel, Layer int
	// Series is the layer's legend label, empty for a layer that has none.
	Series string
	// Kind is what sort of mark this is.
	Kind Kind
	// At is the mark's position in device space.
	At ir.Point
	// X and Y are the data values at At, read back through the panel's
	// scales. On a categorical axis the value is the category index — pass it
	// to [scale.Categorical.Labels] to name it.
	X, Y float64
	// Distance is how far At is from the point that was asked about, in
	// device units. It is zero for a point inside an area.
	Distance float32

	// Row is the source row behind the mark, or -1 when it is not known.
	//
	// It is -1 unless row tracking was on for the render — see
	// [Index.TrackRows] — and it stays -1 for a mark that no row is behind: a
	// boxplot's box aggregates many rows, a density raster is not a mark, an
	// interpolated point across a gap was never measured, and a third-party
	// geom that does not report its rows has none to report.
	Row int
}

// Panel is one panel of a watched render: where it is and what places values
// in it.
type Panel struct {
	// Area is the panel's plot rectangle in device space.
	Area ir.Rect
	// X and Y are the panel's scales, ranged to Area.
	X, Y scale.Scale
}

// Index is a spatial index over one render.
//
// It implements the render package's Observer, and [Index.Watch] wraps the
// backend a chart is drawn into. Neither changes what is drawn.
//
// An Index is not safe for concurrent use; nor is it safe to query while the
// render that fills it is still running.
type Index struct {
	panels []Panel
	marks  []mark
	pts    []ir.Point

	track bool
	rows  []rowMark

	panel int
	layer int
	label string
	open  bool
}

// rowMark is where one source row landed.
//
// It is kept apart from the drawn marks on purpose: what a layer draws and
// what a row is are different points — a smoothed line is a curve through its
// rows, a staircase draws two points per row, a bar is four corners around
// one. See [geom.Rows].
type rowMark struct {
	panel, layer int
	at           ir.Point
	row          int
}

type mark struct {
	panel, layer int
	kind         Kind
	label        string
	lo, hi       int // into pts
	bounds       ir.Rect
}

// New returns an empty index.
func New() *Index { return &Index{panel: -1, layer: -1} }

// Reset empties the index, keeping its memory for the next render.
func (ix *Index) Reset() {
	ix.panels = ix.panels[:0]
	ix.marks = ix.marks[:0]
	ix.pts = ix.pts[:0]
	ix.rows = ix.rows[:0]
	ix.panel, ix.layer, ix.label, ix.open = -1, -1, "", false
}

// TrackRows turns row identity on or off and returns ix, so the call can be
// chained onto [New].
//
// It is off by default because it is not free: every layer that can report its
// rows does the bookkeeping, and the index keeps a position and a row number
// per mark on top of the marks it already keeps. Turn it on when a hit has to
// name a row rather than describe a point — highlighting the matching row of a
// table beside the chart is the case it exists for.
//
// It takes effect on the next render, and only if the caller also hands the
// index to the renderer as its row sink; [github.com/timzifer/refract.Live]
// does that.
func (ix *Index) TrackRows(on bool) *Index { ix.track = on; return ix }

// TrackingRows reports whether row identity is on.
func (ix *Index) TrackingRows() bool { return ix.track }

// Marks implements the geom package's Rows: it is how a layer reports where
// each of its source rows landed.
//
// The slices are lent for the call — they come from the geom's pooled
// scratch — so the positions are copied out.
func (ix *Index) Marks(at []ir.Point, rows []int) {
	if !ix.track || !ix.open || len(at) != len(rows) {
		return
	}
	for i, p := range at {
		if rows[i] < 0 {
			continue
		}
		ix.rows = append(ix.rows, rowMark{
			panel: ix.panel, layer: ix.layer, at: p, row: rows[i],
		})
	}
}

// RowCount reports how many marks carry a source row.
func (ix *Index) RowCount() int { return len(ix.rows) }

// Panel implements the render package's Observer.
func (ix *Index) Panel(i int, area ir.Rect, x, y scale.Scale) {
	for len(ix.panels) <= i {
		ix.panels = append(ix.panels, Panel{})
	}
	ix.panels[i] = Panel{Area: area, X: x, Y: y}
	ix.panel, ix.layer, ix.label, ix.open = i, -1, "", false
}

// Layer implements the render package's Observer.
func (ix *Index) Layer(i int, label string) {
	ix.layer, ix.label, ix.open = i, label, true
}

// Panels reports the panels of the last watched render, in chart order.
func (ix *Index) Panels() []Panel { return ix.panels }

// PanelAt reports which panel contains a device point.
func (ix *Index) PanelAt(pt ir.Point) (int, bool) {
	for i, p := range ix.panels {
		if p.Area.Contains(pt) {
			return i, true
		}
	}
	return 0, false
}

// MarkCount reports how many marks were indexed. It is what a test asserts on
// and what a caller watching memory looks at.
func (ix *Index) MarkCount() int { return len(ix.marks) }

// DefaultTolerance is how near a pointer has to be to a mark to hit it, in
// device units.
//
// Twelve pixels is about a fingertip and about the radius within which a
// reader believes they are pointing at a thing. A smaller number makes a thin
// line unhittable; a much larger one makes two adjacent series
// indistinguishable.
const DefaultTolerance = 12

// At reports the mark nearest a device point, within tol device units. A tol
// of zero uses [DefaultTolerance].
//
// Marks are ranked by how specific they are and then by distance: a point the
// pointer is near beats a shape it is merely inside, and a shape beats a
// label. Pointing at a scatter marker that happens to sit inside a bar and
// under an annotation reports the marker, because the marker is the row the
// reader is asking about. Within one rank, later marks win ties — a later mark
// was drawn on top and is the one that can be seen.
//
// The search is linear in the marks on screen. That is the right shape here: a
// decimated chart draws thousands of marks, not millions, and a tree that has
// to be rebuilt every frame costs more to maintain than the scan costs to run.
func (ix *Index) At(pt ir.Point, tol float32) (Hit, bool) {
	if tol <= 0 {
		tol = DefaultTolerance
	}
	best := Hit{Distance: float32(math.Inf(1)), Row: -1}
	bestRank := len(ranked)
	var bestBounds ir.Rect
	found := false
	for _, m := range ix.marks {
		if !within(m.bounds, pt, tol) {
			continue
		}
		at, d, ok := m.nearest(ix.pts, pt, tol)
		if !ok {
			continue
		}
		r := rank(m.kind)
		if r > bestRank || (r == bestRank && d > best.Distance) {
			continue
		}
		bestRank = r
		bestBounds = m.bounds
		best, found = Hit{
			Panel: m.panel, Layer: m.layer, Series: m.label,
			Kind: m.kind, At: at, Distance: d, Row: -1,
		}, true
	}
	if !found {
		return Hit{}, false
	}
	if p := ix.panelOf(best.Panel); p != nil && p.X != nil && p.Y != nil {
		best.X, best.Y = p.X.Invert(best.At.X), p.Y.Invert(best.At.Y)
	}
	best.Row = ix.rowAt(best, bestBounds)
	return best, true
}

// rowAt finds the source row behind a hit: the nearest position its own layer
// reported, which for every geom that reports one mark per row is the hit's
// own position.
//
// The search is confined to the hit's layer, so two series crossing cannot
// take each other's rows, and it is bounded by the tolerance a hit already
// passed — a bar reports the middle of its end rather than the corner the
// pointer landed on, and that is the largest gap this has to cross.
func (ix *Index) rawRowAt(panel, layer int, at ir.Point, tol float32) int {
	best, bestD := -1, tol*tol
	for _, r := range ix.rows {
		if r.panel != panel || r.layer != layer {
			continue
		}
		dx, dy := r.at.X-at.X, r.at.Y-at.Y
		if d := dx*dx + dy*dy; d <= bestD {
			best, bestD = r.row, d
		}
	}
	return best
}

func (ix *Index) rowAt(h Hit, bounds ir.Rect) int {
	if !ix.track || len(ix.rows) == 0 {
		return -1
	}
	// An area is hit by containment rather than by proximity, so the position
	// its row was reported at may be as far away as the shape is large — a bar
	// reports the middle of its end and can be hit at the far corner.
	// Everything else was already within the tolerance the caller asked for.
	tol := float32(DefaultTolerance)
	if h.Kind != Vertex {
		tol = max(bounds.Dx(), bounds.Dy(), tol)
	}
	return ix.rawRowAt(h.Panel, h.Layer, h.At, tol)
}

// ranked orders the kinds from most specific to least. A vertex is a row; an
// area is a shape a row produced; a label is writing about one.
var ranked = [...]Kind{Vertex, Area, Label}

func rank(k Kind) int {
	for i, r := range ranked {
		if r == k {
			return i
		}
	}
	return len(ranked)
}

func (ix *Index) panelOf(i int) *Panel {
	if i < 0 || i >= len(ix.panels) {
		return nil
	}
	return &ix.panels[i]
}

// nearest returns the mark's closest point to pt and how far away it is.
//
// A vertex mark is hit by proximity to one of its points. An area is hit by
// containment — a bar is its interior, not its corners — and reports the
// nearest corner as the position, because a corner of a bar is where its value
// is and the middle of one is nowhere in particular.
func (m mark) nearest(pts []ir.Point, pt ir.Point, tol float32) (ir.Point, float32, bool) {
	switch m.kind {
	case Area, Label:
		if !m.bounds.Contains(pt) {
			return ir.Point{}, 0, false
		}
		at, _ := closest(pts[m.lo:m.hi], pt)
		return at, 0, true
	}
	at, d := closest(pts[m.lo:m.hi], pt)
	if d > tol {
		return ir.Point{}, 0, false
	}
	return at, d, true
}

func closest(pts []ir.Point, pt ir.Point) (ir.Point, float32) {
	best, bestD := ir.Point{}, float32(math.Inf(1))
	for _, p := range pts {
		dx, dy := p.X-pt.X, p.Y-pt.Y
		if d := dx*dx + dy*dy; d < bestD {
			best, bestD = p, d
		}
	}
	return best, float32(math.Sqrt(float64(bestD)))
}

func within(r ir.Rect, pt ir.Point, tol float32) bool {
	return pt.X >= r.Min.X-tol && pt.X <= r.Max.X+tol &&
		pt.Y >= r.Min.Y-tol && pt.Y <= r.Max.Y+tol
}

// Watch returns a Backend that draws into b and indexes what it draws.
//
// Only marks emitted inside a layer are indexed: the grid, the axes, the
// titles and the guides are furniture, and a pointer landing on a grid line
// has not landed on anything a reader would ask about.
func (ix *Index) Watch(b ir.Backend) ir.Backend { return &probe{ix: ix, b: b, at: ir.Identity} }

// probe is the Backend wrapper. Every method forwards first and indexes
// second, so that a chart drawn through a probe is the chart drawn without
// one — byte for byte, which is what makes one set of golden files cover both.
type probe struct {
	ix    *Index
	b     ir.Backend
	at    ir.Affine
	stack []ir.Affine
	sub   []ir.Point // one subpath, reused
}

// add records one mark, in device space.
//
// The points a drawing call carries are in whatever space the enclosing Push
// established, and a hit test is asked about the pointer — which is in the
// backend's. Layers are drawn under an identity transform today, so this is
// bookkeeping that costs nothing and stops being right the day a coordinate
// system pushes a transform of its own.
func (p *probe) add(kind Kind, pts []ir.Point, pad float32) {
	ix := p.ix
	if !ix.open || len(pts) == 0 {
		return
	}
	lo := len(ix.pts)
	if p.at.IsIdentity() {
		ix.pts = append(ix.pts, pts...)
	} else {
		for _, q := range pts {
			ix.pts = append(ix.pts, p.at.Apply(q))
		}
	}
	ix.marks = append(ix.marks, mark{
		panel: ix.panel, layer: ix.layer, kind: kind, label: ix.label,
		lo: lo, hi: len(ix.pts), bounds: bounds(ix.pts[lo:], pad),
	})
}

func (p *probe) Polyline(pts []ir.Point, style ir.Stroke) {
	p.b.Polyline(pts, style)
	p.add(Vertex, pts, style.Width/2)
}

func (p *probe) StrokePath(path *ir.Path, style ir.Stroke) {
	p.b.StrokePath(path, style)
	p.addSubpaths(Vertex, path, style.Width/2)
}

func (p *probe) FillPath(path *ir.Path, fill ir.Fill, rule ir.FillRule) {
	p.b.FillPath(path, fill, rule)
	p.addSubpaths(Area, path, 0)
}

// addSubpaths indexes one mark per subpath rather than one per call.
//
// A layer draws all its bars in a single path — one call per colour is what
// [geom] batches to — so a mark per call would mean the whole row of bars was
// one shape: pointing at the fourth bar would report whichever corner of
// whichever bar happened to be nearest. A subpath is one bar, one box, one
// closed shape, which is what the reader is pointing at.
func (p *probe) addSubpaths(kind Kind, path *ir.Path, pad float32) {
	if path == nil || path.Empty() || !p.ix.open {
		return
	}
	p.sub = p.sub[:0]
	path.Walk(func(op ir.PathOp, pts []ir.Point) {
		if op == ir.OpMoveTo && len(p.sub) > 0 {
			p.add(kind, p.sub, pad)
			p.sub = p.sub[:0]
		}
		p.sub = append(p.sub, pts...)
	})
	p.add(kind, p.sub, pad)
}

func (p *probe) Text(run ir.TextRun) {
	p.b.Text(run)
	if !p.ix.open {
		return
	}
	m := p.b.Measure(run)
	w, h := m.Advance, m.Height()
	p.add(Label, []ir.Point{
		{X: run.At.X, Y: run.At.Y - h},
		{X: run.At.X + w, Y: run.At.Y},
	}, 0)
}

func (p *probe) Markers(shape ir.Marker, at []ir.Point, style ir.MarkerStyle) {
	p.b.Markers(shape, at, style)
	p.add(Vertex, at, style.Size/2)
}

func (p *probe) Image(img image.Image, dst ir.Rect) {
	p.b.Image(img, dst)
	p.add(Area, []ir.Point{dst.Min, dst.Max}, 0)
}

func (p *probe) Push(clip *ir.Path, xform ir.Affine) {
	p.b.Push(clip, xform)
	p.stack = append(p.stack, p.at)
	p.at = p.at.Mul(xform)
}

func (p *probe) Pop() {
	p.b.Pop()
	if n := len(p.stack); n > 0 {
		p.at, p.stack = p.stack[n-1], p.stack[:n-1]
	}
}
func (p *probe) Measure(run ir.TextRun) ir.TextMetrics {
	return p.b.Measure(run)
}
func (p *probe) Flush() error { return p.b.Flush() }

// Describe forwards a description to the watched backend, when that backend
// can carry one.
//
// A wrapper hides the optional interfaces of what it wraps, and a probe is a
// wrapper: without this, watching a render would quietly cost the chart its
// accessible name, and only for the interactive charts — which are the ones
// that need it most. Nothing is indexed here; words are not marks.
func (p *probe) Describe(d ir.Description) {
	if s, ok := p.b.(ir.Semantics); ok {
		s.Describe(d)
	}
}

func bounds(pts []ir.Point, pad float32) ir.Rect {
	out := ir.Rect{Min: pts[0], Max: pts[0]}
	for _, q := range pts[1:] {
		out.Min.X = min(out.Min.X, q.X)
		out.Min.Y = min(out.Min.Y, q.Y)
		out.Max.X = max(out.Max.X, q.X)
		out.Max.Y = max(out.Max.Y, q.Y)
	}
	return out.Inset(-pad, -pad, -pad, -pad)
}

var (
	_ ir.Backend   = (*probe)(nil)
	_ ir.Semantics = (*probe)(nil)
)
