package release

import (
	"strings"
	"testing"
)

func TestCheckCannotUseWorkspaceOrRepairManifests(t *testing.T) {
	t.Setenv("GOWORK", "some/go.work")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("CGO_ENABLED", "1")
	t.Setenv("GOPRIVATE", "")
	c := Command(".", "test", "./...")
	for key, want := range environment {
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

func TestTagsAreAttributedToTheLongestMatchingPrefix(t *testing.T) {
	for tag, want := range map[string]string{
		"v1.4.0":                ".",
		"backend/gg/v1.4.0":     "backend/gg",
		"backend/gg/gpu/v0.1.5": "backend/gg/gpu",
		"backend/window/v1.4.0": "backend/window",
		"arrow/v18.0.3":         "arrow/v18",
	} {
		m, err := ForTag(tag)
		if err != nil || m.Dir != want {
			t.Errorf("%s: %v, %v; want %s", tag, m.Dir, err, want)
		}
	}
	for _, bad := range []string{"", "release-1", "backend/gg", "../outside"} {
		if m, err := ForTag(bad); err == nil {
			t.Errorf("%q was attributed to %s", bad, m.Dir)
		}
	}
}

func TestEveryModuleTagsUnderItsOwnPrefix(t *testing.T) {
	for _, m := range Modules {
		tag := m.Tag(Version{m.Major, 2, 3})
		owner, err := ForTag(tag)
		if err != nil || owner.Dir != m.Dir {
			t.Errorf("%s tags %s, attributed to %v (%v)", m.Dir, tag, owner.Dir, err)
		}
		if err := m.CheckVersion(Version{m.Major + 1, 0, 0}); err == nil {
			t.Errorf("%s accepted the wrong major", m.Dir)
		}
	}
}
