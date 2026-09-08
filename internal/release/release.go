// Package release holds what the release commands share: the list of modules
// this repository publishes, the tag each one is released under, and the check
// that a module builds against the versions its own go.mod names.
//
// The check is the whole safety argument for a release. A nested module
// requires the core at a published tag rather than through a replace
// directive, and go.work overrides that during development, so a green
// workspace test says nothing about what a downstream go get resolves. Running
// the same build with the workspace off, against exactly the version in the
// require line, is what says the published module works.
package release

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// A Module is one of the modules this repository publishes.
type Module struct {
	// Dir is the module directory relative to the repository root, "." for
	// the core.
	Dir string
	// Prefix is what its tags begin with: the directory without the
	// major-version suffix, empty for the core. A module's tag prefix is its
	// subdirectory without that suffix even when the module lives in a
	// major-version subdirectory, which is why arrow/v18 tags as arrow/
	// (ADR 0030).
	Prefix string
	// Major is the major version its tags must carry. A module path without
	// a /vN suffix can only publish v0 or v1, and one with a suffix has to
	// match it.
	Major int
	// TracksCore is set for the modules released in lockstep with the core:
	// the supported raster and native paths, whose own APIs are small enough
	// that a version of their own would say less than the core's does.
	TracksCore bool
	// Title formats the first line of the tag message, taking the module's
	// version and then the core's.
	Title string
}

// Modules is every module this repository publishes, in dependency order.
// Tagging no longer has to follow that order — see [Check] — but reporting
// still reads better along it.
var Modules = []Module{
	{Dir: ".", Prefix: "", Major: 1, TracksCore: true, Title: "%s — %s"},
	{Dir: "backend/gg", Prefix: "backend/gg/", Major: 1, TracksCore: true, Title: "backend/gg %s — the raster path at core %s"},
	{Dir: "arrow/v18", Prefix: "arrow/", Major: 18, Title: "arrow/%s — the adapter at core %s"},
	{Dir: "backend/gg/gpu", Prefix: "backend/gg/gpu/", Major: 0, Title: "backend/gg/gpu %s — the tier at core %s"},
	{Dir: "backend/window", Prefix: "backend/window/", Major: 1, TracksCore: true, Title: "backend/window %s — the native path at core %s"},
}

// Find returns the module released from a directory.
func Find(dir string) (Module, error) {
	for _, m := range Modules {
		if m.Dir == dir {
			return m, nil
		}
	}
	return Module{}, fmt.Errorf("unknown module %q", dir)
}

// ForTag returns the module a release tag belongs to. The longest prefix wins,
// so backend/gg/gpu/v0.3.0 is the GPU tier rather than a strangely named
// release of the raster backend.
func ForTag(tag string) (Module, error) {
	best := -1
	for i, m := range Modules {
		if !strings.HasPrefix(tag, m.Prefix+"v") {
			continue
		}
		if best < 0 || len(m.Prefix) > len(Modules[best].Prefix) {
			best = i
		}
	}
	if best < 0 {
		return Module{}, fmt.Errorf("unknown release tag %q", tag)
	}
	return Modules[best], nil
}

// environment is what every check runs under, replacing whatever the caller
// set. GOPRIVATE names this repository so that the module proxy is out of the
// path for its own tags: the proxy caches a negative lookup for a few minutes,
// which turns a tag pushed moments ago into a 404 that no amount of retrying
// clears. Fetching straight from git sees the tag as soon as it is pushed.
var environment = map[string]string{
	"GOWORK":      "off",
	"GOFLAGS":     "-mod=readonly",
	"CGO_ENABLED": "0",
	"GOPRIVATE":   "github.com/timzifer/refract",
}

// Command builds a go command for a module directory, under the release
// environment.
func Command(dir string, args ...string) *exec.Cmd {
	c := exec.Command("go", args...)
	c.Dir = filepath.FromSlash(dir)
	// Replace rather than append: duplicate environment keys behave differently
	// across operating systems. A caller's -mod=mod must not repair a release.
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if _, overridden := environment[strings.ToUpper(key)]; !overridden {
			c.Env = append(c.Env, e)
		}
	}
	for key, value := range environment {
		c.Env = append(c.Env, key+"="+value)
	}
	return c
}

// Check builds, tests and vets a module against the versions its own go.mod
// names, with the workspace disabled. It never edits manifests or tags.
//
// Green means the module works with the core it requires, which is all a
// release has to promise: that require is a floor, and minimal version
// selection raises it for anyone importing both. So a green check licences the
// tag without the require line naming the version being tagged. Red is the
// signal — and the only signal — that a require line has to move.
func Check(dir string) error {
	fmt.Printf("Checking %s with GOWORK=off and -mod=readonly\n", dir)
	b, err := Command(dir, "mod", "edit", "-json").Output()
	if err != nil {
		return fmt.Errorf("%s: read go.mod: %w", dir, err)
	}
	var mod struct {
		Replace []json.RawMessage
		Require []json.RawMessage
	}
	if err := json.Unmarshal(b, &mod); err != nil {
		return err
	}
	if len(mod.Replace) != 0 {
		return fmt.Errorf("%s: release manifests must not contain replace directives", dir)
	}
	if dir == "." && len(mod.Require) != 0 {
		return fmt.Errorf("core must not declare dependencies")
	}
	for _, args := range [][]string{{"build", "./..."}, {"test", "./..."}, {"vet", "./..."}} {
		c := Command(dir, args...)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("%s: go %s failed; publish prerequisites and update require lines before tagging this module: %w", dir, strings.Join(args, " "), err)
		}
	}
	return nil
}
