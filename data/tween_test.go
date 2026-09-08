package data_test

// The keyed join and the blend over it: what a transition is made of, before
// anything is drawn.

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/timzifer/refract/data"
)

func stateA() data.Source {
	return data.NewTable().
		String("id", []string{"a", "b", "c"}).
		Float64("v", []float64{0, 10, 20})
}

func stateB() data.Source {
	// b is gone, d is new, and a and c have moved.
	return data.NewTable().
		String("id", []string{"a", "c", "d"}).
		Float64("v", []float64{100, 120, 999})
}

func TestAlignKeepsFirstAppearanceOrder(t *testing.T) {
	al, err := data.Align(stateA(), stateB(), "id")
	if err != nil {
		t.Fatal(err)
	}
	// a, b, c from the start state, then d, which only the end state has.
	want := []string{"a", "b", "c", "d"}
	if len(al.Keys) != len(want) {
		t.Fatalf("keys = %v, want %v", al.Keys, want)
	}
	for i := range want {
		if al.Keys[i] != want[i] {
			t.Fatalf("keys = %v, want %v", al.Keys, want)
		}
	}
	if got := al.Entered(); len(got) != 1 || got[0] != "d" {
		t.Errorf("entered = %v, want [d]", got)
	}
	if got := al.Exited(); len(got) != 1 || got[0] != "b" {
		t.Errorf("exited = %v, want [b]", got)
	}
}

// ADR 0012's house rule: run it twice and compare. Nothing here may depend on
// how a map felt like enumerating itself.
func TestAlignIsDeterministic(t *testing.T) {
	for range 20 {
		x, err := data.Align(stateA(), stateB(), "id")
		if err != nil {
			t.Fatal(err)
		}
		y, err := data.Align(stateA(), stateB(), "id")
		if err != nil {
			t.Fatal(err)
		}
		for i := range x.Keys {
			if x.Keys[i] != y.Keys[i] || x.A[i] != y.A[i] || x.B[i] != y.B[i] {
				t.Fatalf("two alignments of the same tables differ at %d", i)
			}
		}
	}
}

func TestAlignRefusesAKeyColumnThatIsNotThere(t *testing.T) {
	if _, err := data.Align(stateA(), stateB(), "nope"); !errors.Is(err, data.ErrNoKeyColumn) {
		t.Errorf("err = %v, want ErrNoKeyColumn", err)
	}
	if _, err := data.Align(nil, stateB(), "id"); !errors.Is(err, data.ErrNoKeyColumn) {
		t.Errorf("err = %v, want ErrNoKeyColumn", err)
	}
}

func valueOf(t *testing.T, src data.Source, col string, row int) float64 {
	t.Helper()
	v, ok := src.Float64Column(col)
	if !ok {
		t.Fatalf("no column %q", col)
	}
	return v[row]
}

// The ends of the blend are the states themselves.
func TestTweenEndsAtItsStates(t *testing.T) {
	tw, err := data.NewTween(stateA(), stateB(), "id")
	if err != nil {
		t.Fatal(err)
	}
	src := tw.Source()

	tw.At(0)
	if got := valueOf(t, src, "v", 0); got != 0 {
		t.Errorf("a at f=0 is %v, want 0", got)
	}
	tw.At(1)
	if got := valueOf(t, src, "v", 0); got != 100 {
		t.Errorf("a at f=1 is %v, want 100", got)
	}
	tw.At(0.5)
	if got := valueOf(t, src, "v", 0); got != 50 {
		t.Errorf("a at f=0.5 is %v, want 50", got)
	}
}

// The row set is the union and does not change, which is what keeps the frame
// comparable to the last one.
func TestTweenRowCountIsTheUnionAndIsFixed(t *testing.T) {
	tw, err := data.NewTween(stateA(), stateB(), "id")
	if err != nil {
		t.Fatal(err)
	}
	if tw.Rows() != 4 {
		t.Fatalf("rows = %d, want 4 — a, b, c and d", tw.Rows())
	}
	for _, f := range []float64{0, 0.25, 0.5, 0.75, 1} {
		tw.At(f)
		if got := tw.Source().Len(); got != 4 {
			t.Errorf("at f=%v the blend has %d rows, want 4", f, got)
		}
	}
}

// An entering row holds its end value by default, and grows from EnterFrom
// when the caller says where "nothing yet" is.
func TestEnterFromAndExitTo(t *testing.T) {
	plain, err := data.NewTween(stateA(), stateB(), "id")
	if err != nil {
		t.Fatal(err)
	}
	plain.At(0)
	// d is row 3 of the union; with no EnterFrom it sits at its end value.
	if got := valueOf(t, plain.Source(), "v", 3); got != 999 {
		t.Errorf("an entering row at f=0 is %v, want its end value 999", got)
	}

	grown, err := data.NewTween(stateA(), stateB(), "id",
		data.EnterFrom("v", 0), data.ExitTo("v", 0))
	if err != nil {
		t.Fatal(err)
	}
	grown.At(0)
	if got := valueOf(t, grown.Source(), "v", 3); got != 0 {
		t.Errorf("an entering row at f=0 is %v, want its EnterFrom 0", got)
	}
	grown.At(1)
	if got := valueOf(t, grown.Source(), "v", 3); got != 999 {
		t.Errorf("an entering row at f=1 is %v, want 999", got)
	}
	// b is row 1 and is leaving: it starts where it was and ends at ExitTo.
	grown.At(0)
	if got := valueOf(t, grown.Source(), "v", 1); got != 10 {
		t.Errorf("an exiting row at f=0 is %v, want 10", got)
	}
	grown.At(1)
	if got := valueOf(t, grown.Source(), "v", 1); got != 0 {
		t.Errorf("an exiting row at f=1 is %v, want its ExitTo 0", got)
	}
}

// A string is a name, not a quantity: there is nothing between two of them.
func TestStringColumnsDoNotInterpolate(t *testing.T) {
	a := data.NewTable().
		String("id", []string{"x"}).
		String("stage", []string{"ingest"}).
		Float64("v", []float64{1})
	b := data.NewTable().
		String("id", []string{"x"}).
		String("stage", []string{"serve"}).
		Float64("v", []float64{2})

	tw, err := data.NewTween(a, b, "id")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []float64{0, 0.5, 1} {
		tw.At(f)
		got, ok := tw.Source().StringColumn("stage")
		if !ok {
			t.Fatal("the blend lost its string column")
		}
		if got[0] != "serve" {
			t.Errorf("at f=%v the stage is %q, want the end state's %q", f, got[0], "serve")
		}
	}
}

// A time is its nanoseconds, which is the domain a time axis already maps.
func TestTimeColumnsInterpolate(t *testing.T) {
	t0 := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(10 * time.Hour)
	a := data.NewTable().String("id", []string{"x"}).Time("t", []time.Time{t0})
	b := data.NewTable().String("id", []string{"x"}).Time("t", []time.Time{t1})

	tw, err := data.NewTween(a, b, "id")
	if err != nil {
		t.Fatal(err)
	}
	tw.At(0.5)
	got, ok := tw.Source().TimeColumn("t")
	if !ok {
		t.Fatal("the blend lost its time column")
	}
	if want := t0.Add(5 * time.Hour); !got[0].Equal(want) {
		t.Errorf("halfway is %v, want %v", got[0], want)
	}
}

// A number that is a name has nothing between two of its values either.
func TestHoldSnapsAtHalfway(t *testing.T) {
	a := data.NewTable().String("id", []string{"x"}).Float64("slot", []float64{2})
	b := data.NewTable().String("id", []string{"x"}).Float64("slot", []float64{5})

	tw, err := data.NewTween(a, b, "id", data.Hold("slot"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		f    float64
		want float64
	}{{0, 2}, {0.49, 2}, {0.5, 5}, {1, 5}} {
		tw.At(c.f)
		if got := valueOf(t, tw.Source(), "slot", 0); got != c.want {
			t.Errorf("at f=%v the slot is %v, want %v", c.f, got, c.want)
		}
	}
}

// A transition is built before any frame runs, so a mismatch has somewhere
// honest to go. Snapping to the target from inside a render loop would be a
// silent failure.
func TestNewTweenRefusesAMismatchAtConstruction(t *testing.T) {
	a := data.NewTable().String("id", []string{"x"}).Float64("v", []float64{1})
	textual := data.NewTable().String("id", []string{"x"}).String("v", []string{"one"})

	if _, err := data.NewTween(a, textual, "id"); err == nil {
		t.Error("a column numeric on one side and textual on the other was accepted")
	}
	if _, err := data.NewTween(a, stateB(), "nope"); !errors.Is(err, data.ErrNoKeyColumn) {
		t.Error("a missing key column was accepted")
	}
}

// A column only one state has is carried rather than dropped: it is still a
// fact about those rows, and the other state's rows simply have none.
func TestAColumnOnlyOneStateHasIsCarried(t *testing.T) {
	a := data.NewTable().
		String("id", []string{"x"}).
		Float64("v", []float64{1}).
		Float64("only_a", []float64{7})
	b := data.NewTable().
		String("id", []string{"x"}).
		Float64("v", []float64{2})

	tw, err := data.NewTween(a, b, "id")
	if err != nil {
		t.Fatal(err)
	}
	tw.At(1)
	if got := valueOf(t, tw.Source(), "only_a", 0); got != 7 {
		t.Errorf("only_a = %v, want 7", got)
	}
}

// A key naming two rows means the caller said they are the same thing. The
// first wins, and the transition still runs — refusing to draw a chart that
// draws perfectly well would be the worse answer.
func TestADuplicateKeyTakesItsFirstRow(t *testing.T) {
	a := data.NewTable().
		String("id", []string{"x", "x"}).
		Float64("v", []float64{1, 2})
	b := data.NewTable().
		String("id", []string{"x"}).
		Float64("v", []float64{10})

	tw, err := data.NewTween(a, b, "id")
	if err != nil {
		t.Fatal(err)
	}
	if tw.Rows() != 1 {
		t.Fatalf("rows = %d, want 1", tw.Rows())
	}
	tw.At(0)
	if got := valueOf(t, tw.Source(), "v", 0); got != 1 {
		t.Errorf("v at f=0 is %v, want the first row's 1", got)
	}
}

// The fraction is clamped rather than extrapolated: a caller whose clock ran
// past the end gets the end, not a chart beyond it.
func TestAtClampsTheFraction(t *testing.T) {
	tw, err := data.NewTween(stateA(), stateB(), "id")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []float64{-1, math.NaN()} {
		tw.At(f)
		if got := valueOf(t, tw.Source(), "v", 0); got != 0 {
			t.Errorf("at f=%v the value is %v, want the start's 0", f, got)
		}
	}
	tw.At(2)
	if got := valueOf(t, tw.Source(), "v", 0); got != 100 {
		t.Errorf("past the end the value is %v, want 100", got)
	}
}

// Run it twice, compare. Every stat function has this test and so does this.
func TestAtIsDeterministic(t *testing.T) {
	x, _ := data.NewTween(stateA(), stateB(), "id")
	y, _ := data.NewTween(stateA(), stateB(), "id")
	for _, f := range []float64{0, 0.3, 0.5, 0.77, 1} {
		x.At(f)
		y.At(f)
		xc, _ := x.Source().Float64Column("v")
		yc, _ := y.Source().Float64Column("v")
		for i := range xc {
			if xc[i] != yc[i] {
				t.Fatalf("two blends at f=%v differ at row %d: %v vs %v", f, i, xc[i], yc[i])
			}
		}
	}
}

// The claim the whole design rests on: advancing a transition costs nothing.
// The columns are allocated once and rewritten in place, which is what lets a
// chart animate without a rebuild.
func TestAtDoesNotAllocate(t *testing.T) {
	const n = 10_000
	ids := make([]string, n)
	av := make([]float64, n)
	bv := make([]float64, n)
	for i := range n {
		ids[i] = string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + string(rune('a'+i/676))
		av[i], bv[i] = float64(i), float64(2*i)
	}
	a := data.NewTable().String("id", ids).Float64("v", av)
	b := data.NewTable().String("id", ids).Float64("v", bv)

	tw, err := data.NewTween(a, b, "id")
	if err != nil {
		t.Fatal(err)
	}
	f := 0.0
	if got := testing.AllocsPerRun(200, func() {
		f += 0.001
		tw.At(f)
	}); got != 0 {
		t.Errorf("At allocates %.0f times per call, want 0", got)
	}
}
