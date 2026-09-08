package geom

import "github.com/timzifer/refract/data"

// Sourced is implemented by a layer that can hand back the data it holds.
//
// It is the first half of [Faceter], spelled on its own because two different
// callers want it for two different reasons and only one of them can also
// split a layer. A facet needs both — read the column, then cut the rows. A
// caller resolving a row's identity needs only the read, and asking it to
// satisfy an interface with a Subset it will never call would be asking for
// the wrong thing.
//
// Every Faceter satisfies it, so nothing had to be implemented to make this
// true and no geom changed.
type Sourced interface {
	// Source returns the layer's data.
	Source() data.Source
}

// SourceOf returns the data behind a layer, or ok == false for one that does
// not hold any — an annotation takes values rather than columns — or one
// defined outside this package that does not report it.
func SourceOf(g Geom) (data.Source, bool) {
	s, ok := g.(Sourced)
	if !ok {
		return nil, false
	}
	src := s.Source()
	return src, src != nil
}

// KeyOf returns the column a layer identifies its rows by, or "" for a layer
// that named none.
//
// It reads the layer's [Desc], so a third-party mark that describes itself —
// which is what [Register] and the JSON round trip already require — answers
// this without implementing anything further.
func KeyOf(g Geom) string {
	d, ok := Describe(g)
	if !ok {
		return ""
	}
	return d.Key
}
