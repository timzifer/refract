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
	paths := map[string]string{}
	for _, name := range []string{"revenue", "traffic", "calls", "plan"} {
		paths[name] = filepath.Join(dir, name+".svg")
	}
	if err := run(paths["revenue"], paths["traffic"], paths["calls"], paths["plan"]); err != nil {
		t.Fatalf("run: %v", err)
	}

	for name, wants := range map[string][]string{
		// The legend names every series, which is the thing one swatch per
		// layer could not do.
		"revenue": {"<svg", "Revenue by product", "prism", "lens", "filter", "Q4"},
		"traffic": {"<svg", "Traffic by channel", "search", "email"},
		// A heatmap carries a colourbar rather than a legend: the colour is a
		// quantity, and a swatch cannot stand for a continuum.
		"calls": {"<svg", "Calls per hour", "linearGradient", "fri"},
		"plan":  {"<svg", "Plan", "design", "ship"},
	} {
		b, err := os.ReadFile(paths[name])
		if err != nil {
			t.Fatalf("no output was written for %s: %v", name, err)
		}
		for _, want := range wants {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s.svg is missing %q", name, want)
			}
		}
	}
}

func TestTheLongTablesAreWellFormed(t *testing.T) {
	quarter, product, value := revenue()
	if len(quarter) != len(product) || len(product) != len(value) {
		t.Fatalf("the revenue columns differ in length: %d, %d, %d",
			len(quarter), len(product), len(value))
	}
	// One row per (quarter, product) pair is what makes this one layer rather
	// than three.
	if len(quarter) != 12 {
		t.Errorf("got %d rows, want four quarters times three products", len(quarter))
	}

	day, channel, visits := traffic()
	if len(day) != len(channel) || len(channel) != len(visits) {
		t.Fatalf("the traffic columns differ in length")
	}
	for i, v := range visits {
		if v <= 0 {
			t.Fatalf("visit %d is %v: a stacked band cannot be negative here", i, v)
		}
	}
}
