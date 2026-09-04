<p align="center">
  <img src="docs/gopher.jpg" alt="A Go gopher holding a prism that splits a white beam into a spectrum of charts" width="640">
</p>

# refract

[![CI](https://github.com/timzifer/refract/actions/workflows/ci.yml/badge.svg)](https://github.com/timzifer/refract/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Ftimzifer%2Frefract%2Fmain%2Fdocs%2Fcoverage.json)](https://github.com/timzifer/refract/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/timzifer/refract.svg)](https://pkg.go.dev/github.com/timzifer/refract)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**A grammar-driven plotting library for Go: one model, many backends, runs
everywhere — built on the GoGPU stack.**

> **Status: pre-alpha.** This is milestone **v0.4**, "big data". The API is not
> stable; every release below `v1.0.0` may contain breaking changes without a
> deprecation cycle. See [CONCEPT.md](CONCEPT.md) for the design and the road
> ahead.

The name is the thesis: one beam enters a prism, a spectrum comes out. One chart
specification enters refract, a spectrum of output formats comes out.

![A damped sine over a time axis, dark theme](docs/images/signal.png)

---

## What it is

One declarative chart specification, rendered through interchangeable backends.
The core is **pure Go with no dependencies at all** — not "no cgo", literally
nothing outside the standard library — and emits both vector formats, SVG and
PDF. Add one module and the same specification renders to PNG and JPEG through
[`gogpu/gg`](https://github.com/gogpu/gg), still with `CGO_ENABLED=0`.

| | Dependencies | Output |
|---|---|---|
| `github.com/timzifer/refract` | **stdlib only** | SVG, PDF |
| `github.com/timzifer/refract/backend/gg` | GoGPU (`gg`), `x/image` — zero CGO | PNG, JPEG |
| `github.com/timzifer/refract/arrow` | `apache/arrow-go` — zero CGO | — (a data source) |

Raster, GPU, browser and interactive rendering all live behind the same
`ir.Backend` interface. The rest are milestones, not architecture changes.

## Install

```sh
go get github.com/timzifer/refract               # core: SVG and PDF, stdlib only
go get github.com/timzifer/refract/backend/gg    # raster: PNG and JPEG
go get github.com/timzifer/refract/arrow         # optional: plot Arrow data
```

Go 1.25 or newer ([why](docs/adr/0005-go-version.md)).

## Quick start

```go
package main

import (
	"log"
	"math"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

func main() {
	xs := make([]float64, 200)
	ys := make([]float64, 200)
	for i := range xs {
		xs[i] = float64(i) / 20
		ys[i] = math.Sin(xs[i])
	}

	p := refract.New(
		refract.Theme(theme.Dark),
		refract.Size(800, 400),
		refract.Title("Signal"),
		refract.YTitle("amplitude"),
	)
	p.X(scale.Linear(scale.Nice()))
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(
		refract.Float64Columns(map[string][]float64{"x": xs, "y": ys}),
		geom.X("x"), geom.Y("y"),
		geom.Color(palette.Blue),
		geom.Tension(0.4),
	))

	if err := p.Render(refract.SVG("signal.svg")); err != nil {
		log.Fatal(err)
	}
}
```

For PDF or raster, swap the target — nothing else changes:

```go
err := p.Render(refract.PDF("signal.pdf"))          // still stdlib only

import ggbackend "github.com/timzifer/refract/backend/gg"
err := p.Render(ggbackend.PNG("signal.png"))
```

A runnable version is in [`examples/signal`](examples/signal).

## Gallery

Every figure below is rendered by
[`backend/gg/cmd/gallery`](backend/gg/cmd/gallery) and re-checked in CI, so a
picture here cannot drift away from the code that produced it.

| | |
|---|---|
| ![Three series with a legend](docs/images/series.png) | ![Two groups of scattered points](docs/images/scatter.png) |
| ![A response time histogram](docs/images/bars.png) | ![A damped sine on a time axis](docs/images/signal.png) |
| ![An estimate with a shaded interval](docs/images/area.png) | ![A step chart of replica counts](docs/images/steps.png) |
| ![Bars by region, coloured by value with a colourbar](docs/images/categories.png) | ![Latency distributions as boxplots](docs/images/boxplot.png) |
| ![Two growth curves on a log axis](docs/images/logscale.png) | ![A series read against thresholds and a shaded window](docs/images/annotations.png) |
| ![Throughput faceted into one panel per region](docs/images/facets.png) | ![Four subplots on one dark canvas](docs/images/subplots.png) |
| ![A quarter of a million samples drawn as a clean line](docs/images/decimation.png) | ![A million points drawn as a density raster](docs/images/density.png) |

## What it does

- **Scales** — linear, time, **log**, **symlog** and **ordinal/categorical**.
  Linear tick placement uses
  [extended Wilkinson](https://rdrr.io/rforge/labeling/man/extended.html)
  (Talbot, Lin & Hanrahan 2010), so axis labels come out round rather than
  merely evenly spaced. Time ticks step in calendar units. Log and symlog
  subdivide each decade with unlabelled minor ticks; symlog is linear near zero,
  so signed data spanning orders of magnitude is plottable at all.
- **Geoms** — `Line` (optionally tension-smoothed), `Scatter` (six marker
  shapes), `Bar`, **`Area`** (to a baseline, or a band between two series),
  **`Step`** (pre/mid/post), **`Boxplot`** (Tukey whiskers, type-7 quartiles,
  outliers).
- **Colour** — a qualitative palette per chart, plus **continuous colour
  scales**: `geom.ColorBy` maps a column through a sequential or diverging ramp.
  Ramps interpolate in linear light, so a gradient has no dark band through its
  middle.
- **Missing data** — one explicit policy per layer (gap, interpolate, error),
  covering both `NaN`/`Inf` and values a scale has no position for, such as zero
  on a log axis.
- **Annotations** — `HLine`, `VLine`, `HBand`, `VBand`, `Segment`, `Region` and
  `Note`. They take values rather than a data source, because there is no column
  behind "the SLO is 200ms", and they extend the axis so the threshold is in
  view even when the data is nowhere near it.
- **Small multiples and subplots** — `facet.Wrap` and `facet.Grid` split one
  plot by a column; `refract.NewGrid` puts different plots on one canvas. Both
  go through one constraint solver, so panels are the same size and their axes
  line up ([ADR 0010](docs/adr/0010-panel-layout.md)).
- **Chart furniture** — axes, grid, tick labels with collision avoidance, chart
  and axis titles, and a guide column carrying a legend and **colourbars**.
- **Themes** — light and dark, built from a dozen
  [tokens](theme/tokens.go) rather than fifty fields, with a colourblind-safe
  ([Okabe-Ito](https://jfly.uni-koeln.de/color/)) default palette and
  perceptually uniform sequential ramps (Viridis, Cividis, Magma). `Theme.With`
  edits one; `theme.Register` and `theme.ByName` resolve one from a config file.
- **Big data** — a layer with more rows than the plot has pixels reduces itself
  before it draws: `stat.LTTB` for a line, min/max per pixel column for a
  staircase or a band, density binning to an image for a point cloud. It happens
  when the chart is drawn, never when the scales are trained, so the axes still
  report the data rather than the subset that survived
  ([ADR 0011](docs/adr/0011-decimation.md)).
- **Parallel panels** — a facet or a grid builds its panels on separate
  goroutines and replays them in panel order, so the output is byte-identical to
  a serial render ([ADR 0012](docs/adr/0012-parallel-panels.md)).
- **Data** — columnar and batch-oriented, carrying numeric, time and categorical
  columns. A `[]float64`-backed source is borrowed, never copied, and so is a
  null-free `float64` column read straight out of an **Apache Arrow** record
  through the optional `refract/arrow` module.
- **Backends** — two built-in emitters, SVG and PDF, and the gg raster adapter.

Deliberately **not** here yet: the JSON spec, interactivity, GPU, browser. Each
is a later milestone in
[CONCEPT.md §14](CONCEPT.md#14-roadmap--milestones).

## Categories, distributions and orders of magnitude

```go
src := refract.NewTable().
    String("region", []string{"north", "south", "east", "west"}).
    Float64("sales", []float64{18, 42, 31, 25})

p := refract.New(refract.Size(700, 400), refract.Title("Sales by region"))
p.X(scale.Ordinal())                        // equal slots, one per category
p.Y(scale.Linear(scale.Nice(), scale.Zero()))
p.Add(geom.Bar(src,
    geom.X("region"), geom.Y("sales"),
    geom.ColorBy("sales", scale.Sequential(palette.Viridis)),
))
```

An ordinal axis is a *band* scale: it tells the bar how wide to be, rather than
the bar guessing from the spacing of the data. The same applies to
`geom.Boxplot`. For data that spans decades, swap in `scale.Log(scale.LogNice())`
— or `scale.SymLog()` when it also crosses zero.

A runnable version, together with a boxplot over the same kind of data, is in
[`examples/categories`](examples/categories).

## Small multiples

```go
p := refract.New(refract.Size(900, 520), refract.Title("Throughput by region"))
p.Add(
    geom.Line(src, geom.X("hour"), geom.Y("rps"), geom.Label("throughput")),
    geom.HLine(60, geom.Label("target")),          // no data: drawn on every panel
)
p.Facet(facet.Wrap("region", facet.Columns(3)))
```

Panels share their scales by default, which is what makes small multiples
comparable at a glance. `facet.FreeX`, `facet.FreeY` and `facet.Free` give each
panel its own — a deliberate choice, because a reader who does not notice the
axes changed will read the panels as comparable when they are not.

For unrelated charts on one canvas, build a grid of plots instead:

```go
g := refract.NewGrid(2, refract.GridSize(900, 560), refract.GridTitle("Fleet"))
g.Add(latency, throughput, errors, saturation)
err := g.Render(refract.PDF("overview.pdf"))
```

A runnable version of both, with annotations and PDF output, is in
[`examples/dashboard`](examples/dashboard).

## A million rows

```go
p := refract.New(refract.Size(800, 500), refract.Title("A million samples"))
p.Add(geom.Line(src, geom.X("i"), geom.Y("v")))   // nothing else needed
```

That renders in about 60 ms into under 30 kB of SVG. Drawing every row takes six
times as long and produces 15 MB — of a picture that is 800 pixels wide, so the
extra 999,000 vertices land on top of each other.

The layer sees how many rows it has against how wide the plot is and reduces
itself accordingly: `LTTB` for a line, min/max per pixel column for a step or a
band, a density raster for a scatter dense enough that its markers would bury
one another. Override it per layer when the default is not what you want:

```go
geom.Line(src, geom.X("i"), geom.Y("v"), geom.Decimate(geom.MinMax))    // keep every spike
geom.Line(src, geom.X("i"), geom.Y("v"), geom.Decimate(geom.NoDecimation)) // every row
geom.Scatter(src, geom.X("x"), geom.Y("y"), geom.Budget(4000))          // at most 4000 marks
```

The reduction happens when the chart is drawn, not when its scales are trained,
so the axes are the data's either way — a spike survives the reduction *and* the
axis still reaches it.

The same milestone made a redrawn chart cheap: everything sized by the data comes
from a pool, so a steady-state frame over a million rows costs the same handful
of allocations as one over a thousand. There is a test that fails if that stops
being true.

A runnable version — two million samples with a spike and a dropout in them, and
a million-point cloud — is in [`examples/bigdata`](examples/bigdata).

## Plotting Arrow data

```go
import "github.com/timzifer/refract/arrow"

src := arrow.Source(rec)      // rec is an arrow.Record
p.Add(geom.Line(src, geom.X("t"), geom.Y("p99")))
```

A `float64` column with no nulls is Arrow's own buffer — no copy, no conversion.
Everything else (integers, `float32`, timestamps, dictionary-encoded strings)
converts once on first use and is cached, so a record with forty columns and a
chart that plots two pays for two. An Arrow null becomes `NaN`, which means the
missing-data policy you already set covers it
([ADR 0013](docs/adr/0013-arrow-adapter.md)).

## How it fits together

```
   Your spec  ──►  Model  ──►  IR  ──►  Backend  ──►  output
   ─────────      ─────      ────      ───────       ──────
   geoms          scales     ~8        backend/svg    SVG
   scales         layout     drawing   backend/pdf    PDF
   theme          ticks      ops       backend/gg     PNG / JPEG
   facets         panels               (future)       GPU, browser
```

The `ir.Backend` interface is the seam. Geoms never touch a renderer; a renderer
never knows what a scale is. That is what lets refract stand on a young,
fast-moving graphics stack without being welded to it — the whole gg adapter is
about 300 lines ([why that matters](docs/adr/0006-gg-coupling-surface.md)).

## Documentation

- [CONCEPT.md](CONCEPT.md) — the design document: motivation, positioning,
  architecture, roadmap.
- [docs/adr](docs/adr) — why the open questions were answered the way they were.
- [CONTRIBUTING.md](CONTRIBUTING.md) — building a two-module repository, and how
  to regenerate golden files and figures.

## License

MIT. The core links nothing; `backend/gg` links only permissively licensed code
(gg is MIT, `x/image` is BSD-3-Clause). That is a requirement rather than a
preference — refract must be embeddable by downstream projects under any
license.
