package window

import (
	"math"
	"time"
)

// clickTracker turns a stream of presses into single and double clicks.
//
// A browser reports a double click itself; a window layer reports two presses
// and leaves the interpretation to whoever cares, because "how long is quick"
// and "how far is the same place" are policy. The policy here is the desktop
// one: within four hundred milliseconds and within four pixels.
type clickTracker struct {
	last   time.Time
	lastX  float64
	lastY  float64
	primed bool
}

// The thresholds a second press has to meet to be part of a double click.
const (
	doubleClickInterval = 400 * time.Millisecond
	doubleClickSlop     = 4.0
)

// press records a press and reports whether it completed a double click.
//
// A double click consumes the state, so three presses in a row are one double
// click and one press rather than two overlapping double clicks — which is
// what every toolkit does and what a reader expects when they click a third
// time.
func (c *clickTracker) press(x, y float64) bool {
	now := time.Now()
	double := c.primed &&
		now.Sub(c.last) <= doubleClickInterval &&
		math.Abs(x-c.lastX) <= doubleClickSlop &&
		math.Abs(y-c.lastY) <= doubleClickSlop

	c.last, c.lastX, c.lastY = now, x, y
	c.primed = !double
	return double
}
