// Package ir defines refract's intermediate representation: a small,
// backend-agnostic scene description, plus the Backend interface every
// renderer implements.
//
// The IR is deliberately thin. It maps cleanly onto an immediate-mode canvas
// (github.com/gogpu/gg), onto a retained scene graph, and onto the built-in
// SVG emitter, without any of them leaking into the model layer. Geoms,
// scales and layout produce IR; they never talk to a backend directly.
//
// # Stability
//
// The primitive set and the Backend interface are frozen for the v0.1 cycle.
// Pre-1.0 they may still change between minor releases; see docs/adr/0002.
//
// # Coordinates
//
// Device coordinates, origin at the top-left, X right, Y down — the same
// convention as SVG, HTML canvas and gg. Positions are float32: they are
// device-space values after the model layer has already done its float64 work,
// and float32 is what a GPU backend ultimately wants.
package ir
