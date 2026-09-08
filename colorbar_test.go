package refract_test

// A colourbar and a size key that answer to a pointer. Neither maps to a layer
// the way a legend row does — a colourbar band is a range of values and a size
// key row is a magnitude — so what a hit reports is a quantity rather than a
// series.

import (
	"math"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

func guideChart(t *testing.T, layer geom.Geom) *refract.Live {
	t.Helper()
	p := refract.New(refract.Size(640, 320))
	p.X(scale.Linear(scale.Domain(0, 3)))
	p.Y(scale.Linear(scale.Domain(0, 30)))
	p.Add(layer)
	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	return live
}

func guideSource() refract.Source {
	return refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {10, 20, 15, 25},
		"v": {0, 33, 66, 100},
	})
}

// sweep collects every distinct guide hit on the canvas, so a test can talk
// about what a pointer would find rather than about where things were drawn.
func sweep(t *testing.T, l *refract.Live, kind refract.Kind) []refract.Hit {
	t.Helper()
	var out []refract.Hit
	seen := map[string]bool{}
	for x := float32(2); x < 640; x += 2 {
		for y := float32(2); y < 320; y += 2 {
			hit, ok := l.Index().At(ir.Point{X: x, Y: y}, 0)
			if !ok || hit.Kind != kind {
				continue
			}
			key := hit.Series + "|" + itoa(hit.Class)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, hit)
		}
	}
	return out
}

func itoa(i int) string {
	if i < 0 {
		return "-" + itoa(-i)
	}
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

// A continuous bar is one target whose value changes along it, because every
// point of a ramp means something different and there is nothing discrete to
// enumerate.
func TestAContinuousColorbarReportsTheValueUnderThePointer(t *testing.T) {
	live := guideChart(t, geom.Scatter(guideSource(),
		geom.X("x"), geom.Y("y"),
		geom.ColorBy("v", scale.Sequential(palette.Viridis, scale.ColorDomain(0, 100))),
	))

	hits := sweep(t, live, refract.Colorbar)
	if len(hits) != 1 {
		t.Fatalf("a continuous bar reported %d targets, want one", len(hits))
	}
	if hits[0].Class != -1 {
		t.Errorf("class = %d, want -1 — a continuous ramp has no bands", hits[0].Class)
	}

	// The bar runs bottom to top: the top of it is the top of the domain.
	ix := live.Index()
	var top, bottom float64
	found := 0
	for y := float32(2); y < 320; y++ {
		hit, ok := ix.At(ir.Point{X: hits[0].At.X, Y: y}, 0)
		if !ok || hit.Kind != refract.Colorbar {
			continue
		}
		if found == 0 {
			top = hit.Value
		}
		bottom = hit.Value
		found++
	}
	if found < 2 {
		t.Fatal("the bar is too short to read a gradient off")
	}
	if !(top > bottom) {
		t.Errorf("the bar reads %v at the top and %v at the bottom; it runs bottom to top", top, bottom)
	}
	if math.Abs(top-100) > 2 || math.Abs(bottom-0) > 2 {
		t.Errorf("the bar spans %v..%v, want about 0..100", bottom, top)
	}
}

// A classed bar reports one band per class, each with the interval a filter
// would be written against.
func TestAClassedColorbarReportsItsBands(t *testing.T) {
	live := guideChart(t, geom.Scatter(guideSource(),
		geom.X("x"), geom.Y("y"),
		geom.ColorBy("v", scale.Threshold(palette.Viridis, []float64{25, 75},
			scale.ColorDomain(0, 100))),
	))

	hits := sweep(t, live, refract.Colorbar)
	if len(hits) != 3 {
		t.Fatalf("a scale cut at 25 and 75 reported %d bands, want 3", len(hits))
	}
	want := [][2]float64{{0, 25}, {25, 75}, {75, 100}}
	seen := map[int][2]float64{}
	for _, h := range hits {
		if h.Class < 0 || h.Class >= 3 {
			t.Fatalf("class = %d, want 0, 1 or 2", h.Class)
		}
		seen[h.Class] = [2]float64{h.Lo, h.Hi}
		// A band reports the middle of what it covers: one colour stands for
		// one interval, so there is no gradient inside it to read a position
		// off, and Lo and Hi are the truth.
		if want := h.Lo + (h.Hi-h.Lo)/2; h.Value != want {
			t.Errorf("band %d reports value %v, want its midpoint %v", h.Class, h.Value, want)
		}
	}
	for i, w := range want {
		if got := seen[i]; got != w {
			t.Errorf("band %d spans %v, want %v", i, got, w)
		}
	}
}

// A size key row says what magnitude it stands for, which is the number a
// filter is written against rather than its spelling.
func TestASizeKeyRowReportsItsValue(t *testing.T) {
	live := guideChart(t, geom.Scatter(guideSource(),
		geom.X("x"), geom.Y("y"),
		geom.SizeBy("v", scale.Size(scale.SizeRange(4, 24), scale.SizeDomain(0, 100))),
	))

	hits := sweep(t, live, refract.SizeKey)
	if len(hits) == 0 {
		t.Fatal("the size key is not reachable by a pointer")
	}
	for _, h := range hits {
		if h.Series == "" {
			t.Error("a size key row carries no label")
		}
		if h.Value <= 0 || h.Value > 100 {
			t.Errorf("row %q stands for %v, outside the domain 0..100", h.Series, h.Value)
		}
		if h.Class != -1 {
			t.Errorf("row %q reports class %d, want -1 — a size key has no bands", h.Series, h.Class)
		}
	}
}

// Every guide belongs to the chart rather than to a panel, and none of them
// pretends to be a reading off an axis.
func TestNoGuideReportsAPanelValueOrARow(t *testing.T) {
	for _, c := range []struct {
		name  string
		layer geom.Geom
		kind  refract.Kind
	}{
		{"continuous", geom.Scatter(guideSource(), geom.X("x"), geom.Y("y"),
			geom.ColorBy("v", scale.Sequential(palette.Viridis))), refract.Colorbar},
		{"classed", geom.Scatter(guideSource(), geom.X("x"), geom.Y("y"),
			geom.ColorBy("v", scale.Quantize(palette.Viridis, 3))), refract.Colorbar},
		{"size", geom.Scatter(guideSource(), geom.X("x"), geom.Y("y"),
			geom.SizeBy("v", scale.Size(scale.SizeRange(4, 24)))), refract.SizeKey},
	} {
		t.Run(c.name, func(t *testing.T) {
			live := guideChart(t, c.layer)
			hits := sweep(t, live, c.kind)
			if len(hits) == 0 {
				t.Fatalf("no %v was reachable", c.kind)
			}
			for _, h := range hits {
				if h.Panel != -1 {
					t.Errorf("panel = %d, want -1 — a guide belongs to the chart", h.Panel)
				}
				if h.X != 0 || h.Y != 0 {
					t.Errorf("the guide reports x=%v y=%v; a guide is not a reading off an axis", h.X, h.Y)
				}
				if h.Row != -1 {
					t.Errorf("row = %d, want -1", h.Row)
				}
				if h.Layer != -1 {
					t.Errorf("layer = %d, want -1 — neither a bar nor a size key maps to one", h.Layer)
				}
				if !h.Kind.Guides() {
					t.Errorf("%v does not report as a guide", h.Kind)
				}
			}
		})
	}
}

// A hover in the margin finds them, the same exception a legend row gets.
func TestAHoverInTheMarginFindsAColorbar(t *testing.T) {
	p := refract.New(refract.Size(640, 320))
	p.X(scale.Linear(scale.Domain(0, 3)))
	p.Y(scale.Linear(scale.Domain(0, 30)))
	p.Add(geom.Scatter(guideSource(), geom.X("x"), geom.Y("y"),
		geom.ColorBy("v", scale.Sequential(palette.Viridis))))

	var got refract.Event
	p.On(refract.Hover, func(ev refract.Event) { got = ev })

	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	hits := sweep(t, live, refract.Colorbar)
	if len(hits) == 0 {
		t.Fatal("no colourbar")
	}
	at := hits[0].At
	live.Move(float64(at.X), float64(at.Y))
	if !got.Found || got.Hit.Kind != refract.Colorbar {
		t.Fatalf("a hover over the colourbar found %v (found=%v)", got.Hit.Kind, got.Found)
	}
	if got.Panel != -1 {
		t.Errorf("panel = %d, want -1", got.Panel)
	}
}

// A chart with no guide indexes none, so nothing here costs a chart that does
// not have one.
func TestAChartWithoutGuidesIndexesNone(t *testing.T) {
	live := guideChart(t, geom.Scatter(guideSource(), geom.X("x"), geom.Y("y")))
	for _, k := range []refract.Kind{refract.Colorbar, refract.SizeKey, refract.LegendRow} {
		if hits := sweep(t, live, k); len(hits) != 0 {
			t.Errorf("a chart with no guides reported %d %v hits", len(hits), k)
		}
	}
}

// The recipe, run end to end: a click on a colourbar band reaches a handler
// with the interval it covers, which is what a filter is written against.
//
// What the interval means is the caller's — filter, highlight, drill down —
// for the same reason a legend click is: a band that always filtered would be
// wrong for a chart where it should select, or annotate, or do nothing.
func TestClickingAColourbarBandReportsItsInterval(t *testing.T) {
	src := guideSource()
	p := refract.New(refract.Size(640, 320))
	p.X(scale.Linear(scale.Domain(0, 3)))
	p.Y(scale.Linear(scale.Domain(0, 30)))
	p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y"),
		geom.ColorBy("v", scale.Threshold(palette.Viridis, []float64{25, 75},
			scale.ColorDomain(0, 100)))))

	var kept []float64
	p.On(refract.Click, func(ev refract.Event) {
		if ev.Hit.Kind != refract.Colorbar || ev.Hit.Class < 0 {
			return
		}
		// The rows the band covers. refract said which interval; which rows
		// are in it is this program's arithmetic.
		v, _ := src.Float64Column("v")
		kept = kept[:0]
		for _, x := range v {
			if x >= ev.Hit.Lo && x < ev.Hit.Hi {
				kept = append(kept, x)
			}
		}
	})

	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	var middle refract.Hit
	for _, h := range sweep(t, live, refract.Colorbar) {
		if h.Class == 1 {
			middle = h
		}
	}
	if middle.Class != 1 {
		t.Fatal("no middle band")
	}

	in := live.Input()
	if err := in.Down(float64(middle.At.X), float64(middle.At.Y)); err != nil {
		t.Fatal(err)
	}
	if err := in.Up(float64(middle.At.X), float64(middle.At.Y)); err != nil {
		t.Fatal(err)
	}

	// 33 and 66 are in [25, 75); 0 and 100 are not.
	if len(kept) != 2 || kept[0] != 33 || kept[1] != 66 {
		t.Errorf("the band [25 75) kept %v, want [33 66]", kept)
	}
}

// A size key row is clickable the same way, and reports a magnitude.
func TestClickingASizeKeyRowReportsItsValue(t *testing.T) {
	p := refract.New(refract.Size(640, 320))
	p.X(scale.Linear(scale.Domain(0, 3)))
	p.Y(scale.Linear(scale.Domain(0, 30)))
	p.Add(geom.Scatter(guideSource(), geom.X("x"), geom.Y("y"),
		geom.SizeBy("v", scale.Size(scale.SizeRange(4, 24), scale.SizeDomain(0, 100)))))

	var got refract.Hit
	p.On(refract.Click, func(ev refract.Event) {
		if ev.Hit.Kind == refract.SizeKey {
			got = ev.Hit
		}
	})

	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	rows := sweep(t, live, refract.SizeKey)
	if len(rows) == 0 {
		t.Fatal("no size key")
	}
	at := rows[0].At
	in := live.Input()
	if err := in.Down(float64(at.X), float64(at.Y)); err != nil {
		t.Fatal(err)
	}
	if err := in.Up(float64(at.X), float64(at.Y)); err != nil {
		t.Fatal(err)
	}
	if got.Kind != refract.SizeKey {
		t.Fatal("the click did not reach the handler as a size key hit")
	}
	if got.Value != rows[0].Value {
		t.Errorf("the click reports %v, want the row's %v", got.Value, rows[0].Value)
	}
}
