package geom

import (
	"reflect"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

func testSource() data.Source {
	return data.Float64Columns(map[string][]float64{
		"x": {0, 1, 2},
		"y": {1, 2, 3},
		"z": {4, 5, 6},
	})
}

// TestDescribeAndRebuildAgree is the property the JSON spec rests on: a layer
// that describes itself and is built again from that description is configured
// identically.
//
// It compares the two descriptions rather than the two layers, because a
// config is what a layer *is* — and it is exactly what a spec writes down.
func TestDescribeAndRebuildAgree(t *testing.T) {
	src := testSource()
	cs := scale.Sequential(palette.Viridis)
	layers := []Geom{
		Line(src, X("x"), Y("y"), Color(palette.Blue), Width(3), Dash(1, 2), Tension(0.7), OnMissing(Interpolate)),
		Scatter(src, X("x"), Y("y"), Shape(ir.MarkerPlus), Size(11), ColorBy("z", cs)),
		Bar(src, X("x"), Y("y"), BarWidth(0.4), Baseline(-2), Fill(palette.Green), Opacity(0.3)),
		Bar(src, X("x"), X2("z"), Y("y"), GroupBy("z"), Explode(0.08), ExplodeBy("z")),
		Area(src, X("x"), Y("y"), Y2("z"), Decimate(MinMax), Budget(64)),
		Step(src, X("x"), Y("y"), Steps(StepMid)),
		Boxplot(src, X("x"), Y("y"), Whisker(3), Outliers(false)),
		HLine(1.5, Label("limit"), Extend(false)),
		VLine(2.5),
		HBand(1, 2),
		VBand(0.5, 1.5),
		Segment(0, 1, 2, 3),
		Region(0, 1, 2, 3),
		Note(1, 2, "here", FontSize(9), Rotate(0.25), Align(ir.AlignEnd, ir.AlignBottom)),
	}

	for _, g := range layers {
		d, ok := Describe(g)
		if !ok {
			t.Fatalf("%T does not describe itself", g)
		}
		again, err := FromDesc(d)
		if err != nil {
			t.Fatalf("FromDesc(%q): %v", d.Mark, err)
		}
		back, ok := Describe(again)
		if !ok {
			t.Fatalf("a rebuilt %q does not describe itself", d.Mark)
		}
		if !sameDesc(d, back) {
			t.Errorf("%q changed:\n before %+v\n  after %+v", d.Mark, d, back)
		}
	}
}

func TestDescribeReportsTheDefaults(t *testing.T) {
	// A Desc is complete rather than partial: the fields with a non-zero
	// default carry the value the layer is actually using, so nothing
	// downstream has to know what the defaults are.
	d, _ := Describe(Line(testSource(), X("x"), Y("y")))
	if d.BarWidth != 0.8 || d.Whisker != 1.5 || !d.Outliers || !d.Extend || d.Opacity != -1 {
		t.Errorf("defaults were not reported: %+v", d)
	}
	if d.DashSet {
		t.Error("a layer with no Dash option reports one as set")
	}
	if d.Color != nil {
		t.Error("a layer taking its colour from the palette reports an explicit one")
	}
}

func TestAnExplicitlySolidDashIsNotTheSameAsNoDash(t *testing.T) {
	d, _ := Describe(Line(testSource(), X("x"), Y("y"), Dash()))
	if !d.DashSet {
		t.Error("an explicit Dash() with no pattern was reported as unset")
	}
}

func TestFromDescNeedsASource(t *testing.T) {
	if _, err := FromDesc(Desc{Mark: MarkLine, X: "x", Y: "y"}); err == nil {
		t.Error("a line with no data source was built anyway")
	}
	// An annotation does not need one.
	if _, err := FromDesc(Desc{Mark: MarkHLine, Datum: Datum{Y0: 1}}); err != nil {
		t.Errorf("an annotation was refused for having no source: %v", err)
	}
}

func TestFromDescRejectsAnUnknownMark(t *testing.T) {
	if _, err := FromDesc(Desc{Mark: "hexbin", Source: testSource()}); err == nil {
		t.Error("an unknown mark was built")
	}
}

func TestDescribeOnAThirdPartyGeomSaysNo(t *testing.T) {
	if _, ok := Describe(stubGeom{}); ok {
		t.Error("a geom that does not implement Describer reported a description")
	}
}

type stubGeom struct{}

func (stubGeom) Train(scale.Scale, scale.Scale) error { return nil }
func (stubGeom) Build(ir.Backend, Frame) error        { return nil }
func (stubGeom) Legend(Frame) (LegendEntry, bool)     { return LegendEntry{}, false }

// sameDesc compares two descriptions. reflect.DeepEqual is the right tool
// here rather than ==: a Desc holds a dash pattern behind a slice, so it is
// not comparable, and the two references it holds — the Source and the colour
// scale — are the same objects on both sides because FromDesc passes them
// through untouched.
func sameDesc(a, b Desc) bool { return reflect.DeepEqual(a, b) }
