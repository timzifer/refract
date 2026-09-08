package refract

// Layer visibility: turning a series off without taking it out of the chart.

// Hide turns a layer off, or back on, and redraws.
//
// A hidden layer is not drawn. It still trains its scales, and it still appears
// in the legend — dimmed, so that a reader can see what they have put away and
// bring it back.
//
// The axes deliberately do not move. A toggle is a reading aid — let me see
// this one without that one on top — and an axis that rescaled every time one
// was clicked would make the two readings incomparable, which is the thing the
// toggle was for. A caller who wants the axes to follow what is left is making
// a different statement about the chart, and makes it with [Plot.SetLayers] and
// [Live.Rebuild].
//
// The index is redrawn with it, so a hidden layer's marks are no longer under
// the pointer: a tooltip for something invisible would be a tooltip for
// nothing.
//
// A layer index outside the chart's layers is ignored and redraws nothing.
func (l *Live) Hide(layer int, hide bool) error {
	if layer < 0 || layer >= len(l.p.layers) {
		return nil
	}
	for len(l.chart.Hidden) <= layer {
		l.chart.Hidden = append(l.chart.Hidden, false)
	}
	if l.chart.Hidden[layer] == hide {
		return nil
	}
	l.chart.Hidden[layer] = hide
	return l.Draw()
}

// Toggle turns a layer off if it is on, and on if it is off, and redraws.
//
// It is what a click on a legend row calls:
//
//	p.On(refract.Click, func(ev refract.Event) {
//		if ev.Hit.Kind == refract.LegendRow {
//			live.Toggle(ev.Hit.Layer)
//		}
//	})
//
// That the wiring is four lines in the caller rather than a mode on the chart
// is deliberate, and is the same answer this library gives everywhere a
// pointer means something: refract says what was clicked, and what it means is
// the program's. A legend that always toggled would be wrong for a chart whose
// legend selects rather than filters, or one where clicking a series should
// open something.
//
// A [Colorbar] or a [SizeKey] hit has no Toggle: neither stands for a layer,
// so what a click on one means is a range of values or a magnitude rather than
// a series to put away. See [Hit.Lo], [Hit.Hi] and [Hit.Value].
func (l *Live) Toggle(layer int) error { return l.Hide(layer, !l.IsHidden(layer)) }

// IsHidden reports whether a layer is currently turned off.
func (l *Live) IsHidden(layer int) bool {
	return layer >= 0 && layer < len(l.chart.Hidden) && l.chart.Hidden[layer]
}

// ShowAll turns every layer back on and redraws. It is what a "reset" control
// calls, and what [Input.DoubleClick] would call if hiding were a view state —
// it is not, because a hidden series is a statement about what the reader wants
// to see rather than about where they are looking.
func (l *Live) ShowAll() error {
	changed := false
	for i := range l.chart.Hidden {
		if l.chart.Hidden[i] {
			l.chart.Hidden[i], changed = false, true
		}
	}
	if !changed {
		return nil
	}
	return l.Draw()
}
