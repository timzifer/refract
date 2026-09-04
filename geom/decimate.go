package geom

import (
	"image"
	"math"
	"sync"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
)

// Decimation is how a layer reduces its rows before it draws them.
//
// A plot area is a few hundred pixels wide. A column with a million rows in it
// therefore has thousands of rows per pixel column, and drawing all of them
// emits thousands of segments that land on the same handful of pixels: slower
// to build, slower to rasterize, larger as a file, and not one pixel
// different. Reducing first is not a compromise for big data, it is the
// accurate way to draw it.
//
// Nothing here touches a scale's domain. A geom trains on every row and
// reduces only when it draws, so the axes report the data rather than the
// subset that survived.
type Decimation uint8

// The reductions. AutoDecimation is the default: a geom knows what its mark is
// and can see how many rows there are against how wide the plot is, which is
// exactly the information the choice needs.
const (
	// AutoDecimation lets the layer choose — see [Decimate] for what it picks.
	AutoDecimation Decimation = iota

	// NoDecimation draws every row. It is what to ask for when the chart is
	// evidence and a reader will zoom into the vector output.
	NoDecimation

	// LTTB keeps the rows that carry a line's shape, using
	// largest-triangle-three-buckets. See [stat.LTTB].
	LTTB

	// MinMax keeps the extremes of every pixel column, so a spike one sample
	// wide still reaches full height. See [stat.MinMax].
	MinMax

	// DensityRaster counts rows per cell and draws the counts as an image
	// instead of drawing marks. See [stat.Grid].
	DensityRaster
)

// Decimate sets how a layer reduces its rows before drawing.
//
// The default, [AutoDecimation], picks by mark and by size:
//
//   - a line reduces with [LTTB] once it holds more than a few rows per pixel
//     column, because a line is a shape and LTTB is the reduction that keeps
//     one;
//   - a step and a band reduce with [MinMax], because their ink is their
//     extremes — a staircase is its transitions and a band is its edges;
//   - a scatter switches to a [DensityRaster] once its markers would cover the
//     plot area several times over, because at that point the marks overplot
//     and the picture says more about row order than about the data;
//   - a bar or a boxplot never reduces: those marks already aggregate, and
//     there is no row-level reduction that leaves a bar a bar.
//
// A layer whose colour comes from a column is never reduced automatically: it
// draws one fact per mark, and a raster of counts would answer a different
// question. Ask for [DensityRaster] explicitly to override that.
func Decimate(d Decimation) Option { return func(c *config) { c.decimate = d } }

// Budget caps how many marks a reduced layer draws. The default is derived
// from the width of the plot area, which is the only number that decides how
// many marks are distinguishable.
//
// It has no effect on a layer that is not reducing, and none on
// [DensityRaster], whose resolution is the plot area itself.
func Budget(n int) Option { return func(c *config) { c.budget = n } }

// DensityCells sets the size of one density-raster cell in device pixels. The
// default is 1: one cell per pixel, which is as fine as the output can show.
// Larger cells trade resolution for a smaller image and a smoother picture.
func DensityCells(px float64) Option { return func(c *config) { c.cellSize = px } }

// markShape is what a layer's marks are, which is what decides which reduction
// preserves them.
type markShape uint8

const (
	shapePath    markShape = iota // a connected line
	shapeStair                    // a staircase
	shapeBand                     // a filled envelope with two edges
	shapeMarkers                  // independent marks, one per row
)

// How many marks each reduction is allowed per pixel column, and how crowded a
// column has to be before AutoDecimation starts reducing at all.
//
// Two per column is where a polyline stops gaining shape: the eye cannot
// resolve a third vertex in the same column, and the rasterizer draws it over
// the other two. MinMax's four is its own ceiling rather than a choice — entry,
// minimum, maximum, exit. The trigger sits above both so that a chart which is
// merely detailed is left exactly as it was drawn.
const (
	lttbMarksPerColumn   = 2
	minMaxMarksPerColumn = 4
	autoTriggerPerColumn = 4
)

// overplotFactor is how many times over a layer's markers must cover the plot
// area before drawing them individually stops meaning anything. At four, more
// than three quarters of the ink is hidden under other ink, and which marks
// survive is decided by row order rather than by the data.
const overplotFactor = 4

// reduction resolves the reduction for one Build: which one, and how many
// marks it may leave behind.
func (c config) reduction(shape markShape, s series, f Frame) (Decimation, int) {
	cols := plotColumns(f.Area)
	mode := c.decimate
	if mode == AutoDecimation {
		mode = c.autoReduction(shape, s, f, cols)
	}
	switch mode {
	case LTTB:
		if shape == shapeBand {
			// A band has two edges and LTTB reads one series. Reducing on the
			// upper edge alone would clip the lower one wherever the two
			// disagree, which is the whole content of a band.
			return MinMax, c.budgetOr(minMaxMarksPerColumn * cols)
		}
		return LTTB, c.budgetOr(lttbMarksPerColumn * cols)
	case MinMax:
		return MinMax, c.budgetOr(minMaxMarksPerColumn * cols)
	case DensityRaster:
		return DensityRaster, 0
	default:
		return NoDecimation, 0
	}
}

func (c config) autoReduction(shape markShape, s series, f Frame, cols int) Decimation {
	n := len(s.x)
	if c.varying(s) {
		return NoDecimation
	}
	if shape == shapeMarkers {
		if overplotted(n, c, f) {
			return DensityRaster
		}
		return NoDecimation
	}
	if n <= autoTriggerPerColumn*cols {
		return NoDecimation
	}
	if shape == shapePath {
		return LTTB
	}
	return MinMax
}

// overplotted reports whether this many markers would bury each other.
func overplotted(n int, c config, f Frame) bool {
	area := float64(f.Area.Dx()) * float64(f.Area.Dy())
	if area <= 0 {
		return false
	}
	d := float64(pick(c.size, f.Theme.MarkerSize))
	mark := math.Pi * d * d / 4
	return float64(n)*mark > overplotFactor*area
}

func (c config) budgetOr(fallback int) int {
	if c.budget > 0 {
		return c.budget
	}
	return max(fallback, minBudget)
}

// minBudget keeps a reduction from collapsing a chart drawn into a very narrow
// panel — a facet of twenty columns is still a chart, not four points.
const minBudget = 64

// plotColumns is how many pixel columns the marks have to fit into.
func plotColumns(area ir.Rect) int { return max(int(area.Dx()), 1) }

// scratch is the working memory of one Build pass.
//
// Every buffer here is sized by the number of rows, so allocating them per
// frame makes a chart pay for its data twice — once to walk it, once to
// collect it. They come from a pool instead, which is what keeps a chart
// redrawn every frame from allocating anything that grows with the data it
// draws.
//
// A scratch is held for one Build call and released at the end of it, so two
// layers — or two panels on two goroutines — never share one.
type scratch struct {
	ok    []bool
	segs  []series
	fx    []float64 // interpolated data-space columns
	fy    []float64
	fz    []float64
	dx    []float32 // device-space columns
	dy    []float32
	dz    []float32
	keep  []int
	rows  []int
	pts   []ir.Point
	rects []ir.Rect
	edge  []ir.Point
	cols  []ir.Color
	runs  []colorRun
	at    map[ir.Color]int
	fill  ir.Path
	line  ir.Path
	grid  stat.Grid
	img   *image.NRGBA
}

var scratchPool = sync.Pool{New: func() any { return new(scratch) }}

func acquire() *scratch { return scratchPool.Get().(*scratch) }

func (sc *scratch) release() {
	sc.fill.Reset()
	sc.line.Reset()
	scratchPool.Put(sc)
}

// grow returns a slice of length n backed by buf's array when it fits.
func grow[T any](buf []T, n int) []T {
	if cap(buf) >= n {
		return buf[:n]
	}
	return make([]T, n)
}

// plottable marks the rows both scales have a position for, into the scratch's
// buffer. See [series.plottable] for why it is computed once.
func (sc *scratch) plottable(s series, x, y scale.Scale) []bool {
	sc.ok = grow(sc.ok, len(s.x))
	for i := range s.x {
		ok := defined(x, s.x[i]) && defined(y, s.y[i])
		if ok && s.y2 != nil {
			ok = defined(y, s.y2[i])
		}
		sc.ok[i] = ok
	}
	return sc.ok
}

// project maps one segment into device space. z is the band's lower edge, nil
// when the segment has none.
//
// Reducing happens here rather than in data space because a pixel column is
// the unit the whole exercise is about: on a log axis, equal steps in data are
// not equal steps on screen, and a reduction that bucketed by value would
// spend its budget on the wrong end of the axis.
func (sc *scratch) project(seg series, f Frame) (x, y, z []float32) {
	n := len(seg.x)
	sc.dx, sc.dy = grow(sc.dx, n), grow(sc.dy, n)
	for i := range n {
		sc.dx[i] = f.X.Map(seg.x[i])
		sc.dy[i] = f.Y.Map(seg.y[i])
	}
	if seg.y2 == nil {
		return sc.dx, sc.dy, nil
	}
	sc.dz = grow(sc.dz, n)
	for i := range n {
		sc.dz[i] = f.Y.Map(seg.y2[i])
	}
	return sc.dx, sc.dy, sc.dz
}

// reduce runs the reduction over a projected segment and returns the rows it
// kept, or nil when every row is kept.
func (sc *scratch) reduce(mode Decimation, budget int, x, y, z []float32) []int {
	sc.keep = sc.keep[:0]
	switch mode {
	case LTTB:
		sc.keep = stat.AppendLTTB(sc.keep, x, y, budget)
	case MinMax:
		cols := max(budget/minMaxMarksPerColumn, 1)
		if z != nil {
			sc.keep = stat.AppendMinMax(sc.keep, x, cols, y, z)
		} else {
			sc.keep = stat.AppendMinMax(sc.keep, x, cols, y)
		}
	default:
		return nil
	}
	if len(sc.keep) >= len(x) {
		return nil // nothing was dropped; skip the gather
	}
	return sc.keep
}

// marks gathers a projected segment into points, keeping only the given rows —
// or every row when keep is nil.
func (sc *scratch) marks(x, y []float32, keep []int) []ir.Point {
	if keep == nil {
		sc.pts = grow(sc.pts, len(x))
		for i := range x {
			sc.pts[i] = ir.Point{X: x[i], Y: y[i]}
		}
		return sc.pts
	}
	sc.pts = grow(sc.pts, len(keep))
	for i, row := range keep {
		sc.pts[i] = ir.Point{X: x[row], Y: y[row]}
	}
	return sc.pts
}

// lowerEdge gathers a band's second edge in reverse order, so that appending it
// to the upper edge closes the shape.
func (sc *scratch) lowerEdge(x, z []float32, keep []int) []ir.Point {
	n := len(x)
	if keep != nil {
		n = len(keep)
	}
	sc.edge = grow(sc.edge, n)
	for i := range n {
		row := n - 1 - i
		if keep != nil {
			row = keep[n-1-i]
		}
		sc.edge[i] = ir.Point{X: x[row], Y: z[row]}
	}
	return sc.edge
}
