package pdf

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/timzifer/refract/internal/sfnt"
	"github.com/timzifer/refract/ir"
)

// Option configures a PDF target.
type Option func(*options)

type options struct {
	title    string
	author   string
	subject  string
	compress bool

	// fonts are the faces the document embeds, or nil for the base-14
	// Helvetica every reader already has. fontErr carries a parse failure to
	// Open, because an Option cannot return one.
	fonts   *fontSet
	fontErr error
}

// fontSet is the three faces a chart draws in. A nil bold or italic falls back
// to the regular one, which is what a caller supplying a single face gets and
// is better than a bold label drawn in a font that is not there.
type fontSet struct {
	regular, bold, italic *sfnt.Font
}

func (s *fontSet) face(weight int, italic bool) *sfnt.Font {
	if italic && s.italic != nil {
		return s.italic
	}
	if weight >= 600 && s.bold != nil {
		return s.bold
	}
	return s.regular
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

// WithFont embeds the given TrueType or OpenType faces in the document and
// draws every label with them, instead of naming the base-14 Helvetica that
// every reader already has.
//
// It is what a chart labelled in anything but Latin-1 needs. Without it the
// output carries no font at all — which is the right default, because it makes
// a chart of Latin text a few kilobytes and universally readable — and a rune
// outside WinAnsi becomes "?": no Greek, no Cyrillic, no Hebrew, no Thai, no
// CJK. With it, the text is written as glyph ids through an Identity-H
// encoding and the font travels with the document, so the label reads the same
// on a machine that has never heard of the typeface.
//
//	ttf, err := os.ReadFile("NotoSansJP-Regular.ttf")
//	// …
//	p.Render(pdf.File("chart.pdf", pdf.WithFont(ttf, nil, nil)))
//
// bold and italic may be nil, and a face that is absent falls back to the
// regular one — a bold label drawn in the regular weight is a better answer
// than one drawn in a font the document does not carry.
//
// # What is embedded
//
// A TrueType font is **subset**: only the glyphs the document actually draws
// are written out, so a chart with twenty Japanese labels carries twenty
// glyphs rather than a twenty-megabyte font. A CFF-flavoured OpenType font —
// usually an `.otf` — is embedded **whole**, because cutting up charstrings is
// a second outline interpreter this package does not have; the document is
// correct and larger, and a `.ttf` build of the same family avoids it.
//
// A `ToUnicode` map is always written, so the text in the document can still
// be selected, copied, searched and read aloud. A picture of a label is what
// the accessibility work exists to avoid, and it would be exactly what an
// embedded font without one produced.
//
// A parse failure surfaces when the target is opened rather than here, because
// an Option cannot return an error.
func WithFont(regular, bold, italic []byte) Option {
	return func(o *options) {
		reg, err := sfnt.Parse(regular)
		if err != nil {
			o.fontErr = fmt.Errorf("refract/backend/pdf: parsing the supplied regular font: %w", err)
			return
		}
		if !reg.HasUnicodeMap() {
			o.fontErr = errors.New("refract/backend/pdf: the supplied regular font has no Unicode character map, so no label could be mapped to a glyph")
			return
		}
		set := &fontSet{regular: reg}
		if set.bold, err = optionalFont(bold, "bold"); err != nil {
			o.fontErr = err
			return
		}
		if set.italic, err = optionalFont(italic, "italic"); err != nil {
			o.fontErr = err
			return
		}
		o.fonts = set
	}
}

func optionalFont(b []byte, which string) (*sfnt.Font, error) {
	if len(b) == 0 {
		return nil, nil
	}
	f, err := sfnt.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("refract/backend/pdf: parsing the supplied %s font: %w", which, err)
	}
	return f, nil
}

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
	if t.opts.fontErr != nil {
		return nil, t.opts.fontErr
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
