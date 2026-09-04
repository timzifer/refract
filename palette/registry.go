package palette

import (
	"sort"

	"github.com/timzifer/refract/ir"
)

// Ramps are registered by name so that a chart can be written down and read
// back: a spec carries "viridis", not ten hex triples, and a reader who edits
// the file by hand has a word to type rather than a gradient to reproduce.
//
// The name is the scheme name Vega and matplotlib use for the same ramp, so
// that a refract spec and a Vega-Lite one say the same thing where they can.

var ramps = map[string]Ramp{
	"viridis":     Viridis,
	"cividis":     Cividis,
	"magma":       Magma,
	"blues":       Blues,
	"greys":       Greys,
	"blueorange":  BlueOrange,
	"purplegreen": PurpleGreen,
}

// RegisterRamp adds a ramp under a name, replacing any ramp already there. It
// is how a third-party ramp becomes reachable from a spec or a config file.
func RegisterRamp(name string, r Ramp) {
	if name == "" || len(r) == 0 {
		return
	}
	ramps[name] = r
}

// RampByName looks up a registered ramp.
func RampByName(name string) (Ramp, bool) {
	r, ok := ramps[name]
	return r, ok
}

// Qualitative palettes are registered the same way and for the same reason: a
// discrete colour scale names "okabeito" in a spec rather than inlining eight
// hex triples, and a reader editing the document has a word to type.
var qualitatives = map[string]Qualitative{
	"okabeito": OkabeIto,
}

// RegisterQualitative adds a qualitative palette under a name, replacing any
// palette already there. It is how a third-party palette becomes reachable
// from a spec or a config file.
func RegisterQualitative(name string, q Qualitative) {
	if name == "" || len(q) == 0 {
		return
	}
	qualitatives[name] = q
}

// QualitativeByName looks up a registered qualitative palette.
func QualitativeByName(name string) (Qualitative, bool) {
	q, ok := qualitatives[name]
	return q, ok
}

// QualitativeName reports the name a qualitative palette is registered under.
// Like [RampName] it compares colours rather than identity, so a copy is still
// recognised. ok is false for a palette nobody registered.
func QualitativeName(q Qualitative) (string, bool) {
	for _, name := range QualitativeNames() {
		if sameColors(qualitatives[name], q) {
			return name, true
		}
	}
	return "", false
}

// QualitativeNames lists the registered qualitative palettes, sorted.
func QualitativeNames() []string {
	out := make([]string, 0, len(qualitatives))
	for name := range qualitatives {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// RampName reports the name a ramp is registered under.
//
// It compares colours rather than identity, so a ramp that was copied — which
// is what [Ramp.Reverse] and any slice expression produce — is still
// recognised. ok is false for a ramp nobody registered.
func RampName(r Ramp) (string, bool) {
	for _, name := range RampNames() {
		if sameRamp(ramps[name], r) {
			return name, true
		}
	}
	return "", false
}

// RampNames lists the registered ramps, sorted, so that a caller iterating
// them gets the same order every time.
func RampNames() []string {
	out := make([]string, 0, len(ramps))
	for name := range ramps {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func sameRamp(a, b Ramp) bool { return sameColors(a, b) }

func sameColors[A, B ~[]ir.Color](a A, b B) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
