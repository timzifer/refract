package spec

import (
	"fmt"

	"github.com/timzifer/refract/coord"
)

// encodeCoord writes the chart's coordinate system down, or nil for the
// Cartesian one.
//
// Nil rather than `{"type": "cartesian"}`: the absent coord and the Cartesian
// one draw the same chart, and writing the field into every document refract
// has ever produced would be noise in every one of them. It also keeps a v0.7
// document and the v0.8 document of the same chart byte-identical.
func encodeCoord(c coord.Coord) (*Coord, error) {
	if c == nil {
		return nil, nil
	}
	d, ok := coord.Describe(c)
	if !ok {
		return nil, fmt.Errorf("refract/spec: coord %T cannot describe itself", c)
	}
	if d.Default() {
		return nil, nil
	}
	out := &Coord{
		Type:             string(d.Type),
		Hole:             d.Hole,
		Radius:           d.Radius,
		Start:            d.Start,
		Counterclockwise: d.Counterclockwise,
	}
	if d.Theta == coord.FromY {
		out.Theta = ThetaY
	} else {
		out.Theta = ThetaX
	}
	if d.Sweep != 0 {
		sweep := d.Sweep
		out.Sweep = &sweep
	}
	if d.Chord {
		out.Edge = EdgeChord
	}
	return out, nil
}

// decodeCoord reads a coordinate system back. A document with no `coord` is a
// Cartesian chart, and gets no coord at all rather than an explicit one — the
// two draw the same thing, and leaving it nil keeps the round trip exact.
func decodeCoord(c *Coord) (coord.Coord, error) {
	if c == nil {
		return nil, nil
	}
	d := coord.Desc{
		Type:             coord.Type(c.Type),
		Hole:             c.Hole,
		Radius:           c.Radius,
		Start:            c.Start,
		Sweep:            coord.FullTurn,
		Counterclockwise: c.Counterclockwise,
	}
	switch c.Theta {
	case "", ThetaX:
		d.Theta = coord.FromX
	case ThetaY:
		d.Theta = coord.FromY
	default:
		return nil, fmt.Errorf("refract/spec: unknown coord theta %q", c.Theta)
	}
	if c.Sweep != nil {
		d.Sweep = *c.Sweep
	}
	switch c.Edge {
	case "", EdgeArc:
	case EdgeChord:
		d.Chord = true
	default:
		return nil, fmt.Errorf("refract/spec: unknown coord edge %q", c.Edge)
	}
	out, err := coord.FromDesc(d)
	if err != nil {
		return nil, err
	}
	if d.Default() {
		// A document that spelled out the default gets the default: a nil
		// coord and a Cartesian one are the same chart, and handing back nil
		// keeps a re-encode from growing a field the input did not have.
		return nil, nil
	}
	return out, nil
}
