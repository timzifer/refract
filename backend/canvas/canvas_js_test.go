//go:build js && wasm

package canvas_test

import (
	"image"
	"strconv"
	"strings"
	"syscall/js"
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/backend/canvas"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

// The canvas backend is tested against a recording 2D context rather than
// against pixels.
//
// That is the same choice the rest of the repository makes — assert on the
// primitives that were emitted, not on a rasterization of them — and here it
// is also the only choice available: node has no canvas, and a test that
// needed one would be a test nobody could run. What matters is that refract
// asks the context for the right things, which is exactly what this records.

type call struct {
	name string
	args []string
}

type fake struct {
	t     *testing.T
	ctx   js.Value
	calls []call
	fns   []js.Func
}

func newFake(t *testing.T) *fake {
	t.Helper()
	f := &fake{t: t, ctx: js.Global().Get("Object").New()}

	// Path2D keeps the path data it was built with, so a test can read back
	// the geometry refract asked for.
	f.define(js.Global(), "Path2D", func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			this.Set("d", args[0])
		}
		return nil
	})

	for _, name := range []string{
		"save", "restore", "beginPath", "stroke", "fill", "clip", "clearRect",
		"setTransform", "transform", "translate", "rotate", "fillText",
		"setLineDash", "drawImage", "putImageData", "getContext",
	} {
		f.record(name)
	}
	f.define(f.ctx, "measureText", func(_ js.Value, args []js.Value) any {
		f.calls = append(f.calls, call{"measureText", []string{args[0].String()}})
		m := js.Global().Get("Object").New()
		// Six units per character at a nominal 10px, which is close enough to
		// a real sans-serif that layout behaves.
		m.Set("width", 0.6*float64(len(args[0].String()))*f.fontSize())
		m.Set("fontBoundingBoxAscent", 0.8*f.fontSize())
		m.Set("fontBoundingBoxDescent", 0.2*f.fontSize())
		return m
	})
	return f
}

func (f *fake) fontSize() float64 {
	font := f.ctx.Get("font")
	if font.Type() != js.TypeString {
		return 10
	}
	var size float64
	for _, r := range font.String() {
		if r >= '0' && r <= '9' {
			size = size*10 + float64(r-'0')
			continue
		}
		if r == '.' || r == 'p' {
			break
		}
	}
	if size == 0 {
		return 10
	}
	return size
}

func (f *fake) define(on js.Value, name string, fn func(js.Value, []js.Value) any) {
	cb := js.FuncOf(fn)
	f.fns = append(f.fns, cb)
	on.Set(name, cb)
}

func (f *fake) record(name string) {
	f.define(f.ctx, name, func(_ js.Value, args []js.Value) any {
		c := call{name: name}
		for _, a := range args {
			c.args = append(c.args, describe(a))
		}
		f.calls = append(f.calls, c)
		return nil
	})
}

func (f *fake) release() {
	for _, cb := range f.fns {
		cb.Release()
	}
}

// describe renders one argument as a string: a Path2D as its path data, a
// number as itself, and any other object as a placeholder.
func describe(v js.Value) string {
	switch v.Type() {
	case js.TypeNumber:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case js.TypeString:
		return v.String()
	case js.TypeObject:
		if d := v.Get("d"); d.Type() == js.TypeString {
			return d.String()
		}
		return "[object]"
	}
	return v.String()
}

func (f *fake) names() []string {
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = c.name
	}
	return out
}

func (f *fake) count(name string) int {
	n := 0
	for _, c := range f.calls {
		if c.name == name {
			n++
		}
	}
	return n
}

func (f *fake) argsOf(name string) [][]string {
	var out [][]string
	for _, c := range f.calls {
		if c.name == name {
			out = append(out, c.args)
		}
	}
	return out
}

func linePlot() *refract.Plot {
	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3},
		"y": {0, 2, 1, 3},
	})
	p := refract.New(refract.Size(400, 300), refract.Title("Canvas"))
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y"), geom.Color(palette.Blue)))
	return p
}

func TestAChartDrawsOnTheContext(t *testing.T) {
	f := newFake(t)
	defer f.release()

	if err := linePlot().Render(canvas.Context(f.ctx)); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if f.count("fill") == 0 {
		t.Error("nothing was filled: the background and the plot area are fills")
	}
	if f.count("stroke") == 0 {
		t.Error("nothing was stroked: the axes and the line are strokes")
	}
	if f.count("fillText") == 0 {
		t.Error("no text was drawn: the title and the tick labels are text")
	}
	if f.count("measureText") == 0 {
		t.Error("no text was measured: layout sizes its margins from measurements")
	}

	// The data line is a path through four points, in the layer's colour.
	var found bool
	for _, args := range f.argsOf("stroke") {
		if len(args) == 1 && strings.Count(args[0], "L") == 3 && strings.HasPrefix(args[0], "M") {
			found = true
		}
	}
	if !found {
		t.Errorf("no four-point polyline in %v", f.argsOf("stroke"))
	}
	if got := f.ctx.Get("strokeStyle").String(); got == "" {
		t.Error("the stroke colour was never set")
	}
}

func TestEveryPathIsOneCall(t *testing.T) {
	f := newFake(t)
	defer f.release()

	src := refract.Float64Columns(map[string][]float64{
		"x": make([]float64, 500),
		"y": make([]float64, 500),
	})
	xs, _ := src.Float64Column("x")
	ys, _ := src.Float64Column("y")
	for i := range xs {
		xs[i], ys[i] = float64(i), float64(i%17)
	}

	p := refract.New(refract.Size(400, 300))
	p.Add(geom.Line(src, geom.X("x"), geom.Y("y")))
	if err := p.Render(canvas.Context(f.ctx)); err != nil {
		t.Fatalf("Render: %v", err)
	}

	// A polyline is one Path2D and one stroke, however many points it has.
	// Crossing the WebAssembly boundary is what costs, so a chart over five
	// hundred rows must not make five hundred calls.
	if n := len(f.calls); n > 100 {
		t.Errorf("a 500-point line took %d context calls; the path should be one string", n)
	}
	if f.count("beginPath") != 0 {
		t.Error("the backend built a path with beginPath rather than Path2D")
	}
}

func TestMarkersAreOneCallPerStyle(t *testing.T) {
	f := newFake(t)
	defer f.release()

	src := refract.Float64Columns(map[string][]float64{
		"x": {0, 1, 2, 3, 4, 5},
		"y": {0, 1, 0, 1, 0, 1},
	})
	p := refract.New(refract.Size(400, 300))
	p.Add(geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.Shape(ir.MarkerSquare)))
	if err := p.Render(canvas.Context(f.ctx)); err != nil {
		t.Fatalf("Render: %v", err)
	}

	var markers string
	for _, args := range f.argsOf("fill") {
		if len(args) > 0 && strings.Count(args[0], "M") == 6 {
			markers = args[0]
		}
	}
	if markers == "" {
		t.Fatalf("the six markers were not drawn as one path: %v", f.argsOf("fill"))
	}
	if strings.Count(markers, "Z") != 6 {
		t.Errorf("expected six closed squares, got %q", markers)
	}
}

func TestMeasureUsesTheContextFont(t *testing.T) {
	f := newFake(t)
	defer f.release()

	tgt := canvas.Context(f.ctx)
	b, err := tgt.Open(400, 300, 1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	m := b.Measure(ir.TextRun{Text: "hello", Font: ir.FontRef{Size: 20}})
	if m.Advance <= 0 {
		t.Fatalf("Measure reported no advance: %+v", m)
	}
	if m.Ascent <= 0 || m.Descent <= 0 {
		t.Errorf("Measure reported no font box: %+v", m)
	}
	// The fake reports 0.6em per character; the point is that the metrics came
	// from the context rather than from a table refract carries.
	if want := float32(0.6 * 5 * 20); m.Advance != want {
		t.Errorf("Advance = %v, want %v — the metrics did not come from measureText", m.Advance, want)
	}
}

func TestDamageClipsAndClears(t *testing.T) {
	f := newFake(t)
	defer f.release()

	tgt := canvas.Context(f.ctx)
	b, err := tgt.Open(400, 300, 1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	partial, ok := b.(ir.Partial)
	if !ok {
		t.Fatal("the canvas backend does not implement ir.Partial")
	}

	f.calls = nil
	partial.Damage([]ir.Rect{ir.R(10, 20, 30, 40)})
	if f.count("clip") != 1 {
		t.Errorf("damage did not clip: %v", f.names())
	}
	rects := f.argsOf("clearRect")
	if len(rects) != 1 || rects[0][0] != "10" || rects[0][2] != "20" {
		t.Errorf("damage cleared %v, want the one 20x20 rectangle at 10,20", rects)
	}
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	if f.count("restore") != 1 {
		t.Errorf("the damage clip was not unwound on Flush: %v", f.names())
	}

	// A nil list is the whole frame: no clip, one clear of everything.
	f.calls = nil
	partial.Damage(nil)
	if f.count("clip") != 0 {
		t.Errorf("a full repaint clipped: %v", f.names())
	}
	if got := f.argsOf("clearRect"); len(got) != 1 || got[0][2] != "400" {
		t.Errorf("a full repaint cleared %v, want the whole canvas", got)
	}
}

func TestElementIsSizedForTheDevicePixelRatio(t *testing.T) {
	f := newFake(t)
	defer f.release()

	el := js.Global().Get("Object").New()
	el.Set("style", js.Global().Get("Object").New())
	f.define(el, "getContext", func(js.Value, []js.Value) any { return f.ctx })

	if _, err := canvas.Element(el).Open(400, 300, 2); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := el.Get("width").Int(); got != 800 {
		t.Errorf("backing store width = %d, want 800 at dpr 2", got)
	}
	if got := el.Get("style").Get("width").String(); got != "400px" {
		t.Errorf("CSS width = %q, want 400px", got)
	}
	// The transform carries the ratio, so no coordinate in refract changes.
	if got := f.argsOf("setTransform"); len(got) == 0 || got[0][0] != "2" {
		t.Errorf("setTransform = %v, want the device pixel ratio", got)
	}
}

func TestAnImageGoesThroughAScratchCanvas(t *testing.T) {
	f := newFake(t)
	defer f.release()

	scratch := js.Global().Get("Object").New()
	f.define(scratch, "getContext", func(js.Value, []js.Value) any { return f.ctx })
	f.define(js.Global(), "OffscreenCanvas", func(this js.Value, args []js.Value) any {
		this.Set("width", args[0])
		this.Set("height", args[1])
		this.Set("getContext", scratch.Get("getContext"))
		return nil
	})
	f.define(js.Global(), "ImageData", func(js.Value, []js.Value) any { return nil })

	tgt := canvas.Context(f.ctx)
	b, err := tgt.Open(400, 300, 1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	b.Image(img, ir.R(0, 0, 100, 100))

	if f.count("putImageData") != 1 {
		t.Errorf("the pixels were not put onto a scratch canvas: %v", f.names())
	}
	if got := f.argsOf("drawImage"); len(got) != 1 || got[0][3] != "100" {
		t.Errorf("drawImage = %v, want the raster scaled into a 100x100 box", got)
	}
}
