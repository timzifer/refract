package refract

import (
	"github.com/timzifer/refract/interact"
	"github.com/timzifer/refract/scale"
)

// View is where a chart is looking: the domain of every panel axis of the
// frame currently drawn.
//
// It is the zoom and the pan a reader has established, in a form a caller can
// hold on to and put back. [Live.Rebuild] uses it to keep the view across a
// change to the plot, and a caller wanting the same guarantee across something
// wider — swapping a data source, rebuilding a dashboard — reaches for
// [Live.View] and [Live.SetView] directly.
//
// A View is a value: taking one copies the numbers out of the scales, so it
// stays true after the scales move on. It describes the chart it was taken
// from and nothing else — put one back into a chart with a different number of
// panels and it is ignored, because a domain from the third panel of a grid of
// nine means nothing in a chart with one.
type View struct {
	panels []panelView
}

// panelView is the four axes a panel can have. Each carries whether the axis
// was there at all: a chart with one vertical axis has no second one, and a
// zero domain is a legitimate answer rather than a missing one.
type panelView struct {
	axes [4]axisView
}

type axisView struct {
	min, max float64
	ok       bool
}

// Empty reports whether the view describes nothing — which is what [Live.View]
// returns from a chart that has not been drawn, because the panels are known
// from the render rather than from the plot.
func (v View) Empty() bool { return len(v.panels) == 0 }

// Panels reports how many panels the view describes.
func (v View) Panels() int { return len(v.panels) }

// View reports where the chart is currently looking.
//
// It is taken from the frame last drawn: the panels and their scales are what
// the render announced, so a View from a chart that has not been drawn is
// empty. Every axis is read, including the secondary ones, because a view that
// restored one direction and not the other would slide two series apart —
// which is the same reason [Live.Wheel] moves all four.
func (l *Live) View() View {
	panels := l.idx.Panels()
	if len(panels) == 0 {
		return View{}
	}
	v := View{panels: make([]panelView, len(panels))}
	for i, p := range panels {
		for j, s := range axesOf(p) {
			v.panels[i].axes[j] = readAxis(s)
		}
	}
	return v
}

// SetView puts a view back and redraws.
//
// It is the counterpart of [Live.View] and the two are meant to bracket
// something that would otherwise lose the reader's place. A view of a
// different shape — taken from a chart with a different number of panels — is
// ignored rather than applied partly, and an empty view does nothing; both
// return nil and redraw, because "the view did not change" is not a failure.
//
// An axis that cannot be pinned is left alone. [scale.Zoomer] is what a scale
// implements to have its domain set, and a scale that does not is a scale that
// does not zoom either — so there was nothing for a reader to establish and
// nothing to put back.
func (l *Live) SetView(v View) error {
	if len(v.panels) != len(l.idx.Panels()) {
		return l.Draw()
	}
	l.restore(v)
	return l.Draw()
}

// axesOf lists a panel's four axes in a fixed order, so that a View written by
// one call is read back by another in the same places. A nil entry is an axis
// the panel does not have.
func axesOf(p interact.Panel) [4]scale.Scale {
	return [4]scale.Scale{p.X, p.Y, p.Y2, p.X2}
}

func readAxis(s scale.Scale) axisView {
	if s == nil {
		return axisView{}
	}
	// Only a scale that can be pinned is worth reading: a domain nothing can
	// put back is a number with no use.
	if _, ok := s.(scale.Zoomer); !ok {
		return axisView{}
	}
	min, max := s.Domain()
	return axisView{min: min, max: max, ok: true}
}

func writeAxis(s scale.Scale, a axisView) {
	if !a.ok || s == nil {
		return
	}
	if z, ok := s.(scale.Zoomer); ok {
		z.SetDomain(a.min, a.max)
	}
}
