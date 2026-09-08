package scale

import (
	"math"
	"sort"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
)

// Named returns a discrete colour scale whose categories are coloured by name
// rather than by the order they turn up in.
//
// It is [Qualitative]'s sibling and it exists because first appearance is the
// wrong rule for some categories. A machine state, a severity, a pass/fail —
// these carry a colour before any data does: a reader knows that a fault is
// red without consulting the legend, and a chart that painted it green on a
// window where no fault came first would be worse than one with no colour at
// all. Qualitative hands out palette entries in the order the rows arrive, so
// the colour of a state there depends on which slice of the stream is on
// screen. That is right for series nobody has an opinion about and wrong for
// these.
//
// The named categories are listed first and in sorted order, which is also the
// order the legend lists them in. Sorted rather than as-written because a Go
// map has no order to preserve, and ADR 0012 does not allow one that depends
// on map iteration: a parallel render must be byte-identical to a serial one.
//
//	scale.Named(map[string]ir.Color{
//	    "RUN":   palette.Green,
//	    "FAULT": palette.Red,
//	})
//
// A label the map does not mention is still drawn. It takes the next colour
// from the fallback palette, in order of first appearance, and is appended to
// [DiscreteColorScale.Labels] after the named ones — see [ColorFallback]. It
// is not left undefined because a status code nobody enumerated would then be
// a hole in the mark, and a hole in a line reads as a gap in the measurements
// rather than as a category the chart was not told about.
//
// A nil or empty map is legal and makes this a [Qualitative] scale with extra
// steps, which is what a configuration file that named no colours should draw.
// Of the colour options [ColorUndefined], [ColorReverse] and [ColorFallback]
// mean something here; the rest are accepted and ignored, exactly as they are
// on a qualitative scale.
func Named(colors map[string]ir.Color, opts ...ColorOption) DiscreteColorScale {
	var cfg colorScale
	cfg.undef = ir.Transparent
	for _, o := range opts {
		o(&cfg)
	}
	fallback := cfg.fallback
	if len(fallback) == 0 {
		fallback = palette.Default
	}
	n := &named{
		fallback: fallback,
		undef:    cfg.undef,
		reverse:  cfg.reverse,
		named:    make(map[string]ir.Color, len(colors)),
		at:       make(map[string]int, len(colors)),
	}
	for label := range colors {
		n.labels = append(n.labels, label)
	}
	sort.Strings(n.labels)
	for i, label := range n.labels {
		n.named[label] = colors[label]
		n.at[label] = i
	}
	n.fixed = len(n.labels)
	return n
}

type named struct {
	fallback palette.Qualitative
	undef    ir.Color
	reverse  bool

	// named is the colour each enumerated label was given. It is read by
	// label rather than by index so that a label registered later — one the
	// map did not mention — cannot collide with an enumerated one.
	named map[string]ir.Color
	// fixed is how many of labels came from the map. Everything from that
	// index on was discovered in the data and is coloured from the fallback
	// palette, counting from zero at fixed.
	fixed int

	labels []string
	at     map[string]int
}

// Train is a no-op for the same reason [Qualitative]'s is: the domain of a
// discrete scale is the set of labels it has been shown, and a label arrives
// through Encode rather than through a number.
func (n *named) Train(...float64) {}

func (n *named) Domain() (float64, float64) {
	if len(n.labels) == 0 {
		return 0, 0
	}
	return 0, float64(len(n.labels) - 1)
}

func (n *named) Color(v float64) ir.Color {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return n.undef
	}
	return n.colorAt(int(v))
}

func (n *named) Encode(label string) float64 { return float64(n.index(label)) }

func (n *named) ColorOf(label string) ir.Color { return n.colorAt(n.index(label)) }

// Labels returns the enumerated categories in sorted order, followed by the
// ones discovered in the data in order of first sight. The slice is the
// scale's own and must not be modified, exactly as [Qualitative]'s is.
func (n *named) Labels() []string { return n.labels }

func (n *named) index(label string) int {
	if i, ok := n.at[label]; ok {
		return i
	}
	i := len(n.labels)
	n.labels = append(n.labels, label)
	n.at[label] = i
	return i
}

// colorAt answers an enumerated label from the map and a discovered one from
// the fallback palette. ColorReverse runs the fallback the other way round,
// which is what it means everywhere else; it does not touch a colour the
// caller named, because reversing an explicit choice would be overruling it.
func (n *named) colorAt(i int) ir.Color {
	if i < 0 || i >= len(n.labels) {
		return n.undef
	}
	if i < n.fixed {
		return n.named[n.labels[i]]
	}
	j := i - n.fixed
	if n.reverse {
		return n.fallback.At(len(n.fallback) - 1 - j)
	}
	return n.fallback.At(j)
}

// DescribeColor writes the scale down. Only the enumerated labels are written:
// a label the data happened to contain is data rather than configuration, and
// a document that pinned it would stop the next render discovering it — the
// same line [ColorDesc] already draws for a classed scale's derived breaks.
func (n *named) DescribeColor() ColorDesc {
	d := ColorDesc{Kind: KindNamed, Reverse: n.reverse, Undefined: n.undef}
	d.Labels = append([]string(nil), n.labels[:n.fixed]...)
	d.Colors = make(palette.Ramp, 0, n.fixed)
	for _, label := range d.Labels {
		d.Colors = append(d.Colors, n.named[label])
	}
	if name, ok := palette.QualitativeName(n.fallback); ok {
		d.Ramp = name
	} else {
		d.Fallback = append(palette.Ramp(nil), n.fallback...)
	}
	return d
}

var (
	_ DiscreteColorScale = (*named)(nil)
	_ ColorDescriber     = (*named)(nil)
)
