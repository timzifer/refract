package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/timzifer/refract/data"
)

var frames = []string{"start", "middle", "end"}

func runExample(t *testing.T) (data.Alignment, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	al, err := run(filepath.Join(dir, "frame"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	out := map[string]string{}
	for _, name := range frames {
		b, err := os.ReadFile(filepath.Join(dir, "frame-"+name+".svg"))
		if err != nil {
			t.Fatalf("no output was written for %s: %v", name, err)
		}
		out[name] = string(b)
	}
	return al, out
}

// The three groups are D3's, and this chart has one of each so that all three
// are visible in one picture.
func TestTheJoinReportsEnterUpdateExit(t *testing.T) {
	al, _ := runExample(t)
	for _, c := range []struct {
		name string
		got  []string
		want string
	}{
		{"entered", al.Entered(), "zig"},
		{"exited", al.Exited(), "perl"},
	} {
		if len(c.got) != 1 || c.got[0] != c.want {
			t.Errorf("%s = %v, want [%s]", c.name, c.got, c.want)
		}
	}
	if got := strings.Join(al.Updated(), ","); got != "go,rust" {
		t.Errorf("updated = %v, want [go rust]", al.Updated())
	}
}

// dataLabels are the layer's own labels: the value labels follow the six tick
// labels the pinned Y axis writes, so the last four are the bars'.
func dataLabels(t *testing.T, svg string) []string {
	t.Helper()
	all := regexp.MustCompile(`>(-?[0-9]+)</text>`).FindAllStringSubmatch(svg, -1)
	var out []string
	for _, m := range all {
		out = append(out, m[1])
	}
	if len(out) < 4 {
		t.Fatalf("only %d numeric labels were drawn", len(out))
	}
	return out[len(out)-4:]
}

// The label counts, which is the useful half of "animated text": a numeric
// column bound to geom.TextBy is re-spelled from the blend on every frame.
//
// The order is the union's: go, rust, perl (leaving), zig (arriving).
func TestTheLabelsCount(t *testing.T) {
	_, svgs := runExample(t)
	want := map[string][]string{
		"start":  {"40", "25", "18", "0"},
		"middle": {"35", "35", "9", "11"},
		"end":    {"30", "45", "0", "22"},
	}
	for _, name := range frames {
		got := dataLabels(t, svgs[name])
		if strings.Join(got, ",") != strings.Join(want[name], ",") {
			t.Errorf("%s labels = %v, want %v", name, got, want[name])
		}
	}
}

// And they are whole numbers, which is what data.Round is for: without it an
// interpolated value reads its full float and the label is arithmetic rather
// than a number.
func TestTheCountingLabelsAreRounded(t *testing.T) {
	_, svgs := runExample(t)
	for _, name := range frames {
		if strings.Contains(svgs[name], ".0000") {
			t.Errorf("the %s frame carries an unrounded float in a label", name)
		}
		for _, l := range dataLabels(t, svgs[name]) {
			if strings.Contains(l, ".") {
				t.Errorf("%s label %q is not a whole number", name, l)
			}
		}
	}
}

// Enter and exit are values in data space rather than a fade: the arriving bar
// starts at the baseline and the departing one ends there.
func TestEnterAndExitHappenAtTheBaseline(t *testing.T) {
	_, svgs := runExample(t)
	// zig is last in the union and arrives; perl is third and leaves.
	if got := dataLabels(t, svgs["start"])[3]; got != "0" {
		t.Errorf("the entering bar starts at %s, want 0", got)
	}
	if got := dataLabels(t, svgs["end"])[2]; got != "0" {
		t.Errorf("the exiting bar ends at %s, want 0", got)
	}
}

// Three frames of one movement, each of them a chart.
func TestEveryFrameIsAChart(t *testing.T) {
	_, svgs := runExample(t)
	for _, name := range frames {
		if !strings.Contains(svgs[name], "<svg") {
			t.Errorf("the %s frame is not an SVG", name)
		}
		if !strings.Contains(svgs[name], "Share, moving") {
			t.Errorf("the %s frame does not carry its title", name)
		}
	}
}

// A fraction is reproducible where a wall clock is not, which is the whole
// reason At is the primitive.
func TestTheFramesAreReproducible(t *testing.T) {
	_, first := runExample(t)
	_, second := runExample(t)
	for _, name := range frames {
		if first[name] != second[name] {
			t.Errorf("two runs drew different %s frames", name)
		}
	}
}
