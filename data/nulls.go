package data

// Nulls is implemented by a Source that can say which of a column's values are
// absent, rather than merely which are unrepresentable.
//
// A numeric column needs no such thing: a missing number is NaN, every scale
// refuses to place it, and [github.com/timzifer/refract/geom.OnMissing] is
// written against exactly that. A *text* column has no NaN — a null read back
// as "" is indistinguishable from a genuine empty string, and on an ordinal
// axis it becomes a band of its own — and a *time* column has none either: a
// null read back as the zero time is the year 1, which is a real instant that
// stretches a domain over two millennia. Both were silent: the chart drew, and
// it drew something nobody measured.
//
// This is the optional interface [Source]'s own documentation promises for a
// capability arriving after the freeze, and it is asked for with a type
// assertion. A Source that does not implement it has no nulls beyond the NaNs
// already in its numbers, which is what every Source in this package that
// predates it means.
//
// # What a null means
//
// One rule, everywhere a column is read: **a null is a missing value.** On a
// position axis the row has no place, so [OnMissing] decides whether the chart
// gaps, interpolates or errors — the same three answers it already gives a
// NaN. On a colour channel the row takes the scale's undefined colour. In a
// group or facet column the row belongs to no series and no panel, so it is
// not drawn: a category named "" is not a reading, and inventing one is how
// the zero time got onto an axis in the first place. See
// [ADR 0034](../docs/adr/0034-null-values.md).
type Nulls interface {
	// Nulls returns one flag per row, true where the column has no value.
	//
	// ok is false when the column does not exist *or has no nulls at all*.
	// That second half is what keeps the zero-copy path in
	// [Float64Columns] intact: a reader asks first and copies only when the
	// answer is yes, so a table without nulls costs exactly what it did.
	// The result is read-only.
	Nulls(name string) (null []bool, ok bool)
}

// NullMask reports which of a column's rows are absent, or ok == false when
// src cannot say or has nothing to say.
//
// It is the shared spelling of the question, the way [Labels] is for "what
// does this row say in that column": every reader asks through this so that
// one Source implementing [Nulls] answers every channel at once.
func NullMask(src Source, name string) (null []bool, ok bool) {
	if src == nil || name == "" {
		return nil, false
	}
	n, has := src.(Nulls)
	if !has {
		return nil, false
	}
	mask, got := n.Nulls(name)
	if !got || len(mask) == 0 {
		return nil, false
	}
	return mask, true
}

// IsNull reports whether row i of mask is absent, tolerating a nil or short
// mask.
//
// A mask is as long as the column it describes, so the length test is not
// defensive padding: [Rows] gathers a mask along with the column it belongs
// to, and a caller composing sources by hand may hand over one that stops
// early. A row past the end has a value, because that is what the column says.
func IsNull(mask []bool, i int) bool { return i < len(mask) && mask[i] }

// AnyNull reports whether mask marks any row at all.
//
// It is what lets a reader decide between the borrowed column and a copy: a
// mask that marks nothing changes no value, so there is nothing to write and
// the caller's slice is handed on untouched.
func AnyNull(mask []bool) bool {
	for _, null := range mask {
		if null {
			return true
		}
	}
	return false
}
