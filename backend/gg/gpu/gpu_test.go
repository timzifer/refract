package gpu_test

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/timzifer/refract"
	ggbackend "github.com/timzifer/refract/backend/gg"
	"github.com/timzifer/refract/backend/gg/gpu"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
)

// What this module does is register an accelerator, and whether the
// registration takes depends on the machine: a CI runner has no GPU and a
// developer's laptop does. So the test is the property that has to hold either
// way — a chart renders, and it renders the same chart.
//
// Nothing here closes the tier until the test that has to: these run in one
// process in the order they are written, the accelerator cannot be registered
// again once given back, and every test before the close is one that wants the
// tier as the import left it.

func TestAChartRendersWithTheTierEitherWay(t *testing.T) {
	t.Logf("GPU tier enabled: %v", gpu.Enabled())

	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {0, 2, 1, 3},
	})
	p := refract.New(refract.Size(200, 150), refract.Title("GPU"))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))

	var buf bytes.Buffer
	if err := p.Render(ggbackend.Writer(&buf, ggbackend.FormatPNG)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("nothing was encoded")
	}

	// Decoding it rather than measuring it: a GPU tier that never flushed
	// encodes a perfectly valid PNG of an empty buffer, and a length check
	// passes on that. The chart has a title and a line on it, so more than one
	// colour has to come back.
	img, err := png.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("decoding what was rendered: %v", err)
	}
	if !painted(img) {
		t.Error("the encoded chart is a single flat colour")
	}

	// The vector emitter does not go near a GPU, so it is the reference for
	// what the chart is: whatever the raster tier does, the geometry is the
	// same geometry.
	var svg bytes.Buffer
	if err := p.Render(refract.SVGWriter(&svg)); err != nil {
		t.Fatalf("Render SVG: %v", err)
	}
	if svg.Len() == 0 {
		t.Fatal("the reference render is empty")
	}
}

// painted reports whether an image has more than one colour in it.
func painted(img image.Image) bool {
	b := img.Bounds()
	if b.Empty() {
		return false
	}
	first := img.At(b.Min.X, b.Min.Y)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.At(x, y) != first {
				return true
			}
		}
	}
	return false
}

// A tier that registers but cannot draw is the failure this package guards
// against: gg's path operations queued work for a device that was never
// obtained, dropped it at flush, and left a chart with its labels and none of
// its geometry. The CPU render is the reference for how much ink a chart has;
// the tier has to put down a comparable amount rather than a handful of
// glyphs.
func TestTheTierPutsDownAsMuchInkAsTheCPU(t *testing.T) {
	t.Logf("GPU tier enabled: %v", gpu.Enabled())

	// The tier as the import left it, then the same chart with it given back.
	// This is where it is given back, which is why it runs before the test that
	// asserts Close is a no-op.
	withTier := ink(t, render(t))
	gpu.Close()
	onCPU := ink(t, render(t))

	if onCPU == 0 {
		t.Fatal("the reference render is blank")
	}
	if withTier*2 < onCPU {
		t.Errorf("the tier drew %d ink pixels against the CPU's %d: paths are being dropped", withTier, onCPU)
	}
}

func render(t *testing.T) image.Image {
	t.Helper()
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {0, 2, 1, 3},
	})
	p := refract.New(refract.Size(200, 150), refract.Title("Ink"))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))

	var buf bytes.Buffer
	if err := p.Render(ggbackend.Writer(&buf, ggbackend.FormatPNG)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return img
}

// ink counts the pixels that are not the colour of the top-left corner.
func ink(t *testing.T, img image.Image) int {
	t.Helper()
	b := img.Bounds()
	bg := img.At(b.Min.X, b.Min.Y)
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.At(x, y) != bg {
				n++
			}
		}
	}
	return n
}

// Closing twice, and closing with no GPU, are both no-ops: a program that
// defers Close should not have to ask whether the tier took.
func TestCloseIsSafeWhateverHappened(t *testing.T) {
	gpu.Close()
	gpu.Close()
	if gpu.Enabled() {
		t.Error("the accelerator is still registered after Close")
	}
}
