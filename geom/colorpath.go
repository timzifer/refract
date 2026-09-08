package geom

import (
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// Colouring a path from a column, rather than a mark.
//
// A mark takes one colour and is done — [scratch.groupByColor] batches a
// thousand points into as many drawing calls as there are distinct colours and
// nothing is lost, because the points do not touch. A path is not that. Its
// colour changes *between* two vertices, so the question a scatter never has
// to answer is the only question here: where along that edge.
//
// It has two answers, and which one is right follows from the scale rather
// than from an option:
//
//   - A classed scale ([scale.ClassedColorScale]) cuts a quantity at
//     boundaries, and a boundary is a value. The line crosses it somewhere
//     between two rows, and that somewhere is the whole content of the
//     picture: a chart that turned red at the next measurement rather than at
//     the limit reports a different time. So the crossing is interpolated and
//     a vertex is inserted there.
//   - A discrete scale paints categories, and a category is not a quantity to
//     cross. Nothing was measured between two rows of a state column, so the
//     new colour begins at the row where the new state was first seen and the
//     edge into it keeps the old one. Interpolating there would invent the
//     moment the machine changed, which is exactly what the data does not say.
//
// A continuous ramp has no answer at all, which is why a path refuses one —
// see [ErrRampOnPath]. An [ir.Stroke] carries a single colour and no stops, so
// the only thing a path could do with a continuum is cut it into classes
// nobody asked for.

// pathRun is a stretch of a path that is one colour, given as an inclusive
// index range into the split columns.
//
// Two neighbouring runs *share* their boundary vertex — the first ends on it
// and the second begins on it. That is not an off-by-one: leaving it out would
// leave the edge between them undrawn, and a gap in a line reads as missing
// data rather than as a change of colour.
type pathRun struct {
	color  ir.Color
	lo, hi int
}

// crossing is a class boundary met between two vertices: the boundary itself
// and where along the edge it falls, in the colour column's own space.
type crossing struct {
	t float64
	b float64
}

// colorSplit is what a layer knows about how its colour column meets its
// geometry, gathered up so that the splitter itself needs no frame.
type colorSplit struct {
	// breaks are the class boundaries of a classed scale, ascending; nil for
	// a discrete one, whose colour changes at a vertex rather than between
	// two.
	breaks []float64
	// pin maps a boundary onto the axis that also positions the colour
	// column, and reports false for a boundary that axis cannot place. It
	// exists so that on a log axis the corner lands on the drawn threshold
	// rather than a hair off it: interpolating the *value* linearly and
	// interpolating the *position* linearly are the same thing only on a
	// linear axis, and the reader is looking at the position.
	pin func(v float64) (float32, bool)
	// onY says which of the two mapped columns pin answers about.
	onY bool
}

// splitFor works out how a layer's colour scale meets its geometry, and
// reports ok == false for a scale a path cannot draw.
func splitFor(c config, f Frame) (scale.ColorScale, colorSplit, bool) {
	var sp colorSplit
	if c.colorScale == nil {
		return nil, sp, false
	}
	if classed, is := scale.Classed(c.colorScale); is {
		sp.breaks = classed.Breaks()
		switch c.colorCol {
		case c.ycol:
			sp.onY, sp.pin = true, pinTo(f.Y)
		case c.xcol:
			sp.onY, sp.pin = false, pinTo(f.X)
		}
		return c.colorScale, sp, true
	}
	if _, is := scale.Discrete(c.colorScale); is {
		return c.colorScale, sp, true
	}
	return nil, sp, false
}

// pinTo maps a boundary through an axis, reporting false for one the axis
// cannot place — a threshold at zero on a log axis, say.
func pinTo(s scale.Scale) func(float64) (float32, bool) {
	return func(v float64) (float32, bool) {
		if !defined(s, v) {
			return 0, false
		}
		return s.Map(v), true
	}
}

// colorRuns splits mapped columns into contiguous single-colour runs,
// inserting a vertex wherever the colour changes between two of them.
//
// It works on the columns the scales mapped into rather than on device points,
// for the reason [scratch.stepColumns] does: where the colour changes is a
// statement about the data, and a coord that bends its edges would otherwise
// decide it. vals is the colour value of each vertex, index for index with x
// and y.
//
// The split columns and the runs live in the scratch, so a chart redrawn every
// frame splits into the same memory.
func (sc *scratch) colorRuns(cs scale.ColorScale, sp colorSplit, vals []float64, x, y []float32) ([]float32, []float32, []pathRun) {
	n := len(x)
	sx, sy := grow(sc.cx, n)[:0], grow(sc.cy, n)[:0]
	runs := sc.pruns[:0]
	if n == 0 {
		sc.cx, sc.cy, sc.pruns = sx, sy, runs
		return sx, sy, runs
	}

	sx, sy = append(sx, x[0]), append(sy, y[0])
	cur, lo := cs.Color(vals[0]), 0
	// cut ends the current run on the vertex just appended and starts the
	// next one on it. The shared index is deliberate — see [pathRun].
	cut := func(next ir.Color) {
		k := len(sx) - 1
		runs = append(runs, pathRun{color: cur, lo: lo, hi: k})
		lo, cur = k, next
	}
	for i := 1; i < n; i++ {
		col := cs.Color(vals[i])
		if col == cur {
			sx, sy = append(sx, x[i]), append(sy, y[i])
			continue
		}
		v0, v1 := vals[i-1], vals[i]
		sc.cuts = crossings(sc.cuts[:0], sp.breaks, v0, v1)
		for j, cr := range sc.cuts {
			px, py := sp.place(x[i-1], y[i-1], x[i], y[i], cr)
			sx, sy = append(sx, px), append(sy, py)
			// The colour of what follows is read off the middle of the
			// stretch this crossing opens rather than off the boundary
			// itself: a boundary belongs to the class above it, which is
			// the answer for a line going up and the wrong one for a line
			// coming down.
			next := 1.0
			if j+1 < len(sc.cuts) {
				next = sc.cuts[j+1].t
			}
			cut(cs.Color(lerp(v0, v1, (cr.t+next)/2)))
		}
		sx, sy = append(sx, x[i]), append(sy, y[i])
		if cur != col {
			// Either the scale is discrete and the new colour begins on
			// this vertex, or a boundary fell exactly on it.
			cut(col)
		}
	}
	runs = append(runs, pathRun{color: cur, lo: lo, hi: len(sx) - 1})
	sc.cx, sc.cy, sc.pruns = sx, sy, runs
	return sx, sy, runs
}

// place puts a boundary vertex on the edge from (x0, y0) to (x1, y1).
//
// The parameter is the one the colour column gives, unless the colour column
// is a positional one — then the axis has already said where the boundary
// goes, and the other coordinate follows it there.
func (sp colorSplit) place(x0, y0, x1, y1 float32, cr crossing) (float32, float32) {
	t := cr.t
	if sp.pin != nil {
		if pos, ok := sp.pin(cr.b); ok {
			a, b := x0, x1
			if sp.onY {
				a, b = y0, y1
			}
			if u, ok := along(a, b, pos); ok {
				t = u
			}
		}
	}
	return lerp32(x0, x1, t), lerp32(y0, y1, t)
}

func lerp32(a, b float32, t float64) float32 { return a + float32(float64(b-a)*t) }

// along is where pos sits between a and b, clamped, or false when the two are
// the same and the question has no answer.
func along(a, b, pos float32) (float64, bool) {
	if a == b {
		return 0, false
	}
	u := float64(pos-a) / float64(b-a)
	return min(max(u, 0), 1), true
}

// crossings appends the class boundaries met between v0 and v1, in the order
// the edge meets them, leaving out one that lands on either end — there is a
// vertex there already.
//
// A boundary is crossed when the two ends fall in different classes across it,
// which is the same test in both directions: a value equal to a boundary is in
// the class above it (see [scale.Threshold]), so the interval is half-open the
// same way coming down as going up.
func crossings(dst []crossing, breaks []float64, v0, v1 float64) []crossing {
	if len(breaks) == 0 || v0 == v1 {
		return dst
	}
	for i := range breaks {
		b := breaks[i]
		if v1 < v0 {
			b = breaks[len(breaks)-1-i]
		}
		if (v0 < b) == (v1 < b) {
			continue
		}
		t := (b - v0) / (v1 - v0)
		if t <= 0 || t >= 1 {
			continue
		}
		dst = append(dst, crossing{t: t, b: b})
	}
	return dst
}
