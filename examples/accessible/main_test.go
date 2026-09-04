package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExampleWritesAChartThatCanBeRead(t *testing.T) {
	dir := t.TempDir()
	if err := run(dir); err != nil {
		t.Fatalf("run: %v", err)
	}

	svg := read(t, filepath.Join(dir, "accessible.svg"))
	for _, want := range []string{
		`role="img"`,
		"<title id=\"refract-title\">Signal against model</title>",
		"<desc id=\"refract-desc\">",
		"measured",
		"modelled",
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("the picture is missing %q", want)
		}
	}
	// The y title is notation, so it reaches the document as characters rather
	// than as the markup it was written in.
	if strings.Contains(svg, `\sqrt`) || strings.Contains(svg, `\frac`) {
		t.Error("the notation was drawn as markup")
	}
	if !strings.Contains(svg, "σ") || !strings.Contains(svg, "√") {
		t.Error("the notation was not set")
	}
	// Redundant encoding: at least one layer is dashed.
	if !strings.Contains(svg, "stroke-dasharray") {
		t.Error("no layer is dashed; colour is the only channel")
	}

	page := read(t, filepath.Join(dir, "accessible.html"))
	for _, want := range []string{"<table>", "<caption>measured</caption>", `alt="`, "<td>"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page is missing %q", want)
		}
	}

	text := read(t, filepath.Join(dir, "accessible.txt"))
	if !strings.Contains(text, "24 rows") {
		t.Errorf("the description does not say how much data there is:\n%s", text)
	}
	if strings.Contains(text, "$") {
		t.Errorf("the description carries markup:\n%s", text)
	}
}

func TestTheSummaryIsAvailableInParts(t *testing.T) {
	series := summarize(plot())
	if len(series) != 3 {
		t.Fatalf("described %d layers, want three", len(series))
	}
	if series[0].Label != "measured" || series[0].Rows != 24 {
		t.Errorf("the first layer is %+v", series[0])
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", filepath.Base(path), err)
	}
	return string(b)
}
