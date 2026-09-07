package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timzifer/refract/coord"
)

// TestExampleRuns executes the documented example. The README quotes it, so
// this test is what stops it rotting.
func TestExampleRuns(t *testing.T) {
	dir := t.TempDir()
	paths := map[string]string{}
	for _, name := range []string{"antenna", "matching", "admittance"} {
		paths[name] = filepath.Join(dir, name+".svg")
	}
	if err := run(paths["antenna"], paths["matching"], paths["admittance"]); err != nil {
		t.Fatalf("run: %v", err)
	}

	for name, wants := range map[string][]string{
		// Every chart carries the grid a paper Smith chart is printed with,
		// and the labels come from the two axes' own ticks.
		"antenna":    {"<svg", "A patch antenna across its band", "0.2", "0.5", "1.0", "2.0", "5.0"},
		"matching":   {"<svg", "Matching 15 − j25 Ω to 50 Ω"},
		"admittance": {"<svg", "The same two steps, read as admittance"},
	} {
		b, err := os.ReadFile(paths[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, want := range wants {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s.svg does not contain %q", name, want)
			}
		}
	}
}

// A Smith chart's grid is curves whatever its data is: the constant-resistance
// circles and the constant-reactance arcs reach the backend as cubics, where a
// Cartesian grid line reaches it as a two-point polyline. It is the inverse of
// the radar's exception in examples/polar, where the coord curves and the mark
// asks not to.
func TestTheGridIsAlwaysCurves(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "antenna.svg")
	if err := measuredSweep(out); err != nil {
		t.Fatalf("measuredSweep: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), " C"); n < 40 {
		t.Errorf("the document holds %d cubic segments, too few for a grid of circles and arcs", n)
	}
	// The real axis is still a straight run, and still a polyline.
	if !strings.Contains(string(b), "<polyline") {
		t.Error("the real axis is not drawn as a polyline")
	}
}

// The measured sweep arrives as a reflection coefficient and is plotted as an
// impedance. If SmithZ ever stopped being the inverse of the coord's own map,
// the chart would still draw and would be quietly wrong, so the conversion is
// checked against the reflection it came from.
func TestTheSweepConvertsBackToWhatWasMeasured(t *testing.T) {
	re, im := s11Sweep(41)
	for i := range re {
		r, x := coord.SmithZ(re[i], im[i])
		if r < 0 {
			t.Fatalf("sample %d converted to a negative resistance %v", i, r)
		}
		// Γ = (z − 1)/(z + 1), the map the coord applies.
		d := (r+1)*(r+1) + x*x
		gotRe, gotIm := (r*r+x*x-1)/d, 2*x/d
		if abs(gotRe-re[i]) > 1e-9 || abs(gotIm-im[i]) > 1e-9 {
			t.Fatalf("sample %d: Γ = (%v, %v) came back as (%v, %v)", i, re[i], im[i], gotRe, gotIm)
		}
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
