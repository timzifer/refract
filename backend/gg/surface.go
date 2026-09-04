package gg

import (
	"errors"
	"image"

	gogg "github.com/gogpu/gg"
	"github.com/timzifer/refract/ir"
)

// Surface is a target that draws into memory rather than into a file.
//
// [PNG] and [JPEG] encode a chart and are done with it. A Surface keeps the
// pixels: it stays open, its image can be read after every frame, and it can be
// resized and repainted in place. That is what a window needs — see
// github.com/timzifer/refract/backend/window, which presents this image as a
// texture — and what any caller compositing a chart into a larger picture needs
// too.
//
//	s := gg.NewSurface()
//	live, err := p.Live(s)
//	// ... draw, resize, draw again ...
//	img := s.Image()
//
// A Surface draws one chart at a time and is not safe for concurrent use.
type Surface struct {
	opts options
	b    *backend
}

// NewSurface returns an in-memory target. It takes the same options as the file
// targets: the font is the one thing a raster backend has to be told about.
func NewSurface(opts ...Option) *Surface {
	return &Surface{opts: build(opts)}
}

// Open prepares the surface for a chart of the given size. Opening a surface
// that is already open replaces what it held.
func (s *Surface) Open(widthPx, heightPx int, dpr float64) (ir.Backend, error) {
	if s.opts.fontErr != nil {
		return nil, s.opts.fontErr
	}
	if widthPx <= 0 || heightPx <= 0 {
		return nil, errors.New("refract/backend/gg: chart size must be positive")
	}
	fonts := s.opts.fonts
	if fonts == nil {
		var err error
		if fonts, err = defaultFonts(); err != nil {
			return nil, err
		}
	}
	if dpr <= 0 {
		dpr = 1
	}
	s.close()
	s.b = newBackend(gogg.NewContextWithScale(widthPx, heightPx, dpr), fonts)
	s.b.dpr = dpr
	return s.b, nil
}

// Close releases the pixel buffer. The image is not available afterwards.
func (s *Surface) Close() error {
	s.close()
	return nil
}

func (s *Surface) close() {
	if s.b != nil && s.b.ctx != nil {
		_ = s.b.ctx.Close()
	}
	s.b = nil
}

// Image returns what has been drawn, or nil before the first [Surface.Open].
//
// The image is the surface's own buffer rather than a copy, so it is valid
// until the next frame overwrites it — which is what makes reading it every
// frame free. A caller keeping one past that must copy it.
func (s *Surface) Image() image.Image {
	if s.b == nil || s.b.ctx == nil {
		return nil
	}
	// A GPU accelerator batches its work, so the pixels are not there until it
	// is asked for them. Without this the image is the frame before last on
	// the GPU tier and correct everywhere else, which is the worst kind of
	// bug — see backend/gg/gpu.
	_ = s.b.ctx.FlushGPU()
	return s.b.ctx.Image()
}

// Generation reports a counter that changes whenever the pixels do.
//
// It answers "is this frame the same as the last one" without comparing two
// buffers, which is what lets a window skip re-uploading a chart nobody has
// touched. It is zero for a surface that has not been opened, and it is not a
// frame number: a frame that painted nothing does not advance it.
func (s *Surface) Generation() uint64 {
	if s.b == nil || s.b.ctx == nil {
		return 0
	}
	pm := s.b.ctx.ResizeTarget()
	if pm == nil {
		return 0
	}
	return pm.GenerationID()
}

// Size reports the surface's logical size, and the device pixel ratio its
// buffer is scaled by.
func (s *Surface) Size() (w, h int, dpr float64) {
	if s.b == nil || s.b.ctx == nil {
		return 0, 0, 0
	}
	return s.b.ctx.Width(), s.b.ctx.Height(), s.b.dpr
}
