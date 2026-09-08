package geom

import (
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// QQ draws a normal quantile-quantile plot of the X column. The horizontal
// axis holds standard-normal theoretical quantiles; the vertical axis holds
// the ordered observations in their original units. Y is not read. A straight
// pattern indicates agreement up to location and scale; no fit or reference
// line is inferred. GroupBy draws one comparison per series.
//
// The sample is summarised in Train and both axes learn the computed pairs.
// For another theoretical distribution, use stat.QQ with a quantile function
// and draw its pairs with Scatter. Quantiles are summaries, so this mark does
// not claim source-row identity. Missing Interpolate has the same meaning as
// Gap: missing observations are omitted before ranking.
func QQ(src data.Source, opts ...Option) Geom {
	return &qqGeom{ecdfGeom: ecdfGeom{src: src, cfg: newConfig(opts)}}
}

// The ECDF's retained sample/group buffers are shared machinery: both sort
// each series once during training. Drawing, axis training and descriptions
// are the parts that differ, and none is delegated to an ECDF drawing method.
type qqGeom struct{ ecdfGeom }

func (g *qqGeom) Train(x, y scale.Scale) error {
	if _, ok := x.(scale.Categorical); ok {
		g.err = ErrNotContinuous
		return g.err
	}
	if _, ok := y.(scale.Categorical); ok {
		g.err = ErrNotContinuous
		return g.err
	}
	// Observations live on Y, even though X names the input column, as it does
	// for the other one-column distribution marks.
	g.s, g.err = resolveOne(g.src, g.cfg, y)
	if g.err != nil {
		return g.err
	}
	if g.err = g.s.checkMissing(g.cfg, y, y); g.err != nil {
		return g.err
	}
	if g.err = g.gs.train(g.src, g.s, g.cfg, nil, nil, NoStack); g.err != nil {
		return g.err
	}
	// Ranking includes every finite observation, even a zero on a log Y axis:
	// changing display scales must not change the estimated distribution.
	g.accumulate(nil, appendNormalQQ)
	for _, curve := range g.curves {
		g.vals = grow(g.vals, len(curve))
		for i, p := range curve {
			g.vals[i] = p.X
		}
		x.Train(g.vals...)
		for i, p := range curve {
			g.vals[i] = p.Y
		}
		y.Train(g.vals...)
	}
	return nil
}

func appendNormalQQ(dst []stat.Point, sorted []float64) []stat.Point {
	return stat.AppendQQ(dst, sorted, nil)
}

func (g *qqGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	sc := acquire(f)
	defer sc.release()
	cd := f.Coords()
	for i, curve := range g.curves {
		sc.fx, sc.fy = grow(sc.fx, len(curve)), grow(sc.fy, len(curve))
		for j, p := range curve {
			sc.fx[j], sc.fy[j] = p.X, p.Y
		}
		ok := sc.plottable(series{x: sc.fx, y: sc.fy}, f.X, f.Y)
		sc.kx, sc.ky = grow(sc.kx, len(curve))[:0], grow(sc.ky, len(curve))[:0]
		for j, p := range curve {
			if !ok[j] {
				continue
			}
			sc.kx, sc.ky = append(sc.kx, f.X.Map(p.X)), append(sc.ky, f.Y.Map(p.Y))
		}
		sc.pts = cd.Points(grow(sc.pts, len(sc.kx))[:0], sc.kx, sc.ky)
		if len(sc.pts) == 0 {
			continue
		}
		col, shape := g.cfg.colorFor(f), g.cfg.markerFor(f)
		if g.gs.grouped() {
			col, shape = g.cfg.groupColor(f, &g.gs, g.groups[i]), g.cfg.groupMarker(f, g.groups[i])
		}
		style := ir.MarkerStyle{Size: pick(g.cfg.size, f.Theme.MarkerSize), Fill: col}
		if g.cfg.fill != nil {
			style.Fill = *g.cfg.fill
			style.Stroke = ir.Stroke{Color: col, Width: pick(g.cfg.width, 1)}
		}
		b.Markers(shape, sc.pts, style)
	}
	return nil
}

func (g *qqGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil {
		return LegendEntry{}, false
	}
	return LegendEntry{Label: g.cfg.labelForX(), Color: g.cfg.colorFor(f), Kind: SwatchMarker, Marker: g.cfg.markerFor(f)}, true
}

func (g *qqGeom) Legends(f Frame) []LegendEntry {
	if g.err != nil {
		return nil
	}
	return LegendsOr(g, f, g.cfg.legends(f, &g.gs, g.s, SwatchMarker))
}

func (g *qqGeom) Subset(rows []int) Geom {
	return &qqGeom{ecdfGeom: ecdfGeom{src: data.Rows(g.src, rows), cfg: g.cfg}}
}
func (g *qqGeom) Describe() Desc { d := g.cfg.describe(MarkQQ); d.Source = g.src; return d }
