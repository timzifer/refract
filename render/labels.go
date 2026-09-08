package render

import (
	"sync"

	"github.com/timzifer/refract/internal/layout"
	"github.com/timzifer/refract/ir"
)

type labelPlacer struct {
	layout  layout.Labels
	measure ir.Measurer
}

func (p *labelPlacer) PlaceLabel(run ir.TextRun, move bool) (ir.Point, bool) {
	return p.layout.Place(run, p.measure.Measure(run), move)
}

var labelPool sync.Pool

func acquireLabels(area ir.Rect, m ir.Measurer) *labelPlacer {
	p, _ := labelPool.Get().(*labelPlacer)
	if p == nil {
		p = new(labelPlacer)
	}
	p.layout.Reset(area)
	p.measure = m
	return p
}

func releaseLabels(p *labelPlacer) {
	p.measure = nil
	p.layout.Reset(ir.Rect{})
	labelPool.Put(p)
}
