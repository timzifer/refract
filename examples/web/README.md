# The browser example

The same chart model that renders SVG on a server, drawing on a `<canvas>`
through `backend/canvas`, with hover, wheel zoom, drag pan and a double click
to reset.

```sh
GOOS=js GOARCH=wasm go build -o examples/web/chart.wasm ./examples/web
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" examples/web/
```

Then serve this directory with any static file server and open it. The
`.wasm` and `wasm_exec.js` files are build output and are not committed.

## What to look at

- `main_js.go` is the whole application: build a `*refract.Plot` exactly as a
  server would, open it with `p.Live(canvas.Element(el))`, register handlers
  with `p.On`, and wire the DOM to it with `live.Bind(el)`.
- Nothing in the chart is browser-specific. The plot could be marshalled to
  JSON here and rendered to PDF on a server, or the other way round — see
  package `spec`.
