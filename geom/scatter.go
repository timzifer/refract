package geom

import (
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
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
	trainFinite(x, g.s.x)
	trainFinite(y, g.s.y)
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

	// Missing rows are simply not drawn. Unlike a line, a scatter has no
	// connectivity to break, so Gap and Interpolate are the same thing here.
	ok := g.s.plottable(f.X, f.Y)
	pts := make([]ir.Point, 0, len(g.s.x))
	rows := make([]int, 0, len(g.s.x))
	for i := range g.s.x {
		if !ok[i] {
			continue
		}
		pts = append(pts, ir.Point{X: f.X.Map(g.s.x[i]), Y: f.Y.Map(g.s.y[i])})
		rows = append(rows, i)
	}
	if len(pts) == 0 {
		return nil
	}
	cols := g.cfg.colorsFor(f, g.s, rows)
	if cols == nil {
		b.Markers(g.cfg.marker, pts, style)
		return nil
	}
	for _, run := range groupByColor(pts, cols) {
		if run.color.A == 0 {
			continue
		}
		s := style
		s.Fill = run.color
		b.Markers(g.cfg.marker, run.pts, s)
	}
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
