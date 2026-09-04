package window

import (
	"testing"
	"time"

	"github.com/gogpu/gpucontext"
	"github.com/timzifer/refract/ir"
)

// The window itself cannot be tested here: opening one needs a display, and a
// test that needed one would be a test nobody could run — the same position
// backend/canvas takes about a browser. What is tested is everything that is
// not the window: the two pieces of input interpretation, and the seams the
// rest of refract is joined to.

func TestASurfaceIsARenderTarget(t *testing.T) {
	w := New(Title("t"), Size(320, 240))
	var target ir.Target = w.Target()
	if target == nil {
		t.Fatal("a window offers no render target")
	}
	if gotW, gotH := w.Size(); gotW != 320 || gotH != 240 {
		t.Errorf("the window is %dx%d, want 320x240", gotW, gotH)
	}
	if w.ScaleFactor() <= 0 {
		t.Errorf("the window reports a device pixel ratio of %v", w.ScaleFactor())
	}
}

func TestSizeIsIgnoredWhenItIsNotOne(t *testing.T) {
	w := New(Size(0, -1))
	if got, gotH := w.Size(); got <= 0 || gotH <= 0 {
		t.Errorf("a nonsense size was accepted: %dx%d", got, gotH)
	}
}

func TestTheWheelIsReadInPixelsWhateverItCounts(t *testing.T) {
	for _, c := range []struct {
		mode gpucontext.ScrollDeltaMode
		in   float64
		want float64
	}{
		{gpucontext.ScrollDeltaPixel, 120, 120},
		{gpucontext.ScrollDeltaLine, 3, 120},
		{gpucontext.ScrollDeltaPage, 1, 800},
	} {
		got := pixelDelta(gpucontext.ScrollEvent{DeltaY: c.in, DeltaMode: c.mode})
		if got != c.want {
			t.Errorf("%v of %v read as %v, want %v", c.in, c.mode, got, c.want)
		}
	}
}

func TestTwoQuickPressesInOnePlaceAreADoubleClick(t *testing.T) {
	var c clickTracker
	if c.press(10, 10) {
		t.Error("the first press was a double click")
	}
	if !c.press(11, 11) {
		t.Error("a second press in the same place was not a double click")
	}
	// A third press starts again rather than completing a second double click
	// with the same middle press.
	if c.press(11, 11) {
		t.Error("three presses were two double clicks")
	}
}

func TestAPressSomewhereElseIsNotADoubleClick(t *testing.T) {
	var c clickTracker
	c.press(10, 10)
	if c.press(40, 10) {
		t.Error("a press across the window completed a double click")
	}
}

func TestASlowSecondPressIsNotADoubleClick(t *testing.T) {
	var c clickTracker
	c.press(10, 10)
	c.last = c.last.Add(-2 * doubleClickInterval)
	if c.press(10, 10) {
		t.Error("a press long afterwards completed a double click")
	}
}

// The interval is a policy rather than a constant nobody chose: it is what the
// desktop toolkits use, and a test that pinned it to something else would be
// pinning a typo.
func TestTheDoubleClickPolicyIsTheDesktopOne(t *testing.T) {
	if doubleClickInterval < 200*time.Millisecond || doubleClickInterval > time.Second {
		t.Errorf("the double-click interval is %v", doubleClickInterval)
	}
	if doubleClickSlop <= 0 || doubleClickSlop > 10 {
		t.Errorf("the double-click slop is %v pixels", doubleClickSlop)
	}
}
