package coord

import (
	"fmt"
	"math"
)

// Type names a coordinate system. It is the word a written-down chart carries
// in place of the constructor that built the coord.
type Type string

// The coord types.
const (
	TypeCartesian Type = "cartesian"
	TypePolar     Type = "polar"
)

// Desc is a coord reduced to what configures it.
//
// It is the bargain [github.com/timzifer/refract/scale.Desc] makes, for the
// same reason: a Coord is an interface over an unexported type, which is right
// for mapping positions and useless for writing one down. Nothing about a
// coord is a Go function, so unlike a scale, nothing here is lost.
type Desc struct {
	// Type is which coord this is.
	Type Type

	// Theta is the axis a polar coord sweeps around the circle.
	Theta Axis
	// Hole is the inner radius as a fraction of the outer one, zero for a
	// coord with no hole.
	Hole float64
	// Radius is how much of the panel's shorter half-side the circle fills.
	Radius float64
	// Start is where the angular scale begins, in radians clockwise from
	// twelve o'clock, and Sweep how much of the circle it covers.
	Start, Sweep float64
	// Counterclockwise reverses the direction the angular scale runs in.
	Counterclockwise bool
	// Chord reports a coord drawing an edge between two marks as the straight
	// line between them rather than as an arc.
	Chord bool
}

// Describe reports c's configuration, or ok == false if c cannot describe
// itself. A third-party coord that does not implement [Describer] still draws;
// it is simply not serializable.
func Describe(c Coord) (Desc, bool) {
	d, ok := c.(Describer)
	if !ok {
		return Desc{}, false
	}
	return d.Describe(), true
}

// FromDesc rebuilds a coord from its description.
func FromDesc(d Desc) (Coord, error) {
	switch d.Type {
	case "", TypeCartesian:
		return Cartesian(), nil
	case TypePolar:
		opts := []Option{Theta(d.Theta), Hole(d.Hole), Radius(d.Radius),
			Start(d.Start), Sweep(d.Sweep), Counterclockwise(d.Counterclockwise)}
		if d.Chord {
			opts = append(opts, Chord())
		}
		return Polar(opts...), nil
	default:
		return nil, fmt.Errorf("refract/coord: unknown coord type %q", d.Type)
	}
}

func (p *polar) Describe() Desc {
	return Desc{
		Type:             TypePolar,
		Theta:            p.theta,
		Hole:             p.hole,
		Radius:           p.radius,
		Start:            p.start,
		Sweep:            p.sweep,
		Counterclockwise: p.ccw,
		Chord:            p.edge == chordEdges,
	}
}

// Default reports whether d describes the coord a chart has when nobody chose
// one. A document does not carry a field for that: the absent coord and the
// Cartesian one draw the same chart, and writing `"coord": {"type":
// "cartesian"}` into every spec refract has ever produced would be noise.
func (d Desc) Default() bool { return d.Type == "" || d.Type == TypeCartesian }

// FullTurn is the default sweep: a whole circle.
const FullTurn = 2 * math.Pi
