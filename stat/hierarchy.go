package stat

// A hierarchy is a self-referential edge table: every row is a node, and the
// row names the node that is its parent. That is the shape
// docs/chart-types.md argues a treemap, an icicle and a sunburst all read, and
// the three functions here are what turn it into geometry.
//
// Every one of them takes the parent link as an index rather than as a name.
// Naming is the caller's business, and deliberately so: which node is "first"
// decides the order of everything downstream, and that order has to come from
// the order the rows appeared in the source table rather than from a map
// (docs/adr/0012-parallel-panels.md). A package that knows about numbers and
// nothing else cannot intern a string without inventing an order, so it does
// not try — the geom interns, and hands indices down.
//
// The three run in sequence, each reading the one before: [Depth] says how far
// every node is from a root, [Rollup] adds every subtree up, and [Partition]
// turns the totals into a span of the unit interval per node. An icicle draws
// those spans against depth directly; a treemap feeds them to [Squarify] one
// sibling group at a time.

// NoParent is the parent of a root. Any negative index means the same thing;
// this is the one to write.
const NoParent = -1

// Depth returns how far each node is from a root: zero for a root, one more
// than its parent for everyone else.
//
// parent[i] is the index of i's parent, or [NoParent] — or any index outside
// the list, which means the same. A node that cannot be reached from a root is
// on a cycle, and comes back as -1 rather than as a guess or a panic: a
// hierarchy with a cycle is not a hierarchy, and the caller is the one that can
// say so in terms of its own data.
//
// It sweeps one level at a time rather than recursing, so a hierarchy deep
// enough to blow a stack does not, and it costs one pass per level. That is a
// bounded traversal rather than an iteration to convergence: it visits every
// reachable node exactly once and stops when a level adds nobody, which makes
// it a pure function of its input in the sense ADR 0012 requires.
func Depth(parent []int) []int { return AppendDepth(nil, parent) }

// AppendDepth is [Depth] writing into dst, which it truncates and grows as
// needed.
func AppendDepth(dst []int, parent []int) []int {
	n := len(parent)
	dst = dst[:0]
	for i := range n {
		if isRoot(parent, i, n) {
			dst = append(dst, 0)
			continue
		}
		dst = append(dst, -1)
	}
	for d := 0; ; d++ {
		grew := false
		for i := range n {
			if dst[i] >= 0 {
				continue
			}
			if p := parent[i]; dst[p] == d {
				dst[i] = d + 1
				grew = true
			}
		}
		if !grew {
			return dst
		}
	}
}

// isRoot reports whether i has no parent inside the list. An index out of
// range is a root rather than an error: a table that names a parent nothing
// else declares has said the node is a top-level one, and refusing to draw it
// would lose a row the reader can see in the data.
func isRoot(parent []int, i, n int) bool {
	p := parent[i]
	return p < 0 || p >= n || p == i
}

// Rollup returns each node's total: its own value plus every descendant's.
//
// value[i] is what row i carries on its own, which for the usual hierarchy —
// where only the leaves are measured — is zero for every internal node. depth
// comes from [Depth]; a node it left at -1 is on a cycle and contributes to
// nothing, including itself.
//
// A negative value is summed as it stands rather than clamped. A tree that
// mixes signs has no sensible area to draw and the caller is where that is
// worth saying; silently taking the absolute value would draw a picture the
// numbers do not support.
func Rollup(value []float64, parent, depth []int) []float64 {
	return AppendRollup(nil, value, parent, depth)
}

// AppendRollup is [Rollup] writing into dst, which it truncates and grows as
// needed. It is the form a geom calls, because a chart redrawn every frame
// should not allocate a total per node per frame.
func AppendRollup(dst []float64, value []float64, parent, depth []int) []float64 {
	n := min(len(value), len(parent), len(depth))
	dst = dst[:0]
	for i := range n {
		dst = append(dst, value[i])
	}
	// Deepest first, so that a node's own subtree is complete before it is
	// added into its parent's.
	for d := maxDepth(depth); d > 0; d-- {
		for i := range n {
			if depth[i] != d {
				continue
			}
			if p := parent[i]; p >= 0 && p < n {
				dst[p] += dst[i]
			}
		}
	}
	for i := range n {
		if depth[i] < 0 {
			dst[i] = 0
		}
	}
	return dst
}

// Partition returns each node's half-open span [lo, hi) of the unit interval:
// the fraction of the whole hierarchy its subtree occupies, and where that
// fraction sits.
//
// It is the layout an icicle and a sunburst draw directly — a node's span
// across, its depth out — and the ranges a treemap hands to [Squarify] one
// sibling group at a time. total comes from [Rollup].
//
// Siblings are laid out in index order, which is the order their rows appeared
// in the source table. Children fill their parent's span from its start; a
// parent that carries a value of its own beyond its children's keeps the
// remainder, which is what makes an unaccounted-for share visible rather than
// invisible.
//
// Every span is zero when the totals sum to nothing, rather than a division by
// zero: a hierarchy of zeroes has no shape, and the caller draws nothing.
func Partition(total []float64, parent, depth []int) (lo, hi []float64) {
	return AppendPartition(nil, nil, total, parent, depth)
}

// AppendPartition is [Partition] writing into lo and hi, which it truncates and
// grows as needed.
func AppendPartition(lo, hi []float64, total []float64, parent, depth []int) ([]float64, []float64) {
	n := min(len(total), len(parent), len(depth))
	lo, hi = lo[:0], hi[:0]
	for range n {
		lo, hi = append(lo, 0), append(hi, 0)
	}

	grand := 0.0
	for i := range n {
		if depth[i] == 0 {
			grand += total[i]
		}
	}
	if !(grand > 0) {
		return lo, hi
	}

	// hi doubles as the cursor into each parent's span while its children are
	// being placed, and is overwritten with the real far edge at the end. That
	// saves a third buffer for state that lives for one pass — the same reason
	// stat's Append forms exist at all.
	cursor := 0.0
	for i := range n {
		if depth[i] == 0 {
			lo[i] = cursor
			cursor += total[i] / grand
		}
	}
	for d := 1; d <= maxDepth(depth); d++ {
		for i := range n {
			if depth[i] == d-1 {
				hi[i] = lo[i]
			}
		}
		for i := range n {
			if depth[i] != d {
				continue
			}
			p := parent[i]
			lo[i] = hi[p]
			hi[p] += total[i] / grand
		}
	}
	for i := range n {
		if depth[i] < 0 {
			lo[i], hi[i] = 0, 0
			continue
		}
		hi[i] = lo[i] + total[i]/grand
	}
	return lo, hi
}

func maxDepth(depth []int) int {
	m := 0
	for _, d := range depth {
		if d > m {
			m = d
		}
	}
	return m
}
