package svgdiff_test

import (
	"strings"
	"testing"

	"github.com/timzifer/refract/internal/svgdiff"
)

func eq(t *testing.T, a, b string) (bool, string) {
	t.Helper()
	return svgdiff.Equal([]byte(a), []byte(b), svgdiff.DefaultTolerance)
}

func TestIdenticalDocumentsMatch(t *testing.T) {
	doc := `<svg width="10"><path d="M0,0 L1.5,2.25 Z" fill="#ff0000"/></svg>`
	if ok, why := eq(t, doc, doc); !ok {
		t.Fatalf("a document did not match itself: %s", why)
	}
}

// TestLastBitDifferenceIsTolerated is the case this package exists for: arm64
// contracts a*b+c into an FMA and amd64 does not, so the same chart can print a
// coordinate one unit in the last place apart.
func TestLastBitDifferenceIsTolerated(t *testing.T) {
	a := `<path d="C100.603,36.6 104.53,44.221 105.244,46.046"/>`
	b := `<path d="C100.603,36.6 104.53,44.221 105.244,46.047"/>`
	if ok, why := eq(t, a, b); !ok {
		t.Fatalf("a one-thousandth difference should be tolerated: %s", why)
	}
}

func TestRealMovementIsCaught(t *testing.T) {
	a := `<path d="M0,0 L100,50"/>`
	b := `<path d="M0,0 L100,51"/>`
	ok, why := eq(t, a, b)
	if ok {
		t.Fatal("a one-pixel move must not be tolerated")
	}
	if !strings.Contains(why, "tolerance") {
		t.Errorf("the explanation should name the tolerance, got: %s", why)
	}
}

func TestDifferenceJustBeyondToleranceIsCaught(t *testing.T) {
	if ok, _ := eq(t, `<a x="1.00"/>`, `<a x="1.02"/>`); ok {
		t.Fatal("0.02 is beyond the 0.01 tolerance and must be caught")
	}
	if ok, why := eq(t, `<a x="1.00"/>`, `<a x="1.009"/>`); !ok {
		t.Fatalf("0.009 is within tolerance: %s", why)
	}
}

func TestNonNumericContentIsComparedExactly(t *testing.T) {
	cases := [][2]string{
		{`<path fill="#ff0000"/>`, `<path fill="#ff0001"/>`},      // colour
		{`<text>Signal</text>`, `<text>Signa1</text>`},            // text content
		{`<path d="M0,0 L1,1"/>`, `<path d="M0,0 C1,1"/>`},        // path verb
		{`<a x="1" y="2"/>`, `<a y="2" x="1"/>`},                  // attribute order
		{`<g><path/></g>`, `<path/>`},                             // structure
		{`<use href="#m1"/>`, `<use href="#m2"/>`},                // ids
		{`<a stroke-linecap="round"/>`, `<a stroke-linecap=""/>`}, // enum values
	}
	for _, c := range cases {
		if ok, _ := eq(t, c[0], c[1]); ok {
			t.Errorf("these should differ:\n  %s\n  %s", c[0], c[1])
		}
	}
}

func TestLengthMismatchIsReported(t *testing.T) {
	if ok, why := eq(t, `<a/><b/>`, `<a/>`); ok || !strings.Contains(why, "longer") {
		t.Fatalf("ok=%v why=%s", ok, why)
	}
	if ok, why := eq(t, `<a/>`, `<a/><b/>`); ok || !strings.Contains(why, "longer") {
		t.Fatalf("ok=%v why=%s", ok, why)
	}
}

func TestSignsAndExponentsParseAsNumbers(t *testing.T) {
	if ok, why := eq(t, `<a x="-1.5" y="2e-3"/>`, `<a x="-1.5" y="2e-3"/>`); !ok {
		t.Fatalf("signed and exponent forms should parse: %s", why)
	}
	if ok, _ := eq(t, `<a x="-1.5"/>`, `<a x="1.5"/>`); ok {
		t.Fatal("a sign flip is a three-unit difference and must be caught")
	}
}

// TestStrayHyphenIsNotANumber guards the parser: text is compared exactly, and
// a hyphen inside a word must not be treated as the start of a numeric literal.
func TestStrayHyphenIsNotANumber(t *testing.T) {
	if ok, _ := eq(t, `<a stroke-width="1"/>`, `<a stroke-height="1"/>`); ok {
		t.Fatal("differing attribute names must be caught")
	}
	if ok, why := eq(t, `<text>a-b</text>`, `<text>a-b</text>`); !ok {
		t.Fatalf("identical text with a hyphen should match: %s", why)
	}
}

func TestEmptyDocuments(t *testing.T) {
	if ok, why := eq(t, "", ""); !ok {
		t.Fatalf("two empty documents should match: %s", why)
	}
}
