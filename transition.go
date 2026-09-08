package refract

import (
	"errors"
	"time"

	"github.com/timzifer/refract/data"
)

// Easing reshapes a fraction in [0, 1]. It is what makes a movement look like
// something starting and stopping rather than a thing dragged at a constant
// rate.
type Easing func(f float64) float64

// EaseLinear is no easing at all: the fraction unchanged.
func EaseLinear(f float64) float64 { return f }

// EaseIn starts slowly and arrives at speed. It is rarely what a chart wants:
// a movement that ends abruptly reads as an interruption rather than as an
// arrival.
func EaseIn(f float64) float64 { return f * f * f }

// EaseOut leaves at speed and settles. It is what one thing moving to one new
// place wants.
func EaseOut(f float64) float64 {
	g := 1 - f
	return 1 - g*g*g
}

// EaseInOut starts slowly, moves, and settles. It is the default because it is
// the curve that reads as one movement rather than as a start and a stop.
func EaseInOut(f float64) float64 {
	if f < 0.5 {
		return 4 * f * f * f
	}
	g := -2*f + 2
	return 1 - g*g*g/2
}

// DefaultDuration is how long a transition takes when nobody says. A quarter
// of a second is long enough to be followed by eye and short enough that a
// reader clicking through states is not waiting for the chart.
const DefaultDuration = 250 * time.Millisecond

// ErrNoTweens reports a transition with nothing to move.
var ErrNoTweens = errors.New("refract: a transition needs at least one tween")

// Transition moves a chart from where its tweens start to where they end.
//
// # refract owns no clock
//
// [Transition.At] is the whole primitive: a fraction in, a frame out. It reads
// no clock, starts no goroutine and schedules nothing — which is what makes a
// transition a pure function of a number, and therefore something a golden
// file can be taken of. [Transition.Advance] is sugar for a host that has a
// time.Time to hand over.
//
// The loop belongs to whatever is already running one:
//
//	// A browser, from requestAnimationFrame:
//	running, err := tr.Advance(time.Now())
//
//	// A window, from window.Handler.Frame — asking for another frame only
//	// while it is running, so an idle window stays idle:
//	if running {
//		w.Redraw()
//	}
//
//	// A test, with no clock at all:
//	tr.At(0.5)
//
// # The chart is not rebuilt
//
// A [data.Tween] is one Source whose contents change, so the layer over it is
// built once and a frame of a transition costs a frame. Build the layers over
// [data.Tween.Source] before opening the Live, and hand the same tweens here.
//
// # The axes stay where they are
//
// By default a transition moves the data and leaves the axes alone, because an
// axis that rescales itself every frame is one a reader cannot compare two
// frames of — the same reason a live chart pins its axes. A chart whose two
// states need different axes says so with [Transition.Rescale].
//
// A Transition is not safe for concurrent use, and neither is the Live behind
// it.
type Transition struct {
	l      *Live
	tweens []*data.Tween

	ease Easing
	dur  time.Duration

	// from and to are where the axes sit at each end, learned once when
	// Rescale is on and both empty when it is not.
	from, to View
	rescale  bool

	start   time.Time
	started bool
	f       float64
	done    bool
}

// Transition prepares a move between the state its tweens start at and the
// state they end at.
//
// It positions the tweens at the start and draws nothing: the first frame is
// the caller's first [Transition.At] or [Transition.Advance].
func (l *Live) Transition(tweens ...*data.Tween) (*Transition, error) {
	if len(tweens) == 0 {
		return nil, ErrNoTweens
	}
	for _, tw := range tweens {
		if tw == nil {
			return nil, ErrNoTweens
		}
	}
	tr := &Transition{l: l, tweens: tweens, ease: EaseInOut, dur: DefaultDuration}
	for _, tw := range tweens {
		tw.At(0)
	}
	return tr, nil
}

// Ease sets the curve the fraction goes through and returns tr, so the call
// can be chained onto [Live.Transition]. The default is [EaseInOut].
func (tr *Transition) Ease(e Easing) *Transition {
	if e != nil {
		tr.ease = e
	}
	return tr
}

// Over sets how long the transition takes and returns tr. It is read only by
// [Transition.Advance]; [Transition.At] does not know about time. The default
// is [DefaultDuration].
func (tr *Transition) Over(d time.Duration) *Transition {
	if d > 0 {
		tr.dur = d
	}
	return tr
}

// Rescale makes the axes move with the data, and returns tr.
//
// It is off by default: an axis that rescales itself every frame is one a
// reader cannot compare two frames of, and a chart whose two states share an
// axis should keep it. Turn it on when they genuinely do not — a transition
// from last week's range to this year's — and the domains slide between the
// two rather than jumping at the first frame.
//
// It works by finding out where each axis ends up: the tweens are put at each
// end and the chart rendered into a recording nobody sees, twice, here rather
// than per frame. Between the two the axes are released, so **an axis with a
// domain fixed at construction loses it** — which is the point, because a
// fixed domain is a caller saying the axis does not move and this is a caller
// saying it does. Pass false to change one's mind before the first frame.
//
// While it is on the domains are pinned, and pinning is doing a second job:
// a [scale.Nice] axis re-rounds whatever it is trained on, so an unpinned one
// would relabel itself mid-move — and a changed tick *count* is a structural
// change, which makes ir.Damage report the two frames as not comparable and
// turns every frame into a full repaint. The animation would be the slowest
// one available and would look exactly right. [Transition.Finish] releases
// them again.
func (tr *Transition) Rescale(on bool) *Transition {
	tr.rescale = on
	tr.from, tr.to = View{}, View{}
	return tr
}

// prepare learns where the axes are at each end. It runs at the first frame
// rather than at construction, so that Rescale can be set after Transition and
// so that a transition nobody runs costs nothing.
func (tr *Transition) prepare() error {
	if !tr.rescale || !tr.from.Empty() {
		return nil
	}
	at := func(f float64) (View, error) {
		for _, tw := range tr.tweens {
			tw.At(f)
		}
		// Released first: Train accumulates, so an axis carried over from the
		// other end would report the union of the two rather than this one.
		tr.l.autoscaleAll()
		if err := tr.l.probe(); err != nil {
			return View{}, err
		}
		return tr.l.View(), nil
	}
	to, err := at(1)
	if err != nil {
		return err
	}
	from, err := at(0)
	if err != nil {
		return err
	}
	tr.from, tr.to = from, to
	return nil
}

// At puts the transition at a fraction of the way through and redraws.
//
// The fraction is clamped to [0, 1] and goes through the easing curve. It is
// the primitive: everything else here is a way of choosing a number to give it.
func (tr *Transition) At(f float64) error {
	switch {
	case f < 0:
		f = 0
	case f > 1:
		f = 1
	}
	if err := tr.prepare(); err != nil {
		return err
	}
	tr.f = f
	tr.done = f >= 1

	e := tr.ease(f)
	for _, tw := range tr.tweens {
		tw.At(e)
	}
	if tr.rescale {
		tr.l.lerpView(tr.from, tr.to, e)
	}
	return tr.l.Draw()
}

// Advance puts the transition where the clock says it should be and redraws,
// reporting whether there is more to come.
//
// The first call starts it, so a transition begins when the host first asks
// about it rather than when it was built — which is what lets one be prepared
// on a click and driven from the next frame callback.
//
// running is false on the frame that reaches the end and on every call after
// it, so a host asking for another frame only while it is true stops asking
// exactly once.
func (tr *Transition) Advance(now time.Time) (running bool, err error) {
	if tr.done {
		return false, nil
	}
	if !tr.started {
		tr.start, tr.started = now, true
	}
	f := 1.0
	if tr.dur > 0 {
		f = float64(now.Sub(tr.start)) / float64(tr.dur)
	}
	if err := tr.At(f); err != nil {
		return false, err
	}
	return !tr.done, nil
}

// Fraction reports how far through the transition is, before easing.
func (tr *Transition) Fraction() float64 { return tr.f }

// Done reports whether the transition has reached its end.
func (tr *Transition) Done() bool { return tr.done }

// Finish jumps to the end.
//
// With [Transition.Rescale] on it also releases the axes, so that the chart
// goes back to following its data rather than staying pinned to wherever the
// transition left it. An axis left pinned is one that has quietly stopped
// tracking what it describes, which is a bug that shows up an hour later.
func (tr *Transition) Finish() error {
	if err := tr.At(1); err != nil {
		return err
	}
	if !tr.rescale {
		return nil
	}
	tr.l.autoscaleAll()
	return tr.l.Draw()
}

// autoscaleAll releases every axis, so that the next render establishes the
// domain from the data rather than from wherever the last frame left it.
func (l *Live) autoscaleAll() {
	for _, p := range l.idx.Panels() {
		for _, s := range axesOf(p) {
			autoscale(s)
		}
	}
}

// lerpView pins every axis between where it starts and where it ends.
func (l *Live) lerpView(from, to View, f float64) {
	panels := l.idx.Panels()
	if len(from.panels) != len(panels) || len(to.panels) != len(panels) {
		return
	}
	for i, p := range panels {
		for j, s := range axesOf(p) {
			a, b := from.panels[i].axes[j], to.panels[i].axes[j]
			if !a.ok || !b.ok {
				continue
			}
			writeAxis(s, axisView{
				min: a.min + (b.min-a.min)*f,
				max: a.max + (b.max-a.max)*f,
				ok:  true,
			})
		}
	}
}
