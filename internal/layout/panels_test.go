package layout_test

import (
	"testing"

	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/internal/layout"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/theme"
)

func grid(rows, cols int, panels ...layout.Panel) layout.Grid {
	return layout.Grid{
		Canvas: ir.R(0, 0, 900, 600),
		Theme:  theme.Light,
		Rows:   rows,
		Cols:   cols,
		Panels: panels,
	}
}

func numbered(row, col int) layout.Panel {
	return layout.Panel{
		Row: row, Col: col,
		XLabels: []string{"0", "5", "10"},
		YLabels: []string{"0", "50", "100"},
	}
}

// Alignment is the entire point: a value at the same position must mean the
// same thing in every panel, which needs the panels to be the same size and
// lined up on both axes.
func TestPanelsInAColumnShareTheirHorizontalExtent(t *testing.T) {
	g := grid(2, 2, numbered(0, 0), numbered(0, 1), numbered(1, 0), numbered(1, 1))
	got := layout.Panels(g, irtest.New())

	if got.Areas[0].Min.X != got.Areas[2].Min.X || got.Areas[0].Max.X != got.Areas[2].Max.X {
		t.Errorf("column 0 panels span %v and %v", got.Areas[0], got.Areas[2])
	}
	if got.Areas[1].Min.X != got.Areas[3].Min.X || got.Areas[1].Max.X != got.Areas[3].Max.X {
		t.Errorf("column 1 panels span %v and %v", got.Areas[1], got.Areas[3])
	}
}

func TestPanelsInARowShareTheirVerticalExtent(t *testing.T) {
	g := grid(2, 2, numbered(0, 0), numbered(0, 1), numbered(1, 0), numbered(1, 1))
	got := layout.Panels(g, irtest.New())

	if got.Areas[0].Min.Y != got.Areas[1].Min.Y || got.Areas[0].Max.Y != got.Areas[1].Max.Y {
		t.Errorf("row 0 panels span %v and %v", got.Areas[0], got.Areas[1])
	}
}

func TestEveryPanelIsTheSameSize(t *testing.T) {
	// Only the left column and the bottom row carry labels, which is the
	// shared-scale case and the one where unequal gutters would show.
	g := grid(2, 2,
		layout.Panel{Row: 0, Col: 0, YLabels: []string{"0", "1000000"}},
		layout.Panel{Row: 0, Col: 1},
		layout.Panel{Row: 1, Col: 0, YLabels: []string{"0", "1000000"}, XLabels: []string{"a", "b"}},
		layout.Panel{Row: 1, Col: 1, XLabels: []string{"a", "b"}},
	)
	got := layout.Panels(g, irtest.New())
	w, h := got.Areas[0].Dx(), got.Areas[0].Dy()
	for i, a := range got.Areas {
		if a.Dx() != w || a.Dy() != h {
			t.Errorf("panel %d is %vx%v, want %vx%v", i, a.Dx(), a.Dy(), w, h)
		}
	}
}

// A wide label in one panel has to widen its whole column, or the panels in
// that column stop lining up with the ones above and below.
func TestAWideLabelWidensItsColumnOnly(t *testing.T) {
	narrow := layout.Panels(grid(1, 2,
		layout.Panel{Row: 0, Col: 0, YLabels: []string{"1"}},
		layout.Panel{Row: 0, Col: 1, YLabels: []string{"1"}},
	), irtest.New())
	wide := layout.Panels(grid(1, 2,
		layout.Panel{Row: 0, Col: 0, YLabels: []string{"1"}},
		layout.Panel{Row: 0, Col: 1, YLabels: []string{"1000000000"}},
	), irtest.New())

	if wide.Areas[1].Min.X <= narrow.Areas[1].Min.X {
		t.Error("the wide label did not push its own panel right")
	}
	if wide.Areas[0].Min.X != narrow.Areas[0].Min.X {
		t.Error("the wide label moved the panel in the other column")
	}
}

func TestPanelsAreSeparatedByTheThemeGap(t *testing.T) {
	g := grid(1, 2,
		layout.Panel{Row: 0, Col: 0},
		layout.Panel{Row: 0, Col: 1},
	)
	got := layout.Panels(g, irtest.New())
	if gap := got.Areas[1].Min.X - got.Areas[0].Max.X; gap != theme.Light.PanelGap {
		t.Errorf("gap = %v, want %v", gap, theme.Light.PanelGap)
	}
}

func TestStripsSitAboveTheirPanel(t *testing.T) {
	g := grid(1, 2,
		layout.Panel{Row: 0, Col: 0, Strip: "north"},
		layout.Panel{Row: 0, Col: 1, Strip: "south"},
	)
	got := layout.Panels(g, irtest.New())
	for i := range got.Areas {
		s := got.Strips[i]
		if s.Empty() {
			t.Fatalf("panel %d has no strip", i)
		}
		if s.Max.Y != got.Areas[i].Min.Y {
			t.Errorf("strip %d ends at %v, panel starts at %v", i, s.Max.Y, got.Areas[i].Min.Y)
		}
		if s.Min.X != got.Areas[i].Min.X || s.Max.X != got.Areas[i].Max.X {
			t.Errorf("strip %d spans %v, panel spans %v", i, s, got.Areas[i])
		}
	}
}

func TestRightStripsSitBesideTheirPanel(t *testing.T) {
	g := grid(1, 1, layout.Panel{Row: 0, Col: 0, RightStrip: "eu"})
	got := layout.Panels(g, irtest.New())
	s := got.RightStrips[0]
	if s.Empty() {
		t.Fatal("no side strip")
	}
	if s.Min.X != got.Areas[0].Max.X {
		t.Errorf("side strip starts at %v, panel ends at %v", s.Min.X, got.Areas[0].Max.X)
	}
	if s.Min.Y != got.Areas[0].Min.Y || s.Max.Y != got.Areas[0].Max.Y {
		t.Errorf("side strip spans %v, panel spans %v", s, got.Areas[0])
	}
}

// A row with no strip reserves no band, so a two-way facet does not pay for
// the strips it does not have.
func TestOnlyRowsWithStripsReserveABand(t *testing.T) {
	with := layout.Panels(grid(2, 1,
		layout.Panel{Row: 0, Col: 0, Strip: "a"},
		layout.Panel{Row: 1, Col: 0, Strip: "b"},
	), irtest.New())
	one := layout.Panels(grid(2, 1,
		layout.Panel{Row: 0, Col: 0, Strip: "a"},
		layout.Panel{Row: 1, Col: 0},
	), irtest.New())

	if one.Areas[0].Dy() <= with.Areas[0].Dy() {
		t.Error("dropping a strip did not give the panels more room")
	}
}

func TestAHoleLeavesItsCellEmpty(t *testing.T) {
	g := grid(2, 2, numbered(0, 0), numbered(0, 1), numbered(1, 0))
	got := layout.Panels(g, irtest.New())
	if len(got.Areas) != 3 {
		t.Fatalf("got %d areas, want 3", len(got.Areas))
	}
	// The panels that exist keep their places rather than shuffling up.
	if got.Areas[2].Min.X != got.Areas[0].Min.X {
		t.Error("the panel below moved out of its column")
	}
	if got.Areas[2].Min.Y <= got.Areas[0].Min.Y {
		t.Error("the panel below moved out of its row")
	}
}

func TestTitlesAreCentredOnThePanelsRatherThanTheCanvas(t *testing.T) {
	g := grid(1, 2, numbered(0, 0), numbered(0, 1))
	g.Title = "T"
	g.XTitle = "x"
	g.YTitle = "y"
	got := layout.Panels(g, irtest.New())

	mid := (got.Areas[0].Min.X + got.Areas[1].Max.X) / 2
	if got.Title.X != mid {
		t.Errorf("title at x=%v, want the panels' midpoint %v", got.Title.X, mid)
	}
	if got.XTitle.X != mid {
		t.Errorf("x title at x=%v, want %v", got.XTitle.X, mid)
	}
	if got.YTitle.X > got.Areas[0].Min.X {
		t.Error("the y title is inside the panels")
	}
}

func TestGuidesTakeRoomFromThePanels(t *testing.T) {
	plain := layout.Panels(grid(1, 1, numbered(0, 0)), irtest.New())
	g := grid(1, 1, numbered(0, 0))
	g.Guides = []layout.Guide{{Kind: layout.GuideLegend, Labels: []string{"a series with a long name"}}}
	withLegend := layout.Panels(g, irtest.New())

	if withLegend.Areas[0].Max.X >= plain.Areas[0].Max.X {
		t.Error("the legend took no room from the panel")
	}
	if len(withLegend.Guides) != 1 || withLegend.Guides[0].Empty() {
		t.Error("no legend box was reserved")
	}
}

// The three guide kinds share one column, one stacking rule and one result
// slice. That is the v0.9 generalisation, and this is what says a third kind
// did not need a third field: a chart with all of them lays them out in order,
// aligned, without overlap.
func TestEveryGuideKindStacksInOneColumn(t *testing.T) {
	g := grid(1, 1, numbered(0, 0))
	g.Guides = []layout.Guide{
		{Kind: layout.GuideLegend, Labels: []string{"series"}},
		{Kind: layout.GuideColorbar, Title: "value", Labels: []string{"0", "10"}},
		{Kind: layout.GuideSize, Title: "population", Labels: []string{"1M", "10M"}, Sizes: []float32{8, 32}},
	}
	got := layout.Panels(g, irtest.New())

	if len(got.Guides) != 3 {
		t.Fatalf("got %d guide boxes, want one per guide", len(got.Guides))
	}
	for i, box := range got.Guides {
		if box.Empty() {
			t.Fatalf("guide %d was given no room", i)
		}
		if box.Min.X != got.Guides[0].Min.X {
			t.Errorf("guide %d starts at x=%v and the first at x=%v; the guides are not in one column",
				i, box.Min.X, got.Guides[0].Min.X)
		}
		if i > 0 && box.Min.Y < got.Guides[i-1].Max.Y {
			t.Errorf("guide %d at %v overlaps guide %d at %v", i, box, i-1, got.Guides[i-1])
		}
	}
	if got.Guides[2].Max.X <= got.Areas[0].Max.X {
		t.Error("the size key is not beside the panel")
	}
}

// A size key's rows are as tall as their marks, so a key of large bubbles is
// taller than a key of small ones. Measuring it as text would clip them.
func TestALargerSizeKeyTakesMoreRoom(t *testing.T) {
	small := grid(1, 1, numbered(0, 0))
	small.Guides = []layout.Guide{{Kind: layout.GuideSize, Labels: []string{"a", "b"}, Sizes: []float32{4, 6}}}
	large := grid(1, 1, numbered(0, 0))
	large.Guides = []layout.Guide{{Kind: layout.GuideSize, Labels: []string{"a", "b"}, Sizes: []float32{40, 60}}}

	a := layout.Panels(small, irtest.New()).Guides[0]
	b := layout.Panels(large, irtest.New()).Guides[0]
	if b.Dy() <= a.Dy() {
		t.Errorf("a key of 40 and 60 pixel marks is %v tall and one of 4 and 6 is %v", b.Dy(), a.Dy())
	}
	if b.Dx() <= a.Dx() {
		t.Errorf("a key of wider marks is %v wide and one of narrow marks %v", b.Dx(), a.Dx())
	}
}

func TestADegenerateGridIsWellOrdered(t *testing.T) {
	if got := layout.Panels(grid(0, 0), irtest.New()); len(got.Areas) != 0 {
		t.Errorf("a zero grid produced %d areas", len(got.Areas))
	}
	// A canvas far too small for its furniture must still hand back
	// rectangles that are not inside out.
	tiny := grid(2, 2, numbered(0, 0), numbered(0, 1), numbered(1, 0), numbered(1, 1))
	tiny.Canvas = ir.R(0, 0, 20, 20)
	for i, a := range layout.Panels(tiny, irtest.New()).Areas {
		if a.Max.X < a.Min.X || a.Max.Y < a.Min.Y {
			t.Errorf("panel %d is inverted: %v", i, a)
		}
	}
}

func TestPanelsOutsideTheGridAreIgnored(t *testing.T) {
	g := grid(1, 1, numbered(0, 0), numbered(5, 5))
	got := layout.Panels(g, irtest.New())
	if !got.Areas[1].Empty() {
		t.Errorf("a panel outside the grid was placed at %v", got.Areas[1])
	}
}

// --- fixed rows ----------------------------------------------------------
//
// A fixed row is what a track is made of. ADR 0010 says every panel is the
// same size, and that is still true of the panels: a fixed row is not one of
// them, and the flexible rows are still equal to each other.

// height is one rectangle's height, compared with the slack every device
// coordinate in this repository is compared with.
func height(r ir.Rect) float32 { return r.Max.Y - r.Min.Y }

func closeTo(a, b float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= 0.01
}

func TestAFixedRowIsExactlyAsTallAsItWasTold(t *testing.T) {
	for _, canvas := range []ir.Rect{ir.R(0, 0, 900, 600), ir.R(0, 0, 400, 300), ir.R(0, 0, 1600, 900)} {
		g := grid(2, 1, numbered(0, 0), numbered(1, 0))
		g.Canvas = canvas
		g.RowHeights = []float32{0, 48}

		got := layout.Panels(g, irtest.New())
		if h := height(got.Areas[1]); !closeTo(h, 48) {
			t.Errorf("canvas %v: fixed row is %v tall, want 48", canvas, h)
		}
		if h := height(got.Areas[0]); h <= 0 {
			t.Errorf("canvas %v: the flexible row got %v", canvas, h)
		}
	}
}

// The room a fixed row takes comes out of the flexible rows, which is what
// makes a track take its height out of the panel rather than out of the data.
func TestAFixedRowTakesItsHeightFromTheFlexibleOnes(t *testing.T) {
	free := layout.Panels(grid(2, 1, numbered(0, 0), numbered(1, 0)), irtest.New())
	g := grid(2, 1, numbered(0, 0), numbered(1, 0))
	g.RowHeights = []float32{0, 48}
	fixed := layout.Panels(g, irtest.New())

	if height(fixed.Areas[0]) <= height(free.Areas[0]) {
		t.Errorf("flexible row is %v with a short fixed row below and %v without: it did not grow",
			height(fixed.Areas[0]), height(free.Areas[0]))
	}
	if height(fixed.Areas[0])+height(fixed.Areas[1]) > height(free.Areas[0])+height(free.Areas[1])+0.01 {
		t.Error("the fixed row added height to the grid rather than taking it")
	}
}

// Two flexible rows stay equal to each other whatever is fixed around them.
// That is ADR 0010's promise, narrowed rather than abandoned.
func TestFlexibleRowsStayEqualToEachOther(t *testing.T) {
	g := grid(3, 1, numbered(0, 0), numbered(1, 0), numbered(2, 0))
	g.RowHeights = []float32{0, 0, 48}
	got := layout.Panels(g, irtest.New())

	if !closeTo(height(got.Areas[0]), height(got.Areas[1])) {
		t.Errorf("flexible rows are %v and %v tall", height(got.Areas[0]), height(got.Areas[1]))
	}
}

// A canvas too small for its fixed rows must collapse to something degenerate
// but well ordered. An inverted rectangle maps geometry to nonsense.
func TestFixedRowsTallerThanTheCanvasCollapseWellOrdered(t *testing.T) {
	g := grid(2, 1, numbered(0, 0), numbered(1, 0))
	g.Canvas = ir.R(0, 0, 200, 80)
	g.RowHeights = []float32{0, 400}

	got := layout.Panels(g, irtest.New())
	for i, a := range got.Areas {
		if a.Max.Y < a.Min.Y || a.Max.X < a.Min.X {
			t.Errorf("panel %d is inverted: %v", i, a)
		}
	}
}

// A grid that fixes nothing must compute exactly what it computed before fixed
// rows existed. Every golden file in the repository depends on it.
func TestANilRowHeightsChangesNothing(t *testing.T) {
	for _, rows := range []int{1, 2, 3} {
		var panels []layout.Panel
		for r := range rows {
			panels = append(panels, numbered(r, 0))
		}
		before := layout.Panels(grid(rows, 1, panels...), irtest.New())

		g := grid(rows, 1, panels...)
		g.RowHeights = make([]float32, rows)
		after := layout.Panels(g, irtest.New())

		for i := range before.Areas {
			if before.Areas[i] != after.Areas[i] {
				t.Errorf("%d rows, panel %d: %v with zero heights, %v without", rows, i, after.Areas[i], before.Areas[i])
			}
		}
	}
}

// A fixed column is a fixed row turned a quarter turn, and goes through the
// same function. This is the check that the turn was made everywhere it had to
// be — the origins, the rectangles and the span all read the per-column width.
func TestAFixedColumnIsExactlyAsWideAsItWasTold(t *testing.T) {
	g := grid(1, 2, numbered(0, 0), numbered(0, 1))
	g.ColWidths = []float32{56, 0}

	got := layout.Panels(g, irtest.New())
	if w := got.Areas[0].Max.X - got.Areas[0].Min.X; !closeTo(w, 56) {
		t.Errorf("fixed column is %v wide, want 56", w)
	}
	if w := got.Areas[1].Max.X - got.Areas[1].Min.X; w <= 0 {
		t.Errorf("the flexible column got %v", w)
	}
	if got.Areas[1].Min.X < got.Areas[0].Max.X {
		t.Errorf("the columns overlap: %v and %v", got.Areas[0], got.Areas[1])
	}
}

func TestANilColWidthsChangesNothing(t *testing.T) {
	before := layout.Panels(grid(1, 2, numbered(0, 0), numbered(0, 1)), irtest.New())

	g := grid(1, 2, numbered(0, 0), numbered(0, 1))
	g.ColWidths = []float32{0, 0}
	after := layout.Panels(g, irtest.New())

	for i := range before.Areas {
		if before.Areas[i] != after.Areas[i] {
			t.Errorf("panel %d: %v with zero widths, %v without", i, after.Areas[i], before.Areas[i])
		}
	}
}
