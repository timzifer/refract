package geom

import "github.com/timzifer/refract/data"

// Faceter is implemented by a layer that can be split into panels.
//
// Faceting is a data operation, and a geom is the only thing that knows which
// data it holds — so the split happens here rather than in the facet package
// reaching into a layer. A layer that does not implement Faceter is drawn
// unchanged in every panel, which is exactly right for an annotation: a
// threshold line belongs on all of them.
type Faceter interface {
	// Source returns the layer's data, so a facet can read the column it
	// splits on.
	Source() data.Source

	// Subset returns a copy of this layer restricted to the given rows. The
	// copy shares its configuration with the original, including any colour
	// scale — which is what makes one colour mean one thing across every
	// panel, and one colourbar enough to say so.
	Subset(rows []int) Geom
}

func (g *lineGeom) Source() data.Source { return g.src }
func (g *lineGeom) Subset(rows []int) Geom {
	return &lineGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *scatterGeom) Source() data.Source { return g.src }
func (g *scatterGeom) Subset(rows []int) Geom {
	return &scatterGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *barGeom) Source() data.Source { return g.src }
func (g *barGeom) Subset(rows []int) Geom {
	return &barGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *areaGeom) Source() data.Source { return g.src }
func (g *areaGeom) Subset(rows []int) Geom {
	return &areaGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *stepGeom) Source() data.Source { return g.src }
func (g *stepGeom) Subset(rows []int) Geom {
	return &stepGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}

func (g *boxGeom) Source() data.Source { return g.src }
func (g *boxGeom) Subset(rows []int) Geom {
	return &boxGeom{src: data.Rows(g.src, rows), cfg: g.cfg}
}
