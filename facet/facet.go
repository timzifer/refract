// Package facet splits one chart into small multiples.
//
// A facet takes the layers of a plot, cuts their rows by the values of a
// column, and hands each group its own panel. Every panel is drawn with the
// same geoms and — unless told otherwise — the same scales, which is the whole
// point: small multiples work because the only thing that varies between
// panels is the data.
//
//	p.Facet(facet.Wrap("region", facet.Columns(3)))
//
// [Wrap] flows panels left to right and wraps; [Grid] crosses two columns, one
// down the rows and one across the columns. Either way the panels' axes are
// aligned by the layout solver, so a position in one panel means the same as
// the same position in the next.
//
// # Scales
//
// Shared by default. [FreeX], [FreeY] and [Free] give each panel a scale of
// its own, which answers a different question — "what is the shape of this
// group" rather than "how do these groups compare" — and should be a
// deliberate choice, because a reader who does not notice the axes changed
// will read the panels as comparable when they are not.
//
// # Layers without data
//
// A layer that holds no rows — an annotation, a reference line — cannot be
// split, so it is drawn in every panel. That is what you want: a threshold
// applies to all of them.
package facet

import (
	"errors"
	"fmt"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
)

// Spec describes how a plot is split into panels.
type Spec struct {
	rowCol, col string
	wrap        bool
	columns     int
	freeX       bool
	freeY       bool
}

// Option configures a facet.
type Option func(*Spec)

// Columns caps how many panels a [Wrap] puts in a row. The default is derived
// from the panel count, aiming for a roughly square grid.
func Columns(n int) Option {
	return func(s *Spec) {
		if n > 0 {
			s.columns = n
		}
	}
}

// FreeX gives each panel its own horizontal scale.
func FreeX() Option { return func(s *Spec) { s.freeX = true } }

// FreeY gives each panel its own vertical scale.
func FreeY() Option { return func(s *Spec) { s.freeY = true } }

// Free gives each panel its own scales on both axes.
func Free() Option { return func(s *Spec) { s.freeX, s.freeY = true, true } }

// Wrap splits by one column, flowing panels left to right and wrapping onto
// the next row.
func Wrap(col string, opts ...Option) *Spec {
	s := &Spec{col: col, wrap: true}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Grid crosses two columns: rowCol runs down the rows, colCol across the
// columns. A panel exists for every combination that has rows; the rest are
// holes, which is itself information.
func Grid(rowCol, colCol string, opts ...Option) *Spec {
	s := &Spec{rowCol: rowCol, col: colCol}
	for _, o := range opts {
		o(s)
	}
	return s
}

// FreeScales reports whether each panel gets its own X or Y scale.
func (s *Spec) FreeScales() (x, y bool) { return s.freeX, s.freeY }

// Panel is one facet: where it sits, what it is called, and the layers cut
// down to its rows.
type Panel struct {
	// Row and Col place the panel in the grid.
	Row, Col int
	// Strip is the label above the panel.
	Strip string
	// RightStrip is the label beside it, used by [Grid] to name a row once
	// rather than on every panel in it.
	RightStrip string
	// Layers are the plot's layers restricted to this panel's rows. Layers
	// that hold no data are carried through unchanged.
	Layers []geom.Geom
}

// ErrNoColumn reports a facet column that none of the layers has.
var ErrNoColumn = errors.New("refract/facet: column not found in any layer")

// Split cuts layers into panels and reports the shape of the grid.
func (s *Spec) Split(layers []geom.Geom) (panels []Panel, rows, cols int, err error) {
	if s == nil {
		return nil, 0, 0, errors.New("refract/facet: nil spec")
	}
	if s.col == "" && s.rowCol == "" {
		return nil, 0, 0, errors.New("refract/facet: no column to split on")
	}
	if s.wrap {
		return s.splitWrap(layers)
	}
	return s.splitGrid(layers)
}

func (s *Spec) splitWrap(layers []geom.Geom) ([]Panel, int, int, error) {
	keys, err := keysOf(layers, s.col)
	if err != nil {
		return nil, 0, 0, err
	}
	cols := s.columns
	if cols <= 0 {
		cols = squarish(len(keys))
	}
	if cols > len(keys) {
		cols = len(keys)
	}
	rows := (len(keys) + cols - 1) / cols

	panels := make([]Panel, 0, len(keys))
	for i, k := range keys {
		sel, err := selection(layers, map[string]string{s.col: k})
		if err != nil {
			return nil, 0, 0, err
		}
		panels = append(panels, Panel{
			Row:    i / cols,
			Col:    i % cols,
			Strip:  k,
			Layers: sel,
		})
	}
	return panels, rows, cols, nil
}

func (s *Spec) splitGrid(layers []geom.Geom) ([]Panel, int, int, error) {
	rowKeys, err := keysOf(layers, s.rowCol)
	if err != nil {
		return nil, 0, 0, err
	}
	colKeys, err := keysOf(layers, s.col)
	if err != nil {
		return nil, 0, 0, err
	}

	var panels []Panel
	for ri, rk := range rowKeys {
		for ci, ck := range colKeys {
			sel, err := selection(layers, map[string]string{s.rowCol: rk, s.col: ck})
			if err != nil {
				return nil, 0, 0, err
			}
			if sel == nil {
				// No rows in this combination. Leaving the cell empty says so;
				// an empty pair of axes would say the same thing at the cost of
				// the space every other panel needed.
				continue
			}
			p := Panel{Row: ri, Col: ci, Layers: sel}
			// Each key is named once: the column across the top, the row down
			// the side. Repeating both on every panel would be a label per
			// panel saying what the edges already say.
			if ri == 0 {
				p.Strip = ck
			}
			if ci == len(colKeys)-1 {
				p.RightStrip = rk
			}
			panels = append(panels, p)
		}
	}
	if len(panels) == 0 {
		return nil, 0, 0, fmt.Errorf("%w: %q crossed with %q selects no rows", ErrNoColumn, s.rowCol, s.col)
	}
	return panels, len(rowKeys), len(colKeys), nil
}

// keysOf collects the distinct values of a facet column across every layer
// that has one, in first-appearance order.
//
// First appearance rather than sorted: it is the only ordering that behaves
// the same for text, numbers and times, and it lets a caller decide panel
// order by ordering the rows — which is the only control that does not need an
// API of its own.
func keysOf(layers []geom.Geom, col string) ([]string, error) {
	var keys []string
	seen := map[string]bool{}
	found := false
	for _, l := range layers {
		f, ok := l.(geom.Faceter)
		if !ok || f.Source() == nil {
			continue
		}
		labels, ok := data.Labels(f.Source(), col)
		if !ok {
			continue
		}
		found = true
		for _, k := range labels {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: %q", ErrNoColumn, col)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: %q has no rows to split", ErrNoColumn, col)
	}
	return keys, nil
}

// selection restricts every layer to the rows matching want, returning nil if
// no layer with data contributes a single row.
func selection(layers []geom.Geom, want map[string]string) ([]geom.Geom, error) {
	out := make([]geom.Geom, 0, len(layers))
	any := false
	for _, l := range layers {
		f, ok := l.(geom.Faceter)
		if !ok || f.Source() == nil {
			// A layer with no rows is not data to be split; it is furniture,
			// and it belongs on every panel.
			out = append(out, l)
			continue
		}
		rows, matched := matchRows(f.Source(), want)
		if !matched {
			// This layer does not carry the facet column at all, so it is not
			// what the chart is being split by. Draw it whole.
			out = append(out, l)
			continue
		}
		if len(rows) > 0 {
			any = true
		}
		out = append(out, f.Subset(rows))
	}
	if !any {
		return nil, nil
	}
	return out, nil
}

// matchRows returns the rows of src whose facet columns all equal want.
func matchRows(src data.Source, want map[string]string) ([]int, bool) {
	cols := make([][]string, 0, len(want))
	values := make([]string, 0, len(want))
	for name, v := range want {
		labels, ok := data.Labels(src, name)
		if !ok {
			return nil, false
		}
		cols = append(cols, labels)
		values = append(values, v)
	}
	match := func(i int) bool {
		for j, labels := range cols {
			if i >= len(labels) || labels[i] != values[j] {
				return false
			}
		}
		return true
	}

	// Count, then fill. Growing the list by appending costs one allocation per
	// doubling, so a facet over a million rows would allocate twenty times per
	// panel per frame for want of a number it can work out in one pass.
	n := 0
	for i := range src.Len() {
		if match(i) {
			n++
		}
	}
	rows := make([]int, 0, n)
	for i := range src.Len() {
		if match(i) {
			rows = append(rows, i)
		}
	}
	return rows, true
}

// squarish picks a column count for n panels, preferring a grid slightly wider
// than it is tall — which is the shape a screen and a page both are.
func squarish(n int) int {
	c := 1
	for c*c < n {
		c++
	}
	if c > 1 && c*(c-1) >= n {
		return c
	}
	return c
}
