package geom

import (
	"math"
	"sort"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Bar draws a rectangle per row, from a baseline to the row's Y value.
func Bar(src data.Source, opts ...Option) Geom {
	return &barGeom{src: src, cfg: newConfig(opts)}
}

type barGeom struct {
	src data.Source
	cfg config
	s   series
	err error
}

func (g *barGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg); err != nil {
		return err
	}
	trainFinite(x, g.s.x)
	trainFinite(y, g.s.y)
	// A bar is read as the area between the baseline and the value, so the
	// baseline must be in the domain or the chart lies about magnitude.
	y.Train(g.cfg.baseline)

	// Bars have width, so the outermost bars would be clipped in half by a
	// domain that stops at the data. Widen by half a slot on each side.
	half := g.slot() / 2 * g.widthFraction()
	if half > 0 {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, v := range g.s.x {
			if finite(v) {
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
		}
		if !math.IsInf(lo, 0) {
			x.Train(lo-half, hi+half)
		}
	}
	return nil
}

func (g *barGeom) widthFraction() float64 {
	if g.cfg.barWidth <= 0 || g.cfg.barWidth > 1 {
		return 0.8
	}
	return g.cfg.barWidth
}

// slot is the spacing between adjacent bars in data units: the smallest gap
// between distinct X values. Using the smallest rather than the average means
// bars never overlap on irregularly spaced data.
func (g *barGeom) slot() float64 {
	xs := make([]float64, 0, len(g.s.x))
	for _, v := range g.s.x {
		if finite(v) {
			xs = append(xs, v)
		}
	}
	if len(xs) < 2 {
		return 1
	}
	sort.Float64s(xs)
	gap := math.Inf(1)
	for i := 1; i < len(xs); i++ {
		if d := xs[i] - xs[i-1]; d > 0 && d < gap {
			gap = d
		}
	}
	if math.IsInf(gap, 0) {
		return 1
	}
	return gap
}

func (g *barGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	fill := g.cfg.colorFor(f)
	if g.cfg.fill != nil {
		fill = *g.cfg.fill
	}
	if fill.A == 0 {
		return nil
	}

	halfW := g.slot() * g.widthFraction() / 2
	base := f.Y.Map(g.cfg.baseline)

	var p ir.Path
	for i := range g.s.x {
		x, y := g.s.x[i], g.s.y[i]
		if !finite(x) || !finite(y) {
			continue
		}
		x0, x1 := f.X.Map(x-halfW), f.X.Map(x+halfW)
		if x1 < x0 {
			x0, x1 = x1, x0
		}
		top := f.Y.Map(y)
		y0, y1 := top, base
		if y1 < y0 {
			y0, y1 = y1, y0
		}
		p.Rect(ir.R(x0, y0, x1, y1))
	}
	if p.Empty() {
		return nil
	}
	b.FillPath(&p, ir.Solid(fill), ir.NonZero)

	if g.cfg.fill != nil && g.cfg.color != nil {
		b.StrokePath(&p, ir.Stroke{Color: *g.cfg.color, Width: pick(g.cfg.width, 1)})
	}
	return nil
}

func (g *barGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil {
		return LegendEntry{}, false
	}
	col := g.cfg.colorFor(f)
	if g.cfg.fill != nil {
		col = *g.cfg.fill
	}
	return LegendEntry{Label: g.cfg.labelFor(), Color: col, Kind: SwatchBox}, true
}
