package data

import (
	"strconv"
	"time"
)

// Rows returns a Source over the rows of src named by idx, in the order given.
//
// It is how faceting cuts one table into panels: a facet reads the column it
// splits on, groups the row numbers, and hands each group to Rows. Out-of-range
// indices are dropped rather than panicking, because they come from a grouping
// pass rather than from the caller.
//
// The result materialises the rows it is asked for. That is a copy — the
// zero-copy promise in [Float64Columns] is about the whole-column path, and a
// gathered subset has no contiguous slice to borrow. Columns are gathered
// lazily, so a table with forty columns and a chart that reads three copies
// three.
func Rows(src Source, idx []int) Source {
	if src == nil {
		return nil
	}
	keep := make([]int, 0, len(idx))
	n := src.Len()
	for _, i := range idx {
		if i >= 0 && i < n {
			keep = append(keep, i)
		}
	}
	return &rowsSource{
		src:   src,
		idx:   keep,
		nums:  map[string][]float64{},
		times: map[string][]time.Time{},
		strs:  map[string][]string{},
	}
}

type rowsSource struct {
	src Source
	idx []int

	nums  map[string][]float64
	times map[string][]time.Time
	strs  map[string][]string
}

func (r *rowsSource) Len() int          { return len(r.idx) }
func (r *rowsSource) Columns() []string { return r.src.Columns() }

func (r *rowsSource) Float64Column(name string) ([]float64, bool) {
	if got, ok := r.nums[name]; ok {
		return got, true
	}
	src, ok := r.src.Float64Column(name)
	if !ok {
		return nil, false
	}
	out := gather(src, r.idx)
	r.nums[name] = out
	return out, true
}

func (r *rowsSource) TimeColumn(name string) ([]time.Time, bool) {
	if got, ok := r.times[name]; ok {
		return got, true
	}
	src, ok := r.src.TimeColumn(name)
	if !ok {
		return nil, false
	}
	out := gather(src, r.idx)
	r.times[name] = out
	return out, true
}

func (r *rowsSource) StringColumn(name string) ([]string, bool) {
	if got, ok := r.strs[name]; ok {
		return got, true
	}
	src, ok := r.src.StringColumn(name)
	if !ok {
		return nil, false
	}
	out := gather(src, r.idx)
	r.strs[name] = out
	return out, true
}

// Subset is implemented by a Source that is a selection of another source's
// rows, so that a row number can be traced back to the table it came from.
//
// It exists because faceting makes such a source without the caller ever
// seeing it: [github.com/timzifer/refract.Plot.Facet] cuts each layer down to
// its panel's rows with [Rows], and a row number relative to that cut is a row
// number in a table nobody holds. A geom reporting where its rows landed
// resolves them through this first, so what comes out is a row of the table
// that was handed in.
//
// It is an optional interface. A Source that is not a selection of another
// does not implement it, and its rows are already its own.
type Subset interface {
	// SourceRows returns, for each of this source's rows, the row it came from
	// in the source it selects from. The result is read-only.
	SourceRows() []int
}

// SourceRows implements [Subset].
func (r *rowsSource) SourceRows() []int { return r.idx }

// Origins returns the mapping from src's rows to the rows of the table it was
// cut from, or nil if src is not a cut of anything.
//
// One level, not the whole chain: faceting cuts once, and a caller that has
// composed cuts of cuts knows it has and can compose the mappings the same
// way.
func Origins(src Source) []int {
	if sub, ok := src.(Subset); ok {
		return sub.SourceRows()
	}
	return nil
}

func gather[T any](src []T, idx []int) []T {
	out := make([]T, 0, len(idx))
	for _, i := range idx {
		if i < len(src) {
			out = append(out, src[i])
		}
	}
	return out
}

// GroupBy splits src into groups by the values of a column, returning the
// distinct values in first-appearance order and the row numbers of each.
//
// The column may be textual, numeric or temporal; whichever it is, the group
// key is its formatted label, so a facet over a numeric column gets one panel
// per distinct number rather than a continuous axis. ok is false if src has no
// such column.
//
// First-appearance order rather than sorted order is deliberate: it is the one
// ordering that is stable under every column type and lets a caller control
// panel order by ordering its rows.
func GroupBy(src Source, col string) (keys []string, rows [][]int, ok bool) {
	if src == nil || col == "" {
		return nil, nil, false
	}
	labels, ok := Labels(src, col)
	if !ok {
		return nil, nil, false
	}
	// Count first, then fill. Growing each group by appending would allocate
	// once per doubling per group, which is a cost that rises with the row
	// count for no reason: the counts are known after one pass, and one
	// backing array sliced up serves every group.
	at := map[string]int{}
	var counts []int
	for _, l := range labels {
		j, seen := at[l]
		if !seen {
			j = len(keys)
			at[l] = j
			keys = append(keys, l)
			counts = append(counts, 0)
		}
		counts[j]++
	}
	flat := make([]int, 0, len(labels))
	rows = make([][]int, len(keys))
	off := 0
	for j, n := range counts {
		rows[j] = flat[off : off : off+n]
		off += n
	}
	for i, l := range labels {
		j := at[l]
		rows[j] = append(rows[j], i)
	}
	return keys, rows, true
}

// Labels reads a column as one text label per row, whatever its type.
//
// It is the shared spelling of "what does this row say in that column" —
// faceting groups by it, and a categorical axis encodes by it, so the two agree
// about what counts as one category.
func Labels(src Source, col string) ([]string, bool) {
	if src == nil || col == "" {
		return nil, false
	}
	if v, ok := src.StringColumn(col); ok {
		return v, true
	}
	if v, ok := src.Float64Column(col); ok {
		out := make([]string, len(v))
		for i, f := range v {
			out[i] = FormatNumber(f)
		}
		return out, true
	}
	if v, ok := src.TimeColumn(col); ok {
		out := make([]string, len(v))
		for i, t := range v {
			out[i] = t.Format(time.RFC3339)
		}
		return out, true
	}
	return nil, false
}

// FormatNumber is how a numeric value is spelled when it is used as a category
// name. Both faceting and a categorical axis go through it, so a panel key and
// an axis tick for the same number are the same string.
func FormatNumber(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
