// Package arrow adapts Apache Arrow data to refract's data layer.
//
// It is a module of its own so that the core stays what it claims to be: a
// chart library with no dependencies. Arrow is a large dependency and most
// charts have nothing to do with it, so it enters only for the programs that
// already hold Arrow data — and for them it is the shortest possible path,
// because Arrow's columnar layout is the layout refract already wants.
//
// The import path carries a major version, and it is Arrow's rather than
// refract's: this module adapts apache/arrow-go/v18, and its own major version
// follows its upstream's so that the two can never disagree about what an
// Arrow record is. The package name is still arrow.
//
//	import "github.com/timzifer/refract/arrow/v18"
//
//	rec := reader.Record()
//	src := arrow.Source(rec)
//
//	p := refract.New(refract.Title("Latency"))
//	p.Add(geom.Line(src, geom.X("t"), geom.Y("p99")))
//
// # What is borrowed and what is copied
//
// A float64 column with no nulls is borrowed: [data.Source.Float64Column]
// returns Arrow's own buffer, with no copy and no conversion. That is the case
// the two libraries agree about exactly — IEEE-754 doubles, contiguous, one
// per row.
//
// Everything else is converted once, on first use, and cached: an integer or
// float32 column becomes float64, a timestamp becomes time.Time, a string view
// becomes a Go string. A column the chart never reads is never converted, so a
// record with forty columns and a chart that plots two pays for two.
//
// # Nulls
//
// An Arrow null becomes NaN in a numeric column, the zero time in a temporal
// one and the empty string in a categorical one. NaN is what refract's
// missing-data policies are written against, so a null row is gapped,
// interpolated or rejected by the same [geom.OnMissing] setting that handles a
// NaN coming from anywhere else. A float64 column that has nulls is therefore
// copied rather than borrowed: the nulls have to become NaN somewhere, and
// writing them into Arrow's buffer is not this package's memory to write.
//
// # Lifetime and concurrency
//
// A Source borrows the record; it does not retain a reference to it beyond
// what it needs and does not release it. Keep the record alive — and its
// memory unreleased — for as long as the chart may be rendered.
//
// Resolving a column for the first time fills a cache, so a Source is not safe
// for concurrent first use. Render once before sharing one across goroutines,
// or use [Materialize], which converts everything up front and returns a Source
// that is read-only thereafter.
package arrow

import (
	"time"

	arrow "github.com/apache/arrow-go/v18/arrow"
	"github.com/timzifer/refract/data"
)

// Source returns a [data.Source] over one Arrow record batch.
//
// A nil record gives an empty Source rather than a panic: an empty batch is a
// normal thing for a reader to hand back, and a chart over no rows is a chart
// with no marks.
func Source(rec arrow.Record) data.Source {
	if rec == nil {
		return &source{}
	}
	s := &source{n: int(rec.NumRows())}
	for i := range int(rec.NumCols()) {
		s.names = append(s.names, rec.ColumnName(i))
		s.cols = append(s.cols, []arrow.Array{rec.Column(i)})
	}
	return s
}

// TableSource returns a [data.Source] over an Arrow table.
//
// A table holds each column as a list of chunks, and refract's data layer is
// one slice per column — so a chunked column is concatenated on first use.
// A single-chunk column takes the same path a record does, borrowing where it
// can.
func TableSource(t arrow.Table) data.Source {
	if t == nil {
		return &source{}
	}
	s := &source{n: int(t.NumRows())}
	for i := range int(t.NumCols()) {
		col := t.Column(i)
		s.names = append(s.names, col.Name())
		s.cols = append(s.cols, col.Data().Chunks())
	}
	return s
}

// Materialize converts every column src can offer and returns a plain
// [data.Table] holding the results.
//
// It is the escape hatch from the lazy path: the conversions happen once, here,
// and the result shares nothing with Arrow — so the record can be released, and
// the table can be read from as many goroutines as like. The cost is a copy of
// every column, including the float64 ones a lazy Source would have borrowed.
func Materialize(src data.Source) *data.Table {
	t := data.NewTable()
	if src == nil {
		return t
	}
	for _, name := range src.Columns() {
		if v, ok := src.Float64Column(name); ok {
			t.Float64(name, append([]float64(nil), v...))
			continue
		}
		if v, ok := src.TimeColumn(name); ok {
			t.Time(name, append([]time.Time(nil), v...))
			continue
		}
		if v, ok := src.StringColumn(name); ok {
			t.String(name, append([]string(nil), v...))
		}
	}
	return t
}

type source struct {
	names []string
	// One entry per column, holding its chunks: a record batch has exactly
	// one, a table may have many. Keeping the same shape for both is what lets
	// the converters below not care which they came from.
	cols [][]arrow.Array
	n    int

	nums  map[string][]float64
	times map[string][]time.Time
	strs  map[string][]string
}

func (s *source) Len() int          { return s.n }
func (s *source) Columns() []string { return s.names }

func (s *source) at(name string) ([]arrow.Array, bool) {
	for i, n := range s.names {
		if n == name {
			return s.cols[i], true
		}
	}
	return nil, false
}

func (s *source) Float64Column(name string) ([]float64, bool) {
	if v, ok := s.nums[name]; ok {
		return v, true
	}
	chunks, ok := s.at(name)
	if !ok {
		return nil, false
	}
	out, ok := numericColumn(chunks, s.n)
	if !ok {
		return nil, false
	}
	if s.nums == nil {
		s.nums = map[string][]float64{}
	}
	s.nums[name] = out
	return out, true
}

func (s *source) TimeColumn(name string) ([]time.Time, bool) {
	if v, ok := s.times[name]; ok {
		return v, true
	}
	chunks, ok := s.at(name)
	if !ok {
		return nil, false
	}
	out, ok := timeColumn(chunks, s.n)
	if !ok {
		return nil, false
	}
	if s.times == nil {
		s.times = map[string][]time.Time{}
	}
	s.times[name] = out
	return out, true
}

func (s *source) StringColumn(name string) ([]string, bool) {
	if v, ok := s.strs[name]; ok {
		return v, true
	}
	chunks, ok := s.at(name)
	if !ok {
		return nil, false
	}
	out, ok := stringColumn(chunks, s.n)
	if !ok {
		return nil, false
	}
	if s.strs == nil {
		s.strs = map[string][]string{}
	}
	s.strs[name] = out
	return out, true
}

var _ data.Source = (*source)(nil)
