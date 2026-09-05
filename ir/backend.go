package ir

import "image"

// Backend is what every renderer implements. It is an immediate-mode drawing
// sink: refract lowers a chart into calls on a Backend, in paint order.
//
// # Stability
//
// This interface has not changed since v0.1 (docs/adr/0002) and does not
// change from v1: it is implemented outside this module, so it never gains a
// method. What a backend can additionally do is an optional interface beside
// it — [Partial], [Resizer], [Semantics] are the three that exist — and refract
// asks with a type assertion and does without when the answer is no. It exists
// so that refract is insulated from any single rendering library: a change in
// a backend's own API is contained to that backend's adapter.
//
// # State
//
// A Backend carries no style state between calls — every drawing call is
// self-contained. The only state is the transform/clip stack managed by Push
// and Pop.
//
// # Ownership
//
// Everything a drawing call is given — a point slice, a [Path], an
// [image.Image] — is **lent for the duration of that call**. refract draws from
// pooled buffers so that a chart redrawn every frame allocates nothing that
// grows with its data, which means the next call may write over what the last
// one was handed. A Backend that needs to keep any of it must copy it;
// [Recorder] is the worked example.
//
// # Concurrency
//
// A Backend is not safe for concurrent use. refract may build IR on several
// goroutines but always plays it back into a backend from one.
type Backend interface {
	// Polyline strokes an open sequence of points. It is the fast path for
	// line geoms; it is exactly equivalent to StrokePath of a path built with
	// Path.Polyline, but lets a backend skip path construction entirely.
	Polyline(pts []Point, style Stroke)

	// StrokePath strokes an arbitrary path.
	StrokePath(p *Path, style Stroke)

	// FillPath fills a path's interior according to rule.
	FillPath(p *Path, fill Fill, rule FillRule)

	// Text draws a single-line run. The backend shapes it.
	Text(run TextRun)

	// Markers draws one shape at each of the given positions. This is the
	// scatter fast path: a backend may instance it.
	Markers(shape Marker, at []Point, style MarkerStyle)

	// Image blits a raster into dst, scaling to fit. This carries the
	// density-raster big-data path.
	Image(img image.Image, dst Rect)

	// Push pushes a transform and an optional clip path onto the state stack.
	// The transform composes with whatever is already on the stack; a nil clip
	// means "no additional clipping".
	Push(clip *Path, xform Affine)

	// Pop undoes the most recent Push.
	Pop()

	// Measure reports metrics for a run, from the same font stack that Text
	// will draw with. It must be callable before any drawing, because layout
	// runs first.
	Measure(run TextRun) TextMetrics

	// Flush completes the frame and reports any deferred error. A backend that
	// writes a file writes it here.
	Flush() error
}

// Target is a render destination: it opens a Backend sized for one chart, and
// finalises whatever it is writing to when Close is called.
//
// Splitting Target from Backend is what lets refract.Render take a destination
// before the chart's pixel size is known to that destination, and lets the
// same backend implementation serve a file, an io.Writer, or a window surface.
//
// Like [Backend], it is implemented outside this module and never gains a
// method.
type Target interface {
	// Open returns a Backend drawing into a surface of widthPx by heightPx
	// device pixels. dpr is the device pixel ratio: coordinates handed to the
	// backend are already in device pixels, so dpr is informational — backends
	// use it to pick hinting and stroke-snapping strategies.
	Open(widthPx, heightPx int, dpr float64) (Backend, error)

	// Close finalises the destination. It is called after the Backend's Flush.
	Close() error
}

// Resizer is an optional [Backend] interface: a backend drawing into a surface
// whose size can change while it is open implements it.
//
// A file backend never needs it — a document is opened at one size and written
// once — and a surface that stays open does: a window is dragged wider, a
// canvas element reflows, a terminal is resized. Rather than closing the
// target and opening another, which would lose the frame on screen and every
// zoom in the scales, the caller tells the backend its new size and draws
// again.
//
// It is optional for the reason every interface in this package is: adding
// Resize to Backend would break every third-party backend that has no surface
// to resize.
type Resizer interface {
	// Resize sets the surface's size in device pixels. The arguments mean what
	// [Target.Open]'s do, and the backend keeps whatever it was drawing into
	// where that is possible. It takes effect on the next frame.
	Resize(widthPx, heightPx int, dpr float64) error
}

// Description is what a chart says about itself in words.
//
// It carries no geometry and changes nothing about what is drawn. It is what
// a backend needs in order to be readable by something other than an eye: an
// SVG's <title> and <desc>, a PDF's document title, a canvas element's
// aria-label, a window's title bar.
//
// Title is a short label — a line of text, the chart's own title where it has
// one. Detail is the longer reading of the chart: what it plots, over what
// range, and how much of it there is. Either may be empty, and a backend
// writes nothing for an empty one rather than an empty element.
type Description struct {
	Title  string
	Detail string
}

// Empty reports whether d says nothing.
func (d Description) Empty() bool { return d.Title == "" && d.Detail == "" }

// Semantics is an optional [Backend] interface: a backend whose output can
// carry a description of what it draws implements it.
//
// refract calls Describe once, before any drawing call, so that a backend
// writing a document header has the description in hand when it writes one.
// A backend that has nowhere to put words — a raster, which is pixels and
// nothing else — does not implement it, and the description is simply not
// written.
type Semantics interface {
	// Describe attaches a description to the frame about to be drawn.
	Describe(d Description)
}
