package stat

import "math"

// Span is a half-open interval [Lo, Hi) of the unit interval.
type Span struct {
	Lo, Hi float64
}

// Ribbon is one edge's two ends: the span it takes on its source's arc and the
// span it takes on its target's. The two are the same width, because they are
// the same quantity read twice — which is exactly what a chord diagram asserts
// and what tells it apart from two bar charts side by side.
type Ribbon struct {
	Src, Dst Span
}

// Chord lays an edge list out around the unit interval: one arc per node,
// sized by the traffic through it, and one ribbon per edge joining a slice of
// one arc to a slice of another.
//
// Wrapped round a circle by a polar coord this is a chord diagram; left
// straight under a Cartesian one it is an arc diagram, whose edges rise off a
// rail instead of crossing a disc. The layout does not know which — it works in
// the unit interval and the coordinate system decides what that looks like.
//
// It is a struct with a [Chord.Reset] rather than a pair of functions for the
// reason [Sankey] and [Hex] are: the layout keeps a cursor per node, and a
// chart redrawn every frame should reuse it rather than allocate it again.
//
// # Order
//
// Arcs go round in node order and ribbons leave them in edge order, both of
// which are the order the caller's rows appeared in. Nothing here sorts, and
// nothing here reads a map: the picture is a pure function of the table
// (docs/adr/0012-parallel-panels.md). A caller that wants the busiest node
// first sorts its own rows.
//
// The zero Chord is unusable; call [Chord.Reset] first.
type Chord struct {
	// Arcs has one span per node, in node order. A node no edge touches gets an
	// empty span rather than being dropped, so a caller can index Arcs by node.
	Arcs []Span
	// Ribbons has one entry per edge, in the order the edges were given.
	Ribbons []Ribbon

	cursor []float64
	total  []float64
}

// Reset lays out the edge list from[i] → to[i] carrying value[i], over the
// given number of nodes, leaving pad of the unit interval between adjacent
// arcs.
//
// An edge naming a node outside [0, nodes), or carrying a value that is not
// positive, takes no room and gets an empty ribbon. A self-edge is not a
// special case: it takes a slice of its node's arc at each end, which draws as
// a loop.
func (c *Chord) Reset(from, to []int, value []float64, nodes int, pad float64) {
	c.Arcs = c.Arcs[:0]
	c.Ribbons = c.Ribbons[:0]
	if nodes <= 0 {
		return
	}
	m := min(len(from), len(to), len(value))
	for range nodes {
		c.Arcs = append(c.Arcs, Span{})
	}
	for range m {
		c.Ribbons = append(c.Ribbons, Ribbon{})
	}

	c.total = resize(c.total, nodes)
	c.cursor = resize(c.cursor, nodes)

	grand := 0.0
	for e := range m {
		if !chordLinked(from, to, e, nodes) || !(value[e] > 0) {
			continue
		}
		// Both ends of an edge take room, which is why the grand total is twice
		// the sum of the values: an arc's size is the traffic through the node,
		// and traffic is counted where it arrives as well as where it leaves.
		c.total[from[e]] += value[e]
		c.total[to[e]] += value[e]
		grand += 2 * value[e]
	}
	if !(grand > 0) {
		return
	}

	// The gaps come out of the circle before the arcs are sized, so that the
	// arcs still stand in proportion to each other once the gaps are in.
	busy := 0
	for i := range nodes {
		if c.total[i] > 0 {
			busy++
		}
	}
	room := 1 - pad*float64(busy)
	if !(room > 0) {
		room = 0
	}

	cursor := 0.0
	for i := range nodes {
		if !(c.total[i] > 0) {
			c.Arcs[i] = Span{Lo: cursor, Hi: cursor}
			c.cursor[i] = cursor
			continue
		}
		w := room * c.total[i] / grand
		c.Arcs[i] = Span{Lo: cursor, Hi: cursor + w}
		c.cursor[i] = cursor
		cursor += w + pad
	}

	for e := range m {
		if !chordLinked(from, to, e, nodes) || !(value[e] > 0) {
			continue
		}
		w := room * value[e] / grand
		src, dst := from[e], to[e]
		c.Ribbons[e].Src = Span{Lo: c.cursor[src], Hi: c.cursor[src] + w}
		c.cursor[src] += w
		c.Ribbons[e].Dst = Span{Lo: c.cursor[dst], Hi: c.cursor[dst] + w}
		c.cursor[dst] += w
	}
}

// Width is the span's extent, which is what a caller comparing two of them
// actually wants to read.
func (s Span) Width() float64 { return math.Abs(s.Hi - s.Lo) }

func chordLinked(from, to []int, e, nodes int) bool {
	f, t := from[e], to[e]
	return f >= 0 && f < nodes && t >= 0 && t < nodes
}
