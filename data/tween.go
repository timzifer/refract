package data

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// Alignment is two tables lined up by a key column: one entry per distinct
// key, saying which row of each table carries it.
//
// It is the join a transition is built on, and it is separate from [Tween] so
// that a caller can ask what changed without interpolating anything — which
// rows are new, which are going, which are the same thing in a different
// place.
//
// The key order is first appearance in a, then the keys only b has, in first
// appearance in b. It is never map iteration order: a chart whose panels are
// built on separate goroutines has to draw what a serial one drew, so nothing
// here may depend on how a map felt like enumerating itself. See
// docs/adr/0012-parallel-panels.md.
type Alignment struct {
	// Keys are the distinct keys of both tables, in the order above.
	Keys []string
	// A and B are, for each key, the row carrying it in each table, or -1 for
	// a key that table does not have.
	A, B []int
}

// Len reports how many keys the alignment holds, which is the row count of the
// table a [Tween] over it produces.
func (al Alignment) Len() int { return len(al.Keys) }

// Entered lists the keys only b has: the rows a transition brings in.
func (al Alignment) Entered() []string { return al.only(al.A) }

// Exited lists the keys only a has: the rows a transition takes away.
func (al Alignment) Exited() []string { return al.only(al.B) }

func (al Alignment) only(rows []int) []string {
	var out []string
	for i, r := range rows {
		if r < 0 {
			out = append(out, al.Keys[i])
		}
	}
	return out
}

// ErrNoKeyColumn reports a key column neither table has, or one of a type that
// cannot name a row.
var ErrNoKeyColumn = errors.New("refract/data: no such key column")

// Align lines up two tables by a key column.
//
// A key that appears more than once in a table takes its first row and the
// rest are ignored. That is a choice rather than an error: a duplicate key is
// a caller saying two rows are the same thing, and refusing the whole
// transition over it would be refusing to draw a chart that draws perfectly
// well. What it costs is that the second row does not move, which is visible
// and diagnosable, where a refusal at the top of an animation is neither.
func Align(a, b Source, col string) (Alignment, error) {
	ka, err := keysOf(a, col)
	if err != nil {
		return Alignment{}, err
	}
	kb, err := keysOf(b, col)
	if err != nil {
		return Alignment{}, err
	}

	al := Alignment{}
	at := make(map[string]int, len(ka)+len(kb))
	for row, k := range ka {
		if _, dup := at[k]; dup {
			continue
		}
		at[k] = len(al.Keys)
		al.Keys = append(al.Keys, k)
		al.A = append(al.A, row)
		al.B = append(al.B, -1)
	}
	for row, k := range kb {
		i, seen := at[k]
		if !seen {
			at[k] = len(al.Keys)
			al.Keys = append(al.Keys, k)
			al.A = append(al.A, -1)
			al.B = append(al.B, row)
			continue
		}
		if al.B[i] < 0 {
			al.B[i] = row
		}
	}
	return al, nil
}

func keysOf(src Source, col string) ([]string, error) {
	if src == nil {
		return nil, fmt.Errorf("%w: nil source", ErrNoKeyColumn)
	}
	k, ok := Labels(src, col)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoKeyColumn, col)
	}
	return k, nil
}

// Tween is a table that reads as a blend of two others.
//
// It is a Source whose *contents* change rather than a Source per frame: the
// columns are allocated once, at the joined row count, and rewritten in place
// by [Tween.At]. Build the layer over [Tween.Source] once, and a transition
// then costs no allocations per frame and none per row — the same promise
// [Stream] makes, for the same reason and by the same means.
//
// # What interpolates and what does not
//
// Numeric and temporal columns interpolate. A time is its Unix nanoseconds,
// which is the domain scale.Time already maps, so a time axis needs nothing
// special.
//
// String columns do not. They take the end state's value where the key is in
// it and the start's otherwise, because a string is a name rather than a
// quantity: a sankey's from and to are which nodes an edge joins, and half of
// "ingest" is not a node. That has a consequence worth knowing before reaching
// for this — anything a geom decides from a string column decides it abruptly.
// A bar's slot on a categorical axis, a discrete colour class, a GroupBy
// membership: those snap at the moment the column changes rather than sliding.
//
// # The row set is the union, and it is fixed
//
// A key in either table is a row of the blend for the whole of it. An entering
// row exists at f == 0 — sitting wherever [EnterFrom] put it — and an exiting
// one still exists at f == 1.
//
// That is not a detail. It is what keeps the *structure* of the frame
// identical from one frame to the next, which is the condition ir.Damage needs
// to report that two frames are comparable. A blend whose row count changed
// mid-flight would make every frame a full repaint, and nothing about the
// picture would look wrong.
//
// # It refuses at construction
//
// A column one table has and the other does not, or one numeric on one side
// and textual on the other, is an error from [NewTween] rather than a surprise
// three frames into an animation. A transition is built before any frame runs,
// so there is somewhere honest to put the failure.
type Tween struct {
	a, b Source
	al   Alignment

	names []string
	kinds []colKind

	nums  map[string][]float64
	times map[string][]time.Time
	strs  map[string][]string

	enter map[string]float64
	exit  map[string]float64
	hold  map[string]bool

	f float64
	n int
}

type colKind uint8

const (
	numeric colKind = iota
	temporal
	textual
)

// TweenOption configures a [Tween].
type TweenOption func(*tweenConfig)

type tweenConfig struct {
	enter map[string]float64
	exit  map[string]float64
	hold  map[string]bool
}

// EnterFrom is the value a numeric column takes, at f == 0, for a row only the
// end state has — so a bar grows out of its baseline instead of appearing at
// full height.
//
// A column with no EnterFrom holds its end value, which is a row that arrives
// by not moving. That is the default because it needs no configuration and is
// never wrong; growing from somewhere is the choice, and only the caller knows
// where "nothing yet" is for their data.
//
// There is no fade, and that is a limit rather than an oversight: opacity is a
// property of a layer and not of a row, and adding a per-row opacity channel
// would be a change to the IR that ADR 0007 exists to refuse. A value in data
// space is the honest substitute.
func EnterFrom(col string, v float64) TweenOption {
	return func(c *tweenConfig) { c.enter[col] = v }
}

// ExitTo is [EnterFrom] for a row only the start state has: where it goes on
// its way out. A column with no ExitTo holds its start value.
func ExitTo(col string, v float64) TweenOption {
	return func(c *tweenConfig) { c.exit[col] = v }
}

// Hold names numeric columns that snap at the halfway point rather than
// interpolating.
//
// It is for a number that is a name: an identifier, a category encoded as an
// integer, an axis slot. Halfway between category 2 and category 5 is category
// 3.5, which is not a category — so a column like that wants the abruptness a
// string column gets for free.
func Hold(cols ...string) TweenOption {
	return func(c *tweenConfig) {
		for _, col := range cols {
			c.hold[col] = true
		}
	}
}

// NewTween lines up two tables by a key column and returns the blend between
// them, positioned at f == 0.
func NewTween(a, b Source, key string, opts ...TweenOption) (*Tween, error) {
	al, err := Align(a, b, key)
	if err != nil {
		return nil, err
	}
	cfg := tweenConfig{
		enter: map[string]float64{},
		exit:  map[string]float64{},
		hold:  map[string]bool{},
	}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}

	t := &Tween{
		a: a, b: b, al: al,
		nums:  map[string][]float64{},
		times: map[string][]time.Time{},
		strs:  map[string][]string{},
		enter: cfg.enter, exit: cfg.exit, hold: cfg.hold,
		n: al.Len(),
	}
	if err := t.plan(); err != nil {
		return nil, err
	}
	t.At(0)
	return t, nil
}

// plan settles which columns the blend has and of what kind, and allocates
// each one once. It is where a mismatch is refused.
func (t *Tween) plan() error {
	// The end state's column order, then anything only the start state has, so
	// the blend reads like the table it is becoming.
	seen := map[string]bool{}
	for _, name := range append(append([]string{}, t.b.Columns()...), t.a.Columns()...) {
		if seen[name] {
			continue
		}
		seen[name] = true

		ka, oka := kindOf(t.a, name)
		kb, okb := kindOf(t.b, name)
		switch {
		case oka && okb && ka != kb:
			return fmt.Errorf("refract/data: column %q is %s in one state and %s in the other", name, ka, kb)
		case !oka && !okb:
			continue
		}
		k := kb
		if !okb {
			k = ka
		}
		t.names = append(t.names, name)
		t.kinds = append(t.kinds, k)
		switch k {
		case numeric:
			t.nums[name] = make([]float64, t.n)
		case temporal:
			t.times[name] = make([]time.Time, t.n)
		case textual:
			t.strs[name] = make([]string, t.n)
		}
	}
	if len(t.names) == 0 {
		return errors.New("refract/data: the two states share no columns")
	}
	return nil
}

func (k colKind) String() string {
	switch k {
	case numeric:
		return "numeric"
	case temporal:
		return "temporal"
	}
	return "textual"
}

func kindOf(src Source, name string) (colKind, bool) {
	if _, ok := src.Float64Column(name); ok {
		return numeric, true
	}
	if _, ok := src.TimeColumn(name); ok {
		return temporal, true
	}
	if _, ok := src.StringColumn(name); ok {
		return textual, true
	}
	return numeric, false
}

// At sets the blend fraction, clamped to [0, 1], and rewrites the columns in
// place. It allocates nothing.
func (t *Tween) At(f float64) {
	switch {
	case math.IsNaN(f) || f < 0:
		f = 0
	case f > 1:
		f = 1
	}
	t.f = f
	for i, name := range t.names {
		switch t.kinds[i] {
		case numeric:
			t.blendNumeric(name, f)
		case temporal:
			t.blendTime(name, f)
		case textual:
			t.blendString(name, f)
		}
	}
}

// Fraction reports where the blend currently sits.
func (t *Tween) Fraction() float64 { return t.f }

// Alignment reports the join the blend was built on.
func (t *Tween) Alignment() Alignment { return t.al }

// Rows reports the blend's row count, which is the number of distinct keys and
// does not change.
func (t *Tween) Rows() int { return t.n }

func (t *Tween) blendNumeric(name string, f float64) {
	dst := t.nums[name]
	av, hasA := t.a.Float64Column(name)
	bv, hasB := t.b.Float64Column(name)
	held := t.hold[name]
	for i := range dst {
		ra, rb := t.al.A[i], t.al.B[i]
		lo, okLo := at64(av, hasA, ra)
		hi, okHi := at64(bv, hasB, rb)
		switch {
		case okLo && okHi:
			if held {
				dst[i] = pick(lo, hi, f)
				continue
			}
			dst[i] = lo + (hi-lo)*f
		case okHi: // entering
			if from, ok := t.enter[name]; ok && !held {
				dst[i] = from + (hi-from)*f
				continue
			}
			dst[i] = hi
		case okLo: // exiting
			if to, ok := t.exit[name]; ok && !held {
				dst[i] = lo + (to-lo)*f
				continue
			}
			dst[i] = lo
		default:
			dst[i] = math.NaN()
		}
	}
}

func (t *Tween) blendTime(name string, f float64) {
	dst := t.times[name]
	av, hasA := t.a.TimeColumn(name)
	bv, hasB := t.b.TimeColumn(name)
	held := t.hold[name]
	for i := range dst {
		ra, rb := t.al.A[i], t.al.B[i]
		lo, okLo := atTime(av, hasA, ra)
		hi, okHi := atTime(bv, hasB, rb)
		switch {
		case okLo && okHi:
			if held {
				dst[i] = lo
				if f >= 0.5 {
					dst[i] = hi
				}
				continue
			}
			// In nanoseconds, which is the domain scale.Time maps anyway.
			d := hi.Sub(lo)
			dst[i] = lo.Add(time.Duration(float64(d) * f))
		case okHi:
			dst[i] = hi
		case okLo:
			dst[i] = lo
		default:
			dst[i] = time.Time{}
		}
	}
}

// blendString takes the end state's value where there is one. A string is a
// name rather than a quantity, so there is nothing between two of them.
func (t *Tween) blendString(name string, _ float64) {
	dst := t.strs[name]
	av, hasA := t.a.StringColumn(name)
	bv, hasB := t.b.StringColumn(name)
	for i := range dst {
		if v, ok := atStr(bv, hasB, t.al.B[i]); ok {
			dst[i] = v
			continue
		}
		v, _ := atStr(av, hasA, t.al.A[i])
		dst[i] = v
	}
}

// pick is Hold's rule: the start's value up to the halfway point and the end's
// after it, so a number that is a name is never a number between two names.
func pick(lo, hi, f float64) float64 {
	if f < 0.5 {
		return lo
	}
	return hi
}

func at64(col []float64, ok bool, row int) (float64, bool) {
	if !ok || row < 0 || row >= len(col) {
		return 0, false
	}
	return col[row], true
}

func atTime(col []time.Time, ok bool, row int) (time.Time, bool) {
	if !ok || row < 0 || row >= len(col) {
		return time.Time{}, false
	}
	return col[row], true
}

func atStr(col []string, ok bool, row int) (string, bool) {
	if !ok || row < 0 || row >= len(col) {
		return "", false
	}
	return col[row], true
}

// Source returns the blended table.
//
// It is stable for the life of the Tween: hand it to a layer once, and every
// [Tween.At] afterwards changes what that layer draws without the chart being
// rebuilt. That is the same arrangement [Stream.Source] has, and it is what
// makes a transition cost a frame rather than a frame and a rebuild.
func (t *Tween) Source() Source { return (*tweenView)(t) }

type tweenView Tween

func (v *tweenView) Len() int { return v.n }

func (v *tweenView) Columns() []string { return append([]string(nil), v.names...) }

func (v *tweenView) Float64Column(name string) ([]float64, bool) {
	c, ok := v.nums[name]
	return c, ok
}

func (v *tweenView) TimeColumn(name string) ([]time.Time, bool) {
	c, ok := v.times[name]
	return c, ok
}

func (v *tweenView) StringColumn(name string) ([]string, bool) {
	c, ok := v.strs[name]
	return c, ok
}

var _ Source = (*tweenView)(nil)
