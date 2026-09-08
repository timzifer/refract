package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runExample(t *testing.T) (stage string, n int, throughput, flow string) {
	t.Helper()
	dir := t.TempDir()
	tp := filepath.Join(dir, "throughput.svg")
	fp := filepath.Join(dir, "flow.svg")
	stage, n, err := run(tp, fp)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	a, err := os.ReadFile(tp)
	if err != nil {
		t.Fatalf("no throughput chart was written: %v", err)
	}
	b, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("no flow chart was written: %v", err)
	}
	return stage, n, string(a), string(b)
}

// The acceptance test for the whole thing: a hover in one chart arrives in the
// host with a key that identifies the row, and the host uses it to change the
// other chart.
func TestAHoverInOneChartReachesTheOther(t *testing.T) {
	stage, n, _, _ := runExample(t)

	if stage != "parse" {
		t.Errorf("the hover reported stage %q, want %q — the key did not survive", stage, "parse")
	}
	// ingest→parse, parse→index and parse→serve touch it; ingest→index and
	// index→serve do not.
	if n != 3 {
		t.Errorf("%d flows were highlighted, want 3", n)
	}
}

// Both charts are still charts. A wiring example that quietly stopped drawing
// would pass every assertion above.
func TestBothChartsAreDrawn(t *testing.T) {
	_, _, throughput, flow := runExample(t)
	for _, c := range []struct{ name, svg, title string }{
		{"throughput", throughput, "Throughput by stage"},
		{"flow", flow, "Bytes between stages"},
	} {
		if !strings.Contains(c.svg, "<svg") {
			t.Errorf("the %s chart is not an SVG", c.name)
		}
		if !strings.Contains(c.svg, c.title) {
			t.Errorf("the %s chart does not carry its title", c.name)
		}
	}
}

// The highlight is a colour over the whole edge list, so the sankey the reader
// ends up looking at still has every flow in it. A highlight that dropped the
// unselected edges would move every node, which is the mistake the package
// comment is about.
func TestTheHighlightKeepsEveryFlow(t *testing.T) {
	lit := flowsThrough("parse")
	if len(lit) != len(edgeFrom) {
		t.Fatalf("the highlight covers %d edges, want all %d", len(lit), len(edgeFrom))
	}
	tbl := flowTable(lit)
	if tbl.Len() != len(edgeFrom) {
		t.Errorf("the highlighted table has %d rows, want all %d", tbl.Len(), len(edgeFrom))
	}
	marks, ok := tbl.StringColumn("lit")
	if !ok {
		t.Fatal("the highlighted table has no lit column")
	}
	var selected int
	for _, m := range marks {
		if m == "selected" {
			selected++
		}
	}
	if selected != 3 {
		t.Errorf("%d edges are marked selected, want 3", selected)
	}
}

// A pointer over nothing lights nothing, rather than leaving the last
// selection on screen or lighting everything.
func TestNoKeyLightsNothing(t *testing.T) {
	if lit := flowsThrough(""); lit != nil {
		t.Errorf("an empty key lit %v", lit)
	}
	marks, _ := flowTable(nil).StringColumn("lit")
	for i, m := range marks {
		if m != "other" {
			t.Errorf("edge %d is %q with nothing hovered, want %q", i, m, "other")
		}
	}
}

// The feedback is drawn by refract rather than by this program: a crosshair
// and a tooltip on the chart being hovered, and a ring on each flow the hover
// selected in the other.
func TestTheOverlaysAreDrawn(t *testing.T) {
	_, _, throughput, flow := runExample(t)

	// The tooltip names the stage under the pointer and reads its values off
	// the same event the highlight was computed from.
	for _, want := range []string{"parse", "hour ", " rps"} {
		if !strings.Contains(throughput, want) {
			t.Errorf("the throughput chart's tooltip does not carry %q", want)
		}
	}
	// The rings are one stroked path of three circles, at the Highlight's
	// default width of two — which nothing else in this chart strokes at.
	if !strings.Contains(flow, `stroke-width="2"`) {
		t.Error("the flow chart carries no ring round the highlighted flows")
	}
}
