package scale

// Snapshotter is implemented by a scale that can hand back an independent copy
// of itself exactly as it stands — configuration, trained domain, device range
// and all.
//
// It is what makes parallel panels possible. Panels that share an axis share
// one scale object, and drawing sets that object's device range; two panels
// drawing at once would be writing the same field. A snapshot per goroutine
// removes the sharing without changing what any panel draws, because a
// snapshot maps every value exactly as its original does.
//
// It differs from [Cloner], which returns an *untrained* copy for a free facet
// axis. The two exist for opposite reasons: Clone is for a panel that must not
// inherit the plot's domain, Snapshot for one that must inherit it exactly.
//
// It is an optional interface. A scale that does not implement it is drawn one
// panel at a time, which is always correct and is what a single-panel chart
// does anyway.
type Snapshotter interface {
	// Snapshot returns an independent copy that maps, inverts and ticks
	// identically to the receiver.
	Snapshot() Scale
}

// Each snapshot below is a shallow copy plus a copy of whatever the type holds
// behind a pointer. Nothing here shares memory with its original, which is the
// whole requirement: the copy is written to by another goroutine.

func (l *linear) Snapshot() Scale { c := *l; return &c }

func (l *logScale) Snapshot() Scale { c := *l; return &c }

func (s *symlogScale) Snapshot() Scale { c := *s; return &c }

func (t *timeScale) Snapshot() Scale { c := *t; return &c }

// An ordinal scale carries its categories in a map, and Encode writes to it.
// Encoding happens while a geom resolves its columns, before any panel draws —
// but a snapshot that shared the map would still be one concurrent write away
// from a corrupted axis, and the map is at most a few hundred entries.
func (o *ordinal) Snapshot() Scale {
	c := *o
	c.labels = append([]string(nil), o.labels...)
	c.index = make(map[string]int, len(o.index))
	for k, v := range o.index {
		c.index[k] = v
	}
	return &c
}
