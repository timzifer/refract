// Package window draws refract charts in a native window.
//
// It is the fourth surface, after the two vector emitters and the browser
// canvas: a chart on a desktop, panned and zoomed with a mouse rather than
// exported and opened. The window itself comes from gogpu — windowing, input
// and lifecycle across Windows, macOS, Linux/X11 and Linux/Wayland, with no
// cgo — and the chart is rasterized by the same CPU backend that writes a PNG
// and presented as one texture.
//
//	w := window.New(window.Title("Signal"), window.Size(900, 600))
//	live, _ := p.Live(w.Target())
//	defer live.Close()
//	in := live.Input()
//
//	err := w.Run(window.Handler{
//	    Frame:   live.Draw,
//	    Move:    func(x, y float64) { in.Move(x, y) },
//	    Press:   func(x, y float64) { in.Down(x, y) },
//	    Release: func(x, y float64) { in.Up(x, y) },
//	    Scroll:  func(x, y, d float64) { in.Wheel(x, y, d) },
//	    Resize:  func(w, h int) { in.Resize(w, h) },
//	})
//
// Package show is that same wiring in one call, and is what most callers want.
//
// # Why the chart is rasterized on the CPU
//
// Because there is one implementation of every mark, and a window should show
// what a file would. The GPU is not idle in this arrangement — it composites and
// presents — and gg's own GPU tier can be switched on underneath, without
// anything here changing, by importing
// github.com/timzifer/refract/backend/gg/gpu.
//
// The cost of the arrangement is one texture upload per changed frame. It is
// paid only when the frame changed: refract repaints nothing when a frame is
// identical to the last, the rasterizer stamps its buffer with a generation
// that says whether it did, and the window compares two integers rather than
// two buffers. Between that and an event-driven loop that blocks on the
// operating system's queue, a chart nobody is touching costs no CPU at all.
//
// # A separate module
//
// Like backend/gg, this is a nested module: importing refract's core still
// yields a dependency graph with nothing in it but the standard library, and a
// program that renders SVG on a server links no window layer. It depends on
// backend/gg as well as on the core, because the rasterizer is where the marks
// are drawn.
//
// # Status
//
// The window layer is young, and so is the GPU stack under it. A chart in a
// window is v0.6's newest surface and the least proven one; the vector
// emitters remain the supported path for output that has to be right.
package window
