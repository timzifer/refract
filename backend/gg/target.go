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

// WithFont replaces the embedded Go fonts with a supplied TrueType or
// OpenType file. Pass bold as nil to synthesise nothing and reuse regular for
// bold text.
func WithFont(regular, bold []byte) Option {
	return func(o *options) {
		reg, err := text.NewFontSource(regular)
		if err != nil {
			o.fontErr = fmt.Errorf("refract/backend/gg: parsing the supplied regular font: %w", err)
			return
		}
		var boldSrc *text.FontSource
		if len(bold) > 0 {
			boldSrc, err = text.NewFontSource(bold)
			if err != nil {
				o.fontErr = fmt.Errorf("refract/backend/gg: parsing the supplied bold font: %w", err)
				return
			}
		}
		o.fonts = newFontSet(reg, boldSrc)
	}
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
	t.ctx = gogg.NewContextWithScale(widthPx, heightPx, dpr)
	return newBackend(t.ctx, fonts), nil
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
