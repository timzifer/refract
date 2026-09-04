//go:build !(js && wasm)

// Command web draws an interactive chart in a browser. It only builds for
// js/wasm; see main_js.go and README.md in this directory.
//
// This file exists so that `go build ./...` and `go vet ./...` on a server
// still have a package here rather than an error about build constraints
// excluding every file.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr,
		"examples/web runs in a browser. Build it with:\n"+
			"  GOOS=js GOARCH=wasm go build -o examples/web/chart.wasm ./examples/web")
	os.Exit(1)
}
