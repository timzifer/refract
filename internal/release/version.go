package release

import (
	"fmt"
	"strconv"
	"strings"
)

// A Version is a release version. Release tags carry no prerelease or build
// metadata: a version that is not ready to be required by name is not ready to
// be tagged.
type Version struct{ Major, Minor, Patch int }

// ParseVersion reads a vMAJOR.MINOR.PATCH version.
func ParseVersion(s string) (Version, error) {
	rest, ok := strings.CutPrefix(s, "v")
	if !ok {
		return Version{}, fmt.Errorf("version %q does not begin with v", s)
	}
	fields := strings.Split(rest, ".")
	if len(fields) != 3 {
		return Version{}, fmt.Errorf("version %q is not vMAJOR.MINOR.PATCH", s)
	}
	var v Version
	for i, into := range []*int{&v.Major, &v.Minor, &v.Patch} {
		n, err := strconv.Atoi(fields[i])
		if err != nil || n < 0 || (len(fields[i]) > 1 && fields[i][0] == '0') {
			return Version{}, fmt.Errorf("version %q is not vMAJOR.MINOR.PATCH", s)
		}
		*into = n
	}
	return v, nil
}

func (v Version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Less orders two versions.
func (v Version) Less(w Version) bool {
	if v.Major != w.Major {
		return v.Major < w.Major
	}
	if v.Minor != w.Minor {
		return v.Minor < w.Minor
	}
	return v.Patch < w.Patch
}

// NextPatch is the version a module gets when a release does not change its
// own API — the default for the modules that do not track the core.
func (v Version) NextPatch() Version {
	return Version{v.Major, v.Minor, v.Patch + 1}
}

// Tag is the release tag for a version of this module.
func (m Module) Tag(v Version) string { return m.Prefix + v.String() }

// Message is the annotated tag message, given the core's version.
func (m Module) Message(v, core Version, summary string) string {
	if m.Dir == "." {
		return fmt.Sprintf(m.Title, v, summary)
	}
	return fmt.Sprintf(m.Title, v, core)
}

// CheckVersion rejects a version this module cannot publish under. Go resolves
// a module path without a /vN suffix only at v0 or v1, and a path with one only
// at that major, so a wrong major here is a tag nothing can require.
func (m Module) CheckVersion(v Version) error {
	if v.Major != m.Major {
		return fmt.Errorf("%s releases v%d, not %s", m.Dir, m.Major, v)
	}
	return nil
}

// Latest is the highest version already tagged for a module, and whether there
// is one. Tags are attributed with [ForTag], so the raster backend's history
// does not pick up the GPU tier's tags.
func (m Module) Latest(tags []string) (Version, bool) {
	var best Version
	found := false
	for _, tag := range tags {
		owner, err := ForTag(tag)
		if err != nil || owner.Dir != m.Dir {
			continue
		}
		v, err := ParseVersion(strings.TrimPrefix(tag, m.Prefix))
		if err != nil || v.Major != m.Major {
			continue
		}
		if !found || best.Less(v) {
			best, found = v, true
		}
	}
	return best, found
}
