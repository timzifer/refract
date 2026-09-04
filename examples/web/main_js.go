//go:build js && wasm

// Command web draws an interactive chart in a browser.
//
// Build it and serve the directory:
//
//	GOOS=js GOARCH=wasm go build -o examples/web/chart.wasm ./examples/web
//	cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" examples/web/
//	go run ./examples/web/serve   # or any static file server
//
// It is the v0.5 browser story end to end: the same model that renders SVG on
// a server draws on a canvas, a pointer over it reports the row underneath,
// the wheel zooms, a drag pans, and a double click resets the view.
package main

import (
	"math"
	"math/rand/v2"
	"strconv"
	"syscall/js"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/backend/canvas"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func main() {
	doc := js.Global().Get("document")
	el := doc.Call("getElementById", "chart")
	readout := doc.Call("getElementById", "readout")

	p := plot()

	// The tooltip. A handler is told what the pointer is over; drawing the
	// result is the page's business, not refract's.
	p.On(refract.Hover, func(ev refract.Event) {
		if !ev.Found {
			readout.Set("textContent", "—")
			return
		}
		readout.Set("textContent", ev.Series()+": "+
			strconv.FormatFloat(ev.Hit.X, 'f', 2, 64)+", "+
			strconv.FormatFloat(ev.Hit.Y, 'f', 2, 64))
	})
	p.On(refract.Leave, func(refract.Event) { readout.Set("textContent", "—") })

	live, err := p.Live(canvas.Element(el, canvas.Clear(true)))
	if err != nil {
		js.Global().Get("console").Call("error", err.Error())
		return
	}
	defer live.Close()

	if err := live.Draw(); err != nil {
		js.Global().Get("console").Call("error", err.Error())
		return
	}
	defer live.Bind(el)()

	// A wasm main that returns takes the page's Go runtime with it, so park.
	select {}
}

func plot() *refract.Plot {
	sample, value, load := series(4000)
	src := refract.Float64Columns(map[string][]float64{
		"sample": sample, "value": value, "load": load,
	})

	p := refract.New(
		refract.Theme(theme.Light),
		refract.Size(900, 460),
		refract.Title("Interactive"),
		refract.XTitle("sample"),
		refract.YTitle("value"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(
		geom.Line(src, geom.X("sample"), geom.Y("value"), geom.Color(palette.Blue), geom.Label("signal")),
		// No Label here: a layer coloured from a column titles its colourbar
		// with the label when it has one, and "load" is what that bar shows.
		// The tooltip falls back to the column the layer plots, which reads as
		// "value" — see render.layerLabel.
		geom.Scatter(src, geom.X("sample"), geom.Y("value"),
			geom.ColorBy("load", scale.Sequential(palette.Viridis)), geom.Size(4)),
		geom.HLine(2, geom.Label("threshold"), geom.Dash(6, 4)),
	)
	return p
}

// series is four thousand rows, which is enough that the chart is decimated
// when it is zoomed out and not when it is zoomed in — the thing an
// interactive chart is for.
func series(n int) (sample, value, load []float64) {
	r := rand.New(rand.NewPCG(17, 19))
	sample, value, load = make([]float64, n), make([]float64, n), make([]float64, n)
	for i := range n {
		sample[i] = float64(i)
		value[i] = math.Sin(float64(i)/90) + 0.35*r.NormFloat64()
		load[i] = math.Abs(value[i])
	}
	value[n/3] = 3.2 // a spike, so that decimation has something to keep
	return sample, value, load
}
