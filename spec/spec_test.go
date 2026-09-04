package spec_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/render"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/spec"
	"github.com/timzifer/refract/theme"
)

func table() *data.Table {
	return data.NewTable().
		Float64("x", []float64{0, 1, 2, 3}).
		Float64("y", []float64{1, 4, 2, 8}).
		Float64("z", []float64{10, 20, 30, 40}).
		String("region", []string{"a", "b", "a", "b"})
}

// draw renders a chart into a recorder and returns what it emitted. Comparing
// two of these is how a round trip is checked: a spec that reads back as a
// chart that draws the same calls is a spec that lost nothing.
func draw(t *testing.T, c spec.Chart) []string {
	t.Helper()
	rec := irtest.New()
	rc := render.Chart{
		Width: c.Width, Height: c.Height, DPR: c.DPR, Theme: c.Theme,
		Title: c.Title, XTitle: c.XTitle, YTitle: c.YTitle,
		X: c.X, Y: c.Y, Layers: c.Layers,
	}
	if rc.X == nil {
		rc.X = scale.Linear(scale.Nice())
	}
	if rc.Y == nil {
		rc.Y = scale.Linear(scale.Nice())
	}
	if c.Legend != nil {
		rc.ShowLegend = *c.Legend
	}
	if err := render.Draw(rec, rc); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	return rec.Trace()
}

func roundTrip(t *testing.T, c spec.Chart) spec.Chart {
	t.Helper()
	s, err := spec.Of(c)
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := spec.Parse(b)
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, b)
	}
	out, err := back.Chart()
	if err != nil {
		t.Fatalf("Chart: %v\n%s", err, b)
	}
	return out
}

func TestEveryMarkSurvivesTheRoundTrip(t *testing.T) {
	src := table()
	cases := []struct {
		name  string
		layer geom.Geom
	}{
		{"line", geom.Line(src, geom.X("x"), geom.Y("y"), geom.Tension(0.4), geom.Dash(4, 2))},
		{"scatter", geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.Shape(ir.MarkerDiamond), geom.Size(9))},
		{"bar", geom.Bar(src, geom.X("x"), geom.Y("y"), geom.BarWidth(0.5), geom.Baseline(1))},
		{"area", geom.Area(src, geom.X("x"), geom.Y("y"), geom.Y2("z"), geom.Opacity(0.4))},
		{"step", geom.Step(src, geom.X("x"), geom.Y("y"), geom.Steps(geom.StepPre))},
		{"boxplot", geom.Boxplot(src, geom.X("region"), geom.Y("y"), geom.Whisker(2), geom.Outliers(false))},
		{"hline", geom.HLine(3, geom.Color(palette.Red))},
		{"vline", geom.VLine(1.5)},
		{"hband", geom.HBand(1, 3)},
		{"vband", geom.VBand(0.5, 1.5)},
		{"segment", geom.Segment(0, 1, 3, 8)},
		{"region", geom.Region(0.5, 1, 2.5, 6)},
		{"note", geom.Note(1, 4, "peak", geom.Align(ir.AlignCenter, ir.AlignTop), geom.Rotate(0.5))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x := scale.Linear(scale.Nice())
			if tc.name == "boxplot" {
				x = scale.Ordinal()
			}
			c := spec.Chart{
				Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
				X: x, Y: scale.Linear(scale.Nice()),
				Layers: []geom.Geom{tc.layer},
			}
			want := draw(t, c)
			got := draw(t, roundTrip(t, c))
			if len(want) != len(got) {
				t.Fatalf("%d calls after the round trip, %d before", len(got), len(want))
			}
			for i := range want {
				if want[i] != got[i] {
					t.Fatalf("call %d differs:\n before: %s\n  after: %s", i, want[i], got[i])
				}
			}
		})
	}
}

func TestEveryScaleSurvivesTheRoundTrip(t *testing.T) {
	src := table()
	cases := []struct {
		name string
		s    scale.Scale
	}{
		{"linear", scale.Linear(scale.Nice(), scale.Zero())},
		{"linear-fixed", scale.Linear(scale.Domain(-5, 12))},
		{"log", scale.Log(scale.LogBase(2), scale.LogNice())},
		{"log-fixed", scale.Log(scale.LogDomain(1, 1000), scale.LogMinorTicks(false))},
		{"symlog", scale.SymLog(scale.SymLogThreshold(0.5), scale.SymLogBase(2))},
		{"time", scale.Time()},
		{"ordinal", scale.Ordinal(scale.Categories("a", "b"), scale.OrdinalPadding(0.4))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := spec.Chart{
				Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
				X: scale.Linear(scale.Nice()), Y: tc.s,
				Layers: []geom.Geom{geom.Line(src, geom.X("x"), geom.Y("z"))},
			}
			if tc.name == "ordinal" {
				c.Y = tc.s
				c.Layers = []geom.Geom{geom.Line(src, geom.X("x"), geom.Y("region"))}
			}
			want, got := draw(t, c), draw(t, roundTrip(t, c))
			if strings.Join(want, "\n") != strings.Join(got, "\n") {
				t.Errorf("the %s scale did not survive the round trip", tc.name)
			}
		})
	}
}

func TestAColourScaleSurvivesTheRoundTrip(t *testing.T) {
	src := table()
	c := spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X: scale.Linear(scale.Nice()), Y: scale.Linear(scale.Nice()),
		Layers: []geom.Geom{geom.Scatter(src,
			geom.X("x"), geom.Y("y"),
			geom.ColorBy("z", scale.Diverging(palette.PurpleGreen, scale.ColorCenter(25))))},
	}
	want, got := draw(t, c), draw(t, roundTrip(t, c))
	if strings.Join(want, "\n") != strings.Join(got, "\n") {
		t.Error("a diverging colour scale did not survive the round trip")
	}

	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	cs := s.Layer[0].Encoding.Color.Scale
	if cs.Scheme != "purplegreen" {
		t.Errorf("scheme = %q, want the registered ramp name", cs.Scheme)
	}
	if cs.Center == nil || *cs.Center != 25 {
		t.Errorf("center = %v, want 25", cs.Center)
	}
}

func TestAnUnregisteredRampIsWrittenOut(t *testing.T) {
	mine := palette.Ramp{ir.RGB(1, 2, 3), ir.RGB(4, 5, 6)}
	c := spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		Layers: []geom.Geom{geom.Scatter(table(),
			geom.X("x"), geom.Y("y"), geom.ColorBy("z", scale.Sequential(mine)))},
	}
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	cs := s.Layer[0].Encoding.Color.Scale
	if cs.Scheme != "" {
		t.Errorf("scheme = %q, want none: the ramp is not registered", cs.Scheme)
	}
	if got := cs.Range; len(got) != 2 || got[0] != "#010203" {
		t.Errorf("range = %v, want the ramp written out", got)
	}
	// And it reads back as the same ramp rather than as the default one.
	back := roundTrip(t, c)
	d, ok := geom.Describe(back.Layers[0])
	if !ok {
		t.Fatal("the layer cannot describe itself")
	}
	cd, ok := scale.DescribeColor(d.ColorScale)
	if !ok {
		t.Fatal("the colour scale cannot describe itself")
	}
	if len(cd.Colors) != 2 || cd.Colors[0] != mine[0] {
		t.Errorf("the ramp came back as %v, want %v", cd.Colors, mine)
	}
}

func TestOneSourceIsWrittenOnce(t *testing.T) {
	src := table()
	c := spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		Layers: []geom.Geom{
			geom.Line(src, geom.X("x"), geom.Y("y")),
			geom.Line(src, geom.X("x"), geom.Y("z")),
			geom.HLine(3),
		},
	}
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	if s.Data == nil {
		t.Fatal("the shared table was not hoisted to the top level")
	}
	for i, l := range s.Layer {
		if l.Data != nil {
			t.Errorf("layer %d carries a copy of the shared table", i)
		}
	}

	// Two sources, and each layer carries its own.
	c.Layers[1] = geom.Line(table(), geom.X("x"), geom.Y("z"))
	s, err = spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	if s.Data != nil {
		t.Error("two different tables were hoisted into one")
	}
	if s.Layer[0].Data == nil || s.Layer[1].Data == nil {
		t.Error("a layer with data of its own did not carry it")
	}
	if s.Layer[2].Data != nil {
		t.Error("an annotation carries data")
	}
}

func TestColumnTypesSurvive(t *testing.T) {
	when := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	src := data.NewTable().
		Time("t", []time.Time{when, when.Add(time.Minute)}).
		Float64("y", []float64{1, 2}).
		String("k", []string{"a", "b"})

	c := spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X:      scale.Time(),
		Layers: []geom.Geom{geom.Line(src, geom.X("t"), geom.Y("y"))},
	}
	back := roundTrip(t, c)
	d, _ := geom.Describe(back.Layers[0])
	got := d.Source

	if ts, ok := got.TimeColumn("t"); !ok || !ts[0].Equal(when) {
		t.Errorf("the time column came back as %v, %v", ts, ok)
	}
	if _, ok := got.Float64Column("y"); !ok {
		t.Error("the numeric column did not come back numeric")
	}
	if ks, ok := got.StringColumn("k"); !ok || ks[1] != "b" {
		t.Errorf("the category column came back as %v, %v", ks, ok)
	}
}

func TestAMissingValueIsNullAndComesBackNaN(t *testing.T) {
	src := data.NewTable().
		Float64("x", []float64{0, 1, 2}).
		Float64("y", []float64{1, nan(), 3})

	c := spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		Layers: []geom.Geom{geom.Line(src, geom.X("x"), geom.Y("y"))},
	}
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatalf("a NaN made the document unmarshalable: %v", err)
	}
	if !strings.Contains(string(b), "null") {
		t.Error("the missing value was not written as null")
	}

	back := roundTrip(t, c)
	d, _ := geom.Describe(back.Layers[0])
	col, _ := d.Source.Float64Column("y")
	if !isNaN(col[1]) {
		t.Errorf("the hole came back as %v, want NaN", col[1])
	}
}

func TestAFacetSurvivesTheRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    *facet.Spec
	}{
		{"wrap", facet.Wrap("region", facet.Columns(3))},
		{"grid", facet.Grid("region", "region")},
		{"free", facet.Wrap("region", facet.Free())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := spec.Chart{
				Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
				Facet:  tc.f,
				Layers: []geom.Geom{geom.Line(table(), geom.X("x"), geom.Y("y"))},
			}
			back := roundTrip(t, c)
			if back.Facet == nil {
				t.Fatal("the facet was lost")
			}
			if got, want := back.Facet.Describe(), tc.f.Describe(); got != want {
				t.Errorf("facet = %+v, want %+v", got, want)
			}
		})
	}
}

func TestTheDocumentIsVegaLiteShaped(t *testing.T) {
	c := spec.Chart{
		Width: 640, Height: 480, DPR: 1, Theme: theme.Dark, Title: "Signal",
		XTitle: "t", YTitle: "v",
		X:      scale.Linear(scale.Nice()),
		Y:      scale.Linear(scale.Nice()),
		Layers: []geom.Geom{geom.Line(table(), geom.X("x"), geom.Y("y"))},
	}
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"$schema", "width", "height", "title", "data", "encoding", "layer"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("the document has no %q", key)
		}
	}
	if doc["$schema"] != spec.Schema {
		t.Errorf("$schema = %v", doc["$schema"])
	}
	enc := doc["encoding"].(map[string]any)
	x := enc["x"].(map[string]any)
	if x["type"] != "quantitative" {
		t.Errorf("x.type = %v, want Vega-Lite's measurement type", x["type"])
	}
	if x["title"] != "t" {
		t.Errorf("x.title = %v", x["title"])
	}
	layer := doc["layer"].([]any)[0].(map[string]any)
	if layer["mark"].(map[string]any)["type"] != "line" {
		t.Errorf("mark.type = %v", layer["mark"])
	}
	if doc["config"].(map[string]any)["theme"] != "dark" {
		t.Errorf("config.theme = %v", doc["config"])
	}
}

func TestAHandWrittenSpecReads(t *testing.T) {
	// No $schema, no parse map, no scale types: the shape a person types.
	const doc = `{
	  "width": 300, "height": 200,
	  "data": {"values": [{"a": 1, "b": 2}, {"a": 2, "b": 5}]},
	  "encoding": {"x": {"type": "quantitative"}, "y": {"type": "quantitative"}},
	  "layer": [{"mark": {"type": "line", "color": "#f0c"}, "encoding": {"x": {"field": "a"}, "y": {"field": "b"}}}]
	}`
	s, err := spec.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Chart()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Layers) != 1 {
		t.Fatalf("layers = %d", len(c.Layers))
	}
	d, _ := geom.Describe(c.Layers[0])
	if d.Mark != geom.MarkLine {
		t.Errorf("mark = %q", d.Mark)
	}
	if d.Color == nil || *d.Color != ir.RGB(0xff, 0x00, 0xcc) {
		t.Errorf("colour = %v, want the short hex expanded", d.Color)
	}
	if n := d.Source.Len(); n != 2 {
		t.Errorf("rows = %d", n)
	}
	if _, ok := d.Source.Float64Column("a"); !ok {
		t.Error("an inferred numeric column did not come back numeric")
	}
	draw(t, c)
}

func TestALayerThatCannotDescribeItselfIsAnError(t *testing.T) {
	_, err := spec.Of(spec.Chart{Layers: []geom.Geom{opaqueGeom{}}})
	if err == nil {
		t.Fatal("a layer that cannot describe itself was written down anyway")
	}
	if !strings.Contains(err.Error(), "geom.Describer") {
		t.Errorf("error = %v, want it to name the interface", err)
	}
}

func TestAnUnknownMarkIsAnError(t *testing.T) {
	s := spec.Spec{Layer: []spec.Layer{{Mark: spec.Mark{Type: "violin"}}}}
	if _, err := s.Chart(); err == nil {
		t.Fatal("an unknown mark was accepted")
	}
}

func TestAnUnknownThemeIsAnError(t *testing.T) {
	s := spec.Spec{Config: &spec.Config{Theme: "chartreuse"}}
	if _, err := s.Chart(); err == nil {
		t.Fatal("an unknown theme was accepted")
	}
}

type opaqueGeom struct{}

func (opaqueGeom) Train(scale.Scale, scale.Scale) error       { return nil }
func (opaqueGeom) Build(ir.Backend, geom.Frame) error         { return nil }
func (opaqueGeom) Legend(geom.Frame) (geom.LegendEntry, bool) { return geom.LegendEntry{}, false }

func nan() float64         { return zero() / zero() }
func zero() float64        { return 0 }
func isNaN(v float64) bool { return v != v }

func TestMissingAndDecimationSurvive(t *testing.T) {
	src := table()
	for _, tc := range []struct {
		name string
		opts []geom.Option
	}{
		{"interpolate", []geom.Option{geom.OnMissing(geom.Interpolate)}},
		{"error", []geom.Option{geom.OnMissing(geom.Error)}},
		{"none", []geom.Option{geom.Decimate(geom.NoDecimation)}},
		{"lttb", []geom.Option{geom.Decimate(geom.LTTB), geom.Budget(32)}},
		{"minmax", []geom.Option{geom.Decimate(geom.MinMax)}},
		{"density", []geom.Option{geom.Decimate(geom.DensityRaster), geom.DensityCells(4)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]geom.Option{geom.X("x"), geom.Y("y")}, tc.opts...)
			c := spec.Chart{
				Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
				Layers: []geom.Geom{geom.Scatter(src, opts...)},
			}
			before, _ := geom.Describe(c.Layers[0])
			after, ok := geom.Describe(roundTrip(t, c).Layers[0])
			if !ok {
				t.Fatal("the layer came back undescribable")
			}
			if after.Missing != before.Missing {
				t.Errorf("missing policy = %v, want %v", after.Missing, before.Missing)
			}
			if after.Decimate != before.Decimate {
				t.Errorf("decimation = %v, want %v", after.Decimate, before.Decimate)
			}
			if after.Budget != before.Budget || after.CellSize != before.CellSize {
				t.Errorf("budget/cells = %v/%v, want %v/%v",
					after.Budget, after.CellSize, before.Budget, before.CellSize)
			}
		})
	}
}

func TestADefaultIsNotWrittenDown(t *testing.T) {
	// A document should say what was chosen, not repeat what was not — and a
	// line has no whisker extent to report in the first place.
	s, err := spec.Of(spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		Layers: []geom.Geom{geom.Line(table(), geom.X("x"), geom.Y("y"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := s.Layer[0].Mark
	if m.Missing != "" || m.Decimate != "" {
		t.Errorf("the default policy and reduction were written out: %+v", m)
	}
	if m.Extent != 0 || m.Outliers != nil || m.BarWidth != nil {
		t.Errorf("a line carries a boxplot's and a bar's options: %+v", m)
	}
}

func TestTimeDomainsAndDatumsAreTimestamps(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)
	x := scale.Time()
	x.(scale.Zoomer).SetDomain(scale.Nanos(from), scale.Nanos(to))

	c := spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		X:      x,
		Layers: []geom.Geom{geom.VLine(scale.Nanos(from.Add(time.Hour)))},
	}
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	dom := s.Encoding.X.Scale.Domain
	if len(dom) != 2 || dom[0] != from.Format(time.RFC3339Nano) {
		t.Errorf("domain = %v, want timestamps a person can read", dom)
	}
	if got, ok := s.Layer[0].Encoding.X.Datum.(string); !ok || got == "" {
		t.Errorf("the annotation's datum is %v, want a timestamp on a temporal axis", s.Layer[0].Encoding.X.Datum)
	}

	back, err := s.Chart()
	if err != nil {
		t.Fatal(err)
	}
	lo, hi := back.X.Domain()
	if lo != scale.Nanos(from) || hi != scale.Nanos(to) {
		t.Errorf("the domain came back as %v..%v", lo, hi)
	}
	d, _ := geom.Describe(back.Layers[0])
	if d.Datum.X0 != scale.Nanos(from.Add(time.Hour)) {
		t.Errorf("the datum came back as %v", d.Datum.X0)
	}
}

func TestBadColoursAndDomainsAreErrors(t *testing.T) {
	cases := []string{
		`{"layer":[{"mark":{"type":"line","color":"zzz"},"encoding":{"x":{"field":"a"},"y":{"field":"b"}}}]}`,
		`{"layer":[{"mark":{"type":"line","fill":"#gg"},"encoding":{"x":{"field":"a"},"y":{"field":"b"}}}]}`,
		`{"encoding":{"x":{"scale":{"type":"linear","domain":[1]}}}}`,
		`{"encoding":{"x":{"scale":{"type":"parabolic"}}}}`,
		`{"encoding":{"x":{"scale":{"type":"time","timeZone":"Mars/Olympus"}}}}`,
		`{"layer":[{"mark":{"type":"point"},"encoding":{"x":{"field":"a"},"y":{"field":"b"},` +
			`"color":{"field":"c","scale":{"scheme":"nosuchramp"}}}}]}`,
		`{"layer":[{"mark":{"type":"point"},"encoding":{"x":{"field":"a"},"y":{"field":"b"},` +
			`"color":{"field":"c"}}}]}`,
	}
	for _, doc := range cases {
		s, err := spec.Parse([]byte(doc))
		if err != nil {
			t.Fatalf("%s: %v", doc, err)
		}
		if _, err := s.Chart(); err == nil {
			t.Errorf("accepted: %s", doc)
		}
	}
}

func TestARampCanBeRegistered(t *testing.T) {
	mine := palette.Ramp{ir.RGB(9, 9, 9), ir.RGB(90, 90, 90)}
	palette.RegisterRamp("test-ramp", mine)
	if got, ok := palette.RampByName("test-ramp"); !ok || len(got) != 2 {
		t.Fatalf("RampByName = %v, %v", got, ok)
	}
	if name, ok := palette.RampName(mine); !ok || name != "test-ramp" {
		t.Errorf("RampName = %q, %v", name, ok)
	}
	c := spec.Chart{
		Width: 400, Height: 300, DPR: 1, Theme: theme.Light,
		Layers: []geom.Geom{geom.Scatter(table(),
			geom.X("x"), geom.Y("y"), geom.ColorBy("z", scale.Sequential(mine)))},
	}
	s, err := spec.Of(c)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Layer[0].Encoding.Color.Scale.Scheme; got != "test-ramp" {
		t.Errorf("scheme = %q, want the registered name", got)
	}
	// A registered ramp is written by name, so it is not written out.
	if got := s.Layer[0].Encoding.Color.Scale.Range; len(got) != 0 {
		t.Errorf("range = %v, want nothing: the ramp has a name", got)
	}
}

func TestQuotedNumbersAndUnixNanosecondsRead(t *testing.T) {
	// The parse map is Vega-Lite's answer to a column that arrived as strings,
	// and honouring it is the point of having one.
	doc := `{
	  "data": {"values": [{"t": "2026-01-01T00:00:00Z", "y": 1}, {"t": 2000000000, "y": 2}],
	           "format": {"parse": {"t": "date", "y": "number"}}},
	  "encoding": {"x": {"type": "temporal", "scale": {"type": "time"}}},
	  "layer": [{"mark": {"type": "line"}, "encoding": {"x": {"field": "t"}, "y": {"field": "y"}}}]
	}`
	s, err := spec.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Chart()
	if err != nil {
		t.Fatal(err)
	}
	d, _ := geom.Describe(c.Layers[0])
	ts, ok := d.Source.TimeColumn("t")
	if !ok || len(ts) != 2 {
		t.Fatalf("time column = %v, %v", ts, ok)
	}
	if ts[0].UTC().Year() != 2026 {
		t.Errorf("the timestamp string read as %v", ts[0])
	}
	if ts[1].UTC() != time.Unix(0, 2000000000).UTC() {
		t.Errorf("the numeric timestamp read as %v", ts[1])
	}
}
