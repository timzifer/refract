package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/timzifer/refract/internal/svgdiff"
)

// encoded renders a small image as a base64 PNG at the given compression
// level. Two levels give two byte streams for identical pixels, which is
// exactly what two Go releases did to the density figure and turned CI red.
func encoded(t *testing.T, level png.CompressionLevel, shift uint8) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			img.Set(x, y, color.NRGBA{R: uint8(x * 8), G: uint8(y*8) + shift, B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: level}
	if err := enc.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func doc(payload string) []byte {
	return []byte(`<svg><image x="1" y="2" width="3" height="4" href="` +
		embeddedPrefix + payload + `"/><rect x="5"/></svg>`)
}

func TestSplitEmbeddedLiftsThePayloadAndLeavesTheRest(t *testing.T) {
	payload := encoded(t, png.BestSpeed, 0)
	stripped, rasters := splitEmbedded(doc(payload))

	if len(rasters) != 1 {
		t.Fatalf("lifted %d rasters, want 1", len(rasters))
	}
	if string(rasters[0]) != payload {
		t.Error("the lifted payload is not the one that was in the document")
	}
	if bytes.Contains(stripped, []byte(payload)) {
		t.Error("the payload is still in the stripped document")
	}
	// The element around it has to survive, or svgdiff would stop checking
	// where the raster is drawn and how large it is.
	for _, want := range []string{`x="1"`, `y="2"`, `width="3"`, `height="4"`, `<rect x="5"/>`, embeddedPlaceholder} {
		if !bytes.Contains(stripped, []byte(want)) {
			t.Errorf("the stripped document lost %s", want)
		}
	}
}

func TestADocumentWithNoRasterIsUntouched(t *testing.T) {
	in := []byte(`<svg><rect x="1"/></svg>`)
	stripped, rasters := splitEmbedded(in)
	if len(rasters) != 0 {
		t.Errorf("lifted %d rasters from a document with none", len(rasters))
	}
	if !bytes.Equal(stripped, in) {
		t.Errorf("the document was rewritten: %s", stripped)
	}
}

// The regression this whole file exists for: the same pixels, encoded by a
// different compressor, must compare equal. CI hit it as two Go releases
// disagreeing about deflate on a figure neither of them had changed.
func TestTheSamePixelsCompareEqualThroughADifferentEncoder(t *testing.T) {
	fast := encoded(t, png.BestSpeed, 0)
	small := encoded(t, png.BestCompression, 0)
	if fast == small {
		t.Fatal("the two compression levels produced identical bytes; this test proves nothing")
	}

	if msg := compareSVGDocuments(doc(fast), doc(small)); msg != "" {
		t.Errorf("identical pixels were reported as a difference: %s", msg)
	}
}

// ...and the other half: a raster whose pixels really changed must still fail,
// or the comparison would have stopped comparing.
func TestDifferentPixelsAreStillCaught(t *testing.T) {
	a := encoded(t, png.BestSpeed, 0)
	b := encoded(t, png.BestSpeed, 90)
	if msg := compareSVGDocuments(doc(a), doc(b)); msg == "" {
		t.Error("a changed raster was accepted")
	}
}

func TestARasterAppearingOrVanishingIsCaught(t *testing.T) {
	payload := encoded(t, png.BestSpeed, 0)
	plain := []byte(`<svg><rect x="5"/></svg>`)
	if msg := compareSVGDocuments(doc(payload), plain); msg == "" {
		t.Error("a document that gained a raster was accepted")
	}
	if msg := compareSVGDocuments(plain, doc(payload)); msg == "" {
		t.Error("a document that lost a raster was accepted")
	}
}

// A change to where the raster is drawn is vector geometry, not pixels, and
// must still be compared exactly.
func TestTheImageElementsGeometryIsStillCompared(t *testing.T) {
	payload := encoded(t, png.BestSpeed, 0)
	moved := bytes.Replace(doc(payload), []byte(`x="1"`), []byte(`x="40"`), 1)
	if msg := compareSVGDocuments(doc(payload), moved); msg == "" {
		t.Error("a raster drawn in a different place was accepted")
	}
}

// compareSVGDocuments is compareSVG without the file, so that a test can hand
// it two documents directly.
func compareSVGDocuments(got, want []byte) string {
	gotDoc, gotRasters := splitEmbedded(got)
	wantDoc, wantRasters := splitEmbedded(want)
	if msg := compareEmbedded("test.svg", gotRasters, wantRasters); msg != "" {
		return msg
	}
	if ok, why := svgdiff.Equal(gotDoc, wantDoc, svgdiff.DefaultTolerance); !ok {
		return why
	}
	return ""
}

func TestSplittingAnAlreadySplitDocumentIsANoOp(t *testing.T) {
	once, _ := splitEmbedded(doc(encoded(t, png.BestSpeed, 0)))
	twice, rasters := splitEmbedded(once)
	if !bytes.Equal(once, twice) {
		t.Errorf("splitting an already-stripped document changed it:\n%s\n%s", once, twice)
	}
	if len(rasters) != 0 {
		t.Errorf("the second split lifted %q; the placeholder was mistaken for a payload", rasters)
	}
}
