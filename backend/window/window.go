package window

import (
	"errors"
	"fmt"
	"image"

	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
	ggbackend "github.com/timzifer/refract/backend/gg"
	"github.com/timzifer/refract/ir"
)

// Option configures a window.
type Option func(*config)

type config struct {
	title  string
	w, h   int
	fonts  []ggbackend.Option
	resize bool
}

// Title sets the window's title bar text.
func Title(s string) Option { return func(c *config) { c.title = s } }

// Size sets the window's initial size in device-independent pixels. It
// defaults to the chart's own size, which is usually what is wanted: a window
// that opens at the size the plot was designed for.
func Size(w, h int) Option {
	return func(c *config) {
		if w > 0 && h > 0 {
			c.w, c.h = w, h
		}
	}
}

// Font supplies the fonts the chart is drawn with, exactly as the file targets
// take them. See [github.com/timzifer/refract/backend/gg.WithFont].
func Font(opts ...ggbackend.Option) Option {
	return func(c *config) { c.fonts = append(c.fonts, opts...) }
}

// FollowSize controls whether the chart is redrawn at the window's size when
// the window is resized. It is on by default; turn it off to keep the chart at
// the size it was opened with and let the window letterbox it.
func FollowSize(on bool) Option { return func(c *config) { c.resize = on } }

// Window is a native window a chart is drawn in.
//
// It is two things joined at one seam. [Window.Target] is a drawing surface —
// the same CPU rasterizer that writes a PNG, drawing into memory — and
// [Window.Run] is an event loop that presents that memory as a texture and
// reports what the reader does to it. Neither knows what a chart is; the
// package next door, backend/window/show, is what joins them to a plot.
//
// A Window is used from one goroutine: the one that calls Run, which is the
// goroutine every callback then runs on.
type Window struct {
	cfg  config
	surf *ggbackend.Surface

	app *gogpu.App
	tex *gogpu.Texture
	gen uint64 // the pixel generation the texture was uploaded from

	w, h  int
	scale float64
	last  point // the pointer, for a scroll event that does not carry one
	err   error
}

// point is a pointer position in device-independent pixels.
type point struct{ x, y float64 }

// New returns a window that has not been opened yet.
//
// Nothing is created until [Window.Run]: a window is opened by running it, and
// a program that builds one and never runs it has not taken a screen.
func New(opts ...Option) *Window {
	c := config{title: "refract", w: 800, h: 500, resize: true}
	for _, o := range opts {
		o(&c)
	}
	w := &Window{cfg: c, w: c.w, h: c.h, scale: 1}
	w.surf = ggbackend.NewSurface(c.fonts...)
	return w
}

// Target returns the surface the chart is drawn into.
//
// It is an ordinary [ir.Target]: hand it to Plot.Live and draw whenever there
// is something new to show. What the window adds is that the pixels end up on
// a screen.
func (w *Window) Target() ir.Target { return w.surf }

// Size reports the window's current size in device-independent pixels, which
// is the size the chart should be laid out at.
func (w *Window) Size() (int, int) { return w.w, w.h }

// ScaleFactor reports the display's device pixel ratio.
func (w *Window) ScaleFactor() float64 { return w.scale }

// Redraw asks for a frame. It is what to call after changing something the
// window is showing; the loop is event-driven and idle otherwise, which is why
// a change nobody announces is a change nobody sees.
func (w *Window) Redraw() {
	if w.app != nil {
		w.app.RequestRedraw()
	}
}

// Close asks the window to close, ending [Window.Run].
func (w *Window) Close() {
	if w.app != nil {
		w.app.Quit()
	}
}

// Handler is what a window reports. Every field is optional, and every callback
// runs on the goroutine that called [Window.Run], one at a time — so a handler
// may touch a chart without a lock.
//
// Positions are in device-independent pixels relative to the window's content
// area, which is the same space the chart is laid out in: a point handed to a
// handler can go straight to Live.Move.
type Handler struct {
	// Frame is called before each present, and is where the chart is drawn.
	// Returning an error stops the loop and is what [Window.Run] returns.
	Frame func() error

	// Move, Press and Release report the pointer. Press and Release are the
	// primary button; a window reports no other, because a chart has no use
	// for one yet.
	Move    func(x, y float64)
	Press   func(x, y float64)
	Release func(x, y float64)

	// DoubleClick reports two presses in the same place in quick succession,
	// which every interactive chart uses to reset the view.
	DoubleClick func(x, y float64)

	// Scroll reports the wheel, in the browser's pixel convention: positive
	// scrolls the content away from the reader. Lines and pages are converted
	// to it, so a handler sees one unit whatever the platform counts in.
	Scroll func(x, y, delta float64)

	// Resize reports the window's new content size in device-independent
	// pixels, and Rescale a change of device pixel ratio — dragging a window
	// onto a display with a different one.
	Resize  func(w, h int)
	Rescale func(dpr float64)

	// Closed is called once, as the window is going away.
	Closed func()
}

// Run opens the window and runs its event loop until the window is closed.
//
// The loop is event-driven: it blocks on the operating system's event queue
// and wakes for input, a resize, or a [Window.Redraw]. A chart that nobody is
// touching costs nothing, which is the property that makes a window worth
// having over a browser tab and is why Frame is a callback rather than a
// polling loop.
//
// Run returns the first error a Frame reported, or the error the window layer
// failed with. It must be called on the main goroutine on the platforms that
// insist on it — macOS does — which in Go means from main, before anything
// else takes over.
func (w *Window) Run(h Handler) error {
	if w.app != nil {
		return errors.New("refract/backend/window: this window has already been run")
	}
	cfg := gogpu.DefaultConfig().WithTitle(w.cfg.title).WithSize(w.cfg.w, w.cfg.h)
	w.app = gogpu.NewApp(cfg)
	w.wire(h)

	// The texture holding the chart is a GPU resource, and it is released while
	// the renderer that made it is still alive. A window that leaked one per
	// run would leak it for the life of the process.
	w.app.OnClose(func() {
		if w.tex != nil {
			w.tex.Destroy()
			w.tex = nil
		}
	})

	if err := w.app.Run(); err != nil {
		return err
	}
	if h.Closed != nil {
		h.Closed()
	}
	return w.err
}

// wire connects the window layer's events to the handler.
func (w *Window) wire(h Handler) {
	w.app.OnResize(func(width, height int) {
		if width <= 0 || height <= 0 {
			return
		}
		w.w, w.h = width, height
		if s := w.app.ScaleFactor(); s > 0 && s != w.scale {
			w.scale = s
			if h.Rescale != nil {
				h.Rescale(s)
			}
		}
		if w.cfg.resize && h.Resize != nil {
			h.Resize(width, height)
		}
		w.app.RequestRedraw()
	})

	es := w.app.EventSource()
	w.wirePointer(es, h)
	w.wireScroll(es, h)

	w.app.OnDraw(func(dc *gogpu.Context) {
		w.frame(dc, h)
	})
}

// frame draws the chart and puts it on the screen.
//
// The chart is rasterized on the CPU into the surface's own buffer and blitted
// as one texture. That is a deliberate choice rather than a stopgap: it is the
// same rasterizer that writes the PNGs, so a window shows exactly what a file
// would, and there is one implementation of every mark rather than two.
// wirePointer connects the pointer.
//
// The detailed interface is preferred because the plain callbacks lose what a
// chart needs — which button a press was — and it is what this window layer
// implements. The fallback is for an event source that does not.
func (w *Window) wirePointer(es gpucontext.EventSource, h Handler) {
	var clicks clickTracker

	// press is the one place a press becomes either a double click or a plain
	// one, so that both paths through this function agree about which.
	press := func(x, y float64) {
		if clicks.press(x, y) && h.DoubleClick != nil {
			h.DoubleClick(x, y)
			return
		}
		if h.Press != nil {
			h.Press(x, y)
		}
	}

	pointers, ok := es.(gpucontext.PointerEventSource)
	if !ok {
		es.OnMouseMove(func(x, y float64) {
			w.last = point{x, y}
			if h.Move != nil {
				h.Move(x, y)
			}
			w.app.RequestRedraw()
		})
		es.OnMousePress(func(_ gpucontext.MouseButton, x, y float64) {
			w.last = point{x, y}
			press(x, y)
			w.app.RequestRedraw()
		})
		es.OnMouseRelease(func(_ gpucontext.MouseButton, x, y float64) {
			w.last = point{x, y}
			if h.Release != nil {
				h.Release(x, y)
			}
			w.app.RequestRedraw()
		})
		return
	}

	pointers.OnPointer(func(ev gpucontext.PointerEvent) {
		w.last = point{ev.X, ev.Y}
		switch ev.Type {
		case gpucontext.PointerMove:
			if h.Move != nil {
				h.Move(ev.X, ev.Y)
			}
		case gpucontext.PointerDown:
			// A chart has no use for a second button yet, and reporting one as
			// a press would pan on a right-drag.
			if ev.Button != gpucontext.ButtonLeft && ev.Button != gpucontext.ButtonNone {
				return
			}
			press(ev.X, ev.Y)
		case gpucontext.PointerUp:
			if h.Release != nil {
				h.Release(ev.X, ev.Y)
			}
		case gpucontext.PointerCancel:
			// A cancelled pointer is a released one that landed nowhere: the
			// window manager took the pointer away mid-drag. Releasing where it
			// was last seen ends the drag rather than leaving the chart panning
			// forever.
			if h.Release != nil {
				h.Release(ev.X, ev.Y)
			}
		}
		w.app.RequestRedraw()
	})
}

// wireScroll connects the wheel. The detailed interface carries the unit the
// platform counted in and the pointer's position; the plain one carries
// neither, so the fallback reads the delta as pixels and uses the last
// position the pointer was seen at.
func (w *Window) wireScroll(es gpucontext.EventSource, h Handler) {
	scrolls, ok := es.(gpucontext.ScrollEventSource)
	if !ok {
		es.OnScroll(func(_, dy float64) {
			if h.Scroll != nil {
				h.Scroll(w.last.x, w.last.y, dy)
			}
			w.app.RequestRedraw()
		})
		return
	}
	scrolls.OnScrollEvent(func(ev gpucontext.ScrollEvent) {
		if h.Scroll != nil {
			h.Scroll(ev.X, ev.Y, pixelDelta(ev))
		}
		w.app.RequestRedraw()
	})
}

func (w *Window) frame(dc *gogpu.Context, h Handler) {
	if h.Frame != nil {
		if err := h.Frame(); err != nil {
			w.fail(err)
			w.app.Quit()
			return
		}
	}
	img := w.surf.Image()
	if img == nil {
		return
	}
	if err := w.upload(dc, img); err != nil {
		w.fail(err)
		w.app.Quit()
		return
	}
	width, height := w.Size()
	if err := dc.DrawTextureScaled(w.tex, 0, 0, float32(width), float32(height)); err != nil {
		w.fail(err)
		w.app.Quit()
	}
}

// upload puts the chart's pixels on the GPU, and does not when they have not
// changed.
//
// The rasterizer stamps its buffer with a generation, so "has this frame
// changed" is a comparison of two integers rather than of two megabytes. A
// chart nobody is touching therefore reuses the texture it already has, which
// together with the event-driven loop is what makes an idle window free.
func (w *Window) upload(dc *gogpu.Context, img image.Image) error {
	gen := w.surf.Generation()
	if w.tex != nil && gen == w.gen {
		return nil
	}
	tex, err := dc.Renderer().NewTextureFromImage(img)
	if err != nil {
		return fmt.Errorf("refract/backend/window: uploading the chart: %w", err)
	}
	if w.tex != nil {
		w.tex.Destroy()
	}
	w.tex, w.gen = tex, gen
	return nil
}

func (w *Window) fail(err error) {
	if err != nil && w.err == nil {
		w.err = err
	}
}

// pixelDelta converts a scroll event into the pixel convention.
//
// A platform counts the wheel in pixels, lines or pages, and a zoom that
// depended on which would be a fortieth as fast on the machines that count
// lines. The line figure is the browsers' own: a line is forty pixels, a page
// is eight hundred.
func pixelDelta(ev gpucontext.ScrollEvent) float64 {
	switch ev.DeltaMode {
	case gpucontext.ScrollDeltaLine:
		return ev.DeltaY * 40
	case gpucontext.ScrollDeltaPage:
		return ev.DeltaY * 800
	}
	return ev.DeltaY
}
