// Command transition animates a chart between two states of the same table.
//
// It writes three frames of one movement — the start, the middle and the end —
// so that what a transition does is visible in files rather than only in a
// window. The middle frame is the interesting one: it is a real frame of a real
// animation, because a transition is a pure function of its fraction.
//
// What it demonstrates:
//
//   - **The join.** geom.KeyBy names the column that says which row is which,
//     so a bar that moves is understood as the same bar rather than as one bar
//     leaving and another arriving. data.Alignment reports the three groups by
//     D3's names — Entered, Updated, Exited — because the vocabulary is the
//     useful part.
//   - **Enter and exit.** data.EnterFrom and data.ExitTo say where a row comes
//     from and goes to, in data space, so a new bar grows out of the baseline
//     and a departing one sinks back into it. There is no fade: opacity is a
//     property of a layer rather than of a row.
//   - **A label that counts.** The value labels are a numeric column bound to
//     geom.TextBy, and a text layer re-spells its column on every frame — so
//     the numbers count up by themselves. data.Round is what keeps them
//     reading 33 rather than 33.300000000000004.
//   - **The host owns the clock.** refract reads none. Transition.At(f) is the
//     whole primitive, and this program drives it from a loop; a browser drives
//     it from requestAnimationFrame and a window from its frame callback, with
//     Transition.Advance doing the arithmetic.
//
// Like the other documented examples it is executed by a test, so it cannot
// quietly stop compiling or stop producing charts.
//
//	go run ./examples/transition -o frame
package main

import (
	"flag"
	"fmt"
	"os"

	"image"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func main() {
	prefix := flag.String("o", "frame", "prefix for the output paths")
	flag.Parse()
	al, err := run(*prefix)
	if err != nil {
		fmt.Fprintln(os.Stderr, "transition:", err)
		os.Exit(1)
	}
	fmt.Printf("entered %v, updated %v, exited %v\n",
		al.Entered(), al.Updated(), al.Exited())
}

// The two states. Two languages hold their place and move, one arrives, one
// leaves — which is the shape that makes enter, update and exit all visible in
// one picture.
func before() *data.Table {
	return refract.NewTable().
		String("lang", []string{"go", "rust", "perl"}).
		Float64("slot", []float64{0, 1, 2}).
		Float64("share", []float64{40, 25, 18})
}

func after() *data.Table {
	return refract.NewTable().
		String("lang", []string{"go", "rust", "zig"}).
		Float64("slot", []float64{0, 1, 2}).
		Float64("share", []float64{30, 45, 22})
}

func run(prefix string) (data.Alignment, error) {
	// EnterFrom and ExitTo are in data space: a bar that is not there yet is a
	// bar of height zero, which is the honest way to say it without a per-row
	// opacity channel. Round keeps the counting labels reading as numbers.
	//
	// slot is Held rather than blended: it is an axis position that happens to
	// be a number, and halfway between slot 1 and slot 2 is not a slot. A bar
	// that changed places would slide if the column were left to interpolate.
	tw, err := data.NewTween(before(), after(), "lang",
		data.EnterFrom("share", 0),
		data.ExitTo("share", 0),
		data.Round("share", 0),
		data.Hold("slot"),
	)
	if err != nil {
		return data.Alignment{}, err
	}

	p := refract.New(
		refract.Theme(theme.Light),
		refract.Size(640, 360),
		refract.Title("Share, moving"),
		refract.XTitle("slot"),
		refract.YTitle("share"),
	)
	// Both axes are pinned. A transition leaves the axes alone by default, and
	// pinning them here says why that is the right default: an axis that
	// rescaled every frame would make the two end frames impossible to compare
	// by eye, which is the whole thing an animation is for.
	p.X(scale.Linear(scale.Domain(-0.6, 2.6)))
	p.Y(scale.Linear(scale.Domain(0, 50)))
	p.Add(
		geom.Bar(tw.Source(), geom.X("slot"), geom.Y("share"),
			geom.KeyBy("lang"), geom.Color(palette.Blue), geom.BarWidth(0.6)),
		// The label that counts. It reads the same blended column the bars do.
		geom.Text(tw.Source(), geom.X("slot"), geom.Y("share"),
			geom.TextBy("share"), geom.Align(ir.AlignCenter, ir.AlignBottom)),
	)

	// A surface to animate into. The frames written out are ordinary renders
	// of the plot as it stands, which is what a transition leaves behind at
	// any fraction.
	live, err := p.Live(&surface{})
	if err != nil {
		return data.Alignment{}, err
	}
	defer live.Close()

	tr, err := live.Transition(tw)
	if err != nil {
		return data.Alignment{}, err
	}
	tr.Ease(refract.EaseInOut)

	// The loop is this program's, not refract's. Advance would do the same
	// from a clock; At is what a test and a file want, because a fraction is
	// reproducible and a wall clock is not.
	for _, f := range []struct {
		name string
		at   float64
	}{
		{"start", 0},
		{"middle", 0.5},
		{"end", 1},
	} {
		if err := tr.At(f.at); err != nil {
			return data.Alignment{}, err
		}
		if err := p.Render(refract.SVG(prefix + "-" + f.name + ".svg")); err != nil {
			return data.Alignment{}, err
		}
	}
	return tw.Alignment(), nil
}

// surface stands in for the canvas or window this example has no access to. It
// draws nothing; the pictures are written with Plot.Render. See backend/canvas
// and backend/window for the real things.
type surface struct{}

func (s *surface) Open(int, int, float64) (ir.Backend, error) { return s, nil }
func (s *surface) Close() error                               { return nil }

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
