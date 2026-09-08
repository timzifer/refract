package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExampleRuns(t *testing.T) {
	dir := t.TempDir()
	qq, labels := filepath.Join(dir, "qq.svg"), filepath.Join(dir, "labels.svg")
	if err := run(qq, labels); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{qq: "Standard normal quantile", labels: "Alpha"} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), want) {
			t.Fatalf("%s lacks %s", path, want)
		}
	}
}
