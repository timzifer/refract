package stat

import "math"

// SankeySweeps is how many times a [Sankey] relaxes its node positions.
//
// It is a constant rather than a tolerance, and that is the whole point. A
// relaxation that ran "until it settled" would make the picture depend on
// floating-point noise, and a chart whose panels are built on separate
// goroutines has to be byte-identical to one built serially
// (docs/adr/0012-parallel-panels.md). Six sweeps is where the arrangement stops
// visibly improving on the flows this was tested against; a seventh moves
// nothing a reader can see.
const SankeySweeps = 6

// SankeyNode is one node's place in a flow diagram: which column it stands in,
// and the span of the unit interval it fills.
type SankeyNode struct {
	// Layer is the node's column, counting from zero at the sources.
	Layer int
	// Lo and Hi are the node's near and far edge, in [0, 1].
	Lo, Hi float64
	// Value is the flow through the node: the greater of what enters it and
	// what leaves it.
	Value float64
}

// SankeyFlow is one link's band, given as the span it occupies where it leaves
// its source and the span it occupies where it enters its target. The two
// differ, which is what makes the band a ribbon rather than a rectangle.
type SankeyFlow struct {
	SrcLo, SrcHi float64
	DstLo, DstHi float64
}

// Sankey lays an edge list out as a flow diagram: nodes in columns, links as
// bands whose thickness is their value.
//
// It is a struct with a [Sankey.Reset] rather than a pair of functions because
// the layout has working state the size of the data — a value per node, a
// cursor per node — and a chart redrawn every frame should reuse it rather than
// allocate it again. That is the shape [Hex] has, for the same reason.
//
// # What it decides, and what it does not
//
// It decides two things: which column each node stands in, and how far down
// each node and each band sits. It does **not** decide the order of the nodes
// within a column — that is the order they were given in, which is the order
// their rows appeared in the source table. Reordering nodes to reduce crossings
// would mean a sort per sweep, and a sort is where a layout stops being a pure
// function of its input and starts depending on how a tie was broken. A caller
// that wants a different order sorts its own rows and hands them over that way.
//
// The zero Sankey is unusable; call [Sankey.Reset] first.
type Sankey struct {
	// Nodes has one entry per node, indexed as the caller's edge list indexes
	// them.
	Nodes []SankeyNode
	// Flows has one entry per link, in the order the links were given.
	Flows []SankeyFlow

	// Layers is how many columns the diagram has, and Cyclic reports an edge
	// list that is not a DAG — in which case Nodes and Flows are empty, because
	// a flow that returns to where it came from has no column to stand in.
	Layers int
	Cyclic bool

	in, out, cursor []float64
}

// Reset lays out the edge list from[i] → to[i] carrying value[i], over the
// given number of nodes, leaving pad of the unit interval between adjacent
// nodes in a column.
//
// pad is a fraction rather than a length because everything here is in the unit
// square: the geom knows how wide the panel is and this does not.
//
// An edge naming a node outside [0, nodes) is skipped, and an edge list with a
// cycle sets [Sankey.Cyclic] and lays nothing out.
func (s *Sankey) Reset(from, to []int, value []float64, nodes int, pad float64) {
	s.Nodes = s.Nodes[:0]
	s.Flows = s.Flows[:0]
	s.Layers, s.Cyclic = 0, false
	if nodes <= 0 {
		return
	}
	m := min(len(from), len(to), len(value))

	for range nodes {
		s.Nodes = append(s.Nodes, SankeyNode{})
	}
	if !s.assignLayers(from, to, m, nodes) {
		s.Nodes, s.Cyclic = s.Nodes[:0], true
		return
	}

	s.in = resize(s.in, nodes)
	s.out = resize(s.out, nodes)
	s.cursor = resize(s.cursor, nodes)
	for e := range m {
		if !s.linked(from, to, e, nodes) || !(value[e] > 0) {
			continue
		}
		s.out[from[e]] += value[e]
		s.in[to[e]] += value[e]
	}
	for i := range nodes {
		s.Nodes[i].Value = math.Max(s.in[i], s.out[i])
	}

	s.stack(nodes, pad)
	s.relax(from, to, value, m, nodes, pad)
	s.band(from, to, value, m, nodes)
}

// assignLayers puts every node in the column one past the deepest source that
// reaches it, and reports whether the edge list is a DAG.
//
// It relaxes rather than sorting topologically: one pass per column, and a
// graph whose longest path exceeds the number of nodes has a cycle in it, which
// is the same bound a topological sort would have found and needs no queue.
func (s *Sankey) assignLayers(from, to []int, m, nodes int) bool {
	for pass := 0; pass <= nodes; pass++ {
		changed := false
		for e := range m {
			if !s.linked(from, to, e, nodes) {
				continue
			}
			if d := s.Nodes[from[e]].Layer + 1; s.Nodes[to[e]].Layer < d {
				s.Nodes[to[e]].Layer = d
				changed = true
			}
		}
		if !changed {
			for i := range nodes {
				s.Layers = max(s.Layers, s.Nodes[i].Layer+1)
			}
			return true
		}
	}
	return false
}

func (s *Sankey) linked(from, to []int, e, nodes int) bool {
	f, t := from[e], to[e]
	return f >= 0 && f < nodes && t >= 0 && t < nodes && f != t
}

// stack gives every node its first position: the columns are packed from the
// top, and the busiest column is scaled to fill the unit interval so the
// diagram uses the height it is given.
//
// pad is a fraction of that interval, so a column's gaps are subtracted from
// the room its nodes have before the scale is chosen. Every column shares one
// scale — that is what makes a band's thickness comparable across the diagram,
// which is the only quantity a sankey actually asserts.
func (s *Sankey) stack(nodes int, pad float64) {
	scale := math.Inf(1)
	for l := range s.Layers {
		sum, count := 0.0, 0
		for i := range nodes {
			if s.Nodes[i].Layer == l {
				sum += s.Nodes[i].Value
				count++
			}
		}
		if count == 0 || !(sum > 0) {
			continue
		}
		room := 1 - pad*float64(count-1)
		if !(room > 0) {
			// The gaps alone fill the column. Draw the nodes as hairlines
			// rather than upside down: the reader can see there are too many.
			room = 0
		}
		scale = math.Min(scale, room/sum)
	}
	if math.IsInf(scale, 1) {
		return
	}
	for l := range s.Layers {
		cursor := 0.0
		for i := range nodes {
			if s.Nodes[i].Layer != l {
				continue
			}
			s.Nodes[i].Lo = cursor
			s.Nodes[i].Hi = cursor + s.Nodes[i].Value*scale
			cursor = s.Nodes[i].Hi + pad
		}
	}
}

// relax moves each node towards the middle of what it is joined to, alternating
// the direction it reads so that a node is pulled by both its sources and its
// targets, and pushes the column back apart afterwards.
func (s *Sankey) relax(from, to []int, value []float64, m, nodes int, pad float64) {
	for sweep := range SankeySweeps {
		alpha := math.Pow(0.99, float64(sweep))
		s.pull(from, to, value, m, nodes, alpha, true)
		s.separate(nodes, pad)
		s.pull(from, to, value, m, nodes, alpha, false)
		s.separate(nodes, pad)
	}
}

// pull moves every node a fraction alpha of the way to the value-weighted
// middle of its neighbours on one side.
func (s *Sankey) pull(from, to []int, value []float64, m, nodes int, alpha float64, forward bool) {
	for i := range nodes {
		s.in[i], s.out[i] = 0, 0 // reused here as the weighted sum and its weight
	}
	for e := range m {
		if !s.linked(from, to, e, nodes) || !(value[e] > 0) {
			continue
		}
		src, dst := from[e], to[e]
		if forward {
			s.in[dst] += mid(s.Nodes[src]) * value[e]
			s.out[dst] += value[e]
		} else {
			s.in[src] += mid(s.Nodes[dst]) * value[e]
			s.out[src] += value[e]
		}
	}
	for i := range nodes {
		if !(s.out[i] > 0) {
			continue
		}
		want := s.in[i] / s.out[i]
		shift := (want - mid(s.Nodes[i])) * alpha
		s.Nodes[i].Lo += shift
		s.Nodes[i].Hi += shift
	}
}

// separate pushes a column's nodes apart until none overlaps, then back inside
// the unit interval. The order is never changed — only the gaps are.
func (s *Sankey) separate(nodes int, pad float64) {
	for l := range s.Layers {
		cursor := 0.0
		for i := range nodes {
			if s.Nodes[i].Layer != l {
				continue
			}
			if s.Nodes[i].Lo < cursor {
				h := s.Nodes[i].Hi - s.Nodes[i].Lo
				s.Nodes[i].Lo, s.Nodes[i].Hi = cursor, cursor+h
			}
			cursor = s.Nodes[i].Hi + pad
		}
		// The column may now hang below the bottom; walk it back up, which can
		// only close gaps the pass above opened.
		cursor = 1
		for i := nodes - 1; i >= 0; i-- {
			if s.Nodes[i].Layer != l {
				continue
			}
			if s.Nodes[i].Hi > cursor {
				h := s.Nodes[i].Hi - s.Nodes[i].Lo
				s.Nodes[i].Hi, s.Nodes[i].Lo = cursor, cursor-h
			}
			cursor = s.Nodes[i].Lo - pad
		}
	}
}

// band stacks the links against each node's edge, in the order the links were
// given, so that a ribbon leaves its source and arrives at its target at a
// definite place rather than at the node's middle.
func (s *Sankey) band(from, to []int, value []float64, m, nodes int) {
	for i := range nodes {
		s.cursor[i] = s.Nodes[i].Lo
		s.in[i] = s.Nodes[i].Lo
	}
	for range m {
		s.Flows = append(s.Flows, SankeyFlow{})
	}
	for e := range m {
		if !s.linked(from, to, e, nodes) || !(value[e] > 0) {
			continue
		}
		src, dst := from[e], to[e]
		// A node's thickness is the greater of what enters and what leaves, so
		// the thinner side is stacked at the same scale and simply stops short.
		h := s.thick(src, value[e])
		s.Flows[e].SrcLo, s.Flows[e].SrcHi = s.cursor[src], s.cursor[src]+h
		s.cursor[src] += h

		h = s.thick(dst, value[e])
		s.Flows[e].DstLo, s.Flows[e].DstHi = s.in[dst], s.in[dst]+h
		s.in[dst] += h
	}
}

// thick is how tall a link of the given value is at one end of a node, in the
// same units the node's own span is in.
func (s *Sankey) thick(i int, v float64) float64 {
	n := s.Nodes[i]
	if !(n.Value > 0) {
		return 0
	}
	return (n.Hi - n.Lo) * v / n.Value
}

func mid(n SankeyNode) float64 { return (n.Lo + n.Hi) / 2 }

func resize(dst []float64, n int) []float64 {
	dst = dst[:0]
	for range n {
		dst = append(dst, 0)
	}
	return dst
}
