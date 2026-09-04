package geom

import (
	"fmt"

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
// resolved a colour column through a colour scale.
func (c config) colorGuide(s series, err error) (ColorGuide, bool) {
	if err != nil || !c.varying(s) {
		return ColorGuide{}, false
	}
	label := c.label
	if label == "" {
		label = c.colorCol
	}
	return ColorGuide{Label: label, Scale: c.colorScale}, true
}
