package geom

import (
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Area fills the region between a series and a baseline, or — given [Y2] — the
// band between two series.
//
// The upper edge is stroked in the layer's full colour and the interior is
// filled with a faded version of it. A band drawn in one flat colour reads as
// a solid object; a band with a drawn edge reads as a series with uncertainty
// around it, which is what an area chart is for. Use [Opacity] to change the
// fill, [Fill] to set it outright.
func Area(src data.Source, opts ...Option) Geom {
	return &areaGeom{src: src, cfg: newConfig(opts)}
}

// areaFillOpacity is how much of the layer's colour an inherited area fill
// keeps. It is low enough that a grid line still reads through the fill and
// high enough that two overlapping bands are still two bands.
const areaFillOpacity = 0.25

type areaGeom struct {
	src data.Source
	cfg config
	s   series
	err error
}

func (g *areaGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	trainFinite(x, g.s.x)
	trainFinite(y, g.s.y)
	if g.s.y2 != nil {
		trainFinite(y, g.s.y2)
		return nil
	}
	// The baseline is part of the shape, so it has to be inside the domain or
	// the fill is clipped at an edge the reader cannot see.
	y.Train(g.cfg.baseline)
	return nil
}

func (g *areaGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	fill := g.cfg.fillFor(f, areaFillOpacity)
	stroke := ir.Stroke{
		Color: g.cfg.colorFor(f),
		Width: pick(g.cfg.width, f.Theme.LineWidth),
		Cap:   ir.CapRound,
		Join:  ir.JoinRound,
		Dash:  g.cfg.dash,
	}
	tension := float32(clamp01(g.cfg.tension))
	base := baselinePos(f, g.cfg.baseline)

	for _, seg := range segments(g.s, g.s.plottable(f.X, f.Y), g.cfg.missing) {
		top := project(seg, f)
		if len(top) < 2 {
			continue
		}
		if fill.A != 0 {
			var p ir.Path
			appendCurve(&p, top, tension, true)
			if seg.y2 != nil {
				appendCurve(&p, projectY2(seg, f), tension, false)
			} else {
				p.LineTo(top[len(top)-1].X, base)
				p.LineTo(top[0].X, base)
			}
			p.Close()
			b.FillPath(&p, ir.Solid(fill), ir.NonZero)
		}
		if !stroke.Visible() {
			continue
		}
		var edge ir.Path
		appendCurve(&edge, top, tension, true)
		b.StrokePath(&edge, stroke)
		if seg.y2 != nil {
			var lower ir.Path
			appendCurve(&lower, projectY2(seg, f), tension, true)
			b.StrokePath(&lower, stroke)
		}
	}
	return nil
}

func (g *areaGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil {
		return LegendEntry{}, false
	}
	return LegendEntry{
		Label: g.cfg.labelFor(),
		Color: g.cfg.colorFor(f),
		Kind:  SwatchBox,
	}, true
}
