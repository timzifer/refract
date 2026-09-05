package stat

import (
	"image"
	"image/color"
	"math"
)

// Grid is a two-dimensional histogram: how many rows fall in each cell of a
// regular grid over a rectangle.
//
// It is the aggregate behind the density raster. A scatter of ten million
// points has no honest drawing as ten million markers — they overplot, the
// last one drawn wins, and the picture says more about row order than about
// the data. Counting per cell and painting the counts says how many rows are
// there, which is the thing the marks were standing in for.
//
// The zero Grid is unusable; call [Grid.Reset] first. Reset keeps the count
// buffer, so a chart redrawn every frame bins into the same memory.
type Grid struct {
	// Cols and Rows are the grid's shape.
	Cols, Rows int
	// X0, Y0, X1, Y1 are the region the grid covers. X0 may exceed X1, and Y0
	// may exceed Y1: a device-space Y axis runs downwards, and a grid over it
	// has to bin the same way round as the axis it covers.
	X0, Y0, X1, Y1 float64

	// Counts holds Cols*Rows cells in row-major order.
	Counts []uint32
	// Max is the busiest cell's count, and N the number of rows binned.
	Max uint32
	N   int
}

// Reset prepares g for cols by rows cells over the given rectangle, clearing
// any previous counts and reusing the existing buffer when it is large enough.
func (g *Grid) Reset(cols, rows int, x0, y0, x1, y1 float64) {
	g.Cols, g.Rows = max(cols, 1), max(rows, 1)
	g.X0, g.Y0, g.X1, g.Y1 = x0, y0, x1, y1
	g.Max, g.N = 0, 0
	n := g.Cols * g.Rows
	if cap(g.Counts) < n {
		g.Counts = make([]uint32, n)
		return
	}
	g.Counts = g.Counts[:n]
	clear(g.Counts)
}

// Cell returns the cell a position falls in, and whether it is inside the
// grid. A point on the far edge belongs to the last cell rather than to a cell
// that does not exist.
func (g *Grid) Cell(x, y float64) (col, row int, ok bool) {
	col, ok = axisCell(x, g.X0, g.X1, g.Cols)
	if !ok {
		return 0, 0, false
	}
	row, ok = axisCell(y, g.Y0, g.Y1, g.Rows)
	if !ok {
		return 0, 0, false
	}
	return col, row, true
}

func axisCell(v, lo, hi float64, n int) (int, bool) {
	if hi < lo {
		lo, hi = hi, lo
		i, ok := axisCell(v, lo, hi, n)
		return n - 1 - i, ok
	}
	if !(v >= lo) || !(v <= hi) { // written so that NaN falls outside
		return 0, false
	}
	if hi == lo {
		return 0, true
	}
	i := int(float64(n) * (v - lo) / (hi - lo))
	return min(i, n-1), true
}

// Add counts one position, reporting whether it landed inside the grid.
func (g *Grid) Add(x, y float64) bool {
	col, row, ok := g.Cell(x, y)
	if !ok {
		return false
	}
	i := row*g.Cols + col
	g.Counts[i]++
	g.Max = max(g.Max, g.Counts[i])
	g.N++
	return true
}

// At returns the count in one cell.
func (g *Grid) At(col, row int) uint32 {
	if col < 0 || row < 0 || col >= g.Cols || row >= g.Rows {
		return 0
	}
	return g.Counts[row*g.Cols+col]
}

// BinGrid counts every row of xs against ys into g, returning it.
//
// The coordinates are whatever g's rectangle is in — device space for a geom
// binning what it was about to draw, data space for a caller aggregating
// before it builds a chart.
//
// It is BinGrid rather than Bin because [Bin] is the one-dimensional
// histogram, which is what "bin" means without a qualifier. The three binners
// are named after what they fill: BinGrid, [BinHex] and Bin itself.
func BinGrid[F Float](g *Grid, xs, ys []F) *Grid {
	n := min(len(xs), len(ys))
	for i := range n {
		g.Add(float64(xs[i]), float64(ys[i]))
	}
	return g
}

// Scaling is how a cell's count becomes a fraction of the busiest cell.
type Scaling uint8

// The scalings. Log is the default because it is the one that shows anything:
// counts over a real point cloud span orders of magnitude, and under a linear
// mapping every cell but the densest few rounds to the background.
const (
	Log Scaling = iota
	Sqrt
	Linear
)

// Fraction maps a count onto [0, 1] against the busiest cell. An empty cell is
// 0 under every scaling, which is what keeps the background of a density
// raster empty rather than faintly painted.
func (g *Grid) Fraction(count uint32, s Scaling) float64 { return fraction(count, g.Max, s) }

// fraction is the shared implementation, so that a square cell and a hexagonal
// one are shaded by the same rule. See [Hex.Fraction].
func fraction(count, maxCount uint32, s Scaling) float64 {
	if count == 0 || maxCount == 0 {
		return 0
	}
	if count >= maxCount {
		return 1
	}
	c, m := float64(count), float64(maxCount)
	switch s {
	case Linear:
		return c / m
	case Sqrt:
		return math.Sqrt(c / m)
	default:
		return math.Log1p(c) / math.Log1p(m)
	}
}

// Raster paints the grid, one pixel per cell, and returns the image.
//
// paint is called once per non-empty cell with that cell's [Grid.Fraction];
// empty cells are left fully transparent so the plot's own background and grid
// read through. dst is reused when it is large enough, so a chart redrawn every
// frame paints into the same pixels; pass nil to have one allocated.
func (g *Grid) Raster(dst *image.NRGBA, s Scaling, paint func(t float64) color.NRGBA) *image.NRGBA {
	r := image.Rect(0, 0, g.Cols, g.Rows)
	if dst == nil || cap(dst.Pix) < 4*g.Cols*g.Rows {
		dst = image.NewNRGBA(r)
	} else {
		dst.Pix = dst.Pix[:4*g.Cols*g.Rows]
		dst.Stride = 4 * g.Cols
		dst.Rect = r
		clear(dst.Pix)
	}
	for row := range g.Rows {
		for col := range g.Cols {
			n := g.Counts[row*g.Cols+col]
			if n == 0 {
				continue
			}
			c := paint(g.Fraction(n, s))
			i := row*dst.Stride + col*4
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
	return dst
}
