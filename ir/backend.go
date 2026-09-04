package ir

import "image"

// Backend is what every renderer implements. It is an immediate-mode drawing
// sink: refract lowers a chart into calls on a Backend, in paint order.
//
// # Stability
//
// This interface is frozen for the v0.1 cycle (docs/adr/0002). It exists so
// that refract is insulated from any single rendering library: a change in a
// backend's own API is contained to that backend's adapter.
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
type Target interface {
	// Open returns a Backend drawing into a surface of widthPx by heightPx
	// device pixels. dpr is the device pixel ratio: coordinates handed to the
	// backend are already in device pixels, so dpr is informational — backends
	// use it to pick hinting and stroke-snapping strategies.
	Open(widthPx, heightPx int, dpr float64) (Backend, error)

	// Close finalises the destination. It is called after the Backend's Flush.
	Close() error
}
