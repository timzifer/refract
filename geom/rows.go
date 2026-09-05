package geom

import "github.com/timzifer/refract/ir"

// Rows is where a layer reports which source row is behind each mark it draws.
//
// It exists for the one question a hit test cannot answer from geometry alone:
// *which row is this*. A tooltip is happy with the values under the pointer,
// which come from inverting the scales; highlighting the matching row of a
// table is not, because a neighbouring row highlighted confidently is a wrong
// answer rather than a missing one.
//
// # Why it is separate from drawing
//
// What a layer *draws* and what a reader can *point at* are not the same
// points. A smoothed line is a Bézier path whose control points are not
// measurements; a staircase draws two points per row; a bar is four corners.
// Attributing rows to the points of a drawing call would therefore mean
// attributing them to whichever encoding the geom happened to use. So a geom
// reports its marks — the positions a row actually landed at — separately, and
// the two are correlated afterwards.
//
// # Cost
//
// Nothing, unless someone is listening: [Frame.Rows] is nil for an ordinary
// render, and every geom checks it before doing the bookkeeping. See
// [Frame.Marks].
//
// Rows is implemented outside the geom — by whoever wants the answer — so it
// never gains a method.
type Rows interface {
	// Marks attributes device positions to source rows: rows[i] is the row
	// behind at[i], and a row of -1 marks a position that is not a row.
	//
	// Both slices are lent for the duration of the call, like everything else
	// a geom hands out — they come from a pool, and the next frame writes over
	// them.
	Marks(at []ir.Point, rows []int)
}

// Marks reports the rows behind a set of marks, if anyone asked. A geom calls
// it with the positions a row landed at, whatever it then draws through them.
func (f Frame) Marks(at []ir.Point, rows []int) {
	if f.Rows == nil || rows == nil || len(at) != len(rows) {
		return
	}
	f.Rows.Marks(at, rows)
}

// tracking reports whether this frame's caller wants row identity. Geoms
// consult it before doing work that is only for that.
func (f Frame) tracking() bool { return f.Rows != nil }

// rowsOf returns the source rows behind the marks [scratch.marks] just
// gathered from seg: keep is the reduction's surviving indices, or nil when
// every one of the segment's n elements was kept.
//
// It returns nil when nobody is listening, which is what keeps this free for
// an ordinary render.
func (sc *scratch) rowsOf(seg series, keep []int, n int) []int {
	if !sc.wantRows {
		return nil
	}
	if keep == nil {
		sc.mrows = grow(sc.mrows, n)
		for i := range n {
			sc.mrows[i] = seg.rowAt(i)
		}
		return sc.mrows
	}
	sc.mrows = grow(sc.mrows, len(keep))
	for i, k := range keep {
		sc.mrows[i] = seg.rowAt(k)
	}
	return sc.mrows
}

// sourceRows maps element numbers of s onto source rows, for a geom that has
// already collected the elements it drew — a scatter and a bar both keep such
// a list for their per-mark colours.
//
// It is separate from [scratch.rowsOf] because those geoms index the whole
// series rather than a reduced segment of it, and because the list they keep
// has another use: colours are looked up by element, rows are reported by
// source row, and conflating the two would give a faceted chart the wrong
// colour or the wrong row.
func (sc *scratch) sourceRows(s series, idx []int) []int {
	if !sc.wantRows {
		return nil
	}
	sc.mrows = grow(sc.mrows, len(idx))
	for i, e := range idx {
		sc.mrows[i] = s.rowAt(e)
	}
	return sc.mrows
}

// rowAt returns the source row of element i, in the table the layer's data was
// cut from rather than in the cut.
//
// A segment is normally a contiguous run of the source, so one offset answers
// it. The exception is an interpolated series, whose elements are partly
// values that were never measured; that one carries a row per element, with -1
// where a value was invented, and those rows are already resolved.
func (s series) rowAt(i int) int {
	if s.rows != nil {
		return s.rows[i]
	}
	r := s.off + i
	if s.origin != nil {
		if r < 0 || r >= len(s.origin) {
			return -1
		}
		return s.origin[r]
	}
	return r
}
