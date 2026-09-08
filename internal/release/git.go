package release

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func git(args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Stderr = os.Stderr
	b, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(b)), nil
}

// Tags is every tag in the repository.
func Tags() ([]string, error) {
	out, err := git("tag", "--list")
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// CheckWorktree refuses to release from a tree that does not match what is
// committed. A tag names a commit, so anything only in the working tree is not
// in the release however green the check was.
func CheckWorktree() error {
	out, err := git("status", "--porcelain")
	if err != nil {
		return err
	}
	if out != "" {
		return fmt.Errorf("the working tree has uncommitted changes:\n%s", out)
	}
	return nil
}

// CheckPushed refuses to tag a commit that is not on the upstream branch.
// Pushing a tag does not push the commit under it, so a tag on a local-only
// commit publishes a version the proxy cannot fetch — and the proxy caches
// that failure.
func CheckPushed() error {
	upstream, err := git("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return fmt.Errorf("no upstream branch to check HEAD against: %w", err)
	}
	if err := exec.Command("git", "merge-base", "--is-ancestor", "HEAD", upstream).Run(); err != nil {
		return fmt.Errorf("HEAD is not on %s; push the branch before tagging it", upstream)
	}
	return nil
}

// CreateTag writes an annotated tag on HEAD.
func CreateTag(name, message string) error {
	_, err := git("tag", "--annotate", name, "--message", message)
	return err
}

// PushTags pushes tags to a remote in one call, so a release either arrives or
// does not.
func PushTags(remote string, names []string) error {
	c := exec.Command("git", append([]string{"push", remote}, names...)...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("git push %s: %w", remote, err)
	}
	return nil
}

// DeleteTag removes a local tag, to undo a tagging that could not be finished.
func DeleteTag(name string) error {
	_, err := git("tag", "--delete", name)
	return err
}
