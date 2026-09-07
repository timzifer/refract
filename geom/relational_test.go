package geom_test

import (
	"errors"
	"math"
	"testing"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// The relational and hierarchical marks. What these tests are about is the seam
// rather than the arithmetic — the layouts are pure functions with their own
// tests in package stat. What a geom has to get right is which axis it decides,
// that one shape is one subpath, that the coordinate stage is what turns an
// icicle into a sunburst, and which rows it claims to have drawn.

// disk is the hierarchy the treemap and icicle tests read: a root, two
// directories under it, and three files under those.
func disk() data.Source {
	return data.NewTable().
		String("path", []string{"/", "src", "docs", "a.go", "b.go", "readme"}).
		String("under", []string{"", "/", "/", "src", "src", "docs"}).
		Float64("bytes", []float64{0, 0, 0, 30, 10, 20})
}

// calls is the edge list the sankey and arc tests read.
func calls() data.Source {
	return data.NewTable().
		String("from", []string{"web", "web", "api", "api"}).
		String("to", []string{"api", "cache", "db", "queue"}).
		Float64("n", []float64{60, 20, 50, 10})
}

func relFrame(t *testing.T, g geom.Geom, c coord.Coord, w, h float32) (*irtest.Recorder, geom.Frame) {
	t.Helper()
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	area := ir.R(0, 0, w, h)
	if c == nil {
		c = coord.Cartesian()
	}
	c = c.Frame(area, x, y)
	return irtest.New(), geom.Frame{Area: area, X: x, Y: y, Coord: c, Theme: theme.Light}
}

// subpathsOf splits a filled path into its shapes. One shape per subpath is
// what makes each of them separately pointable — see docs/adr/0015-hit-testing.md.
func subpathsOf(t *testing.T, c irtest.Call) [][]ir.Point {
	t.Helper()
	if c.Path == nil {
		t.Fatal("a fill call carried no path")
	}
	var out [][]ir.Point
	var cur []ir.Point
	c.Path.Walk(func(op ir.PathOp, pts []ir.Point) {
		if op == ir.OpMoveTo && len(cur) > 0 {
			out = append(out, cur)
			cur = nil
		}
		cur = append(cur, pts...)
	})
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func shapesIn(t *testing.T, rec *irtest.Recorder) [][]ir.Point {
	t.Helper()
	var out [][]ir.Point
	for _, c := range rec.Filter("FillPath") {
		out = append(out, subpathsOf(t, c)...)
	}
	return out
}

func boundsOf(pts []ir.Point) ir.Rect {
	r := ir.Rect{Min: pts[0], Max: pts[0]}
	for _, p := range pts {
		r.Min.X, r.Min.Y = min(r.Min.X, p.X), min(r.Min.Y, p.Y)
		r.Max.X, r.Max.Y = max(r.Max.X, p.X), max(r.Max.Y, p.Y)
	}
	return r
}

// A treemap draws its leaves and nothing else: an internal node's rectangle is
// exactly the union of its children's, so painting it would be paint under
// paint.
func TestATreemapDrawsOneCellPerLeaf(t *testing.T) {
	g := geom.Treemap(disk(), geom.ID("path"), geom.Parent("under"), geom.Value("bytes"))
	rec, f := relFrame(t, g, nil, 300, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if got := len(shapesIn(t, rec)); got != 3 {
		t.Errorf("got %d cells for three files under two directories, want 3", got)
	}
}

// The area is the reading. Three files of 30, 10 and 20 bytes fill a panel in
// that proportion, whatever shape the packing gave them.
func TestATreemapCellsAreaIsItsShareOfTheWhole(t *testing.T) {
	g := geom.Treemap(disk(), geom.ID("path"), geom.Parent("under"), geom.Value("bytes"))
	rec, f := relFrame(t, g, nil, 300, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	total := 0.0
	areas := make([]float64, 0, 3)
	for _, s := range shapesIn(t, rec) {
		r := boundsOf(s)
		a := float64(r.Max.X-r.Min.X) * float64(r.Max.Y-r.Min.Y)
		areas = append(areas, a)
		total += a
	}
	if math.Abs(total-300*200) > 1 {
		t.Errorf("the cells cover %v of a 60000-unit panel", total)
	}
	// The packing decides which cell is which, so compare the set of shares.
	want := map[int]bool{50: true, 17: true, 33: true}
	for _, a := range areas {
		pct := int(math.Round(a / total * 100))
		if !want[pct] {
			t.Errorf("a cell is %d%% of the panel; the shares are 50, 33 and 17", pct)
		}
	}
}

// Padding is a gap rather than a stroke: a border drawn round a cell is ink a
// reader has to discount from the area they are comparing, and it hit-tests
// above the cell it outlines.
func TestTreemapPaddingSeparatesTheCellsWithoutInk(t *testing.T) {
	plain := geom.Treemap(disk(), geom.ID("path"), geom.Parent("under"), geom.Value("bytes"))
	rec, f := relFrame(t, plain, nil, 300, 200)
	if err := plain.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	full := 0.0
	for _, s := range shapesIn(t, rec) {
		r := boundsOf(s)
		full += float64(r.Max.X-r.Min.X) * float64(r.Max.Y-r.Min.Y)
	}

	padded := geom.Treemap(disk(), geom.ID("path"), geom.Parent("under"), geom.Value("bytes"), geom.Padding(0.02))
	rec2, f2 := relFrame(t, padded, nil, 300, 200)
	if err := padded.Build(rec2, f2); err != nil {
		t.Fatal(err)
	}
	gapped := 0.0
	for _, s := range shapesIn(t, rec2) {
		r := boundsOf(s)
		gapped += float64(r.Max.X-r.Min.X) * float64(r.Max.Y-r.Min.Y)
	}
	if gapped >= full {
		t.Errorf("padding left the cells covering %v, no less than the %v they covered without it", gapped, full)
	}
	if n := rec2.Count("StrokePath"); n != 0 {
		t.Errorf("a padded treemap stroked %d paths; the gap is meant to be room, not ink", n)
	}
}

// The milestone in one test: one icicle layer, two coords, two charts. A
// sunburst is not a second implementation of anything.
func TestASunburstIsAnIcicleUnderAPolarCoord(t *testing.T) {
	layer := func() geom.Geom {
		return geom.Icicle(disk(), geom.ID("path"), geom.Parent("under"), geom.Value("bytes"))
	}
	draw := func(c coord.Coord) *irtest.Recorder {
		t.Helper()
		g := layer()
		rec, f := relFrame(t, g, c, 300, 300)
		if err := g.Build(rec, f); err != nil {
			t.Fatal(err)
		}
		return rec
	}

	flat, round := draw(nil), draw(coord.Polar())
	if a, b := len(shapesIn(t, flat)), len(shapesIn(t, round)); a != b {
		t.Fatalf("the two coords drew %d and %d shapes out of one layer", a, b)
	}
	if curves(flat.Filter("FillPath")) != 0 {
		t.Error("a Cartesian icicle drew curves; its bands are rectangles")
	}
	if curves(round.Filter("FillPath")) == 0 {
		t.Error("a polar icicle drew no curves; a sunburst's rings are arcs")
	}
}

func curves(calls []irtest.Call) int {
	n := 0
	for _, c := range calls {
		c.Path.Walk(func(op ir.PathOp, _ []ir.Point) {
			if op == ir.OpCubicTo {
				n++
			}
		})
	}
	return n
}

// An icicle draws every node, root included: the depth is the reading, and a
// hierarchy with its levels missing is not one.
func TestAnIcicleDrawsEveryNode(t *testing.T) {
	g := geom.Icicle(disk(), geom.ID("path"), geom.Parent("under"), geom.Value("bytes"))
	rec, f := relFrame(t, g, nil, 300, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if got := len(shapesIn(t, rec)); got != 6 {
		t.Errorf("got %d bands for six rows, want one each", got)
	}
}

// The root is at the hub and the leaves are at the rim, which is what makes the
// polar reading a sunburst rather than a sunburst turned inside out.
func TestAnIciclesRootIsAtTheHub(t *testing.T) {
	g := geom.Icicle(disk(), geom.ID("path"), geom.Parent("under"), geom.Value("bytes"))
	rec, f := relFrame(t, g, nil, 300, 300)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	// Y is flipped under Cartesian, so the hub — data-space zero — is the
	// bottom of the panel, and the root band has to touch it.
	lowest := float32(math.Inf(-1))
	for _, s := range shapesIn(t, rec) {
		lowest = max(lowest, boundsOf(s).Max.Y)
	}
	if lowest < 299 {
		t.Errorf("the deepest band reaches y=%v of a 300-unit panel; the root should sit on the axis", lowest)
	}
}

// A sankey's columns come out of the edge list: a node stands one past the
// deepest source that reaches it.
func TestASankeyPutsItsNodesInColumns(t *testing.T) {
	g := geom.Sankey(calls(), geom.From("from"), geom.To("to"), geom.Value("n"))
	rec, f := relFrame(t, g, nil, 400, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	// The nodes are the last fill calls, drawn on top of the bands.
	var lefts []float32
	for _, s := range shapesIn(t, rec) {
		r := boundsOf(s)
		if r.Max.X-r.Min.X > 40 {
			continue // a band, not a node
		}
		lefts = append(lefts, r.Min.X)
	}
	if len(lefts) != 5 {
		t.Fatalf("got %d nodes, want web, api, cache, db and queue", len(lefts))
	}
	seen := map[int]bool{}
	for _, x := range lefts {
		seen[int(math.Round(float64(x)))] = true
	}
	if len(seen) != 3 {
		t.Errorf("the nodes stand at %d distinct positions, want three columns", len(seen))
	}
}

// A band leaves and arrives horizontally, which is what says the quantity did
// not change on the way: its control points sit halfway between the columns.
func TestASankeyDrawsABandPerLink(t *testing.T) {
	g := geom.Sankey(calls(), geom.From("from"), geom.To("to"), geom.Value("n"))
	rec, f := relFrame(t, g, nil, 400, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	bands := 0
	for _, s := range shapesIn(t, rec) {
		if r := boundsOf(s); r.Max.X-r.Min.X > 40 {
			bands++
		}
	}
	if bands != 4 {
		t.Errorf("got %d bands for four links", bands)
	}
	if curves(rec.Filter("FillPath")) == 0 {
		t.Error("the bands are straight; a sankey's link is an S-curve")
	}
}

// A flow that returns to where it came from has no column to stand in. Refusing
// it is a sentence the caller can read; drawing it would be an ordering nobody
// wrote.
func TestASankeyRefusesACycle(t *testing.T) {
	src := data.NewTable().
		String("from", []string{"a", "b", "c"}).
		String("to", []string{"b", "c", "a"}).
		Float64("n", []float64{1, 1, 1})
	g := geom.Sankey(src, geom.From("from"), geom.To("to"), geom.Value("n"))
	err := g.Train(scale.Linear(), scale.Linear())
	if !errors.Is(err, geom.ErrCyclic) {
		t.Errorf("Train over a cycle returned %v, want ErrCyclic", err)
	}
}

// The other half of the milestone: one arc layer, two coords.
func TestAChordDiagramIsAnArcUnderAPolarCoord(t *testing.T) {
	flat := geom.Arc(calls(), geom.From("from"), geom.To("to"), geom.Value("n"))
	rec, f := relFrame(t, flat, nil, 300, 300)
	if err := flat.Build(rec, f); err != nil {
		t.Fatal(err)
	}

	round := geom.Arc(calls(), geom.From("from"), geom.To("to"), geom.Value("n"), geom.Baseline(1))
	rec2, f2 := relFrame(t, round, coord.Polar(), 300, 300)
	if err := round.Build(rec2, f2); err != nil {
		t.Fatal(err)
	}

	if a, b := len(shapesIn(t, rec)), len(shapesIn(t, rec2)); a != b {
		t.Fatalf("the two coords drew %d and %d shapes out of one mark", a, b)
	}
	// Nine shapes: five nodes and four ribbons.
	if got := len(shapesIn(t, rec)); got != 9 {
		t.Errorf("got %d shapes, want five nodes and four ribbons", got)
	}
	if curves(rec.Filter("FillPath")) == 0 || curves(rec2.Filter("FillPath")) == 0 {
		t.Error("a ribbon is a pair of cubics in either coord")
	}
}

// Baseline is where the rail sits, and it is the whole difference between an
// arc diagram and a chord diagram.
func TestBaselineMovesTheRailToTheRim(t *testing.T) {
	low := geom.Arc(calls(), geom.From("from"), geom.To("to"), geom.Value("n"))
	rec, f := relFrame(t, low, nil, 300, 300)
	if err := low.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	high := geom.Arc(calls(), geom.From("from"), geom.To("to"), geom.Value("n"), geom.Baseline(1))
	rec2, f2 := relFrame(t, high, nil, 300, 300)
	if err := high.Build(rec2, f2); err != nil {
		t.Fatal(err)
	}
	// Y is flipped, so a rail at the data-space bottom is at the panel's
	// bottom and one at the top is at its top.
	if bottom(shapesIn(t, rec)) < 290 {
		t.Error("the default rail is not on the axis")
	}
	if top(shapesIn(t, rec2)) > 10 {
		t.Error("Baseline(1) did not move the rail to the far edge")
	}
}

func bottom(shapes [][]ir.Point) float32 {
	v := float32(math.Inf(-1))
	for _, s := range shapes {
		v = max(v, boundsOf(s).Max.Y)
	}
	return v
}

func top(shapes [][]ir.Point) float32 {
	v := float32(math.Inf(1))
	for _, s := range shapes {
		v = min(v, boundsOf(s).Min.Y)
	}
	return v
}

// An edge with no weight counts as one, so an unweighted graph draws without a
// Value column at all.
func TestAnUnweightedEdgeListStillDraws(t *testing.T) {
	src := data.NewTable().
		String("from", []string{"a", "b", "c"}).
		String("to", []string{"b", "c", "a"})
	g := geom.Arc(src, geom.From("from"), geom.To("to"))
	rec, f := relFrame(t, g, nil, 300, 200)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if got := len(shapesIn(t, rec)); got != 6 {
		t.Errorf("got %d shapes, want three nodes and three ribbons", got)
	}
}

// A mark that places its own layout needs an axis it can put a fraction on. An
// ordinal scale has slots, so every node would land in slot zero — drawn on top
// of each other rather than refused.
func TestALayoutMarkRefusesAnOrdinalAxis(t *testing.T) {
	for _, g := range []geom.Geom{
		geom.Treemap(disk(), geom.ID("path"), geom.Parent("under"), geom.Value("bytes")),
		geom.Icicle(disk(), geom.ID("path"), geom.Parent("under"), geom.Value("bytes")),
		geom.Sankey(calls(), geom.From("from"), geom.To("to"), geom.Value("n")),
		geom.Arc(calls(), geom.From("from"), geom.To("to"), geom.Value("n")),
	} {
		if err := g.Train(scale.Ordinal(), scale.Linear()); !errors.Is(err, geom.ErrNotContinuous) {
			t.Errorf("%T accepted an ordinal axis: %v", g, err)
		}
	}
}

// Both axes describe the unit square. That is the honest answer — what a
// treemap draws are shares of the whole — and it is what lets a polar coord
// read the same pair and wrap the first of them round a circle.
func TestALayoutMarkTrainsTheUnitSquare(t *testing.T) {
	g := geom.Sankey(calls(), geom.From("from"), geom.To("to"), geom.Value("n"))
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatal(err)
	}
	for _, s := range []scale.Scale{x, y} {
		if lo, hi := s.Domain(); lo != 0 || hi != 1 {
			t.Errorf("an axis runs %v..%v, want the unit interval", lo, hi)
		}
	}
}

// A node is what several rows have in common rather than a row of its own, so
// it reports none — which leaves a hit on it at row -1 rather than at a
// neighbouring row reported confidently. The links are the rows.
func TestASankeyReportsItsLinksAndNotItsNodes(t *testing.T) {
	g := geom.Sankey(calls(), geom.From("from"), geom.To("to"), geom.Value("n"))
	rec, f := relFrame(t, g, nil, 400, 200)
	var got []int
	f.Rows = rowsFunc(func(_ []ir.Point, rows []int) { got = append(got, rows...) })
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("reported %d marks for four links and five nodes, want 4", len(got))
	}
	for i, r := range got {
		if r != i {
			t.Errorf("link %d reports row %d", i, r)
		}
	}
}

// A relational layer is a layer of many things the way a grouped one is, and
// the node names are the reading.
func TestARelationalLayerNamesItsNodesInTheLegend(t *testing.T) {
	g := geom.Arc(calls(), geom.From("from"), geom.To("to"), geom.Value("n"))
	_, f := relFrame(t, g, nil, 300, 200)
	entries := geom.Legends(g, f)
	if len(entries) != 5 {
		t.Fatalf("got %d legend entries, want one per node", len(entries))
	}
	want := []string{"web", "api", "cache", "db", "queue"}
	for i, e := range entries {
		if e.Label != want[i] {
			t.Errorf("entry %d is %q, want %q — the order is the order the rows named them", i, e.Label, want[i])
		}
	}
}

// A layer painted from a ramp gets a colourbar instead of a ladder of swatches
// nothing is painted with, which is the seam ADR 0020 draws.
func TestARelationalLayerColouredByARampHasAGuideAndNoSwatches(t *testing.T) {
	src := data.NewTable().
		String("from", []string{"a", "b"}).
		String("to", []string{"b", "c"}).
		Float64("n", []float64{2, 3})
	cs := scale.Sequential(nil)
	g := geom.Arc(src, geom.From("from"), geom.To("to"), geom.Value("n"), geom.ColorBy("n", cs))
	_, f := relFrame(t, g, nil, 300, 200)
	if got := geom.Legends(g, f); len(got) != 0 {
		t.Errorf("a layer on a ramp contributed %d swatches", len(got))
	}
	if _, ok := g.(geom.Guided).ColorGuide(); !ok {
		t.Error("a layer on a ramp contributed no colourbar")
	}
}

// Two rows claiming one node have two values, and picking one of them would
// draw a number nobody wrote.
func TestAHierarchyRefusesADuplicateNode(t *testing.T) {
	src := data.NewTable().
		String("path", []string{"a", "a"}).
		String("under", []string{"", ""}).
		Float64("bytes", []float64{1, 2})
	g := geom.Treemap(src, geom.ID("path"), geom.Parent("under"), geom.Value("bytes"))
	if err := g.Train(scale.Linear(), scale.Linear()); err == nil {
		t.Error("a hierarchy with two rows for one node was accepted")
	}
}

// A layer redrawn twice draws the same thing: the buffers are reused and the
// order is the table's, never a map's.
func TestARelationalLayerRedrawnTwiceDrawsTheSameThing(t *testing.T) {
	for _, make := range []func() geom.Geom{
		func() geom.Geom {
			return geom.Treemap(disk(), geom.ID("path"), geom.Parent("under"), geom.Value("bytes"))
		},
		func() geom.Geom {
			return geom.Sankey(calls(), geom.From("from"), geom.To("to"), geom.Value("n"))
		},
		func() geom.Geom {
			return geom.Arc(calls(), geom.From("from"), geom.To("to"), geom.Value("n"))
		},
	} {
		g := make()
		rec, f := relFrame(t, g, nil, 300, 200)
		if err := g.Build(rec, f); err != nil {
			t.Fatal(err)
		}
		first := rec.Trace()
		rec.Reset()
		if err := g.Build(rec, f); err != nil {
			t.Fatal(err)
		}
		second := rec.Trace()
		if len(first) != len(second) {
			t.Fatalf("%T drew %d calls and then %d", g, len(first), len(second))
		}
		for i := range first {
			if first[i] != second[i] {
				t.Fatalf("%T call %d: %q then %q", g, i, first[i], second[i])
			}
		}
	}
}
