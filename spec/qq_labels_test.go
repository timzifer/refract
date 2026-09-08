package spec_test

import (
	"reflect"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/spec"
	"github.com/timzifer/refract/theme"
)

func TestQQAndLabelPlacementRoundTrip(t *testing.T) {
	src := data.NewTable().Float64("x", []float64{1, 3, 7}).Float64("y", []float64{1, 3, 7}).String("label", []string{"a", "b", "c"})
	for _, g := range []geom.Geom{
		geom.QQ(src, geom.X("x"), geom.Size(7), geom.Shape(ir.MarkerDiamond)),
		geom.Text(src, geom.X("x"), geom.Y("y"), geom.TextBy("label"), geom.AvoidOverlap(true)),
	} {
		c := spec.Chart{Width: 500, Height: 350, DPR: 1, Theme: theme.Light, X: scale.Linear(), Y: scale.Linear(), Layers: []geom.Geom{g}}
		back := roundTrip(t, c)
		if !reflect.DeepEqual(draw(t, c), draw(t, back)) {
			t.Fatal("round trip changed drawing")
		}
		d, _ := geom.Describe(g)
		bd, _ := geom.Describe(back.Layers[0])
		if d.Mark != bd.Mark || d.AvoidOverlap != bd.AvoidOverlap {
			t.Fatal("round trip lost configuration")
		}
	}
}

func TestHandwrittenQQSpecNeedsOnlyTheSampleColumn(t *testing.T) {
	s, err := spec.Parse([]byte(`{"data":{"values":[{"v":1},{"v":3}]},"layer":[{"mark":{"type":"qq"},"encoding":{"x":{"field":"v"}}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Chart()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Layers) != 1 {
		t.Fatal("lost sample")
	}
	if err := c.Layers[0].Train(scale.Linear(), scale.Linear()); err != nil {
		t.Fatal(err)
	}
}
