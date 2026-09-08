package data_test

import (
	"testing"
	"time"

	"github.com/timzifer/refract/data"
)

func labelSource() data.Source {
	return data.NewTable().
		String("name", []string{"a", "b"}).
		Float64("n", []float64{1, 2.5}).
		Time("t", []time.Time{
			time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC),
		})
}

// One cell, of whatever type, spelled the way Labels spells the column.
func TestLabelAgreesWithLabels(t *testing.T) {
	src := labelSource()
	for _, col := range []string{"name", "n", "t"} {
		want, ok := data.Labels(src, col)
		if !ok {
			t.Fatalf("Labels(%q) says no", col)
		}
		for row := range want {
			got, ok := data.Label(src, col, row)
			if !ok {
				t.Fatalf("Label(%q, %d) says no", col, row)
			}
			if got != want[row] {
				t.Errorf("Label(%q, %d) = %q, Labels says %q", col, row, got, want[row])
			}
		}
	}
}

func TestLabelRefusesWhatItCannotAnswer(t *testing.T) {
	src := labelSource()
	cases := []struct {
		why string
		col string
		row int
	}{
		{"a column that is not there", "nope", 0},
		{"a row past the end", "name", 2},
		{"a negative row", "name", -1},
		{"no column named", "", 0},
	}
	for _, c := range cases {
		if _, ok := data.Label(src, c.col, c.row); ok {
			t.Errorf("Label answered %s", c.why)
		}
	}
	if _, ok := data.Label(nil, "name", 0); ok {
		t.Error("Label answered a nil source")
	}
}

// The reason this exists rather than Labels: a pointer asks about one row on
// every move, and spelling the whole column to answer would allocate the table
// once per pixel of travel.
func TestLabelDoesNotSpellTheWholeColumn(t *testing.T) {
	const n = 100_000
	nums := make([]float64, n)
	for i := range nums {
		nums[i] = float64(i)
	}
	src := data.NewTable().Float64("n", nums)

	one := testing.AllocsPerRun(100, func() { data.Label(src, "n", n/2) })
	if one > 2 {
		t.Errorf("Label allocates %.0f times per call over %d rows, want a small constant", one, n)
	}
	// A string column needs no spelling at all.
	strs := make([]string, n)
	ssrc := data.NewTable().String("s", strs)
	if got := testing.AllocsPerRun(100, func() { data.Label(ssrc, "s", n/2) }); got != 0 {
		t.Errorf("Label over a string column allocates %.0f times, want 0", got)
	}
}
