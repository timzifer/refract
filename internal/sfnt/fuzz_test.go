package sfnt_test

import (
	"bytes"
	"testing"

	"github.com/timzifer/refract/internal/sfnt"
	"github.com/timzifer/refract/internal/sfnttest"
)

// Exercise the directory, cmap, metrics and composite-glyph readers together:
// a font accepted by Parse must remain safe to query and subset.
func FuzzFont(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("OTTO"))
	f.Add(sfnttest.Font(sfnttest.Cmap4(), sfnttest.NumGlyphs))
	f.Add(sfnttest.Font(sfnttest.Cmap12(), sfnttest.NumGlyphs))
	f.Add(sfnttest.CFFFont())
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			t.Skip()
		}
		font, err := sfnt.Parse(b)
		if err != nil {
			return
		}
		order := []uint16{0}
		for _, r := range []rune{'A', 'é', 'Ω', '中', '\U0001f600'} {
			g := font.GlyphIndex(r)
			font.Advance(g)
			if g != 0 {
				found := false
				for _, old := range order {
					found = found || old == g
				}
				if !found {
					order = append(order, g)
				}
			}
		}
		if !font.CanSubset() {
			return
		}
		out, ids, err := font.Subset(order)
		if err != nil {
			return // malformed outlines can be discovered lazily
		}
		back, err := sfnt.Parse(out)
		if err != nil {
			t.Fatalf("subset cannot be parsed: %v", err)
		}
		for i, old := range ids {
			if back.Advance(uint16(i)) != font.Advance(old) {
				t.Fatalf("subset changed advance of glyph %d", old)
			}
		}
		again, _, err := font.Subset(order)
		if err != nil || !bytes.Equal(out, again) {
			t.Fatal("subset is not deterministic")
		}
	})
}
