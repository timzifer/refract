package refract_test

// Dragging a range along a colourbar: the gesture ADR 0048 named as the
// natural one and did not give.

import (
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

// dragBar presses at one point of a colourbar and releases at another,
// returning the Select the drag fired.
func dragBar(t *testing.T, p *refract.Plot, live *refract.Live, from, to ir.Point) refract.Event {
	t.Helper()
	var got refract.Event
	p.On(refract.Select, func(ev refract.Event) { got = ev })
	in := live.Input()
	if err := in.Down(float64(from.X), float64(from.Y)); err != nil {
		t.Fatal(err)
	}
	if err := in.Move(float64(to.X), float64(to.Y)); err != nil {
		t.Fatal(err)
	}
	if err := in.Up(float64(to.X), float64(to.Y)); err != nil {
		t.Fatal(err)
	}
	return got
}

func continuousBarChart(t *testing.T) (*refract.Plot, *refract.Live) {
	t.Helper()
	p := refract.New(refract.Size(640, 320))
	p.X(scale.Linear(scale.Domain(0, 3)))
	p.Y(scale.Linear(scale.Domain(0, 30)))
	p.Add(geom.Scatter(guideSource(), geom.X("x"), geom.Y("y"),
		geom.ColorBy("v", scale.Sequential(palette.Viridis, scale.ColorDomain(0, 100)))))
	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	return p, live
}

// barExtent is where the colourbar is, found the way a pointer would find it.
func barExtent(t *testing.T, live *refract.Live) (x, top, bottom float32) {
	t.Helper()
	hits := sweep(t, live, refract.Colorbar)
	if len(hits) == 0 {
		t.Fatal("no colourbar")
	}
	a := hits[0].Area
	return hits[0].At.X, a.Min.Y + 1, a.Max.Y - 1
}

// Press at one value, release at another, and the range between them is what
// was meant.
func TestDraggingAContinuousBarSelectsARange(t *testing.T) {
	p, live := continuousBarChart(t)
	x, top, bottom := barExtent(t, live)

	from := ir.Point{X: x, Y: top + (bottom-top)*0.25}
	to := ir.Point{X: x, Y: top + (bottom-top)*0.75}
	ev := dragBar(t, p, live, from, to)

	if ev.Kind != refract.Select {
		t.Fatalf("kind = %v, want select", ev.Kind)
	}
	if ev.Hit.Kind != refract.Colorbar {
		t.Fatalf("the select is about %v, want a colourbar", ev.Hit.Kind)
	}
	if !(ev.Hit.Lo < ev.Hit.Hi) {
		t.Errorf("the range is [%v %v]; it is not ordered", ev.Hit.Lo, ev.Hit.Hi)
	}
	// The bar runs bottom to top over 0..100, so a drag across its middle half
	// is roughly the middle half of the domain.
	if ev.Hit.Lo < 10 || ev.Hit.Lo > 40 || ev.Hit.Hi < 60 || ev.Hit.Hi > 90 {
		t.Errorf("the middle half of the bar reads [%v %v], want about [25 75]", ev.Hit.Lo, ev.Hit.Hi)
	}
	// The band spans the bar: how far sideways the pointer wandered while
	// choosing an interval means nothing.
	if ev.Rect.Min.X != ev.Hit.Area.Min.X || ev.Rect.Max.X != ev.Hit.Area.Max.X {
		t.Errorf("the band spans %v..%v, want the bar %v..%v",
			ev.Rect.Min.X, ev.Rect.Max.X, ev.Hit.Area.Min.X, ev.Hit.Area.Max.X)
	}
	// A range is a statement about values, not about rows.
	if len(ev.Rows) != 0 {
		t.Errorf("the range reported %d rows; which rows fall in it is a question about the data", len(ev.Rows))
	}
}

// Dragged is the feedback while the gesture is in progress, and it is confined
// to the bar however far sideways the pointer strays.
func TestADragOnABarPaintsAlongIt(t *testing.T) {
	_, live := continuousBarChart(t)
	x, top, bottom := barExtent(t, live)

	in := live.Input()
	if err := in.Down(float64(x), float64(top+10)); err != nil {
		t.Fatal(err)
	}
	if err := in.Move(float64(x+40), float64(bottom-10)); err != nil {
		t.Fatal(err)
	}
	band, ok := in.Dragged()
	if !ok {
		t.Fatal("no band while dragging along a colourbar")
	}
	hits := sweep(t, live, refract.Colorbar)
	if band.Min.X != hits[0].Area.Min.X || band.Max.X != hits[0].Area.Max.X {
		t.Errorf("the band spans %v..%v, want the bar %v..%v",
			band.Min.X, band.Max.X, hits[0].Area.Min.X, hits[0].Area.Max.X)
	}
	if !(band.Min.Y < band.Max.Y) {
		t.Errorf("the band is %v..%v tall", band.Min.Y, band.Max.Y)
	}
}

// A drag on a bar is a range whatever the drag mode says: a bar cannot be
// panned and there is no view on it to zoom.
func TestADragOnABarIgnoresTheDragMode(t *testing.T) {
	for _, mode := range []refract.Drag{refract.DragPans, refract.DragSelects, refract.DragZooms} {
		t.Run(mode.String(), func(t *testing.T) {
			p, live := continuousBarChart(t)
			x, top, bottom := barExtent(t, live)
			before, beforeHi := live.Index().Panels()[0].X.Domain()

			var got refract.Event
			p.On(refract.Select, func(ev refract.Event) { got = ev })
			in := live.Input().Drag(mode)
			if err := in.Down(float64(x), float64(top+5)); err != nil {
				t.Fatal(err)
			}
			if err := in.Move(float64(x), float64(bottom-5)); err != nil {
				t.Fatal(err)
			}
			if err := in.Up(float64(x), float64(bottom-5)); err != nil {
				t.Fatal(err)
			}

			if got.Hit.Kind != refract.Colorbar {
				t.Errorf("the drag fired %v, want a colourbar range", got.Hit.Kind)
			}
			if after, afterHi := live.Index().Panels()[0].X.Domain(); after != before || afterHi != beforeHi {
				t.Errorf("a drag on the colourbar moved the panel view to [%v %v]", after, afterHi)
			}
		})
	}
}

// Across bands the range is the union: a reader dragging over three bands
// means all three, not the two boundaries they crossed.
func TestDraggingAcrossClassedBandsSelectsTheirUnion(t *testing.T) {
	p := refract.New(refract.Size(640, 320))
	p.X(scale.Linear(scale.Domain(0, 3)))
	p.Y(scale.Linear(scale.Domain(0, 30)))
	p.Add(geom.Scatter(guideSource(), geom.X("x"), geom.Y("y"),
		geom.ColorBy("v", scale.Threshold(palette.Viridis, []float64{25, 75},
			scale.ColorDomain(0, 100)))))
	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	byClass := map[int]refract.Hit{}
	for _, h := range sweep(t, live, refract.Colorbar) {
		byClass[h.Class] = h
	}
	lowest, highest := byClass[0], byClass[2]
	if lowest.Class != 0 || highest.Class != 2 {
		t.Fatal("the bar does not have the three bands this test needs")
	}

	ev := dragBar(t, p, live, highest.At, lowest.At)
	if ev.Hit.Lo != 0 || ev.Hit.Hi != 100 {
		t.Errorf("a drag over all three bands selected [%v %v], want [0 100]", ev.Hit.Lo, ev.Hit.Hi)
	}
	if ev.Hit.Class != -1 {
		t.Errorf("class = %d, want -1 — the range spans more than one band", ev.Hit.Class)
	}
}

// A drag that stays inside one band keeps that band's identity.
func TestDraggingWithinOneBandKeepsIt(t *testing.T) {
	p := refract.New(refract.Size(640, 320))
	p.X(scale.Linear(scale.Domain(0, 3)))
	p.Y(scale.Linear(scale.Domain(0, 30)))
	p.Add(geom.Scatter(guideSource(), geom.X("x"), geom.Y("y"),
		geom.ColorBy("v", scale.Threshold(palette.Viridis, []float64{25, 75},
			scale.ColorDomain(0, 100)))))
	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	var band refract.Hit
	for _, h := range sweep(t, live, refract.Colorbar) {
		if h.Class == 0 {
			band = h
		}
	}
	if band.Class != 0 {
		t.Fatal("no first band")
	}
	// Down at the middle of the band, up a couple of pixels away but still in
	// it — far enough to be a drag rather than a click.
	from := band.At
	to := ir.Point{X: band.At.X, Y: band.At.Y + 6}
	ev := dragBar(t, p, live, from, to)

	if ev.Hit.Class != 0 {
		t.Errorf("a drag within one band reports class %d, want 0", ev.Hit.Class)
	}
	if ev.Hit.Lo != 0 || ev.Hit.Hi != 25 {
		t.Errorf("a drag within the first band selected [%v %v], want [0 25]", ev.Hit.Lo, ev.Hit.Hi)
	}
}

// A press and release without moving is still a click, not a zero-width range.
func TestAClickOnABarIsStillAClick(t *testing.T) {
	p, live := continuousBarChart(t)
	x, top, _ := barExtent(t, live)

	var clicks, selects int
	p.On(refract.Click, func(refract.Event) { clicks++ })
	p.On(refract.Select, func(refract.Event) { selects++ })

	at := ir.Point{X: x, Y: top + 20}
	in := live.Input()
	if err := in.Down(float64(at.X), float64(at.Y)); err != nil {
		t.Fatal(err)
	}
	if err := in.Up(float64(at.X), float64(at.Y)); err != nil {
		t.Fatal(err)
	}
	if clicks != 1 || selects != 0 {
		t.Errorf("clicks = %d selects = %d, want 1 and 0", clicks, selects)
	}
}

// A drag that begins on the panel is unaffected: it pans, as it always did.
func TestADragOnThePanelStillPans(t *testing.T) {
	_, live := continuousBarChart(t)
	area := live.Index().Panels()[0].Area
	before, _ := live.Index().Panels()[0].X.Domain()

	in := live.Input()
	mid := ir.Point{X: (area.Min.X + area.Max.X) / 2, Y: (area.Min.Y + area.Max.Y) / 2}
	if err := in.Down(float64(mid.X), float64(mid.Y)); err != nil {
		t.Fatal(err)
	}
	if err := in.Move(float64(mid.X+60), float64(mid.Y)); err != nil {
		t.Fatal(err)
	}
	if err := in.Up(float64(mid.X+60), float64(mid.Y)); err != nil {
		t.Fatal(err)
	}
	if after, _ := live.Index().Panels()[0].X.Domain(); after == before {
		t.Error("a drag on the panel no longer pans")
	}
}
