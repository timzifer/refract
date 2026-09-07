package stat

import "math"

// Tile is one rectangle of a treemap: the box a value was given.
//
// It is four float64s rather than an ir.Rect because stat does not import ir,
// and it is a value rather than a pointer because a treemap of a hundred
// thousand nodes is a slice of these and nothing else.
type Tile struct {
	X0, Y0, X1, Y1 float64
}

// Squarify packs values into a rectangle, one tile per value, each with an area
// proportional to its value and an aspect ratio as close to square as the
// packing allows.
//
// It is the squarified treemap of Bruls, Huizing and van Wijk (2000): tiles are
// gathered into a row along the rectangle's shorter side for as long as adding
// one improves the row's worst aspect ratio, then the row is laid out and the
// rest of the rectangle is packed the same way. The alternative — slicing the
// rectangle in one direction — produces slivers that a reader cannot compare,
// which is the whole reason this algorithm exists.
//
// It lays one level out: the values are one node's children, and a hierarchy is
// drawn by calling it once per sibling group with the box that group's parent
// was given. Recursing here would make it a tree walker in a package that knows
// about numbers and nothing else.
//
// # Order is the caller's
//
// Values are packed in the order they are given, and that order is what decides
// which tiles end up beside which. Descending order gives the best aspect
// ratios and is what a treemap usually wants; source order keeps the picture
// stable as the numbers move, which is what an animated one wants. This
// function sorts neither way — sorting would mean mutating the caller's column
// or allocating a copy of it per frame, and the caller already has a buffer
// (see CONTRIBUTING, "Take ordered input rather than sorting").
//
// A value that is zero or negative gets an empty tile where it falls, rather
// than being dropped: the result has one tile per value, at the same index, so
// a caller can read a tile against the row it came from without a second
// mapping.
func Squarify(values []float64, x0, y0, x1, y1 float64) []Tile {
	return AppendSquarify(nil, values, x0, y0, x1, y1)
}

// AppendSquarify is [Squarify] writing into dst, which it truncates and grows
// as needed. It is the form a geom calls, because a chart redrawn every frame
// should not allocate a tile per node per frame.
func AppendSquarify(dst []Tile, values []float64, x0, y0, x1, y1 float64) []Tile {
	dst = dst[:0]
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	n := len(values)
	for range n {
		dst = append(dst, Tile{X0: x0, Y0: y0, X1: x0, Y1: y0})
	}
	if n == 0 {
		return dst
	}

	total := 0.0
	for _, v := range values {
		if v > 0 {
			total += v
		}
	}
	w, h := x1-x0, y1-y0
	if !(total > 0) || !(w > 0) || !(h > 0) {
		return dst
	}
	// Values become areas once, so the aspect-ratio arithmetic below is in the
	// same units as the rectangle it is packing.
	unit := (w * h) / total

	for i := 0; i < n; {
		if values[i] <= 0 {
			// An empty tile takes no room and starts no row. It sits at the
			// corner of what is left, which is where its zero belongs.
			dst[i] = Tile{X0: x0, Y0: y0, X1: x0, Y1: y0}
			i++
			continue
		}
		side := math.Min(x1-x0, y1-y0)
		if !(side > 0) {
			break
		}

		// Grow the row while the worst aspect ratio in it keeps improving.
		// lo and hi track the row's smallest and largest area so that worst()
		// needs no second pass over it.
		sum, lo, hi := 0.0, math.Inf(1), 0.0
		j := i
		best := math.Inf(1)
		for ; j < n; j++ {
			a := values[j] * unit
			if a <= 0 {
				break
			}
			nsum, nlo, nhi := sum+a, math.Min(lo, a), math.Max(hi, a)
			w := worst(nsum, nlo, nhi, side)
			if j > i && w > best {
				break
			}
			sum, lo, hi, best = nsum, nlo, nhi, w
		}

		// Lay the row out along the shorter side and take its slab off the
		// rectangle. A row of zero thickness would loop forever; it cannot
		// happen while sum and side are both positive, and the guard above is
		// what keeps that true.
		thick := sum / side
		if x1-x0 >= y1-y0 {
			cursor := y0
			for k := i; k < j; k++ {
				height := (values[k] * unit) / thick
				dst[k] = Tile{X0: x0, Y0: cursor, X1: x0 + thick, Y1: cursor + height}
				cursor += height
			}
			x0 += thick
		} else {
			cursor := x0
			for k := i; k < j; k++ {
				width := (values[k] * unit) / thick
				dst[k] = Tile{X0: cursor, Y0: y0, X1: cursor + width, Y1: y0 + thick}
				cursor += width
			}
			y0 += thick
		}
		i = j
	}
	return dst
}

// worst is the worst aspect ratio in a row of total area sum, whose smallest
// and largest tiles are lo and hi, laid along a side of the given length. It is
// the max(w²r⁺/s², s²/w²r⁻) of the paper, written out.
func worst(sum, lo, hi, side float64) float64 {
	if !(sum > 0) || !(lo > 0) {
		return math.Inf(1)
	}
	s2, w2 := sum*sum, side*side
	return math.Max((w2*hi)/s2, s2/(w2*lo))
}
