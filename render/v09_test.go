package render_test

import (
	"math"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// The third guide kind. v0.9's claim is that the guide column was generalised
// once rather than extended twice, and the way to hold that is to put all three
// kinds beside one plot and check that none of them lands on another.

func bubbles(size scale.SizeScale, cs scale.ColorScale) geom.Geom {
	src := data.Float64Columns(map[string][]float64{
		"x":   {0, 1, 2, 3},
		"y":   {0, 2, 1, 3},
		"v":   {10, 20, 30, 40},
		"pop": {100, 400, 900, 1600},
	})
	opts := []geom.Option{geom.X("x"), geom.Y("y"), geom.SizeBy("pop", size)}
	if cs != nil {
		opts = append(opts, geom.ColorBy("v", cs))
	}
	return geom.Scatter(src, opts...)
}

func TestALegendAColourbarAndASizeKeySitBesideEachOther(t *testing.T) {
	c := chart(
		line(),
		bubbles(scale.Size(), scale.Sequential(palette.Viridis)),
	)
	c.ShowLegend = true
	rec := draw(t, c)

	legend := textAt(rec, "series")
	bar := gradients(rec)
	key := textAt(rec, "pop")
	if legend == nil {
		t.Fatal("the legend was not drawn")
	}
	if len(bar) != 1 {
		t.Fatalf("got %d colourbars, want one", len(bar))
	}
	if key == nil {
		t.Fatal("the size key's title was not drawn")
	}

	// All three in one column, none of them on top of another, and the whole
	// column to the right of the plot.
	boxes := []ir.Rect{
		{Min: *legend, Max: ir.Point{X: legend.X, Y: legend.Y}},
		bar[0].Path.Bounds(),
	}
	if boxes[0].Min.X > boxes[1].Min.X+40 || boxes[1].Min.X > boxes[0].Min.X+40 {
		t.Errorf("the legend starts at x=%v and the colourbar at x=%v; they are not in one column",
			boxes[0].Min.X, boxes[1].Min.X)
	}
	if key.X < boxes[1].Min.X-40 {
		t.Errorf("the size key's title is at x=%v and the colourbar at x=%v", key.X, boxes[1].Min.X)
	}
	// The guides stack downwards in the order they were collected.
	if !(legend.Y < bar[0].Path.Bounds().Min.Y) {
		t.Errorf("the legend is at y=%v and the colourbar at y=%v; the legend comes first",
			legend.Y, bar[0].Path.Bounds().Min.Y)
	}
	if !(key.Y > bar[0].Path.Bounds().Min.Y) {
		t.Errorf("the size key is at y=%v and the colourbar at y=%v; the key comes last",
			key.Y, bar[0].Path.Bounds().Min.Y)
	}
}

// The key's samples are drawn at the sizes the marks are drawn at, so a reader
// can measure a bubble against them.
func TestTheSizeKeySamplesAreDrawnAtTheScalesOwnSizes(t *testing.T) {
	s := scale.Size()
	c := chart(bubbles(s, nil))
	rec := draw(t, c)

	// The key's samples are the only stroked circles outside the plot area.
	var samples []float32
	for _, call := range rec.Filter("StrokePath") {
		b := call.Path.Bounds()
		if b.Dx() <= 0 || math.Abs(float64(b.Dx()-b.Dy())) > 0.5 {
			continue
		}
		samples = append(samples, b.Dx())
	}
	if len(samples) == 0 {
		t.Fatal("the size key drew no samples")
	}
	_, hi := s.Domain()
	want := s.Size(hi)
	widest := float32(0)
	for _, v := range samples {
		widest = max(widest, v)
	}
	if widest > want+0.5 {
		t.Errorf("the key's widest sample is %v across and the scale's largest mark is %v", widest, want)
	}
}

// A chart with a size key takes room from the panel for it, exactly as one with
// a legend does.
func TestASizeKeyTakesRoomFromThePanel(t *testing.T) {
	plain := chart(line())
	keyed := chart(bubbles(scale.Size(), nil))

	a := plotRight(t, plain)
	b := plotRight(t, keyed)
	if !(b < a) {
		t.Errorf("the plot reaches x=%v with a size key and x=%v without one", b, a)
	}
}

// Two layers over one size scale contribute one key, the way two layers over
// one colour scale contribute one colourbar.
func TestTwoLayersOverOneSizeScaleGetOneKey(t *testing.T) {
	s := scale.Size()
	c := chart(bubbles(s, nil), bubbles(s, nil))
	rec := draw(t, c)
	n := 0
	for _, call := range rec.Filter("Text") {
		if call.Text.Text == "pop" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the chart drew %d size-key titles, want one", n)
	}
}

// Collecting the key is also what gives the size scale the diameters the theme
// asks for — the geoms cannot do it themselves, because panels are built
// concurrently and Train has no theme. So the marks come out at the theme's
// size rather than at the scale's fallback.
func TestTheThemeDecidesHowLargeABubbleGets(t *testing.T) {
	small := chart(bubbles(scale.Size(), nil))
	small.Theme = theme.Light.With(func(th *theme.Theme) { th.BubbleSize = 10 })
	large := chart(bubbles(scale.Size(), nil))
	large.Theme = theme.Light.With(func(th *theme.Theme) { th.BubbleSize = 50 })

	if a, b := widestBubble(t, small), widestBubble(t, large); !(a < b) {
		t.Errorf("a theme asking for 10px bubbles drew %v and one asking for 50px drew %v", a, b)
	}
}

// widestBubble measures the marks a sized layer drew.
//
// The layer emits one filled path holding every bubble as a subpath, and the
// size key emits one circle per call — so the cubic fill with the most subpaths
// is the cloud, and its widest subpath is its largest bubble.
func widestBubble(t *testing.T, c render.Chart) float32 {
	t.Helper()
	rec := draw(t, c)
	best, widest := 0, float32(0)
	for _, call := range rec.Filter("FillPath") {
		if call.Fill.IsGradient() {
			continue
		}
		ws := circleWidths(call.Path)
		if len(ws) <= best {
			continue
		}
		best, widest = len(ws), 0
		for _, w := range ws {
			widest = max(widest, w)
		}
	}
	return widest
}

// circleWidths measures each cubic subpath of a path. A path with none is not
// made of circles and reports nothing.
func circleWidths(p *ir.Path) []float32 {
	var out []float32
	var lo, hi float32
	open, curved := false, false
	flush := func() {
		if open && curved {
			out = append(out, hi-lo)
		}
		open, curved = false, false
	}
	p.Walk(func(op ir.PathOp, pts []ir.Point) {
		if op == ir.OpMoveTo {
			flush()
			lo, hi, open = pts[0].X, pts[0].X, true
		}
		if op == ir.OpCubicTo {
			curved = true
		}
		for _, q := range pts {
			lo, hi = min(lo, q.X), max(hi, q.X)
		}
	})
	flush()
	return out
}

// textAt returns where a run of text was drawn, or nil.
func textAt(rec *irtest.Recorder, s string) *ir.Point {
	for _, c := range rec.Filter("Text") {
		if c.Text.Text == s {
			at := c.Text.At
			return &at
		}
	}
	return nil
}
