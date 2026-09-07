package a11y_test

import (
	"strings"
	"testing"

	"github.com/timzifer/refract/a11y"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
)

// A layer that reads an edge table names none of the positional channels, so
// the accessible table has to know about the ones it does name — or the whole
// layer comes out of the document with no columns and no rows, which is a
// silent answer to the one question ADR 0024 exists to answer.
func TestASankeysDataTableHasItsColumns(t *testing.T) {
	src := data.NewTable().
		String("from", []string{"web", "api"}).
		String("to", []string{"api", "db"}).
		Float64("n", []float64{6, 5})

	c := a11y.Chart{
		Title: "Requests",
		X:     scale.Linear(), Y: scale.Linear(),
		Layers: []geom.Geom{
			geom.Sankey(src, geom.From("from"), geom.To("to"), geom.Value("n")),
		},
	}
	out, err := a11y.Table(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"from", "to", "web", "api", "db", "6", "5"} {
		if !strings.Contains(out, want) {
			t.Errorf("the table does not mention %q:\n%s", want, out)
		}
	}
}

// It is named after what it measures rather than after what it draws: "sankey"
// is a shape, and the reader wants to know it is about requests.
func TestARelationalLayerIsNamedAfterItsValueColumn(t *testing.T) {
	src := data.NewTable().
		String("a", []string{"x"}).
		String("b", []string{"y"}).
		Float64("requests", []float64{3})
	s := a11y.Describe(a11y.Chart{
		X: scale.Linear(), Y: scale.Linear(),
		Layers: []geom.Geom{geom.Arc(src, geom.From("a"), geom.To("b"), geom.Value("requests"))},
	})
	if !strings.Contains(s.Detail, "requests") {
		t.Errorf("the description does not name the value column:\n%s", s.Detail)
	}
}
