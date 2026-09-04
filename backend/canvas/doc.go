// Package canvas draws a chart into a browser canvas.
//
// It is the browser half of v0.5, and it is in the core module because it
// needs nothing to be there: a `<canvas>` element's 2D context is reached
// through `syscall/js`, which is the standard library, so refract in a browser
// links exactly what refract on a server links.
//
//	//go:build js && wasm
//
//	el := js.Global().Get("document").Call("getElementById", "chart")
//	live, err := p.Live(canvas.Element(el))
//	defer live.Close()
//	live.Draw()
//	canvas.Bind(el, live)   // wire pointer, wheel and drag
//
// # Only on js/wasm
//
// Every symbol in this package is behind `js && wasm`, so on any other
// platform the package is empty. That is deliberate: a stub that compiled
// everywhere and failed at run time would let a chart reach production before
// finding out there is no canvas on a Linux server.
//
// # Not WebGPU
//
// CONCEPT.md's roadmap put browser rendering behind the gg backend, on WebGPU
// with a canvas fallback. gg has no js support at the version this backend was
// written against — no `syscall/js` anywhere in it — so there is no WebGPU
// path to take and no canvas fallback to fall back to. refract therefore draws
// on the 2D context itself, exactly as it emits its own PDF for the same kind
// of reason. See docs/adr/0017-browser-backend.md.
package canvas
