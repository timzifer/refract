package data

import (
	"fmt"
	"sync"
	"time"
)

// Stream is a table a producer appends to while a renderer draws.
//
// It is deliberately not a [Source]. A Source is read column by column, over
// several calls, and a table being appended to between two of them is a table
// that disagrees with itself — so the only way to draw a Stream is to freeze
// it:
//
//	st := data.NewStream("t", "y").Window(2000)
//	p.Add(geom.Line(st.Source(), geom.X("t"), geom.Y("y")))
//
//	go func() {
//	    for sample := range samples {
//	        st.Append(scale.Nanos(sample.At), sample.Value)
//	    }
//	}()
//
//	for range ticker.C {
//	    st.Snapshot()   // freeze what has arrived
//	    live.Draw()     // draw the frozen view
//	}
//
// [Stream.Source] hands back a Source that reads the most recent snapshot, so
// a layer is built once rather than per frame. [Stream.Snapshot] is what moves
// it forward.
//
// # Two buffers, one copy
//
// Snapshot copies the live rows into a buffer the renderer is not reading and
// swaps the two. That is one copy per frame — not per row, and not per column
// read — and the buffers are reused, so a steady stream costs no allocations
// after the first frame at each size.
//
// # The one rule
//
// Append may be called from any goroutine, at any time. Snapshot may not be
// called while a render is in flight: it is the swap, and swapping the table
// out from under a half-drawn chart is the race this type exists to remove.
// Producer appends, renderer snapshots and draws, in that order.
//
// # Columns
//
// A Stream carries numeric columns only. A timestamp is its Unix nanoseconds —
// [github.com/timzifer/refract/scale.Nanos] converts one, and
// [github.com/timzifer/refract/scale.Time] maps that domain — so a time series
// needs no second column type and no per-row allocation to carry one.
type Stream struct {
	mu    sync.Mutex
	names []string
	index map[string]int

	// ring holds the live rows, one slice per column. head is where the next
	// row goes and n how many rows are live; when a window is set the two
	// wrap, which is what makes an append O(1) however long the stream runs.
	ring  [][]float64
	head  int
	n     int
	limit int

	bufs [2]*frozen
	cur  int
}

// NewStream returns an empty stream over the named numeric columns.
//
// The order of the names is the order [Stream.Append] takes values in. It
// panics on no columns or a duplicate name, which are programming errors
// rather than runtime conditions.
func NewStream(cols ...string) *Stream {
	if len(cols) == 0 {
		panic("refract/data: NewStream needs at least one column")
	}
	s := &Stream{
		names: append([]string(nil), cols...),
		index: make(map[string]int, len(cols)),
		ring:  make([][]float64, len(cols)),
	}
	for i, name := range cols {
		if _, dup := s.index[name]; dup {
			panic("refract/data: duplicate column " + name)
		}
		s.index[name] = i
	}
	s.bufs[0] = newFrozen(s.names)
	s.bufs[1] = newFrozen(s.names)
	return s
}

// Window caps the stream at the last n rows, dropping the oldest as new ones
// arrive. It returns s so the call can be chained onto [NewStream].
//
// A window is what makes a stream a *stream* rather than a log: a live chart
// shows the last few thousand samples, and keeping every sample since the
// process started is a memory leak with a plot attached. Zero means unbounded,
// which is the default.
//
// Setting a window smaller than the rows already held drops the oldest of
// them, because that is what the window means.
func (s *Stream) Window(n int) *Stream {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n < 0 {
		n = 0
	}
	if n == s.limit {
		return s
	}
	// Unwrap into row order before the geometry changes: every slot number in
	// the ring is relative to the window it was written under.
	keep := s.n
	if n > 0 && keep > n {
		keep = n
	}
	s.compact(keep, s.limit)
	s.limit = n
	return s
}

// ErrColumnCount reports an Append whose value count does not match the
// stream's columns.
var ErrColumnCount = fmt.Errorf("refract/data: wrong number of values")

// Append adds one row. The values are positional, in the order [NewStream] was
// given.
//
// It is safe to call from any goroutine, and it allocates nothing in the
// steady state: the row is copied into buffers that are already the right
// size.
func (s *Stream) Append(vals ...float64) error {
	if len(vals) != len(s.names) {
		return fmt.Errorf("%w: %d values for %d columns", ErrColumnCount, len(vals), len(s.names))
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.limit == 0 {
		for i, v := range vals {
			s.ring[i] = append(s.ring[i], v)
		}
		s.n++
		return nil
	}
	if len(s.ring[0]) < s.limit {
		for i, v := range vals {
			s.ring[i] = append(s.ring[i], v)
		}
		s.n++
		s.head = s.n % s.limit
		return nil
	}
	for i, v := range vals {
		s.ring[i][s.head] = v
	}
	s.head = (s.head + 1) % s.limit
	if s.n < s.limit {
		s.n++
	}
	return nil
}

// AppendTime is [Stream.Append] with the first column given as a timestamp,
// which is what the first column of a live chart almost always is. The
// timestamp becomes its Unix nanoseconds, the domain a
// [github.com/timzifer/refract/scale.Time] axis maps.
func (s *Stream) AppendTime(t time.Time, vals ...float64) error {
	// A fixed array for the row a chart actually has, so that the convenience
	// costs no allocation either.
	var buf [8]float64
	row := buf[:0]
	if len(vals)+1 > len(buf) {
		row = make([]float64, 0, len(vals)+1)
	}
	row = append(row, float64(t.UnixNano()))
	row = append(row, vals...)
	return s.Append(row...)
}

// Len reports how many rows are live. It is the producer's view, which may be
// ahead of what the last snapshot froze.
func (s *Stream) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

// Columns lists the stream's columns, in append order.
func (s *Stream) Columns() []string { return append([]string(nil), s.names...) }

// Reset empties the stream, keeping its buffers. The snapshot a renderer is
// holding is untouched until the next [Stream.Snapshot].
func (s *Stream) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.ring {
		s.ring[i] = s.ring[i][:0]
	}
	s.head, s.n = 0, 0
}

// Snapshot freezes the live rows and returns a Source over them.
//
// The Source [Stream.Source] returned reads the same frozen rows, so a layer
// built once draws whatever the last Snapshot froze.
//
// The returned Source is valid until the next call to Snapshot, which reuses
// its memory. Call it between frames, never during one.
func (s *Stream) Snapshot() Source {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := 1 - s.cur
	f := s.bufs[next]
	f.fill(s)
	s.cur = next
	return f
}

// Source returns a Source over the stream's most recent snapshot.
//
// It is stable: the same value reads every frame, and what it reads changes
// only when [Stream.Snapshot] is called. That is what lets a layer be built
// once, before the first row has even arrived.
func (s *Stream) Source() Source { return &streamView{s: s} }

// compact keeps the last keep rows, in row order, under a ring whose window
// was limit. It runs under the lock and leaves the ring unwrapped: head is
// zero and row i is at slot i.
//
// The slots are resolved before any column is rewritten. [Stream.at] answers
// from the ring's length, and rewriting the first column would change the
// answer for the rest — which is a bug that only shows up on the second
// column, and only after the ring has wrapped.
func (s *Stream) compact(keep, limit int) {
	if keep <= 0 {
		for i := range s.ring {
			s.ring[i] = s.ring[i][:0]
		}
		s.n, s.head = 0, 0
		return
	}
	slots := make([]int, keep)
	for j := range keep {
		slots[j] = s.at(s.n-keep+j, limit)
	}
	for i := range s.ring {
		out := make([]float64, keep)
		for j, at := range slots {
			out[j] = s.ring[i][at]
		}
		s.ring[i] = out
	}
	s.n, s.head = keep, 0
}

// at maps a row number in [0, n) onto its slot in a ring windowed at limit.
func (s *Stream) at(i, limit int) int {
	if limit == 0 || len(s.ring[0]) < limit {
		return i
	}
	return (s.head + i) % limit
}

// streamView is the stable Source. Every call reads through to the buffer the
// last snapshot filled, which is why a layer holding one does not have to be
// rebuilt per frame.
type streamView struct{ s *Stream }

func (v *streamView) current() *frozen {
	v.s.mu.Lock()
	defer v.s.mu.Unlock()
	return v.s.bufs[v.s.cur]
}

func (v *streamView) Len() int          { return v.current().Len() }
func (v *streamView) Columns() []string { return v.current().Columns() }

func (v *streamView) Float64Column(name string) ([]float64, bool) {
	return v.current().Float64Column(name)
}

func (v *streamView) TimeColumn(string) ([]time.Time, bool) { return nil, false }
func (v *streamView) StringColumn(string) ([]string, bool)  { return nil, false }

// frozen is one of the two buffers: a plain columnar Source that nothing
// writes to while it is the current one.
type frozen struct {
	names []string
	cols  [][]float64
	n     int
}

func newFrozen(names []string) *frozen {
	return &frozen{names: names, cols: make([][]float64, len(names))}
}

// fill copies the stream's live rows in, growing the buffers only when the row
// count grows past what they already hold.
func (f *frozen) fill(s *Stream) {
	f.n = s.n
	for i := range f.cols {
		f.cols[i] = grow(f.cols[i], s.n)
		if s.limit == 0 {
			copy(f.cols[i], s.ring[i][:s.n])
			continue
		}
		// Unwrap the ring into row order, which is the order a line has to be
		// drawn in.
		for j := range s.n {
			f.cols[i][j] = s.ring[i][s.at(j, s.limit)]
		}
	}
}

func grow(buf []float64, n int) []float64 {
	if cap(buf) >= n {
		return buf[:n]
	}
	return make([]float64, n)
}

func (f *frozen) Len() int          { return f.n }
func (f *frozen) Columns() []string { return f.names }

func (f *frozen) Float64Column(name string) ([]float64, bool) {
	for i, n := range f.names {
		if n == name {
			return f.cols[i][:f.n], true
		}
	}
	return nil, false
}

func (f *frozen) TimeColumn(string) ([]time.Time, bool) { return nil, false }
func (f *frozen) StringColumn(string) ([]string, bool)  { return nil, false }

var (
	_ Source = (*frozen)(nil)
	_ Source = (*streamView)(nil)
)
