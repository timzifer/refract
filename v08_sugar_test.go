package refract_test

// The v0.8 sugar, end to end. A donut is a recipe with a name; a slice's inner
// and outer radius are columns of the table like its share is; and a slice can
// be broken out of the ring without becoming a different mark.

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/interact"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// rings is the table the donuts below are drawn from: one row per slice, with
// the share that becomes the angle and the two radii that become its ends.
func rings() refract.Source {
	return refract.NewTable().
		Float64("share", []float64{45, 25, 20, 10}).
		Float64("floor", []float64{0.35, 0.35, 0.35, 0.35}).
		Float64("reach", []float64{1, 0.7, 0.55, 0.9}).
		Float64("pull", []float64{0, 0, 0, 0.15}).
		String("browser", []string{"chrome", "safari", "firefox", "edge"})
}

// donut is the three-dimensional donut: the share goes round, and each slice
// starts and stops where its own row says.
func donut(opts ...geom.Option) *refract.Plot {
	p := refract.New(refract.Size(400, 400), refract.Theme(bare()),
		refract.Coord(coord.Pie(coord.Radius(1))), refract.Legend(false))
	p.X(scale.Linear(scale.Domain(0, 1)))
	p.Y(scale.Linear())
	p.Add(geom.Bar(rings(), append([]geom.Option{
		geom.X("floor"), geom.X2("reach"), geom.Y("share"), geom.GroupBy("browser"),
	}, opts...)...))
	return p
}

// The sugar draws the recipe and nothing else. If it drew anything else it
// would be a second implementation of a pie, which is the thing the coordinate
// stage exists to avoid.
func TestADonutIsTheRecipeItNames(t *testing.T) {
	draw := func(c coord.Coord) []string {
		t.Helper()
		p := refract.New(refract.Size(400, 400), refract.Theme(bare()),
			refract.Coord(c), refract.Legend(false))
		p.X(scale.Linear())
		p.Y(scale.Linear())
		p.Add(geom.Bar(browsers(), geom.X("all"), geom.Y("share"), geom.GroupBy("browser")))
		rec := irtest.New()
		if err := p.Render(rec.Target()); err != nil {
			t.Fatal(err)
		}
		return rec.Trace()
	}
	sugar := draw(coord.Donut(0.45))
	long := draw(coord.Polar(coord.Theta(coord.FromY), coord.Hole(0.45)))
	if strings.Join(sugar, "\n") != strings.Join(long, "\n") {
		t.Error("coord.Donut(0.45) and the polar coord it is sugar for drew different charts")
	}
}

// A slice reaches as far as its row says and starts where its row says, so a
// donut carries three dimensions: how far round a slice goes, where it begins
// and where it ends. Nothing about the layer changed to make that true — the
// second radius is a column, exactly as the first one is.
func TestASlicesRadiiAreDimensionsOfTheData(t *testing.T) {
	rec := irtest.New()
	live, err := donut().Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	live.TrackRows(true)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	ix := live.Index()
	pn := ix.Panels()[0]

	// The middle of each slice's arc, at a radius the row does reach and at one
	// it does not.
	reach := []float64{1, 0.7, 0.55, 0.9}
	for row, mid := range midAngles() {
		inside := pn.Coord.Point(pn.X.Map(reach[row]-0.1), pn.Y.Map(mid))
		if h, ok := ix.At(inside, 2); !ok || h.Row != row {
			t.Errorf("row %d: inside its own reach the pointer found %v", row, hitOf(h, ok))
		}
		// Well past it, rather than just past it: a filled arc is hit-tested
		// against the control polygon of its cubics, which bulges a little way
		// outside the ink — see AGENTS.md. A slice that stops at 0.7 is not
		// pointable at 0.9.
		if reach[row]+0.2 > 1 {
			continue
		}
		beyond := pn.Coord.Point(pn.X.Map(reach[row]+0.2), pn.Y.Map(mid))
		if h, ok := ix.At(beyond, 2); ok {
			t.Errorf("row %d: past its reach the pointer found row %d", row, h.Row)
		}
	}
}

// A broken-out slice is the same slice somewhere else: it leaves the ring, and
// it is still the row it was — a pointer follows it out.
func TestABrokenOutSliceLeavesTheRingAndKeepsItsRow(t *testing.T) {
	probe := func(p *refract.Plot) (*interact.Index, func()) {
		t.Helper()
		rec := irtest.New()
		live, err := p.Live(rec.Target())
		if err != nil {
			t.Fatal(err)
		}
		live.TrackRows(true)
		if err := live.Draw(); err != nil {
			t.Fatal(err)
		}
		return live.Index(), func() { live.Close() }
	}

	still, done := probe(donut())
	defer done()
	moved, done2 := probe(donut(geom.ExplodeBy("pull")))
	defer done2()

	pn := still.Panels()[0]
	// The fourth slice is the one the column pulls out, by 0.15 of the outer
	// radius along its own bisector.
	const row = 3
	mid := midAngles()[row]
	r1 := float64(min32(pn.Area.Dx(), pn.Area.Dy())) / 2
	angle := float64(pn.Y.Map(mid)) // the mapped angle is the canvas angle: the coord starts at noon
	s, c := math.Sincos(angle)
	d := ir.Point{X: float32(0.15 * r1 * s), Y: float32(-0.15 * r1 * c)}

	// Just inside the slice's own inner radius: in the ring, that is the
	// slice; once the slice has left, there is nothing there at all.
	at := pn.Coord.Point(pn.X.Map(0.4), pn.Y.Map(mid))
	if h, ok := still.At(at, 2); !ok || h.Row != row {
		t.Fatalf("in the ring the pointer found %v, want row %d", hitOf(h, ok), row)
	}
	if h, ok := moved.At(at, 2); ok {
		t.Errorf("the slice left the ring but row %d is still in it", h.Row)
	}
	// And where it went, it is still itself.
	if h, ok := moved.At(ir.Point{X: at.X + d.X, Y: at.Y + d.Y}, 2); !ok || h.Row != row {
		t.Errorf("where the slice went the pointer found %v, want row %d", hitOf(h, ok), row)
	}
}

// midAngles is the middle of each slice's arc in the Y scale's own units: the
// running total of the shares, less half of each.
func midAngles() []float64 {
	shares := []float64{45, 25, 20, 10}
	out := make([]float64, len(shares))
	sum := 0.0
	for i, v := range shares {
		out[i] = sum + v/2
		sum += v
	}
	return out
}

// hitOf names what a probe found, for a message that says so.
func hitOf(h interact.Hit, ok bool) string {
	if !ok {
		return "nothing"
	}
	return fmt.Sprintf("row %d", h.Row)
}

func min32(a, b float32) float32 {
	if b < a {
		return b
	}
	return a
}
