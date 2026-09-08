package main

import (
	"strings"
	"testing"
)

func TestReleaseTagsSelectOnlyTheirModule(t *testing.T) {
	for tag, want := range map[string]string{
		"v1.4.0": ".", "backend/gg/v1.4.0": "backend/gg",
		"backend/gg/gpu/v0.1.5": "backend/gg/gpu",
		"backend/gg/gpu/v1.0.0": "backend/gg/gpu",
		"backend/window/v1.4.0": "backend/window", "arrow/v18.0.3": "arrow/v18",
	} {
		got, err := selectModules("all", tag)
		if err != nil || len(got) != 1 || got[0] != want {
			t.Fatalf("%s: %v, %v; want %s", tag, got, err, want)
		}
	}
	for _, bad := range []string{"../outside", "backend/gg/../../outside"} {
		if _, err := selectModules(bad, ""); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestReleaseCheckCannotUseWorkspaceOrRepairManifests(t *testing.T) {
	t.Setenv("GOWORK", "some/go.work")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("CGO_ENABLED", "1")
	c := command(".", "test", "./...")
	for key, want := range map[string]string{"GOWORK": "off", "GOFLAGS": "-mod=readonly", "CGO_ENABLED": "0"} {
		n := 0
		for _, e := range c.Env {
			k, v, _ := strings.Cut(e, "=")
			if strings.EqualFold(k, key) {
				n++
				if v != want {
					t.Errorf("%s=%s, want %s", key, v, want)
				}
			}
		}
		if n != 1 {
			t.Errorf("%s appears %d times", key, n)
		}
	}
}
