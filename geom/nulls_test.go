package geom_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/theme"
)

// The two failures the mask exists to end. Both were silent: the chart drew,
// and what it drew was a value nobody measured.

// A null on a categorical axis used to be a band between the categories
// somebody did measure, because "" is what a text column reads back as.
func TestANullCategoryIsNotABandOfItsOwn(t *testing.T) {
	src := data.NewTable().
		String("k", []string{"a", "", "b"}).
		Float64("v", []float64{1, 2, 3}).
		WithNulls("k", []bool{false, true, false})

	x, y := scale.Ordinal(), scale.Linear()
	if err := geom.Bar(src, geom.X("k"), geom.Y("v")).Train(x, y); err != nil {
		t.Fatal(err)
	}
	ticks := x.Ticks(0)
	if len(ticks) != 2 {
		t.Fatalf("the axis has %d bands, want one per category somebody measured", len(ticks))
	}
	if ticks[0].Label != "a" || ticks[1].Label != "b" {
		t.Errorf("bands are %q and %q, want a and b", ticks[0].Label, ticks[1].Label)
	}
}

// An empty string somebody measured is still a category. Without this the
// feature is indistinguishable from dropping every empty string, which is a
// different and wrong thing.
func TestAnEmptyCategoryIsStillABand(t *testing.T) {
	src := data.NewTable().
		String("k", []string{"a", "", "b"}).
		Float64("v", []float64{1, 2, 3})

	x, y := scale.Ordinal(), scale.Linear()
	if err := geom.Bar(src, geom.X("k"), geom.Y("v")).Train(x, y); err != nil {
		t.Fatal(err)
	}
	if got := len(x.Ticks(0)); got != 3 {
		t.Errorf("the axis has %d bands, want three", got)
	}
}

// A null instant used to be the zero time, which is the year 1 — so one absent
// row stretched a domain of three hours across two millennia.
func TestANullInstantDoesNotStretchTheAxis(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	src := data.NewTable().
		Time("t", []time.Time{at, {}, at.Add(2 * time.Hour)}).
		Float64("v", []float64{1, 2, 3}).
		WithNulls("t", []bool{false, true, false})

	x, y := scale.Time(), scale.Linear()
	if err := geom.Line(src, geom.X("t"), geom.Y("v")).Train(x, y); err != nil {
		t.Fatal(err)
	}
	lo, hi := x.Domain()
	from, to := scale.InstantOf(x, lo), scale.InstantOf(x, hi)
	if span := to.Sub(from); span != 2*time.Hour {
		t.Errorf("the axis spans %v (%v..%v), want the two hours between the rows that have an instant", span, from, to)
	}
}

// The mask and a NaN are the same failure, so the policy that catches one
// catches the other. Without this a caller who asked to be told about missing
// data would be told about their numbers and not about their categories.
func TestANullIsMissingUnderTheErrorPolicy(t *testing.T) {
	src := data.NewTable().
		String("k", []string{"a", "b", "c"}).
		Float64("v", []float64{1, 2, 3}).
		WithNulls("k", []bool{false, true, false})

	err := geom.Bar(src, geom.X("k"), geom.Y("v"), geom.OnMissing(geom.Error)).
		Train(scale.Ordinal(), scale.Linear())
	if err == nil {
		t.Fatal("a null row passed OnMissing(Error)")
	}
}

// The columns are borrowed, and a chart must not edit the table it is given.
// The mask is applied to a copy, and only for a column that needs one.
func TestABorrowedColumnIsNotWrittenTo(t *testing.T) {
	vs := []float64{1, 2, 3}
	src := data.NewTable().
		Float64("v", vs).
		Float64("y", []float64{4, 5, 6}).
		WithNulls("v", []bool{false, true, false})

	if err := geom.Line(src, geom.X("v"), geom.Y("y")).Train(scale.Linear(), scale.Linear()); err != nil {
		t.Fatal(err)
	}
	if vs[1] != 2 {
		t.Errorf("the caller's column now reads %v; a chart may not write into the table it was handed", vs)
	}
}

// A numeric column whose nulls are already NaN — which is every Arrow column —
// changes nothing, so it is handed on rather than copied. This is the property
// that keeps the zero-copy path what it was.
func TestAMaskThatAgreesWithItsNaNsCopiesNothing(t *testing.T) {
	vs := []float64{1, math.NaN(), 3}
	src := data.NewTable().
		Float64("v", vs).
		Float64("y", []float64{4, 5, 6}).
		WithNulls("v", []bool{false, true, false})

	x := scale.Linear()
	if err := geom.Line(src, geom.X("v"), geom.Y("y")).Train(x, scale.Linear()); err != nil {
		t.Fatal(err)
	}
	if lo, hi := x.Domain(); lo != 1 || hi != 3 {
		t.Errorf("domain is %v..%v, want 1..3", lo, hi)
	}
}

// A series nobody named is not a series. Registering the key would put an
// entry called "" in the legend and spend a colour of the palette on it.
func TestANullSeriesIsNotALegendEntry(t *testing.T) {
	src := data.NewTable().
		Float64("t", []float64{0, 1, 2, 3}).
		Float64("v", []float64{1, 2, 3, 4}).
		String("series", []string{"a", "b", "", "a"}).
		WithNulls("series", []bool{false, false, true, false})

	g := geom.Line(src, geom.X("t"), geom.Y("v"), geom.GroupBy("series"),
		geom.ColorBy("series", scale.Qualitative(nil)))
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatal(err)
	}
	entries := geom.Legends(g, geom.Frame{X: x, Y: y, Theme: theme.Light})
	if len(entries) != 2 {
		t.Fatalf("got %d legend entries %v, want one each for a and b", len(entries), entries)
	}
	for _, e := range entries {
		if e.Label == "" {
			t.Error("the legend named a series nobody named")
		}
	}
}

// Every row's series absent is no series at all, which is the ungrouped layer
// reached by a different road. It must not be an index into an empty list.
func TestALayerWhoseEverySeriesIsAbsentIsUngrouped(t *testing.T) {
	src := data.NewTable().
		Float64("t", []float64{0, 1}).
		Float64("v", []float64{1, 2}).
		String("series", []string{"", ""}).
		WithNulls("series", []bool{true, true})

	g := geom.Bar(src, geom.X("t"), geom.Y("v"), geom.GroupBy("series"))
	if err := g.Train(scale.Linear(), scale.Linear()); err != nil {
		t.Fatal(err)
	}
}

// A row whose colour category is absent is still a measurement. It is painted
// in the scale's undefined colour rather than dropped, which is the one
// channel where a null has an answer of its own — and the answer belongs to
// the scale, so a caller who wants such rows visible names a colour for them.
func TestARowWithNoColourTakesTheUndefinedColour(t *testing.T) {
	grey := ir.RGB(128, 128, 128)
	src := data.NewTable().
		Float64("t", []float64{0, 1, 2}).
		Float64("v", []float64{1, 2, 3}).
		String("k", []string{"a", "", "b"}).
		WithNulls("k", []bool{false, true, false})

	g := geom.Scatter(src, geom.X("t"), geom.Y("v"),
		geom.ColorBy("k", scale.Qualitative(nil, scale.ColorUndefined(grey))))
	rec, f := frameOn(t, g, scale.Linear(), scale.Linear(), 200, 100)
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	n, undefined := 0, 0
	for _, c := range rec.Filter("Markers") {
		n += len(c.Points)
		if c.Style.Fill == grey {
			undefined += len(c.Points)
		}
	}
	if n != 3 {
		t.Errorf("drew %d markers, want three: a row with no category is still a reading", n)
	}
	if undefined != 1 {
		t.Errorf("%d markers took the undefined colour, want the one row with no category", undefined)
	}
}

// The error a null produces under the Error policy is the missing-data error
// and not something of its own, because from the chart's point of view they
// are one failure.
func TestANullErrorIsTheMissingDataError(t *testing.T) {
	src := data.NewTable().
		Float64("t", []float64{0, 1}).
		Float64("v", []float64{1, 2}).
		WithNulls("v", []bool{false, true})

	err := geom.Line(src, geom.X("t"), geom.Y("v"), geom.OnMissing(geom.Error)).
		Train(scale.Linear(), scale.Linear())
	if err == nil {
		t.Fatal("a marked numeric row passed OnMissing(Error)")
	}
	if errors.Is(err, geom.ErrNoColumn) {
		t.Errorf("a null was reported as a missing column: %v", err)
	}
}
