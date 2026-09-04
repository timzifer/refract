package render

import (
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/mathtext"
)

// mathBackend is a backend that typesets the labels passing through it.
//
// Every label a chart draws — the title, an axis title, a tick, a legend
// entry, a facet strip, a geom's own note — reaches the backend as a
// [ir.TextRun], and every label a chart *measures* reaches it as one too,
// because layout sizes its gutters from the same call. So wrapping the backend
// is the whole integration: notation works in every label, including the ones
// a geom draws and the ones a third-party geom will draw, and no call site
// knows this exists.
//
// It wraps rather than replaces, and forwards everything it does not handle.
// Text and Measure are the two methods it changes, and it changes them
// consistently: a label is measured by laying it out and drawn by drawing that
// layout, so the margin a title is given is the width the title turns out to
// have.
//
// A chart with no typesetter is not wrapped at all, which is what keeps the
// ordinary path exactly as expensive as it was.
type mathBackend struct {
	ir.Backend
	ts mathtext.Typesetter
}

func withMath(b ir.Backend, ts mathtext.Typesetter) ir.Backend {
	if ts == nil {
		return b
	}
	return mathBackend{Backend: b, ts: ts}
}

func (m mathBackend) Measure(run ir.TextRun) ir.TextMetrics {
	if l, ok := m.ts.Typeset(run.Text, run.Font, m.Backend); ok {
		return l.Metrics()
	}
	return m.Backend.Measure(run)
}

func (m mathBackend) Text(run ir.TextRun) {
	l, ok := m.ts.Typeset(run.Text, run.Font, m.Backend)
	if !ok {
		m.Backend.Text(run)
		return
	}
	mathtext.Draw(m.Backend, l, run.At, alignOffset(run, l), run.Color, run.Rotation)
}

// alignOffset turns a run's alignment into where the layout's own origin sits
// relative to the anchor.
//
// A [ir.TextRun] is anchored by its alignment about At; a [mathtext.Layout] is
// positioned from the baseline start of its first piece. This is the one
// conversion between them, and it is the same arithmetic every backend does
// for a text run — which is why it is here rather than in every backend, and
// why the typesetter reports ascent and descent at all.
func alignOffset(run ir.TextRun, l mathtext.Layout) ir.Point {
	var at ir.Point
	switch run.H {
	case ir.AlignCenter:
		at.X -= l.Width / 2
	case ir.AlignEnd:
		at.X -= l.Width
	}
	switch run.V {
	case ir.AlignTop:
		at.Y += l.Ascent
	case ir.AlignMiddle:
		at.Y += (l.Ascent - l.Descent) / 2
	case ir.AlignBottom:
		at.Y -= l.Descent
	}
	return at
}
