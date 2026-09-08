package refract_test

// Selection: a rectangle, the rows under it, and what a drag does about it.

import (
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

func selectLive(t *testing.T, layers ...geom.Geom) *refract.Live {
	t.Helper()
	p := refract.New(refract.Size(600, 300))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(layers...)
	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	return live
}

// boxAround is the rectangle enclosing where the given rows of a layer landed,
// padded so the marks are inside it rather than on its edge.
func boxAround(t *testing.T, l *refract.Live, layer int, rows ...int) ir.Rect {
	t.Helper()
	var r ir.Rect
	for n, row := range rows {
		at, ok := l.Index().Locate(0, layer, row)
		if !ok {
			t.Fatalf("row %d of layer %d was not located", row, layer)
		}
		if n == 0 {
			r = ir.Rect{Min: at, Max: at}
			continue
		}
		r.Min.X, r.Min.Y = min(r.Min.X, at.X), min(r.Min.Y, at.Y)
		r.Max.X, r.Max.Y = max(r.Max.X, at.X), max(r.Max.Y, at.Y)
	}
	r.Min.X, r.Min.Y = r.Min.X-2, r.Min.Y-2
	r.Max.X, r.Max.Y = r.Max.X+2, r.Max.Y+2
	return r
}

func TestSelectReportsTheRowsUnderTheRectangle(t *testing.T) {
	live := selectLive(t, geom.Scatter(keyTable(), geom.X("x"), geom.Y("y")))

	evs := live.Select(boxAround(t, live, 0, 1, 2))
	if len(evs) != 1 {
		t.Fatalf("events = %d, want one per layer touched", len(evs))
	}
	ev := evs[0]
	if ev.Kind != refract.Select {
		t.Errorf("kind = %v, want select", ev.Kind)
	}
	if ev.Hit.Layer != 0 {
		t.Errorf("layer = %d, want 0", ev.Hit.Layer)
	}
	got := map[int]bool{}
	for _, r := range ev.Rows {
		got[r] = true
	}
	if !got[1] || !got[2] {
		t.Errorf("rows = %v, want 1 and 2 among them", ev.Rows)
	}
	if got[0] || got[3] {
		t.Errorf("rows = %v, which reaches outside the rectangle", ev.Rows)
	}
}

// One event per layer, so a list of rows never has to say which layer's rows
// it holds.
func TestSelectFiresOncePerLayer(t *testing.T) {
	tbl := keyTable()
	live := selectLive(t,
		geom.Scatter(tbl, geom.X("x"), geom.Y("y")),
		geom.Line(tbl, geom.X("x"), geom.Y("y")),
	)
	// A box over everything, so both layers are in it.
	panel := live.Index().Panels()[0]
	evs := live.Select(panel.Area)

	if len(evs) != 2 {
		t.Fatalf("events = %d, want one per layer", len(evs))
	}
	seen := map[int]bool{}
	for _, ev := range evs {
		if seen[ev.Hit.Layer] {
			t.Errorf("layer %d was reported twice", ev.Hit.Layer)
		}
		seen[ev.Hit.Layer] = true
		for _, row := range ev.Rows {
			if row < 0 {
				t.Errorf("layer %d reported row %d", ev.Hit.Layer, row)
			}
		}
	}
	if !seen[0] || !seen[1] {
		t.Errorf("layers reported = %v, want both", seen)
	}
}

// Handlers registered with Plot.On see the selection too.
func TestSelectFiresTheHandler(t *testing.T) {
	p := refract.New(refract.Size(600, 300))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Scatter(keyTable(), geom.X("x"), geom.Y("y")))

	var got []refract.Event
	p.On(refract.Select, func(ev refract.Event) { got = append(got, ev) })

	live, _ := p.Live(irtest.New().Target())
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	live.Select(live.Index().Panels()[0].Area)

	if len(got) != 1 {
		t.Fatalf("the handler saw %d events, want 1", len(got))
	}
	if len(got[0].Rows) != 4 {
		t.Errorf("rows = %v, want all four", got[0].Rows)
	}
}

// Without row tracking there are no rows, so there is nothing to report — and
// reporting the marks instead would answer a question nobody asked.
func TestSelectNeedsRowTracking(t *testing.T) {
	p := refract.New(refract.Size(600, 300))
	p.Add(geom.Scatter(keyTable(), geom.X("x"), geom.Y("y")))
	live, _ := p.Live(irtest.New().Target())
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	if evs := live.Select(live.Index().Panels()[0].Area); len(evs) != 0 {
		t.Errorf("a selection with tracking off reported %d events", len(evs))
	}
}

// A drag pans by default, which is what a drag has always done.
func TestDragPansByDefault(t *testing.T) {
	live := selectLive(t, geom.Line(viewTable(), geom.X("x"), geom.Y("y")))
	before, _ := live.Index().Panels()[0].X.Domain()

	in := live.Input()
	in.Down(100, 100)
	in.Move(160, 100)
	in.Up(160, 100)

	if after, _ := live.Index().Panels()[0].X.Domain(); after == before {
		t.Error("a drag did not pan")
	}
}

// In DragSelects a drag marks out a region and moves nothing.
func TestDragSelectsDoesNotPan(t *testing.T) {
	p := refract.New(refract.Size(600, 300))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Scatter(keyTable(), geom.X("x"), geom.Y("y")))

	var got []refract.Event
	p.On(refract.Select, func(ev refract.Event) { got = append(got, ev) })

	live, _ := p.Live(irtest.New().Target())
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	before, _ := live.Index().Panels()[0].X.Domain()

	area := live.Index().Panels()[0].Area
	in := live.Input().Drag(refract.DragSelects)
	in.Down(float64(area.Min.X), float64(area.Min.Y))
	in.Move(float64(area.Max.X), float64(area.Max.Y))

	if _, ok := in.Dragged(); !ok {
		t.Error("no rubber band during a select drag")
	}
	in.Up(float64(area.Max.X), float64(area.Max.Y))

	if after, _ := live.Index().Panels()[0].X.Domain(); after != before {
		t.Errorf("a select drag panned the chart: %v became %v", before, after)
	}
	if len(got) != 1 {
		t.Fatalf("the drag fired %d select events, want 1", len(got))
	}
	if len(got[0].Rows) != 4 {
		t.Errorf("rows = %v, want all four", got[0].Rows)
	}
}

// In DragZooms a drag zooms to what it marked out.
func TestDragZoomsToTheBand(t *testing.T) {
	live := selectLive(t, geom.Line(viewTable(), geom.X("x"), geom.Y("y")))
	before, _ := live.Index().Panels()[0].X.Domain()

	area := live.Index().Panels()[0].Area
	in := live.Input().Drag(refract.DragZooms)
	in.Down(float64(area.Min.X+20), float64(area.Min.Y+20))
	in.Move(float64(area.Max.X-20), float64(area.Max.Y-20))
	in.Up(float64(area.Max.X-20), float64(area.Max.Y-20))

	after, _ := live.Index().Panels()[0].X.Domain()
	if after == before {
		t.Error("a zoom drag did not zoom")
	}
}

// A rubber band with no drag behind it is a click, and clicks still work in
// every mode.
func TestAClickIsStillAClickInEveryMode(t *testing.T) {
	for _, mode := range []refract.Drag{refract.DragPans, refract.DragSelects, refract.DragZooms} {
		t.Run(mode.String(), func(t *testing.T) {
			p := refract.New(refract.Size(600, 300))
			p.X(scale.Linear())
			p.Y(scale.Linear())
			p.Add(geom.Scatter(keyTable(), geom.X("x"), geom.Y("y")))

			clicks := 0
			p.On(refract.Click, func(refract.Event) { clicks++ })

			live, _ := p.Live(irtest.New().Target())
			defer live.Close()
			live.TrackRows(true)
			if err := live.Draw(); err != nil {
				t.Fatal(err)
			}
			at, _ := live.Index().Locate(0, 0, 2)
			in := live.Input().Drag(mode)
			in.Down(float64(at.X), float64(at.Y))
			in.Up(float64(at.X), float64(at.Y))

			if clicks != 1 {
				t.Errorf("clicks = %d, want 1", clicks)
			}
		})
	}
}
