// Command releasecheck verifies a module against its published dependencies,
// with the development workspace disabled. It never edits manifests or tags.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/timzifer/refract/internal/release"
)

func main() {
	module := flag.String("module", "all", "module directory, or all")
	tag := flag.String("tag", "", "release tag; selects its module")
	flag.Parse()
	if err := run(*module, *tag); err != nil {
		fmt.Fprintln(os.Stderr, "releasecheck:", err)
		os.Exit(1)
	}
}

func run(module, tag string) error {
	selected, err := selectModules(module, tag)
	if err != nil {
		return err
	}
	for _, m := range selected {
		if err := release.Check(m.Dir); err != nil {
			return err
		}
	}
	return nil
}

func selectModules(module, tag string) ([]release.Module, error) {
	if tag != "" {
		m, err := release.ForTag(tag)
		if err != nil {
			return nil, err
		}
		return []release.Module{m}, nil
	}
	if module == "all" {
		return release.Modules, nil
	}
	m, err := release.Find(module)
	if err != nil {
		return nil, err
	}
	return []release.Module{m}, nil
}
