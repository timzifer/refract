package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExampleRuns executes the documented example, so that the chart tracks
// exist for cannot stop compiling without anyone noticing.
func TestExampleRuns(t *testing.T) {
	out := filepath.Join(t.TempDir(), "machine.svg")
	if err := run(out); err != nil {
		t.Fatalf("run: %v", err)
	}

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("no output was written: %v", err)
	}
	got := string(b)

	// The panel, both tracks and the legend: the speed's axis title, the state
	// lane names, the order lane, and an order's own label.
	for _, want := range []string{"<svg", "Line 3", "m/min", "running", "fault", "order", "WO-4472", "at target", "</svg>"} {
		if !strings.Contains(got, want) {
			t.Errorf("output is missing %q", want)
		}
	}
}

// TestTheSpansLineUpWithTheTrace. The strips are worth drawing only because
// they name what the trace shows, so the fault span has to cover the dip.
func TestTheSpansLineUpWithTheTrace(t *testing.T) {
	times, speed := trace()
	if len(times) != len(speed) {
		t.Fatalf("columns differ in length: %d vs %d", len(times), len(speed))
	}

	st := states(times[0])
	starts, ok := st.TimeColumn("start")
	if !ok {
		t.Fatal("no start column")
	}
	ends, ok := st.TimeColumn("end")
	if !ok {
		t.Fatal("no end column")
	}

	// The middle span is the fault. Every sample inside it is a slow one, and
	// the samples outside it are not.
	lo, hi := starts[1], ends[1]
	for i, at := range times {
		slow := speed[i] < 80
		inside := !at.Before(lo) && at.Before(hi)
		if slow != inside {
			t.Fatalf("sample %d at %v: speed %v, inside the fault span = %v", i, at, speed[i], inside)
		}
	}
}
