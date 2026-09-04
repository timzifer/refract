package gg_test

import (
	"image"
	"testing"

	"github.com/timzifer/refract"
	ggbackend "github.com/timzifer/refract/backend/gg"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

func surfacePlot() *refract.Plot {
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {0, 2, 1, 3},
	})
	p := refract.New(refract.Size(200, 150), refract.Title("Surface"))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	return p
}

func TestASurfaceKeepsItsPixels(t *testing.T) {
	s := ggbackend.NewSurface()
	if s.Image() != nil {
		t.Error("an unopened surface has an image")
	}

	live, err := surfacePlot().Live(s)
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	img := s.Image()
	if img == nil {
		t.Fatal("a drawn surface has no image")
	}
	if got, want := img.Bounds(), (image.Rect(0, 0, 200, 150)); got != want {
		t.Errorf("the image is %v, want %v", got, want)
	}
	if !painted(img) {
		t.Error("the image is empty")
	}
}

func TestASurfaceFollowsAResize(t *testing.T) {
	s := ggbackend.NewSurface()
	live, err := surfacePlot().Live(s)
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	if err := live.Resize(320, 240); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	if got, want := s.Image().Bounds(), image.Rect(0, 0, 320, 240); got != want {
		t.Errorf("the image is %v after a resize, want %v", got, want)
	}
	if w, h, dpr := s.Size(); w != 320 || h != 240 || dpr != 1 {
		t.Errorf("the surface reports %dx%d at %v", w, h, dpr)
	}
	if !painted(s.Image()) {
		t.Error("the resized surface was not repainted")
	}
}

// The generation is what a window watches to know whether to send the chart to
// the GPU again. A frame that changed nothing must not advance it, or an idle
// chart uploads a texture every time the pointer moves.
func TestTheGenerationTracksThePixels(t *testing.T) {
	s := ggbackend.NewSurface()
	live, err := surfacePlot().Live(s)
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	defer live.Close()

	if err := live.Draw(); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	drawn := s.Generation()

	if err := live.Draw(); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	if s.Generation() != drawn {
		t.Error("an unchanged frame advanced the pixel generation")
	}

	if err := live.Resize(240, 200); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if s.Generation() == drawn {
		t.Error("a repaint did not advance the pixel generation")
	}
}

// A partial repaint has to clear what it is about to redraw: compositing an
// antialiased chart over itself darkens every edge in it.
func TestAPartialRepaintDoesNotDrawTwiceOverTheSamePixels(t *testing.T) {
	s := ggbackend.NewSurface()
	b, err := s.Open(120, 90, 1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	partial, ok := b.(ir.Partial)
	if !ok {
		t.Fatal("the raster backend cannot repaint part of a frame")
	}

	stroke := ir.Stroke{Color: ir.RGBA(0, 0, 0, 128), Width: 4}
	line := []ir.Point{{X: 10, Y: 45}, {X: 110, Y: 45}}

	partial.Damage(nil)
	b.Polyline(line, stroke)
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	once := s.Image().At(60, 45)

	partial.Damage([]ir.Rect{ir.R(0, 30, 120, 60)})
	b.Polyline(line, stroke)
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	twice := s.Image().At(60, 45)

	if once != twice {
		t.Errorf("a repainted pixel is %v where a freshly painted one is %v", twice, once)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func painted(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 3 {
		for x := b.Min.X; x < b.Max.X; x += 3 {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				return true
			}
		}
	}
	return false
}

// A window dragged onto a display with a different device pixel ratio asks for
// more pixels behind the same chart. The chart does not change size; its
// buffer does.
func TestASurfaceFollowsTheDevicePixelRatio(t *testing.T) {
	s := ggbackend.NewSurface()
	live, err := surfacePlot().Live(s)
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	if err := live.Rescale(2); err != nil {
		t.Fatalf("Rescale: %v", err)
	}

	if w, h := live.Size(); w != 200 || h != 150 {
		t.Errorf("the chart is %dx%d, want the size it was", w, h)
	}
	if got, want := s.Image().Bounds(), image.Rect(0, 0, 400, 300); got != want {
		t.Errorf("the buffer is %v, want %v", got, want)
	}
	if _, _, dpr := s.Size(); dpr != 2 {
		t.Errorf("the surface reports a device pixel ratio of %v", dpr)
	}
	if !painted(s.Image()) {
		t.Error("the rescaled surface was not repainted")
	}
}

// The raster backend has to honour an italic run, because refract now produces
// them: a typesetter sets a variable italic, and both vector emitters already
// ask for an italic face. A raster that ignored it would draw a different chart
// from the SVG beside it in the documentation.
func TestTheRasterBackendSetsItalic(t *testing.T) {
	s := ggbackend.NewSurface()
	b, err := s.Open(120, 90, 1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	run := ir.TextRun{Text: "measure", Font: ir.FontRef{Size: 16}}
	upright := b.Measure(run)
	run.Font.Italic = true
	italic := b.Measure(run)

	if upright.Advance == italic.Advance {
		t.Errorf("an italic run measures %v, the same as an upright one", italic.Advance)
	}
	if italic.Advance <= 0 {
		t.Errorf("an italic run measures %v", italic.Advance)
	}
}
