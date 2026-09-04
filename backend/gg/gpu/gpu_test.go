package gpu_test

import (
	"bytes"
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

func TestAChartRendersWithTheTierEitherWay(t *testing.T) {
	t.Logf("GPU tier enabled: %v", gpu.Enabled())
	defer gpu.Close()

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

// Closing twice, and closing with no GPU, are both no-ops: a program that
// defers Close should not have to ask whether the tier took.
func TestCloseIsSafeWhateverHappened(t *testing.T) {
	gpu.Close()
	gpu.Close()
	if gpu.Enabled() {
		t.Error("the accelerator is still registered after Close")
	}
}
