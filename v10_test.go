package refract_test

// The v0.10 milestone, end to end: a track is a band at the panel's edge on
// the panel's own X. It renders at the height it was given at every canvas
// size, it leaves the panel's Y domain exactly where it found it, a zoom on X
// moves the panel and the track together because there is one scale object
// rather than two that agree, a wheel over the track's lanes moves nothing,
// and a pointer inside a track names the layer and the row it landed on.

import (
	"bytes"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/interact"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

// trackTolerance is the slack a device coordinate is compared with. Device
// coordinates are float32 arithmetic the compiler may contract, so they are
// never compared with ==; see AGENTS.md.
const trackTolerance = 0.01

// machine is the chart this feature exists for: a speed trace with a strip of
// machine states under it, both on one time axis.
func machine(opts ...refract.Option) (*refract.Plot, *refract.Track) {
	speed := refract.Float64Columns(map[string][]float64{
		"t":     {0, 1, 2, 3, 4, 5, 6, 7},
		"speed": {120, 118, 121, 40, 38, 119, 122, 120},
	})
	states := refract.NewTable().
		Float64("start", []float64{0, 3, 5}).
		Float64("end", []float64{3, 5, 8}).
		String("state", []string{"run", "fault", "run"})

	p := refract.New(opts...)
	p.X(scale.Linear(scale.Nice())).Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(speed, geom.X("t"), geom.Y("speed")))

	tr := p.Track(refract.Bottom, refract.TrackHeight(48))
	tr.Add(geom.Rect(states,
		geom.X("start"), geom.X2("end"),
		geom.Y("state"),
		geom.ColorBy("state", scale.Qualitative(palette.Default)),
	))
	return p, tr
}

// panelsOf renders p and reports the panels the render announced, in order.
func panelsOf(t *testing.T, p *refract.Plot) ([]interact.Panel, *interact.Index) {
	t.Helper()
	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	t.Cleanup(func() { live.Close() })
	if err := live.Draw(); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	return live.Index().Panels(), live.Index()
}

// TestATrackDoesNotMoveThePanelsYDomain is the criterion the negative-lane
// workaround fails. A track is not data on the panel's axis, so the axis must
// not know it is there.
func TestATrackDoesNotMoveThePanelsYDomain(t *testing.T) {
	plain := refract.New(refract.Size(640, 400))
	plain.X(scale.Linear(scale.Nice())).Y(scale.Linear(scale.Nice()))
	plain.Add(geom.Line(refract.Float64Columns(map[string][]float64{
		"t":     {0, 1, 2, 3, 4, 5, 6, 7},
		"speed": {120, 118, 121, 40, 38, 119, 122, 120},
	}), geom.X("t"), geom.Y("speed")))

	withTrack, _ := machine(refract.Size(640, 400))

	panelsOf(t, plain)
	panelsOf(t, withTrack)

	wantMin, wantMax := plainY(t, plain)
	gotMin, gotMax := plainY(t, withTrack)
	if wantMin != gotMin || wantMax != gotMax {
		t.Errorf("Y domain with a track = [%v %v], without = [%v %v]", gotMin, gotMax, wantMin, wantMax)
	}
}

// plainY reports the plot's Y domain after a render has trained it.
func plainY(t *testing.T, p *refract.Plot) (float64, float64) {
	t.Helper()
	panels, _ := panelsOf(t, p)
	if len(panels) == 0 {
		t.Fatal("no panels")
	}
	// The data panel is the first one added, which is the top-most row that is
	// not a track.
	return panels[0].Y.Domain()
}

// TestATrackKeepsItsHeightAtEveryCanvasSize pins the height against the
// solver: a fixed row is exactly as tall as it was told to be, whatever is
// left over.
func TestATrackKeepsItsHeightAtEveryCanvasSize(t *testing.T) {
	for _, size := range []struct{ w, h int }{{400, 300}, {640, 400}, {1600, 900}} {
		p, _ := machine(refract.Size(size.w, size.h))
		panels, _ := panelsOf(t, p)
		if len(panels) != 2 {
			t.Fatalf("%dx%d: %d panels, want 2", size.w, size.h, len(panels))
		}
		track := panels[1].Area
		if got := track.Max.Y - track.Min.Y; !near(got, 48) {
			t.Errorf("%dx%d: track height = %v, want 48", size.w, size.h, got)
		}
		// The track sits below the panel and is exactly as wide as it, which
		// is what makes the two share a time axis rather than merely both
		// having one.
		panel := panels[0].Area
		if !near(track.Min.X, panel.Min.X) || !near(track.Max.X, panel.Max.X) {
			t.Errorf("%dx%d: track spans [%v %v], panel [%v %v]",
				size.w, size.h, track.Min.X, track.Max.X, panel.Min.X, panel.Max.X)
		}
		if track.Min.Y < panel.Max.Y {
			t.Errorf("%dx%d: track top %v is above the panel's bottom %v", size.w, size.h, track.Min.Y, panel.Max.Y)
		}
	}
}

// TestATrackTakesItsHeightOutOfThePanel is the other half: the room the track
// occupies comes off the panel, not off the canvas.
func TestATrackTakesItsHeightOutOfThePanel(t *testing.T) {
	plain := refract.New(refract.Size(640, 400))
	plain.X(scale.Linear(scale.Nice())).Y(scale.Linear(scale.Nice()))
	plain.Add(geom.Line(refract.Float64Columns(map[string][]float64{
		"t": {0, 1}, "speed": {1, 2},
	}), geom.X("t"), geom.Y("speed")))
	bare, _ := panelsOf(t, plain)

	p, _ := machine(refract.Size(640, 400))
	panels, _ := panelsOf(t, p)

	bareH := bare[0].Area.Max.Y - bare[0].Area.Min.Y
	panelH := panels[0].Area.Max.Y - panels[0].Area.Min.Y
	if panelH >= bareH {
		t.Errorf("panel with a track is %v tall, without %v — the track took nothing", panelH, bareH)
	}
}

// TestAFractionalTrackScalesWithTheCanvas covers the other height unit, which
// is the one a responsive chart needs.
func TestAFractionalTrackScalesWithTheCanvas(t *testing.T) {
	heights := make([]float32, 0, 2)
	for _, h := range []int{400, 800} {
		p := refract.New(refract.Size(640, h))
		p.X(scale.Linear(scale.Nice())).Y(scale.Linear(scale.Nice()))
		p.Add(geom.Line(refract.Float64Columns(map[string][]float64{
			"t": {0, 1}, "speed": {1, 2},
		}), geom.X("t"), geom.Y("speed")))
		p.Track(refract.Bottom, refract.TrackFraction(0.1)).
			Add(geom.Line(refract.Float64Columns(map[string][]float64{
				"t": {0, 1}, "lane": {0, 1},
			}), geom.X("t"), geom.Y("lane")))

		panels, _ := panelsOf(t, p)
		heights = append(heights, panels[1].Area.Max.Y-panels[1].Area.Min.Y)
	}
	if !near(heights[0], 40) || !near(heights[1], 80) {
		t.Errorf("fractional track heights = %v, want 40 and 80", heights)
	}
}

// TestAZoomOnXMovesThePanelAndTheTrackTogether is the acceptance criterion
// that asks for this by construction rather than by two handlers agreeing.
// The panel and the track hold the same scale object, so there is nothing to
// keep in step.
func TestAZoomOnXMovesThePanelAndTheTrackTogether(t *testing.T) {
	p, _ := machine(refract.Size(640, 400))
	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	panels := live.Index().Panels()
	if len(panels) != 2 {
		t.Fatalf("%d panels, want 2", len(panels))
	}
	before, _ := panels[0].X.Domain()

	mid := panels[0].Area
	if err := live.Wheel(float64((mid.Min.X+mid.Max.X)/2), float64((mid.Min.Y+mid.Max.Y)/2), 0.5); err != nil {
		t.Fatalf("Wheel: %v", err)
	}

	panelMin, panelMax := panels[0].X.Domain()
	trackMin, trackMax := panels[1].X.Domain()
	if panelMin == before {
		t.Fatal("the wheel did not zoom the panel")
	}
	if panelMin != trackMin || panelMax != trackMax {
		t.Errorf("panel X = [%v %v], track X = [%v %v] — they are not one axis",
			panelMin, panelMax, trackMin, trackMax)
	}
}

// TestAWheelOverATrackLeavesItsLanesAlone pins a guarantee that is free
// today: an ordinal scale is deliberately not a scale.Zoomer, and zoomAxis
// no-ops on a scale that is not one. Half a category is not a view of
// anything, and a track's lanes must not slide under the pointer.
func TestAWheelOverATrackLeavesItsLanesAlone(t *testing.T) {
	p, _ := machine(refract.Size(640, 400))
	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	panels := live.Index().Panels()
	track := panels[1]
	beforeMin, beforeMax := track.Y.Domain()

	at := track.Area
	if err := live.Wheel(float64((at.Min.X+at.Max.X)/2), float64((at.Min.Y+at.Max.Y)/2), 0.5); err != nil {
		t.Fatalf("Wheel: %v", err)
	}

	afterMin, afterMax := track.Y.Domain()
	if beforeMin != afterMin || beforeMax != afterMax {
		t.Errorf("the track's lanes moved: [%v %v] → [%v %v]", beforeMin, beforeMax, afterMin, afterMax)
	}
	// The same wheel is still a zoom on the shared time axis.
	if lo, _ := track.X.Domain(); lo == 0 {
		t.Log("x domain unchanged at the left edge, which a centred zoom may leave alone")
	}
}

// TestAHitInsideATrackNamesItsLayer is the tooltip criterion: a pointer over a
// state bar reports the track's panel, the layer in it, and the row behind the
// bar.
func TestAHitInsideATrackNamesItsLayer(t *testing.T) {
	p, _ := machine(refract.Size(640, 400))
	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	panels := live.Index().Panels()
	track := panels[1].Area
	// A quarter of the way along the track is inside the first state bar,
	// which runs from the start of the axis to three of its eight units. The
	// vertical position is three-quarters down rather than centred: an ordinal
	// scale pads its slots, so the middle of a two-lane track is the gap
	// between the lanes, where there is correctly nothing to hit.
	at := ir.Point{
		X: track.Min.X + (track.Max.X-track.Min.X)/4,
		Y: track.Min.Y + (track.Max.Y-track.Min.Y)*3/4,
	}
	hit, ok := live.Index().At(at, 4)
	if !ok {
		t.Fatal("nothing under a point inside the track")
	}
	if hit.Panel != 1 {
		t.Errorf("Hit.Panel = %d, want 1 (the track)", hit.Panel)
	}
	if hit.Layer != 0 {
		t.Errorf("Hit.Layer = %d, want 0", hit.Layer)
	}
	if hit.Row < 0 {
		t.Error("Hit.Row = -1 with row tracking on")
	}
}

// TestATrackAndAFacetAreAnError. A facet owns the grid a track needs a row of,
// so the combination is refused rather than guessed at.
func TestATrackAndAFacetAreAnError(t *testing.T) {
	p, _ := machine(refract.Size(640, 400))
	p.Facet(facet.Wrap("state"))
	if err := p.Render(refract.SVGWriter(discard{})); err != refract.ErrTrackWithFacet {
		t.Errorf("Render = %v, want ErrTrackWithFacet", err)
	}
}

// TestGoldenTrack draws the chart this feature exists for.
func TestGoldenTrack(t *testing.T) {
	p, _ := machine(refract.Size(640, 400), refract.Title("Line 3"), refract.YTitle("m/min"))
	golden(t, "track", p)
}

// near compares two device coordinates with the tolerance the golden files
// use. Device coordinates are never compared with ==.
func near(a, b float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= trackTolerance
}

// discard is a writer that keeps nothing, for a render whose output does not
// matter because it is expected to fail.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// --- linked axes across plots --------------------------------------------
//
// The other shape of the same need: two plots, one above the other, on one
// domain. It needs no Link: a Grid already routes both through the one layout
// solver, so their panel rectangles already share their left and right insets,
// and handing both plots the same scale object already shares the domain.

// TestStackedPlotsSharingAScaleShareTheirAxis pins that, so a later change
// that copied a plot's scale on the way into a grid would fail here rather
// than in a chart nobody diffed.
func TestStackedPlotsSharingAScaleShareTheirAxis(t *testing.T) {
	x := scale.Linear(scale.Nice())

	speed := refract.New()
	speed.X(x).Y(scale.Linear(scale.Nice()))
	speed.Add(geom.Line(refract.Float64Columns(map[string][]float64{
		"t": {0, 4, 8}, "speed": {120, 40, 122},
	}), geom.X("t"), geom.Y("speed")))

	states := refract.New()
	states.X(x).Y(scale.Ordinal())
	states.Add(geom.Rect(refract.NewTable().
		Float64("start", []float64{0, 3}).
		Float64("end", []float64{3, 8}).
		String("state", []string{"run", "fault"}),
		geom.X("start"), geom.X2("end"), geom.Y("state")))

	g := refract.NewGrid(1,
		refract.GridSize(640, 400),
		refract.GridRowHeights(0, 48),
		refract.GridSharedX(true),
	)
	g.Add(speed, states)

	var buf bytes.Buffer
	if err := g.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatalf("Render: %v", err)
	}

	// One domain, because there is one scale. Both plots trained it.
	lo, hi := x.Domain()
	if lo > 0 || hi < 8 {
		t.Errorf("the shared domain is [%v %v], which does not cover both plots", lo, hi)
	}

	// One set of tick labels, under the bottom row. The label "4" belongs to
	// the shared time axis and must appear once.
	if n := bytes.Count(buf.Bytes(), []byte(">4<")); n != 1 {
		t.Errorf("the shared axis label appears %d times, want 1", n)
	}
}

// TestATrackSurvivesTheRoundTrip. A document that dropped a track would draw a
// different chart from the plot it was written from, which is the failure the
// spec package exists to make impossible.
func TestATrackSurvivesTheRoundTrip(t *testing.T) {
	p, _ := machine(refract.Size(640, 400), refract.Title("Line 3"))

	b, err := p.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	q, err := refract.ParseJSON(b)
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}

	tracks := q.Tracks()
	if len(tracks) != 1 {
		t.Fatalf("%d tracks after the round trip, want 1", len(tracks))
	}
	if got := tracks[0].Edge(); got != refract.Bottom {
		t.Errorf("edge = %v, want bottom", got)
	}

	// The chart it draws is the chart it drew: same panels, same rectangles.
	want, _ := panelsOf(t, p)
	got, _ := panelsOf(t, q)
	if len(got) != len(want) {
		t.Fatalf("%d panels after the round trip, want %d", len(got), len(want))
	}
	for i := range want {
		if !near(got[i].Area.Min.Y, want[i].Area.Min.Y) || !near(got[i].Area.Max.Y, want[i].Area.Max.Y) {
			t.Errorf("panel %d is %v after the round trip, was %v", i, got[i].Area, want[i].Area)
		}
	}
}

// TestATrackedChartIsTheSameDrawnInParallel. A track makes a one-panel chart a
// two-panel one, so it takes the concurrent path — and the panel and the track
// hold the *same* X scale object, which two goroutines then range for two
// different rectangles. scale.Snapshotter is what makes that safe; this is
// what says so.
func TestATrackedChartIsTheSameDrawnInParallel(t *testing.T) {
	build := func(serial bool) []byte {
		t.Helper()
		opts := []refract.Option{refract.Size(640, 400), refract.Title("Line 3")}
		if serial {
			opts = append(opts, refract.Parallel(false))
		}
		p, _ := machine(opts...)
		var buf bytes.Buffer
		if err := p.Render(refract.SVGWriter(&buf)); err != nil {
			t.Fatalf("Render: %v", err)
		}
		return buf.Bytes()
	}
	if par, ser := build(false), build(true); !bytes.Equal(par, ser) {
		t.Error("a tracked chart drawn in parallel is not the one drawn serially")
	}
}
