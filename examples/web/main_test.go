package main

import (
	"os"
	"strings"
	"testing"
)

// TestThePageLoadsWhatTheBuildProduces checks the two halves of the example
// agree: the page fetches the file the documented build command writes, and it
// loads the Go runtime shim that file needs.
//
// A browser example cannot be run by `go test` on a server, so this is what
// there is: the parts that can silently drift are the file names.
func TestThePageLoadsWhatTheBuildProduces(t *testing.T) {
	page, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"chart.wasm", "wasm_exec.js", `id="chart"`, `id="readout"`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("index.html does not mention %q", want)
		}
	}

	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "examples/web/chart.wasm") {
		t.Error("README.md does not document the build that produces chart.wasm")
	}
}
