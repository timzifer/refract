package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExampleRuns executes the documented example. The README quotes it, so
// this test is what stops it rotting.
func TestExampleRuns(t *testing.T) {
	dir := t.TempDir()
	facets := filepath.Join(dir, "regions.svg")
	budget := filepath.Join(dir, "budget.svg")
	overview := filepath.Join(dir, "overview.pdf")
	if err := run(facets, budget, overview); err != nil {
		t.Fatalf("run: %v", err)
	}

	for path, wants := range map[string][]string{
		facets: {"<svg", "Throughput by region", "north", "central", "target", "</svg>"},
		budget: {"<svg", "Latency against its budget", "tolerance", "SLO", "budget", "</svg>"},
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

	b, err := os.ReadFile(overview)
	if err != nil {
		t.Fatalf("no PDF was written: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Error("the overview is not a PDF")
	}
	if !bytes.Contains(b, []byte("/Title (Fleet overview)")) {
		t.Error("the PDF carries no document title")
	}
}

func TestFleetDataIsWellFormed(t *testing.T) {
	regions, hours, rps := fleet()
	if len(regions) != len(hours) || len(hours) != len(rps) {
		t.Fatalf("columns differ in length: %d/%d/%d", len(regions), len(hours), len(rps))
	}
	seen := map[string]int{}
	for _, r := range regions {
		seen[r]++
	}
	if len(seen) != 5 {
		t.Fatalf("got %d regions, want 5", len(seen))
	}
	for name, n := range seen {
		if n != 24 {
			t.Errorf("region %s has %d hours, want 24", name, n)
		}
	}
}
