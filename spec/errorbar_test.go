package spec_test

import (
	"strings"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/spec"
	"github.com/timzifer/refract/theme"
)

func measured() *data.Table {
	return data.NewTable().
		Float64("t", []float64{0, 1, 2}).
		Float64("mean", []float64{10, 12, 11}).
		Float64("sd", []float64{1, 2, 0.5}).
		Float64("lo", []float64{9, 10, 10.5}).
		Float64("hi", []float64{11, 14, 11.5})
}

// Both spellings round-trip, because a document that lost the difference
// between them would read back as a chart that draws nothing.
func TestAnErrorBarSurvivesTheRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		layer geom.Geom
	}{
		{"spread", geom.ErrorBar(measured(), geom.X("t"), geom.Y("mean"), geom.ErrorBy("sd"))},
		{"bounds", geom.ErrorBar(measured(), geom.X("t"), geom.Y("lo"), geom.Y2("hi"), geom.Mid("mean"))},
		{"horizontal", geom.ErrorBar(measured(), geom.Y("t"), geom.X("mean"), geom.ErrorXBy("sd"))},
		{"no caps", geom.ErrorBar(measured(), geom.X("t"), geom.Y("mean"), geom.ErrorBy("sd"), geom.Caps(false))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ch := spec.Chart{
				Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
				X: scale.Linear(scale.Nice()), Y: scale.Linear(scale.Nice()),
				Layers: []geom.Geom{c.layer},
			}
			want, got := draw(t, ch), draw(t, roundTrip(t, ch))
			if strings.Join(want, "\n") != strings.Join(got, "\n") {
				t.Errorf("the %s spelling did not survive the round trip", c.name)
			}
		})
	}
}

// Caps default to true, so the document has to be able to say false — which is
// why the field is a pointer rather than a bool that vanishes when it is set.
func TestCapsOffSurvivesAsAField(t *testing.T) {
	ch := spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		Layers: []geom.Geom{
			geom.ErrorBar(measured(), geom.X("t"), geom.Y("mean"), geom.ErrorBy("sd"), geom.Caps(false)),
		},
	}
	s, err := spec.Of(ch)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.Marshal()
	if !strings.Contains(string(b), `"caps": false`) {
		t.Errorf("the document does not say the caps were turned off:\n%s", b)
	}
}

// The three channels are refract's own, and they are on the encoding because
// they are columns.
func TestTheIntervalChannelsAreOnTheEncoding(t *testing.T) {
	ch := spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		Layers: []geom.Geom{
			geom.ErrorBar(measured(), geom.X("t"), geom.Y("lo"), geom.Y2("hi"), geom.Mid("mean")),
		},
	}
	s, err := spec.Of(ch)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.Marshal()
	doc := string(b)
	if !strings.Contains(doc, `"mid"`) || !strings.Contains(doc, `"errorbar"`) {
		t.Errorf("the document is missing the mark or its measurement channel:\n%s", doc)
	}
}
