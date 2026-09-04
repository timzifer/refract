package scale_test

import (
	"math"
	"testing"
	"time"

	"github.com/timzifer/refract/scale"
)

// zoomable is every scale that can be panned, with a domain to start from.
func zoomable() map[string]scale.Scale {
	return map[string]scale.Scale{
		"linear": scale.Linear(scale.Nice()),
		"log":    scale.Log(scale.LogNice()),
		"symlog": scale.SymLog(),
		"time":   scale.Time(),
	}
}

func TestSetDomainPinsTheDomain(t *testing.T) {
	for name, s := range zoomable() {
		t.Run(name, func(t *testing.T) {
			z, ok := s.(scale.Zoomer)
			if !ok {
				t.Fatalf("%T does not implement scale.Zoomer", s)
			}
			s.Train(1, 1000)
			z.SetDomain(10, 100)

			lo, hi := s.Domain()
			if lo != 10 || hi != 100 {
				t.Errorf("domain = %v..%v, want exactly 10..100 — a pinned view is not niced", lo, hi)
			}
			// Training must not move a pinned domain: the next render trains
			// on the same data that was already there.
			s.Train(0.001, 1e9)
			if lo, hi = s.Domain(); lo != 10 || hi != 100 {
				t.Errorf("training moved a pinned domain to %v..%v", lo, hi)
			}
		})
	}
}

func TestSetDomainTakesItsBoundsInEitherOrder(t *testing.T) {
	s := scale.Linear()
	s.(scale.Zoomer).SetDomain(100, 10)
	if lo, hi := s.Domain(); lo != 10 || hi != 100 {
		t.Errorf("domain = %v..%v, want 10..100 — a drag can cross itself", lo, hi)
	}
}

func TestAutoscaleReleasesThePin(t *testing.T) {
	for name, s := range zoomable() {
		t.Run(name, func(t *testing.T) {
			z := s.(scale.Zoomer)
			z.SetDomain(10, 100)
			z.Autoscale()
			s.Train(2, 8)
			lo, hi := s.Domain()
			if lo > 2 || hi < 8 {
				t.Errorf("after Autoscale the domain is %v..%v, want it to cover the data 2..8", lo, hi)
			}
		})
	}
}

func TestAutoscaleReleasesAFixedDomainToo(t *testing.T) {
	s := scale.Linear(scale.Domain(0, 1))
	s.Train(50)
	if _, hi := s.Domain(); hi != 1 {
		t.Fatalf("a fixed domain trained to %v", hi)
	}
	s.(scale.Zoomer).Autoscale()
	s.Train(50)
	if _, hi := s.Domain(); hi < 50 {
		t.Errorf("after Autoscale the domain is capped at %v, want the data", hi)
	}
}

func TestALogScaleClampsAZoomOffTheBottom(t *testing.T) {
	s := scale.Log()
	s.(scale.Zoomer).SetDomain(-100, 50)
	lo, hi := s.Domain()
	if lo <= 0 {
		t.Errorf("domain = %v..%v; a log axis has no position for a value at or below zero", lo, hi)
	}
	if hi != 50 {
		t.Errorf("the upper bound moved to %v, want 50", hi)
	}
	if s.Map(lo) != s.Map(lo) {
		t.Error("the clamped domain does not map")
	}
}

func TestATimeScaleZoomsInNanoseconds(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	s := scale.Time()
	s.Train(scale.Nanos(from.Add(-24*time.Hour)), scale.Nanos(to.Add(24*time.Hour)))
	s.(scale.Zoomer).SetDomain(scale.Nanos(from), scale.Nanos(to))
	s.SetRange(0, 100)

	if got := scale.FromNanos(s.Invert(0)).UTC(); !got.Equal(from) {
		t.Errorf("the left edge is %v, want %v", got, from)
	}
	if got := scale.FromNanos(s.Invert(100)).UTC(); !got.Equal(to) {
		t.Errorf("the right edge is %v, want %v", got, to)
	}
	// The ticks of an hour-long view are an hour's ticks, not a two-day one's.
	ticks := s.Ticks(5)
	if len(ticks) == 0 {
		t.Fatal("no ticks")
	}
	for _, tick := range ticks {
		at := scale.FromNanos(tick.Value)
		if at.Before(from.Add(-time.Minute)) || at.After(to.Add(time.Minute)) {
			t.Errorf("tick at %v is outside the zoomed view", at)
		}
	}
}

func TestAPinnedDomainSurvivesACloneAndASnapshot(t *testing.T) {
	s := scale.Linear(scale.Nice())
	s.Train(0, 1000)
	s.(scale.Zoomer).SetDomain(10, 20)

	cl := s.(scale.Cloner).Clone()
	if lo, hi := cl.Domain(); lo != 10 || hi != 20 {
		t.Errorf("a clone's domain is %v..%v; a pinned domain is configuration", lo, hi)
	}
	sn := s.(scale.Snapshotter).Snapshot()
	if lo, hi := sn.Domain(); lo != 10 || hi != 20 {
		t.Errorf("a snapshot's domain is %v..%v", lo, hi)
	}
}

func TestAnOrdinalScaleIsNotZoomable(t *testing.T) {
	// Half a category is not a view of anything, so the ordinal scale says so
	// by not implementing the interface rather than by rounding.
	if _, ok := scale.Ordinal().(scale.Zoomer); ok {
		t.Error("an ordinal scale claims to be zoomable")
	}
}

func TestZoomingKeepsThePointerOverItsValue(t *testing.T) {
	// This is the whole promise of a wheel zoom, and it holds on every scale
	// because the arithmetic happens in device space.
	for name, s := range zoomable() {
		t.Run(name, func(t *testing.T) {
			s.Train(1, 1000)
			s.SetRange(0, 800)
			const at = 300
			before := s.Invert(at)

			z := s.(scale.Zoomer)
			const f = 0.5
			z.SetDomain(s.Invert(at+(0-at)*f), s.Invert(at+(800-at)*f))
			s.SetRange(0, 800)

			after := s.Invert(at)
			if rel := math.Abs(after-before) / math.Max(math.Abs(before), 1e-12); rel > 1e-5 {
				t.Errorf("the value under the pointer moved from %v to %v", before, after)
			}
		})
	}
}
