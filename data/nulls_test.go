package data_test

import (
	"testing"
	"time"

	"github.com/timzifer/refract/data"
)

// A mask that marks nothing is not a mask. The interface promises that a
// null-free column answers no, because every reader decides between the
// borrowed column and a copy on that answer.
func TestAMaskThatMarksNothingIsNotAMask(t *testing.T) {
	tab := data.NewTable().
		String("k", []string{"a", "b", "c"}).
		WithNulls("k", []bool{false, false, false})
	if mask, ok := tab.Nulls("k"); ok {
		t.Errorf("a column with no nulls reported %v; want no mask at all", mask)
	}
	if _, ok := data.NullMask(tab, "k"); ok {
		t.Error("NullMask agreed there were nulls")
	}
}

// A table nobody told about a null has none, which is what keeps every table
// written before the interface existed exactly what it was.
func TestATableWithoutNullsHasNone(t *testing.T) {
	tab := data.NewTable().Float64("v", []float64{1, 2, 3})
	if _, ok := data.NullMask(tab, "v"); ok {
		t.Error("a table nobody marked reported nulls")
	}
	if _, ok := data.NullMask(tab, "nope"); ok {
		t.Error("a column that does not exist reported nulls")
	}
}

// The mask is gathered with the rows its column is gathered with. A mask that
// was not cut alongside would mark whichever rows happened to land in those
// slots — which is worse than losing it, because it is wrong rather than
// absent.
func TestANullMaskIsCutWithItsColumn(t *testing.T) {
	tab := data.NewTable().
		String("k", []string{"a", "b", "c", "d"}).
		WithNulls("k", []bool{false, true, false, true})

	cut := data.Rows(tab, []int{3, 0, 1})
	mask, ok := data.NullMask(cut, "k")
	if !ok {
		t.Fatal("the cut lost the mask")
	}
	if want := []bool{true, false, true}; len(mask) != 3 || mask[0] != want[0] || mask[1] != want[1] || mask[2] != want[2] {
		t.Errorf("mask over rows 3,0,1 is %v, want %v", mask, want)
	}
}

// A cut that leaves every null behind has no nulls. Reporting the parent's
// answer would make a reader copy a column to change none of it.
func TestACutWithoutNullsReportsNone(t *testing.T) {
	tab := data.NewTable().
		String("k", []string{"a", "b"}).
		WithNulls("k", []bool{false, true})
	if _, ok := data.NullMask(data.Rows(tab, []int{0}), "k"); ok {
		t.Error("a cut holding only present rows reported nulls")
	}
}

// A row whose key is absent belongs to no group. Gathering it under "" would
// make a panel that is indistinguishable from a panel for the rows that really
// are labelled with nothing.
func TestARowWithNoKeyIsInNoGroup(t *testing.T) {
	tab := data.NewTable().
		String("k", []string{"a", "", "b", "a"}).
		WithNulls("k", []bool{false, true, false, false})

	keys, rows, ok := data.GroupBy(tab, "k")
	if !ok {
		t.Fatal("GroupBy refused the column")
	}
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("keys are %q, want a and b and nothing for the null", keys)
	}
	if len(rows[0]) != 2 || rows[0][0] != 0 || rows[0][1] != 3 {
		t.Errorf("group a holds rows %v, want 0 and 3", rows[0])
	}
	for _, list := range rows {
		for _, i := range list {
			if i == 1 {
				t.Error("the null row was gathered into a group")
			}
		}
	}
}

// A genuine empty string is a category. That is the distinction the mask
// exists to make, so it is worth a test of its own: without one, "drop the
// nulls" and "drop the empty strings" pass the same tests.
func TestAnEmptyStringIsStillACategory(t *testing.T) {
	tab := data.NewTable().String("k", []string{"a", "", "b"})
	keys, _, ok := data.GroupBy(tab, "k")
	if !ok {
		t.Fatal("GroupBy refused the column")
	}
	if len(keys) != 3 {
		t.Errorf("keys are %q, want three: an empty string somebody measured is a category", keys)
	}
}

// A mask is set on a column, so there has to be one, and it has to be one flag
// per row. Both are programming errors rather than runtime conditions, which
// is the same line [data.Table.Float64] already draws about a ragged table.
func TestWithNullsRefusesWhatItCannotMark(t *testing.T) {
	t.Run("no such column", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("marking a column that is not there did not panic")
			}
		}()
		data.NewTable().Float64("v", []float64{1}).WithNulls("nope", []bool{true})
	})
	t.Run("wrong length", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("a mask of the wrong length did not panic")
			}
		}()
		data.NewTable().Float64("v", []float64{1, 2}).WithNulls("v", []bool{true})
	})
}

// A temporal null is the one that used to be loudest: the zero time is the
// year 1, and a domain that reaches it spans two millennia.
func TestATemporalNullIsMarkedRatherThanDated(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	tab := data.NewTable().
		Time("t", []time.Time{now, {}, now.Add(time.Hour)}).
		WithNulls("t", []bool{false, true, false})
	mask, ok := data.NullMask(tab, "t")
	if !ok || !data.IsNull(mask, 1) || data.IsNull(mask, 0) {
		t.Errorf("mask is %v, want only the middle row marked", mask)
	}
}

// IsNull answers about a row past the end of a mask, because a caller
// composing sources by hand may hand over a short one and a row the column has
// a value for is not absent.
func TestARowPastTheEndOfAMaskHasAValue(t *testing.T) {
	if data.IsNull(nil, 0) || data.IsNull([]bool{true}, 1) {
		t.Error("a row the mask does not describe was reported absent")
	}
	if !data.AnyNull([]bool{false, true}) || data.AnyNull([]bool{false}) || data.AnyNull(nil) {
		t.Error("AnyNull disagreed with its mask")
	}
}
