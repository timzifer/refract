package geom

import (
	"errors"
	"fmt"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// The four marks that read an edge table rather than a pair of axes — [Treemap],
// [Icicle], [Sankey] and [Arc] — share everything in this file: how a name
// becomes a node, how a node takes a colour, and how a layout in the unit
// square reaches the plot.
//
// What they do not share is the coordinate system, and that is the point. Each
// places its layout in the unit square and hands the result to the coord, so an
// [Icicle] under [github.com/timzifer/refract/coord.Polar] is a sunburst and an
// [Arc] under one is a chord diagram — two more charts and no more marks. See
// docs/adr/0039-relational-layouts.md.

// ErrNotContinuous reports a positional scale that cannot carry a layout.
//
// A mark that places its own geometry needs an axis it can put a fraction on.
// An ordinal scale has slots rather than positions, so every node would land in
// slot zero — drawn on top of each other rather than refused, which is the
// failure this error exists to turn into a sentence.
var ErrNotContinuous = errors.New("refract/geom: this mark places its own layout and needs a continuous axis")

// ErrCyclic reports an edge table that runs in a circle where it must not.
// A flow that returns to where it came from has no column to stand in, and a
// hierarchy that is its own ancestor has no root to be measured against.
var ErrCyclic = errors.New("refract/geom: this mark needs an edge table with no cycle in it")

// interner assigns every distinct name an index, in the order the names first
// appear in the table.
//
// That order is the one thing every layout downstream depends on: which column
// a sankey's node stands in, which way round a chord diagram goes, and which
// entry of the palette each node takes. Reading it out of a map's iteration
// order would make a chart built on several goroutines differ from one built on
// on one — see docs/adr/0012-parallel-panels.md — so the map is only ever asked
// whether it has seen a name, never what it holds.
//
// The map is cleared rather than replaced between frames, so a chart redrawn
// every frame keeps its buckets.
type interner struct {
	at   map[string]int
	keys []string
	row  []int
}

func (in *interner) reset() {
	in.keys, in.row = in.keys[:0], in.row[:0]
	if in.at == nil {
		in.at = make(map[string]int, 16)
	}
	clear(in.at)
}

// of returns name's node index, creating it on first sight and remembering the
// row that named it so that a hit can be reported against the caller's table.
func (in *interner) of(name string, row int) int {
	if j, seen := in.at[name]; seen {
		return j
	}
	j := len(in.keys)
	in.keys = append(in.keys, name)
	in.row = append(in.row, row)
	in.at[name] = j
	return j
}

func (in *interner) count() int { return len(in.keys) }

// edges is a resolved edge table: one row per link, the two nodes it joins and
// what it carries. It is what [Sankey] and [Arc] read.
type edges struct {
	interner
	from, to []int
	val      []float64
	ones     []float64
	s        series
}

func (e *edges) reset(src data.Source, c config) error {
	if src == nil {
		return errors.New("refract/geom: nil data source")
	}
	if c.fromCol == "" || c.toCol == "" {
		return fmt.Errorf("%w: this mark reads an edge list; name both ends with geom.From and geom.To", ErrNoColumn)
	}
	fromLabels, ok := data.Labels(src, c.fromCol)
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoColumn, c.fromCol)
	}
	toLabels, ok := data.Labels(src, c.toCol)
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoColumn, c.toCol)
	}
	if len(fromLabels) != len(toLabels) {
		return errLength(c.fromCol, c.toCol, len(fromLabels), len(toLabels))
	}
	n := len(fromLabels)

	e.interner.reset()
	e.from, e.to = grow(e.from, n), grow(e.to, n)
	for i := range n {
		// Both ends of a row are interned before the next row is read, so the
		// node order is the order a reader meets the names going down the
		// table.
		e.from[i] = e.of(fromLabels[i], i)
		e.to[i] = e.of(toLabels[i], i)
	}

	var err error
	if e.val, e.ones, err = magnitudes(src, c, n, e.val, e.ones); err != nil {
		return err
	}
	return e.colors(src, c, n)
}

func (e *edges) colors(src data.Source, c config, n int) error {
	e.s = series{origin: data.Origins(src)}
	if c.colorCol == "" || c.colorScale == nil {
		return nil
	}
	cs, err := colorColumn(src, c)
	if err != nil {
		return err
	}
	if len(cs) != n {
		return errLength(c.fromCol, c.colorCol, n, len(cs))
	}
	e.s.c = cs
	return nil
}

// tree is a resolved hierarchy: one row per node, the node above it, and the
// layout that falls out of the two. It is what [Treemap] and [Icicle] read.
type tree struct {
	interner
	parent, depth []int
	val, total    []float64
	ones          []float64
	lo, hi        []float64
	s             series
}

func (t *tree) reset(src data.Source, c config) error {
	if src == nil {
		return errors.New("refract/geom: nil data source")
	}
	if c.idCol == "" {
		return fmt.Errorf("%w: this mark reads a hierarchy; name the node column with geom.ID", ErrNoColumn)
	}
	ids, ok := data.Labels(src, c.idCol)
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoColumn, c.idCol)
	}
	n := len(ids)
	var parents []string
	if c.parentCol != "" {
		if parents, ok = data.Labels(src, c.parentCol); !ok {
			return fmt.Errorf("%w: %q", ErrNoColumn, c.parentCol)
		}
		if len(parents) != n {
			return errLength(c.idCol, c.parentCol, n, len(parents))
		}
	}

	// Every row is a node, so the table's own order is the node order and a
	// node's index is its row. A name that appears twice is refused rather than
	// merged: two rows claiming one node have two values, and picking one of
	// them would draw a number nobody wrote.
	t.interner.reset()
	for i, id := range ids {
		if t.of(id, i) != i {
			return fmt.Errorf("refract/geom: %q appears twice in %q, and a hierarchy's node names have to be distinct", id, c.idCol)
		}
	}

	t.parent = grow(t.parent, n)
	for i := range n {
		t.parent[i] = -1
		if parents == nil || parents[i] == "" {
			continue
		}
		// A parent nothing declares is a root, not an error: the row is in the
		// table and the reader can see it. Only a self-reference is dropped,
		// because a node cannot be under itself.
		if j, seen := t.at[parents[i]]; seen && j != i {
			t.parent[i] = j
		}
	}

	var err error
	if t.val, t.ones, err = magnitudes(src, c, n, t.val, t.ones); err != nil {
		return err
	}
	return t.colors(src, c, n)
}

func (t *tree) colors(src data.Source, c config, n int) error {
	t.s = series{origin: data.Origins(src)}
	if c.colorCol == "" || c.colorScale == nil {
		return nil
	}
	cs, err := colorColumn(src, c)
	if err != nil {
		return err
	}
	if len(cs) != n {
		return errLength(c.idCol, c.colorCol, n, len(cs))
	}
	t.s.c = cs
	return nil
}

// magnitudes reads the [Value] column, or hands back a column of ones when the
// layer named none: an unweighted graph is a real chart, and every edge in it
// counts the same.
func magnitudes(src data.Source, c config, n int, val, ones []float64) ([]float64, []float64, error) {
	if c.valCol == "" {
		ones = grow(ones, n)
		for i := range ones {
			ones[i] = 1
		}
		return ones, ones, nil
	}
	v, ok := src.Float64Column(c.valCol)
	if !ok {
		return val, ones, fmt.Errorf("%w: %q, which has to be a number", ErrNoColumn, c.valCol)
	}
	if len(v) != n {
		return val, ones, errLength(c.valCol, c.valCol, n, len(v))
	}
	return v, ones, nil
}

// trainUnit is what a mark that places its own layout does with the two scales:
// it tells them the layout runs from zero to one and asks for nothing else.
//
// The axes then describe a unit square, which is the honest answer — the
// numbers a treemap draws are areas of the whole, and a sankey's are fractions
// of its busiest column. It is also what makes the coordinate stage work: a
// polar coord reads the same pair and wraps the first of them round a circle.
//
// An ordinal scale is refused rather than filled in, per [ErrNotContinuous].
func trainUnit(x, y scale.Scale) error {
	for _, s := range [...]scale.Scale{x, y} {
		if _, ordinal := s.(scale.Categorical); ordinal {
			return fmt.Errorf("%w: give it a scale.Linear", ErrNotContinuous)
		}
	}
	x.Train(0, 1)
	y.Train(0, 1)
	return nil
}

// nodeColor resolves the colour of one node.
//
// It is [config.groupColor] for a layer whose series are nodes: an explicit
// colour wins, a discrete scale is asked by name so a node keeps its colour
// across facet panels, and otherwise the palette is walked from the layer's own
// entry.
func (c config) nodeColor(f Frame, keys []string, i int) ir.Color {
	if c.color != nil {
		return *c.color
	}
	if c.fill != nil {
		return *c.fill
	}
	if d, ok := scale.Discrete(c.colorScale); ok && i < len(keys) {
		return d.ColorOf(keys[i])
	}
	pal := f.Theme.Palette
	if len(pal) == 0 {
		pal = theme.Light.Palette
	}
	return pal.At(f.Index + i)
}

// nodeLegends is one entry per node, which is what [Legender] is for: a
// relational layer is a layer of many things the way a grouped one is, and the
// names are the reading.
//
// A layer painted from a continuous colour scale contributes none — it has a
// colourbar instead, and a swatch per node would be a ladder of colours nothing
// is painted with. That is the seam ADR 0020 draws.
func (c config) nodeLegends(f Frame, keys []string) []LegendEntry {
	if c.colorScale != nil {
		if _, discrete := scale.Discrete(c.colorScale); !discrete {
			return nil
		}
	}
	if c.color != nil || c.fill != nil {
		return nil
	}
	out := make([]LegendEntry, 0, len(keys))
	for i, k := range keys {
		out = append(out, LegendEntry{Label: k, Color: c.nodeColor(f, keys, i), Kind: SwatchBox})
	}
	return out
}

// padded shrinks a data-space box by half the gap on every side, so that
// neighbouring shapes are separated by the whole gap.
//
// A box narrower than twice the gap keeps half its width rather than
// disappearing: a treemap cell too small for its margin still says something is
// there, and a layout that swallowed its smallest cells would be lying about
// what is in the data.
func padded(x0, y0, x1, y1, gap float64) (float64, float64, float64, float64) {
	x0, x1 = shrink(x0, x1, gap)
	y0, y1 = shrink(y0, y1, gap)
	return x0, y0, x1, y1
}

func shrink(lo, hi, gap float64) (float64, float64) {
	if gap <= 0 || hi <= lo {
		return lo, hi
	}
	h := gap / 2
	if room := (hi - lo) / 4; h > room {
		h = room
	}
	return lo + h, hi - h
}

// arcSteps is how many sub-edges a span of the angular axis is drawn in.
//
// A polar coord joins two points the short way round, which is the right answer
// for a mark narrower than half the circle and the wrong one for a mark wider
// than it. Splitting the span keeps every step short, and under a Cartesian
// coord the extra points are collinear and cost a few line segments nobody can
// see. See coord.Coord.Edge.
func arcSteps(span float64) int {
	const longest = 0.15 // of the circle
	n := int(span/longest) + 1
	return n
}

// edgeAlong appends the path of a span of the layout at one height, split so
// that a polar coord never has to guess which way round to go.
func edgeAlong(p *ir.Path, cd coord.Coord, x, y func(float64) float32, lo, hi, at float64) {
	steps := arcSteps(hi - lo)
	prev := ir.Point{X: x(lo), Y: y(at)}
	for k := 1; k <= steps; k++ {
		t := lo + (hi-lo)*float64(k)/float64(steps)
		next := ir.Point{X: x(t), Y: y(at)}
		cd.Edge(p, prev, next)
		prev = next
	}
}

// boxColors resolves one colour per shape: the colour scale's answer where the
// layer named a column, and the node's own otherwise.
//
// Both write into the scratch's one colour buffer, which is safe because they
// are alternatives rather than layers — a mark is painted from a column or from
// the palette, never from both.
func (sc *scratch) boxColors(c config, f Frame, s series, keys []string, rows []int) []ir.Color {
	if cols := sc.colorsFor(c, s, rows); cols != nil {
		for i := range cols {
			cols[i] = c.fillOf(cols[i], 1)
		}
		return cols
	}
	sc.cols = grow(sc.cols, len(rows))
	for k, i := range rows {
		sc.cols[k] = c.fillOf(c.nodeColor(f, keys, i), 1)
	}
	return sc.cols
}

// fillBoxes paints a layout's boxes through the coord, batched by colour and
// one subpath per box.
//
// The subpath is the part that matters: a pointer lands on the shape it is
// inside rather than on the sheet the batch was drawn as, which is what makes
// a treemap cell and a sankey node hit-testable at all. See
// docs/adr/0015-hit-testing.md.
func fillBoxes(b ir.Backend, sc *scratch, cd coord.Coord, c config, f Frame, s series, keys []string, rects []ir.Rect, rows []int) error {
	cols := sc.boxColors(c, f, s, keys, rows)
	for _, run := range sc.groupByRect(rects, cols, nil) {
		if run.color.A == 0 {
			continue
		}
		sc.fill.Reset()
		for _, r := range run.rects {
			area(&sc.fill, cd, r)
		}
		b.FillPath(&sc.fill, ir.Solid(run.color), ir.NonZero)
	}
	return nil
}

// reportBoxes tells the frame which source row is behind each shape, at the
// shape's middle.
//
// A layout's box is bounded on both axes, so unlike a bar there is no end that
// means more than the other — the same reasoning [Rect] reports its cells by.
func reportBoxes(sc *scratch, f Frame, rects []ir.Rect, rows []int, s series) {
	cd := f.Coords()
	sc.pts = grow(sc.pts, len(rects))
	for i, r := range rects {
		sc.pts[i] = cd.Point((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	}
	f.Marks(sc.pts, sc.sourceRows(s, rows))
}

// oneNodeLegend is the single entry a relational layer contributes when every
// node is one colour, which is the only time it has one thing to say.
//
// A layer that walks the palette has an entry per node instead — see
// [config.nodeLegends] — and one painted from a ramp has a colourbar.
func oneNodeLegend(c config, f Frame, err error) (LegendEntry, bool) {
	if err != nil || (c.color == nil && c.fill == nil) {
		return LegendEntry{}, false
	}
	return LegendEntry{Label: c.labelForNodes(), Color: c.fillOf(c.colorFor(f), 1), Kind: SwatchBox}, true
}

// labelForNodes names a relational layer after what it measures, since it has
// no Y column to be named after.
func (c config) labelForNodes() string {
	if c.label != "" {
		return c.label
	}
	return c.valCol
}
