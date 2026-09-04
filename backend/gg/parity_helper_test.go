package gg_test

import (
	"io"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/backend/svg"
	"github.com/timzifer/refract/ir"
	"golang.org/x/image/font/gofont/goregular"
)

// refractSVG builds an SVG target measuring with the same font the gg backend
// draws with, so the two can be compared like for like.
func refractSVG(w io.Writer) ir.Target {
	return refract.SVGWriter(w, svg.WithFont(goregular.TTF))
}
