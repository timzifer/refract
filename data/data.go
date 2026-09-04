// Package data is refract's data layer: columnar, batch-oriented access to a
// table of values.
//
// The interface returns whole typed columns, never one value at a time. Scalar
// access is the single easiest way to make a plotting library slow, and a
// columnar shape is also what lets a []float64-backed source be borrowed
// instead of copied.
package data

import "time"

// Source exposes columnar, batch access to a table.
//
// Implementations return read-only views: the caller must not mutate a
// returned slice, and refract never does. An implementation that already holds
// its data as a Go slice should return that slice directly rather than
// copying.
type Source interface {
	// Len reports the number of rows. Every column has this length.
	Len() int

	// Columns lists the available column names. The order is stable across
	// calls on the same Source.
	Columns() []string

	// Float64Column returns a numeric column by name. ok is false if the
	// column does not exist or is not numeric.
	Float64Column(name string) (data []float64, ok bool)

	// TimeColumn returns a time column by name. ok is false if the column does
	// not exist or is not temporal.
	TimeColumn(name string) (data []time.Time, ok bool)

	// StringColumn returns a categorical column by name. ok is false if the
	// column does not exist or is not textual.
	StringColumn(name string) (data []string, ok bool)
}

// Float64Columns builds a Source over the given numeric columns.
//
// The slices are borrowed, not copied: the returned Source aliases the caller's
// memory, and mutating it afterwards mutates what refract will plot. All
// columns must have the same length; Float64Columns panics otherwise, because
// a ragged table is a programming error rather than a runtime condition.
func Float64Columns(cols map[string][]float64) Source {
	s := &float64Source{cols: cols, names: sortedKeys(cols)}
	for i, n := range s.names {
		if i == 0 {
			s.n = len(cols[n])
			continue
		}
		if len(cols[n]) != s.n {
			panic("refract/data: Float64Columns: column " + n + " has a different length than " + s.names[0])
		}
	}
	return s
}

type float64Source struct {
	cols  map[string][]float64
	names []string
	n     int
}

func (s *float64Source) Len() int          { return s.n }
func (s *float64Source) Columns() []string { return s.names }

func (s *float64Source) Float64Column(name string) ([]float64, bool) {
	c, ok := s.cols[name]
	return c, ok
}

func (s *float64Source) TimeColumn(string) ([]time.Time, bool) { return nil, false }

func (s *float64Source) StringColumn(string) ([]string, bool) { return nil, false }

// Table is a Source that mixes numeric, temporal and categorical columns.
//
// It is the general-purpose implementation: use it when a chart plots time or
// a category against values, which is the common case for the Time and Ordinal
// scales.
type Table struct {
	nums  map[string][]float64
	times map[string][]time.Time
	strs  map[string][]string
	names []string
	n     int
	fixed bool // true once the row count has been established
}

// NewTable returns an empty Table.
func NewTable() *Table {
	return &Table{
		nums:  map[string][]float64{},
		times: map[string][]time.Time{},
		strs:  map[string][]string{},
	}
}

// Float64 adds a numeric column, borrowing the slice. It returns t so calls
// can be chained. It panics if the column length disagrees with columns
// already added, or if the name is already taken.
func (t *Table) Float64(name string, v []float64) *Table {
	t.claim(name, len(v))
	t.nums[name] = v
	return t
}

// Time adds a temporal column, borrowing the slice. It returns t so calls can
// be chained. It panics if the column length disagrees with columns already
// added, or if the name is already taken.
func (t *Table) Time(name string, v []time.Time) *Table {
	t.claim(name, len(v))
	t.times[name] = v
	return t
}

// String adds a categorical column, borrowing the slice. It returns t so calls
// can be chained. It panics if the column length disagrees with columns
// already added, or if the name is already taken.
//
// Plot such a column against a [scale.Ordinal] axis; a continuous scale has no
// position for a category name and a geom says so rather than guessing one.
func (t *Table) String(name string, v []string) *Table {
	t.claim(name, len(v))
	t.strs[name] = v
	return t
}

func (t *Table) claim(name string, n int) {
	if _, dup := t.nums[name]; dup {
		panic("refract/data: duplicate column " + name)
	}
	if _, dup := t.times[name]; dup {
		panic("refract/data: duplicate column " + name)
	}
	if _, dup := t.strs[name]; dup {
		panic("refract/data: duplicate column " + name)
	}
	if t.fixed && n != t.n {
		panic("refract/data: column " + name + " has a different length than the existing columns")
	}
	t.n, t.fixed = n, true
	t.names = append(t.names, name)
}

// Len reports the number of rows.
func (t *Table) Len() int { return t.n }

// Columns lists the column names in insertion order.
func (t *Table) Columns() []string { return t.names }

// Float64Column returns a numeric column by name.
func (t *Table) Float64Column(name string) ([]float64, bool) {
	c, ok := t.nums[name]
	return c, ok
}

// TimeColumn returns a temporal column by name.
func (t *Table) TimeColumn(name string) ([]time.Time, bool) {
	c, ok := t.times[name]
	return c, ok
}

// StringColumn returns a categorical column by name.
func (t *Table) StringColumn(name string) ([]string, bool) {
	c, ok := t.strs[name]
	return c, ok
}
