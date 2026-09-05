package geom

import (
	"fmt"
	"math"
	"sort"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/stat"
	"github.com/timzifer/refract/theme"
)

// Groups, and the position adjustments defined over them.
//
// A long table with a series column is one layer, not N. That is what
// [GroupBy] says, and it is the shape a stacked bar, a grouped bar and a
// streamgraph all need anyway: stacking is defined over the groups *within* a
// layer, because a layer that had to discover its siblings would need a
// coordinating stage above the geoms that this design does not have. See
// docs/adr/0019-position-adjustments.md.
//
// The split of work is exact and both halves are forced. The offsets are
// derived in Train, because the axis has to describe the totals — a stacked
// bar reaches the cumulative sum, and an axis trained on the individual values
// would let the tallest stack run off the top of it. The geometry is built in
// Build, because that is where geometry lives, and because what a chart's axis
// says must not depend on how wide the chart is.

// Stacking is how the segments of one slot are stacked, or [NoStack] when they
// are not.
type Stacking uint8

// The position adjustments. NoStack is the default for a layer with no groups;
// a grouped [Bar] or [Area] stacks from zero unless it is told otherwise,
// which is what a reader expects of a bar chart with a series column and what
// Vega-Lite does with the same document.
const (
	// NoStack draws every group from the layer's own baseline. Groups then
	// overplot unless [Dodge] separates them, which is right for a line or a
	// scatter and rarely what a bar wants.
	NoStack Stacking = iota

	// StackZero stacks each group on the one before it, from the baseline up.
	StackZero

	// StackFill stacks the groups and scales each slot to fill the axis, so
	// the chart reads as proportions rather than as magnitudes. The axis then
	// runs from 0 to 1.
	StackFill

	// StackSilhouette centres each slot on the baseline: the ThemeRiver.
	StackSilhouette

	// StackWiggle offsets each slot to minimise how much the interior
	// boundaries climb: the streamgraph. See [stat.StackOffsets].
	StackWiggle
)

// Ordering is the order the groups of a layer are stacked and listed in.
type Ordering uint8

// The group orders. OrderAppearance is the default and is the convention the
// rest of refract already uses: a facet's panels, a categorical axis and a
// boxplot's groups all come out in order of first appearance in the source
// table. ADR 0012 is why it is not negotiable — a parallel render must be
// byte-identical to a serial one, and an order that depends on hashing is an
// order that depends on scheduling.
const (
	// OrderAppearance is the order the groups first appear in the table.
	OrderAppearance Ordering = iota

	// OrderValue puts the largest group at the bottom of the stack, measured
	// by its total.
	OrderValue

	// OrderInsideOut puts the groups that peak earliest in the middle of the
	// stack and works outwards, which is the ordering a streamgraph is read
	// with. Byron & Wattenberg pair it with [StackWiggle].
	OrderInsideOut
)

// GroupBy splits a layer's rows into series by the values of a column.
//
// One layer over a long table then draws one mark set per distinct value —
// N lines, N stacked segments, N bars in a slot — and contributes one legend
// entry per group rather than one for the layer. It is the prerequisite for
// every position adjustment: stacking is defined over the groups of a layer.
//
// The column may hold text, numbers or instants; whichever it is, the group
// key is its formatted label, exactly as [data.Labels] spells it, so a group
// key and a categorical axis tick for the same value are the same string.
//
// Colour comes from the layer's colour scale when it was given a discrete one
// — [scale.Qualitative] — and from the theme's palette otherwise. A faceted
// chart wants the scale: a palette index is per panel, and a panel missing one
// group would shift the colours of every group after it.
func GroupBy(col string) Option { return func(c *config) { c.groupCol = col } }

// Stack sets the position adjustment for a grouped layer. It has no effect on
// a layer with no [GroupBy]: one series stacked on itself is the series.
//
// A stacked layer takes its baseline from the adjustment rather than from
// [Baseline], because that is what the adjustment decides: [StackZero] grows
// from zero, [StackSilhouette] centres each slot, and [StackWiggle] puts the
// baseline wherever the interior boundaries are flattest.
func Stack(s Stacking) Option {
	return func(c *config) { c.stack, c.stackSet = s, true }
}

// Dodge places the groups of a slot side by side instead of stacking them,
// leaving padding of the slot blank between them — 0.1 is a tenth of the
// slot's width, and the default when [Dodge] is asked for with a padding
// outside [0, 1).
//
// It is the other answer to the same question stacking answers, and a layer
// has one or the other: a dodged layer is not stacked, whatever [Stack] said.
// Comparing the parts is what dodging is for and comparing the totals is what
// stacking is for; a chart that needs both needs two layers.
func Dodge(padding float64) Option {
	return func(c *config) { c.dodge, c.dodgePad = true, padding }
}

// Order sets the order a layer's groups are stacked and listed in.
func Order(o Ordering) Option { return func(c *config) { c.order = o } }

// WidthBy reads a bar's width from a column instead of from the spacing of the
// data, in the axis's own units.
//
// It is what a mosaic reads as: a slot whose width is itself a value. Combine
// it with [Stack] and X positions the caller has already accumulated — a
// marimekko is one layer whose X column holds each column's centre and whose
// width column holds its share — and the vertical proportions come out of
// [StackFill].
//
// A row with no width, or a negative one, falls back to the slot the spacing
// of the data implies.
func WidthBy(col string) Option { return func(c *config) { c.widthCol = col } }

// Legender is implemented by a layer that contributes more than one legend
// entry.
//
// A pie has N slices inside one layer. So does a stacked bar, a grouped bar, a
// waffle and anything painted from a discrete colour scale — and
// [Geom.Legend]'s single entry cannot name them. It is an *optional* interface
// rather than a second method on [Geom] because most geoms have exactly one
// entry to contribute and would otherwise all need a stub: the same argument
// [Guided] already made for colourbars, and the reason the extension API can
// still freeze at v1.0 with three methods on Geom. See
// docs/adr/0020-discrete-colour-and-multi-entry-legends.md.
type Legender interface {
	// Legends returns the entries this layer contributes, in legend order.
	//
	// It is the whole answer: a layer that implements it is not asked for
	// [Geom.Legend] as well, so a layer with one entry returns that one entry
	// here, and returning none means the layer stays out of the legend. The
	// geoms in this package answer it through [LegendsOr], which is that
	// fallback written once.
	Legends(f Frame) []LegendEntry
}

// LegendsOr is a layer's own entries, or its single one where it has no series
// and no categories to name.
//
// Every geom here that can contribute many entries can also contribute one —
// the same [Bar] draws a stack of five series or one plain row of bars — and
// this is the one place that decides which.
func LegendsOr(g Geom, f Frame, many []LegendEntry) []LegendEntry {
	if len(many) > 0 {
		return many
	}
	if e, ok := g.Legend(f); ok {
		return []LegendEntry{e}
	}
	return nil
}

// Legends returns the legend entries of a layer: its own list where it has
// one, and its single entry otherwise.
//
// It is what render calls, so that the preference between the two interfaces
// is written down once.
func Legends(g Geom, f Frame) []LegendEntry {
	if l, ok := g.(Legender); ok {
		return l.Legends(f)
	}
	if e, ok := g.Legend(f); ok {
		return []LegendEntry{e}
	}
	return nil
}

// groups is a layer's rows split into series, and — for a layer that stacks —
// the bounds the adjustment gave each row.
//
// It is held by the geom and refilled by every Train rather than allocated per
// frame: a chart redrawn over a live table trains as often as it draws, and
// the group index of a million-row column is exactly the kind of thing that
// must not be allocated per frame. The buffers below are the same ones the
// allocation gate is about.
type groups struct {
	keys  []string // group labels, in registration order
	of    []int    // group index per row
	rows  [][]int  // the rows of each group, in table order
	order []int    // group indices in stacking order
	rank  []int    // rank[g] is where group g sits in the stack

	lo, hi []float64 // the stacked bounds per row, NaN for a row not drawn
	stack  Stacking

	// The working buffers. They are reused, never handed out.
	at      map[string]int
	posAt   map[float64]int
	pos     []float64   // distinct X positions, ascending
	slotOf  []int       // position index per row
	vals    [][]float64 // group × position
	ordered [][]float64 // the same rows, in stacking order
	base    []float64   // one baseline per position
	sums    []float64   // the running sum per position
	totals  []float64   // the column total per position
	weight  []float64   // one number per group, for ordering
	peaks   []int
	tops    []int
	bottoms []int
	counts  []int
	flat    []int // one backing array behind every group's row list
	ok      []bool
}

// grouped reports whether this layer has groups at all. A nil *groups is the
// ungrouped case and every reader here tolerates it, which is what keeps an
// ordinary layer's code path exactly what it was.
func (gs *groups) grouped() bool { return gs != nil && len(gs.keys) > 0 }

// stacked reports whether the rows carry adjusted bounds.
func (gs *groups) stacked() bool { return gs.grouped() && gs.stack != NoStack }

// count is how many groups there are, and 1 for a layer with none — a dodge
// over no groups is one bar in the middle of its slot.
func (gs *groups) count() int {
	if !gs.grouped() {
		return 1
	}
	return len(gs.keys)
}

// train splits the rows and derives the adjustment.
//
// stack is the layer's effective adjustment: what [Stack] asked for, or the
// mark's own default when it asked for nothing. The scales are trained on the
// derived bounds rather than on the raw column, because that is what will be
// drawn — the same reason [Boxplot] trains on its quantiles.
func (gs *groups) train(src data.Source, s series, c config, x, y scale.Scale, stack Stacking) error {
	gs.reset()
	if c.groupCol == "" {
		return nil
	}
	labels, ok := data.Labels(src, c.groupCol)
	if !ok {
		return fmt.Errorf("%w: %q names the groups and the table has no such column", ErrNoColumn, c.groupCol)
	}
	if len(labels) != len(s.x) {
		return fmt.Errorf("refract/geom: group column %q has %d rows and the plotted columns have %d",
			c.groupCol, len(labels), len(s.x))
	}

	gs.of = grow(gs.of, len(labels))
	for i, l := range labels {
		j, seen := gs.at[l]
		if !seen {
			j = len(gs.keys)
			gs.keys = append(gs.keys, l)
			gs.at[l] = j
		}
		gs.of[i] = j
	}
	gs.split(len(labels))

	// The rows this layer can draw, computed once. Both the cumulative sum and
	// the drawing traversal read this answer: a NaN the sum skipped but the
	// draw did not would shift every segment above it.
	gs.ok = grow(gs.ok, len(s.x))
	for i := range s.x {
		gs.ok[i] = defined(x, s.x[i]) && defined(y, s.y[i])
	}

	gs.stack = stack
	if stack == NoStack {
		gs.orderGroups(c.order, s, nil)
		return nil
	}
	gs.adjust(s, c.order, stack)

	// The axis has to describe the totals, so it is trained on the bounds
	// rather than on the column. The baseline goes in as well: a stack that
	// starts at zero is read as the distance from zero.
	trainColumn(y, gs.lo)
	trainColumn(y, gs.hi)
	y.Train(0)
	return nil
}

// split lists the rows of each group, so that nothing downstream has to scan
// the whole table once per group.
//
// It counts first and fills afterwards, out of one backing array sliced up per
// group — the same shape [data.GroupBy] uses, and for the same reason: growing
// each list by appending would allocate once per doubling per group, which is
// a cost that rises with the row count for no reason.
func (gs *groups) split(n int) {
	g := len(gs.keys)
	gs.counts = grow(gs.counts, g)
	for i := range gs.counts {
		gs.counts[i] = 0
	}
	for _, at := range gs.of[:n] {
		gs.counts[at]++
	}
	gs.flat = grow(gs.flat, n)
	if cap(gs.rows) >= g {
		gs.rows = gs.rows[:g]
	} else {
		gs.rows = append(gs.rows[:cap(gs.rows)], make([][]int, g-cap(gs.rows))...)
	}
	off := 0
	for j, c := range gs.counts[:g] {
		gs.rows[j] = gs.flat[off : off : off+c]
		off += c
	}
	for i, at := range gs.of[:n] {
		gs.rows[at] = append(gs.rows[at], i)
	}
}

func (gs *groups) reset() {
	gs.keys, gs.stack = gs.keys[:0], NoStack
	gs.lo, gs.hi = gs.lo[:0], gs.hi[:0]
	gs.order, gs.rank = gs.order[:0], gs.rank[:0]
	if gs.at == nil {
		gs.at = make(map[string]int, 8)
	}
	clear(gs.at)
}

// adjust fills the per-row bounds.
//
// The positions are the distinct X values in ascending order, because a
// silhouette and a wiggle read each column against the one before it and
// "before" is an order on the axis rather than on the table.
func (gs *groups) adjust(s series, order Ordering, stack Stacking) {
	if gs.posAt == nil {
		gs.posAt = make(map[float64]int, 16)
	}
	clear(gs.posAt)
	gs.pos = gs.pos[:0]
	gs.slotOf = grow(gs.slotOf, len(s.x))
	for i := range s.x {
		if !gs.ok[i] {
			gs.slotOf[i] = -1
			continue
		}
		p, seen := gs.posAt[s.x[i]]
		if !seen {
			p = len(gs.pos)
			gs.pos = append(gs.pos, s.x[i])
			gs.posAt[s.x[i]] = p
		}
		gs.slotOf[i] = p
	}
	sort.Float64s(gs.pos)
	// Sorting moved the positions, so the index every row was given has to be
	// resolved again against the sorted list.
	for p, v := range gs.pos {
		gs.posAt[v] = p
	}
	for i := range s.x {
		if gs.slotOf[i] >= 0 {
			gs.slotOf[i] = gs.posAt[s.x[i]]
		}
	}

	n, m := len(gs.keys), len(gs.pos)
	gs.vals = growRows(gs.vals, n, m)
	for g := range n {
		for p := range m {
			gs.vals[g][p] = 0
		}
	}
	for i := range s.x {
		if p := gs.slotOf[i]; p >= 0 {
			gs.vals[gs.of[i]][p] += s.y[i]
		}
	}

	gs.orderGroups(order, s, gs.vals)

	// A 100 % chart is the same stack over shares rather than over values,
	// which is a property of the numbers rather than of the baseline — so it
	// happens here, and stat is left with the one question that is genuinely
	// about where the bottom of the stack goes.
	gs.totals = grow(gs.totals, m)
	for p := range m {
		t := 0.0
		for g := range n {
			t += gs.vals[g][p]
		}
		gs.totals[p] = t
		if stack == StackFill && t != 0 {
			for g := range n {
				gs.vals[g][p] /= t
			}
		}
	}

	// stat sees the groups in stacking order, because the wiggle is a function
	// of what lies below each band.
	gs.ordered = growRows(gs.ordered, n, 0)[:0]
	for _, g := range gs.order {
		gs.ordered = append(gs.ordered, gs.vals[g])
	}
	gs.base = stat.AppendStackOffsets(gs.base, stackMode(stack), gs.ordered)

	// Walk the stack in order, running a sum per position, and hand each row
	// the pair of bounds its own segment ended up with. The row carries its own
	// value rather than its group's total at that position: two rows of one
	// group in one slot are two segments that happen to be the same colour, and
	// they still have to add up to the group.
	gs.sums = grow(gs.sums, m)
	copy(gs.sums, gs.base[:m])
	gs.lo, gs.hi = grow(gs.lo, len(s.x)), grow(gs.hi, len(s.x))
	for i := range s.x {
		gs.lo[i], gs.hi[i] = math.NaN(), math.NaN()
	}
	for _, g := range gs.order {
		for _, i := range gs.rows[g] {
			p := gs.slotOf[i]
			if p < 0 {
				continue
			}
			v := s.y[i]
			if stack == StackFill {
				if gs.totals[p] == 0 {
					continue
				}
				v /= gs.totals[p]
			}
			gs.lo[i], gs.hi[i] = gs.sums[p], gs.sums[p]+v
			gs.sums[p] += v
		}
	}
}

// orderGroups fills the stacking order. vals is nil for a layer that is not
// stacking, where only [OrderValue] has anything to measure and it measures
// the raw column.
func (gs *groups) orderGroups(mode Ordering, s series, vals [][]float64) {
	n := len(gs.keys)
	gs.order, gs.rank = grow(gs.order, n), grow(gs.rank, n)
	for g := range n {
		gs.order[g] = g
	}
	switch mode {
	case OrderValue:
		gs.weight = grow(gs.weight, n)
		for g := range n {
			gs.weight[g] = gs.totalOf(g, s, vals)
		}
		w := gs.weight[:n]
		sort.SliceStable(gs.order, func(a, b int) bool {
			return w[gs.order[a]] > w[gs.order[b]]
		})
	case OrderInsideOut:
		gs.insideOut(s, vals)
	}
	for r, g := range gs.order {
		gs.rank[g] = r
	}
}

func (gs *groups) totalOf(g int, s series, vals [][]float64) float64 {
	if vals != nil {
		t := 0.0
		for _, v := range vals[g] {
			t += v
		}
		return t
	}
	t := 0.0
	for _, i := range gs.rows[g] {
		if gs.ok[i] {
			t += s.y[i]
		}
	}
	return t
}

// insideOut orders the groups so that the ones peaking earliest sit in the
// middle of the stack and the rest alternate outwards, balancing the two
// halves by weight. It is the ordering a streamgraph wants: the bands that
// rise first are the ones a reader follows, and the middle of a stream is
// where a band keeps its shape.
func (gs *groups) insideOut(s series, vals [][]float64) {
	n := len(gs.keys)
	gs.peaks, gs.weight = grow(gs.peaks, n), grow(gs.weight, n)
	for g := range n {
		gs.peaks[g], gs.weight[g] = gs.peakOf(g, s, vals), gs.totalOf(g, s, vals)
	}
	peaks := gs.peaks[:n]
	byPeak := gs.order[:n]
	sort.SliceStable(byPeak, func(a, b int) bool { return peaks[byPeak[a]] < peaks[byPeak[b]] })

	// Two piles, and each group goes on the lighter one — so the stack is
	// balanced about its middle rather than growing in one direction.
	top, bottom := 0.0, 0.0
	gs.tops, gs.bottoms = gs.tops[:0], gs.bottoms[:0]
	for _, g := range byPeak {
		if top < bottom {
			top += gs.weight[g]
			gs.tops = append(gs.tops, g)
		} else {
			bottom += gs.weight[g]
			gs.bottoms = append(gs.bottoms, g)
		}
	}
	out := gs.order[:0]
	for i := len(gs.bottoms) - 1; i >= 0; i-- {
		out = append(out, gs.bottoms[i])
	}
	gs.order = append(out, gs.tops...)
}

// peakOf is the position at which a group is largest, which is what orders the
// groups before they are dealt out inside-out.
func (gs *groups) peakOf(g int, s series, vals [][]float64) int {
	best, at := math.Inf(-1), 0
	if vals != nil {
		for p, v := range vals[g] {
			if v > best {
				best, at = v, p
			}
		}
		return at
	}
	for _, i := range gs.rows[g] {
		if gs.ok[i] && s.y[i] > best {
			best, at = s.y[i], i
		}
	}
	return at
}

// stackFor resolves the layer's adjustment: what [Stack] asked for, or the
// mark's own default for a grouped layer that asked for nothing.
//
// A grouped bar or area stacks by default because that is what a reader
// expects of a bar chart with a series column, and what the same document
// means in Vega-Lite. A layer with no groups has nothing to stack.
func (c config) stackFor(def Stacking) Stacking {
	switch {
	case c.groupCol == "":
		return NoStack
	case c.dodge:
		// Comparing the parts and comparing the totals are different
		// questions, and a layer answers one of them.
		return NoStack
	case c.stackSet:
		return c.stack
	}
	return def
}

func stackMode(s Stacking) stat.StackMode {
	switch s {
	case StackFill:
		return stat.StackFill
	case StackSilhouette:
		return stat.StackSilhouette
	case StackWiggle:
		return stat.StackWiggle
	}
	return stat.StackZero
}

// bounds returns the series a stacked layer actually draws: the upper bound as
// Y and the lower one as Y2.
//
// Every traversal downstream — plottable, project, the band's two edges — then
// works on the adjustment without knowing there was one, which is what keeps
// stacking out of the drawing code.
func (gs *groups) bounds(s series) series {
	if !gs.stacked() {
		return s
	}
	out := s
	out.y, out.y2 = gs.hi, gs.lo
	return out
}

// groupOf reports which group row i belongs to, and 0 for a layer with no
// groups.
func (gs *groups) groupOf(i int) int {
	if !gs.grouped() || i < 0 || i >= len(gs.of) {
		return 0
	}
	return gs.of[i]
}

// slotIndex is where row i's group sits when the slot is divided between the
// groups: its rank in the stacking order, so that [Order] moves a dodged bar
// as well as a stacked one.
func (gs *groups) slotIndex(i int) int {
	if !gs.grouped() {
		return 0
	}
	return gs.rank[gs.of[i]]
}

// eachGroup calls fn once per series, in stacking order, with that group's
// rows gathered into a contiguous series of their own.
//
// It is how a layer whose mark is a connected shape draws groups: a line and a
// band are written over a run of rows, and the rows of one group are scattered
// through the table. The gather is into the caller's own scratch, so N series
// cost one buffer rather than N allocations.
//
// The scratch is passed in rather than taken here, and that is not tidiness:
// a scratch is held for one Build and released at the end of it, so a helper
// that took a second one would leave two objects with *disjoint* buffers
// cycling through one pool. Whichever came back as the gatherer next frame
// would have to grow the drawer's buffers and vice versa — a real per-row
// allocation, on a path whose whole point is not having one.
func eachGroup(sc *scratch, gs *groups, s series, fn func(seg series, group int) error) error {
	bounds := gs.bounds(s)
	for _, grp := range gs.order {
		rows := gs.rows[grp]
		if len(rows) == 0 {
			continue
		}
		if err := fn(sc.gather(bounds, rows), grp); err != nil {
			return err
		}
	}
	return nil
}

// groupRun is the cells of one series, batched so that a grouped layer costs
// one drawing call per series rather than one per row.
type groupRun struct {
	group int
	rects []ir.Rect
	// offs is how far each of the run's cells is broken out of the middle of
	// the coord, index for index with rects. It is empty for a layer that is
	// not broken out — read it through [offsetAt].
	offs []ir.Point
}

// groupRuns batches rects by series, in stacking order. rows[i] is the source
// row behind rects[i], which is what says which series it belongs to, and
// offs[i] how far it is broken out — nil when nothing is.
func (sc *scratch) groupRuns(gs *groups, rects []ir.Rect, rows []int, offs []ir.Point) []groupRun {
	n := 0
	for _, g := range gs.order {
		if n == len(sc.gruns) {
			sc.gruns = append(sc.gruns, groupRun{})
		}
		sc.gruns[n].group = g
		sc.gruns[n].rects = sc.gruns[n].rects[:0]
		sc.gruns[n].offs = sc.gruns[n].offs[:0]
		n++
	}
	for i, r := range rects {
		j := gs.rank[gs.of[rows[i]]]
		sc.gruns[j].rects = append(sc.gruns[j].rects, r)
		if offs != nil {
			sc.gruns[j].offs = append(sc.gruns[j].offs, offs[i])
		}
	}
	return sc.gruns[:n]
}

// growRows grows a slice of slices to n rows of m columns, reusing both.
func growRows(buf [][]float64, n, m int) [][]float64 {
	if cap(buf) >= n {
		buf = buf[:n]
	} else {
		buf = append(buf[:cap(buf)], make([][]float64, n-cap(buf))...)
	}
	for i := range buf {
		buf[i] = grow(buf[i], m)
	}
	return buf
}

// --- what the groups mean when it is time to draw --------------------------

// groupColor resolves the colour of one group.
//
// A discrete colour scale is asked by label, so a group keeps its colour
// across facet panels and across two layers that share the scale. Without one
// the colour is the palette entry after the layer's own, which is right for a
// single grouped layer and is exactly what a facet cannot rely on.
//
// An explicit [Color] still wins and paints every group alike: naming a colour
// is a decision, and a default — even a more useful one — does not outrank it.
func (c config) groupColor(f Frame, gs *groups, g int) ir.Color {
	if c.color != nil {
		return *c.color
	}
	if d, ok := scale.Discrete(c.colorScale); ok && gs.grouped() {
		return d.ColorOf(gs.keys[g])
	}
	pal := f.Theme.Palette
	if len(pal) == 0 {
		pal = theme.Light.Palette
	}
	return pal.At(f.Index + g)
}

// groupDash walks the theme's dash ladder per group, so that a redundant
// encoding distinguishes the series of one layer the way it distinguishes two
// layers.
func (c config) groupDash(f Frame, g int) []float32 {
	if c.dashSet {
		return c.dash
	}
	return f.Theme.SeriesDash(f.Index + g)
}

func (c config) groupMarker(f Frame, g int) ir.Marker {
	if c.markerSet {
		return c.marker
	}
	if m, ok := f.Theme.SeriesMarker(f.Index + g); ok {
		return m
	}
	return c.marker
}

// legends is the shared multi-entry legend: one entry per group, or one per
// category of a discrete colour scale for a layer that paints per mark.
//
// It returns nil where there is nothing to say beyond the layer's own single
// entry, which is what tells [Legends] to fall back to [Geom.Legend].
func (c config) legends(f Frame, gs *groups, s series, kind SwatchKind) []LegendEntry {
	entry := func(label string, col ir.Color, g int) LegendEntry {
		e := LegendEntry{Label: label, Color: col, Kind: kind}
		switch kind {
		case SwatchLine:
			e.Dash, e.Width = c.groupDash(f, g), pick(c.width, f.Theme.LineWidth)
		case SwatchMarker:
			e.Marker = c.groupMarker(f, g)
		}
		return e
	}
	if gs.grouped() {
		out := make([]LegendEntry, 0, len(gs.keys))
		for _, g := range gs.order {
			out = append(out, entry(gs.keys[g], c.groupColor(f, gs, g), g))
		}
		return out
	}
	// A layer painted per mark from a discrete scale has one entry per
	// category rather than one for the layer — the same swatch could not stand
	// for eight colours.
	if d, ok := scale.Discrete(c.colorScale); ok && s.c != nil {
		labels := d.Labels()
		out := make([]LegendEntry, 0, len(labels))
		for i, l := range labels {
			out = append(out, entry(l, d.ColorOf(l), i))
		}
		return out
	}
	return nil
}

// dodgeSpan narrows a slot to one group's share of it.
//
// The slot is divided evenly and the padding is taken out of each share, so
// that the groups of one slot sit side by side with a gap between them and the
// slot itself keeps the width the axis gave it.
func dodgeSpan(x0, x1 float32, g, n int, padding float64) (float32, float32) {
	if n <= 1 {
		return x0, x1
	}
	if padding < 0 || padding >= 1 {
		padding = defaultDodgePadding
	}
	w := (x1 - x0) / float32(n)
	lo := x0 + w*float32(g)
	pad := w * float32(padding) / 2
	return lo + pad, lo + w - pad
}

// defaultDodgePadding is the gap between two dodged bars, as a fraction of one
// bar's share of the slot. A tenth is enough to read as a gap at the sizes a
// chart is drawn at and small enough that the bars still read as one group.
const defaultDodgePadding = 0.1
