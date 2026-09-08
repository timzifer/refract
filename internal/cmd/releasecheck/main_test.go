package main

import "testing"

func TestReleaseTagsSelectOnlyTheirModule(t *testing.T) {
	for tag, want := range map[string]string{
		"v1.4.0": ".", "backend/gg/v1.4.0": "backend/gg",
		"backend/gg/gpu/v0.1.5": "backend/gg/gpu",
		"backend/gg/gpu/v1.0.0": "backend/gg/gpu",
		"backend/window/v1.4.0": "backend/window", "arrow/v18.0.3": "arrow/v18",
	} {
		got, err := selectModules("all", tag)
		if err != nil || len(got) != 1 || got[0].Dir != want {
			t.Fatalf("%s: %v, %v; want %s", tag, got, err, want)
		}
	}
	for _, bad := range []string{"../outside", "backend/gg/../../outside"} {
		if _, err := selectModules(bad, ""); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
