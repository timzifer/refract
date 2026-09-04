package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExampleRuns executes the documented example.
//
// The example is copied into the README and into CONCEPT.md §13, so it is the
// piece of code most likely to be read and least likely to be exercised. This
// test is what stops it rotting.
func TestExampleRuns(t *testing.T) {
	out := filepath.Join(t.TempDir(), "signal.svg")
	if err := run(out); err != nil {
		t.Fatalf("run: %v", err)
	}

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("no output was written: %v", err)
	}
	got := string(b)

	for _, want := range []string{"<svg", "Signal", "amplitude", "</svg>"} {
		if !strings.Contains(got, want) {
			t.Errorf("output is missing %q", want)
		}
	}
}

func TestSampleDataIsWellFormed(t *testing.T) {
	times, values := sample()
	if len(times) != len(values) {
		t.Fatalf("columns differ in length: %d vs %d", len(times), len(values))
	}
	if len(times) == 0 {
		t.Fatal("no sample data")
	}
	for i := 1; i < len(times); i++ {
		if !times[i].After(times[i-1]) {
			t.Fatalf("timestamps are not ascending at %d", i)
		}
	}
}
