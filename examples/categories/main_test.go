package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExampleRuns executes the documented example. The README quotes it, so
// this test is what stops it rotting.
func TestExampleRuns(t *testing.T) {
	dir := t.TempDir()
	bars := filepath.Join(dir, "sales.svg")
	boxes := filepath.Join(dir, "latency.svg")
	if err := run(bars, boxes); err != nil {
		t.Fatalf("run: %v", err)
	}

	for path, wants := range map[string][]string{
		bars:  {"<svg", "Sales by region", "north", "central", "</svg>"},
		boxes: {"<svg", "Latency by cohort", "alpha", "gamma", "</svg>"},
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("no output was written to %s: %v", path, err)
		}
		for _, want := range wants {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s is missing %q", filepath.Base(path), want)
			}
		}
	}
}

func TestSampleDataIsWellFormed(t *testing.T) {
	cohorts, values := samples()
	if len(cohorts) != len(values) {
		t.Fatalf("columns differ in length: %d vs %d", len(cohorts), len(values))
	}
	seen := map[string]int{}
	for _, c := range cohorts {
		seen[c]++
	}
	if len(seen) != 3 {
		t.Fatalf("got %d cohorts, want 3", len(seen))
	}
	for name, n := range seen {
		if n != 41 {
			t.Errorf("cohort %s has %d observations, want 41", name, n)
		}
	}
}
