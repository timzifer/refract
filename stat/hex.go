package stat

import "math"

// Hex is a hexagonal lattice over a rectangle: how many rows fall in each
// hexagonal cell.
//
// It is [Grid] with a better tiling. A square grid has four neighbours at one
// distance and four at another, so a cloud binned into it grows faint crosses
// and diagonal seams that are artefacts of the bins rather than features of the
// data; a hexagon has six neighbours all the same distance away, which is why
// this is the aggregate a hexbin chart is made of. The other difference is what
// is drawn: a density raster paints one pixel per cell, and a hexbin draws a
// mark per cell, so the cells are counted in the hundreds rather than in the
// hundreds of thousands.
//
// The lattice is pointy-topped: a cell is 2·Radius tall and √3·Radius wide,
// rows are 1.5·Radius apart, and odd rows are offset half a cell to the right.
//
// The zero Hex is unusable; call [Hex.Reset] first. Reset keeps the count
// buffer, so a chart redrawn every frame bins into the same memory.
type Hex struct {
	// Radius is a cell's circumradius: the distance from its centre to a
	// vertex, and half its height.
	Radius float64
	// Cols and Rows are the lattice's shape.
	Cols, Rows int
	// X0 and Y0 are the centre of cell (0, 0).
	X0, Y0 float64

	// MinX, MinY, MaxX and MaxY are the rectangle the lattice covers,
	// normalised so that the minimum is the lower corner on both axes. A
	// device-space Y axis runs downwards and a caller may hand the two corners
	// over in either order; a hexagonal lattice is symmetric about both, so
	// which way round it was given changes nothing but which points are inside.
	MinX, MinY, MaxX, MaxY float64

	// Counts holds Cols*Rows cells in row-major order.
	Counts []uint32
	// Max is the busiest cell's count, and N the number of rows binned.
	Max uint32
	N   int
}

// Cell is one populated hexagon: where it is and how many rows landed in it.
type Cell struct {
	Col, Row int
	X, Y     float64
	Count    uint32
}

// Reset prepares h for cells of the given radius over the rectangle with
// corners (x0, y0) and (x1, y1), clearing any previous counts and reusing the
// existing buffer when it is large enough.
//
// A radius of zero or less is meaningless and is replaced with one, which
// draws a lattice of unit cells rather than dividing by nothing.
func (h *Hex) Reset(radius, x0, y0, x1, y1 float64) {
	if !(radius > 0) {
		radius = 1
	}
	h.Radius = radius
	h.MinX, h.MaxX = math.Min(x0, x1), math.Max(x0, x1)
	h.MinY, h.MaxY = math.Min(y0, y1), math.Max(y0, y1)
	h.Max, h.N = 0, 0

	dx, dy := h.dx(), h.dy()
	// The lattice origin sits one cell outside the rectangle's lower corner, so
	// that the half-cell offset of an odd row and the nearest-centre refinement
	// below both stay inside the array without a special case at the edge.
	h.X0, h.Y0 = h.MinX-dx, h.MinY-dy
	h.Cols = int(math.Ceil((h.MaxX-h.MinX)/dx)) + 3
	h.Rows = int(math.Ceil((h.MaxY-h.MinY)/dy)) + 3

	n := h.Cols * h.Rows
	if cap(h.Counts) < n {
		h.Counts = make([]uint32, n)
		return
	}
	h.Counts = h.Counts[:n]
	clear(h.Counts)
}

// dx and dy are the lattice spacings: the horizontal distance between two cells
// of one row, and the vertical distance between two rows.
func (h *Hex) dx() float64 { return math.Sqrt(3) * h.Radius }
func (h *Hex) dy() float64 { return 1.5 * h.Radius }

// Cell returns the cell a position falls in, and whether it is inside the
// lattice.
//
// The nearest centre is found the way d3-hexbin finds it: round to the nearest
// row, round to the nearest column of that row, and then — only when the point
// is in the band where two rows' cells interlock — compare the candidate
// against the one diagonally across from it. That band is the middle third of
// a row's height, which is where |py - pj|·3 > 1.
//
// The comparison is not d3's, and the difference is load-bearing. Both work in
// lattice coordinates, where a column step is 1 and a row step is 1 — but a
// column step is √3·Radius on screen and a row step is 1.5·Radius, so comparing
// px² + py² there measures a distance in two different units and hands some
// points to a cell that is not their nearest. The vertical term therefore
// carries (dy/dx)², which is exactly 3/4, and [TestEveryHexPointLandsInItsNearestCell]
// is what holds it: without it the picture grows seams along every second row.
func (h *Hex) Cell(x, y float64) (col, row int, ok bool) {
	if !(x >= h.MinX) || !(x <= h.MaxX) || !(y >= h.MinY) || !(y <= h.MaxY) {
		// Written so that NaN falls outside.
		return 0, 0, false
	}
	px := (x - h.X0) / h.dx()
	py := (y - h.Y0) / h.dy()

	pj := math.Round(py)
	px -= float64(int(pj)&1) / 2
	pi := math.Round(px)
	py1 := py - pj

	if math.Abs(py1)*3 > 1 {
		px1 := px - pi
		pi2 := pi + step(px < pi)/2
		pj2 := pj + step(py < pj)
		px2, py2 := px-pi2, py-pj2
		if px1*px1+rowWeight*py1*py1 > px2*px2+rowWeight*py2*py2 {
			// The half-cell correction uses the parity of the row being left,
			// because that is the row pi was measured in.
			pi = pi2 + step(int(pj)&1 == 0)/2
			pj = pj2
		}
	}

	col, row = int(pi), int(pj)
	if col < 0 || row < 0 || col >= h.Cols || row >= h.Rows {
		return 0, 0, false
	}
	return col, row, true
}

// rowWeight converts a row step into a column step, squared: (1.5·r)² over
// (√3·r)² is 3/4, whatever the radius. It is what makes the comparison in
// [Hex.Cell] a distance rather than a mixture of two units.
const rowWeight = 0.75

// step is -1 or 1, which is the only arithmetic the nearest-centre refinement
// needs a branch for.
func step(negative bool) float64 {
	if negative {
		return -1
	}
	return 1
}

// Center returns the centre of one cell.
func (h *Hex) Center(col, row int) (x, y float64) {
	return h.X0 + (float64(col)+float64(row&1)/2)*h.dx(), h.Y0 + float64(row)*h.dy()
}

// Add counts one position, reporting whether it landed inside the lattice.
func (h *Hex) Add(x, y float64) bool {
	col, row, ok := h.Cell(x, y)
	if !ok {
		return false
	}
	i := row*h.Cols + col
	h.Counts[i]++
	h.Max = max(h.Max, h.Counts[i])
	h.N++
	return true
}

// At returns the count in one cell.
func (h *Hex) At(col, row int) uint32 {
	if col < 0 || row < 0 || col >= h.Cols || row >= h.Rows {
		return 0
	}
	return h.Counts[row*h.Cols+col]
}

// Cells appends the populated cells to dst, in row-major order, and returns it.
// dst is truncated first.
//
// Row-major and not "whichever order a map produced them in": a parallel render
// has to be byte identical to a serial one, so the order marks are emitted in
// cannot depend on hashing. See docs/adr/0012-parallel-panels.md.
func (h *Hex) Cells(dst []Cell) []Cell {
	dst = dst[:0]
	for row := range h.Rows {
		for col := range h.Cols {
			n := h.Counts[row*h.Cols+col]
			if n == 0 {
				continue
			}
			x, y := h.Center(col, row)
			dst = append(dst, Cell{Col: col, Row: row, X: x, Y: y, Count: n})
		}
	}
	return dst
}

// Fraction maps a count onto [0, 1] against the busiest cell, exactly as
// [Grid.Fraction] does.
func (h *Hex) Fraction(count uint32, s Scaling) float64 { return fraction(count, h.Max, s) }

// BinHex counts every row of xs against ys into h, returning it.
//
// The coordinates are whatever h's rectangle is in — device space for a geom
// binning what it was about to draw, which is what makes the cells regular
// hexagons on screen rather than hexagons stretched by the axes.
func BinHex[F Float](h *Hex, xs, ys []F) *Hex {
	n := min(len(xs), len(ys))
	for i := range n {
		h.Add(float64(xs[i]), float64(ys[i]))
	}
	return h
}

// Vertices writes the six corners of a cell of the given radius, centred on
// (cx, cy), into dst — which is truncated first and returned.
//
// It is here rather than in a geom because the vertices are the lattice's own
// geometry: a cell drawn a degree out of phase with the one it was counted in
// would tile with gaps. The first vertex is the top one, and they run
// clockwise in a coordinate system whose Y axis points down.
func Vertices(dst []Point, cx, cy, radius float64) []Point {
	dst = dst[:0]
	for k := range 6 {
		a := math.Pi * (float64(k)/3 - 0.5)
		dst = append(dst, Point{X: cx + radius*math.Cos(a), Y: cy + radius*math.Sin(a)})
	}
	return dst
}
