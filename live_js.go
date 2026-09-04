//go:build js && wasm

package refract

import "syscall/js"

// Bind wires a DOM element's pointer input to the chart and returns a function
// that unwires it.
//
//	live, _ := p.Live(canvas.Element(el))
//	defer live.Close()
//	live.Draw()
//	defer live.Bind(el)()
//
// It is here rather than in the canvas backend because it is not drawing: a
// backend consumes IR and must not know about scales or panels, and this
// translates a browser event into a pan, a zoom or a hover. The canvas backend
// draws; this steers.
//
// The steering itself is [Input], which is portable — a native window drives
// the same state machine from its own event loop. This is the DOM half: which
// events to listen to, where the pointer is in them, and how a browser counts
// a wheel.
//
// What it wires:
//
//   - moving the pointer hovers, and leaving the element fires [Leave];
//   - pressing and releasing without moving clicks;
//   - the wheel zooms about the pointer, and the page does not scroll;
//   - dragging pans;
//   - a double click autoscales, which is the "reset view" every interactive
//     chart needs and the one control a reader looks for first.
//
// Positions come from the event's offsetX and offsetY, which are CSS pixels
// relative to the element — the same device-independent units a chart is laid
// out in, whatever the device pixel ratio is.
//
// Handlers registered with [Plot.On] fire from here, on the browser's event
// loop goroutine.
func (l *Live) Bind(el js.Value) (unbind func()) {
	var (
		in         = l.Input()
		listeners  []js.Func
		names      []string
		addHandler func(name string, fn func(js.Value))
	)

	addHandler = func(name string, fn func(js.Value)) {
		cb := js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) > 0 {
				fn(args[0])
			}
			return nil
		})
		listeners = append(listeners, cb)
		names = append(names, name)
		el.Call("addEventListener", name, cb)
	}

	at := func(ev js.Value) (float64, float64) {
		return ev.Get("offsetX").Float(), ev.Get("offsetY").Float()
	}

	addHandler("pointermove", func(ev js.Value) {
		x, y := at(ev)
		in.Move(x, y)
	})
	addHandler("pointerleave", func(js.Value) { in.Leave() })
	addHandler("pointerdown", func(ev js.Value) {
		x, y := at(ev)
		in.Down(x, y)
		if id := ev.Get("pointerId"); id.Truthy() {
			el.Call("setPointerCapture", id)
		}
	})
	// The click comes from the release rather than from the DOM's own click
	// event, because a drag that ends over a mark is not a click on it — and
	// the browser fires click for that too.
	addHandler("pointerup", func(ev js.Value) {
		x, y := at(ev)
		in.Up(x, y)
	})
	addHandler("dblclick", func(js.Value) { in.DoubleClick() })
	addHandler("wheel", func(ev js.Value) {
		ev.Call("preventDefault")
		x, y := at(ev)
		in.Wheel(x, y, wheelDelta(ev))
	})

	return func() {
		for i, cb := range listeners {
			el.Call("removeEventListener", names[i], cb)
			cb.Release()
		}
	}
}

// wheelDelta reads a wheel event in the pixel convention [Input.Wheel] takes.
//
// A browser reports the wheel in one of three units, and says which in
// deltaMode: pixels, lines, or pages. Line and page modes report a handful of
// units where the pixel mode reports a hundred, so they are scaled up to it
// rather than zooming a fortieth as far on the trackpads that use them.
func wheelDelta(ev js.Value) float64 {
	delta := ev.Get("deltaY").Float()
	if mode := ev.Get("deltaMode"); mode.Type() == js.TypeNumber && mode.Int() != 0 {
		delta *= 40
	}
	return delta
}
