package data

import "time"

// Label reads one cell of a column as the text that row says there.
//
// It is [Labels] for a single row, and it does not build a column to answer.
// That is the whole reason it exists: Labels spells a numeric or temporal
// column by allocating a []string as long as the table, which is the right
// shape for faceting — done once, for every row — and the wrong shape for a
// pointer, which asks about one row on every move. A hover over a chart of a
// million rows should not allocate a million strings to name one of them.
//
// The spellings are Labels', so a key, a facet panel key and a categorical
// tick for the same value are the same string.
//
// ok is false when the source is nil, the column is not there, the column is
// of no type this understands, or row is outside the table.
func Label(src Source, col string, row int) (string, bool) {
	if src == nil || col == "" || row < 0 {
		return "", false
	}
	if v, ok := src.StringColumn(col); ok {
		if row >= len(v) {
			return "", false
		}
		return v[row], true
	}
	if v, ok := src.Float64Column(col); ok {
		if row >= len(v) {
			return "", false
		}
		return FormatNumber(v[row]), true
	}
	if v, ok := src.TimeColumn(col); ok {
		if row >= len(v) {
			return "", false
		}
		return v[row].Format(time.RFC3339), true
	}
	return "", false
}
