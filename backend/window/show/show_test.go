package show_test

import (
	"strings"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/backend/window/show"
)

// Opening a window needs a display, so what is tested here is what happens
// before one is opened. The wiring itself is [refract.Input], which is tested
// in the core against the same state machine a browser drives.

func TestShowingNothingIsAnError(t *testing.T) {
	if err := show.Plot(nil); err == nil {
		t.Error("showing a nil plot succeeded")
	}
	if err := show.Into(nil, refract.New()); err == nil {
		t.Error("showing into a nil window succeeded")
	}
}

func TestAnEmptyPlotIsRefusedBeforeAWindowOpens(t *testing.T) {
	err := show.Plot(refract.New())
	if err == nil {
		t.Fatal("a plot with no layers and no scales opened a window")
	}
	if !strings.Contains(err.Error(), "no layers") {
		t.Errorf("the error is %v, want the one Plot.Live gives", err)
	}
}
