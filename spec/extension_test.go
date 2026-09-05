package spec_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
	"github.com/timzifer/refract/spec"
	"github.com/timzifer/refract/theme"
)

// stemGeom is a mark this module does not define: it reads the shared options
// through geom.Configure, takes one knob of its own through geom.Extra, and is
// registered so that a document naming it reads back.
type stemGeom struct {
	src data.Source
	cfg geom.Desc
}

const markStem geom.Mark = "test.stem"

func newStem(src data.Source, opts ...geom.Option) geom.Geom {
	return &stemGeom{src: src, cfg: geom.Configure(opts...)}
}

func (g *stemGeom) Train(x, y scale.Scale) error {
	xs, _ := g.src.Float64Column(g.cfg.X)
	ys, _ := g.src.Float64Column(g.cfg.Y)
	x.Train(xs...)
	y.Train(ys...)
	return nil
}

func (g *stemGeom) Build(b ir.Backend, f geom.Frame) error {
	xs, _ := g.src.Float64Column(g.cfg.X)
	ys, _ := g.src.Float64Column(g.cfg.Y)
	w := float32(1)
	if v, ok := g.cfg.Extra["stem"].(float64); ok {
		w = float32(v)
	}
	c := f.Coords()
	for i := range xs {
		b.Polyline([]ir.Point{
			c.Point(f.X.Map(xs[i]), f.Y.Map(0)),
			c.Point(f.X.Map(xs[i]), f.Y.Map(ys[i])),
		}, ir.Stroke{Color: ir.RGB(1, 2, 3), Width: w})
	}
	return nil
}

func (g *stemGeom) Legend(geom.Frame) (geom.LegendEntry, bool) { return geom.LegendEntry{}, false }

func (g *stemGeom) Describe() geom.Desc {
	d := g.cfg
	d.Mark, d.Source = markStem, g.src
	return d
}

func registerStem() {
	geom.Register(markStem, func(d geom.Desc) (geom.Geom, error) {
		if d.Source == nil {
			return nil, errors.New("a stem layer needs a data source")
		}
		return newStem(d.Source, d.Options()...), nil
	})
}

func TestARegisteredMarkSurvivesTheRoundTrip(t *testing.T) {
	registerStem()
	src := table()
	in := spec.Chart{Width: 300, Height: 200, DPR: 1, Theme: theme.Light, Layers: []geom.Geom{
		newStem(src, geom.X("x"), geom.Y("y"), geom.Width(2), geom.Extra("stem", 3.0), geom.Extra("cap", "round")),
	}}
	out := roundTrip(t, in)

	if got, want := draw(t, out), draw(t, in); !reflect.DeepEqual(got, want) {
		t.Errorf("the rebuilt chart draws differently:\n got %v\nwant %v", got, want)
	}
	back, _ := geom.Describe(out.Layers[0])
	if back.Extra["stem"] != 3.0 || back.Extra["cap"] != "round" {
		t.Errorf("Extra did not survive: %v", back.Extra)
	}
	if back.Width != 2 || back.X != "x" {
		t.Errorf("the shared options did not survive: %+v", back)
	}
}

func TestAMarksOwnPropertiesSitOnTheMarkObject(t *testing.T) {
	registerStem()
	s, err := spec.Of(spec.Chart{Layers: []geom.Geom{
		newStem(table(), geom.X("x"), geom.Y("y"), geom.Extra("stem", 3.0)),
	}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Layer []struct {
			Mark map[string]any `json:"mark"`
		} `json:"layer"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	m := doc.Layer[0].Mark
	if m["type"] != "test.stem" {
		t.Errorf("type = %v, want the registered name", m["type"])
	}
	if m["stem"] != 3.0 {
		t.Errorf("stem = %v, want it beside the built-in properties, not nested", m["stem"])
	}
}

func TestAnExtraCannotShadowABuiltInProperty(t *testing.T) {
	registerStem()
	s, err := spec.Of(spec.Chart{Layers: []geom.Geom{
		newStem(table(), geom.X("x"), geom.Y("y"), geom.Extra("strokeWidth", 9.0)),
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Marshal()
	if err == nil || !strings.Contains(err.Error(), "strokeWidth") {
		t.Errorf("err = %v, want a refusal naming the key", err)
	}
}

func TestAHandWrittenRegisteredMarkReads(t *testing.T) {
	registerStem()
	doc := `{"data":{"values":[{"x":0,"y":1},{"x":1,"y":2}]},
	  "layer":[{"mark":{"type":"test.stem","stem":4,"cap":"round"},
	            "encoding":{"x":{"field":"x"},"y":{"field":"y"}}}]}`
	s, err := spec.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Chart()
	if err != nil {
		t.Fatal(err)
	}
	g, ok := c.Layers[0].(*stemGeom)
	if !ok {
		t.Fatalf("read a %T, want the registered mark", c.Layers[0])
	}
	if g.cfg.Extra["stem"] != 4.0 || g.cfg.Extra["cap"] != "round" || g.cfg.X != "x" {
		t.Errorf("read %+v", g.cfg)
	}
}

// halved is a scale this module does not define, registered under its own kind.
type halved struct{ scale.Scale }

const kindHalved scale.Kind = "test.halved"

func (h halved) Describe() scale.Desc {
	d, _ := scale.Describe(h.Scale)
	d.Kind = kindHalved
	return d
}

func TestARegisteredScaleKindSurvivesTheRoundTrip(t *testing.T) {
	scale.Register(kindHalved, func(d scale.Desc) (scale.Scale, error) {
		d.Kind = scale.KindLinear
		s, err := scale.FromDesc(d)
		return halved{s}, err
	})
	in := spec.Chart{X: halved{scale.Linear(scale.Domain(0, 8))}, Y: scale.Linear()}
	out := roundTrip(t, in)
	if _, ok := out.X.(halved); !ok {
		t.Fatalf("x came back as %T", out.X)
	}
	if lo, hi := out.X.Domain(); lo != 0 || hi != 8 {
		t.Errorf("the fixed domain did not survive: [%v, %v]", lo, hi)
	}
}
