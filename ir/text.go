package ir

// FontRef names a font logically. refract never carries glyph data or font
// files through the IR: a backend resolves a FontRef with whatever font stack
// it has (gg's shaper, the SVG viewer, a host framework's text engine).
type FontRef struct {
	Family string  // logical family; "" means the backend's default sans
	Size   float64 // em size in device units
	Weight int     // CSS-style numeric weight; 0 means 400
	Italic bool
}

// HAlign is horizontal alignment of a text run relative to its anchor point.
type HAlign uint8

// The horizontal alignments.
const (
	AlignStart HAlign = iota
	AlignCenter
	AlignEnd
)

// VAlign is vertical alignment of a text run relative to its anchor point.
type VAlign uint8

// The vertical alignments. AlignBaseline puts the anchor on the text baseline;
// the others align the anchor to the run's font box.
const (
	AlignBaseline VAlign = iota
	AlignTop
	AlignMiddle
	AlignBottom
)

// TextRun is a single-line string to be drawn.
//
// refract passes strings, never pre-shaped glyphs: shaping belongs to the
// active backend. refract owns only placement — anchoring, rotation, and
// overlap avoidance. Paragraph layout is out of scope (see CONCEPT.md §5).
type TextRun struct {
	Text     string
	Font     FontRef
	At       Point   // anchor position in device space
	H        HAlign  // horizontal alignment about At
	V        VAlign  // vertical alignment about At
	Rotation float64 // radians, clockwise, about At
	Color    Color
}

// TextMetrics is what layout needs back from a backend about a run.
//
// Measure is the only text capability refract requires beyond drawing: layout
// must size margins, space ticks, and detect label collisions using metrics
// from the very shaper that will draw the text.
type TextMetrics struct {
	// Advance is the total advance width of the run.
	Advance float32
	// Ascent and Descent are the font's ascent above and descent below the
	// baseline, both positive. They describe the font box, not this run's ink,
	// so successive runs in one font line up.
	Ascent  float32
	Descent float32
	// Ink is the run's tight bounding box relative to an origin at the
	// baseline start, Y up-negative as usual for screen coordinates.
	Ink Rect
}

// Height returns the font box height, Ascent + Descent.
func (m TextMetrics) Height() float32 { return m.Ascent + m.Descent }
