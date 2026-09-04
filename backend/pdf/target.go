package pdf

import (
	"bufio"
	"errors"
	"io"
	"os"

	"github.com/timzifer/refract/ir"
)

// Option configures a PDF target.
type Option func(*options)

type options struct {
	title    string
	author   string
	subject  string
	compress bool
}

// Title sets the document title shown in a reader's properties panel. It is
// independent of the chart title.
func Title(s string) Option { return func(o *options) { o.title = s } }

// Author sets the document author.
func Author(s string) Option { return func(o *options) { o.author = s } }

// Subject sets the document subject.
func Subject(s string) Option { return func(o *options) { o.subject = s } }

// Uncompressed writes the content stream as plain text rather than deflating
// it. The file is several times larger and can be read in a text editor, which
// is what it is for: reading a diff of what the backend emitted.
func Uncompressed() Option { return func(o *options) { o.compress = false } }

// Writer returns a Target that writes a PDF document to w.
func Writer(w io.Writer, opts ...Option) ir.Target {
	return &target{w: nopCloser{w}, opts: build(opts)}
}

// File returns a Target that writes a PDF document to the named file. The file
// is created on Open and closed on Close.
func File(path string, opts ...Option) ir.Target {
	return &target{path: path, opts: build(opts)}
}

func build(opts []Option) options {
	o := options{compress: true}
	for _, fn := range opts {
		fn(&o)
	}
	return o
}

type target struct {
	path string
	w    io.WriteCloser
	opts options

	bw *bufio.Writer
	b  *backend
}

// Open starts a document.
//
// dpr is ignored: PDF is resolution-independent, so a device pixel ratio has
// nothing to scale. One device-independent pixel becomes one PDF point, which
// makes a chart specified as 800x500 come out as an 800x500pt page.
func (t *target) Open(widthPx, heightPx int, dpr float64) (ir.Backend, error) {
	if widthPx <= 0 || heightPx <= 0 {
		return nil, errors.New("refract/backend/pdf: chart size must be positive")
	}
	if t.w == nil {
		f, err := os.Create(t.path)
		if err != nil {
			return nil, err
		}
		t.w = f
	}
	t.bw = bufio.NewWriterSize(t.w, 64<<10)
	t.b = newBackend(widthPx, heightPx, t.opts)
	return t.b, nil
}

// Close serialises the document and finishes the destination.
//
// The whole file is assembled here rather than in Flush because a PDF's cross
// reference table records the byte offset of every object, so nothing can be
// written until the last object exists.
func (t *target) Close() error {
	var err error
	if t.b != nil && t.bw != nil {
		if t.b.rootRef == 0 {
			err = errors.New("refract/backend/pdf: Close before Flush")
		} else {
			err = t.b.doc.writeTo(t.bw, t.b.rootRef, t.b.infoRef)
		}
	}
	if t.bw != nil {
		if ferr := t.bw.Flush(); err == nil {
			err = ferr
		}
	}
	if t.w != nil {
		if cerr := t.w.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
