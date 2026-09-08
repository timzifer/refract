// Command release tags a release of every module in this repository.
//
// The tag names are mechanical: the supported paths share the core's version,
// the Arrow adapter and the GPU tier carry majors of their own, and each tag is
// prefixed with its module's directory. Spelling that out by hand five times
// was the part of releasing that went wrong quietly, so this derives it.
//
// Tagging order does not matter. Each module is checked against the versions
// its own go.mod names, with the workspace off, and a green check is what
// licences the tag — see [release.Check]. Nothing here edits a manifest: a
// module that needs a newer core says so by failing its check, and that is a
// change to commit and review, not one to make during a release.
//
// Without -push the command stops after the checks and prints what it would
// have tagged.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/timzifer/refract/internal/release"
)

func main() {
	var (
		core    = flag.String("core", "", "core version to release, vX.Y.Z")
		summary = flag.String("summary", "", "what the core release is called, for its tag message")
		arrow   = flag.String("arrow", "", "version for arrow/v18 (default: a patch above its latest tag)")
		gpu     = flag.String("gpu", "", "version for backend/gg/gpu (default: a patch above its latest tag)")
		skip    = flag.String("skip", "", "comma-separated module directories to leave unreleased")
		push    = flag.Bool("push", false, "create the tags and push them; otherwise only check and report")
		remote  = flag.String("remote", "origin", "remote to push the tags to")
	)
	flag.Parse()
	if err := run(*core, *summary, map[string]string{"arrow/v18": *arrow, "backend/gg/gpu": *gpu}, *skip, *push, *remote); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

// A tagging is one module's place in the release: the version it gets, the tag
// that names it and the message that tag carries.
type tagging struct {
	module  release.Module
	version release.Version
	tag     string
	message string
}

func run(core, summary string, overrides map[string]string, skip string, push bool, remote string) error {
	if core == "" {
		return fmt.Errorf("-core is required")
	}
	coreVersion, err := release.ParseVersion(core)
	if err != nil {
		return err
	}
	tags, err := release.Tags()
	if err != nil {
		return err
	}
	releases, err := plan(coreVersion, summary, overrides, strings.Split(skip, ","), tags)
	if err != nil {
		return err
	}
	// Report before checking anything, so that a release blocked by a
	// precondition still says what it was going to do.
	report(releases, push)
	if err := precondition(releases, tags); err != nil {
		return err
	}
	for _, t := range releases {
		if err := release.Check(t.module.Dir); err != nil {
			return err
		}
	}
	if !push {
		fmt.Printf("\nChecks passed. Re-run with -push to tag and push.\n")
		return nil
	}
	return tag(releases, remote)
}

// plan works out what each module is released as. The modules that track the
// core take its version; the others move a patch above their own latest tag
// unless told otherwise, because a release that does not change their API
// still has to say which core they were tested against.
func plan(core release.Version, summary string, overrides map[string]string, skip, tags []string) ([]tagging, error) {
	skipped := map[string]bool{}
	for _, s := range skip {
		if s = strings.TrimSpace(s); s != "" {
			if _, err := release.Find(s); err != nil {
				return nil, fmt.Errorf("-skip: %w", err)
			}
			skipped[s] = true
		}
	}
	var plan []tagging
	for _, m := range release.Modules {
		if skipped[m.Dir] {
			continue
		}
		version, err := versionFor(m, core, overrides[m.Dir], tags)
		if err != nil {
			return nil, err
		}
		if err := m.CheckVersion(version); err != nil {
			return nil, err
		}
		if m.Dir == "." && summary == "" {
			return nil, fmt.Errorf("-summary is required: the core's tag message says what the release is called")
		}
		plan = append(plan, tagging{m, version, m.Tag(version), m.Message(version, core, summary)})
	}
	if len(plan) == 0 {
		return nil, fmt.Errorf("-skip left nothing to release")
	}
	return plan, nil
}

func versionFor(m release.Module, core release.Version, override string, tags []string) (release.Version, error) {
	if override != "" {
		return release.ParseVersion(override)
	}
	if m.TracksCore {
		return core, nil
	}
	latest, ok := m.Latest(tags)
	if !ok {
		return release.Version{}, fmt.Errorf("%s has no tag to move on from; give it a version explicitly", m.Dir)
	}
	return latest.NextPatch(), nil
}

// precondition refuses a release the repository cannot actually publish: a tag
// that exists, work that is not committed, or a commit that is not pushed.
func precondition(plan []tagging, existing []string) error {
	taken := map[string]bool{}
	for _, tag := range existing {
		taken[tag] = true
	}
	for _, t := range plan {
		if taken[t.tag] {
			return fmt.Errorf("tag %s already exists; a released version is never re-cut", t.tag)
		}
	}
	if err := release.CheckWorktree(); err != nil {
		return err
	}
	return release.CheckPushed()
}

func report(plan []tagging, push bool) {
	verb := "Would tag"
	if push {
		verb = "Tagging"
	}
	fmt.Printf("%s HEAD:\n", verb)
	width := 0
	for _, t := range plan {
		if len(t.tag) > width {
			width = len(t.tag)
		}
	}
	for _, t := range plan {
		fmt.Printf("  %-*s  %s\n", width, t.tag, t.message)
	}
	fmt.Println()
}

// tag writes every tag before pushing any, and removes the ones it wrote if it
// cannot write them all. A half-tagged HEAD is worse than an untagged one:
// the tags that exist look like a finished release.
func tag(plan []tagging, remote string) error {
	var written []string
	for _, t := range plan {
		if err := release.CreateTag(t.tag, t.message); err != nil {
			for _, name := range written {
				release.DeleteTag(name)
			}
			return err
		}
		written = append(written, t.tag)
	}
	if err := release.PushTags(remote, written); err != nil {
		return fmt.Errorf("%w\nthe tags exist locally; push them once the cause is fixed", err)
	}
	fmt.Printf("Pushed %d tags to %s.\n", len(written), remote)
	return nil
}
