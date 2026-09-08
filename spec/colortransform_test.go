package spec_test

import (
	"strings"
	"testing"

	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/spec"
	"github.com/timzifer/refract/theme"
)

func colorChart(cs scale.ColorScale) spec.Chart {
	return spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X: scale.Linear(scale.Nice()), Y: scale.Linear(scale.Nice()),
		Layers: []geom.Geom{geom.Scatter(table(),
			geom.X("x"), geom.Y("y"), geom.ColorBy("z", cs))},
	}
}

func TestAColourTransformSurvivesTheRoundTrip(t *testing.T) {
	for name, cs := range map[string]scale.ColorScale{
		"log":           scale.Sequential(palette.Viridis, scale.ColorLog(0)),
		"log base 2":    scale.Sequential(palette.Viridis, scale.ColorLog(2)),
		"symlog":        scale.Sequential(palette.Viridis, scale.ColorSymLog(0, 0.25)),
		"diverging log": scale.Diverging(palette.PurpleGreen, scale.ColorCenter(25), scale.ColorLog(0)),
	} {
		c := colorChart(cs)
		want, got := draw(t, c), draw(t, roundTrip(t, c))
		if strings.Join(want, "\n") != strings.Join(got, "\n") {
			t.Errorf("%s: the transform did not survive the round trip", name)
		}
	}
}

func TestAColourTransformIsWrittenBesideTheKind(t *testing.T) {
	s, err := spec.Of(colorChart(scale.Diverging(palette.PurpleGreen, scale.ColorLog(2))))
	if err != nil {
		t.Fatal(err)
	}
	cs := s.Layer[0].Encoding.Color.Scale
	if cs.Type != "diverging" || cs.Transform != "log" {
		t.Errorf("type/transform = %q/%q, want both words written", cs.Type, cs.Transform)
	}
	if cs.Base != 2 {
		t.Errorf("base = %v, want 2", cs.Base)
	}
}

func TestTheDefaultBaseIsNotWrittenOut(t *testing.T) {
	s, err := spec.Of(colorChart(scale.Sequential(palette.Viridis, scale.ColorLog(0))))
	if err != nil {
		t.Fatal(err)
	}
	cs := s.Layer[0].Encoding.Color.Scale
	if cs.Base != 0 || cs.Constant != 0 {
		t.Errorf("base/constant = %v/%v, want the defaults left out", cs.Base, cs.Constant)
	}
	// A linear ramp says nothing about a transform at all.
	lin, err := spec.Of(colorChart(scale.Sequential(palette.Viridis)))
	if err != nil {
		t.Fatal(err)
	}
	if got := lin.Layer[0].Encoding.Color.Scale.Transform; got != "" {
		t.Errorf("transform = %q on a linear ramp, want none", got)
	}
}

// A document that spells the transform as the scale's type is Vega-Lite's
// spelling, and refract reads it rather than refusing it.
func TestAVegaLiteLogColourTypeReadsAsASequentialLogRamp(t *testing.T) {
	doc := `{"width":400,"height":300,"data":{"values":[{"x":1,"y":1,"z":10},{"x":2,"y":2,"z":10000}]},` +
		`"layer":[{"mark":{"type":"point"},"encoding":{"x":{"field":"x"},"y":{"field":"y"},` +
		`"color":{"field":"z","scale":{"type":"log","scheme":"viridis"}}}}]}`
	s, err := spec.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Chart()
	if err != nil {
		t.Fatal(err)
	}
	cs, ok := colorScaleOf(t, c)
	if !ok {
		t.Fatal("the layer has no colour scale")
	}
	if got := scale.ColorTransformOf(cs); got != scale.TransformLog {
		t.Errorf("transform = %q, want a log ramp", got)
	}
}

func colorScaleOf(t *testing.T, c spec.Chart) (scale.ColorScale, bool) {
	t.Helper()
	d, ok := geom.Describe(c.Layers[0])
	if !ok || d.ColorScale == nil {
		return nil, false
	}
	return d.ColorScale, true
}

func TestAClassedColourScaleSurvivesTheRoundTrip(t *testing.T) {
	for name, cs := range map[string]scale.ColorScale{
		"threshold":    scale.Threshold(palette.Viridis, []float64{20, 30}),
		"quantize":     scale.Quantize(palette.Viridis, 4),
		"quantize log": scale.Quantize(palette.Viridis, 4, scale.ColorLog(0)),
	} {
		c := colorChart(cs)
		want, got := draw(t, c), draw(t, roundTrip(t, c))
		if strings.Join(want, "\n") != strings.Join(got, "\n") {
			t.Errorf("%s: the classed scale did not survive the round trip", name)
		}
	}
}

func TestBreaksAndClassesAreWrittenSeparately(t *testing.T) {
	s, err := spec.Of(colorChart(scale.Threshold(palette.Viridis, []float64{20, 30})))
	if err != nil {
		t.Fatal(err)
	}
	cs := s.Layer[0].Encoding.Color.Scale
	if cs.Type != "threshold" {
		t.Errorf("type = %q, want threshold", cs.Type)
	}
	if len(cs.Breaks) != 2 || cs.Breaks[0] != 20 || cs.Breaks[1] != 30 {
		t.Errorf("breaks = %v, want [20 30]", cs.Breaks)
	}
	if cs.Classes != 0 {
		t.Errorf("classes = %d on a threshold scale, want none: its count follows from its breaks", cs.Classes)
	}

	q, err := spec.Of(colorChart(scale.Quantize(palette.Viridis, 4)))
	if err != nil {
		t.Fatal(err)
	}
	qs := q.Layer[0].Encoding.Color.Scale
	if qs.Classes != 4 || len(qs.Breaks) != 0 {
		t.Errorf("classes/breaks = %d/%v, want 4 and no pinned boundaries", qs.Classes, qs.Breaks)
	}
}
