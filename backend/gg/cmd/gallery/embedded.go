package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
)

// An SVG figure can carry a raster inside it: a density chart is an <image>
// element whose href is a base64 PNG, because that is how a big-data scatter is
// drawn (see docs/adr/0011).
//
// That payload cannot be compared byte for byte. It is a deflate stream, and
// deflate output is the standard library's business rather than refract's — the
// same chart, rendered by the same code on two Go releases, produces two
// different streams. Pinning it would be a golden test of `compress/flate`.
//
// So the payloads are lifted out of both documents and compared as images, with
// the same per-channel tolerance the PNG half of every figure already uses; what
// is left — the vector half, and the <image> element's own position and size —
// goes to svgdiff and is compared exactly, as before. That is not a widened
// tolerance. It is the same tolerance, applied to pixels instead of to an
// encoding of them.

// embeddedPrefix is how the SVG emitter writes an embedded raster.
const embeddedPrefix = "data:image/png;base64,"

// embeddedPlaceholder stands in for a whole data URI once its payload has been
// lifted out, so that the surrounding attribute still looks like an attribute
// to svgdiff. It deliberately does not itself start with embeddedPrefix, which
// makes splitting an already-split document a no-op.
const embeddedPlaceholder = "embedded:raster"

// splitEmbedded replaces every embedded PNG payload with a placeholder and
// returns the payloads it removed, in document order.
func splitEmbedded(doc []byte) (stripped []byte, rasters [][]byte) {
	var out bytes.Buffer
	rest := doc
	for {
		i := bytes.Index(rest, []byte(embeddedPrefix))
		if i < 0 {
			out.Write(rest)
			return out.Bytes(), rasters
		}
		payload := rest[i+len(embeddedPrefix):]
		// base64's alphabet has no quote in it, so the attribute's closing
		// quote is the end of the payload.
		end := bytes.IndexByte(payload, '"')
		if end < 0 {
			out.Write(rest)
			return out.Bytes(), rasters
		}
		out.Write(rest[:i])
		out.WriteString(embeddedPlaceholder)
		rasters = append(rasters, payload[:end])
		rest = payload[end:]
	}
}

// compareEmbedded checks that two documents embed the same rasters.
func compareEmbedded(path string, got, want [][]byte) string {
	if len(got) != len(want) {
		return fmt.Sprintf("%s: embeds %d rasters, the committed figure embeds %d", path, len(got), len(want))
	}
	for i := range got {
		a, err := base64.StdEncoding.DecodeString(string(want[i]))
		if err != nil {
			return fmt.Sprintf("%s: raster %d in the committed figure is not valid base64: %v", path, i, err)
		}
		b, err := base64.StdEncoding.DecodeString(string(got[i]))
		if err != nil {
			return fmt.Sprintf("%s: raster %d is not valid base64: %v", path, i, err)
		}
		diff, err := pngDiff(a, b)
		if err != nil {
			return fmt.Sprintf("%s: raster %d: %v", path, i, err)
		}
		if diff > pngTolerance {
			return fmt.Sprintf("%s: raster %d differs (%.4f%% of channels beyond tolerance)", path, i, diff*100)
		}
	}
	return ""
}
