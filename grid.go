package refract

import (
	"errors"

	"github.com/timzifer/refract/render"
	themepkg "github.com/timzifer/refract/theme"
)

// Grid renders several plots together in one image, with their axes aligned.
//
// It is the other half of the multi-panel story: [Plot.Facet] splits one plot
// by a column, and a Grid puts different plots side by side. Both go through
// the same solver, so the panels line up either way.
//
//	g := refract.NewGrid(2, refract.GridSize(900, 600), refract.GridTitle("Fleet"))
//	g.Add(latency, throughput, errors, saturation)
//	err := g.Render(refract.SVG("fleet.svg"))
//
// A member plot contributes its layers, its scales and its title, which
// becomes the label above its panel. The canvas is the grid's: its size,
// theme, chart title and axis titles are the ones used, and a member plot's
// own size, theme and axis titles are not. That is the price of one image —
// two panels cannot disagree about the colour of the paper they are printed
// on.
type Grid struct {
	width, height int
	dpr           float64
	theme         themepkg.Theme

	title  string
	xTitle string
	yTitle string

	cols  int
	cells []gridCell

	legend    bool
	legendSet bool
}

type gridCell struct {
	row, col int
	plot     *Plot
}

// GridOption configures a Grid at construction.
type GridOption func(*Grid)

// GridSize sets the output size in device-independent pixels. The default is
// 900x600, which is a grid's worth rather than a single chart's.
func GridSize(w, h int) GridOption {
	return func(g *Grid) {
		if w > 0 {
			g.width = w
		}
		if h > 0 {
			g.height = h
		}
	}
}

// GridDPR sets the device pixel ratio. See [DPR].
func GridDPR(r float64) GridOption {
	return func(g *Grid) {
		if r > 0 {
			g.dpr = r
		}
	}
}

// GridTheme sets the visual tokens for the whole grid.
func GridTheme(t themepkg.Theme) GridOption { return func(g *Grid) { g.theme = t } }

// GridTitle sets the title above the grid.
func GridTitle(s string) GridOption { return func(g *Grid) { g.title = s } }

// GridAxisTitles labels the shared axes, once for the grid. Panels keep their
// own scales; these name what those scales measure.
func GridAxisTitles(x, y string) GridOption {
	return func(g *Grid) { g.xTitle, g.yTitle = x, y }
}

// GridLegend forces the legend on or off. By default it appears when any
// panel would have shown one.
func GridLegend(show bool) GridOption {
	return func(g *Grid) { g.legend, g.legendSet = show, true }
}

// NewGrid creates a grid that flows plots into rows of cols panels.
func NewGrid(cols int, opts ...GridOption) *Grid {
	g := &Grid{
		width:  900,
		height: 600,
		dpr:    1,
		theme:  themepkg.Light,
		cols:   cols,
	}
	if g.cols < 1 {
		g.cols = 1
	}
	for _, o := range opts {
		o(g)
	}
	return g
}

// Add appends plots, filling the grid left to right and wrapping.
func (g *Grid) Add(ps ...*Plot) *Grid {
	for _, p := range ps {
		n := len(g.cells)
		g.cells = append(g.cells, gridCell{row: n / g.cols, col: n % g.cols, plot: p})
	}
	return g
}

// At places a plot in a specific cell, replacing whatever was there. Cells
// left empty stay empty, which is how a grid is given a deliberate hole.
func (g *Grid) At(row, col int, p *Plot) *Grid {
	if row < 0 || col < 0 {
		return g
	}
	for i := range g.cells {
		if g.cells[i].row == row && g.cells[i].col == col {
			g.cells[i].plot = p
			return g
		}
	}
	g.cells = append(g.cells, gridCell{row: row, col: col, plot: p})
	if col >= g.cols {
		g.cols = col + 1
	}
	return g
}

// ErrEmptyGrid reports a render of a grid with no plots in it.
var ErrEmptyGrid = errors.New("refract: grid has no plots")

// Render draws the grid into t.
func (g *Grid) Render(t Target) (err error) {
	if t == nil {
		return errors.New("refract: nil render target")
	}
	c, err := g.chart()
	if err != nil {
		return err
	}

	b, err := t.Open(g.width, g.height, g.dpr)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := t.Close(); err == nil {
			err = cerr
		}
	}()

	if err = render.Draw(b, c); err != nil {
		return err
	}
	return b.Flush()
}

func (g *Grid) chart() (render.Chart, error) {
	c := render.Chart{
		Width:      g.width,
		Height:     g.height,
		DPR:        g.dpr,
		Theme:      g.theme,
		Title:      g.title,
		XTitle:     g.xTitle,
		YTitle:     g.yTitle,
		ShowLegend: g.showLegend(),
	}
	rows := 0
	for _, cell := range g.cells {
		if cell.plot == nil {
			continue
		}
		rows = max(rows, cell.row+1)
		c.Panels = append(c.Panels, render.Panel{
			Row: cell.row,
			Col: cell.col,
			// A subplot's title names its panel. There is one chart title, and
			// it belongs to the grid.
			Strip:  cell.plot.title,
			X:      cell.plot.scaleX(),
			Y:      cell.plot.scaleY(),
			Layers: cell.plot.layers,
			// Every panel has scales of its own, so every panel writes its own
			// axes: the numbers on one are not the numbers on the next.
			ShowX: true,
			ShowY: true,
		})
	}
	if len(c.Panels) == 0 {
		return render.Chart{}, ErrEmptyGrid
	}
	c.Rows, c.Cols = rows, g.cols
	return c, nil
}

func (g *Grid) showLegend() bool {
	if g.legendSet {
		return g.legend
	}
	for _, cell := range g.cells {
		if cell.plot != nil && cell.plot.showLegend() {
			return true
		}
	}
	return false
}
