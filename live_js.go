//go:build js && wasm

package refract

import (
	"math"
	"syscall/js"
)

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
// What it wires:
//
//   - moving the pointer hovers, and leaving the element fires [Leave];
//   - a click clicks;
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
		dragging   bool
		lastX      float64
		lastY      float64
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
		if dragging {
			l.PanBy(x-lastX, y-lastY)
			lastX, lastY = x, y
			return
		}
		l.Move(x, y)
	})
	addHandler("pointerleave", func(js.Value) {
		dragging = false
		l.Leave()
	})
	addHandler("pointerdown", func(ev js.Value) {
		dragging = true
		lastX, lastY = at(ev)
		if id := ev.Get("pointerId"); id.Truthy() {
			el.Call("setPointerCapture", id)
		}
	})
	addHandler("pointerup", func(js.Value) { dragging = false })
	addHandler("click", func(ev js.Value) {
		x, y := at(ev)
		l.Click(x, y)
	})
	addHandler("dblclick", func(js.Value) { l.Autoscale() })
	addHandler("wheel", func(ev js.Value) {
		ev.Call("preventDefault")
		x, y := at(ev)
		l.Wheel(x, y, wheelFactor(ev))
	})

	return func() {
		for i, cb := range listeners {
			el.Call("removeEventListener", names[i], cb)
			cb.Release()
		}
	}
}

// wheelFactor turns a wheel event into a zoom factor.
//
// The exponential keeps a fast scroll from inverting: any deltaY maps into
// (0, ∞) and never through zero, so holding the wheel down zooms smoothly
// rather than jumping. The rate is chosen so that one notch on a mouse — 100
// units in the browser's pixel mode — is about 10%, which is the step a reader
// expects from a map.
func wheelFactor(ev js.Value) float64 {
	delta := ev.Get("deltaY").Float()
	if mode := ev.Get("deltaMode"); mode.Type() == js.TypeNumber && mode.Int() != 0 {
		// Line and page modes report a handful of units rather than a hundred.
		delta *= 40
	}
	return math.Exp(delta / 1000)
}
