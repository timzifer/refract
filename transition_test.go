package refract_test

// Transitions: the ends, the middle, the determinism, and the property that
// makes the whole thing affordable.

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/scale"
)

func barsA() data.Source {
	return refract.NewTable().
		String("id", []string{"a", "b", "c"}).
		Float64("x", []float64{0, 1, 2}).
		Float64("y", []float64{10, 20, 30})
}

func barsB() data.Source {
	// b leaves, d arrives, a and c move.
	return refract.NewTable().
		String("id", []string{"a", "c", "d"}).
		Float64("x", []float64{0, 2, 3}).
		Float64("y", []float64{40, 15, 25})
}

// tweened builds a chart over a blend, the way a caller must: the layer is
// built over Tween.Source once, and what changes afterwards is what that
// source says.
func tweened(t *testing.T, opts ...data.TweenOption) (*refract.Live, *data.Tween, *irtest.Recorder) {
	t.Helper()
	tw, err := data.NewTween(barsA(), barsB(), "id", opts...)
	if err != nil {
		t.Fatal(err)
	}
	p := refract.New(refract.Size(600, 300))
	p.X(scale.Linear(scale.Domain(-1, 4)))
	p.Y(scale.Linear(scale.Domain(0, 50)))
	p.Add(geom.Bar(tw.Source(), geom.X("x"), geom.Y("y")))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	return live, tw, rec
}

// plain draws one state on its own, for comparing an end of a transition
// against the thing it is supposed to be.
func plain(t *testing.T, src data.Source) []string {
	t.Helper()
	p := refract.New(refract.Size(600, 300))
	p.X(scale.Linear(scale.Domain(-1, 4)))
	p.Y(scale.Linear(scale.Domain(0, 50)))
	p.Add(geom.Bar(src, geom.X("x"), geom.Y("y")))
	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	return rec.Trace()
}

func frameAt(t *testing.T, live *refract.Live, rec *irtest.Recorder, tr *refract.Transition, f float64) []string {
	t.Helper()
	rec.Reset()
	if err := tr.At(f); err != nil {
		t.Fatal(err)
	}
	return rec.Trace()
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The ends of a transition are the states themselves. A transition that drew
// something else at f=1 would be an animation to the wrong place.
func TestATransitionEndsAtItsStates(t *testing.T) {
	live, tw, rec := tweened(t)
	tr, err := live.Transition(tw)
	if err != nil {
		t.Fatal(err)
	}
	tr.Ease(refract.EaseLinear)

	// The union's row set means the ends are not literally the two tables: at
	// f=0 the entering row is already there, holding its end value. So the
	// comparison is against the blend's own ends, which is what the chart is
	// actually animating between.
	tw.At(0)
	wantStart := plain(t, snapshot(t, tw))
	tw.At(1)
	wantEnd := plain(t, snapshot(t, tw))

	if got := frameAt(t, live, rec, tr, 0); !equal(got, wantStart) {
		t.Errorf("the frame at f=0 is not the start state\ngot  %v\nwant %v", got, wantStart)
	}
	if got := frameAt(t, live, rec, tr, 1); !equal(got, wantEnd) {
		t.Errorf("the frame at f=1 is not the end state\ngot  %v\nwant %v", got, wantEnd)
	}
}

// snapshot copies a blend's current contents into a table of their own, so it
// can be drawn beside the blend rather than through it.
func snapshot(t *testing.T, tw *data.Tween) data.Source {
	t.Helper()
	src := tw.Source()
	tbl := refract.NewTable()
	for _, name := range src.Columns() {
		if v, ok := src.Float64Column(name); ok {
			tbl = tbl.Float64(name, append([]float64(nil), v...))
			continue
		}
		if v, ok := src.StringColumn(name); ok {
			tbl = tbl.String(name, append([]string(nil), v...))
		}
	}
	return tbl
}

// The claim the whole design rests on: two frames of a transition are the same
// chart with different numbers in it, so ir.Damage can compare them call for
// call and repaint only what moved.
//
// If this ever goes false the animation silently becomes a full repaint every
// frame — and nothing about the picture looks wrong, which is why it is a test
// rather than something anyone would notice.
func TestATransitionKeepsItsFramesComparable(t *testing.T) {
	live, tw, rec := tweened(t, data.EnterFrom("y", 0), data.ExitTo("y", 0))
	tr, err := live.Transition(tw)
	if err != nil {
		t.Fatal(err)
	}
	tr.Ease(refract.EaseLinear)

	// The first frame has nothing to compare against and is a full repaint by
	// definition. Every frame after it must be a partial one.
	if err := tr.At(0); err != nil {
		t.Fatal(err)
	}
	rec.Whole = nil
	for i := 1; i <= 20; i++ {
		if err := tr.At(float64(i) / 20); err != nil {
			t.Fatal(err)
		}
	}
	if len(rec.Whole) == 0 {
		t.Fatal("no frame reported what it repainted")
	}
	for i, whole := range rec.Whole {
		if whole {
			t.Fatalf("frame %d of the transition repainted the whole canvas: ir.Damage found the two frames not comparable, so every frame of this animation is a full repaint", i+1)
		}
	}
}

// The same claim from the other side: the row set is the union and is fixed,
// so the call count never changes mid-transition. A structural change is what
// makes two frames incomparable, and an entering row appearing partway through
// is the way it would happen.
func TestATransitionEmitsTheSameCallsThroughout(t *testing.T) {
	live, tw, rec := tweened(t, data.EnterFrom("y", 0), data.ExitTo("y", 0))
	tr, err := live.Transition(tw)
	if err != nil {
		t.Fatal(err)
	}
	tr.Ease(refract.EaseLinear)

	var prev []string
	for i := range 21 {
		got := frameAt(t, live, rec, tr, float64(i)/20)
		if prev != nil && len(got) != len(prev) {
			t.Fatalf("frame %d emits %d calls where the last emitted %d — the structure changed mid-transition", i, len(got), len(prev))
		}
		prev = got
	}
}

// Run it twice, compare. A transition is a pure function of its fraction,
// which is what lets a golden file be taken of one.
func TestATransitionIsDeterministic(t *testing.T) {
	for _, f := range []float64{0, 0.25, 0.5, 0.75, 1} {
		liveA, twA, recA := tweened(t)
		trA, _ := liveA.Transition(twA)
		liveB, twB, recB := tweened(t)
		trB, _ := liveB.Transition(twB)

		x := frameAt(t, liveA, recA, trA, f)
		y := frameAt(t, liveB, recB, trB, f)
		if !equal(x, y) {
			t.Errorf("two transitions at f=%v drew different frames", f)
		}
	}
}

// The middle is between the ends. A blend that jumped would pass every
// end-state assertion above.
func TestTheMiddleIsBetweenTheEnds(t *testing.T) {
	tw, err := data.NewTween(barsA(), barsB(), "id")
	if err != nil {
		t.Fatal(err)
	}
	tw.At(0.5)
	y, ok := tw.Source().Float64Column("y")
	if !ok {
		t.Fatal("no y column")
	}
	// Row 0 is "a": 10 at the start, 40 at the end.
	if y[0] != 25 {
		t.Errorf("halfway between 10 and 40 is %v, want 25", y[0])
	}
}

// The host owns the clock: refract never reads one, so a synthetic time drives
// a transition exactly as a real one would.
func TestAdvanceRunsOnTheHostsClock(t *testing.T) {
	live, tw, _ := tweened(t)
	tr, err := live.Transition(tw)
	if err != nil {
		t.Fatal(err)
	}
	tr.Ease(refract.EaseLinear).Over(time.Second)

	base := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	running, err := tr.Advance(base)
	if err != nil {
		t.Fatal(err)
	}
	if !running || tr.Fraction() != 0 {
		t.Errorf("the first frame is at %v (running=%v), want 0 and running", tr.Fraction(), running)
	}

	if _, err := tr.Advance(base.Add(500 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if tr.Fraction() != 0.5 {
		t.Errorf("halfway through a second is %v, want 0.5", tr.Fraction())
	}

	running, err = tr.Advance(base.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if running {
		t.Error("the frame that reaches the end still reports running")
	}
	if !tr.Done() {
		t.Error("the transition is not done at its duration")
	}

	// And it stays done, so a host asking for another frame stops asking once.
	if running, _ := tr.Advance(base.Add(2 * time.Second)); running {
		t.Error("a finished transition reports running")
	}
}

// Past the end is the end, not beyond it.
func TestAtClampsAndFinishJumps(t *testing.T) {
	live, tw, _ := tweened(t)
	tr, _ := live.Transition(tw)

	if err := tr.At(-1); err != nil {
		t.Fatal(err)
	}
	if tr.Fraction() != 0 || tr.Done() {
		t.Errorf("f=-1 gives %v (done=%v), want 0 and not done", tr.Fraction(), tr.Done())
	}
	if err := tr.At(2); err != nil {
		t.Fatal(err)
	}
	if tr.Fraction() != 1 || !tr.Done() {
		t.Errorf("f=2 gives %v (done=%v), want 1 and done", tr.Fraction(), tr.Done())
	}

	live2, tw2, _ := tweened(t)
	tr2, _ := live2.Transition(tw2)
	if err := tr2.Finish(); err != nil {
		t.Fatal(err)
	}
	if !tr2.Done() {
		t.Error("Finish did not finish")
	}
}

// The axes stay where they are unless asked, because an axis that rescales
// every frame is one a reader cannot compare two frames of.
func TestTheAxesStayPutByDefault(t *testing.T) {
	live, tw, _ := tweened(t)
	tr, _ := live.Transition(tw)
	if err := tr.At(0); err != nil {
		t.Fatal(err)
	}
	before, _ := live.Index().Panels()[0].Y.Domain()

	for _, f := range []float64{0.25, 0.5, 0.75, 1} {
		if err := tr.At(f); err != nil {
			t.Fatal(err)
		}
		if got, _ := live.Index().Panels()[0].Y.Domain(); got != before {
			t.Fatalf("at f=%v the axis moved to %v from %v", f, got, before)
		}
	}
}

// With Rescale on they slide between the two ends rather than jumping.
func TestRescaleSlidesTheAxes(t *testing.T) {
	tw, err := data.NewTween(barsA(), barsB(), "id")
	if err != nil {
		t.Fatal(err)
	}
	// Free axes, so there is something for a rescale to establish.
	p := refract.New(refract.Size(600, 300))
	p.X(scale.Linear())
	p.Y(scale.Linear())
	p.Add(geom.Bar(tw.Source(), geom.X("x"), geom.Y("y")))
	live, err := p.Live(irtest.New().Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}

	tr, err := live.Transition(tw)
	if err != nil {
		t.Fatal(err)
	}
	tr.Ease(refract.EaseLinear).Rescale(true)

	if err := tr.At(0); err != nil {
		t.Fatal(err)
	}
	_, startHi := live.Index().Panels()[0].Y.Domain()
	if err := tr.At(1); err != nil {
		t.Fatal(err)
	}
	_, endHi := live.Index().Panels()[0].Y.Domain()
	if startHi == endHi {
		t.Skip("the two states share a domain, so this chart has nothing to slide")
	}

	if err := tr.At(0.5); err != nil {
		t.Fatal(err)
	}
	_, midHi := live.Index().Panels()[0].Y.Domain()
	lo, hi := min(startHi, endHi), max(startHi, endHi)
	if midHi <= lo || midHi >= hi {
		t.Errorf("halfway the axis reaches %v, want strictly between %v and %v", midHi, lo, hi)
	}
}

func TestATransitionNeedsATween(t *testing.T) {
	live, _, _ := tweened(t)
	if _, err := live.Transition(); err != refract.ErrNoTweens {
		t.Errorf("err = %v, want ErrNoTweens", err)
	}
	if _, err := live.Transition(nil); err != refract.ErrNoTweens {
		t.Errorf("a nil tween gave %v, want ErrNoTweens", err)
	}
}

func TestEasingCurvesRunFromZeroToOne(t *testing.T) {
	for name, e := range map[string]refract.Easing{
		"linear": refract.EaseLinear,
		"in":     refract.EaseIn,
		"out":    refract.EaseOut,
		"inout":  refract.EaseInOut,
	} {
		if got := e(0); got != 0 {
			t.Errorf("%s(0) = %v, want 0", name, got)
		}
		if got := e(1); got != 1 {
			t.Errorf("%s(1) = %v, want 1", name, got)
		}
		// Monotonic, or the chart would move backwards partway through.
		prev := -1.0
		for i := range 101 {
			v := e(float64(i) / 100)
			if v < prev {
				t.Errorf("%s goes backwards at %v", name, float64(i)/100)
				break
			}
			prev = v
		}
	}
}

// The pictures. A transition is a pure function of its fraction, which is what
// makes a golden file of one meaningful: t50 is a real frame of a real
// animation, not an approximation of one.
//
// The entering bar grows out of the baseline and the leaving one sinks back
// into it, which is what EnterFrom and ExitTo are for and is the thing to look
// at when reading the diff.
func TestTransitionGoldens(t *testing.T) {
	tw, err := data.NewTween(barsA(), barsB(), "id",
		data.EnterFrom("y", 0), data.ExitTo("y", 0))
	if err != nil {
		t.Fatal(err)
	}
	p := refract.New(
		refract.Size(600, 300),
		refract.Title("Halfway"),
		refract.XTitle("x"),
		refract.YTitle("y"),
	)
	p.X(scale.Linear(scale.Domain(-1, 4)))
	p.Y(scale.Linear(scale.Domain(0, 50)))
	p.Add(geom.Bar(tw.Source(), geom.X("x"), geom.Y("y")))

	for _, c := range []struct {
		name string
		f    float64
	}{
		{"transition-t0", 0},
		{"transition-t50", 0.5},
		{"transition-t100", 1},
	} {
		tw.At(c.f)
		golden(t, c.name, p)
	}
}

// benchmarkTransitionFrame measures a frame of an animation over n rows.
//
// The claim is the one the whole design rests on: advancing a transition is a
// blend written into buffers that already exist, so a frame of an animation
// costs a frame and nothing per row. A Tween that rebuilt its columns, or a
// chart that had to be resolved again between frames, would show up here as
// thousands of allocations and nowhere else — the picture would be right and
// the animation would merely be slow.
func benchmarkTransitionFrame(b *testing.B, n int) {
	onOnePGate(b)

	ids := make([]string, n)
	x := make([]float64, n)
	ya := make([]float64, n)
	yb := make([]float64, n)
	for i := range n {
		ids[i] = strconv.Itoa(i)
		x[i] = float64(i)
		ya[i] = math.Sin(float64(i) / 500)
		yb[i] = math.Cos(float64(i) / 500)
	}
	a := refract.NewTable().String("id", ids).Float64("x", x).Float64("y", ya)
	end := refract.NewTable().String("id", ids).Float64("x", x).Float64("y", yb)

	tw, err := data.NewTween(a, end, "id")
	if err != nil {
		b.Fatal(err)
	}

	p := refract.New(refract.Size(800, 400))
	p.X(scale.Linear(scale.Domain(0, float64(n))))
	p.Y(scale.Linear(scale.Domain(-1, 1)))
	p.Add(geom.Line(tw.Source(), geom.X("x"), geom.Y("y")))

	live, err := p.Live(irtest.NullTarget())
	if err != nil {
		b.Fatal(err)
	}
	defer live.Close()

	tr, err := live.Transition(tw)
	if err != nil {
		b.Fatal(err)
	}
	if err := tr.At(0); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// A fraction that never repeats, so every frame is a frame rather
		// than a skip.
		if err := tr.At(float64(i%1000) / 1000); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransitionFrame1k(b *testing.B)   { benchmarkTransitionFrame(b, 1_000) }
func BenchmarkTransitionFrame100k(b *testing.B) { benchmarkTransitionFrame(b, 100_000) }

// textLine is the trace of the Text call carrying want, which is where that
// label's position is written down. A chart's other Text calls are its tick
// labels and its titles, and they do not move.
func textLine(t *testing.T, rec *irtest.Recorder, want string) string {
	t.Helper()
	for _, line := range rec.Trace() {
		if strings.HasPrefix(line, "Text "+strconv.Quote(want)+" ") {
			return line
		}
	}
	t.Fatalf("no label reading %q was drawn", want)
	return ""
}

// Text, both ways round.
//
// A label bound to a numeric column counts, because a text layer re-spells its
// column from the blend on every frame. A label bound to a string column
// snaps, because there is nothing between two names. Both are consequences of
// one rule and both are worth pinning: the first is the animation people mean
// by "animated text", and the second is the limitation.
func TestALabelOverANumberCounts(t *testing.T) {
	a := refract.NewTable().String("id", []string{"a"}).
		Float64("x", []float64{1}).Float64("y", []float64{1}).
		Float64("n", []float64{0})
	b := refract.NewTable().String("id", []string{"a"}).
		Float64("x", []float64{1}).Float64("y", []float64{1}).
		Float64("n", []float64{100})

	tw, err := data.NewTween(a, b, "id", data.Round("n", 0))
	if err != nil {
		t.Fatal(err)
	}
	p := refract.New(refract.Size(400, 300))
	p.X(scale.Linear(scale.Domain(0, 2)))
	p.Y(scale.Linear(scale.Domain(0, 2)))
	p.Add(geom.Text(tw.Source(), geom.X("x"), geom.Y("y"), geom.TextBy("n")))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()

	want := map[float64]string{0: "0", 1.0 / 3: "33", 0.5: "50", 1: "100"}
	for _, f := range []float64{0, 1.0 / 3, 0.5, 1} {
		tw.At(f)
		rec.Reset()
		if err := live.Draw(); err != nil {
			t.Fatal(err)
		}
		texts := rec.Texts()
		if len(texts) == 0 {
			t.Fatalf("at f=%v nothing was drawn", f)
		}
		// The layer's label is the last text drawn; the rest are tick labels.
		if got := texts[len(texts)-1]; got != want[f] {
			t.Errorf("at f=%v the label reads %q, want %q", f, got, want[f])
		}
	}
}

// Without Round the same label reads its full float, which is arithmetic
// rather than a number. This is the reason Round exists, so it is worth a test
// that fails if the formatting ever starts rounding on its own.
func TestAnUnroundedCountingLabelShowsItsFloat(t *testing.T) {
	a := refract.NewTable().String("id", []string{"a"}).
		Float64("x", []float64{1}).Float64("y", []float64{1}).
		Float64("n", []float64{0})
	b := refract.NewTable().String("id", []string{"a"}).
		Float64("x", []float64{1}).Float64("y", []float64{1}).
		Float64("n", []float64{100})

	tw, err := data.NewTween(a, b, "id")
	if err != nil {
		t.Fatal(err)
	}
	p := refract.New(refract.Size(400, 300))
	p.X(scale.Linear(scale.Domain(0, 2)))
	p.Y(scale.Linear(scale.Domain(0, 2)))
	p.Add(geom.Text(tw.Source(), geom.X("x"), geom.Y("y"), geom.TextBy("n")))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()

	tw.At(1.0 / 3)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	texts := rec.Texts()
	if got := texts[len(texts)-1]; got == "33" {
		t.Error("the label rounded itself, so data.Round has nothing to do")
	}
}

// A label that is a string changes once, at the moment its column does. There
// is no cross-fade and no character-level morph — see data.Tween's doc for why.
func TestALabelOverAStringSnaps(t *testing.T) {
	a := refract.NewTable().String("id", []string{"a"}).
		Float64("x", []float64{1}).Float64("y", []float64{1}).
		String("s", []string{"start"})
	b := refract.NewTable().String("id", []string{"a"}).
		Float64("x", []float64{1}).Float64("y", []float64{1}).
		String("s", []string{"end"})

	tw, err := data.NewTween(a, b, "id")
	if err != nil {
		t.Fatal(err)
	}
	p := refract.New(refract.Size(400, 300))
	p.X(scale.Linear(scale.Domain(0, 2)))
	p.Y(scale.Linear(scale.Domain(0, 2)))
	p.Add(geom.Text(tw.Source(), geom.X("x"), geom.Y("y"), geom.TextBy("s")))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()

	// The first frame establishes the label.
	tw.At(0)
	if err := live.Draw(); err != nil {
		t.Fatal(err)
	}
	texts := rec.Texts()
	if got := texts[len(texts)-1]; got != "end" {
		t.Errorf("the label reads %q, want the end state's %q — a string has no halfway", got, "end")
	}

	// Every frame after it is byte-identical, so Draw paints nothing at all.
	// That is the snap, stated as strongly as it can be: there is no
	// intermediate frame because there is nothing to draw between two names.
	for _, f := range []float64{0.25, 0.5, 0.75, 1} {
		tw.At(f)
		rec.Reset()
		if err := live.Draw(); err != nil {
			t.Fatal(err)
		}
		if n := len(rec.Trace()); n != 0 {
			t.Errorf("at f=%v the chart repainted %d calls, want none — the label snapped and nothing else moved", f, n)
		}
	}
}

// The position moves even though the text does not, which is the thing that is
// actually available for a label whose words change.
func TestALabelMovesWhileItsTextSnaps(t *testing.T) {
	a := refract.NewTable().String("id", []string{"a"}).
		Float64("x", []float64{0}).Float64("y", []float64{1}).
		String("s", []string{"start"})
	b := refract.NewTable().String("id", []string{"a"}).
		Float64("x", []float64{2}).Float64("y", []float64{1}).
		String("s", []string{"end"})

	tw, err := data.NewTween(a, b, "id")
	if err != nil {
		t.Fatal(err)
	}
	p := refract.New(refract.Size(400, 300))
	p.X(scale.Linear(scale.Domain(0, 2)))
	p.Y(scale.Linear(scale.Domain(0, 2)))
	p.Add(geom.Text(tw.Source(), geom.X("x"), geom.Y("y"), geom.TextBy("s")))

	rec := irtest.New()
	live, err := p.Live(rec.Target())
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()

	var seen []string
	for _, f := range []float64{0, 0.5, 1} {
		tw.At(f)
		rec.Reset()
		if err := live.Draw(); err != nil {
			t.Fatal(err)
		}
		seen = append(seen, textLine(t, rec, "end"))
	}
	if seen[0] == seen[1] || seen[1] == seen[2] {
		t.Errorf("the label did not move between frames:\n%v", seen)
	}
}
