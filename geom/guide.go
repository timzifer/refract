package geom

import (
	"fmt"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// ColorGuide is the guide a layer needs when its colour comes from a
// continuous scale: a colourbar, not a legend swatch.
//
// A single swatch cannot represent a ramp — it would have to pick one colour
// out of a continuum and label it with a column name — so a layer using
// [ColorBy] contributes one of these instead of a [LegendEntry].
type ColorGuide struct {
	// Label titles the bar. It defaults to the name of the coloured column.
	Label string
	// Scale is the trained colour scale the bar shows.
	Scale scale.ColorScale
}

// Key identifies a guide by what it looks like rather than by which scale
// object produced it.
//
// Two layers sharing one colour scale must produce one colourbar, and so must
// two layers whose separate scales agree about the label, the domain and the
// ramp — those would be drawn identically, so showing both would only take
// room from the chart. Comparing the scales themselves is not available: a
// ColorScale is an interface, and an implementation is free to be a type that
// == panics on.
func (g ColorGuide) Key() string {
	lo, hi := g.Scale.Domain()
	mid := lo + (hi-lo)/2
	return fmt.Sprintf("%s|%v|%v|%v|%v|%v",
		g.Label, lo, hi, g.Scale.Color(lo), g.Scale.Color(mid), g.Scale.Color(hi))
}

// Guided is implemented by a layer that paints from a continuous colour scale.
//
// It is an optional interface rather than a method on [Geom]: a layer that
// colours itself one colour has nothing to say here, and every third-party
// geom would otherwise have to write a stub returning false.
type Guided interface {
	// ColorGuide returns the colour guide this layer contributes, or
	// ok == false if it has none.
	ColorGuide() (ColorGuide, bool)
}

// colorGuide is the shared implementation: a layer has a guide exactly when it
// resolved a colour column through a *continuous* colour scale.
//
// A discrete scale gets legend entries instead — see [Legender]. Which guide a
// layer contributes follows from the kind of scale it was handed, which is
// exactly the seam ADR 0020 draws: a colourbar over eight categories would be
// a ramp through colours nothing is painted with.
func (c config) colorGuide(s series, err error) (ColorGuide, bool) {
	if err != nil || !c.varying(s) {
		return ColorGuide{}, false
	}
	if _, discrete := scale.Discrete(c.colorScale); discrete {
		return ColorGuide{}, false
	}
	label := c.label
	if label == "" {
		label = c.colorCol
	}
	return ColorGuide{Label: label, Scale: c.colorScale}, true
}

// SizeGuide is the guide a layer needs when its marks take their size from a
// column: a ladder of sample marks with the values they stand for.
//
// It is the third guide kind, and it is a third kind rather than a variation on
// a legend because what it shows is neither a swatch nor a ramp — it is the
// mark itself at several sizes, which is the only way a reader can measure one.
// A single swatch cannot represent a continuum, and a ramp has no size.
type SizeGuide struct {
	// Label titles the key. It defaults to the name of the sized column.
	Label string
	// Scale is the trained size scale the key samples.
	Scale scale.SizeScale
	// Color is what the samples are drawn in, so that a key beside a chart of
	// blue bubbles is a key of blue bubbles.
	//
	// It is the transparent zero when the layer takes its colour from the
	// palette, because which palette entry that is depends on where the layer
	// sits in the chart and a guide is collected before there is a frame to ask.
	// The renderer fills it in; see render's guide collection.
	Color ir.Color
}

// Key identifies a guide by what it looks like, exactly as [ColorGuide.Key]
// does and for the same reason: two layers sharing one size scale must produce
// one key, and comparing the scales themselves is not available through an
// interface.
//
// The colour is deliberately not part of it. A size key answers "how big is how
// much", and two layers that read one column through one scale are answering it
// identically whatever colour they are painted in — so they get one key, drawn
// in the first of their colours, rather than two keys saying the same thing.
func (g SizeGuide) Key() string {
	lo, hi := g.Scale.Domain()
	return fmt.Sprintf("%s|%v|%v|%v|%v", g.Label, lo, hi, g.Scale.Size(lo), g.Scale.Size(hi))
}

// Sized is implemented by a layer whose marks take their size from a column.
//
// It is an optional interface for the reason [Guided] is: a layer whose marks
// are all one size has nothing to say here, and every third-party geom would
// otherwise have to write a stub returning false.
type Sized interface {
	// SizeGuide returns the size guide this layer contributes, or ok == false
	// if it has none.
	SizeGuide() (SizeGuide, bool)
}

// sizeGuide is the shared implementation: a layer has one exactly when it
// resolved a size column through a size scale.
func (c config) sizeGuide(s series, err error) (SizeGuide, bool) {
	if err != nil || c.sizeScale == nil || s.sz == nil {
		return SizeGuide{}, false
	}
	label := c.label
	if label == "" {
		label = c.sizeCol
	}
	g := SizeGuide{Label: label, Scale: c.sizeScale}
	if c.color != nil {
		g.Color = *c.color
	}
	return g, true
}
