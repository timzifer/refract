package gg

import (
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"os"

	gogg "github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
	"github.com/timzifer/refract/ir"
)

// Format is an output image format.
type Format uint8

// The supported formats.
const (
	FormatPNG Format = iota
	FormatJPEG
)

// Option configures a target.
type Option func(*options)

type options struct {
	fonts       *fontSet
	fontErr     error
	jpegQuality int
}

// WithFont replaces the embedded Go fonts with supplied TrueType or OpenType
// files.
//
// Pass bold or italic as nil to synthesise nothing and reuse regular for that
// style. Italic is worth supplying for a chart whose labels carry notation: a
// typesetter sets variables italic, and the vector emitters ask the viewer for
// an italic face whether or not one is given here.
func WithFont(regular, bold, italic []byte) Option {
	return func(o *options) {
		reg, err := text.NewFontSource(regular)
		if err != nil {
			o.fontErr = fmt.Errorf("refract/backend/gg: parsing the supplied regular font: %w", err)
			return
		}
		boldSrc, err := optionalFont(bold, "bold")
		if err != nil {
			o.fontErr = err
			return
		}
		italicSrc, err := optionalFont(italic, "italic")
		if err != nil {
			o.fontErr = err
			return
		}
		o.fonts = newFontSet(reg, boldSrc, italicSrc)
	}
}

// optionalFont parses one of WithFont's optional styles, or reports nil for a
// style the caller did not supply.
func optionalFont(ttf []byte, style string) (*text.FontSource, error) {
	if len(ttf) == 0 {
		return nil, nil
	}
	src, err := text.NewFontSource(ttf)
	if err != nil {
		return nil, fmt.Errorf("refract/backend/gg: parsing the supplied %s font: %w", style, err)
	}
	return src, nil
}

// JPEGQuality sets the JPEG encoder quality, 1 to 100. The default is 90.
func JPEGQuality(q int) Option {
	return func(o *options) {
		if q > 0 && q <= 100 {
			o.jpegQuality = q
		}
	}
}

// PNG returns a target that writes a PNG file.
func PNG(path string, opts ...Option) ir.Target {
	return &target{path: path, format: FormatPNG, opts: build(opts)}
}

// JPEG returns a target that writes a JPEG file.
func JPEG(path string, opts ...Option) ir.Target {
	return &target{path: path, format: FormatJPEG, opts: build(opts)}
}

// Writer returns a target that encodes into w in the given format.
func Writer(w io.Writer, format Format, opts ...Option) ir.Target {
	return &target{w: w, format: format, opts: build(opts)}
}

func build(opts []Option) options {
	o := options{jpegQuality: 90}
	for _, fn := range opts {
		fn(&o)
	}
	return o
}

type target struct {
	path   string
	w      io.Writer
	format Format
	opts   options

	ctx *gogg.Context
}

func (t *target) Open(widthPx, heightPx int, dpr float64) (ir.Backend, error) {
	if t.opts.fontErr != nil {
		return nil, t.opts.fontErr
	}
	if widthPx <= 0 || heightPx <= 0 {
		return nil, errors.New("refract/backend/gg: chart size must be positive")
	}
	fonts := t.opts.fonts
	if fonts == nil {
		var err error
		if fonts, err = defaultFonts(); err != nil {
			return nil, err
		}
	}

	// The context is created at logical size with a device scale, so that
	// coordinates handed to the backend stay in device-independent units while
	// the pixel buffer is dpr times larger. gg owns the HiDPI mapping; refract
	// does not scale anything itself.
	if dpr <= 0 {
		dpr = 1
	}
	t.ctx = gogg.NewContextWithScale(widthPx, heightPx, dpr)
	b := newBackend(t.ctx, fonts)
	b.dpr = dpr
	return b, nil
}

func (t *target) Close() error {
	if t.ctx == nil {
		return nil
	}
	defer func() {
		_ = t.ctx.Close()
		t.ctx = nil
	}()

	if t.w != nil {
		return t.encode(t.w)
	}
	f, err := os.Create(t.path)
	if err != nil {
		return err
	}
	if err := t.encode(f); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (t *target) encode(w io.Writer) error {
	img := t.ctx.Image()
	switch t.format {
	case FormatJPEG:
		return jpeg.Encode(w, img, &jpeg.Options{Quality: t.opts.jpegQuality})
	default:
		return png.Encode(w, img)
	}
}
