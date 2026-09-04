package facet_test

import (
	"errors"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
)

func table() data.Source {
	return data.NewTable().
		String("region", []string{"north", "north", "south", "south", "east"}).
		String("tier", []string{"free", "pro", "free", "pro", "free"}).
		Float64("x", []float64{0, 1, 2, 3, 4}).
		Float64("y", []float64{1, 2, 3, 4, 5})
}

func line(src data.Source) geom.Geom {
	return geom.Line(src, geom.X("x"), geom.Y("y"))
}

// rows counts what a panel's data layer actually holds, by training a scale on
// it and asking how wide the domain came out.
func rowsOf(t *testing.T, g geom.Geom) int {
	t.Helper()
	f, ok := g.(geom.Faceter)
	if !ok {
		return -1
	}
	return f.Source().Len()
}

func TestWrapMakesOnePanelPerValue(t *testing.T) {
	panels, rows, cols, err := facet.Wrap("region").Split([]geom.Geom{line(table())})
	if err != nil {
		t.Fatal(err)
	}
	if len(panels) != 3 {
		t.Fatalf("got %d panels, want 3", len(panels))
	}
	if rows*cols < 3 {
		t.Errorf("grid is %dx%d, too small for 3 panels", rows, cols)
	}
	want := map[string]int{"north": 2, "south": 2, "east": 1}
	for _, p := range panels {
		if got := rowsOf(t, p.Layers[0]); got != want[p.Strip] {
			t.Errorf("panel %q holds %d rows, want %d", p.Strip, got, want[p.Strip])
		}
	}
}

// Panel order follows the data, which is the only ordering that works the same
// for text, numbers and times — and the only one a caller can control without
// a second API.
func TestWrapKeepsFirstAppearanceOrder(t *testing.T) {
	panels, _, _, err := facet.Wrap("region").Split([]geom.Geom{line(table())})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"north", "south", "east"}
	for i, p := range panels {
		if p.Strip != want[i] {
			t.Errorf("panel %d is %q, want %q", i, p.Strip, want[i])
		}
	}
}

func TestWrapRespectsAColumnCount(t *testing.T) {
	panels, rows, cols, err := facet.Wrap("region", facet.Columns(1)).Split([]geom.Geom{line(table())})
	if err != nil {
		t.Fatal(err)
	}
	if cols != 1 || rows != 3 {
		t.Errorf("grid is %dx%d, want 3x1", rows, cols)
	}
	for i, p := range panels {
		if p.Row != i || p.Col != 0 {
			t.Errorf("panel %d at (%d,%d)", i, p.Row, p.Col)
		}
	}
}

func TestWrapDefaultsToARoughlySquareGrid(t *testing.T) {
	for _, tc := range []struct{ n, cols int }{{1, 1}, {2, 2}, {3, 2}, {4, 2}, {5, 3}, {9, 3}, {10, 4}} {
		keys := make([]string, tc.n)
		xs := make([]float64, tc.n)
		for i := range keys {
			keys[i] = string(rune('a' + i))
			xs[i] = float64(i)
		}
		src := data.NewTable().String("k", keys).Float64("x", xs).Float64("y", xs)
		_, rows, cols, err := facet.Wrap("k").Split([]geom.Geom{line(src)})
		if err != nil {
			t.Fatal(err)
		}
		if cols != tc.cols {
			t.Errorf("%d panels wrapped into %d columns, want %d", tc.n, cols, tc.cols)
		}
		if rows*cols < tc.n {
			t.Errorf("%d panels do not fit in %dx%d", tc.n, rows, cols)
		}
	}
}

func TestGridCrossesTwoColumns(t *testing.T) {
	panels, rows, cols, err := facet.Grid("region", "tier").Split([]geom.Geom{line(table())})
	if err != nil {
		t.Fatal(err)
	}
	if rows != 3 || cols != 2 {
		t.Errorf("grid is %dx%d, want 3x2", rows, cols)
	}
	// east has no "pro" row, so that cell is a hole rather than empty axes.
	if len(panels) != 5 {
		t.Errorf("got %d panels, want 5 — the empty combination should be a hole", len(panels))
	}
}

// Each key is named once: columns across the top, rows down the side.
func TestGridNamesEachKeyOnce(t *testing.T) {
	panels, _, cols, err := facet.Grid("region", "tier").Split([]geom.Geom{line(table())})
	if err != nil {
		t.Fatal(err)
	}
	strips, rights := 0, 0
	for _, p := range panels {
		if p.Strip != "" {
			strips++
			if p.Row != 0 {
				t.Errorf("a top strip appeared on row %d", p.Row)
			}
		}
		if p.RightStrip != "" {
			rights++
			if p.Col != cols-1 {
				t.Errorf("a side strip appeared in column %d", p.Col)
			}
		}
	}
	if strips != 2 {
		t.Errorf("%d top strips, want one per column", strips)
	}
	if rights != 2 {
		t.Errorf("%d side strips, want one per row that reaches the last column", rights)
	}
}

// A layer with no rows is furniture, not data: it belongs on every panel.
func TestLayersWithoutDataAreDrawnInEveryPanel(t *testing.T) {
	limit := geom.HLine(3)
	panels, _, _, err := facet.Wrap("region").Split([]geom.Geom{line(table()), limit})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range panels {
		if len(p.Layers) != 2 {
			t.Fatalf("panel %q has %d layers, want 2", p.Strip, len(p.Layers))
		}
		if p.Layers[1] != limit {
			t.Errorf("panel %q did not get the annotation unchanged", p.Strip)
		}
	}
}

// A layer that simply lacks the facet column is not being split, so it is
// drawn whole rather than emptied.
func TestALayerWithoutTheFacetColumnIsDrawnWhole(t *testing.T) {
	other := line(data.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {0, 1}}))
	panels, _, _, err := facet.Wrap("region").Split([]geom.Geom{line(table()), other})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range panels {
		if p.Layers[1] != other {
			t.Errorf("panel %q did not get the unfacetable layer unchanged", p.Strip)
		}
	}
}

// Panels share their colour scale, which is what makes one colourbar describe
// all of them — and what makes a colour mean the same thing in every panel.
func TestPanelsShareTheirColourScale(t *testing.T) {
	cs := scale.Sequential(nil)
	src := data.NewTable().
		String("region", []string{"north", "south"}).
		Float64("x", []float64{0, 1}).
		Float64("y", []float64{0, 1}).
		Float64("v", []float64{10, 20})
	g := geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.ColorBy("v", cs))

	panels, _, _, err := facet.Wrap("region").Split([]geom.Geom{g})
	if err != nil {
		t.Fatal(err)
	}
	// Every panel trains before any guide is read, which is the order render
	// works in — and the reason the guides agree at all.
	for _, p := range panels {
		if err := p.Layers[0].Train(scale.Linear(), scale.Linear()); err != nil {
			t.Fatal(err)
		}
	}
	var keys []string
	for _, p := range panels {
		guided, ok := p.Layers[0].(geom.Guided)
		if !ok {
			t.Fatalf("panel %q lost its colour guide", p.Strip)
		}
		got, ok := guided.ColorGuide()
		if !ok {
			t.Fatalf("panel %q reports no colour guide", p.Strip)
		}
		keys = append(keys, got.Key())
	}
	if len(keys) != 2 || keys[0] != keys[1] {
		t.Errorf("panels describe different colour guides: %v", keys)
	}
	// Training one panel's data has to move the shared domain, or the panels
	// would be coloured on different scales.
	if lo, hi := cs.Domain(); lo != 10 || hi != 20 {
		t.Errorf("shared colour domain = %v..%v, want 10..20", lo, hi)
	}
}

func TestSplitReportsAMissingColumn(t *testing.T) {
	_, _, _, err := facet.Wrap("nope").Split([]geom.Geom{line(table())})
	if !errors.Is(err, facet.ErrNoColumn) {
		t.Errorf("err = %v, want ErrNoColumn", err)
	}
}

func TestSplitRejectsAnEmptySpec(t *testing.T) {
	if _, _, _, err := (&facet.Spec{}).Split([]geom.Geom{line(table())}); err == nil {
		t.Error("a spec with no column split anyway")
	}
	var nilSpec *facet.Spec
	if _, _, _, err := nilSpec.Split(nil); err == nil {
		t.Error("a nil spec split anyway")
	}
}

func TestFreeScalesAreReported(t *testing.T) {
	for _, tc := range []struct {
		opt  facet.Option
		x, y bool
	}{
		{facet.FreeX(), true, false},
		{facet.FreeY(), false, true},
		{facet.Free(), true, true},
	} {
		x, y := facet.Wrap("region", tc.opt).FreeScales()
		if x != tc.x || y != tc.y {
			t.Errorf("FreeScales = %v/%v, want %v/%v", x, y, tc.x, tc.y)
		}
	}
	if x, y := facet.Wrap("region").FreeScales(); x || y {
		t.Error("scales are free by default; they should be shared")
	}
}

func TestColumnsIgnoresNonsense(t *testing.T) {
	_, _, cols, err := facet.Wrap("region", facet.Columns(0), facet.Columns(-2)).
		Split([]geom.Geom{line(table())})
	if err != nil {
		t.Fatal(err)
	}
	if cols < 1 {
		t.Errorf("cols = %d", cols)
	}
}

// Faceting over a numeric column gives one panel per distinct number, named
// the way a categorical axis names it.
func TestFacetOverANumericColumn(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{
		"run": {1, 1, 2},
		"x":   {0, 1, 2},
		"y":   {0, 1, 2},
	})
	panels, _, _, err := facet.Wrap("run").Split([]geom.Geom{line(src)})
	if err != nil {
		t.Fatal(err)
	}
	if len(panels) != 2 || panels[0].Strip != "1" || panels[1].Strip != "2" {
		t.Errorf("panels = %v", strips(panels))
	}
}

func strips(ps []facet.Panel) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Strip
	}
	return out
}
