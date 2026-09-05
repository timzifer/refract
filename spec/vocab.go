package spec

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
)

// The vocabulary: the translation between refract's names for things and the
// document's. It lives in one file so that the encoder and the decoder cannot
// drift, which is the only way a round trip stays a round trip.

// markType returns the document's mark type and orientation for a geom mark.
//
// Where Vega-Lite has a name for the shape, that is the name used: a rule is a
// rule and a rect is a rect, oriented rather than renamed. Where it does not —
// a step is a Vega-Lite line with an interpolation, a note is a text mark —
// the nearest true statement is made rather than a new word invented.
func markType(m geom.Mark) (typ, orient string, err error) {
	switch m {
	case geom.MarkLine, geom.MarkStep:
		return "line", "", nil
	case geom.MarkScatter:
		return "point", "", nil
	case geom.MarkBar:
		return "bar", "", nil
	case geom.MarkArea:
		return "area", "", nil
	case geom.MarkBoxplot:
		return "boxplot", "", nil
	case geom.MarkHistogram:
		// Vega-Lite reaches a histogram with a bar mark and a `bin` transform on
		// the channel. refract bins in the layer, so the mark carries the name
		// and the bin properties travel with it — see [Mark.Bins].
		return "histogram", "", nil
	case geom.MarkViolin:
		return "violin", "", nil
	case geom.MarkRidgeline:
		return "ridgeline", "", nil
	case geom.MarkHexbin:
		return "hexbin", "", nil
	case geom.MarkBeeswarm:
		return "beeswarm", "", nil
	case geom.MarkECDF:
		return "ecdf", "", nil
	case geom.MarkTrend:
		return "trend", "", nil
	case geom.MarkHLine:
		return "rule", "horizontal", nil
	case geom.MarkVLine:
		return "rule", "vertical", nil
	case geom.MarkSegment:
		return "rule", "", nil
	case geom.MarkHBand:
		return "rect", "horizontal", nil
	case geom.MarkVBand:
		return "rect", "vertical", nil
	case geom.MarkRegion, geom.MarkRect:
		// Both are a rect and neither is oriented, so the type does not tell
		// them apart — the encoding does. See [geomMark].
		return "rect", "", nil
	case geom.MarkNote:
		return "text", "", nil
	}
	return "", "", fmt.Errorf("refract/spec: no mark type for %q", m)
}

// geomMark is markType inverted. The interpolation decides between a line and
// a step, and the orientation between a rule and a segment, or a band and a
// region.
//
// The encoding decides the one case the mark object cannot. `("rect", "")` is
// both a data-driven [geom.Rect] and the [geom.Region] annotation, and the two
// differ in where their corners come from: a rect reads columns and a region
// carries four literals. So a rect with a field is data and a rect with a
// datum is an annotation, which is how Vega-Lite resolves the same ambiguity.
func geomMark(m Mark, enc *Encoding) (geom.Mark, error) {
	switch m.Type {
	case "line":
		if strings.HasPrefix(m.Interpolate, "step") {
			return geom.MarkStep, nil
		}
		return geom.MarkLine, nil
	case "point", "circle", "square":
		return geom.MarkScatter, nil
	case "bar":
		return geom.MarkBar, nil
	case "area":
		return geom.MarkArea, nil
	case "boxplot":
		return geom.MarkBoxplot, nil
	case "histogram":
		return geom.MarkHistogram, nil
	case "violin":
		return geom.MarkViolin, nil
	case "ridgeline":
		return geom.MarkRidgeline, nil
	case "hexbin":
		return geom.MarkHexbin, nil
	case "beeswarm":
		return geom.MarkBeeswarm, nil
	case "ecdf":
		return geom.MarkECDF, nil
	case "trend":
		return geom.MarkTrend, nil
	case "rule":
		switch m.Orient {
		case "horizontal":
			return geom.MarkHLine, nil
		case "vertical":
			return geom.MarkVLine, nil
		}
		return geom.MarkSegment, nil
	case "rect":
		switch m.Orient {
		case "horizontal":
			return geom.MarkHBand, nil
		case "vertical":
			return geom.MarkVBand, nil
		}
		if hasField(enc) {
			return geom.MarkRect, nil
		}
		return geom.MarkRegion, nil
	case "text":
		return geom.MarkNote, nil
	}
	return "", fmt.Errorf("refract/spec: unknown mark type %q", m.Type)
}

// hasField reports whether a layer's encoding names any column, which is what
// separates a layer with data from an annotation placed at literal values.
func hasField(enc *Encoding) bool {
	if enc == nil {
		return false
	}
	for _, ch := range [...]*Channel{enc.X, enc.Y, enc.X2, enc.Y2, enc.Color, enc.Detail, enc.Width, enc.Explode, enc.Size} {
		if ch != nil && ch.Field != "" {
			return true
		}
	}
	return false
}

// Position adjustments. The first three are Vega-Lite's spelling; "wiggle" is
// Vega's name for the streamgraph offset, and "none" is what Vega-Lite writes
// as a JSON null — which an absent field cannot mean here, because absent
// already means "the mark's own default".
var stackings = []struct {
	stack geom.Stacking
	name  string
}{
	{geom.NoStack, "none"},
	{geom.StackZero, "zero"},
	{geom.StackFill, "normalize"},
	{geom.StackSilhouette, "center"},
	{geom.StackWiggle, "wiggle"},
}

func stackName(s geom.Stacking) string {
	for _, x := range stackings {
		if x.stack == s {
			return x.name
		}
	}
	return "zero"
}

func stacking(name string) (geom.Stacking, bool) {
	for _, x := range stackings {
		if x.name == name {
			return x.stack, true
		}
	}
	return geom.NoStack, false
}

// Group orders.
var orderings = []struct {
	order geom.Ordering
	name  string
}{
	{geom.OrderAppearance, "appearance"},
	{geom.OrderValue, "value"},
	{geom.OrderInsideOut, "inside-out"},
}

func orderName(o geom.Ordering) string {
	for _, x := range orderings {
		if x.order == o {
			return x.name
		}
	}
	return "appearance"
}

func ordering(name string) geom.Ordering {
	for _, x := range orderings {
		if x.name == name {
			return x.order
		}
	}
	return geom.OrderAppearance
}

// Step positions, in Vega-Lite's spelling. "step-after" holds each value until
// the next row, which is refract's default and what a sampled signal did.
var steps = []struct {
	pos  geom.StepPos
	name string
}{
	{geom.StepPost, "step-after"},
	{geom.StepPre, "step-before"},
	{geom.StepMid, "step"},
}

func stepName(p geom.StepPos) string {
	for _, s := range steps {
		if s.pos == p {
			return s.name
		}
	}
	return "step-after"
}

func stepPos(name string) geom.StepPos {
	for _, s := range steps {
		if s.name == name {
			return s.pos
		}
	}
	return geom.StepPost
}

// Marker shapes. The first five are Vega-Lite's own shape names; "plus" is
// refract's, because Vega-Lite's "cross" is already the shape refract calls a
// cross and there is no second name to borrow.
var shapes = []struct {
	marker ir.Marker
	name   string
}{
	{ir.MarkerCircle, "circle"},
	{ir.MarkerSquare, "square"},
	{ir.MarkerDiamond, "diamond"},
	{ir.MarkerTriangle, "triangle"},
	{ir.MarkerCross, "cross"},
	{ir.MarkerPlus, "plus"},
}

func shapeName(m ir.Marker) string {
	for _, s := range shapes {
		if s.marker == m {
			return s.name
		}
	}
	return "circle"
}

func markerShape(name string) ir.Marker {
	for _, s := range shapes {
		if s.name == name {
			return s.marker
		}
	}
	return ir.MarkerCircle
}

// Trend fits. "loess" is the name the statistics literature uses and Vega
// spells the same way; "linear" is ordinary least squares.
var smoothings = []struct {
	method geom.Smoothing
	name   string
}{
	{geom.Loess, "loess"},
	{geom.LinearFit, "linear"},
}

func smoothName(m geom.Smoothing) string {
	for _, x := range smoothings {
		if x.method == m {
			return x.name
		}
	}
	return "loess"
}

func smoothing(name string) geom.Smoothing {
	for _, x := range smoothings {
		if x.name == name {
			return x.method
		}
	}
	return geom.Loess
}

// Missing-data policies.
var missings = []struct {
	policy geom.Missing
	name   string
}{
	{geom.Gap, "gap"},
	{geom.Interpolate, "interpolate"},
	{geom.Error, "error"},
}

func missingName(m geom.Missing) string {
	for _, x := range missings {
		if x.policy == m {
			return x.name
		}
	}
	return "gap"
}

func missingPolicy(name string) geom.Missing {
	for _, x := range missings {
		if x.name == name {
			return x.policy
		}
	}
	return geom.Gap
}

// Reductions.
var decimations = []struct {
	mode geom.Decimation
	name string
}{
	{geom.AutoDecimation, "auto"},
	{geom.NoDecimation, "none"},
	{geom.LTTB, "lttb"},
	{geom.MinMax, "minmax"},
	{geom.DensityRaster, "density"},
}

func decimationName(d geom.Decimation) string {
	for _, x := range decimations {
		if x.mode == d {
			return x.name
		}
	}
	return "auto"
}

func decimationMode(name string) geom.Decimation {
	for _, x := range decimations {
		if x.name == name {
			return x.mode
		}
	}
	return geom.AutoDecimation
}

// Text alignment, in Vega-Lite's spelling.
var hAligns = []struct {
	align ir.HAlign
	name  string
}{
	{ir.AlignStart, "left"},
	{ir.AlignCenter, "center"},
	{ir.AlignEnd, "right"},
}

var vAligns = []struct {
	align ir.VAlign
	name  string
}{
	{ir.AlignBaseline, "alphabetic"},
	{ir.AlignTop, "top"},
	{ir.AlignMiddle, "middle"},
	{ir.AlignBottom, "bottom"},
}

func hAlignName(a ir.HAlign) string {
	for _, x := range hAligns {
		if x.align == a {
			return x.name
		}
	}
	return "left"
}

func hAlignOf(name string) ir.HAlign {
	for _, x := range hAligns {
		if x.name == name {
			return x.align
		}
	}
	return ir.AlignStart
}

func vAlignName(a ir.VAlign) string {
	for _, x := range vAligns {
		if x.align == a {
			return x.name
		}
	}
	return "alphabetic"
}

func vAlignOf(name string) ir.VAlign {
	for _, x := range vAligns {
		if x.name == name {
			return x.align
		}
	}
	return ir.AlignBaseline
}

// Colours are hex, as everywhere else on the web: #rrggbb, or #rrggbbaa when
// the colour is not opaque.
func colorHex(c ir.Color) string {
	if c.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

func parseColor(s string) (ir.Color, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	switch len(h) {
	case 3, 4:
		// The short form, doubled: #f0c is #ff00cc.
		var long strings.Builder
		for _, r := range h {
			long.WriteRune(r)
			long.WriteRune(r)
		}
		h = long.String()
	case 6, 8:
	default:
		return ir.Color{}, fmt.Errorf("refract/spec: %q is not a hex colour", s)
	}
	v, err := strconv.ParseUint(h, 16, 64)
	if err != nil {
		return ir.Color{}, fmt.Errorf("refract/spec: %q is not a hex colour", s)
	}
	if len(h) == 6 {
		return ir.RGB(uint8(v>>16), uint8(v>>8), uint8(v)), nil
	}
	return ir.RGBA(uint8(v>>24), uint8(v>>16), uint8(v>>8), uint8(v)), nil
}

// Angles are degrees in the document and radians in the model. Vega-Lite
// measures text rotation in degrees, and a person editing a spec by hand
// should not have to divide by pi.
func degrees(radians float64) float64 { return radians * 180 / math.Pi }
func radians(deg float64) float64     { return deg * math.Pi / 180 }
