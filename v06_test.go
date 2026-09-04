package refract_test

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/mathtext"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// The v0.6 milestone, end to end: a chart says what it is in words, keeps its
// proportions at any size, sets the notation in its labels, tells its layers
// apart by more than colour, survives a zoom deep enough to run a float64 out
// of digits, and is steered by one input state machine wherever it is drawn.

func chart() *refract.Plot { p, _ := chartOn(scale.Linear(scale.Nice())); return p }

// chartOn is chart with its horizontal scale handed back, for the tests that
// ask what a zoom did to it. A Plot does not give its scales out: they are what
// it was configured with, and the caller who configured it has them.
func chartOn(x scale.Scale) (*refract.Plot, scale.Scale) {
	src := refract.NewTable().
		Float64("x", []float64{0, 1, 2, 3, 4}).
		Float64("y", []float64{2, 5, 3, 9, 4}).
		Float64("z", []float64{1, 4, 2, 8, 3})

	p := refract.New(
		refract.Size(640, 400),
		refract.Title("Throughput"),
		refract.XTitle("batch"),
		refract.YTitle("rows/s"),
	)
	p.X(x)
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(
		geom.Line(src, geom.X("x"), geom.Y("y"), geom.Label("measured")),
		geom.Line(src, geom.X("x"), geom.Y("z"), geom.Label("modelled")),
	)
	return p, x
}

// targetOf hands an existing backend out as a target.
func targetOf(b ir.Backend) ir.Target { return fixedTarget{b} }

type fixedTarget struct{ b ir.Backend }

func (t fixedTarget) Open(int, int, float64) (ir.Backend, error) { return t.b, nil }
func (fixedTarget) Close() error                                 { return nil }

// --- accessibility -------------------------------------------------------

func TestAChartCarriesItsNameIntoTheDocument(t *testing.T) {
	var buf bytes.Buffer
	if err := chart().Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	doc := buf.String()
	for _, want := range []string{`role="img"`, `aria-labelledby="refract-title"`, "<title id=\"refract-title\">Throughput</title>"} {
		if !strings.Contains(doc, want) {
			t.Errorf("the document is missing %q", want)
		}
	}
}

func TestADescribedChartCarriesTheDescriptionToo(t *testing.T) {
	p := chart()
	sum := p.Describe()
	if sum.Detail == "" {
		t.Fatal("Describe produced no description")
	}

	var buf bytes.Buffer
	if err := p.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	doc := buf.String()
	if !strings.Contains(doc, `aria-labelledby="refract-title refract-desc"`) {
		t.Error("the document does not point at both its title and its description")
	}
	if !strings.Contains(doc, "<desc id=\"refract-desc\">") {
		t.Error("the document has no <desc>")
	}
	if !strings.Contains(doc, "measured") || !strings.Contains(doc, "modelled") {
		t.Errorf("the description does not name the layers:\n%s", sum.Detail)
	}
}

func TestAnExplicitDescriptionWins(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {0, 1}})
	p := refract.New(refract.Title("Throughput"), refract.Description("Fig. 3", "Rows per second over five batches."))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))

	var buf bytes.Buffer
	if err := p.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "<title id=\"refract-title\">Fig. 3</title>") {
		t.Error("the explicit title is not in the document")
	} else if !strings.Contains(got, "Rows per second over five batches.") {
		t.Error("the explicit description is not in the document")
	}
}

// A chart with nothing to say gets no ARIA at all: an empty title announces an
// unnamed graphic, which is worse than a graphic a reader can skip.
func TestAnUnnamedChartGetsNoAria(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {0, 1}})
	p := refract.New()
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))

	var buf bytes.Buffer
	if err := p.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "role=\"img\"") {
		t.Error("a chart with no name was given an accessible one")
	}
}

// An interactive chart is the one that most needs a name, and it is drawn
// through a recorder rather than straight into the backend — so the
// description has to survive being recorded and replayed.
func TestALiveChartKeepsItsName(t *testing.T) {
	var buf bytes.Buffer
	live, err := chart().Live(refract.SVGWriter(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	if err := live.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "<title id=\"refract-title\">Throughput</title>") {
		t.Error("a live render lost the chart's accessible name")
	}
}

func TestTheDataTableIsTheFallback(t *testing.T) {
	var buf bytes.Buffer
	if err := chart().DataTable(&buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if n := strings.Count(html, "<table>"); n != 2 {
		t.Errorf("wrote %d tables for two layers", n)
	}
	for _, want := range []string{"<caption>measured</caption>", "<caption>modelled</caption>", "<td>9</td>"} {
		if !strings.Contains(html, want) {
			t.Errorf("the fallback is missing %q", want)
		}
	}
}

func TestRedundantEncodingTellsLayersApartWithoutColour(t *testing.T) {
	plain := irtest.New()
	if err := chartInto(plain, theme.Light); err != nil {
		t.Fatal(err)
	}
	redundant := irtest.New()
	if err := chartInto(redundant, theme.Light.With(theme.Redundant(true))); err != nil {
		t.Fatal(err)
	}

	// Without it the two lines differ in colour and in nothing else.
	if got := dashesDrawn(plain); len(got) != 0 {
		t.Errorf("a plain theme dashed %d lines", len(got))
	}
	// With it the first layer is still solid — a single-layer chart must not
	// change — and the second is not.
	got := dashesDrawn(redundant)
	if len(got) != 1 {
		t.Fatalf("redundant encoding dashed %d lines, want exactly the second", len(got))
	}
}

// chartInto draws a two-layer chart into a recorder under the given theme.
func chartInto(b ir.Backend, th theme.Theme) error {
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3, 4},
		"y": {2, 5, 3, 9, 4},
		"z": {1, 4, 2, 8, 3},
	})
	p := refract.New(refract.Size(640, 400), refract.Theme(th))
	p.Add(
		geom.Line(src, geom.X("x"), geom.Y("y")),
		geom.Line(src, geom.X("x"), geom.Y("z")),
	)
	return p.Render(targetOf(b))
}

// dashesDrawn reports the dash patterns of the polylines that carry data.
//
// A legend swatch is a polyline with the layer's own stroke on it, and it is
// dashed for the same reason the line is — so the data lines are picked out by
// having more than the swatch's two points.
func dashesDrawn(rec *irtest.Recorder) [][]float32 {
	var out [][]float32
	for _, c := range rec.Calls {
		if c.Op == "Polyline" && len(c.Stroke.Dash) > 0 && len(c.Points) > 2 {
			out = append(out, c.Stroke.Dash)
		}
	}
	return out
}

func TestRedundantEncodingLeavesAnExplicitChoiceAlone(t *testing.T) {
	th := theme.Light.With(theme.Redundant(true))
	rec := irtest.New()

	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {0, 1}, "z": {1, 0}})
	p := refract.New(refract.Theme(th))
	p.Add(
		geom.Line(src, geom.X("x"), geom.Y("y")),
		geom.Line(src, geom.X("x"), geom.Y("z"), geom.Dash(9, 9)),
	)
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, c := range rec.Calls {
		if c.Op == "Polyline" && len(c.Stroke.Dash) == 2 && c.Stroke.Dash[0] == 9 {
			found = true
		}
	}
	if !found {
		t.Error("the layer's own dash pattern was replaced by the theme's")
	}
}

// --- responsiveness ------------------------------------------------------

func TestAResponsiveChartScalesItsTypeWithItsSize(t *testing.T) {
	sizes := func(p *refract.Plot, w, h int) (float64, float32) {
		rec := irtest.New()
		live, err := p.Live(rec.Target())
		if err != nil {
			t.Fatal(err)
		}
		defer live.Close()
		// The first frame is drawn at the plot's own size; asking for another
		// one is what a resize is. A frame identical to the last is not
		// painted at all, so the recording is cleared rather than the chart
		// being drawn twice.
		if err := live.Draw(); err != nil {
			t.Fatal(err)
		}
		if pw, ph := p.Size(); w != pw || h != ph {
			rec.Calls = nil
			if err := live.Resize(w, h); err != nil {
				t.Fatal(err)
			}
		}
		var title float64
		var stroke float32
		for _, c := range rec.Calls {
			if c.Op == "Text" && c.Text.Text == "Throughput" {
				title = c.Text.Font.Size
			}
			if c.Op == "Polyline" && c.Stroke.Width > stroke {
				stroke = c.Stroke.Width
			}
		}
		return title, stroke
	}

	full, fullStroke := sizes(chart(), 640, 400)
	half, halfStroke := sizes(responsive(), 320, 200)
	if full == 0 || half == 0 {
		t.Fatalf("the title was not drawn: %v and %v", full, half)
	}
	if math.Abs(half-full/2) > 0.01 {
		t.Errorf("a half-size responsive chart set its title at %v, want %v", half, full/2)
	}
	if math.Abs(float64(halfStroke-fullStroke/2)) > 0.01 {
		t.Errorf("a half-size responsive chart stroked at %v, want %v", halfStroke, fullStroke/2)
	}
}

// responsive is the same chart with the option that makes it scale with its
// size. Responsive is an option, so it is given at construction rather than set
// on the chart above.
func responsive() *refract.Plot {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1, 2, 3, 4}, "y": {2, 5, 3, 9, 4}})
	p := refract.New(
		refract.Size(640, 400),
		refract.Responsive(true),
		refract.Title("Throughput"),
		refract.XTitle("batch"),
		refract.YTitle("rows/s"),
	)
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	return p
}

// A plain chart drawn at another size keeps its type: only a responsive one
// rescales, so turning the option on cannot change an existing still.
func TestAPlainChartKeepsItsTypeAtAnySize(t *testing.T) {
	rec := irtest.New()
	live, err := chart().Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	before := titleSize(rec)

	rec.Calls = nil
	if err := live.Resize(320, 200); err != nil {
		t.Fatal(err)
	}
	if after := titleSize(rec); after != before {
		t.Errorf("the title went from %v to %v without being asked to", before, after)
	}
}

func titleSize(rec *irtest.Recorder) float64 {
	for _, c := range rec.Calls {
		if c.Op == "Text" && c.Text.Text == "Throughput" {
			return c.Text.Font.Size
		}
	}
	return 0
}

func TestResizingKeepsTheZoom(t *testing.T) {
	rec := irtest.New()
	p, x := chartOn(scale.Linear(scale.Nice()))
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	if err := live.Wheel(300, 200, 0.5); err != nil {
		t.Fatal(err)
	}
	lo, hi := x.Domain()

	if err := live.Resize(500, 300); err != nil {
		t.Fatal(err)
	}
	if w, h := live.Size(); w != 500 || h != 300 {
		t.Errorf("the chart reports %dx%d after a resize", w, h)
	}
	if nlo, nhi := x.Domain(); nlo != lo || nhi != hi {
		t.Errorf("a resize moved the view from [%v %v] to [%v %v]", lo, hi, nlo, nhi)
	}
}

func TestABackendIsToldItsNewSize(t *testing.T) {
	rec := irtest.New()
	live, err := chart().Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	if err := live.Resize(300, 200); err != nil {
		t.Fatal(err)
	}
	if len(rec.Resized) != 1 || rec.Resized[0] != [2]int{300, 200} {
		t.Errorf("the backend was told %v, want one resize to 300x200", rec.Resized)
	}
}

// --- notation ------------------------------------------------------------

func TestNotationIsSetInEveryLabel(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {0, 1}})
	p := refract.New(
		refract.Size(400, 300),
		refract.Math(mathtext.TeX()),
		refract.Legend(true),
		refract.Title(`decay of $N_0e^{-\lambda t}$`),
		refract.YTitle(`$\sigma^2$`),
	)
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y"), geom.Label(`$\alpha$`)))

	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}

	var texts []string
	for _, c := range rec.Calls {
		if c.Op == "Text" {
			texts = append(texts, c.Text.Text)
		}
	}
	joined := strings.Join(texts, "|")
	for _, want := range []string{"λ", "σ", "α", "N"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%q was not set: %s", want, joined)
		}
	}
	for _, unwanted := range []string{`\lambda`, `\sigma`, "$"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("%q reached the backend as markup: %s", unwanted, joined)
		}
	}
}

func TestALabelIsMeasuredAsItIsDrawn(t *testing.T) {
	// The margin a chart leaves for its Y title comes from measuring that
	// title. A typeset title that measured as its markup would be given room
	// for a backslash and a brace.
	plain := yTitleX(t, refract.New(refract.Size(400, 300), refract.YTitle("sigma")))
	typeset := yTitleX(t, refract.New(
		refract.Size(400, 300),
		refract.Math(mathtext.TeX()),
		refract.YTitle(`$\sigma$`),
	))
	if math.Abs(plain-typeset) > 6 {
		t.Errorf("a one-character title was laid out at %v against %v for the plain one", typeset, plain)
	}
}

func yTitleX(t *testing.T, p *refract.Plot) float64 {
	t.Helper()
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {0, 1}})
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))

	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}
	// The Y title is the one run drawn rotated, or the one frame pushed for a
	// rotated layout.
	for _, c := range rec.Calls {
		if c.Op == "Text" && c.Text.Rotation != 0 {
			return float64(c.Text.At.X)
		}
		if c.Op == "Push" && c.Affine.B != 0 {
			return float64(c.Affine.E)
		}
	}
	t.Fatal("no rotated label was drawn")
	return 0
}

func TestAChartWithoutATypesetterLeavesLabelsAlone(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {0, 1}})
	p := refract.New(refract.Title(`$5 to $10`))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))

	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		t.Fatal(err)
	}
	for _, c := range rec.Calls {
		if c.Op == "Text" && c.Text.Text == `$5 to $10` {
			return
		}
	}
	t.Error("a label was not drawn as it was written")
}

func TestADescriptionReadsNotationAloud(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1}, "y": {0, 1}})
	p := refract.New(refract.Math(mathtext.TeX()), refract.YTitle(`$\sigma^2$`))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))

	sum := p.Describe()
	if strings.Contains(sum.Detail, `\sigma`) || strings.Contains(sum.Detail, "$") {
		t.Errorf("the description carries markup: %s", sum.Detail)
	}
	if !strings.Contains(sum.Detail, "σ^2") {
		t.Errorf("the description does not read the notation: %s", sum.Detail)
	}
}

// --- deep zoom -----------------------------------------------------------

func TestATimeAxisWithAnOriginKeepsItsNanoseconds(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	next := base.Add(time.Nanosecond)

	// Without an origin, a Unix nanosecond count in this century needs more
	// bits than a float64 has, and the two instants become one number.
	if scale.Nanos(base) != scale.Nanos(next) {
		t.Skip("this platform's float64 has more precision than the standard one")
	}

	s := scale.Time(scale.Origin(base))
	if s.(scale.Temporal).Value(base) == s.(scale.Temporal).Value(next) {
		t.Fatal("a rebased time scale still cannot separate two adjacent nanoseconds")
	}

	s.Train(s.(scale.Temporal).Value(base), s.(scale.Temporal).Value(next))
	s.SetRange(0, 100)
	if s.Map(s.(scale.Temporal).Value(base)) == s.Map(s.(scale.Temporal).Value(next)) {
		t.Error("two instants a nanosecond apart landed on the same position")
	}
}

func TestARebasedTimeAxisRoundTrips(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	src := refract.NewTable().
		Time("t", []time.Time{base, base.Add(time.Millisecond)}).
		Float64("y", []float64{1, 2})

	p := refract.New(refract.Size(400, 300))
	p.X(scale.Time(scale.Origin(base)))
	p.Add(geom.Line(src, geom.X("t"), geom.Y("y")))

	doc, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), `"origin"`) {
		t.Errorf("the document does not carry the axis's origin:\n%s", doc)
	}
	q, err := refract.ParseJSON(doc)
	if err != nil {
		t.Fatal(err)
	}

	want, err := trace(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := trace(q)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("a rebased chart came back drawing something else:\n%s\n%s", got, want)
	}
}

func trace(p *refract.Plot) (string, error) {
	rec := irtest.New()
	if err := p.Render(rec.Target()); err != nil {
		return "", err
	}
	return strings.Join(rec.Trace(), "\n"), nil
}

// A geom reads a time column through the axis's own space, so the rebasing
// reaches the data without anyone converting anything by hand.
func TestAGeomReadsTimeInTheAxisSpace(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	src := refract.NewTable().
		Time("t", []time.Time{base, base.Add(time.Second)}).
		Float64("y", []float64{1, 2})

	s := scale.Time(scale.Origin(base))
	p := refract.New(refract.Size(400, 300))
	p.X(s)
	p.Add(geom.Line(src, geom.X("t"), geom.Y("y")))
	if err := p.Render(irtest.New().Target()); err != nil {
		t.Fatal(err)
	}

	lo, hi := s.Domain()
	if lo != 0 || hi != float64(time.Second) {
		t.Errorf("the axis trained on [%v %v], want nanoseconds from its origin", lo, hi)
	}
}

// --- input ---------------------------------------------------------------

func TestOneStateMachineSteersEverySurface(t *testing.T) {
	p := chart()
	var kinds []refract.EventKind
	for _, k := range []refract.EventKind{refract.Hover, refract.Click, refract.Pan, refract.Zoom, refract.Leave} {
		p.On(k, func(ev refract.Event) { kinds = append(kinds, ev.Kind) })
	}

	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	in := live.Input()
	in.Move(300, 200) // hover
	in.Down(300, 200) // press
	in.Move(301, 200) // inside the slop: still not a drag
	in.Up(301, 200)   // a click, because the pointer barely moved
	in.Down(300, 200) //
	in.Move(360, 240) // past the slop: a pan
	in.Up(360, 240)   // the end of a drag is not a click
	in.Wheel(300, 200, -100)
	in.Leave()

	want := []refract.EventKind{
		refract.Hover, refract.Click, refract.Pan, refract.Zoom, refract.Leave,
	}
	if len(kinds) != len(want) {
		t.Fatalf("fired %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("fired %v, want %v", kinds, want)
		}
	}
}

func TestADragDoesNotAlsoClick(t *testing.T) {
	p := chart()
	clicks := 0
	p.On(refract.Click, func(refract.Event) { clicks++ })

	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	in := live.Input()
	in.Down(200, 200)
	in.Move(280, 200)
	in.Up(280, 200)
	if clicks != 0 {
		t.Errorf("a drag fired %d clicks", clicks)
	}
	if in.Dragging() {
		t.Error("the drag is still in progress after the release")
	}
}

func TestTheWheelZoomsAboutThePointer(t *testing.T) {
	p, x := chartOn(scale.Linear(scale.Nice()))
	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	before, _ := x.Domain()
	in := live.Input()
	if err := in.Wheel(300, 200, -240); err != nil {
		t.Fatal(err)
	}
	after, _ := x.Domain()
	if after == before {
		t.Error("the wheel did not zoom")
	}
	if refract.WheelFactor(-240) >= 1 {
		t.Error("scrolling towards the reader did not zoom in")
	}
	if err := in.DoubleClick(); err != nil {
		t.Fatal(err)
	}
	if reset, _ := x.Domain(); reset != before {
		t.Errorf("a double click left the axis at %v, want the original %v", reset, before)
	}
}

// A still is drawn once, at the size it was given, so a responsive one has to
// be told what size it was designed at. That is the thumbnail case, and it is
// the only place Responsive does anything to a Render.
func TestAThumbnailScalesFromItsDesignSize(t *testing.T) {
	full := irtest.New()
	if err := thumbnail(800, 500, false).Render(full.Target()); err != nil {
		t.Fatal(err)
	}
	small := irtest.New()
	if err := thumbnail(200, 125, true).Render(small.Target()); err != nil {
		t.Fatal(err)
	}

	got, want := titleSize(small), titleSize(full)/4
	if got == 0 || math.Abs(got-want) > 0.01 {
		t.Errorf("a quarter-size thumbnail set its title at %v, want %v", got, want)
	}
}

func thumbnail(w, h int, scaled bool) *refract.Plot {
	opts := []refract.Option{refract.Size(w, h), refract.Title("Throughput")}
	if scaled {
		opts = append(opts, refract.ResponsiveFrom(800, 500))
	}
	p := refract.New(opts...)
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1, 2}, "y": {1, 3, 2}})
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	return p
}

// A grid is a chart too: it carries a name into the document and sets the
// notation in its own labels. What it does not have is Describe — several
// charts on one page have no single reading, and whoever put them there is the
// one who knows why.
func TestAGridCarriesItsNameAndItsNotation(t *testing.T) {
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1, 2}, "y": {1, 3, 2}})
	panel := func(title string) *refract.Plot {
		p := refract.New(refract.Title(title))
		p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
		return p
	}

	g := refract.NewGrid(2,
		refract.GridSize(600, 400),
		refract.GridTitle("Fleet"),
		refract.GridMath(mathtext.TeX()),
		refract.GridDescription("Fleet overview", "Two panels, one per host."),
	)
	g.Add(panel(`host $\alpha$`), panel(`host $\beta$`))

	var buf bytes.Buffer
	if err := g.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	doc := buf.String()
	for _, want := range []string{
		"<title id=\"refract-title\">Fleet overview</title>",
		"Two panels, one per host.",
		"α", "β",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("the document is missing %q", want)
		}
	}
	if strings.Contains(doc, `\alpha`) {
		t.Error("a panel's notation was drawn as markup")
	}
}

// A surface has more ways to lose a press than to report one: a double click
// consumes the second press, a drag can start outside the chart, a window
// manager can take the pointer away. A release with nothing behind it must not
// invent a click, or resetting the view would open a tooltip every time.
func TestAReleaseWithNoPressIsNotAClick(t *testing.T) {
	p := chart()
	clicks := 0
	p.On(refract.Click, func(refract.Event) { clicks++ })

	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	in := live.Input()
	in.Up(300, 200) // a release out of nowhere
	if clicks != 0 {
		t.Errorf("a release with no press fired %d clicks", clicks)
	}

	in.Down(300, 200)
	in.DoubleClick() // the view is reset, and the press is spent
	in.Up(300, 200)
	if clicks != 0 {
		t.Errorf("a double click fired %d clicks on the way out", clicks)
	}

	in.Down(300, 200)
	in.Up(300, 200)
	if clicks != 1 {
		t.Errorf("an ordinary press and release fired %d clicks, want 1", clicks)
	}
}
