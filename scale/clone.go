package scale

// Cloner is implemented by a scale that can hand back a fresh copy of itself:
// the same configuration, with nothing trained into it yet.
//
// Faceting needs it for free scales. "Each panel gets its own Y axis" means
// each panel gets its own scale object, configured the way the plot's scale
// was but trained only on that panel's rows — and the only thing that knows
// how a scale was configured is the scale.
//
// It is an optional interface. A scale that does not implement it can still be
// shared across panels, which is the default and by far the common case; only
// a free axis needs a copy, and a facet says so rather than guessing.
type Cloner interface {
	// Clone returns an untrained copy with the same configuration. A fixed
	// domain counts as configuration and is kept.
	Clone() Scale
}

// A fixed domain is set at construction and is part of what the scale is, so
// every Clone below keeps it and clears only what training put there.

func (l *linear) Clone() Scale {
	c := *l
	if !c.fixed {
		c.domainRange = domainRange{}
	} else {
		c.rlo, c.rhi, c.rset = 0, 0, false
	}
	c.hasNiced = false
	return &c
}

func (l *logScale) Clone() Scale {
	c := *l
	if !c.fixed {
		c.domainRange = domainRange{}
	} else {
		c.rlo, c.rhi, c.rset = 0, 0, false
	}
	return &c
}

func (s *symlogScale) Clone() Scale {
	c := *s
	if !c.fixed {
		c.domainRange = domainRange{}
	} else {
		c.rlo, c.rhi, c.rset = 0, 0, false
	}
	return &c
}

func (s *timeScale) Clone() Scale {
	c := *s
	c.domainRange = domainRange{}
	return &c
}

// Clone on an ordinal scale keeps a fixed category list and drops a learned
// one. A fixed list is the axis; a learned one is the data, and the whole
// point of a free axis is that the data differs per panel.
func (o *ordinal) Clone() Scale {
	c := *o
	c.rlo, c.rhi, c.rset = 0, 0, false
	if c.fixed {
		c.labels = append([]string(nil), o.labels...)
		c.index = make(map[string]int, len(o.index))
		for k, v := range o.index {
			c.index[k] = v
		}
		return &c
	}
	c.domainRange = domainRange{}
	c.labels = nil
	c.index = map[string]int{}
	return &c
}
