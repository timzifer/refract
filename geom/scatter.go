package geom

import (
	"slices"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// Scatter draws one marker per row.
//
// Given [GroupBy] it draws one set of markers per series, each in its own
// colour and — where the theme asks for redundant encoding — its own shape.
//
// Given [SizeBy] it draws a circle per row, sized by area from a column: the
// bubble chart. That is a channel rather than a second geom, and it is circles
// rather than markers because the IR carries one marker style per drawing call
// — see [SizeBy].
func Scatter(src data.Source, opts ...Option) Geom {
	return &scatterGeom{src: src, cfg: newConfig(opts)}
}

type scatterGeom struct {
	src data.Source
	cfg config
	s   series
	gs  groups
	err error
}

func (g *scatterGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	trainColumn(x, g.s.x)
	trainColumn(y, g.s.y)
	g.cfg.trainColors(g.s)
	g.cfg.trainSizes(g.s)
	// Independent marks have nothing to stack: two points at one X are two
	// observations, not a total.
	g.err = g.gs.train(g.src, g.s, g.cfg, x, y, NoStack)
	return g.err
}

func (g *scatterGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	sc := acquire(f)
	defer sc.release()

	if g.gs.grouped() {
		return eachGroup(sc, &g.gs, g.s, func(seg series, grp int) error {
			return g.build(b, f, sc, seg, g.cfg.groupColor(f, &g.gs, grp), g.cfg.groupMarker(f, grp))
		})
	}
	return g.build(b, f, sc, g.s, g.cfg.colorFor(f), g.cfg.markerFor(f))
}

func (g *scatterGeom) build(b ir.Backend, f Frame, sc *scratch, s series, col ir.Color, marker ir.Marker) error {
	style := ir.MarkerStyle{
		Size: pick(g.cfg.size, f.Theme.MarkerSize),
		Fill: col,
	}
	if g.cfg.fill != nil {
		style.Fill = *g.cfg.fill
		style.Stroke = ir.Stroke{Color: col, Width: pick(g.cfg.width, 1)}
	}

	// Missing rows are simply not drawn. Unlike a line, a scatter has no
	// connectivity to break, so Gap and Interpolate are the same thing here.
	ok := sc.plottable(s, f.X, f.Y)

	if g.cfg.sizing(s) {
		return g.bubbles(b, f, sc, s, ok, col)
	}

	if mode, _ := g.cfg.reduction(shapeMarkers, s, f); mode == DensityRaster {
		// The raster takes the marker's own fill, so an explicit geom.Fill
		// still decides what colour the cloud is.
		return g.rasterize(b, f, sc, s, ok, style.Fill)
	}

	// The rows that survive are gathered into a contiguous pair first, so the
	// coord places the whole cloud in one call rather than one per marker.
	cd := f.Coords()
	sc.kx, sc.ky = grow(sc.kx, len(s.x))[:0], grow(sc.ky, len(s.x))[:0]
	rows := sc.rows[:0]
	for i := range s.x {
		if !ok[i] {
			continue
		}
		sc.kx = append(sc.kx, f.X.Map(s.x[i]))
		sc.ky = append(sc.ky, f.Y.Map(s.y[i]))
		rows = append(rows, i)
	}
	pts := cd.Points(grow(sc.pts, len(rows))[:0], sc.kx, sc.ky)
	sc.pts, sc.rows = pts, rows
	if len(pts) == 0 {
		return nil
	}
	// A scatter is the easy case: one mark per row, at the row's own position.
	f.Marks(pts, sc.sourceRows(s, rows))
	cols := sc.colorsFor(g.cfg, s, rows)
	if cols == nil {
		b.Markers(marker, pts, style)
		return nil
	}
	for _, run := range sc.groupByColor(pts, cols) {
		if run.color.A == 0 {
			continue
		}
		st := style
		st.Fill = run.color
		b.Markers(marker, run.pts, st)
	}
	return nil
}

// bubbles draws the layer as circles sized from a column.
//
// A bubble is a shape rather than a marker, and every bubble is its own subpath
// of one path per colour. Both halves of that matter. One call per colour is
// what [ir.Backend.Markers] cannot give a layer whose size varies per row,
// because it carries one style per call — the same argument ADR 0007 makes
// about colour, reaching the same answer. And one subpath per bubble is what
// lets a pointer land on the bubble it is inside rather than on the nearest
// centre, because hit-testing indexes a mark per subpath.
//
// The circles are drawn largest first, so that a small bubble inside a large
// one is still visible and still pointable at. Order of first appearance
// decides ties, so the drawing is reproducible.
func (g *scatterGeom) bubbles(b ir.Backend, f Frame, sc *scratch, s series, ok []bool, col ir.Color) error {
	cd := f.Coords()
	sc.kx, sc.ky = grow(sc.kx, len(s.x))[:0], grow(sc.ky, len(s.x))[:0]
	rows := sc.rows[:0]
	for i := range s.x {
		if !ok[i] || !finite(s.sz[i]) {
			continue
		}
		sc.kx = append(sc.kx, f.X.Map(s.x[i]))
		sc.ky = append(sc.ky, f.Y.Map(s.y[i]))
		rows = append(rows, i)
	}
	pts := cd.Points(grow(sc.pts, len(rows))[:0], sc.kx, sc.ky)
	sc.pts, sc.rows = pts, rows
	if len(pts) == 0 {
		return nil
	}

	sizes := grow(sc.sizes, len(rows))
	for i, r := range rows {
		sizes[i] = g.cfg.sizeScale.Size(s.sz[r])
	}
	sc.sizes = sizes

	// A bubble's row is its centre, which is where a reader points when they
	// mean "this one".
	f.Marks(pts, sc.sourceRows(s, rows))

	fill := g.cfg.fillOf(col, bubbleFillOpacity)
	stroke := ir.Stroke{Color: col, Width: pick(g.cfg.width, 1)}

	cols := sc.colorsFor(g.cfg, s, rows)
	if cols == nil {
		g.circles(b, sc, pts, sizes, sc.marksIn(len(pts)), fill, stroke)
		return nil
	}
	for _, run := range sc.groupByColorAt(cols) {
		if run.color.A == 0 {
			continue
		}
		g.circles(b, sc, pts, sizes, run.idx,
			g.cfg.fillOf(run.color, bubbleFillOpacity),
			ir.Stroke{Color: run.color, Width: stroke.Width})
	}
	return nil
}

// circles emits one path of bubbles: the marks named by idx.
//
// idx is reordered in place, largest first, so that a small bubble inside a
// large one is still visible and still pointable at. It is always a buffer this
// package owns — the scratch's own mark list, or one colour run's — never the
// row list the caller was just handed.
func (g *scatterGeom) circles(b ir.Backend, sc *scratch, pts []ir.Point, sizes []float32, idx []int, fill ir.Color, stroke ir.Stroke) {
	// A stable sort, so that two bubbles of one size keep the order the table
	// gave them and a render stays reproducible.
	slices.SortStableFunc(idx, func(a, c int) int {
		switch {
		case sizes[a] > sizes[c]:
			return -1
		case sizes[a] < sizes[c]:
			return 1
		}
		return 0
	})
	// Reserved once rather than grown per circle: a hundred thousand bubbles
	// appended into an empty path is a doubling ladder twenty rungs high, and a
	// pool emptied by a collection walks it again on the next frame.
	sc.fill.Reset()
	sc.fill.Grow(len(idx)*ir.CircleOps, len(idx)*ir.CirclePts)
	for _, i := range idx {
		if d := sizes[i]; d > 0 {
			sc.fill.Circle(pts[i], d/2)
		}
	}
	if sc.fill.Empty() {
		return
	}
	if fill.A != 0 {
		b.FillPath(&sc.fill, ir.Solid(fill), ir.NonZero)
	}
	if stroke.Visible() {
		b.StrokePath(&sc.fill, stroke)
	}
}

// bubbleFillOpacity is how much of the layer's colour a bubble's interior
// keeps. Bubbles overlap by construction — that is what a size channel does to
// a cloud — so a solid fill would hide whatever is behind the largest of them,
// and the outline is what carries the size.
const bubbleFillOpacity = 0.55

// rasterize draws the layer as a density image instead of as markers.
//
// Every cell of the plot area gets the layer's colour at an opacity set by how
// many rows landed in it, on a logarithmic scaling — counts over a real point
// cloud span orders of magnitude, and under a linear one every cell but the
// densest few would round away to nothing. An empty cell is left transparent,
// so the plot's own grid still reads through.
func (g *scatterGeom) rasterize(b ir.Backend, f Frame, sc *scratch, s series, ok []bool, col ir.Color) error {
	area := f.Area
	if area.Empty() || col.A == 0 {
		return nil
	}
	cell := g.cfg.cellSize
	if cell <= 0 {
		cell = 1
	}
	sc.grid.Reset(
		max(int(float64(area.Dx())/cell), 1),
		max(int(float64(area.Dy())/cell), 1),
		float64(area.Min.X), float64(area.Min.Y),
		float64(area.Max.X), float64(area.Max.Y),
	)
	cd := f.Coords()
	for i := range s.x {
		if !ok[i] {
			continue
		}
		at := cd.Point(f.X.Map(s.x[i]), f.Y.Map(s.y[i]))
		sc.grid.Add(float64(at.X), float64(at.Y))
	}
	if sc.grid.N == 0 {
		return nil
	}
	sc.img = sc.grid.Raster(sc.img, stat.Log, func(t float64) ir.Color {
		return ir.Fade(col, t)
	})
	b.Image(sc.img, area)
	return nil
}

func (g *scatterGeom) ColorGuide() (ColorGuide, bool) {
	return g.cfg.colorGuide(g.s, g.err)
}

func (g *scatterGeom) SizeGuide() (SizeGuide, bool) {
	return g.cfg.sizeGuide(g.s, g.err)
}

func (g *scatterGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.legends(f, &g.gs, g.s, SwatchMarker))
}

func (g *scatterGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil || g.cfg.varying(g.s) {
		return LegendEntry{}, false
	}
	return LegendEntry{
		Label:  g.cfg.labelFor(),
		Color:  g.cfg.colorFor(f),
		Kind:   SwatchMarker,
		Marker: g.cfg.markerFor(f),
	}, true
}
