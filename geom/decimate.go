package geom

import (
	"image"
	"math"
	"sync"

	"github.com/timzifer/refract/coord"
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
	// A reduction is defined over pixel columns, and under a coord where a
	// column of screen is not a column of data it would be measuring something
	// else. The coord says so rather than every geom guessing — see
	// docs/adr/0018-coordinate-systems.md.
	if !f.Coords().Decimates() {
		return NoDecimation, 0
	}
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
// layers — or two panels on two goroutines — never share one. It is exactly
// one per Build, too: a helper that took a second one while the first was
// still held would put two objects with *disjoint* buffers into one pool, and
// whichever came back in the other's role next frame would have to grow the
// other's buffers. That is a per-row allocation on the path whose whole point
// is not having one, and it is invisible until the allocation gate is looked
// at — see [eachGroup], which takes the caller's scratch for this reason.
type scratch struct {
	ok    []bool
	segs  []series
	fx    []float64 // interpolated data-space columns
	fy    []float64
	fz    []float64
	gx    []float64 // one group's columns, gathered out of the table
	gy    []float64
	gz    []float64
	grows []int
	dx    []float32 // mapped columns, which are device columns under Cartesian
	dy    []float32
	dz    []float32
	kx    []float32 // the surviving columns, gathered for the coord's batch form
	ky    []float32
	sx    []float32 // a staircase, in the space the scales map into
	sy    []float32
	keep  []int
	rows  []int
	mrows []int // source rows behind the marks, when someone asked
	irows []int // source rows of an interpolated series
	pts   []ir.Point
	offs  []ir.Point // how far each mark is broken out of the middle
	rects []ir.Rect
	edge  []ir.Point
	cols  []ir.Color
	runs  []colorRun
	rruns []rectRun
	gruns []groupRun
	at    map[ir.Color]int
	fill  ir.Path
	line  ir.Path
	grid  stat.Grid
	img   *image.NRGBA

	// wantRows is whether this Build's caller asked which source row is behind
	// each mark. It is a field rather than a parameter because the answer is
	// needed several calls deep — segmenting an interpolated series has to
	// know — and it is false for every ordinary render.
	wantRows bool
}

var scratchPool = sync.Pool{New: func() any { return new(scratch) }}

// acquire takes a scratch for one Build pass, told whether the frame's caller
// wants row identity.
func acquire(f Frame) *scratch {
	sc := scratchPool.Get().(*scratch)
	sc.wantRows = f.tracking()
	return sc
}

func (sc *scratch) release() {
	sc.fill.Reset()
	sc.line.Reset()
	sc.wantRows = false
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

// project maps one segment through the scales. z is the band's lower edge, nil
// when the segment has none.
//
// What comes back is the interval position each scale chose, which under
// [coord.Cartesian] is already the device coordinate and under another coord is
// an angle or a radius. The reduction runs here, on those numbers, and it is
// the coord that decides whether that is meaningful at all.
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

// gather collects the given rows of s into a contiguous series of its own.
//
// It is how a grouped layer draws one series at a time: the rows of one group
// are scattered through the table, and every traversal below this — segmenting
// at the holes, projecting, reducing — is written over a contiguous run. The
// gathered series is not a cut of the source, so it carries a source row per
// element rather than an offset, and only when someone asked for one.
func (sc *scratch) gather(s series, rows []int) series {
	n := len(rows)
	out := series{x: grow(sc.gx, n), y: grow(sc.gy, n)}
	for i, r := range rows {
		out.x[i], out.y[i] = s.x[r], s.y[r]
	}
	sc.gx, sc.gy = out.x, out.y
	if s.y2 != nil {
		out.y2 = grow(sc.gz, n)
		for i, r := range rows {
			out.y2[i] = s.y2[r]
		}
		sc.gz = out.y2
	}
	if sc.wantRows {
		out.rows = grow(sc.grows, n)
		for i, r := range rows {
			out.rows[i] = s.rowAt(r)
		}
		sc.grows = out.rows
	}
	return out
}

// marks turns a projected segment into device points through the coord,
// keeping only the given rows — or every row when keep is nil.
//
// The surviving pair is gathered into a contiguous column first so that the
// coord is asked once for the whole run. That is what [coord.Coord.Points] is
// for: an interface method called per row is the shape that cost a million
// allocations on a million-row column once already, and this is exactly the
// path it would reappear on.
func (sc *scratch) marks(cd coord.Coord, x, y []float32, keep []int) []ir.Point {
	if keep != nil {
		sc.kx, sc.ky = grow(sc.kx, len(keep)), grow(sc.ky, len(keep))
		for i, row := range keep {
			sc.kx[i], sc.ky[i] = x[row], y[row]
		}
		x, y = sc.kx, sc.ky
	}
	sc.pts = cd.Points(grow(sc.pts, len(x))[:0], x, y)
	return sc.pts
}

// lowerEdge gathers a band's second edge in reverse order, so that appending it
// to the upper edge closes the shape.
func (sc *scratch) lowerEdge(cd coord.Coord, x, z []float32, keep []int) []ir.Point {
	n := len(x)
	if keep != nil {
		n = len(keep)
	}
	sc.kx, sc.ky = grow(sc.kx, n), grow(sc.ky, n)
	for i := range n {
		row := n - 1 - i
		if keep != nil {
			row = keep[n-1-i]
		}
		sc.kx[i], sc.ky[i] = x[row], z[row]
	}
	sc.edge = cd.Points(grow(sc.edge, n)[:0], sc.kx[:n], sc.ky[:n])
	return sc.edge
}
