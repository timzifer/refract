package main

import (
	"strings"
	"testing"

	"github.com/timzifer/refract/internal/release"
)

// v is a version written the way a tag writes it.
func v(s string) release.Version {
	version, err := release.ParseVersion(s)
	if err != nil {
		panic(err)
	}
	return version
}

// tags is a repository history: the v1.5.0 release, tagged across all five
// modules the way this command tags them.
var tags = []string{
	"v1.4.0", "v1.5.0",
	"backend/gg/v1.4.0", "backend/gg/v1.5.0",
	"backend/window/v1.5.0",
	"backend/gg/gpu/v0.1.4", "backend/gg/gpu/v0.2.0",
	"arrow/v18.0.2", "arrow/v18.0.3",
}

func TestPlanDerivesEveryTagFromTheCoreVersion(t *testing.T) {
	got, err := plan(v("v1.6.0"), "colour transforms", nil, nil, tags)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"v1.6.0":                "v1.6.0 — colour transforms",
		"backend/gg/v1.6.0":     "backend/gg v1.6.0 — the raster path at core v1.6.0",
		"backend/window/v1.6.0": "backend/window v1.6.0 — the native path at core v1.6.0",
		"backend/gg/gpu/v0.2.1": "backend/gg/gpu v0.2.1 — the tier at core v1.6.0",
		"arrow/v18.0.4":         "arrow/v18.0.4 — the adapter at core v1.6.0",
	}
	if len(got) != len(want) {
		t.Fatalf("planned %d tags, want %d", len(got), len(want))
	}
	for _, tagging := range got {
		if message, ok := want[tagging.tag]; !ok || message != tagging.message {
			t.Errorf("%s: %q", tagging.tag, tagging.message)
		}
	}
}

func TestPlanTakesOverridesAndSkips(t *testing.T) {
	got, err := plan(v("v1.6.0"), "x",
		map[string]string{"backend/gg/gpu": "v0.3.0"},
		[]string{"backend/window", " arrow/v18 "}, tags)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tagging := range got {
		names = append(names, tagging.tag)
	}
	if want := "v1.6.0 backend/gg/v1.6.0 backend/gg/gpu/v0.3.0"; strings.Join(names, " ") != want {
		t.Errorf("planned %v, want %s", names, want)
	}
}

func TestPlanRefusesWhatCannotBePublished(t *testing.T) {
	for name, run := range map[string]func() error{
		"a core release with no summary": func() error {
			_, err := plan(v("v1.6.0"), "", nil, nil, tags)
			return err
		},
		"a major the module cannot publish": func() error {
			_, err := plan(v("v2.0.0"), "x", nil, nil, tags)
			return err
		},
		"an override that is not a version": func() error {
			_, err := plan(v("v1.6.0"), "x", map[string]string{"arrow/v18": "18.0.4"}, nil, tags)
			return err
		},
		"a module that does not exist": func() error {
			_, err := plan(v("v1.6.0"), "x", nil, []string{"backend/pdf"}, tags)
			return err
		},
		"a first release with nothing to move on from": func() error {
			_, err := plan(v("v1.6.0"), "x", nil, nil, []string{"v1.5.0"})
			return err
		},
		"skipping everything": func() error {
			_, err := plan(v("v1.6.0"), "x", nil,
				[]string{".", "backend/gg", "backend/window", "backend/gg/gpu", "arrow/v18"}, tags)
			return err
		},
	} {
		if err := run(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestPreconditionRefusesATagThatExists(t *testing.T) {
	got, err := plan(v("v1.5.0"), "already out", nil, nil, tags)
	if err != nil {
		t.Fatal(err)
	}
	if err := precondition(got, tags); err == nil {
		t.Error("re-cutting v1.5.0 was accepted")
	}
}
