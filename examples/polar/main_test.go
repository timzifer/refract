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
	for _, name := range []string{"browsers", "designs", "wind", "capacity", "budgets"} {
		paths[name] = filepath.Join(dir, name+".svg")
	}
	if err := run(paths["browsers"], paths["designs"], paths["wind"], paths["capacity"],
		paths["budgets"]); err != nil {
		t.Fatalf("run: %v", err)
	}

	for name, wants := range map[string][]string{
		// The legend names every slice, because a pie's slices live inside one
		// layer and one swatch per layer could not name them.
		"browsers": {"<svg", "Browser share", "chrome", "other", "C"},
		"designs":  {"<svg", "Two designs", "prism", "lens", "clarity"},
		"wind":     {"<svg", "Hours by wind direction", "NW"},
		"capacity": {"<svg", "Capacity used"},
		// The broken-out donut names every team, for the same reason: its
		// slices are one layer, and the legend is what tells them apart.
		"budgets": {"<svg", "Spend by team", "growth", "platform"},
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

// Every chart here is a curve, and every curve is cubics: the IR gained nothing
// for the coordinate stage, which is ADR 0002's claim under its first serious
// test.
func TestEveryPolarChartIsDrawnWithCubics(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 0, 5)
	for _, name := range []string{"browsers", "designs", "wind", "capacity", "budgets"} {
		paths = append(paths, filepath.Join(dir, name+".svg"))
	}
	if err := run(paths[0], paths[1], paths[2], paths[3], paths[4]); err != nil {
		t.Fatalf("run: %v", err)
	}
	// The radar is the exception and is meant to be: its sides are chords, so
	// its contour has no curve in it at all. Its rings do.
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), " C") {
			t.Errorf("%s has no cubic segments in it; a polar chart is arcs",
				filepath.Base(path))
		}
	}
}

func TestTheLongTableIsWellFormed(t *testing.T) {
	axes, names, scores := designs()
	if len(axes) != len(names) || len(names) != len(scores) {
		t.Fatalf("the design columns differ in length: %d, %d, %d",
			len(axes), len(names), len(scores))
	}
	// One row per (axis, design) pair is what makes this one layer rather than
	// two.
	if len(axes) != 10 {
		t.Errorf("got %d rows, want five axes times two designs", len(axes))
	}

	_, share := browsers()
	total := 0.0
	for _, v := range share {
		total += v
	}
	teams, spend, floor, used, pull := budgets()
	for _, col := range [][]float64{spend, floor, used, pull} {
		if len(col) != len(teams) {
			t.Errorf("a budget column has %d rows, want %d", len(col), len(teams))
		}
	}
	// Exactly one team is pulled out of the ring: a chart where everything is
	// emphasised emphasises nothing.
	out := 0
	for _, v := range pull {
		if v > 0 {
			out++
		}
	}
	if out != 1 {
		t.Errorf("%d slices are broken out, want the one that went over budget", out)
	}
	// The ring closes because the total is the domain's end. A share that did
	// not add up would leave a gap at twelve o'clock, and the chart would be
	// lying about the parts as well.
	if total != 100 {
		t.Errorf("the shares add to %v, want 100", total)
	}
}
