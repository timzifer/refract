package arrow_test

import (
	"testing"
	"time"

	aw "github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/timzifer/refract/arrow/v18"
	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
)

// The validity bitmap is the one place a null survives the conversion, because
// "" and the zero time are values rather than absences. Reading it is what
// makes refract's missing-data policies cover all three column kinds.
func TestATextNullIsMarkedRatherThanEmpty(t *testing.T) {
	rec := record(t, schemaOf(aw.Field{Name: "k", Type: aw.BinaryTypes.String, Nullable: true}),
		func(b *array.RecordBuilder) {
			b.Field(0).(*array.StringBuilder).AppendValues([]string{"a", "", "b"}, []bool{true, false, true})
		})

	src := arrow.Source(rec)
	mask, ok := data.NullMask(src, "k")
	if !ok {
		t.Fatal("a column with a null reported none")
	}
	if data.IsNull(mask, 0) || !data.IsNull(mask, 1) || data.IsNull(mask, 2) {
		t.Errorf("mask is %v, want only the middle row marked", mask)
	}
	if v, _ := src.StringColumn("k"); v[1] != "" {
		t.Errorf("the stand-in value changed to %q; the mask is beside the values, not instead of them", v[1])
	}
}

// A null-free column answers no, which is what keeps the borrowed float64 path
// exactly what it was.
func TestANullFreeColumnReportsNoMask(t *testing.T) {
	rec := record(t, schemaOf(aw.Field{Name: "y", Type: aw.PrimitiveTypes.Float64}),
		func(b *array.RecordBuilder) {
			b.Field(0).(*array.Float64Builder).AppendValues([]float64{1, 2, 3}, nil)
		})
	if _, ok := data.NullMask(arrow.Source(rec), "y"); ok {
		t.Error("a column with no nulls offered a mask")
	}
}

// The failure the mask ends, end to end: one absent timestamp used to put the
// year 1 on the axis.
func TestANullTimestampDoesNotStretchTheAxis(t *testing.T) {
	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	rec := record(t, schemaOf(
		aw.Field{Name: "t", Type: &aw.TimestampType{Unit: aw.Millisecond, TimeZone: "UTC"}, Nullable: true},
		aw.Field{Name: "v", Type: aw.PrimitiveTypes.Float64},
	), func(b *array.RecordBuilder) {
		tb := b.Field(0).(*array.TimestampBuilder)
		tb.AppendValues([]aw.Timestamp{
			aw.Timestamp(at.UnixMilli()), 0, aw.Timestamp(at.Add(2 * time.Hour).UnixMilli()),
		}, []bool{true, false, true})
		b.Field(1).(*array.Float64Builder).AppendValues([]float64{1, 2, 3}, nil)
	})

	x, y := scale.Time(), scale.Linear()
	if err := geom.Line(arrow.Source(rec), geom.X("t"), geom.Y("v")).Train(x, y); err != nil {
		t.Fatal(err)
	}
	lo, hi := x.Domain()
	if span := scale.InstantOf(x, hi).Sub(scale.InstantOf(x, lo)); span != 2*time.Hour {
		t.Errorf("the axis spans %v, want the two hours between the rows that have an instant", span)
	}
}

// Materialize is the call whose purpose is to preserve the data, so it is the
// last place a mask may be dropped.
func TestMaterializeKeepsTheMask(t *testing.T) {
	rec := record(t, schemaOf(aw.Field{Name: "k", Type: aw.BinaryTypes.String, Nullable: true}),
		func(b *array.RecordBuilder) {
			b.Field(0).(*array.StringBuilder).AppendValues([]string{"a", "", "b"}, []bool{true, false, true})
		})

	tab := arrow.Materialize(arrow.Source(rec))
	mask, ok := data.NullMask(tab, "k")
	if !ok || !data.IsNull(mask, 1) {
		t.Errorf("the materialised table's mask is %v, want the middle row marked", mask)
	}
}
