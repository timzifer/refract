package data_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/timzifer/refract/data"
)

func sample() *data.Table {
	return data.NewTable().
		String("region", []string{"north", "south", "north", "east"}).
		Float64("sales", []float64{1, 2, 3, 4}).
		Time("when", []time.Time{
			time.Unix(0, 0).UTC(),
			time.Unix(1, 0).UTC(),
			time.Unix(2, 0).UTC(),
			time.Unix(3, 0).UTC(),
		})
}

func TestRowsGathersInTheOrderGiven(t *testing.T) {
	sub := data.Rows(sample(), []int{3, 0})
	if got := sub.Len(); got != 2 {
		t.Fatalf("Len = %d, want 2", got)
	}
	if got, _ := sub.Float64Column("sales"); !reflect.DeepEqual(got, []float64{4, 1}) {
		t.Errorf("sales = %v, want [4 1]", got)
	}
	if got, _ := sub.StringColumn("region"); !reflect.DeepEqual(got, []string{"east", "north"}) {
		t.Errorf("region = %v, want [east north]", got)
	}
	if got, _ := sub.TimeColumn("when"); len(got) != 2 || got[0] != time.Unix(3, 0).UTC() {
		t.Errorf("when = %v", got)
	}
}

// A gathered column is built once and handed back on every later read, because
// a geom asks for the same column from Train and from Build.
func TestRowsCachesAGatheredColumn(t *testing.T) {
	sub := data.Rows(sample(), []int{1, 2})
	a, _ := sub.Float64Column("sales")
	b, _ := sub.Float64Column("sales")
	if &a[0] != &b[0] {
		t.Error("the second read re-gathered the column")
	}
}

func TestRowsDropsOutOfRangeIndices(t *testing.T) {
	sub := data.Rows(sample(), []int{-1, 2, 99})
	if got := sub.Len(); got != 1 {
		t.Fatalf("Len = %d, want 1", got)
	}
	if got, _ := sub.Float64Column("sales"); !reflect.DeepEqual(got, []float64{3}) {
		t.Errorf("sales = %v, want [3]", got)
	}
}

func TestRowsReportsAMissingColumn(t *testing.T) {
	sub := data.Rows(sample(), []int{0})
	if _, ok := sub.Float64Column("nope"); ok {
		t.Error("a column that does not exist was reported present")
	}
	if _, ok := sub.StringColumn("sales"); ok {
		t.Error("a numeric column was reported as textual")
	}
}

func TestGroupByKeepsFirstAppearanceOrder(t *testing.T) {
	keys, rows, ok := data.GroupBy(sample(), "region")
	if !ok {
		t.Fatal("GroupBy reported no such column")
	}
	if !reflect.DeepEqual(keys, []string{"north", "south", "east"}) {
		t.Errorf("keys = %v, want [north south east]", keys)
	}
	if !reflect.DeepEqual(rows, [][]int{{0, 2}, {1}, {3}}) {
		t.Errorf("rows = %v", rows)
	}
}

// Faceting over a numeric column means one panel per distinct number, which is
// only well defined if the key is spelled the same way an ordinal axis spells
// it.
func TestGroupByOverNumbersUsesTheCategoryLabel(t *testing.T) {
	src := data.Float64Columns(map[string][]float64{
		"run": {1, 2, 1},
		"y":   {9, 8, 7},
	})
	keys, rows, ok := data.GroupBy(src, "run")
	if !ok {
		t.Fatal("GroupBy reported no such column")
	}
	if !reflect.DeepEqual(keys, []string{"1", "2"}) {
		t.Errorf("keys = %v, want [1 2]", keys)
	}
	if !reflect.DeepEqual(rows, [][]int{{0, 2}, {1}}) {
		t.Errorf("rows = %v", rows)
	}
}

func TestGroupByReportsAMissingColumn(t *testing.T) {
	if _, _, ok := data.GroupBy(sample(), "nope"); ok {
		t.Error("a column that does not exist was reported present")
	}
}
