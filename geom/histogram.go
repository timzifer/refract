package geom

import (
	"sort"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// Histogram counts the X column into bins and draws a bar per bin.
//
// It is the first geom whose Y axis is not in the data: the counts are the
// layer's own, so the axis is trained on them and the Y column is not read at
// all. That is the same arrangement [Boxplot] already has, where the quantiles
// rather than the observations decide the domain.
//
// The bins are contiguous by construction, so the bars touch. That is not a
// missing gap: a histogram's bars are an unbroken division of the axis, and
// separating them would make it read as a bar chart of categories, which is a
// different claim about the data. [BarWidth] is therefore one of the options
// this layer ignores.
//
// [Bins] sets the count and [BinRange] the interval. Left alone, the layer bins
// over the data's own extent with the Freedman–Diaconis rule, falling back to
// Sturges's where the column has no interquartile range to measure.
//
// # Groups
//
// A histogram ignores [GroupBy]. Two distributions drawn as two overlapping
// histograms hide each other wherever they agree, which is exactly where the
// comparison is; [Violin], [Ridgeline] and [ECDF] are the three marks that
// answer that question without overplotting, and each of them takes the series
// column.
func Histogram(src data.Source, opts ...Option) Geom {
	return &histGeom{src: src, cfg: newConfig(opts)}
}

type histGeom struct {
	src     data.Source
	cfg     config
	vals    []float64
	buckets []stat.Bucket
	sorted  []float64 // the buffer the bin rule is measured out of
	err     error
}

func (g *histGeom) Train(x, y scale.Scale) error {
	g.vals, g.err = column(g.src, g.cfg.xcol, x)
	if g.err != nil {
		return g.err
	}
	g.buckets = stat.AppendBin(g.buckets, g.vals, g.cfg.binLo, g.cfg.binHi, g.binCount())
	for _, b := range g.buckets {
		x.Train(b.Lo, b.Hi)
		y.Train(float64(b.Count))
	}
	// A bar is read as the distance from the baseline, so the baseline has to
	// be inside the domain or the chart lies about magnitude.
	y.Train(g.cfg.baseline)
	return nil
}

// binCount resolves how many bins to use.
//
// The rule is measured out of a buffer the layer keeps, for the reason
// [barGeom.gap] is: Train runs on every frame, and sorting a copy of the column
// per frame is most of what a large histogram would otherwise allocate.
func (g *histGeom) binCount() int {
	if g.cfg.bins > 0 {
		return g.cfg.bins
	}
	g.sorted = g.sorted[:0]
	for _, v := range g.vals {
		if finite(v) {
			g.sorted = append(g.sorted, v)
		}
	}
	sort.Float64s(g.sorted)
	if n := stat.FreedmanDiaconis(g.sorted); n > 0 {
		return n
	}
	return stat.Sturges(len(g.sorted))
}

func (g *histGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	fill := g.cfg.colorFor(f)
	if g.cfg.fill != nil {
		fill = *g.cfg.fill
	}
	if g.cfg.opacity >= 0 {
		fill = ir.Fade(fill, clamp01(g.cfg.opacity))
	}
	if fill.A == 0 || len(g.buckets) == 0 {
		return nil
	}

	sc := acquire(f)
	defer sc.release()

	cd := f.Coords()
	base := baselinePos(f, g.cfg.baseline)
	sc.fill.Reset()
	for _, bu := range g.buckets {
		if bu.Count == 0 {
			continue
		}
		x0, x1 := f.X.Map(bu.Lo), f.X.Map(bu.Hi)
		if x1 < x0 {
			x0, x1 = x1, x0
		}
		y0, y1 := f.Y.Map(float64(bu.Count)), base
		if y1 < y0 {
			y0, y1 = y1, y0
		}
		area(&sc.fill, cd, ir.R(x0, y0, x1, y1))
	}
	if sc.fill.Empty() {
		return nil
	}
	b.FillPath(&sc.fill, ir.Solid(fill), ir.NonZero)

	// The outline is drawn when the caller named both a fill and a stroke,
	// which is the same rule [Bar] follows — and it is what separates two
	// adjacent bars of one colour when a reader wants the edges.
	if g.cfg.fill != nil && g.cfg.color != nil {
		b.StrokePath(&sc.fill, ir.Stroke{Color: *g.cfg.color, Width: pick(g.cfg.width, 1)})
	}
	return nil
}

func (g *histGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil {
		return LegendEntry{}, false
	}
	col := g.cfg.colorFor(f)
	if g.cfg.fill != nil {
		col = *g.cfg.fill
	}
	return LegendEntry{Label: g.cfg.labelForX(), Color: col, Kind: SwatchBox}, true
}

func (g *histGeom) Source() data.Source { return g.src }
func (g *histGeom) Subset(rows []int) Geom {
	return &histGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *histGeom) Describe() Desc {
	d := g.cfg.describe(MarkHistogram)
	d.Source = g.src
	return d
}

var (
	_ Describer = (*histGeom)(nil)
	_ Faceter   = (*histGeom)(nil)
)
