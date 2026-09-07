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

// The hole this closes: a chart written down as JSON used to label its ticks
// the standard way whatever it had been told, because the only way to say
// otherwise was a Go function. A dashboard lives in a document, so that meant
// no thousands separator, no currency, no percentage and no decimals at all.
func TestATickFormatSurvivesTheRoundTrip(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{
		"x": {0, 1, 2},
		"y": {1000, 2000, 3000},
	})
	c := spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X: scale.Linear(scale.Nice()),
		Y: scale.Linear(scale.Nice(), scale.NumberFormat("€ #,.2")),
		Layers: []geom.Geom{
			geom.Line(src, geom.X("x"), geom.Y("y")),
		},
	}
	want, got := draw(t, c), draw(t, roundTrip(t, c))
	if len(want) != len(got) {
		t.Fatalf("the round trip drew %d calls, want %d", len(got), len(want))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("call %d differs:\n want %s\n  got %s", i, want[i], got[i])
		}
	}
	if !strings.Contains(strings.Join(got, "\n"), "€") {
		t.Error("no label carried the currency the format asked for")
	}
}

// The document carries the spec as one field, because the scale's own type
// already says whether it is a number format or a time layout.
func TestTheFormatFieldIsOneField(t *testing.T) {
	c := spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X: scale.Time(scale.TimeLayout("2006-01-02")),
		Y: scale.Linear(scale.NumberFormat("#,")),
	}
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	if !strings.Contains(doc, `"format": "2006-01-02"`) {
		t.Errorf("the time layout is not in the document as a format:\n%s", doc)
	}
	if !strings.Contains(doc, `"format": "#,"`) {
		t.Errorf("the number format is not in the document:\n%s", doc)
	}

	back := roundTrip(t, c)
	xd, _ := scale.Describe(back.X)
	yd, _ := scale.Describe(back.Y)
	if xd.Layout != "2006-01-02" {
		t.Errorf("the time scale read its format back as %q", xd.Layout)
	}
	if yd.Format != "#," {
		t.Errorf("the numeric scale read its format back as %q", yd.Format)
	}
}

// The language travels as a name, resolved against what the reading process
// registered — the same bargain a registered scale kind makes.
func TestALocaleSurvivesTheRoundTripAsAName(t *testing.T) {
	y := scale.Linear(scale.Nice(), scale.NumberFormat("#,.1"))
	scale.Localize(y, scale.LocaleDE)
	c := spec.Chart{Width: 400, Height: 300, DPR: 1, Theme: theme.Light, Y: y}

	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.Marshal()
	if !strings.Contains(string(b), `"locale": "de"`) {
		t.Errorf("the locale is not in the document:\n%s", b)
	}

	back := roundTrip(t, c)
	back.Y.Train(0, 10000)
	back.Y.SetRange(0, 300)
	if got := scale.LabelOf(back.Y, 1234.5); got != "1.234,5" {
		t.Errorf("the chart read back labels %q, want 1.234,5", got)
	}
}

// A format a document carries is input, so a malformed one is an error rather
// than a panic. It is the same parser as the Go path, reached from the other
// side.
func TestAMalformedFormatInADocumentIsAnError(t *testing.T) {
	doc := `{"width":400,"height":300,"encoding":{"y":{"field":"v","type":"quantitative","scale":{"type":"linear","format":"nope"}}},"mark":"line","data":{"values":[{"v":1}]}}`
	s, err := spec.Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := s.Chart(); err == nil {
		t.Error("a malformed format was accepted from a document")
	}
}
