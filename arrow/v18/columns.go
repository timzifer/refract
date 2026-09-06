package arrow

import (
	"math"
	"time"

	arrow "github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

// numericColumn reads a column as float64, borrowing when it can.
//
// The borrow is the one case where Arrow's memory is exactly what refract
// wants: one contiguous run of IEEE-754 doubles, one per row, with nothing to
// substitute. A single null forfeits it, because writing NaN into the nulls
// would mean writing into a buffer this package does not own.
func numericColumn(chunks []arrow.Array, n int) ([]float64, bool) {
	if len(chunks) == 1 {
		if a, ok := chunks[0].(*array.Float64); ok && a.NullN() == 0 {
			return a.Float64Values(), true
		}
	}
	if !numeric(chunks) {
		return nil, false
	}
	out := make([]float64, 0, n)
	for _, c := range chunks {
		out = appendNumeric(out, c)
	}
	return out, true
}

func numeric(chunks []arrow.Array) bool {
	for _, c := range chunks {
		switch c.(type) {
		case *array.Float64, *array.Float32,
			*array.Int64, *array.Int32, *array.Int16, *array.Int8,
			*array.Uint64, *array.Uint32, *array.Uint16, *array.Uint8,
			*array.Boolean, *array.Duration:
		default:
			return false
		}
	}
	return len(chunks) > 0
}

// appendNumeric widens one chunk into float64.
//
// A null becomes NaN, which is what refract's missing-data policies are
// written against — so a null row is gapped, interpolated or rejected by the
// same setting that handles a NaN from anywhere else.
func appendNumeric(out []float64, c arrow.Array) []float64 {
	switch a := c.(type) {
	case *array.Float64:
		return appendVals(out, a, a.Len(), func(i int) float64 { return a.Value(i) })
	case *array.Float32:
		return appendVals(out, a, a.Len(), func(i int) float64 { return float64(a.Value(i)) })
	case *array.Int64:
		return appendVals(out, a, a.Len(), func(i int) float64 { return float64(a.Value(i)) })
	case *array.Int32:
		return appendVals(out, a, a.Len(), func(i int) float64 { return float64(a.Value(i)) })
	case *array.Int16:
		return appendVals(out, a, a.Len(), func(i int) float64 { return float64(a.Value(i)) })
	case *array.Int8:
		return appendVals(out, a, a.Len(), func(i int) float64 { return float64(a.Value(i)) })
	case *array.Uint64:
		return appendVals(out, a, a.Len(), func(i int) float64 { return float64(a.Value(i)) })
	case *array.Uint32:
		return appendVals(out, a, a.Len(), func(i int) float64 { return float64(a.Value(i)) })
	case *array.Uint16:
		return appendVals(out, a, a.Len(), func(i int) float64 { return float64(a.Value(i)) })
	case *array.Uint8:
		return appendVals(out, a, a.Len(), func(i int) float64 { return float64(a.Value(i)) })
	case *array.Duration:
		// A duration is a count of its unit, and a count is a number. Which
		// unit it counts is the axis title's business, not this package's.
		return appendVals(out, a, a.Len(), func(i int) float64 { return float64(a.Value(i)) })
	case *array.Boolean:
		return appendVals(out, a, a.Len(), func(i int) float64 {
			if a.Value(i) {
				return 1
			}
			return 0
		})
	}
	return out
}

// timeColumn reads a column as time.Time. A null becomes the zero time.
func timeColumn(chunks []arrow.Array, n int) ([]time.Time, bool) {
	if !temporal(chunks) {
		return nil, false
	}
	out := make([]time.Time, 0, n)
	for _, c := range chunks {
		out = appendTimes(out, c)
	}
	return out, true
}

func temporal(chunks []arrow.Array) bool {
	for _, c := range chunks {
		switch c.(type) {
		case *array.Timestamp, *array.Date32, *array.Date64:
		default:
			return false
		}
	}
	return len(chunks) > 0
}

func appendTimes(out []time.Time, c arrow.Array) []time.Time {
	switch a := c.(type) {
	case *array.Timestamp:
		// The unit lives in the type, not in the values, so it has to be read
		// from the schema — a timestamp in microseconds and one in nanoseconds
		// are the same integers three orders of magnitude apart.
		unit := arrow.Nanosecond
		if t, ok := a.DataType().(*arrow.TimestampType); ok {
			unit = t.Unit
		}
		return appendVals(out, a, a.Len(), func(i int) time.Time { return a.Value(i).ToTime(unit) })
	case *array.Date32:
		return appendVals(out, a, a.Len(), func(i int) time.Time { return a.Value(i).ToTime() })
	case *array.Date64:
		return appendVals(out, a, a.Len(), func(i int) time.Time { return a.Value(i).ToTime() })
	}
	return out
}

// stringColumn reads a column as text. A null becomes the empty string.
//
// A dictionary-encoded column is decoded here rather than rejected: dictionary
// encoding is how Arrow spells "categorical", and a categorical column is
// exactly what a [scale.Ordinal] axis or a facet is asking for.
func stringColumn(chunks []arrow.Array, n int) ([]string, bool) {
	if !textual(chunks) {
		return nil, false
	}
	out := make([]string, 0, n)
	for _, c := range chunks {
		out = appendStrings(out, c)
	}
	return out, true
}

func textual(chunks []arrow.Array) bool {
	for _, c := range chunks {
		switch a := c.(type) {
		case *array.String, *array.LargeString, *array.StringView:
		case *array.Dictionary:
			if !textual([]arrow.Array{a.Dictionary()}) {
				return false
			}
		default:
			return false
		}
	}
	return len(chunks) > 0
}

func appendStrings(out []string, c arrow.Array) []string {
	switch a := c.(type) {
	case *array.String:
		return appendVals(out, a, a.Len(), func(i int) string { return a.Value(i) })
	case *array.LargeString:
		return appendVals(out, a, a.Len(), func(i int) string { return a.Value(i) })
	case *array.StringView:
		return appendVals(out, a, a.Len(), func(i int) string { return a.Value(i) })
	case *array.Dictionary:
		var vals []string
		vals = appendStrings(vals, a.Dictionary())
		return appendVals(out, a, a.Len(), func(i int) string {
			if j := a.GetValueIndex(i); j >= 0 && j < len(vals) {
				return vals[j]
			}
			return ""
		})
	}
	return out
}

// appendVals is the shared null-aware walk: value where there is one, the
// type's own stand-in for missing where there is not.
func appendVals[T any](out []T, a arrow.Array, n int, value func(int) T) []T {
	null := missing[T]()
	for i := range n {
		if a.IsNull(i) {
			out = append(out, null)
			continue
		}
		out = append(out, value(i))
	}
	return out
}

// missing is what a null becomes: NaN for a number, so that the missing-data
// policies see it; the zero value for anything else, because a time and a
// string have no equivalent of NaN and refract does not pretend otherwise.
func missing[T any]() T {
	var zero T
	if p, ok := any(&zero).(*float64); ok {
		*p = math.NaN()
	}
	return zero
}
