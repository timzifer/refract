package svg

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/timzifer/refract/internal/fontmetrics"
	"github.com/timzifer/refract/ir"
)

// Option configures an SVG target.
type Option func(*options)

type options struct {
	font     *fontmetrics.Font
	fontFail error
	family   string
	pretty   bool
}

// WithFont supplies a TrueType or OpenType file whose metrics are used for
// layout. Without it the backend measures with a built-in generic-sans table,
// which is close but not exact.
//
// The font is not embedded in the output; only its metrics are read. Pair it
// with [WithFontFamily] so the viewer picks the same face.
func WithFont(ttf []byte) Option {
	return func(o *options) {
		f, err := fontmetrics.Parse(ttf)
		if err != nil {
			o.fontFail = err
			return
		}
		o.font = f
	}
}

// WithFontFamily sets the font-family attribute written into the SVG. The
// default is "sans-serif".
func WithFontFamily(family string) Option {
	return func(o *options) { o.family = family }
}

// Pretty writes one element per line. Off by default: the compact form is
// smaller, and golden files are diffed by tooling rather than read.
func Pretty() Option { return func(o *options) { o.pretty = true } }

// Writer returns a Target that writes an SVG document to w.
func Writer(w io.Writer, opts ...Option) ir.Target {
	return &target{w: nopCloser{w}, opts: build(opts)}
}

// File returns a Target that writes an SVG document to the named file. The
// file is created on Open and closed on Close.
func File(path string, opts ...Option) ir.Target {
	return &target{path: path, opts: build(opts)}
}

func build(opts []Option) options {
	o := options{family: "sans-serif"}
	for _, fn := range opts {
		fn(&o)
	}
	return o
}

type target struct {
	path string
	w    io.WriteCloser
	opts options
	bw   *bufio.Writer
}

func (t *target) Open(widthPx, heightPx int, dpr float64) (ir.Backend, error) {
	if t.opts.fontFail != nil {
		return nil, fmt.Errorf("refract/backend/svg: %w", t.opts.fontFail)
	}
	if t.w == nil {
		f, err := os.Create(t.path)
		if err != nil {
			return nil, err
		}
		t.w = f
	}
	t.bw = bufio.NewWriterSize(t.w, 64<<10)
	return newBackend(t.bw, widthPx, heightPx, dpr, t.opts), nil
}

func (t *target) Close() error {
	var err error
	if t.bw != nil {
		err = t.bw.Flush()
	}
	if cerr := t.w.Close(); err == nil {
		err = cerr
	}
	return err
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
