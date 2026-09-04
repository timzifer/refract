package gg

import (
	"fmt"
	"sync"

	gogg "github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// fontSet holds the regular and bold sources a backend draws with.
type fontSet struct {
	regular *text.FontSource
	bold    *text.FontSource

	mu    sync.Mutex
	faces map[faceKey]text.Face
}

type faceKey struct {
	size float64
	bold bool
}

var (
	defaultOnce sync.Once
	defaultSet  *fontSet
	defaultErr  error
)

// defaultFonts parses the embedded Go fonts once per process.
//
// Parsing is not free and every chart needs the same two faces, so a package
// singleton is the right shape here. Faces derived from a source are safe for
// concurrent use, which is what makes sharing it across backends sound.
func defaultFonts() (*fontSet, error) {
	defaultOnce.Do(func() {
		reg, err := text.NewFontSource(goregular.TTF)
		if err != nil {
			defaultErr = fmt.Errorf("refract/backend/gg: parsing the embedded regular font: %w", err)
			return
		}
		bold, err := text.NewFontSource(gobold.TTF)
		if err != nil {
			defaultErr = fmt.Errorf("refract/backend/gg: parsing the embedded bold font: %w", err)
			return
		}
		defaultSet = newFontSet(reg, bold)
	})
	return defaultSet, defaultErr
}

func newFontSet(regular, bold *text.FontSource) *fontSet {
	if bold == nil {
		bold = regular
	}
	return &fontSet{regular: regular, bold: bold, faces: map[faceKey]text.Face{}}
}

// face returns a cached face for the given size and weight.
func (f *fontSet) face(size float64, weight int) text.Face {
	if size <= 0 {
		size = 12
	}
	k := faceKey{size: size, bold: weight >= 600}
	f.mu.Lock()
	defer f.mu.Unlock()
	if got, ok := f.faces[k]; ok {
		return got
	}
	src := f.regular
	if k.bold {
		src = f.bold
	}
	fc := src.Face(size)
	f.faces[k] = fc
	return fc
}

// apply sets the face on a context and returns it, so callers can measure with
// the very face they are about to draw with.
func (f *fontSet) apply(c *gogg.Context, size float64, weight int) text.Face {
	fc := f.face(size, weight)
	c.SetFont(fc)
	return fc
}
