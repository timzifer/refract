package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var charts = []string{"disk", "icicle", "sunburst", "flow", "arcs", "chord"}

func runAll(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, 0, len(charts))
	for _, name := range charts {
		paths = append(paths, filepath.Join(dir, name+".svg"))
	}
	if err := run(paths[0], paths[1], paths[2], paths[3], paths[4], paths[5]); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := map[string]string{}
	for i, name := range charts {
		b, err := os.ReadFile(paths[i])
		if err != nil {
			t.Fatalf("no output was written for %s: %v", name, err)
		}
		out[name] = string(b)
	}
	return out
}

// TestExampleRuns executes the documented example. The README quotes it, so
// this test is what stops it rotting.
func TestExampleRuns(t *testing.T) {
	got := runAll(t)
	for name, wants := range map[string][]string{
		"disk":     {"<svg", "Disk by directory"},
		"icicle":   {"<svg", "Disk by depth"},
		"sunburst": {"<svg", "Disk by directory"},
		// The legend names every node, because a flow's nodes live inside one
		// layer and one swatch per layer could not name them.
		"flow":  {"<svg", "Requests per second", "web", "mobile", "api", "cache", "search"},
		"arcs":  {"<svg", "Service traffic", "cdn", "db"},
		"chord": {"<svg", "Service traffic", "cache"},
	} {
		for _, want := range wants {
			if !strings.Contains(got[name], want) {
				t.Errorf("%s.svg is missing %q", name, want)
			}
		}
	}
}

// The claim the whole example is here to make: a sunburst is the icicle above
// it and a chord diagram is the arc diagram beside it, and the coord is the
// only difference. What is straight in the unit square is an arc once the
// square is wrapped round a circle, so the polar reading of each pair carries
// curves the flat one does not.
func TestTheRecipesAreTheMarksTheyAreMadeOf(t *testing.T) {
	got := runAll(t)
	for _, pair := range []struct{ flat, round string }{
		{"icicle", "sunburst"},
		{"arcs", "chord"},
	} {
		a, b := strings.Count(got[pair.flat], " C"), strings.Count(got[pair.round], " C")
		if b <= a {
			t.Errorf("%s has %d curve segments and %s has %d; the polar reading bends what the flat one leaves straight",
				pair.flat, a, pair.round, b)
		}
	}
}

// A layout mark places its own geometry, so the chart has no axis worth
// labelling — and drawing one would put a ladder of numbers from nought to one
// beside a picture of proportions.
func TestALayoutChartDrawsNoAxis(t *testing.T) {
	got := runAll(t)
	for _, name := range charts {
		for _, unwanted := range []string{"0.25", "0.75"} {
			if strings.Contains(got[name], ">"+unwanted+"<") {
				t.Errorf("%s.svg carries a tick label %q from the unit square", name, unwanted)
			}
		}
	}
}

// The edge list is what the flow charts read, and it has to be one: every link
// names two ends, and the hierarchy has exactly one root and no cycle.
func TestTheDataIsWellFormed(t *testing.T) {
	src := traffic()
	from, okF := src.StringColumn("from")
	to, okT := src.StringColumn("to")
	rps, okV := src.Float64Column("rps")
	if !okF || !okT || !okV {
		t.Fatal("the edge list is missing a column")
	}
	if len(from) != len(to) || len(from) != len(rps) {
		t.Fatalf("the edge list has %d sources, %d targets and %d values", len(from), len(to), len(rps))
	}
	for i := range from {
		if from[i] == "" || to[i] == "" {
			t.Errorf("edge %d has an unnamed end", i)
		}
		if rps[i] <= 0 {
			t.Errorf("edge %d carries %v, and a flow of nothing is not a flow", i, rps[i])
		}
	}

	h := disk()
	path, _ := h.StringColumn("path")
	under, _ := h.StringColumn("under")
	seen := map[string]bool{}
	roots := 0
	for i, id := range path {
		if seen[id] {
			t.Errorf("%q appears twice, and a hierarchy's names have to be distinct", id)
		}
		seen[id] = true
		if under[i] == "" {
			roots++
		}
	}
	if roots != 1 {
		t.Errorf("the tree has %d roots, want one", roots)
	}
	for i, p := range under {
		if p == "" {
			continue
		}
		if !seen[p] {
			t.Errorf("row %d hangs under %q, which no row declares", i, p)
		}
	}
}
