package scale

import (
	"math"
	"strconv"
)

// OrdinalOption configures an ordinal scale.
type OrdinalOption func(*ordinal)

// Categories fixes the category set and its axis order.
//
// Without it the scale discovers categories as it encodes them, in the order
// the data presents them — which is right for data that is already in the
// order you want to read it, and wrong for anything else. A chart whose bars
// reorder themselves because yesterday's export sorted differently is a chart
// nobody trusts.
//
// A label outside a fixed set has no position: it encodes to NaN and the geom
// treats the row as missing under its own policy.
func Categories(labels ...string) OrdinalOption {
	return func(o *ordinal) {
		o.fixed = true
		o.labels = o.labels[:0]
		clear(o.index)
		for _, l := range labels {
			o.register(l)
		}
	}
}

// OrdinalPadding sets the fraction of each slot left blank, in [0, 1). The
// default is 0.2, which separates adjacent bars without making them look
// unrelated.
func OrdinalPadding(f float64) OrdinalOption {
	return func(o *ordinal) {
		if f >= 0 && f < 1 {
			o.padding = f
		}
	}
}

// Ordinal returns a categorical band scale: each category gets an equal slot,
// and a value sits at the centre of its slot.
//
// It satisfies the same [Scale] interface as every continuous scale by
// carrying the category *index* as its numeric domain — see [Categorical]. A
// geom mapping a string column onto an ordinal axis encodes the labels through
// the scale; a geom mapping a numeric or time column onto one treats each
// distinct formatted value as a category, so a bar chart over the values 10,
// 20 and 30 gets three equally spaced bars rather than a numeric axis with
// gaps.
//
// It also satisfies [Band], so bars and boxplots take their width from the
// scale rather than inferring it from the spacing of the data.
func Ordinal(opts ...OrdinalOption) Scale {
	o := &ordinal{padding: 0.2, index: map[string]int{}}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

type ordinal struct {
	domainRange
	labels  []string
	index   map[string]int
	fixed   bool
	padding float64
}

func (o *ordinal) register(label string) int {
	if i, ok := o.index[label]; ok {
		return i
	}
	i := len(o.labels)
	o.labels = append(o.labels, label)
	o.index[label] = i
	return i
}

// Encode implements [Categorical].
func (o *ordinal) Encode(label string) float64 {
	if i, ok := o.index[label]; ok {
		return float64(i)
	}
	if o.fixed {
		return math.NaN()
	}
	return float64(o.register(label))
}

// Labels implements [Categorical].
func (o *ordinal) Labels() []string { return o.labels }

// Train grows the category set so that the given indices exist. Encoding a
// label is the normal way a category comes into being; this is what makes a
// scale trained with bare numbers — an index nothing has named — still
// produce an axis, labelling the slot with its own index.
func (o *ordinal) Train(vs ...float64) {
	if o.fixed {
		return
	}
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			continue
		}
		for i := len(o.labels); i <= int(v); i++ {
			o.register(strconv.Itoa(i))
		}
	}
}

func (o *ordinal) count() int {
	if len(o.labels) == 0 {
		return 1
	}
	return len(o.labels)
}

// Domain reports the index interval the range covers. It runs from half a slot
// before the first category to half a slot after the last, because a category
// sits at the centre of its slot rather than on its edge.
func (o *ordinal) Domain() (float64, float64) { return -0.5, float64(o.count()) - 0.5 }

// Defined implements [Definite]: a value outside the slots has no position.
func (o *ordinal) Defined(v float64) bool {
	lo, hi := o.Domain()
	return !math.IsNaN(v) && v >= lo && v <= hi
}

func (o *ordinal) Map(v float64) float32 {
	rlo, rhi := o.rangeOf()
	lo, hi := o.Domain()
	switch v {
	case lo:
		return rlo
	case hi:
		return rhi
	}
	t := (v - lo) / (hi - lo)
	return rlo + float32(t)*(rhi-rlo)
}

func (o *ordinal) Invert(pos float32) float64 {
	rlo, rhi := o.rangeOf()
	lo, hi := o.Domain()
	if rhi == rlo {
		return lo
	}
	t := float64((pos - rlo) / (rhi - rlo))
	return lo + t*(hi-lo)
}

// Bandwidth implements [Band].
func (o *ordinal) Bandwidth() float32 {
	rlo, rhi := o.rangeOf()
	slot := float32(math.Abs(float64(rhi-rlo))) / float32(o.count())
	return slot * float32(1-o.padding)
}

// Ticks returns one tick per category, at the centre of its slot.
//
// It ignores want. Dropping a category would leave an unlabelled bar, which is
// worse than a crowded axis; the renderer already omits labels that would
// collide, and it omits them from an axis whose ticks are all still there.
func (o *ordinal) Ticks(int) []Tick {
	out := make([]Tick, 0, len(o.labels))
	for i, l := range o.labels {
		out = append(out, Tick{Value: float64(i), Pos: o.Map(float64(i)), Label: l})
	}
	return out
}
