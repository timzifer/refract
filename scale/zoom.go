package scale

// Zoomer is implemented by a scale whose domain can be set outright, which is
// what pan and zoom do.
//
// It is an optional interface, and the third of its family: [Cloner] hands
// back an untrained copy for a free facet axis, [Snapshotter] an exact copy
// for another goroutine, and Zoomer changes the domain of the scale in place.
// A scale that does not implement it is simply not pannable — an ordinal axis
// is the honest example, because half a category is not a view of anything.
//
// # Pinning
//
// SetDomain pins the domain the way [Domain] does at construction: training
// stops moving it, and a linear scale stops nicing it. Both are what an
// interactive view needs — a chart whose axis snapped to round numbers after
// every wheel notch would not follow the pointer, and one that retrained on
// the next frame would undo the zoom the reader just asked for. [Autoscale]
// releases the pin.
type Zoomer interface {
	// SetDomain pins the data domain to [min, max]. A scale that cannot place
	// part of that interval — a log scale given a negative bound — clamps to
	// what it can place rather than refusing, because the caller is a pointer
	// drag rather than a programmer.
	SetDomain(min, max float64)

	// Autoscale releases a pinned domain and forgets what was trained into it,
	// so the next render establishes the domain from the data again.
	//
	// It releases a domain fixed at construction too. A scale built with a
	// fixed domain and then autoscaled is a scale that was asked, twice, to do
	// two different things; the later call wins.
	Autoscale()
}

// The implementations below share a shape: pin, and clear whatever cache the
// scale keeps of a domain it derived.

func (l *linear) SetDomain(min, max float64) {
	l.dmin, l.dmax = order(min, max)
	l.trained, l.fixed, l.pinned = true, true, true
	l.hasNiced = false
}

func (l *linear) Autoscale() {
	l.domainRange = domainRange{rlo: l.rlo, rhi: l.rhi, rset: l.rset}
	l.fixed, l.pinned, l.hasNiced = false, false, false
}

func (l *logScale) SetDomain(min, max float64) {
	min, max = order(min, max)
	// A log axis has no position for zero or a negative number, so a drag that
	// walks off the bottom is clamped to the smallest decade it can draw
	// rather than emptying the axis.
	if max <= 0 {
		max = 1
	}
	if min <= 0 {
		min = max / 1e6
	}
	l.dmin, l.dmax = min, max
	l.trained, l.fixed, l.pinned = true, true, true
}

func (l *logScale) Autoscale() {
	l.domainRange = domainRange{rlo: l.rlo, rhi: l.rhi, rset: l.rset}
	l.fixed, l.pinned = false, false
}

func (s *symlogScale) SetDomain(min, max float64) {
	s.dmin, s.dmax = order(min, max)
	s.trained, s.fixed = true, true
}

func (s *symlogScale) Autoscale() {
	s.domainRange = domainRange{rlo: s.rlo, rhi: s.rhi, rset: s.rset}
	s.fixed = false
}

func (s *timeScale) SetDomain(min, max float64) {
	s.dmin, s.dmax = order(min, max)
	s.trained, s.fixed = true, true
}

func (s *timeScale) Autoscale() {
	s.domainRange = domainRange{rlo: s.rlo, rhi: s.rhi, rset: s.rset}
	s.fixed = false
}

// order returns the pair smallest first. A drag that crosses itself, or one on
// an inverted axis, arrives here backwards.
func order(a, b float64) (float64, float64) {
	if a > b {
		return b, a
	}
	return a, b
}
