package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExampleRuns executes the documented example, so that the charts a
// coloured path exists for cannot stop compiling without anyone noticing.
func TestExampleRuns(t *testing.T) {
	dir := t.TempDir()
	budget := filepath.Join(dir, "budget.svg")
	state := filepath.Join(dir, "state.svg")
	if err := run(budget, state); err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, tc := range []struct {
		path string
		want []string
	}{
		{budget, []string{"<svg", "Checkout latency", "ms", "200", "300", "</svg>"}},
		// The states are named in the legend, which is what a discrete scale
		// contributes in place of a colourbar.
		{state, []string{"<svg", "Line 3", "running", "fault", "maintenance", "</svg>"}},
	} {
		b, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("no output at %s: %v", tc.path, err)
		}
		got := string(b)
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Errorf("%s is missing %q", filepath.Base(tc.path), want)
			}
		}
	}
}

// TestTheTraceCrossesBothBoundaries. A threshold chart with three classes and
// only two of them drawn would be a chart that proves nothing.
func TestTheTraceCrossesBothBoundaries(t *testing.T) {
	src := trace()
	ms, ok := src.Float64Column("ms")
	if !ok {
		t.Fatal("no ms column")
	}
	var under, over, far bool
	for _, v := range ms {
		switch {
		case v < 200:
			under = true
		case v < 300:
			over = true
		default:
			far = true
		}
	}
	if !under || !over || !far {
		t.Fatalf("the trace visits under=%v over=%v far=%v, want all three classes", under, over, far)
	}
}
