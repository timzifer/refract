// Command releasecheck verifies a module against its published dependencies,
// with the development workspace disabled. It never edits manifests or tags.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var modules = []string{".", "backend/gg", "arrow/v18", "backend/gg/gpu", "backend/window"}

func main() {
	module := flag.String("module", "all", "module directory, or all")
	tag := flag.String("tag", "", "release tag; selects its module")
	flag.Parse()
	selected, err := selectModules(*module, *tag)
	if err == nil {
		for _, m := range selected {
			if err = check(m); err != nil {
				break
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "releasecheck:", err)
		os.Exit(1)
	}
}

func selectModules(module, tag string) ([]string, error) {
	if tag != "" {
		switch {
		case strings.HasPrefix(tag, "backend/gg/gpu/v"):
			module = "backend/gg/gpu"
		case strings.HasPrefix(tag, "backend/gg/v"):
			module = "backend/gg"
		case strings.HasPrefix(tag, "backend/window/v"):
			module = "backend/window"
		case strings.HasPrefix(tag, "arrow/v18."):
			module = "arrow/v18"
		case strings.HasPrefix(tag, "v") && !strings.Contains(tag, "/"):
			module = "."
		default:
			return nil, fmt.Errorf("unknown release tag %q", tag)
		}
	}
	if module == "all" {
		return modules, nil
	}
	for _, m := range modules {
		if module == m {
			return []string{m}, nil
		}
	}
	return nil, fmt.Errorf("unknown module %q", module)
}

func command(dir string, args ...string) *exec.Cmd {
	c := exec.Command("go", args...)
	c.Dir = filepath.FromSlash(dir)
	// Replace rather than append: duplicate environment keys behave differently
	// across operating systems. A caller's -mod=mod must not repair a release.
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		if !strings.EqualFold(key, "GOWORK") && !strings.EqualFold(key, "GOFLAGS") && !strings.EqualFold(key, "CGO_ENABLED") {
			c.Env = append(c.Env, e)
		}
	}
	c.Env = append(c.Env, "GOWORK=off", "GOFLAGS=-mod=readonly", "CGO_ENABLED=0")
	return c
}

func check(dir string) error {
	fmt.Printf("Checking %s with GOWORK=off and -mod=readonly\n", dir)
	b, err := command(dir, "mod", "edit", "-json").Output()
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
		c := command(dir, args...)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("%s: go %s failed; publish prerequisites and update require lines before tagging this module: %w", dir, strings.Join(args, " "), err)
		}
	}
	return nil
}
