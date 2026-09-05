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
	name := func(s string) string { return filepath.Join(dir, s+".svg") }

	if err := run(
		name("latency"), name("services"), name("seasons"),
		name("cohorts"), name("cloud"), name("nations"),
	); err != nil {
		t.Fatalf("run: %v", err)
	}

	for file, wants := range map[string][]string{
		"latency": {"<svg", "Request latency", "milliseconds"},
		// The ECDF is written beside the histogram, on an axis of its own: a
		// fraction and a count are not the same quantity.
		"latency-cumulative": {"<svg", "cumulative", "fraction of requests"},
		// The legend names both regions, which is the composition with v0.7's
		// groups the milestone asks for.
		"services": {"<svg", "Latency by service", "checkout", "eu", "us"},
		"seasons":  {"<svg", "Daily maximum", "jan", "dec"},
		"cohorts":  {"<svg", "Scores by cohort", "variant B"},
		"cloud":    {"<svg", "Fifty thousand observations"},
		// A bubble chart carries three guides: the legend for the regions, and
		// the size key naming what the diameters mean.
		"nations": {"<svg", "Income and life expectancy", "Africa", "population (millions)"},
	} {
		b, err := os.ReadFile(name(file))
		if err != nil {
			t.Fatalf("no output was written for %s: %v", file, err)
		}
		for _, want := range wants {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s.svg is missing %q", file, want)
			}
		}
	}
}
