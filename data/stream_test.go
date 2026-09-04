package data_test

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/timzifer/refract/data"
)

func TestAStreamIsEmptyUntilItIsSnapshotted(t *testing.T) {
	s := data.NewStream("t", "y")
	src := s.Source()
	if got := src.Len(); got != 0 {
		t.Errorf("a stream nobody has snapshotted has %d rows", got)
	}

	for i := range 3 {
		if err := s.Append(float64(i), float64(i*i)); err != nil {
			t.Fatal(err)
		}
	}
	if got := src.Len(); got != 0 {
		t.Errorf("the view moved without a snapshot: %d rows", got)
	}
	if got := s.Len(); got != 3 {
		t.Errorf("the stream holds %d rows, want 3", got)
	}

	s.Snapshot()
	if got := src.Len(); got != 3 {
		t.Fatalf("after a snapshot the view has %d rows, want 3", got)
	}
	y, ok := src.Float64Column("y")
	if !ok {
		t.Fatal("no y column")
	}
	if want := []float64{0, 1, 4}; !equal(y, want) {
		t.Errorf("y = %v, want %v", y, want)
	}
}

func TestASnapshotDoesNotSeeLaterAppends(t *testing.T) {
	s := data.NewStream("y")
	s.Append(1)
	snap := s.Snapshot()

	s.Append(2)
	s.Append(3)
	if got := snap.Len(); got != 1 {
		t.Errorf("the snapshot grew to %d rows while the producer appended", got)
	}
	y, _ := snap.Float64Column("y")
	if len(y) != 1 || y[0] != 1 {
		t.Errorf("the snapshot's rows changed: %v", y)
	}
}

func TestAWindowKeepsTheLastRows(t *testing.T) {
	s := data.NewStream("i").Window(4)
	for i := range 10 {
		s.Append(float64(i))
	}
	if got := s.Len(); got != 4 {
		t.Errorf("the stream holds %d rows, want the window's 4", got)
	}
	s.Snapshot()
	col, _ := s.Source().Float64Column("i")
	if want := []float64{6, 7, 8, 9}; !equal(col, want) {
		t.Errorf("rows = %v, want %v — the window keeps the newest and in order", col, want)
	}
}

func TestAWindowSetAfterwardsDropsTheOldest(t *testing.T) {
	s := data.NewStream("i", "j")
	for i := range 10 {
		s.Append(float64(i), float64(-i))
	}
	s.Window(3)
	s.Snapshot()
	src := s.Source()
	i, _ := src.Float64Column("i")
	j, _ := src.Float64Column("j")
	if want := []float64{7, 8, 9}; !equal(i, want) {
		t.Errorf("i = %v, want %v", i, want)
	}
	// The second column must be cut the same way as the first: unwrapping one
	// column at a time is exactly where a ring buffer goes wrong.
	if want := []float64{-7, -8, -9}; !equal(j, want) {
		t.Errorf("j = %v, want %v", j, want)
	}
}

func TestAWindowedStreamStaysInRowOrder(t *testing.T) {
	// Wrap the ring several times, so that the newest row is in the middle of
	// the buffer and only the unwrapping puts it last.
	s := data.NewStream("i").Window(5)
	for i := range 23 {
		s.Append(float64(i))
	}
	s.Snapshot()
	col, _ := s.Source().Float64Column("i")
	if want := []float64{18, 19, 20, 21, 22}; !equal(col, want) {
		t.Errorf("rows = %v, want %v", col, want)
	}
}

func TestAppendChecksItsColumns(t *testing.T) {
	s := data.NewStream("a", "b")
	if err := s.Append(1); err == nil {
		t.Error("a short row was accepted")
	}
	if err := s.Append(1, 2, 3); err == nil {
		t.Error("a long row was accepted")
	}
	if err := s.Append(1, 2); err != nil {
		t.Errorf("a correct row was refused: %v", err)
	}
}

func TestAppendTimeUsesUnixNanoseconds(t *testing.T) {
	when := time.Date(2026, 5, 4, 3, 2, 1, 0, time.UTC)
	s := data.NewStream("t", "v")
	if err := s.AppendTime(when, 7); err != nil {
		t.Fatal(err)
	}
	s.Snapshot()
	src := s.Source()
	ts, _ := src.Float64Column("t")
	if ts[0] != float64(when.UnixNano()) {
		t.Errorf("t = %v, want %v", ts[0], when.UnixNano())
	}
	v, _ := src.Float64Column("v")
	if v[0] != 7 {
		t.Errorf("v = %v, want 7", v)
	}
}

func TestResetEmptiesTheStreamButNotTheSnapshot(t *testing.T) {
	s := data.NewStream("y")
	s.Append(1)
	snap := s.Snapshot()
	s.Reset()
	if got := s.Len(); got != 0 {
		t.Errorf("after Reset the stream holds %d rows", got)
	}
	if got := snap.Len(); got != 1 {
		t.Errorf("Reset emptied a snapshot a renderer was holding: %d rows", got)
	}
}

func TestAStreamCarriesOnlyNumbers(t *testing.T) {
	s := data.NewStream("y")
	s.Append(1)
	src := s.Snapshot()
	if _, ok := src.TimeColumn("y"); ok {
		t.Error("a stream column reported itself as temporal")
	}
	if _, ok := src.StringColumn("y"); ok {
		t.Error("a stream column reported itself as categorical")
	}
	if got := src.Columns(); len(got) != 1 || got[0] != "y" {
		t.Errorf("columns = %v", got)
	}
	if _, ok := src.Float64Column("nope"); ok {
		t.Error("an absent column was found")
	}
}

func TestAppendIsSafeFromManyGoroutines(t *testing.T) {
	// The producer half of the contract: appends may come from anywhere. Run
	// this under -race, which is where it says something.
	s := data.NewStream("y").Window(1000)
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				s.Append(float64(g*1000 + i))
			}
		}()
	}
	wg.Wait()
	if got := s.Len(); got != 1000 {
		t.Errorf("the stream holds %d rows, want its window's 1000", got)
	}
}

func TestNewStreamRejectsNonsense(t *testing.T) {
	assertPanics(t, "no columns", func() { data.NewStream() })
	assertPanics(t, "a duplicate column", func() { data.NewStream("a", "a") })
}

func assertPanics(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic", what)
		}
	}()
	fn()
}

func equal(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] && !(math.IsNaN(a[i]) && math.IsNaN(b[i])) {
			return false
		}
	}
	return true
}

func BenchmarkStreamAppend(b *testing.B) {
	s := data.NewStream("t", "y").Window(4096)
	// Fill the window first: what is claimed is the steady state, where a row
	// overwrites the oldest one in a buffer that is already the right size.
	for i := range 4096 {
		s.Append(float64(i), 0)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		s.Append(float64(i), float64(i%97))
	}
}

func BenchmarkStreamSnapshot(b *testing.B) {
	s := data.NewStream("t", "y").Window(4096)
	for i := range 4096 {
		s.Append(float64(i), float64(i%97))
	}
	// Both buffers, because Snapshot alternates between them and each is
	// sized the first time it is filled.
	s.Snapshot()
	s.Snapshot()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		s.Snapshot()
	}
}

func TestTheStreamAndItsViewAgreeAboutColumns(t *testing.T) {
	s := data.NewStream("a", "b")
	if got := s.Columns(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Columns = %v, want the order NewStream was given", got)
	}
	// The list is a copy: a caller mangling it must not rename the stream's
	// columns.
	s.Columns()[0] = "mangled"
	if got := s.Columns(); got[0] != "a" {
		t.Errorf("Columns = %v after a caller wrote to a previous result", got)
	}

	s.Append(1, 2)
	s.Snapshot()
	view := s.Source()
	if got := view.Columns(); len(got) != 2 {
		t.Errorf("the view reports %v", got)
	}
	if _, ok := view.TimeColumn("a"); ok {
		t.Error("the view reported a temporal column")
	}
	if _, ok := view.StringColumn("a"); ok {
		t.Error("the view reported a categorical column")
	}
	if got := view.Len(); got != 1 {
		t.Errorf("the view holds %d rows", got)
	}
}
