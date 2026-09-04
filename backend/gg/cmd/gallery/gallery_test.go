package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// docsDir is the committed figure directory, relative to this package.
const docsDir = "../../../../docs/images"

// TestCommittedFiguresAreUpToDate is the documentation's drift test.
//
// The README and the docs embed rendered charts. A picture that no longer
// matches the code it claims to demonstrate is worse than no picture, and
// nothing about a stale PNG announces itself. So the figures are treated like
// any other generated artefact: regenerate, compare, fail on a difference.
//
// When it fails legitimately — because a render genuinely changed — the fix is
// to run the generator and commit the result:
//
//	go run ./backend/gg/cmd/gallery
func TestCommittedFiguresAreUpToDate(t *testing.T) {
	if err := verify(docsDir); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestEveryFigureRendersBothFormats(t *testing.T) {
	for _, f := range figures() {
		t.Run(f.name, func(t *testing.T) {
			svg, png, err := f.render()
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if !bytes.HasPrefix(svg, []byte("<?xml")) || !bytes.HasSuffix(svg, []byte("</svg>")) {
				t.Error("the SVG half is not a complete document")
			}
			if !bytes.HasPrefix(png, []byte("\x89PNG\r\n\x1a\n")) {
				t.Error("the PNG half does not start with a PNG signature")
			}
			if len(png) < 1000 {
				t.Errorf("the PNG half is only %d bytes — suspiciously empty", len(png))
			}
		})
	}
}

func TestFigureNamesAreUniqueAndPresent(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range figures() {
		if f.name == "" {
			t.Fatal("a figure has no name")
		}
		if seen[f.name] {
			t.Fatalf("duplicate figure name %q — one would overwrite the other", f.name)
		}
		seen[f.name] = true

		for _, ext := range []string{".svg", ".png"} {
			path := filepath.Join(docsDir, f.name+ext)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s is not committed: %v", path, err)
			}
		}
	}
}

func TestRenderingIsRepeatable(t *testing.T) {
	// -check compares against committed bytes, so a generator that is not
	// deterministic would make CI fail at random. Prove it is.
	for _, f := range figures() {
		svgA, pngA, err := f.render()
		if err != nil {
			t.Fatalf("%s: %v", f.name, err)
		}
		svgB, pngB, err := f.render()
		if err != nil {
			t.Fatalf("%s: %v", f.name, err)
		}
		if !bytes.Equal(svgA, svgB) {
			t.Errorf("%s: two SVG renders differ", f.name)
		}
		if !bytes.Equal(pngA, pngB) {
			t.Errorf("%s: two PNG renders differ", f.name)
		}
	}
}

func TestVerifyDetectsDrift(t *testing.T) {
	// A corrupted figure must be caught. Without this, a passing -check would
	// prove nothing about whether the comparison actually compares.
	dir := t.TempDir()
	if err := run(dir, false); err != nil {
		t.Fatalf("generating into a scratch directory: %v", err)
	}
	if err := verify(dir); err != nil {
		t.Fatalf("freshly generated figures should verify: %v", err)
	}

	name := figures()[0].name
	path := filepath.Join(dir, name+".svg")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify(dir); err == nil {
		t.Fatal("verify accepted a modified figure")
	}
}

func TestPNGDiffToleratesNothingButNoise(t *testing.T) {
	_, png, err := figures()[0].render()
	if err != nil {
		t.Fatal(err)
	}
	if d, err := pngDiff(png, png); err != nil || d != 0 {
		t.Fatalf("identical images differ by %v (err %v)", d, err)
	}

	_, other, err := figures()[1].render()
	if err != nil {
		t.Fatal(err)
	}
	d, err := pngDiff(png, other)
	if err == nil && d <= pngTolerance {
		t.Fatalf("two different figures compared equal (diff %v)", d)
	}
}
