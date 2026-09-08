package spec_test

import (
	"testing"

	"github.com/timzifer/refract/spec"
)

// Decoding user documents must report errors rather than panic. Rendering is
// intentionally separate: an arbitrary document can request an enormous image.
func FuzzSpec(f *testing.F) {
	for _, seed := range []string{
		`{}`,
		`{"data":{"values":[{"x":1,"y":2}]},"layer":[{"mark":{"type":"point"},"encoding":{"x":{"field":"x"},"y":{"field":"y"}}}]}`,
		`{"encoding":{"x":{"scale":{"type":"linear","format":"#,.2"}}}}`,
		`{"layer":[{"mark":{"type":"text","text":"hello"}}]}`,
		`{"data":{"values":[{"v":1},{"v":3}]},"layer":[{"mark":{"type":"qq"},"encoding":{"x":{"field":"v"}}}]}`,
		`{"data":{"values":[{"x":1,"y":2,"name":"A"}]},"layer":[{"mark":{"type":"text","avoidOverlap":true},"encoding":{"x":{"field":"x"},"y":{"field":"y"},"text":{"field":"name"}}}]}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 64<<10 {
			t.Skip()
		}
		s, err := spec.Parse(b)
		if err != nil {
			return
		}
		if _, err := s.Chart(); err != nil {
			return
		}
		encoded, err := s.Marshal()
		if err != nil {
			t.Fatalf("accepted document cannot be encoded: %v", err)
		}
		back, err := spec.Parse(encoded)
		if err != nil {
			t.Fatalf("encoded document cannot be parsed: %v", err)
		}
		if _, err := back.Chart(); err != nil {
			t.Fatalf("encoded document cannot be decoded: %v", err)
		}
	})
}
