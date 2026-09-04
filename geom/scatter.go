package geom

import (
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// Scatter draws one marker per row.
func Scatter(src data.Source, opts ...Option) Geom {
	return &scatterGeom{src: src, cfg: newConfig(opts)}
}

type scatterGeom struct {
	src data.Source
	cfg config
	s   series
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
	return nil
}

func (g *scatterGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	col := g.cfg.colorFor(f)
	style := ir.MarkerStyle{
		Size: pick(g.cfg.size, f.Theme.MarkerSize),
		Fill: col,
	}
	if g.cfg.fill != nil {
		style.Fill = *g.cfg.fill
		style.Stroke = ir.Stroke{Color: col, Width: pick(g.cfg.width, 1)}
	}

	sc := acquire()
	defer sc.release()

	// Missing rows are simply not drawn. Unlike a line, a scatter has no
	// connectivity to break, so Gap and Interpolate are the same thing here.
	ok := sc.plottable(g.s, f.X, f.Y)

	if mode, _ := g.cfg.reduction(shapeMarkers, g.s, f); mode == DensityRaster {
		// The raster takes the marker's own fill, so an explicit geom.Fill
		// still decides what colour the cloud is.
		return g.rasterize(b, f, sc, ok, style.Fill)
	}

	pts := sc.pts[:0]
	rows := sc.rows[:0]
	for i := range g.s.x {
		if !ok[i] {
			continue
		}
		pts = append(pts, ir.Point{X: f.X.Map(g.s.x[i]), Y: f.Y.Map(g.s.y[i])})
		rows = append(rows, i)
	}
	sc.pts, sc.rows = pts, rows
	if len(pts) == 0 {
		return nil
	}
	cols := sc.colorsFor(g.cfg, g.s, rows)
	if cols == nil {
		b.Markers(g.cfg.marker, pts, style)
		return nil
	}
	for _, run := range sc.groupByColor(pts, cols) {
		if run.color.A == 0 {
			continue
		}
		s := style
		s.Fill = run.color
		b.Markers(g.cfg.marker, run.pts, s)
	}
	return nil
}

// rasterize draws the layer as a density image instead of as markers.
//
// Every cell of the plot area gets the layer's colour at an opacity set by how
// many rows landed in it, on a logarithmic scaling — counts over a real point
// cloud span orders of magnitude, and under a linear one every cell but the
// densest few would round away to nothing. An empty cell is left transparent,
// so the plot's own grid still reads through.
func (g *scatterGeom) rasterize(b ir.Backend, f Frame, sc *scratch, ok []bool, col ir.Color) error {
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
	for i := range g.s.x {
		if !ok[i] {
			continue
		}
		sc.grid.Add(float64(f.X.Map(g.s.x[i])), float64(f.Y.Map(g.s.y[i])))
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

func (g *scatterGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil || g.cfg.varying(g.s) {
		return LegendEntry{}, false
	}
	return LegendEntry{
		Label:  g.cfg.labelFor(),
		Color:  g.cfg.colorFor(f),
		Kind:   SwatchMarker,
		Marker: g.cfg.marker,
	}, true
}
