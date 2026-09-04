package render

import (
	"runtime"
	"sync"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// drawData draws every panel's layers.
//
// Panels are independent — each has its own rectangle, its own scales and its
// own rows — so building them is the part of a render that parallelises
// cleanly. What does not parallelise is the backend: it is an immediate-mode
// sink and holds a state stack, so two goroutines drawing into one would
// interleave clips and paths. Each panel therefore builds into an
// [ir.Recorder] of its own and the recordings are replayed into the backend
// afterwards, in panel order.
//
// Replaying in panel order rather than in completion order is the point. A
// parallel render emits exactly the calls a serial one does, in exactly the
// same sequence, so the golden files cover both paths at once and neither can
// drift from the other.
//
// A chart with an [Observer] takes the serial path. The observer is told which
// layer is drawing so that a caller can attribute the calls that follow; two
// panels drawing at once would interleave those announcements into nonsense.
func drawData(b ir.Backend, c Chart, panels []Panel, areas []ir.Rect, th theme.Theme) error {
	if !concurrent(c, panels) {
		for i, p := range panels {
			p.setRange(areas[i])
			if c.Observer != nil {
				c.Observer.Panel(i, areas[i], p.X, p.Y)
			}
			if err := drawLayers(b, p, areas[i], th, c.Observer, c.RowSink); err != nil {
				return err
			}
		}
		return nil
	}

	recs := make([]*ir.Recorder, len(panels))
	errs := make([]error, len(panels))
	m := &syncMeasurer{b: b}

	var wg sync.WaitGroup
	for i := range panels {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A snapshot per goroutine: panels that share an axis share one
			// scale object, and setting its device range is a write.
			p := panels[i].snapshot()
			p.setRange(areas[i])
			rec := acquireRecorder(m)
			recs[i] = rec
			errs[i] = drawLayers(rec, p, areas[i], th, nil, nil)
		}()
	}
	wg.Wait()

	var err error
	for i, rec := range recs {
		if errs[i] != nil {
			// Stop where a serial pass would have stopped: the panels before
			// this one were drawn, this one and the rest were not.
			err = errs[i]
			break
		}
		rec.Replay(b)
	}
	for _, rec := range recs {
		releaseRecorder(rec)
	}
	if err != nil {
		return err
	}

	// Leave the real scales ranged as a serial pass would have left them, so
	// that nothing downstream can tell which path ran.
	for i, p := range panels {
		p.setRange(areas[i])
	}
	return nil
}

// concurrent reports whether this chart's panels can be built in parallel.
//
// Two panels are the minimum for there to be anything to overlap, one
// processor means there is nothing to overlap it with, and a scale that cannot
// snapshot itself cannot be given to two goroutines — see
// [scale.Snapshotter]. Any of those, and the serial path is not a fallback but
// the right answer.
func concurrent(c Chart, panels []Panel) bool {
	if c.Serial || c.Observer != nil || c.RowSink != nil || len(panels) < 2 || runtime.GOMAXPROCS(0) < 2 {
		return false
	}
	for _, p := range panels {
		if !snapshotable(p.X) || !snapshotable(p.Y) {
			return false
		}
	}
	return true
}

func snapshotable(s scale.Scale) bool {
	_, ok := s.(scale.Snapshotter)
	return ok
}

// snapshot returns the panel with scales of its own.
func (p Panel) snapshot() Panel {
	p.X, p.Y = snapshotScale(p.X), snapshotScale(p.Y)
	return p
}

func snapshotScale(s scale.Scale) scale.Scale {
	if sn, ok := s.(scale.Snapshotter); ok {
		return sn.Snapshot()
	}
	return s
}

// syncMeasurer serialises text measurement onto one backend.
//
// Measure is the only Backend call a geom may make while it builds, and it is
// rare and cheap when it happens, so a mutex is the whole answer. Handing each
// goroutine its own font stack would be the alternative, and it would mean two
// panels could disagree about how wide a label is.
type syncMeasurer struct {
	mu sync.Mutex
	b  ir.Backend
}

func (s *syncMeasurer) Measure(run ir.TextRun) ir.TextMetrics {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Measure(run)
}

// Recorders are pooled because their arenas are sized by the data they hold: a
// chart redrawn every frame refills the same memory instead of asking for more.
var recorderPool sync.Pool

func acquireRecorder(m ir.Measurer) *ir.Recorder {
	r, _ := recorderPool.Get().(*ir.Recorder)
	if r == nil {
		return ir.NewRecorder(m)
	}
	r.Reset()
	r.SetMeasurer(m)
	return r
}

func releaseRecorder(r *ir.Recorder) {
	if r == nil {
		return
	}
	r.Reset()
	r.SetMeasurer(nil)
	recorderPool.Put(r)
}
