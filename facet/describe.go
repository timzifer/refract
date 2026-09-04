package facet

// Desc is a facet specification reduced to what configures it, so that a
// faceted chart can be written down and read back.
type Desc struct {
	// Row and Col name the columns split on. A wrapped facet leaves Row empty
	// and flows its panels; a grid crosses the two.
	Row, Col string
	// Wrap reports a wrapped facet rather than a grid.
	Wrap bool
	// Columns caps how many panels a wrap puts in a row. Zero means the
	// derived, roughly square default.
	Columns int
	// FreeX and FreeY give each panel its own axis.
	FreeX, FreeY bool
}

// Describe returns the spec's configuration.
func (s *Spec) Describe() Desc {
	if s == nil {
		return Desc{}
	}
	return Desc{
		Row:     s.rowCol,
		Col:     s.col,
		Wrap:    s.wrap,
		Columns: s.columns,
		FreeX:   s.freeX,
		FreeY:   s.freeY,
	}
}

// FromDesc builds the facet d describes. It returns nil for a Desc naming no
// column, which is how "this chart is not faceted" is written down.
func FromDesc(d Desc) *Spec {
	if d.Col == "" && d.Row == "" {
		return nil
	}
	var opts []Option
	if d.Columns > 0 {
		opts = append(opts, Columns(d.Columns))
	}
	if d.FreeX {
		opts = append(opts, FreeX())
	}
	if d.FreeY {
		opts = append(opts, FreeY())
	}
	if d.Wrap || d.Row == "" {
		return Wrap(d.Col, opts...)
	}
	return Grid(d.Row, d.Col, opts...)
}
