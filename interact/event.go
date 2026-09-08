package interact

import "github.com/timzifer/refract/ir"

// EventKind is what happened.
type EventKind uint8

// The event kinds.
const (
	// Hover is the pointer moving over the chart. It fires on every move,
	// whether or not a mark is under the pointer — [Event.Found] says which.
	Hover EventKind = iota
	// Leave is the pointer leaving the chart, or leaving every mark it was
	// over. A tooltip is dismissed here.
	Leave
	// Click is a press and release on the chart.
	Click
	// Zoom is a change of scale about a point, or into a rectangle.
	Zoom
	// Pan is a translation of the view.
	Pan
)

// String names the kind, for tests and error messages.
func (k EventKind) String() string {
	switch k {
	case Hover:
		return "hover"
	case Leave:
		return "leave"
	case Click:
		return "click"
	case Zoom:
		return "zoom"
	case Pan:
		return "pan"
	}
	return "unknown"
}

// Event is one thing that happened to a chart.
//
// It is one struct rather than one type per kind. CONCEPT.md §13 sketched
// separate HoverEvent and ZoomEvent types, and in Go that shape forces the
// handler through an `any` and a type assertion — so the kinds share a struct
// and each says which of its fields are meaningful. A handler registered for
// one kind never sees another, so the fields that do not apply are never read.
type Event struct {
	// Kind is what happened. Every event has one.
	Kind EventKind

	// Point is where the pointer was, in device space. Hover, Click, Zoom and
	// Pan all have one; Leave carries the last position.
	Point ir.Point

	// Panel is which panel Point is in, or -1 for a point outside every panel.
	Panel int

	// Hit is the mark under the pointer, and Found whether there was one.
	// They are set on Hover and Click.
	Hit   Hit
	Found bool

	// Key identifies the row under the pointer: the value its layer's key
	// column holds at [Hit.Row], from
	// [github.com/timzifer/refract/geom.KeyBy].
	//
	// It is what a caller linking two charts sends across. A row number is an
	// index into a table as it stands this frame, so it names a different
	// measurement after an append, a filter or a window; a key is a value the
	// data carries, so it survives all three and means the same thing in
	// another chart drawn from another table.
	//
	// It is empty when the layer named no key column, when row tracking is off,
	// and when the mark has no row behind it. Those are three different reasons
	// for the same answer, and [Hit.Row] tells them apart when it matters.
	//
	// It is on the event rather than on [Hit] because reading it means reading
	// the data, and this package does not: an Index knows where marks are and
	// which layer drew them, not what the layer holds. The root package fills
	// it in, where the plot's layers and their sources are both in scope.
	Key string

	// Rect is the region a rubber-band zoom selected, in device space. It is
	// set on Zoom when the zoom came from a selection rather than a wheel;
	// [ir.Rect.Empty] reports which.
	Rect ir.Rect

	// Factor is a wheel zoom's scale factor: below 1 zooms in, above 1 zooms
	// out. It is set on Zoom when Rect is empty.
	Factor float64

	// Delta is how far a Pan moved the view, in device units.
	Delta ir.Point
}

// Series is the label of the layer under the pointer, empty when there is no
// hit. It is the field a tooltip reaches for first, so it is spelled out
// rather than left as Hit.Series behind a Found check.
func (e Event) Series() string {
	if !e.Found {
		return ""
	}
	return e.Hit.Series
}
