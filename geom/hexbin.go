package geom

import (
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// Hexbin counts the rows falling in each cell of a hexagonal lattice over the
// plot area and draws a hexagon per populated cell, shaded by its count.
//
// It is the third answer to overplotting, beside decimation and the density
// raster: a scatter of a million rows says more about row order than about the
// data, because the last mark drawn wins. A hexbin says how many rows are
// there. It differs from the raster in what a cell *is* — a hexagon has six
// neighbours all the same distance away, where a square has four near and four
// far, so a cloud binned into squares grows faint crosses and diagonal seams
// that belong to the bins rather than to the data. And it differs in
// resolution: the raster paints a pixel per cell and this draws a mark per
// cell, so the cells are counted in the hundreds.
//
// [DensityCells] sets the cell radius in device units; the default is
// [DefaultHexRadius].
//
// # Why the lattice is in device space
//
// A hexagon is only a hexagon if its six neighbours really are equidistant, and
// on screen is where that has to be true — a lattice laid out in data space and
// then mapped through the axes comes out stretched by whatever aspect ratio the
// panel happens to have. So the binning happens in Build, where the rectangle
// is known, exactly as the density raster's does.
//
// The cost of that is one thing this layer deliberately does not have: a
// colourbar. The counts are not known until the plot rectangle is, and the
// guide column is measured before it — the same ordering ADR 0011 describes for
// decimation. So a hexbin shades from a faded version of its own colour to the
// full one, and says how many rows are behind a cell through a hit rather than
// through a key. Give it a [ColorBy] scale to shade through a ramp instead; the
// column named there is not read, because the quantity being coloured is the
// layer's own count.
func Hexbin(src data.Source, opts ...Option) Geom {
	return &hexGeom{src: src, cfg: newConfig(opts)}
}

// DefaultHexRadius is the circumradius of one hexbin cell in device units when
// the layer names none. Ten pixels puts a few hundred cells in a normal panel,
// which is enough to show structure and few enough that each hexagon still
// reads as a mark.
const DefaultHexRadius = 10

type hexGeom struct {
	src data.Source
	cfg config
	s   series
	err error
}

func (g *hexGeom) Train(x, y scale.Scale) error {
	// The colour column is deliberately not resolved: what a hexbin colours by
	// is its own count, and the name given to [ColorBy] is a label rather than
	// a column this layer reads.
	cfg := g.cfg
	cfg.colorCol = ""
	g.s, g.err = resolve(g.src, cfg, x, y)
	if g.err != nil {
		return g.err
	}
	if err := g.s.checkMissing(g.cfg, x, y); err != nil {
		return err
	}
	trainColumn(x, g.s.x)
	trainColumn(y, g.s.y)
	return nil
}

func (g *hexGeom) Build(b ir.Backend, f Frame) error {
	if g.err != nil {
		return g.err
	}
	area := f.Area
	col := g.cfg.colorFor(f)
	if g.cfg.fill != nil {
		col = *g.cfg.fill
	}
	if area.Empty() || (col.A == 0 && g.cfg.colorScale == nil) {
		return nil
	}

	sc := acquire(f)
	defer sc.release()

	radius := g.cfg.cellSize
	if radius <= 0 {
		radius = DefaultHexRadius
	}
	sc.hex.Reset(radius,
		float64(area.Min.X), float64(area.Min.Y),
		float64(area.Max.X), float64(area.Max.Y))

	cd := f.Coords()
	ok := sc.plottable(g.s, f.X, f.Y)
	for i := range g.s.x {
		if !ok[i] {
			continue
		}
		at := cd.Point(f.X.Map(g.s.x[i]), f.Y.Map(g.s.y[i]))
		sc.hex.Add(float64(at.X), float64(at.Y))
	}
	if sc.hex.N == 0 {
		return nil
	}
	sc.cells = sc.hex.Cells(sc.cells)

	// One path per distinct shade rather than one per cell: a lattice of a
	// thousand cells over a handful of shades is a handful of drawing calls,
	// which is the same bargain groupByColor makes for per-mark colour.
	for _, run := range g.shades(sc, col) {
		if run.color.A == 0 {
			continue
		}
		sc.fill.Reset()
		for _, i := range run.idx {
			c := sc.cells[i]
			sc.verts = stat.Vertices(sc.verts, c.X, c.Y, radius)
			for k, v := range sc.verts {
				if k == 0 {
					sc.fill.MoveTo(float32(v.X), float32(v.Y))
					continue
				}
				sc.fill.LineTo(float32(v.X), float32(v.Y))
			}
			sc.fill.Close()
		}
		b.FillPath(&sc.fill, ir.Solid(run.color), ir.NonZero)
	}
	return nil
}

// shades resolves each cell's colour and batches the cells by it.
//
// The scaling is logarithmic, for the reason the density raster's is: counts
// over a real point cloud span orders of magnitude, and under a linear mapping
// every cell but the densest few rounds to the background.
func (g *hexGeom) shades(sc *scratch, base ir.Color) []indexRun {
	sc.cols = grow(sc.cols, len(sc.cells))
	for i, c := range sc.cells {
		t := sc.hex.Fraction(c.Count, stat.Log)
		if g.cfg.colorScale != nil {
			// The ramp is read across its own domain rather than across the
			// counts. Nothing trained it on them — the count column does not
			// exist in the table — and training it here would write a scale two
			// panels may be drawing from at once.
			lo, hi := g.cfg.colorScale.Domain()
			sc.cols[i] = g.cfg.colorScale.Color(lo + t*(hi-lo))
			continue
		}
		// Without a ramp the cell is the layer's own colour, faded by how empty
		// it is — never to nothing, so that a cell holding one row is still a
		// mark rather than a hole.
		sc.cols[i] = ir.Fade(base, hexFloor+(1-hexFloor)*t)
	}
	return sc.groupByColorAt(sc.cols[:len(sc.cells)])
}

// hexFloor is the opacity of the emptiest populated cell. A cell with one row
// in it is evidence, and evidence drawn at two percent opacity is evidence
// nobody sees.
const hexFloor = 0.25

func (g *hexGeom) Legend(f Frame) (LegendEntry, bool) {
	if g.err != nil {
		return LegendEntry{}, false
	}
	col := g.cfg.colorFor(f)
	if g.cfg.fill != nil {
		col = *g.cfg.fill
	}
	return LegendEntry{Label: g.cfg.labelFor(), Color: col, Kind: SwatchBox}, true
}

func (g *hexGeom) Source() data.Source { return g.src }
func (g *hexGeom) Subset(rows []int) Geom {
	return &hexGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *hexGeom) Describe() Desc {
	d := g.cfg.describe(MarkHexbin)
	d.Source = g.src
	return d
}

var (
	_ Describer = (*hexGeom)(nil)
	_ Faceter   = (*hexGeom)(nil)
)
