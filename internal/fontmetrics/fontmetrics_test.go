package fontmetrics_test

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/timzifer/refract/internal/fontmetrics"
)

func TestBuiltinAdvancesScaleWithSize(t *testing.T) {
	a := fontmetrics.Builtin(10, 400, false)
	b := fontmetrics.Builtin(20, 400, false)

	if got, want := a.Advance("iii"), 3*0.222*10; math.Abs(got-want) > 1e-9 {
		t.Errorf(`Advance("iii") at 10px = %v, want %v`, got, want)
	}
	if math.Abs(b.Advance("Hello")-2*a.Advance("Hello")) > 1e-9 {
		t.Error("doubling the size must double the advance")
	}
	if a.Advance("") != 0 {
		t.Error("the empty string has zero advance")
	}
}

func TestBuiltinDistinguishesGlyphWidths(t *testing.T) {
	f := fontmetrics.Builtin(12, 400, false)
	// A proportional table is the whole point: if 'i' and 'M' measured the
	// same, tick-label collision detection would be worthless.
	if f.Advance("i") >= f.Advance("M") {
		t.Error("'i' must be narrower than 'M'")
	}
}

func TestBuiltinBoldIsWider(t *testing.T) {
	reg := fontmetrics.Builtin(12, 400, false)
	bold := fontmetrics.Builtin(12, 700, false)
	if bold.Advance("Hello") <= reg.Advance("Hello") {
		t.Error("bold text must measure at least as wide as regular")
	}
}

func TestBuiltinFallsBackForUnknownRunes(t *testing.T) {
	f := fontmetrics.Builtin(12, 400, false)
	// CJK is outside the table; it must still produce a positive, finite width
	// rather than zero, or margins collapse on a chart with CJK labels.
	if w := f.Advance("日本語"); w <= 0 || math.IsInf(w, 0) {
		t.Fatalf("Advance for out-of-table runes = %v", w)
	}
}

func TestBuiltinHasSaneVerticalMetrics(t *testing.T) {
	f := fontmetrics.Builtin(20, 400, false)
	if f.Ascent() <= 0 || f.Descent() <= 0 {
		t.Fatalf("ascent %v descent %v, both must be positive", f.Ascent(), f.Descent())
	}
	// A sans font box is a little under the em to a little over it.
	if h := f.Ascent() + f.Descent(); h < 0.85*20 || h > 1.3*20 {
		t.Errorf("font box height %v at 20px is implausible", h)
	}
}

func TestParseReadsAdvancesAndMetrics(t *testing.T) {
	font, err := fontmetrics.Parse(syntheticFont(t))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	face := font.Face(1000) // one em per unit, so advances read directly

	if got := face.Advance("A"); math.Abs(got-500) > 1e-6 {
		t.Errorf(`Advance("A") = %v, want 500`, got)
	}
	if got := face.Advance("B"); math.Abs(got-750) > 1e-6 {
		t.Errorf(`Advance("B") = %v, want 750`, got)
	}
	if got := face.Advance("AB"); math.Abs(got-1250) > 1e-6 {
		t.Errorf(`Advance("AB") = %v, want 1250 — advances must sum`, got)
	}
	if got := face.Ascent(); math.Abs(got-800) > 1e-6 {
		t.Errorf("Ascent() = %v, want 800", got)
	}
	if got := face.Descent(); math.Abs(got-200) > 1e-6 {
		t.Errorf("Descent() = %v, want 200 — hhea stores it negative and it must come back positive", got)
	}
}

func TestParseScalesWithFaceSize(t *testing.T) {
	font, err := fontmetrics.Parse(syntheticFont(t))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := font.Face(100).Advance("A"); math.Abs(got-50) > 1e-6 {
		t.Errorf("Advance at 100px = %v, want 50", got)
	}
}

func TestParseUsesTheFallbackForUnmappedRunes(t *testing.T) {
	font, err := fontmetrics.Parse(syntheticFont(t))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// 'Z' is not in the synthetic cmap; it must measure as the .notdef advance
	// rather than as zero.
	if got := font.Face(1000).Advance("Z"); got <= 0 {
		t.Fatalf(`Advance("Z") = %v, want the fallback advance`, got)
	}
}

func TestParseRejectsGarbageWithoutPanicking(t *testing.T) {
	// Font files arrive from users. Every one of these must produce an error,
	// not a panic and not a silently wrong measurement.
	cases := map[string][]byte{
		"empty":        {},
		"short":        {0, 1, 0, 0},
		"wrong tag":    append([]byte("XXXX"), make([]byte, 64)...),
		"header only":  append([]byte{0, 1, 0, 0}, make([]byte, 8)...),
		"truncated":    truncated(t),
		"no cmap":      withoutTable(t, "cmap"),
		"no head":      withoutTable(t, "head"),
		"no hmtx":      withoutTable(t, "hmtx"),
		"no hhea":      withoutTable(t, "hhea"),
		"nul-stuffing": make([]byte, 512),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := fontmetrics.Parse(in); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func truncated(t *testing.T) []byte {
	t.Helper()
	b := syntheticFont(t)
	return b[:len(b)/2]
}

func withoutTable(t *testing.T, drop string) []byte {
	t.Helper()
	return buildFont(t, drop)
}

// --- a minimal font, built in memory -------------------------------------

// syntheticFont builds the smallest font that carries real metrics: head,
// hhea, hmtx and a format-4 cmap, and nothing else.
//
// Building it here rather than committing a .ttf keeps the core module free of
// binary fixtures and makes the expected numbers visible in the test.
func syntheticFont(t *testing.T) []byte { return buildFont(t, "") }

func buildFont(t *testing.T, drop string) []byte {
	t.Helper()

	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000) // unitsPerEm

	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[4:6], 800)                        // ascender
	binary.BigEndian.PutUint16(hhea[6:8], uint16(0xFFFF&int32(-200))) // descender, stored negative
	binary.BigEndian.PutUint16(hhea[34:36], 4)                        // numberOfHMetrics
	//
	// Four glyphs: .notdef, A, B, C.
	advances := []uint16{600, 500, 750, 400}
	hmtx := make([]byte, 0, len(advances)*4)
	for _, a := range advances {
		hmtx = binary.BigEndian.AppendUint16(hmtx, a)
		hmtx = binary.BigEndian.AppendUint16(hmtx, 0) // lsb
	}

	cmap := buildCmap()

	type table struct {
		tag  string
		data []byte
	}
	tables := []table{{"cmap", cmap}, {"head", head}, {"hhea", hhea}, {"hmtx", hmtx}}
	if drop != "" {
		kept := tables[:0]
		for _, tb := range tables {
			if tb.tag != drop {
				kept = append(kept, tb)
			}
		}
		tables = kept
	}

	n := len(tables)
	out := make([]byte, 0, 1024)
	out = binary.BigEndian.AppendUint32(out, 0x00010000)
	out = binary.BigEndian.AppendUint16(out, uint16(n))
	out = binary.BigEndian.AppendUint16(out, 0) // searchRange
	out = binary.BigEndian.AppendUint16(out, 0) // entrySelector
	out = binary.BigEndian.AppendUint16(out, 0) // rangeShift

	offset := 12 + n*16
	records := make([]byte, 0, n*16)
	body := make([]byte, 0, 512)
	for _, tb := range tables {
		records = append(records, tb.tag...)
		records = binary.BigEndian.AppendUint32(records, 0) // checksum
		records = binary.BigEndian.AppendUint32(records, uint32(offset))
		records = binary.BigEndian.AppendUint32(records, uint32(len(tb.data)))
		body = append(body, tb.data...)
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		offset = 12 + n*16 + len(body)
	}
	out = append(out, records...)
	out = append(out, body...)
	return out
}

// buildCmap builds a format-4 subtable mapping 'A'..'C' to glyphs 1..3.
func buildCmap() []byte {
	const segCount = 2
	endCodes := []uint16{'C', 0xFFFF}
	startCodes := []uint16{'A', 0xFFFF}
	// idDelta maps a start code to its glyph: 'A' (65) must become glyph 1.
	idDeltas := []uint16{uint16(int32(1-'A') & 0xFFFF), 1}
	idRangeOffsets := []uint16{0, 0}

	sub := make([]byte, 0, 64)
	sub = binary.BigEndian.AppendUint16(sub, 4) // format
	sub = binary.BigEndian.AppendUint16(sub, 0) // length, patched below
	sub = binary.BigEndian.AppendUint16(sub, 0) // language
	sub = binary.BigEndian.AppendUint16(sub, segCount*2)
	sub = binary.BigEndian.AppendUint16(sub, 4) // searchRange
	sub = binary.BigEndian.AppendUint16(sub, 1) // entrySelector
	sub = binary.BigEndian.AppendUint16(sub, 0) // rangeShift
	for _, v := range endCodes {
		sub = binary.BigEndian.AppendUint16(sub, v)
	}
	sub = binary.BigEndian.AppendUint16(sub, 0) // reservedPad
	for _, v := range startCodes {
		sub = binary.BigEndian.AppendUint16(sub, v)
	}
	for _, v := range idDeltas {
		sub = binary.BigEndian.AppendUint16(sub, v)
	}
	for _, v := range idRangeOffsets {
		sub = binary.BigEndian.AppendUint16(sub, v)
	}
	binary.BigEndian.PutUint16(sub[2:4], uint16(len(sub)))

	out := make([]byte, 0, len(sub)+12)
	out = binary.BigEndian.AppendUint16(out, 0) // version
	out = binary.BigEndian.AppendUint16(out, 1) // numTables
	out = binary.BigEndian.AppendUint16(out, 3) // platform: Windows
	out = binary.BigEndian.AppendUint16(out, 1) // encoding: BMP
	out = binary.BigEndian.AppendUint32(out, 12)
	return append(out, sub...)
}

func TestSyntheticFontIsWellFormedEnough(t *testing.T) {
	// A guard on the fixture itself: if the builder drifts, the parser tests
	// above would start passing or failing for the wrong reason.
	b := syntheticFont(t)
	if len(b) < 12 || binary.BigEndian.Uint32(b[:4]) != 0x00010000 {
		t.Fatal("the synthetic font does not start with a TrueType header")
	}
	if strings.Count(string(b[12:12+4*16]), "head") != 1 {
		t.Fatal("the table directory does not list head exactly once")
	}
}
