// Command linked wires two charts together through the host program.
//
// Hovering a stage in the throughput chart highlights the flows that touch it
// in the sankey beside it. That is the whole shape of dependent interaction in
// refract: the library says what the pointer is on, the host decides what that
// means somewhere else, and the second chart is told in the same vocabulary
// any other caller would use.
//
// There is no link between the two charts inside refract, and that is
// deliberate. A link is a statement about two charts, and refract's model is
// one chart — so the mapping from "the parse stage" to "the flows through it"
// lives here, in ordinary Go, where it can be a lookup, a database query, or a
// rule nobody could have written down in a chart specification.
//
// The three pieces refract contributes:
//
//   - geom.KeyBy names the column that identifies a row, so Event.Key is a
//     name the other chart also knows. A row number would not be: it is an
//     index into a table as it stands this frame, and the two charts are drawn
//     from two different tables.
//   - Plot.SetLayers replaces the second chart's layers, so a highlight that
//     arrives on every pointer move does not accumulate one layer per move.
//   - Live.Rebuild puts the reader's zoom back, so answering a hover does not
//     throw away wherever they had been looking.
//   - An overlay draws the feedback. The first chart gets a crosshair and a
//     tooltip where the pointer is; the second gets a ring round the node the
//     hovered stage feeds. Both are drawn by refract, over the finished chart,
//     and neither is hit-testable — so pointing at the tooltip cannot dismiss
//     it.
//
// # Why the highlight recolours rather than overdraws
//
// The obvious way to highlight some flows is to draw them again on top, and
// geom.Faceter.Subset would build exactly that layer. It would be wrong here.
// A sankey's geometry is a property of its whole edge list — where a node sits
// and how thick it is depends on everything flowing through it — so a sankey
// over three of five edges lays out three edges, and the highlight would sit
// beside the flows it meant to mark rather than on them.
//
// So the highlight is a colour rather than a layer: the same five edges, drawn
// once, with a column saying which of them are interesting. A mark whose
// layout is a function of one row — a scatter, a bar, a rect — can be
// overdrawn with Subset instead. Which of the two applies is a property of the
// mark, and it is worth knowing before reaching for either.
//
// Like the other documented examples it is executed by a test, so it cannot
// quietly stop compiling or stop producing charts.
//
//	go run ./examples/linked -throughput throughput.svg -flow flow.svg
package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"strconv"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func main() {
	throughput := flag.String("throughput", "throughput.svg", "output path for the scatter")
	flow := flag.String("flow", "flow.svg", "output path for the sankey")
	flag.Parse()
	stage, n, err := run(*throughput, *flow)
	if err != nil {
		fmt.Fprintln(os.Stderr, "linked:", err)
		os.Exit(1)
	}
	fmt.Printf("hovering %q highlighted %d of the flows\n", stage, n)
}

// stages is the pipeline both charts describe. The stage name is what they
// have in common, and is therefore what a key can be.
var stages = []string{"ingest", "parse", "index", "serve"}

// samples is one row per stage per hour: the table the scatter plots.
func samples() *data.Table {
	var (
		stage []string
		hour  []float64
		rps   []float64
	)
	for h := range 6 {
		for i, s := range stages {
			stage = append(stage, s)
			hour = append(hour, float64(h))
			rps = append(rps, 100+float64(20*i)+float64(5*h))
		}
	}
	return refract.NewTable().
		String("stage", stage).
		Float64("hour", hour).
		Float64("rps", rps)
}

// The edge list the sankey plots: one row per link, its two ends spelled as
// stage names. That is what makes a stage name a key both charts understand.
var (
	edgeFrom  = []string{"ingest", "ingest", "parse", "parse", "index"}
	edgeTo    = []string{"parse", "index", "index", "serve", "serve"}
	edgeBytes = []float64{40, 12, 30, 18, 34}
)

// flowTable builds the sankey's data with a column saying which edges are
// highlighted. The edge list is always all of it — that is what keeps the
// layout still while the colours change.
func flowTable(lit []bool) *data.Table {
	mark := make([]string, len(edgeFrom))
	for i := range mark {
		mark[i] = "other"
		if lit != nil && lit[i] {
			mark[i] = "selected"
		}
	}
	return refract.NewTable().
		String("from", edgeFrom).
		String("to", edgeTo).
		Float64("bytes", edgeBytes).
		String("lit", mark)
}

// bare is the theme a mark that places its own layout wants: a sankey's two
// axes describe the unit square, and a ladder of numbers from 0 to 1 beside it
// means nothing.
func bare() theme.Theme {
	return theme.Light.With(
		theme.Grid(false, false),
		theme.AxisLines(false, false),
		theme.Ticks(false, false),
	)
}

// sankeyOver is the flow chart's one layer, over whichever colouring it is
// currently showing.
func sankeyOver(lit []bool) geom.Geom {
	return geom.Sankey(flowTable(lit),
		geom.From("from"), geom.To("to"), geom.Value("bytes"),
		geom.ColorBy("lit", scale.Qualitative(palette.OkabeIto)),
		geom.Padding(0.02),
	)
}

func run(throughputPath, flowPath string) (hovered string, highlighted int, err error) {
	src := samples()

	// Chart 1: throughput per stage. KeyBy names the column whose value
	// identifies a row — the thing the other chart also knows about.
	line := refract.New(
		refract.Theme(theme.Light),
		refract.Size(720, 360),
		refract.Title("Throughput by stage"),
		refract.XTitle("hour"),
		refract.YTitle("requests/s"),
	)
	line.X(scale.Linear(scale.Nice()))
	line.Y(scale.Linear(scale.Nice()))
	line.Add(geom.Scatter(src,
		geom.X("hour"), geom.Y("rps"),
		geom.ColorBy("stage", scale.Qualitative(palette.OkabeIto)),
		geom.KeyBy("stage"),
		geom.Size(6),
	))

	// Chart 2: the flows, in their resting colours to begin with.
	flow := refract.New(
		refract.Theme(bare()),
		refract.Size(720, 360),
		refract.Title("Bytes between stages"),
	)
	flow.Add(sankeyOver(nil))

	// Each chart gets a surface of its own. Two charts on one canvas would be
	// a Grid, and a Grid renders a document rather than keeping a surface
	// open — so an interactive dashboard is several Lives, wired together
	// here rather than composed by refract.
	lineLive, err := line.Live(&surface{})
	if err != nil {
		return "", 0, err
	}
	defer lineLive.Close()
	lineLive.TrackRows(true)

	flowLive, err := flow.Live(&surface{})
	if err != nil {
		return "", 0, err
	}
	defer flowLive.Close()
	// The sankey reports its rows so that the ring below can be put where a
	// flow actually landed.
	flowLive.TrackRows(true)

	// The feedback. Both overlays are installed once and their fields moved
	// per event: an overlay is a pointer the caller keeps, so there is nothing
	// to re-install when the pointer moves.
	// They go on the *plots* rather than on the Lives, so that the SVGs written
	// out at the end carry them: Plot.Render draws the plot's overlay, and a
	// picture of an interactive chart that left out what the reader was looking
	// at would be a picture of a different chart. Live.Overlay is what to reach
	// for when the overlay belongs to one surface and not to the model.
	cross := &refract.Crosshair{Panel: -1}
	tip := &refract.Tooltip{}
	line.Overlay(refract.Overlays{cross, tip})

	rings := &refract.Highlight{Panel: -1, Radius: 10}
	flow.Overlay(rings)

	// The wire. A hover in chart 1 arrives here with a key; what that key
	// means in chart 2 is this program's business, and nothing refract could
	// have guessed.
	line.On(refract.Hover, func(ev refract.Event) {
		lit := flowsThrough(ev.Key)
		hovered, highlighted = ev.Key, count(lit)

		// The crosshair follows the pointer; the tooltip says what is under
		// it. A hover with no mark under it clears both, which is one frame's
		// full repaint — see ADR 0046 on why appearing and disappearing costs
		// more than moving.
		cross.At, cross.Show = ev.Point, ev.Found
		if ev.Found {
			tip.At = ev.Point
			tip.Lines = []string{
				ev.Key,
				"hour " + strconv.FormatFloat(ev.Hit.X, 'f', 0, 64),
				strconv.FormatFloat(ev.Hit.Y, 'f', 0, 64) + " rps",
			}
		} else {
			tip.Lines = nil
		}

		// SetLayers rather than Add: this runs on every pointer move, and a
		// chart that only ever gained layers would gain one per pixel of
		// travel.
		flow.SetLayers(sankeyOver(lit))
		// Rebuild keeps whatever the reader had zoomed to. Without that,
		// answering a hover would cost them their place in the chart.
		if err := flowLive.Rebuild(); err != nil {
			return
		}
		if err := flowLive.Draw(); err != nil {
			return
		}

		// And a ring round each of them. Index.Locate is the inverse of a hit
		// test: chart 1 said which stage, this program worked out which edges,
		// and the sankey says where those edges are on screen. Nothing about
		// the layer changed, so the ring cannot disturb the reading.
		rings.At = rings.At[:0]
		for row, on := range lit {
			if !on {
				continue
			}
			if at, ok := flowLive.Index().Locate(0, 0, row); ok {
				rings.At = append(rings.At, at)
			}
		}
		flowLive.Draw()
	})

	if err := lineLive.Draw(); err != nil {
		return "", 0, err
	}
	if err := flowLive.Draw(); err != nil {
		return "", 0, err
	}

	// Stand in for a pointer: hover a mark belonging to the parse stage.
	// Locate is the reverse of a hit test, and is the same call a host makes
	// to draw its own marker over a row in the other chart.
	at, ok := markOf(lineLive, src, "stage", "parse")
	if !ok {
		return "", 0, fmt.Errorf("no mark was drawn for the parse stage")
	}
	lineLive.Move(float64(at.X), float64(at.Y))

	// The pictures, so that there is something to look at. The flow chart is
	// rendered as it now stands, which is with the hover's highlight on it.
	if err := line.Render(refract.SVG(throughputPath)); err != nil {
		return "", 0, err
	}
	if err := flow.Render(refract.SVG(flowPath)); err != nil {
		return "", 0, err
	}
	return hovered, highlighted, nil
}

// flowsThrough is the mapping refract does not own: which edges touch a stage.
// It is a scan here; in a real program it is whatever answers the question,
// and refract never needs to know. An empty key — the pointer is over nothing,
// or the layer named no key column — lights nothing.
func flowsThrough(stage string) []bool {
	if stage == "" {
		return nil
	}
	lit := make([]bool, len(edgeFrom))
	for i := range edgeFrom {
		lit[i] = edgeFrom[i] == stage || edgeTo[i] == stage
	}
	return lit
}

func count(lit []bool) int {
	n := 0
	for _, b := range lit {
		if b {
			n++
		}
	}
	return n
}

// markOf finds where a row carrying a value landed, so the example can point
// at it without a pointer.
func markOf(l *refract.Live, src data.Source, col, want string) (ir.Point, bool) {
	vals, ok := src.StringColumn(col)
	if !ok {
		return ir.Point{}, false
	}
	for row, v := range vals {
		if v != want {
			continue
		}
		if at, ok := l.Index().Locate(0, 0, row); ok {
			return at, true
		}
	}
	return ir.Point{}, false
}

// surface stands in for the canvas or window this example has no access to. It
// draws nothing; what is being demonstrated is the wiring, and the pictures
// are written with Plot.Render at the end. See backend/canvas and
// backend/window for the real things.
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
