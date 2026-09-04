// Package show opens a plot in a native window.
//
//	func main() {
//	    p := refract.New(refract.Responsive(true), refract.Title("Signal"))
//	    p.Add(geom.Line(src, geom.X("t"), geom.Y("y")))
//
//	    if err := show.Plot(p, window.Title("Signal")); err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// The window hovers, clicks, zooms about the pointer, pans on a drag, resets
// the view on a double click and follows the window's size — all of which is
// [refract.Input] driving [refract.Live], the same state machine the browser
// uses. What this package adds is the wiring, and nothing else: a caller who
// wants a different one uses [Window] directly and writes their own.
//
// # Why this is not in package window
//
// Because wiring input is not drawing. A backend consumes IR and must not know
// what a scale or a panel is, and everything here is about scales and panels;
// package window draws, and this steers. The core makes the same split with
// Live.Bind, which is in the root package rather than in backend/canvas — and
// the reason it cannot simply live there too is that the core module depends on
// nothing, and a window is a dependency. See docs/adr/0021.
package show

import (
	"errors"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/backend/window"
)

// Plot opens p in a native window and returns when the window closes.
//
// It must be called from the main goroutine: some platforms will only run a
// window loop there. Handlers registered with [refract.Plot.On] fire from that
// goroutine too, one at a time, so they may touch the plot without a lock.
//
// The window opens at the plot's own size unless [window.Size] says otherwise,
// and the chart is laid out again whenever the window is resized. Give the plot
// [refract.Responsive] as well and the type and stroke weights follow the size;
// without it the chart keeps its proportions and gains room, which is what a
// dashboard usually wants and a poster usually does not.
func Plot(p *refract.Plot, opts ...window.Option) error {
	if p == nil {
		return errors.New("refract/backend/window/show: nil plot")
	}
	w := window.New(append([]window.Option{window.Size(p.Size())}, opts...)...)
	return Into(w, p)
}

// Into runs an already-configured window showing p.
//
// It is [Plot] with the window handed in, for a caller who wants to hold the
// window — to close it from elsewhere, to ask it to redraw when their data
// changes, or to read its size.
func Into(w *window.Window, p *refract.Plot) error {
	if w == nil || p == nil {
		return errors.New("refract/backend/window/show: nil window or plot")
	}
	live, err := p.Live(w.Target())
	if err != nil {
		return err
	}
	defer live.Close()

	in := live.Input()
	width, height := w.Size()
	if err := live.Resize(width, height); err != nil {
		return err
	}
	// The surface is rasterized at the display's device pixel ratio rather than
	// at the plot's, so a chart on a retina display is sharp rather than
	// magnified. The window reports a change of display the same way.
	if err := live.Rescale(w.ScaleFactor()); err != nil {
		return err
	}

	return w.Run(window.Handler{
		// The first frame and every frame after it are the same call: Live
		// paints only what changed and skips a frame identical to the last, so
		// asking for one costs nothing when nothing happened.
		Frame:       live.Draw,
		Move:        func(x, y float64) { in.Move(x, y) },
		Press:       func(x, y float64) { in.Down(x, y) },
		Release:     func(x, y float64) { in.Up(x, y) },
		DoubleClick: func(float64, float64) { in.DoubleClick() },
		Scroll:      func(x, y, d float64) { in.Wheel(x, y, d) },
		Resize:      func(width, height int) { in.Resize(width, height) },
		Rescale:     func(dpr float64) { in.Rescale(dpr) },
	})
}
