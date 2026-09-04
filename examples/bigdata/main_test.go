package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExampleRuns draws both charts and checks that the reduction actually
// happened. Without it, either file would be tens of megabytes — which is the
// whole reason this example exists.
func TestExampleRuns(t *testing.T) {
	dir := t.TempDir()
	if err := run(dir); err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, c := range []struct {
		file string
		want []string
		max  int64
	}{
		{"bigdata-signal.svg", []string{"<svg", "Two million samples", "volts", "</svg>"}, 500_000},
		{"bigdata-cloud.svg", []string{"<svg", "A million points", "<image ", "</svg>"}, 500_000},
	} {
		path := filepath.Join(dir, c.file)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", c.file, err)
			continue
		}
		for _, want := range c.want {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s is missing %q", c.file, want)
			}
		}
		if int64(len(b)) > c.max {
			t.Errorf("%s is %d bytes, want under %d — the reduction did not happen",
				c.file, len(b), c.max)
		}
	}
}

// A gap in the data must still be a gap after the reduction, or a dropout is
// drawn as a line across the thing that was not measured.
func TestTheDropoutIsStillAGap(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "signal.svg")
	if err := signal(out); err != nil {
		t.Fatalf("signal: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "<polyline"); n < 2 {
		t.Errorf("the trace is %d polylines, want at least two: the dropout must break it", n)
	}
}

func TestTheGeneratorIsDeterministic(t *testing.T) {
	a, b := lcg(1), lcg(1)
	for range 100 {
		if x, y := a(), b(); x != y {
			t.Fatalf("two runs of the same seed disagree: %v vs %v", x, y)
		}
	}
}
