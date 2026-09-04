// Package gg renders refract charts through github.com/gogpu/gg.
//
// This is a separate Go module. Importing refract's core gets you a
// stdlib-only dependency graph and SVG output; importing this module adds the
// GoGPU stack and gets you raster output — still with zero CGO, so
// CGO_ENABLED=0 builds and cross-compiles exactly as before.
//
//	import ggbackend "github.com/timzifer/refract/backend/gg"
//
//	err := p.Render(ggbackend.PNG("chart.png"))
//
// # What this backend uses, and what it deliberately does not
//
// It touches only the gg root package and gg/text. It does not import gg/gpu,
// gg/scene or gg/recording. That is a deliberate limit on the coupling surface
// (docs/adr/0006): gg is young and moves fast, so the narrower the adapter's
// contact with it, the cheaper it is to follow. It also means the CPU
// rasterizer is what runs — no GPU device is ever created, and CI needs no
// graphics hardware.
//
// The GPU tier, PDF output and a native window are later milestones. When they
// arrive they will arrive here, behind the same ir.Backend interface.
//
// # Text
//
// gg ships no default font, so this backend embeds Go Regular and Go Bold
// (golang.org/x/image/font/gofont, BSD-3-Clause) and uses them unless a font
// is supplied with [WithFont]. Measurement comes from the same face that draws
// the text, so metrics here are exact — unlike the built-in SVG backend, which
// approximates.
package gg
