package geom

import (
	"math"
	"sort"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Boxplot summarises the distribution of the Y column within each distinct X
// value: a box spanning the interquartile range, a line at the median,
// whiskers reaching to the furthest observation within [Whisker] times the IQR,
// and a marker for every observation beyond them.
//
// The X column is the grouping key, so a boxplot wants many rows per X value —
// typically a categorical column against a [scale.Ordinal] axis, which also
// gives every box the same width. On a continuous axis the width comes from
// the closest pair of groups, as it does for a bar.
//
// Whiskers stop at an observation, never at the theoretical fence. A whisker
// drawn out to 1.5·IQR when the data stops well short of it would be claiming
// a reading that does not exist.
func Boxplot(src data.Source, opts ...Option) Geom {
	return &boxGeom{src: src, cfg: newConfig(opts)}
}

type boxGeom struct {
	src    data.Source
	cfg    config
	s      series
	groups []boxGroup
	err    error
}

// boxGroup is one summarised distribution.
type boxGroup struct {
	at                 float64 // the group's X position
	q1, median, q3     float64
	loWhisker, hiWhisk float64
	outliers           []float64
}

func (g *boxGeom) Train(x, y scale.Scale) error {
	g.s, g.err = resolve(g.src, g.cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	g.groups = summarise(g.s, x, y, g.cfg.whisker)

	for _, grp := range g.groups {
		x.Train(grp.at)
		y.Train(grp.loWhisker, grp.hiWhisk, grp.q1, grp.q3, grp.median)
		y.Train(grp.outliers...)
	}
	if _, band := x.(scale.Band); band {
		return nil
	}
	// Boxes have width, so the outermost ones would be clipped in half by a
	// domain that stops at the data.
	half := g.slot() / 2 * g.widthFraction()
	if half > 0 && len(g.groups) > 0 {
		x.Train(g.groups[0].at-half, g.groups[len(g.groups)-1].at+half)
	}
	return nil
}

func (g *boxGeom) widthFraction() float64 {
	if g.cfg.barWidth <= 0 || g.cfg.barWidth > 1 {
		return 0.8
	}
	return g.cfg.barWidth
}

func (g *boxGeom) slot() float64 {
	at := make([]float64, len(g.groups))
	for i, grp := range g.groups {
		at[i] = grp.at
	}
	return smallestGap(at)
}

func (g *boxGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	stroke := ir.Stroke{
		Color: g.cfg.colorFor(f),
		Width: pick(g.cfg.width, 1),
		Cap:   ir.CapButt,
	}
	fill := g.cfg.fillFor(f, boxFillOpacity)
	half := g.slot() * g.widthFraction() / 2

	for _, grp := range g.groups {
		x0, x1 := markSpan(f, grp.at, half)
		mid := (x0 + x1) / 2
		q1, q3 := f.Y.Map(grp.q1), f.Y.Map(grp.q3)
		if q3 > q1 {
			q1, q3 = q3, q1
		}

		var box ir.Path
		box.Rect(ir.R(x0, q3, x1, q1))
		if fill.A != 0 {
			b.FillPath(&box, ir.Solid(fill), ir.NonZero)
		}
		if !stroke.Visible() {
			continue
		}
		b.StrokePath(&box, stroke)

		// The median is the one number a reader takes off a boxplot without
		// measuring, so it is drawn heavier than the box around it.
		med := f.Y.Map(grp.median)
		b.Polyline([]ir.Point{{X: x0, Y: med}, {X: x1, Y: med}}, ir.Stroke{
			Color: stroke.Color,
			Width: stroke.Width * 2,
			Cap:   ir.CapButt,
		})

		// Whiskers, each with a cap a quarter of the box wide on each side.
		cap0, cap1 := mid-(x1-x0)/4, mid+(x1-x0)/4
		for _, w := range [2]float64{grp.loWhisker, grp.hiWhisk} {
			end := f.Y.Map(w)
			from := q1
			if w > grp.median {
				from = q3
			}
			b.Polyline([]ir.Point{{X: mid, Y: from}, {X: mid, Y: end}}, stroke)
			b.Polyline([]ir.Point{{X: cap0, Y: end}, {X: cap1, Y: end}}, stroke)
		}

		if !g.cfg.outliers || len(grp.outliers) == 0 {
			continue
		}
		pts := make([]ir.Point, 0, len(grp.outliers))
		for _, v := range grp.outliers {
			pts = append(pts, ir.Point{X: mid, Y: f.Y.Map(v)})
		}
		b.Markers(g.cfg.marker, pts, ir.MarkerStyle{
			Size: pick(g.cfg.size, f.Theme.MarkerSize*0.7),
			Fill: stroke.Color,
		})
	}
	return nil
}

// boxFillOpacity is how much of the layer's colour an inherited box fill
// keeps. A box is mostly read from its edges, so the fill only has to separate
// it from the grid behind it.
const boxFillOpacity = 0.3

func (g *boxGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil {
		return LegendEntry{}, false
	}
	return LegendEntry{
		Label: g.cfg.labelFor(),
		Color: g.cfg.colorFor(f),
		Kind:  SwatchBox,
	}, true
}

// summarise groups the rows by X value and reduces each group to its
// five-number summary plus outliers. Groups come out ordered by position, so
// the drawing order does not depend on map iteration.
func summarise(s series, x, y scale.Scale, k float64) []boxGroup {
	if k <= 0 {
		k = 1.5
	}
	byX := map[float64][]float64{}
	var order []float64
	ok := s.plottable(x, y)
	for i := range s.x {
		if !ok[i] {
			continue
		}
		if _, seen := byX[s.x[i]]; !seen {
			order = append(order, s.x[i])
		}
		byX[s.x[i]] = append(byX[s.x[i]], s.y[i])
	}
	sort.Float64s(order)

	out := make([]boxGroup, 0, len(order))
	for _, at := range order {
		vs := byX[at]
		sort.Float64s(vs)
		g := boxGroup{
			at:     at,
			q1:     quantile(vs, 0.25),
			median: quantile(vs, 0.5),
			q3:     quantile(vs, 0.75),
		}
		iqr := g.q3 - g.q1
		lo, hi := g.q1-k*iqr, g.q3+k*iqr
		g.loWhisker, g.hiWhisk = g.median, g.median
		for _, v := range vs {
			switch {
			case v < lo || v > hi:
				g.outliers = append(g.outliers, v)
			case v < g.loWhisker:
				g.loWhisker = v
			case v > g.hiWhisk:
				g.hiWhisk = v
			}
		}
		out = append(out, g)
	}
	return out
}

// quantile returns the p'th quantile of an ascending slice by linear
// interpolation between the two closest order statistics.
//
// This is the definition R calls type 7 and NumPy uses by default. Choosing it
// deliberately matters: the nine standard definitions disagree by a visible
// amount on the small samples a boxplot is usually drawn from, and a box that
// does not match the reader's own analysis is worse than no box.
func quantile(sorted []float64, p float64) float64 {
	n := len(sorted)
	switch n {
	case 0:
		return math.NaN()
	case 1:
		return sorted[0]
	}
	h := p * float64(n-1)
	i := int(math.Floor(h))
	if i >= n-1 {
		return sorted[n-1]
	}
	return sorted[i] + (h-float64(i))*(sorted[i+1]-sorted[i])
}
