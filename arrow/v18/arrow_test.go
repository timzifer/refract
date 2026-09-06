package arrow_test

import (
	"bytes"
	"math"
	"testing"
	"time"

	aw "github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/timzifer/refract"
	"github.com/timzifer/refract/arrow/v18"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/scale"
)

// record builds a batch from a schema and a per-column filler.
func record(t *testing.T, schema *aw.Schema, fill func(*array.RecordBuilder)) aw.Record {
	t.Helper()
	b := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	t.Cleanup(b.Release)
	fill(b)
	rec := b.NewRecord()
	t.Cleanup(rec.Release)
	return rec
}

func schemaOf(fields ...aw.Field) *aw.Schema { return aw.NewSchema(fields, nil) }

func TestAFloat64ColumnIsBorrowed(t *testing.T) {
	rec := record(t, schemaOf(aw.Field{Name: "y", Type: aw.PrimitiveTypes.Float64}),
		func(b *array.RecordBuilder) {
			b.Field(0).(*array.Float64Builder).AppendValues([]float64{1, 2, 3, 4}, nil)
		})

	src := arrow.Source(rec)
	got, ok := src.Float64Column("y")
	if !ok {
		t.Fatal("the float64 column was not offered as numeric")
	}
	want := rec.Column(0).(*array.Float64).Float64Values()
	if len(got) != len(want) || &got[0] != &want[0] {
		t.Error("the column was copied; a float64 column with no nulls must be Arrow's own buffer")
	}
}

func TestNullsBecomeNaN(t *testing.T) {
	rec := record(t, schemaOf(aw.Field{Name: "y", Type: aw.PrimitiveTypes.Float64, Nullable: true}),
		func(b *array.RecordBuilder) {
			b.Field(0).(*array.Float64Builder).AppendValues([]float64{1, 0, 3}, []bool{true, false, true})
		})

	got, ok := arrow.Source(rec).Float64Column("y")
	if !ok {
		t.Fatal("the column was not offered as numeric")
	}
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("got %v, want the present values kept", got)
	}
	if !math.IsNaN(got[1]) {
		t.Errorf("the null became %v, want NaN so that the missing-data policies see it", got[1])
	}
	// The copy is the point: a null cannot be written into Arrow's buffer.
	if same := rec.Column(0).(*array.Float64).Float64Values(); &got[0] == &same[0] {
		t.Error("a column with nulls was borrowed rather than copied")
	}
}

func TestIntegerAndFloat32ColumnsWiden(t *testing.T) {
	rec := record(t, schemaOf(
		aw.Field{Name: "i64", Type: aw.PrimitiveTypes.Int64},
		aw.Field{Name: "i32", Type: aw.PrimitiveTypes.Int32},
		aw.Field{Name: "u8", Type: aw.PrimitiveTypes.Uint8},
		aw.Field{Name: "f32", Type: aw.PrimitiveTypes.Float32},
		aw.Field{Name: "flag", Type: aw.FixedWidthTypes.Boolean},
	), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Int64Builder).AppendValues([]int64{-1, 2}, nil)
		b.Field(1).(*array.Int32Builder).AppendValues([]int32{3, 4}, nil)
		b.Field(2).(*array.Uint8Builder).AppendValues([]uint8{5, 6}, nil)
		b.Field(3).(*array.Float32Builder).AppendValues([]float32{0.5, 1.5}, nil)
		b.Field(4).(*array.BooleanBuilder).AppendValues([]bool{true, false}, nil)
	})

	src := arrow.Source(rec)
	for _, c := range []struct {
		name string
		want []float64
	}{
		{"i64", []float64{-1, 2}},
		{"i32", []float64{3, 4}},
		{"u8", []float64{5, 6}},
		{"f32", []float64{0.5, 1.5}},
		{"flag", []float64{1, 0}},
	} {
		got, ok := src.Float64Column(c.name)
		if !ok {
			t.Errorf("%s was not offered as numeric", c.name)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("%s has %d rows, want %d", c.name, len(got), len(c.want))
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s[%d] = %v, want %v", c.name, i, got[i], c.want[i])
			}
		}
	}
}

func TestTimestampsCarryTheirUnit(t *testing.T) {
	base := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	rec := record(t, schemaOf(
		aw.Field{Name: "ms", Type: &aw.TimestampType{Unit: aw.Millisecond, TimeZone: "UTC"}},
		aw.Field{Name: "ns", Type: &aw.TimestampType{Unit: aw.Nanosecond, TimeZone: "UTC"}},
	), func(b *array.RecordBuilder) {
		b.Field(0).(*array.TimestampBuilder).AppendValues(
			[]aw.Timestamp{aw.Timestamp(base.UnixMilli())}, nil)
		b.Field(1).(*array.TimestampBuilder).AppendValues(
			[]aw.Timestamp{aw.Timestamp(base.UnixNano())}, nil)
	})

	src := arrow.Source(rec)
	for _, name := range []string{"ms", "ns"} {
		got, ok := src.TimeColumn(name)
		if !ok {
			t.Fatalf("%s was not offered as temporal", name)
		}
		if !got[0].UTC().Equal(base) {
			t.Errorf("%s decoded to %v, want %v — the unit comes from the schema, not the values",
				name, got[0].UTC(), base)
		}
	}
}

func TestDateColumns(t *testing.T) {
	day := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	rec := record(t, schemaOf(
		aw.Field{Name: "d32", Type: aw.FixedWidthTypes.Date32},
		aw.Field{Name: "d64", Type: aw.FixedWidthTypes.Date64},
	), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Date32Builder).AppendValues([]aw.Date32{aw.Date32FromTime(day)}, nil)
		b.Field(1).(*array.Date64Builder).AppendValues([]aw.Date64{aw.Date64FromTime(day)}, nil)
	})
	src := arrow.Source(rec)
	for _, name := range []string{"d32", "d64"} {
		got, ok := src.TimeColumn(name)
		if !ok {
			t.Fatalf("%s was not offered as temporal", name)
		}
		if !got[0].UTC().Equal(day) {
			t.Errorf("%s decoded to %v, want %v", name, got[0].UTC(), day)
		}
	}
}

func TestStringAndDictionaryColumns(t *testing.T) {
	dict := &aw.DictionaryType{IndexType: aw.PrimitiveTypes.Int32, ValueType: aw.BinaryTypes.String}
	rec := record(t, schemaOf(
		aw.Field{Name: "plain", Type: aw.BinaryTypes.String},
		aw.Field{Name: "cat", Type: dict},
	), func(b *array.RecordBuilder) {
		b.Field(0).(*array.StringBuilder).AppendValues([]string{"north", "south", "north"}, nil)
		db := b.Field(1).(*array.BinaryDictionaryBuilder)
		for _, v := range []string{"a", "b", "a"} {
			if err := db.AppendString(v); err != nil {
				t.Fatal(err)
			}
		}
	})

	src := arrow.Source(rec)
	plain, ok := src.StringColumn("plain")
	if !ok || len(plain) != 3 || plain[0] != "north" || plain[1] != "south" {
		t.Fatalf("plain string column: got %v ok=%v", plain, ok)
	}
	// Dictionary encoding is how Arrow spells "categorical", and a categorical
	// column is exactly what an ordinal axis or a facet wants.
	cat, ok := src.StringColumn("cat")
	if !ok {
		t.Fatal("a dictionary-encoded string column was not offered as textual")
	}
	if len(cat) != 3 || cat[0] != "a" || cat[1] != "b" || cat[2] != "a" {
		t.Errorf("dictionary column decoded to %v, want [a b a]", cat)
	}
}

func TestAColumnIsOnlyOfferedAsWhatItIs(t *testing.T) {
	rec := record(t, schemaOf(
		aw.Field{Name: "y", Type: aw.PrimitiveTypes.Float64},
		aw.Field{Name: "name", Type: aw.BinaryTypes.String},
	), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Float64Builder).AppendValues([]float64{1}, nil)
		b.Field(1).(*array.StringBuilder).AppendValues([]string{"x"}, nil)
	})
	src := arrow.Source(rec)
	if _, ok := src.TimeColumn("y"); ok {
		t.Error("a float column was offered as temporal")
	}
	if _, ok := src.Float64Column("name"); ok {
		t.Error("a string column was offered as numeric")
	}
	if _, ok := src.Float64Column("absent"); ok {
		t.Error("a column that is not there was offered")
	}
}

func TestColumnsAndLen(t *testing.T) {
	rec := record(t, schemaOf(
		aw.Field{Name: "t", Type: aw.PrimitiveTypes.Float64},
		aw.Field{Name: "v", Type: aw.PrimitiveTypes.Float64},
	), func(b *array.RecordBuilder) {
		b.Field(0).(*array.Float64Builder).AppendValues([]float64{0, 1, 2}, nil)
		b.Field(1).(*array.Float64Builder).AppendValues([]float64{3, 4, 5}, nil)
	})
	src := arrow.Source(rec)
	if src.Len() != 3 {
		t.Errorf("Len is %d, want 3", src.Len())
	}
	cols := src.Columns()
	if len(cols) != 2 || cols[0] != "t" || cols[1] != "v" {
		t.Errorf("Columns is %v, want the schema order [t v]", cols)
	}
}

func TestANilRecordIsAnEmptySource(t *testing.T) {
	src := arrow.Source(nil)
	if src.Len() != 0 || len(src.Columns()) != 0 {
		t.Errorf("a nil record gave %d rows and %v columns, want an empty source", src.Len(), src.Columns())
	}
	if src := arrow.TableSource(nil); src.Len() != 0 {
		t.Errorf("a nil table gave %d rows", src.Len())
	}
}

func TestATableConcatenatesItsChunks(t *testing.T) {
	schema := schemaOf(aw.Field{Name: "y", Type: aw.PrimitiveTypes.Float64})
	first := record(t, schema, func(b *array.RecordBuilder) {
		b.Field(0).(*array.Float64Builder).AppendValues([]float64{1, 2}, nil)
	})
	second := record(t, schema, func(b *array.RecordBuilder) {
		b.Field(0).(*array.Float64Builder).AppendValues([]float64{3, 4, 5}, nil)
	})
	tbl := array.NewTableFromRecords(schema, []aw.Record{first, second})
	t.Cleanup(tbl.Release)

	src := arrow.TableSource(tbl)
	if src.Len() != 5 {
		t.Fatalf("Len is %d, want 5 rows across both chunks", src.Len())
	}
	got, ok := src.Float64Column("y")
	if !ok {
		t.Fatal("the column was not offered as numeric")
	}
	for i, want := range []float64{1, 2, 3, 4, 5} {
		if got[i] != want {
			t.Fatalf("row %d is %v, want %v — the chunks are in the wrong order or missing", i, got[i], want)
		}
	}
}

func TestMaterializeCutsTheSourceLooseFromArrow(t *testing.T) {
	rec := record(t, schemaOf(
		aw.Field{Name: "t", Type: &aw.TimestampType{Unit: aw.Second, TimeZone: "UTC"}},
		aw.Field{Name: "y", Type: aw.PrimitiveTypes.Float64},
		aw.Field{Name: "g", Type: aw.BinaryTypes.String},
	), func(b *array.RecordBuilder) {
		b.Field(0).(*array.TimestampBuilder).AppendValues([]aw.Timestamp{1, 2}, nil)
		b.Field(1).(*array.Float64Builder).AppendValues([]float64{7, 8}, nil)
		b.Field(2).(*array.StringBuilder).AppendValues([]string{"a", "b"}, nil)
	})

	tbl := arrow.Materialize(arrow.Source(rec))
	if tbl.Len() != 2 {
		t.Fatalf("Len is %d, want 2", tbl.Len())
	}
	ys, ok := tbl.Float64Column("y")
	if !ok || ys[0] != 7 {
		t.Fatalf("y is %v ok=%v", ys, ok)
	}
	borrowed := rec.Column(1).(*array.Float64).Float64Values()
	if &ys[0] == &borrowed[0] {
		t.Error("Materialize kept Arrow's buffer; the point of it is that the record can be released")
	}
	if _, ok := tbl.TimeColumn("t"); !ok {
		t.Error("the timestamp column did not survive materialisation")
	}
	if _, ok := tbl.StringColumn("g"); !ok {
		t.Error("the string column did not survive materialisation")
	}
}

// The adapter earns its keep only if a chart can be drawn straight from a
// record, so draw one.
func TestAChartRendersStraightFromARecord(t *testing.T) {
	const n = 200
	rec := record(t, schemaOf(
		aw.Field{Name: "t", Type: &aw.TimestampType{Unit: aw.Millisecond, TimeZone: "UTC"}},
		aw.Field{Name: "v", Type: aw.PrimitiveTypes.Float64},
		aw.Field{Name: "g", Type: aw.BinaryTypes.String},
	), func(b *array.RecordBuilder) {
		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		ts := make([]aw.Timestamp, n)
		vs := make([]float64, n)
		gs := make([]string, n)
		for i := range n {
			ts[i] = aw.Timestamp(base.Add(time.Duration(i) * time.Minute).UnixMilli())
			vs[i] = math.Sin(float64(i) / 10)
			gs[i] = []string{"north", "south"}[i%2]
		}
		b.Field(0).(*array.TimestampBuilder).AppendValues(ts, nil)
		b.Field(1).(*array.Float64Builder).AppendValues(vs, nil)
		b.Field(2).(*array.StringBuilder).AppendValues(gs, nil)
	})

	src := arrow.Source(rec)
	p := refract.New(refract.Size(600, 400), refract.Title("From Arrow"))
	p.X(scale.Time())
	p.Y(scale.Linear(scale.Nice()))
	p.Add(geom.Line(src, geom.X("t"), geom.Y("v")))

	var buf bytes.Buffer
	if err := p.Render(refract.SVGWriter(&buf)); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("<polyline")) {
		t.Error("the chart came out with no line in it")
	}
	if !bytes.Contains(buf.Bytes(), []byte("From Arrow")) {
		t.Error("the chart came out with no title")
	}
}

// The two paths the adapter is about: a float64 column, which is Arrow's own
// buffer, and an int64 one, which has to be widened. The first must not scale
// with the row count at all.
func benchmarkColumn(b *testing.B, field aw.Field, fill func(*array.RecordBuilder, int)) {
	const n = 1_000_000
	bld := array.NewRecordBuilder(memory.DefaultAllocator, schemaOf(field))
	defer bld.Release()
	fill(bld, n)
	rec := bld.NewRecord()
	defer rec.Release()

	b.ReportAllocs()
	for b.Loop() {
		src := arrow.Source(rec)
		got, ok := src.Float64Column(field.Name)
		if !ok || len(got) != n {
			b.Fatalf("read %d rows, ok=%v", len(got), ok)
		}
	}
}

func BenchmarkBorrowedFloat64Column(b *testing.B) {
	benchmarkColumn(b, aw.Field{Name: "y", Type: aw.PrimitiveTypes.Float64},
		func(bld *array.RecordBuilder, n int) {
			vs := make([]float64, n)
			for i := range vs {
				vs[i] = float64(i)
			}
			bld.Field(0).(*array.Float64Builder).AppendValues(vs, nil)
		})
}

func BenchmarkWidenedInt64Column(b *testing.B) {
	benchmarkColumn(b, aw.Field{Name: "y", Type: aw.PrimitiveTypes.Int64},
		func(bld *array.RecordBuilder, n int) {
			vs := make([]int64, n)
			for i := range vs {
				vs[i] = int64(i)
			}
			bld.Field(0).(*array.Int64Builder).AppendValues(vs, nil)
		})
}
