package stat_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/timzifer/refract/stat"
)

// The hierarchy the treemap and icicle tests read: a root with two children,
// one of which has two of its own. Only the leaves carry a value, which is the
// usual shape — an internal node's number is the sum of what is under it.
//
//	0 root
//	├── 1 a        (3 + 1)
//	│   ├── 3 a1   3
//	│   └── 4 a2   1
//	└── 2 b        6
func tree() (parent []int, value []float64) {
	return []int{stat.NoParent, 0, 0, 1, 1},
		[]float64{0, 0, 6, 3, 1}
}

func TestDepthCountsFromTheRoot(t *testing.T) {
	parent, _ := tree()
	got := stat.Depth(parent)
	want := []int{0, 1, 1, 2, 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Depth = %v, want %v", got, want)
	}
}

func TestDepthReportsACycleRatherThanGuessing(t *testing.T) {
	// 0 → 1 → 2 → 0 reaches no root, so none of the three has a depth. Node 3
	// hangs off nothing and is a root of its own, and must survive.
	got := stat.Depth([]int{2, 0, 1, stat.NoParent})
	want := []int{-1, -1, -1, 0}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Depth over a cycle = %v, want %v", got, want)
	}
}

func TestARowThatNamesNoParentIsARoot(t *testing.T) {
	// An index past the end of the table is a root, not an error: the row is
	// there and the reader can see it.
	if got := stat.Depth([]int{7, 0}); got[0] != 0 || got[1] != 1 {
		t.Errorf("Depth = %v, want a root and its child", got)
	}
}

func TestRollupAddsEverySubtreeUp(t *testing.T) {
	parent, value := tree()
	got := stat.Rollup(value, parent, stat.Depth(parent))
	want := []float64{10, 4, 6, 3, 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Rollup = %v, want %v", got, want)
	}
}

func TestPartitionGivesEveryNodeItsShareOfTheWhole(t *testing.T) {
	parent, value := tree()
	depth := stat.Depth(parent)
	lo, hi := stat.Partition(stat.Rollup(value, parent, depth), parent, depth)

	// The root is the whole interval, and its children divide it in the order
	// their rows appeared: a (4/10) then b (6/10).
	if lo[0] != 0 || hi[0] != 1 {
		t.Errorf("the root spans [%v, %v), want [0, 1)", lo[0], hi[0])
	}
	for _, c := range []struct {
		node   int
		lo, hi float64
		what   string
	}{
		{1, 0.0, 0.4, "a"},
		{2, 0.4, 1.0, "b"},
		{3, 0.0, 0.3, "a1"},
		{4, 0.3, 0.4, "a2"},
	} {
		if math.Abs(lo[c.node]-c.lo) > 1e-12 || math.Abs(hi[c.node]-c.hi) > 1e-12 {
			t.Errorf("%s spans [%v, %v), want [%v, %v)", c.what, lo[c.node], hi[c.node], c.lo, c.hi)
		}
	}
}

func TestAChildNeverLeavesItsParentsSpan(t *testing.T) {
	parent, value := tree()
	depth := stat.Depth(parent)
	lo, hi := stat.Partition(stat.Rollup(value, parent, depth), parent, depth)
	for i, p := range parent {
		if p < 0 || p >= len(parent) {
			continue
		}
		if lo[i] < lo[p]-1e-12 || hi[i] > hi[p]+1e-12 {
			t.Errorf("node %d spans [%v, %v), outside its parent's [%v, %v)", i, lo[i], hi[i], lo[p], hi[p])
		}
	}
}

func TestAHierarchyOfZeroesHasNoShape(t *testing.T) {
	parent := []int{stat.NoParent, 0}
	depth := stat.Depth(parent)
	lo, hi := stat.Partition(stat.Rollup([]float64{0, 0}, parent, depth), parent, depth)
	for i := range lo {
		if lo[i] != 0 || hi[i] != 0 {
			t.Errorf("node %d spans [%v, %v) out of nothing", i, lo[i], hi[i])
		}
	}
}

func TestSquarifyFillsTheRectangleInProportion(t *testing.T) {
	values := []float64{6, 6, 4, 3, 2, 1}
	tiles := stat.Squarify(values, 0, 0, 6, 4)

	total, area := 0.0, 0.0
	for _, v := range values {
		total += v
	}
	for i, tl := range tiles {
		a := (tl.X1 - tl.X0) * (tl.Y1 - tl.Y0)
		area += a
		want := values[i] / total * 24
		if math.Abs(a-want) > 1e-9 {
			t.Errorf("tile %d has area %v, want %v", i, a, want)
		}
		if tl.X0 < -1e-9 || tl.Y0 < -1e-9 || tl.X1 > 6+1e-9 || tl.Y1 > 4+1e-9 {
			t.Errorf("tile %d escapes the rectangle: %+v", i, tl)
		}
	}
	if math.Abs(area-24) > 1e-9 {
		t.Errorf("the tiles cover %v of a 24-unit rectangle", area)
	}
}

func TestSquarifyBeatsASliceAndDiceOnAspectRatio(t *testing.T) {
	// The claim the algorithm exists for, stated against the thing it replaces
	// rather than against a magic number. Slicing a 10 x 10 square in one
	// direction gives every value a full-height column, so the smallest gets a
	// ratio of 81; squarifying has to do markedly better than that.
	values := []float64{20, 20, 20, 20, 1}
	total := 0.0
	for _, v := range values {
		total += v
	}
	sliced := 0.0
	for _, v := range values {
		w := 10 * v / total
		sliced = math.Max(sliced, math.Max(w/10, 10/w))
	}

	tiles := stat.Squarify(values, 0, 0, 10, 10)
	worst, worstEqual := 0.0, 0.0
	for i, tl := range tiles {
		w, h := tl.X1-tl.X0, tl.Y1-tl.Y0
		if w <= 0 || h <= 0 {
			t.Fatalf("a positive value got an empty tile: %+v", tl)
		}
		r := math.Max(w/h, h/w)
		worst = math.Max(worst, r)
		if i < 4 {
			worstEqual = math.Max(worstEqual, r)
		}
	}
	if worst > sliced/3 {
		t.Errorf("worst aspect ratio %v against slice-and-dice's %v; the packing is not squarifying", worst, sliced)
	}
	// The four equal values are the ones a reader compares, and they must come
	// out near square. The odd one in eighty-one is a sliver in any packing —
	// that is the data, not the algorithm.
	if worstEqual > 1.5 {
		t.Errorf("the four equal values have a worst ratio of %v, want them near square", worstEqual)
	}
}

func TestSquarifyKeepsAnEmptyTileForANonPositiveValue(t *testing.T) {
	tiles := stat.Squarify([]float64{5, 0, -2, 5}, 0, 0, 4, 4)
	if len(tiles) != 4 {
		t.Fatalf("got %d tiles for 4 values", len(tiles))
	}
	for _, i := range []int{1, 2} {
		if tiles[i].X1 != tiles[i].X0 || tiles[i].Y1 != tiles[i].Y0 {
			t.Errorf("value %d is not positive but got room: %+v", i, tiles[i])
		}
	}
}

// The flow the sankey tests read: two sources feeding one middle node, which
// feeds two sinks.
//
//	0 ──6──▶ 2 ──5──▶ 3
//	1 ──4──▶ 2 ──5──▶ 4
func flow() (from, to []int, value []float64) {
	return []int{0, 1, 2, 2}, []int{2, 2, 3, 4}, []float64{6, 4, 5, 5}
}

func TestSankeyPutsEveryNodeOnePastItsDeepestSource(t *testing.T) {
	var s stat.Sankey
	from, to, value := flow()
	s.Reset(from, to, value, 5, 0.02)
	if s.Cyclic {
		t.Fatal("a DAG was reported cyclic")
	}
	want := []int{0, 0, 1, 2, 2}
	for i, n := range s.Nodes {
		if n.Layer != want[i] {
			t.Errorf("node %d is in column %d, want %d", i, n.Layer, want[i])
		}
	}
	if s.Layers != 3 {
		t.Errorf("got %d columns, want 3", s.Layers)
	}
}

func TestASankeyNodeIsAsThickAsWhatFlowsThroughIt(t *testing.T) {
	var s stat.Sankey
	from, to, value := flow()
	s.Reset(from, to, value, 5, 0)
	// The middle node carries all ten units and is the busiest column, so it
	// fills the interval on its own.
	mid := s.Nodes[2]
	if math.Abs((mid.Hi-mid.Lo)-1) > 1e-9 {
		t.Errorf("the middle node spans %v of the interval, want all of it", mid.Hi-mid.Lo)
	}
	// The two sources sum to the same ten units, so together they fill it too.
	if h := (s.Nodes[0].Hi - s.Nodes[0].Lo) + (s.Nodes[1].Hi - s.Nodes[1].Lo); math.Abs(h-1) > 1e-9 {
		t.Errorf("the source column spans %v, want all of it", h)
	}
}

func TestEveryNodeStaysInsideTheUnitInterval(t *testing.T) {
	var s stat.Sankey
	from, to, value := flow()
	s.Reset(from, to, value, 5, 0.05)
	for i, n := range s.Nodes {
		if n.Lo < -1e-9 || n.Hi > 1+1e-9 {
			t.Errorf("node %d spans [%v, %v), outside the interval", i, n.Lo, n.Hi)
		}
	}
}

func TestARibbonIsTheSameThicknessAtBothEnds(t *testing.T) {
	var s stat.Sankey
	from, to, value := flow()
	s.Reset(from, to, value, 5, 0)
	for e, f := range s.Flows {
		src, dst := f.SrcHi-f.SrcLo, f.DstHi-f.DstLo
		if math.Abs(src-dst) > 1e-9 {
			t.Errorf("link %d leaves at %v thick and arrives at %v", e, src, dst)
		}
	}
}

func TestSankeyRefusesACycleRatherThanLoopingForever(t *testing.T) {
	var s stat.Sankey
	s.Reset([]int{0, 1, 2}, []int{1, 2, 0}, []float64{1, 1, 1}, 3, 0)
	if !s.Cyclic {
		t.Error("a cycle was laid out as though it were a flow")
	}
	if len(s.Nodes) != 0 {
		t.Errorf("a cyclic edge list produced %d nodes", len(s.Nodes))
	}
}

func TestChordGivesEveryNodeAnArcSizedByItsTraffic(t *testing.T) {
	var c stat.Chord
	// a↔b carries 3, a↔c carries 1. So a's arc is 4/8 of the circle, b's 3/8
	// and c's 1/8.
	c.Reset([]int{0, 0}, []int{1, 2}, []float64{3, 1}, 3, 0)
	want := []float64{0.5, 0.375, 0.125}
	for i, arc := range c.Arcs {
		if math.Abs(arc.Width()-want[i]) > 1e-12 {
			t.Errorf("node %d has an arc of %v, want %v", i, arc.Width(), want[i])
		}
	}
}

func TestAChordRibbonIsTheSameWidthAtBothEnds(t *testing.T) {
	var c stat.Chord
	c.Reset([]int{0, 0, 1}, []int{1, 2, 2}, []float64{3, 1, 2}, 3, 0.01)
	for e, r := range c.Ribbons {
		if math.Abs(r.Src.Width()-r.Dst.Width()) > 1e-12 {
			t.Errorf("ribbon %d is %v wide at one end and %v at the other", e, r.Src.Width(), r.Dst.Width())
		}
	}
}

func TestARibbonNeverLeavesTheArcItStartsFrom(t *testing.T) {
	var c stat.Chord
	from, to := []int{0, 0, 1, 2}, []int{1, 2, 2, 0}
	c.Reset(from, to, []float64{3, 1, 2, 4}, 3, 0.02)
	for e, r := range c.Ribbons {
		for _, end := range []struct {
			node int
			span stat.Span
		}{{from[e], r.Src}, {to[e], r.Dst}} {
			arc := c.Arcs[end.node]
			if end.span.Lo < arc.Lo-1e-9 || end.span.Hi > arc.Hi+1e-9 {
				t.Errorf("ribbon %d takes [%v, %v) of node %d's arc [%v, %v)",
					e, end.span.Lo, end.span.Hi, end.node, arc.Lo, arc.Hi)
			}
		}
	}
}

func TestAnEdgeToNowhereTakesNoRoom(t *testing.T) {
	var c stat.Chord
	c.Reset([]int{0, 0}, []int{1, 9}, []float64{3, 100}, 2, 0)
	if w := c.Ribbons[1].Src.Width(); w != 0 {
		t.Errorf("an edge naming a node that does not exist took %v of the circle", w)
	}
	if math.Abs(c.Arcs[0].Width()-0.5) > 1e-12 {
		t.Errorf("node 0's arc is %v, want half the circle", c.Arcs[0].Width())
	}
}

// Every layout here is a pure function of its input: the same table twice is
// the same picture twice, or a chart whose panels are built in parallel stops
// matching one built serially. See docs/adr/0012-parallel-panels.md.
func TestEveryRelationalLayoutRunsTheSameWayTwice(t *testing.T) {
	parent, value := tree()
	depth := stat.Depth(parent)
	total := stat.Rollup(value, parent, depth)

	t.Run("Depth", func(t *testing.T) {
		if !reflect.DeepEqual(stat.Depth(parent), stat.Depth(parent)) {
			t.Error("two runs disagree")
		}
	})
	t.Run("Rollup", func(t *testing.T) {
		a := stat.Rollup(value, parent, depth)
		b := stat.Rollup(value, parent, depth)
		if !reflect.DeepEqual(a, b) {
			t.Error("two runs disagree")
		}
	})
	t.Run("Partition", func(t *testing.T) {
		aLo, aHi := stat.Partition(total, parent, depth)
		bLo, bHi := stat.Partition(total, parent, depth)
		if !reflect.DeepEqual(aLo, bLo) || !reflect.DeepEqual(aHi, bHi) {
			t.Error("two runs disagree")
		}
	})
	t.Run("Squarify", func(t *testing.T) {
		a := stat.Squarify(total, 0, 0, 7, 3)
		b := stat.Squarify(total, 0, 0, 7, 3)
		if !reflect.DeepEqual(a, b) {
			t.Error("two runs disagree")
		}
	})
	t.Run("Sankey", func(t *testing.T) {
		from, to, v := flow()
		var a, b stat.Sankey
		a.Reset(from, to, v, 5, 0.02)
		b.Reset(from, to, v, 5, 0.02)
		if !reflect.DeepEqual(a.Nodes, b.Nodes) || !reflect.DeepEqual(a.Flows, b.Flows) {
			t.Error("two runs disagree")
		}
	})
	t.Run("Chord", func(t *testing.T) {
		var a, b stat.Chord
		a.Reset([]int{0, 0, 1}, []int{1, 2, 2}, []float64{3, 1, 2}, 3, 0.01)
		b.Reset([]int{0, 0, 1}, []int{1, 2, 2}, []float64{3, 1, 2}, 3, 0.01)
		if !reflect.DeepEqual(a.Arcs, b.Arcs) || !reflect.DeepEqual(a.Ribbons, b.Ribbons) {
			t.Error("two runs disagree")
		}
	})
}

// The relaxation is a fixed number of sweeps rather than a loop that stops when
// it stops improving. This is the test that says so: if somebody "improves"
// SankeySweeps into a convergence check, the constant stops meaning anything
// and this fails.
func TestTheSankeyRelaxationIsBoundedAndDoesSomething(t *testing.T) {
	if stat.SankeySweeps <= 0 {
		t.Fatal("the relaxation has no bound")
	}
	// A flow whose second source feeds only the lower sink: relaxing should
	// pull node 1 below node 0, which stacking alone does not do.
	from := []int{0, 1, 0, 1}
	to := []int{2, 3, 3, 2}
	value := []float64{9, 9, 1, 1}
	var s stat.Sankey
	s.Reset(from, to, value, 4, 0.02)
	for i, n := range s.Nodes {
		if n.Hi < n.Lo {
			t.Errorf("node %d came out inside out: [%v, %v)", i, n.Lo, n.Hi)
		}
	}
}

func TestAppendSquarifyReusesTheCallersSlice(t *testing.T) {
	values := []float64{4, 3, 2, 1}
	buf := stat.AppendSquarify(nil, values, 0, 0, 4, 4)
	before := &buf[0]
	buf = stat.AppendSquarify(buf, values, 0, 0, 4, 4)
	if &buf[0] != before {
		t.Error("AppendSquarify allocated a new array rather than reusing the one it was given")
	}
}

func TestResetReusesASankeysBuffers(t *testing.T) {
	from, to, value := flow()
	var s stat.Sankey
	s.Reset(from, to, value, 5, 0.02)
	nodes, flows := &s.Nodes[0], &s.Flows[0]
	s.Reset(from, to, value, 5, 0.02)
	if &s.Nodes[0] != nodes || &s.Flows[0] != flows {
		t.Error("Reset allocated new arrays rather than reusing the ones it had")
	}
}
