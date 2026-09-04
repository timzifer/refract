package refract_test

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/facet"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// The v0.5 milestone, end to end: a chart can be written down and read back, a
// pointer can ask it what it is over, a redraw repaints only what moved, and a
// producer can append to it while it is being drawn.

func dashboard() *refract.Plot {
	src := refract.NewTable().
		Float64("x", []float64{0, 1, 2, 3, 4}).
		Float64("y", []float64{2, 5, 3, 9, 4}).
		Float64("z", []float64{10, 20, 30, 40, 50}).
		String("region", []string{"north", "south", "north", "south", "north"})

	p := refract.New(
		refract.Size(640, 400),
		refract.Theme(theme.Dark),
		refract.Title("Throughput"),
		refract.XTitle("batch"),
		refract.YTitle("rows/s"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice(), scale.Zero()))
	p.Add(
		geom.Line(src, geom.X("x"), geom.Y("y"), geom.Color(palette.Blue), geom.Label("measured")),
		geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.ColorBy("z", scale.Sequential(palette.Viridis))),
		geom.HLine(8, geom.Label("limit"), geom.Dash(4, 2)),
		geom.Note(3, 9, "peak"),
	)
	return p
}

func TestAChartSurvivesBeingWrittenDown(t *testing.T) {
	p := dashboard()

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	q, err := refract.ParseJSON(b)
	if err != nil {
		t.Fatalf("ParseJSON: %v\n%s", err, b)
	}

	var want, got bytes.Buffer
	if err := p.Render(refract.SVGWriter(&want)); err != nil {
		t.Fatal(err)
	}
	if err := q.Render(refract.SVGWriter(&got)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want.Bytes(), got.Bytes()) {
		t.Errorf("the chart read back from JSON draws differently: %d bytes against %d",
			got.Len(), want.Len())
	}
	// And the document is one a person can read: it says what it is in the
	// vocabulary of the format it borrows from.
	for _, s := range []string{`"mark"`, `"encoding"`, `"data"`, `"$schema"`, `"layer"`} {
		if !strings.Contains(string(b), s) {
			t.Errorf("the document has no %s", s)
		}
	}
}

func TestAFacetedChartSurvivesBeingWrittenDown(t *testing.T) {
	p := dashboard()
	p.Facet(facet.Wrap("region", facet.Columns(2)))

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	q, err := refract.ParseJSON(b)
	if err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	var want, got bytes.Buffer
	if err := p.Render(refract.SVGWriter(&want)); err != nil {
		t.Fatal(err)
	}
	if err := q.Render(refract.SVGWriter(&got)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want.Bytes(), got.Bytes()) {
		t.Error("a faceted chart read back from JSON draws differently")
	}
}

func TestAPlotRoundTripsThroughUnmarshalJSON(t *testing.T) {
	b, err := json.Marshal(dashboard())
	if err != nil {
		t.Fatal(err)
	}
	var p refract.Plot
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	var buf bytes.Buffer
	if err := p.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatalf("the unmarshalled plot does not render: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("the unmarshalled plot rendered nothing")
	}
}

func TestAPointerFindsTheRowUnderIt(t *testing.T) {
	p := dashboard()

	var hovered []refract.Event
	p.On(refract.Hover, func(ev refract.Event) { hovered = append(hovered, ev) })

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	panel := live.Index().Panels()[0]
	at := ir.Point{X: panel.X.Map(3), Y: panel.Y.Map(9)}
	ev := live.Move(float64(at.X), float64(at.Y))

	if !ev.Found {
		t.Fatal("the pointer found nothing where a row is")
	}
	if math.Abs(ev.Hit.X-3) > 0.05 || math.Abs(ev.Hit.Y-9) > 0.05 {
		t.Errorf("the hit reports (%v,%v), want the row at (3,9)", ev.Hit.X, ev.Hit.Y)
	}
	if len(hovered) != 1 || hovered[0].Kind != refract.Hover {
		t.Errorf("the handler saw %v", hovered)
	}

	// Off the panel entirely: one Leave, and no more after that.
	if got := live.Move(2, 2); got.Kind != refract.Leave {
		t.Errorf("moving into the margins fired %v, want Leave", got.Kind)
	}
	if got := live.Move(3, 3); got.Kind == refract.Leave {
		t.Error("Leave fired twice for one departure")
	}
}

func TestARedrawRepaintsOnlyWhatMoved(t *testing.T) {
	// A pinned axis and moving data: the frame the chart draws is the same
	// frame apart from the line, which is the case damage tracking is for.
	y := []float64{2, 5, 3, 9, 4}
	src := refract.Float64Columns(map[string][]float64{"x": {0, 1, 2, 3, 4}, "y": y})

	p := refract.New(refract.Size(400, 300))
	p.X(scale.Linear(scale.Domain(0, 4)))
	p.Y(scale.Linear(scale.Domain(0, 10)))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()

	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	if len(rec.Whole) != 1 || !rec.Whole[0] {
		t.Fatalf("the first frame was not a full repaint: %v", rec.Whole)
	}

	// Nothing changed: nothing is painted at all.
	frames := rec.Frames
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	if rec.Frames != frames {
		t.Error("an unchanged chart was repainted")
	}

	// One point moves.
	y[2] = 6
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	if rec.Frames != frames+1 {
		t.Fatalf("the changed chart was not painted")
	}
	if len(rec.Damaged) != 2 {
		t.Fatalf("Damage was called %d times, want one per painted frame", len(rec.Damaged))
	}
	damage := rec.Damaged[1]
	if len(damage) == 0 {
		t.Fatal("a changed chart reported no damage")
	}
	// The unit of damage is a drawing call, and the line is one call — so a
	// moved point repaints the line's box, which spans the plot. What it must
	// not repaint is everything else: the title, the axes, the tick labels and
	// the margins are untouched, and that is most of the canvas.
	plot := live.Index().Panels()[0].Area
	var area float32
	for _, r := range damage {
		area += r.Dx() * r.Dy()
		if r.Min.X < plot.Min.X-2 || r.Max.X > plot.Max.X+2 ||
			r.Min.Y < plot.Min.Y-2 || r.Max.Y > plot.Max.Y+2 {
			t.Errorf("damage %v reaches outside the plot area %v: the furniture did not change", r, plot)
		}
	}
	if whole := float32(400 * 300); area > whole*2/3 {
		t.Errorf("repainting %.0f of %.0f square pixels for one moved point", area, whole)
	}
}

func TestAStreamDrawsAConsistentViewWhileItGrows(t *testing.T) {
	const window = 512
	st := data.NewStream("t", "y").Window(window)

	p := refract.New(refract.Size(600, 300), refract.Title("Live"))
	p.X(scale.Time())
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(st.Source(), geom.X("t"), geom.Y("y")))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()

	// A producer on its own goroutine, a renderer on this one. Under -race
	// this is the whole claim.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		base := time.Now()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			st.AppendTime(base.Add(time.Duration(i)*time.Millisecond), math.Sin(float64(i)/10))
		}
	}()

	for range 20 {
		st.Snapshot()
		if err := live.Draw(); err != nil {
			t.Fatalf("Draw: %v", err)
		}
		// Every column of the frozen view has the same length: that is what
		// "a consistent snapshot" means, and it is what an unlocked read
		// would break.
		src := st.Source()
		x, _ := src.Float64Column("t")
		y, _ := src.Float64Column("y")
		if len(x) != len(y) {
			t.Fatalf("the snapshot is ragged: %d timestamps, %d values", len(x), len(y))
		}
		if len(x) > window {
			t.Fatalf("the window held %d rows, want at most %d", len(x), window)
		}
	}
	close(stop)
	wg.Wait()

	if rec.Frames == 0 {
		t.Error("nothing was drawn")
	}
}

func TestZoomingAndPanningMoveTheAxes(t *testing.T) {
	p := dashboard()
	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	panel := live.Index().Panels()[0]
	before0, before1 := panel.X.Domain()
	mid := ir.Point{
		X: (panel.Area.Min.X + panel.Area.Max.X) / 2,
		Y: (panel.Area.Min.Y + panel.Area.Max.Y) / 2,
	}

	var zooms, pans int
	p.On(refract.Zoom, func(refract.Event) { zooms++ })
	p.On(refract.Pan, func(refract.Event) { pans++ })

	if err := live.Wheel(float64(mid.X), float64(mid.Y), 0.5); err != nil {
		t.Fatal(err)
	}
	after0, after1 := panel.X.Domain()
	if after1-after0 >= (before1-before0)/1.5 {
		t.Errorf("zooming in left the domain at %v..%v, from %v..%v", after0, after1, before0, before1)
	}
	// The value under the pointer did not move.
	if got, want := panel.X.Invert(mid.X), (before0+before1)/2; math.Abs(got-want) > 0.05 {
		t.Errorf("the value under the pointer moved from %v to %v", want, got)
	}

	live.Move(float64(mid.X), float64(mid.Y))
	panned0, _ := panel.X.Domain()
	if err := live.PanBy(20, 0); err != nil {
		t.Fatal(err)
	}
	if now, _ := panel.X.Domain(); now >= panned0 {
		t.Errorf("panning right moved the domain from %v to %v, want it earlier", panned0, now)
	}

	if err := live.Autoscale(); err != nil {
		t.Fatal(err)
	}
	if lo, hi := panel.X.Domain(); lo != before0 || hi != before1 {
		t.Errorf("autoscale left the domain at %v..%v, want the original %v..%v", lo, hi, before0, before1)
	}
	if zooms != 1 || pans != 1 {
		t.Errorf("handlers saw %d zooms and %d pans, want one of each", zooms, pans)
	}
}

func TestAWatchedRenderDrawsWhatAnUnwatchedOneDoes(t *testing.T) {
	// Interaction must not change the picture. One set of golden files covers
	// both paths only if this holds.
	var plain bytes.Buffer
	if err := dashboard().Render(refract.SVGWriter(&plain)); err != nil {
		t.Fatal(err)
	}

	var live bytes.Buffer
	l, err := dashboard().Live(refract.SVGWriter(&live))
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Draw(); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain.Bytes(), live.Bytes()) {
		t.Errorf("a live render differs from a static one: %d bytes against %d", live.Len(), plain.Len())
	}
}
