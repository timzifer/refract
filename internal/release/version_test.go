package release

import "testing"

func TestVersionsRoundTripAndOrder(t *testing.T) {
	for _, s := range []string{"v0.1.0", "v1.6.0", "v18.0.3", "v1.10.2"} {
		v, err := ParseVersion(s)
		if err != nil || v.String() != s {
			t.Errorf("%s: %v, %v", s, v, err)
		}
	}
	// A prerelease or a missing field is not a release version, and a padded
	// field is a different version from the one it looks like.
	for _, s := range []string{"1.6.0", "v1.6", "v1.6.0-rc1", "v1.6.0+meta", "v1.06.0", "v1.-6.0", "varrow"} {
		if v, err := ParseVersion(s); err == nil {
			t.Errorf("%q parsed as %s", s, v)
		}
	}
	if !(Version{1, 9, 0}).Less(Version{1, 10, 0}) {
		t.Error("v1.9.0 sorted after v1.10.0")
	}
	if (Version{1, 6, 0}).Less(Version{1, 6, 0}) {
		t.Error("a version sorted before itself")
	}
	if got := (Version{18, 0, 3}).NextPatch(); got != (Version{18, 0, 4}) {
		t.Errorf("next patch %s", got)
	}
}

func TestLatestIgnoresOtherModulesTags(t *testing.T) {
	tags := []string{
		"v1.5.0", "v1.6.0", "backend/gg/v1.6.0",
		"backend/gg/gpu/v0.2.0", "backend/gg/gpu/v0.3.0",
		"arrow/v18.0.3", "not-a-release",
	}
	gg, _ := Find("backend/gg")
	if v, ok := gg.Latest(tags); !ok || v != (Version{1, 6, 0}) {
		t.Errorf("backend/gg latest %s (%v); the GPU tier's tags are not its own", v, ok)
	}
	core, _ := Find(".")
	if v, ok := core.Latest(tags); !ok || v != (Version{1, 6, 0}) {
		t.Errorf("core latest %s (%v)", v, ok)
	}
	window, _ := Find("backend/window")
	if v, ok := window.Latest(tags); ok {
		t.Errorf("backend/window has no tags here, got %s", v)
	}
}

func TestTagMessagesFollowTheirModule(t *testing.T) {
	core, _ := Find(".")
	v := Version{1, 6, 0}
	if got, want := core.Message(v, v, "colour transforms"), "v1.6.0 — colour transforms"; got != want {
		t.Errorf("core message %q, want %q", got, want)
	}
	arrow, _ := Find("arrow/v18")
	if got, want := arrow.Message(Version{18, 0, 4}, v, ""), "arrow/v18.0.4 — the adapter at core v1.6.0"; got != want {
		t.Errorf("arrow message %q, want %q", got, want)
	}
	gg, _ := Find("backend/gg")
	if got, want := gg.Message(v, v, ""), "backend/gg v1.6.0 — the raster path at core v1.6.0"; got != want {
		t.Errorf("gg message %q, want %q", got, want)
	}
}
