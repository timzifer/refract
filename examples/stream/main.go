// Command stream draws a live chart over a growing series.
//
// It is the v0.5 streaming story in one file: a producer appends samples on
// its own goroutine, the renderer freezes a snapshot between frames, and each
// redraw repaints only the part of the canvas that changed. The last frame is
// written out as SVG so that there is something to look at.
//
//	go run ./examples/stream -o live.svg -frames 120
package main

import (
	"flag"
	"fmt"
	"image"
	"math"
	"math/rand/v2"
	"os"
	"sync"
	"time"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func main() {
	out := flag.String("o", "live.svg", "where to write the last frame")
	frames := flag.Int("frames", 120, "how many frames to draw")
	flag.Parse()
	stats, err := run(*out, *frames)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stream:", err)
		os.Exit(1)
	}
	fmt.Printf("%d frames, %d painted, %.0f%% of the canvas repainted on average\n",
		stats.Frames, stats.Painted, 100*stats.Fraction())
}

// window is how much of the series is on screen. A live chart shows the recent
// past; keeping every sample since the process started is a memory leak with a
// plot attached.
const window = 600

func run(out string, frames int) (stats, error) {
	st := data.NewStream("t", "y", "load").Window(window)

	p := refract.New(
		refract.Theme(theme.Dark),
		refract.Size(900, 400),
		refract.Title("Live throughput"),
		refract.YTitle("rows/s"),
	)
	p.X(scale.Time())
	// A pinned Y axis is what makes a live chart readable: an axis that
	// rescales itself every frame turns every change into a redraw of
	// everything, and makes two frames impossible to compare by eye.
	p.Y(scale.Linear(scale.Domain(0, 120)))
	p.Add(
		geom.Line(st.Source(), geom.X("t"), geom.Y("y"), geom.Color(palette.SkyBlue), geom.Label("throughput")),
		geom.HLine(100, geom.Label("capacity"), geom.Dash(6, 4)),
	)

	// A tooltip's worth of wiring: what the pointer is over, whenever it moves.
	p.On(refract.Hover, func(ev refract.Event) {
		if ev.Found {
			_ = ev.Series() // a real UI would draw this; here it only has to compile
		}
	})

	// A surface stands in for the browser canvas this example has no access
	// to: it draws nothing and counts what each frame was asked to repaint.
	// See backend/canvas for the real thing.
	surface := &surface{w: 900, h: 400}
	live, err := p.Live(surface)
	if err != nil {
		return stats{}, err
	}

	// Start with a full window of history, the way a live chart that has been
	// open for a minute already is. A chart whose axis is still growing
	// repaints its labels every frame; one whose window is full slides, and
	// sliding is what damage tracking is good at.
	seed(st, window)

	// The producer runs until the renderer has had enough. It only ever
	// appends; it never reads, and it never touches a snapshot.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		produce(st, stop)
	}()

	for range frames {
		st.Snapshot() // freeze
		if err := live.Draw(); err != nil {
			close(stop)
			wg.Wait()
			live.Close()
			return stats{}, err
		}
		time.Sleep(time.Millisecond)
	}
	close(stop)
	wg.Wait()

	if err := live.Close(); err != nil {
		return stats{}, err
	}

	// The picture. A live chart lives on a surface that is repainted; a file
	// is a document that is written once, so the artefact is a plain render of
	// the plot as it now stands rather than 120 frames stacked on top of each
	// other.
	if err := p.Render(refract.SVG(out)); err != nil {
		return stats{}, err
	}
	return surface.stats(frames), nil
}

// seed fills the window with history, so that the first frame is the frame a
// chart that has been open for a while draws.
func seed(st *data.Stream, n int) {
	r := rand.New(rand.NewPCG(3, 5))
	for i := range n {
		st.Append(sample(r, i))
	}
}

// produce is the other goroutine: samples arriving from wherever samples come
// from.
func produce(st *data.Stream, stop <-chan struct{}) {
	r := rand.New(rand.NewPCG(11, 13))
	for i := window; ; i++ {
		select {
		case <-stop:
			return
		default:
		}
		st.Append(sample(r, i))
		time.Sleep(200 * time.Microsecond)
	}
}

// base is when the series starts. It is fixed rather than time.Now() so that
// two runs of the example produce the same chart.
var base = time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)

func sample(r *rand.Rand, i int) (t, y, load float64) {
	at := base.Add(time.Duration(i) * 20 * time.Millisecond)
	return scale.Nanos(at), 60 + 30*math.Sin(float64(i)/40) + 6*r.NormFloat64(), float64(i % 7)
}

// surface stands in for a repaintable surface — a canvas, a window — and does
// nothing but count.
//
// It is both the [ir.Target] and the [ir.Backend], which is the smallest a
// backend gets: every drawing call is a no-op, [ir.Partial] records what the
// frame was asked to repaint, and Measure approximates rather than shaping,
// because there is no font stack here to ask.
type surface struct {
	w, h    int
	painted int
	area    float32
}

func (s *surface) Open(int, int, float64) (ir.Backend, error) { return s, nil }
func (s *surface) Close() error                               { return nil }

// Damage is the whole point of the example: a real backend clips to these
// rectangles and repaints them, and a nil list means the whole frame.
func (s *surface) Damage(rects []ir.Rect) {
	s.painted++
	if rects == nil {
		s.area += float32(s.w * s.h)
		return
	}
	for _, r := range rects {
		s.area += r.Dx() * r.Dy()
	}
}

func (s *surface) Measure(run ir.TextRun) ir.TextMetrics {
	adv := float32(0.6 * run.Font.Size * float64(len(run.Text)))
	asc, desc := float32(0.8*run.Font.Size), float32(0.2*run.Font.Size)
	return ir.TextMetrics{Advance: adv, Ascent: asc, Descent: desc, Ink: ir.R(0, -asc, adv, desc)}
}

func (s *surface) Polyline([]ir.Point, ir.Stroke)                {}
func (s *surface) StrokePath(*ir.Path, ir.Stroke)                {}
func (s *surface) FillPath(*ir.Path, ir.Fill, ir.FillRule)       {}
func (s *surface) Text(ir.TextRun)                               {}
func (s *surface) Markers(ir.Marker, []ir.Point, ir.MarkerStyle) {}
func (s *surface) Image(image.Image, ir.Rect)                    {}
func (s *surface) Push(*ir.Path, ir.Affine)                      {}
func (s *surface) Pop()                                          {}
func (s *surface) Flush() error                                  { return nil }

func (s *surface) stats(frames int) stats {
	return stats{
		Frames:  frames,
		Painted: s.painted,
		Area:    s.area,
		Canvas:  float32(s.w*s.h) * float32(max(s.painted, 1)),
	}
}

type stats struct {
	Frames  int
	Painted int
	Area    float32
	Canvas  float32
}

// Fraction is how much of the canvas an average painted frame repainted.
func (s stats) Fraction() float32 {
	if s.Canvas == 0 {
		return 0
	}
	return s.Area / s.Canvas
}

var (
	_ ir.Target  = (*surface)(nil)
	_ ir.Backend = (*surface)(nil)
	_ ir.Partial = (*surface)(nil)
)
