// Package scale maps data values onto visual positions and generates the ticks
// that label them.
//
// A Scale owns two things: the domain→range mapping, and the choice of tick
// positions and labels for that domain. Keeping both in one place is what lets
// a time axis label itself in calendar units while a linear axis labels itself
// with round numbers, without either the geom or the layout knowing which is
// which.
package scale

import "math"

// Tick is one labelled position on an axis.
type Tick struct {
	// Value is the tick's position in data space.
	Value float64
	// Pos is the tick's position in device space, already mapped.
	Pos float32
	// Label is the formatted text for the tick. An empty label means a tick
	// mark and grid line are drawn but nothing is written.
	Label string
	// Minor marks a tick that subdivides the axis without a label.
	Minor bool
}

// Scale maps data values onto a device-space range.
//
// A scale is trained on data, given a device range, and then queried. The
// order matters: Map and Ticks are only meaningful once both the domain and
// the range are set.
type Scale interface {
	// Train extends the scale's data domain to include vs. Values that are
	// NaN or infinite are ignored. Calling Train repeatedly accumulates.
	Train(vs ...float64)

	// SetRange sets the device-space output interval. lo may be greater than
	// hi, which is how a Y axis is flipped so that larger values are higher on
	// screen.
	SetRange(lo, hi float32)

	// Domain reports the current data domain, after any nicing.
	Domain() (min, max float64)

	// Map converts a data value to a device position. Values outside the
	// domain map outside the range; clipping is the caller's business.
	Map(v float64) float32

	// Invert converts a device position back to a data value. It is the
	// inverse of Map over the whole real line, not just the range.
	Invert(pos float32) float64

	// Ticks returns tick positions and labels, aiming for about want ticks.
	// The result is ordered ascending by Value.
	Ticks(want int) []Tick
}

// domainRange is the state every scale in this package shares.
type domainRange struct {
	dmin, dmax float64
	trained    bool
	rlo, rhi   float32
	rset       bool
}

func (d *domainRange) Train(vs ...float64) {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if !d.trained {
			d.dmin, d.dmax, d.trained = v, v, true
			continue
		}
		d.dmin = math.Min(d.dmin, v)
		d.dmax = math.Max(d.dmax, v)
	}
}

func (d *domainRange) SetRange(lo, hi float32) { d.rlo, d.rhi, d.rset = lo, hi, true }

// span returns the domain, substituting a usable interval when the scale was
// never trained or saw constant data. Every scale needs this: an axis over one
// repeated value must still render.
func (d *domainRange) span() (float64, float64) {
	if !d.trained {
		return 0, 1
	}
	if d.dmin == d.dmax {
		pad := math.Abs(d.dmin) * 0.05
		if pad == 0 {
			pad = 0.5
		}
		return d.dmin - pad, d.dmax + pad
	}
	return d.dmin, d.dmax
}

func (d *domainRange) rangeOf() (float32, float32) {
	if !d.rset {
		return 0, 1
	}
	return d.rlo, d.rhi
}
