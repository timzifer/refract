package geom_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// wideFrame is a frame over a plot area the size of a real chart, so that the
// automatic reduction sees the pixel count it actually reasons about.
func wideFrame(t *testing.T, g geom.Geom) (*irtest.Recorder, geom.Frame) {
	t.Helper()
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	area := ir.R(0, 0, 400, 300)
	x.SetRange(area.Min.X, area.Max.X)
	y.SetRange(area.Max.Y, area.Min.Y)
	return irtest.New(), geom.Frame{Area: area, X: x, Y: y, Theme: theme.Light}
}

func wave(n int) data.Source {
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = float64(i)
		y[i] = math.Sin(float64(i)/37) + 0.1*math.Sin(float64(i)/3)
	}
	return data.Float64Columns(map[string][]float64{"x": x, "y": y})
}

func drawn(t *testing.T, g geom.Geom) (*irtest.Recorder, int) {
	t.Helper()
	rec, f := wideFrame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	n := 0
	for _, c := range rec.Filter("Polyline") {
		n += len(c.Points)
	}
	for _, c := range rec.Filter("Markers") {
		n += len(c.Points)
	}
	return rec, n
}

func TestALineReducesItselfOnceItOutnumbersThePixels(t *testing.T) {
	const rows = 200_000
	_, n := drawn(t, geom.Line(wave(rows), geom.X("x"), geom.Y("y")))
	if n >= rows {
		t.Fatalf("drew %d vertices for %d rows; a 400px plot cannot show them", n, rows)
	}
	// Two per pixel column is the budget; allow the segment boundaries.
	if n > 2*400+8 {
		t.Errorf("drew %d vertices, want about two per pixel column of a 400px plot", n)
	}
	if n < 100 {
		t.Errorf("drew %d vertices, want a line rather than a sketch of one", n)
	}
}

func TestASmallLineIsDrawnRowForRow(t *testing.T) {
	const rows = 500
	_, n := drawn(t, geom.Line(wave(rows), geom.X("x"), geom.Y("y")))
	if n != rows {
		t.Errorf("drew %d vertices for %d rows, want every one: a chart that is merely detailed must be left alone", n, rows)
	}
}

func TestNoDecimationDrawsEveryRow(t *testing.T) {
	const rows = 50_000
	_, n := drawn(t, geom.Line(wave(rows), geom.X("x"), geom.Y("y"), geom.Decimate(geom.NoDecimation)))
	if n != rows {
		t.Errorf("drew %d vertices for %d rows, want all of them", n, rows)
	}
}

func TestBudgetOverridesTheDefault(t *testing.T) {
	_, n := drawn(t, geom.Line(wave(100_000), geom.X("x"), geom.Y("y"), geom.Budget(300)))
	if n != 300 {
		t.Errorf("drew %d vertices, want the 300 asked for", n)
	}
}

// The reduction happens when the layer draws, so the axis still reports what
// the data holds. A reduced chart whose axis shrank would be a lie about the
// data rather than a summary of it.
func TestDecimationLeavesTheDomainAlone(t *testing.T) {
	const rows = 100_000
	src := wave(rows)
	x, y := scale.Linear(), scale.Linear()
	g := geom.Line(src, geom.X("x"), geom.Y("y"))
	if err := g.Train(x, y); err != nil {
		t.Fatalf("Train: %v", err)
	}
	lo, hi := x.Domain()
	if lo != 0 || hi != float64(rows-1) {
		t.Errorf("x domain is [%v, %v], want the whole column [0, %d]", lo, hi, rows-1)
	}
}

func TestMinMaxKeepsASpikeAStepWouldOtherwiseLose(t *testing.T) {
	const rows = 100_000
	x := make([]float64, rows)
	y := make([]float64, rows)
	for i := range rows {
		x[i] = float64(i)
	}
	y[rows/3] = 1000
	src := data.Float64Columns(map[string][]float64{"x": x, "y": y})

	g := geom.Step(src, geom.X("x"), geom.Y("y"))
	rec, f := wideFrame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	top := float32(math.Inf(1))
	for _, c := range rec.Filter("Polyline") {
		for _, p := range c.Points {
			top = min(top, p.Y)
		}
	}
	// The spike is the domain maximum, so it maps to the top of the area.
	if top > f.Area.Min.Y+0.5 {
		t.Errorf("the tallest vertex drawn is at y=%v, want the top of the plot at %v — the spike was reduced away", top, f.Area.Min.Y)
	}
}

func TestABandIsBoundedByBothEdges(t *testing.T) {
	const rows = 60_000
	x := make([]float64, rows)
	lo := make([]float64, rows)
	hi := make([]float64, rows)
	for i := range rows {
		x[i] = float64(i)
		lo[i], hi[i] = -1, 1
	}
	lo[rows/4] = -500
	hi[3*rows/4] = 500
	src := data.Float64Columns(map[string][]float64{"x": x, "lo": lo, "hi": hi})

	g := geom.Area(src, geom.X("x"), geom.Y("hi"), geom.Y2("lo"))
	rec, f := wideFrame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	top, bottom := float32(math.Inf(1)), float32(math.Inf(-1))
	for _, c := range rec.Filter("FillPath") {
		for _, p := range c.Path.Pts {
			top, bottom = min(top, p.Y), max(bottom, p.Y)
		}
	}
	if top > f.Area.Min.Y+0.5 {
		t.Errorf("the band's top reaches only y=%v, want %v", top, f.Area.Min.Y)
	}
	if bottom < f.Area.Max.Y-0.5 {
		t.Errorf("the band's bottom reaches only y=%v, want %v", bottom, f.Area.Max.Y)
	}
}

// cloud is a noisy diagonal: dense enough to overplot, and shaped so that the
// corners of the plot area stay empty.
func cloud(n int) data.Source {
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		t := float64(i%977) / 977
		x[i] = t
		y[i] = t + 0.05*float64((i*7919)%101)/101
	}
	return data.Float64Columns(map[string][]float64{"x": x, "y": y})
}

func TestACrowdedScatterBecomesARaster(t *testing.T) {
	g := geom.Scatter(cloud(500_000), geom.X("x"), geom.Y("y"))
	rec, f := wideFrame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	imgs := rec.Filter("Image")
	if len(imgs) != 1 {
		t.Fatalf("got %d images and %d marker calls, want one density raster", len(imgs), rec.Count("Markers"))
	}
	if rec.Count("Markers") != 0 {
		t.Error("the layer drew markers as well as a raster")
	}
	if imgs[0].Rect != f.Area {
		t.Errorf("the raster covers %v, want the plot area %v", imgs[0].Rect, f.Area)
	}
	b := imgs[0].Image.Bounds()
	if b.Dx() != 400 || b.Dy() != 300 {
		t.Errorf("the raster is %dx%d, want one cell per pixel of a 400x300 area", b.Dx(), b.Dy())
	}
	if _, _, _, a := imgs[0].Image.At(0, 0).RGBA(); a != 0 {
		t.Error("the corner cell is painted; an empty cell must stay transparent so the grid reads through")
	}
}

func TestASparseScatterKeepsItsMarkers(t *testing.T) {
	g := geom.Scatter(cloud(500), geom.X("x"), geom.Y("y"))
	rec, n := drawn(t, g)
	if rec.Count("Image") != 0 {
		t.Fatal("a scatter that does not overplot was turned into a raster")
	}
	if n != 500 {
		t.Errorf("drew %d markers, want 500", n)
	}
}

// A layer coloured from a column draws one fact per mark. Counting them into
// cells would answer a different question, so the automatic path leaves it be.
func TestAColouredScatterIsNeverRasterizedByDefault(t *testing.T) {
	n := 500_000
	x := make([]float64, n)
	y := make([]float64, n)
	v := make([]float64, n)
	for i := range n {
		x[i] = float64(i%977) / 977
		y[i] = float64((i*7919)%1009) / 1009
		v[i] = float64(i)
	}
	src := data.Float64Columns(map[string][]float64{"x": x, "y": y, "v": v})
	g := geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.ColorBy("v", scale.Sequential(palette.Viridis)))
	rec, f := wideFrame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if rec.Count("Image") != 0 {
		t.Error("a layer coloured from a column was replaced by a raster of counts")
	}
	if rec.Count("Markers") == 0 {
		t.Error("the layer drew nothing at all")
	}
}

func TestExplicitRasterHonoursTheCellSize(t *testing.T) {
	g := geom.Scatter(cloud(2000), geom.X("x"), geom.Y("y"),
		geom.Decimate(geom.DensityRaster), geom.DensityCells(4))
	rec, f := wideFrame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	imgs := rec.Filter("Image")
	if len(imgs) != 1 {
		t.Fatalf("got %d images, want one", len(imgs))
	}
	if b := imgs[0].Image.Bounds(); b.Dx() != 100 || b.Dy() != 75 {
		t.Errorf("the raster is %dx%d, want 100x75 at four pixels per cell", b.Dx(), b.Dy())
	}
}

func TestABarChartIsNeverReduced(t *testing.T) {
	const rows = 20_000
	x := make([]float64, rows)
	y := make([]float64, rows)
	for i := range rows {
		x[i], y[i] = float64(i), float64(i%7+1)
	}
	src := data.Float64Columns(map[string][]float64{"x": x, "y": y})
	g := geom.Bar(src, geom.X("x"), geom.Y("y"))
	rec, f := wideFrame(t, g)
	if err := g.Build(rec, f); err != nil {
		t.Fatalf("Build: %v", err)
	}
	fills := rec.Filter("FillPath")
	if len(fills) != 1 {
		t.Fatalf("got %d fills, want the one batched path", len(fills))
	}
	// Four points and a close per bar.
	if got := len(fills[0].Path.Pts); got != 4*rows {
		t.Errorf("the path holds %d points, want %d — a bar chart has no rows to drop", got, 4*rows)
	}
}
