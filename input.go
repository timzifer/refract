package refract

import "math"

// Input turns a surface's raw pointer input into chart interaction.
//
// [Live] takes deliberate instructions — hover here, zoom about there by this
// much, pan by that. A surface reports something rawer: a button went down, the
// pointer moved, the wheel turned by ninety-six of whatever units this platform
// counts in. The translation between the two is a small state machine — is this
// move a hover or a drag, was that release a click or the end of a pan — and it
// is the same state machine on every surface.
//
// So it lives here, once, rather than being written again in every backend.
// [Live.Bind] is this driving a DOM element; a native window drives it from its
// own event loop; a test drives it directly. A backend consumes IR and must not
// know what a panel or a scale is, which is why none of this is in one.
//
//	in := live.Input()
//	// from the surface's event loop:
//	in.Down(x, y)
//	in.Move(x, y)   // pans, because a button is down
//	in.Up(x, y)     // clicks, because the pointer barely moved
//	in.Wheel(x, y, deltaY)
//
// An Input is not safe for concurrent use, and neither is the Live behind it.
type Input struct {
	l *Live

	down     bool
	dragging bool
	downX    float64
	downY    float64
	lastX    float64
	lastY    float64

	slop float64
}

// DefaultClickSlop is how far the pointer may travel between press and release
// and still count as a click rather than a drag, in device-independent pixels.
//
// It is not zero because a pointer never is: a hand on a mouse moves a pixel or
// two during a click, and a finger on a trackpad more. Three pixels is the
// figure the desktop toolkits settled on, and it is well below the distance a
// deliberate pan covers.
const DefaultClickSlop = 3

// Input returns a driver for this chart's surface input.
//
// Each call returns a fresh driver with no button held. A surface wants one,
// made once and kept for as long as the Live is open.
func (l *Live) Input() *Input { return &Input{l: l, slop: DefaultClickSlop} }

// Live returns the chart this input drives.
func (i *Input) Live() *Live { return i.l }

// ClickSlop sets how far the pointer may move between press and release and
// still count as a click. It returns i so the call can be chained.
func (i *Input) ClickSlop(px float64) *Input {
	if px >= 0 {
		i.slop = px
	}
	return i
}

// Move reports the pointer at a device position.
//
// With no button held it hovers, which fires [Hover] and reports the mark under
// the pointer. With a button held it pans, which drags the data under the
// pointer and redraws — so the value the reader grabbed stays under their
// finger, which is the whole reason a drag pans in the direction it does.
func (i *Input) Move(x, y float64) error {
	if i.down {
		i.dragging = i.dragging || i.beyondSlop(x, y)
		dx, dy := x-i.lastX, y-i.lastY
		i.lastX, i.lastY = x, y
		if !i.dragging {
			// Still inside the slop: this is the wobble of a click, not a pan.
			return nil
		}
		return i.l.PanBy(dx, dy)
	}
	i.lastX, i.lastY = x, y
	i.l.Move(x, y)
	return nil
}

// Down reports a button pressed at a device position. It starts a drag, which
// becomes a pan once the pointer has moved past the click slop.
func (i *Input) Down(x, y float64) error {
	i.down, i.dragging = true, false
	i.downX, i.downY = x, y
	i.lastX, i.lastY = x, y
	return nil
}

// Up reports the button released at a device position.
//
// A release that never moved past the click slop is a click, and fires [Click];
// one that did is the end of a pan and fires nothing, because every step of it
// has already fired [Pan]. That is what keeps a dragged chart from also
// selecting whatever the pointer happened to land on.
//
// A release with no press behind it fires nothing either. A surface has more
// ways to lose a press than to report one — a double click consumed by
// [Input.DoubleClick], a press that started outside the chart, a window that
// took the pointer away — and inventing a click for each of them would put a
// tooltip on screen every time a reader reset the view.
func (i *Input) Up(x, y float64) error {
	if !i.down {
		i.lastX, i.lastY = x, y
		return nil
	}
	dragged := i.dragging || i.beyondSlop(x, y)
	i.down, i.dragging = false, false
	i.lastX, i.lastY = x, y
	if dragged {
		return nil
	}
	i.l.Click(x, y)
	return nil
}

// Leave reports the pointer leaving the surface. It cancels any drag in
// progress and fires [Leave], so that a tooltip opened on a hover closes.
func (i *Input) Leave() error {
	i.down, i.dragging = false, false
	i.l.Leave()
	return nil
}

// DoubleClick resets the view, releasing every zoom and pan, and redraws. It is
// the one control a reader looks for first.
func (i *Input) DoubleClick() error {
	i.down, i.dragging = false, false
	return i.l.Autoscale()
}

// Wheel zooms about a device position by a raw scroll delta, and redraws.
//
// The delta is in the browser's pixel convention, which is the one every
// platform can be converted into: positive scrolls the content away from the
// reader and zooms out, and one notch of a mouse wheel is about a hundred.
// [WheelFactor] is the curve it goes through.
func (i *Input) Wheel(x, y, delta float64) error {
	i.lastX, i.lastY = x, y
	return i.l.Wheel(x, y, WheelFactor(delta))
}

// Resize reports the surface's new size and redraws. See [Live.Resize].
func (i *Input) Resize(w, h int) error { return i.l.Resize(w, h) }

// Rescale reports the surface's new device pixel ratio and redraws. See
// [Live.Rescale].
func (i *Input) Rescale(dpr float64) error { return i.l.Rescale(dpr) }

// Dragging reports whether a drag is in progress — a button is held and the
// pointer has moved past the click slop. A surface uses it to decide what
// cursor to show.
func (i *Input) Dragging() bool { return i.dragging }

func (i *Input) beyondSlop(x, y float64) bool {
	return math.Abs(x-i.downX) > i.slop || math.Abs(y-i.downY) > i.slop
}

// WheelFactor turns a raw scroll delta into a zoom factor.
//
// The exponential keeps a fast scroll from inverting: any delta maps into
// (0, ∞) and never through zero, so holding the wheel down zooms smoothly
// rather than jumping. The rate is chosen so that one notch on a mouse — a
// hundred units in the browser's pixel mode — is about ten percent, which is
// the step a reader expects from a map.
func WheelFactor(delta float64) float64 { return math.Exp(delta / 1000) }
