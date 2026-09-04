// Package stat aggregates data before it is drawn.
//
// Everything here answers the same question: a column has more rows than the
// plot has pixels, so which rows actually decide what the reader sees? The
// answers differ by mark. A line wants the rows that preserve its shape; a
// signal envelope wants the extremes of every pixel column, because a spike
// one sample wide is the reason someone opened the chart; a point cloud wants
// no rows at all but a count per cell, drawn as an image.
//
// None of it changes a scale's domain. A geom trains on every row and
// aggregates only when it draws, so an axis reports what the data holds rather
// than what survived the reduction.
//
// The functions come in pairs: LTTB and AppendLTTB, MinMax and AppendMinMax.
// The Append forms write into a caller-owned slice, which is how a chart
// redrawn every frame keeps its per-frame allocations flat.
package stat

// Float is the coordinate type these functions accept.
//
// Both widths are here because both are real: a geom decimates in device
// space, where coordinates are float32 and a pixel is the unit that matters,
// while a caller aggregating before it ever reaches a chart has float64 data.
// Converting one to the other to cross this boundary would cost a copy of the
// whole column.
type Float interface{ ~float32 | ~float64 }
