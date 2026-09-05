package geom_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/timzifer/refract/data"
	"github.com/timzifer/refract/geom"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/palette"
	"github.com/timzifer/refract/scale"
)

// lollipop is the third-party mark every extension test is written against: a
// geom outside the package that takes the shared options, reads them through
// Configure, adds one knob of its own through Extra, and describes itself so
// that the JSON spec can carry it.
type lollipop struct {
	src data.Source
	cfg geom.Desc
}

const markLollipop geom.Mark = "test.lollipop"

func newLollipop(src data.Source, opts ...geom.Option) geom.Geom {
	return &lollipop{src: src, cfg: geom.Configure(opts...)}
}

func (l *lollipop) Train(x, y scale.Scale) error {
	xs, ok := l.src.Float64Column(l.cfg.X)
	if !ok {
		return geom.ErrNoColumn
	}
	ys, ok := l.src.Float64Column(l.cfg.Y)
	if !ok {
		return geom.ErrNoColumn
	}
	x.Train(xs...)
	y.Train(ys...)
	return nil
}

func (l *lollipop) Build(b ir.Backend, f geom.Frame) error {
	xs, _ := l.src.Float64Column(l.cfg.X)
	ys, _ := l.src.Float64Column(l.cfg.Y)
	col := f.Theme.Palette.At(f.Index)
	if l.cfg.Color != nil {
		col = *l.cfg.Color
	}
	stem := float32(1)
	if w, ok := l.cfg.Extra["stem"].(float64); ok {
		stem = float32(w)
	}
	c := f.Coords()
	for i := range xs {
		top := c.Point(f.X.Map(xs[i]), f.Y.Map(ys[i]))
		base := c.Point(f.X.Map(xs[i]), f.Y.Map(0))
		b.Polyline([]ir.Point{base, top}, ir.Stroke{Color: col, Width: stem})
		b.Markers(ir.MarkerCircle, []ir.Point{top}, ir.MarkerStyle{Size: 6, Fill: col})
	}
	return nil
}

func (l *lollipop) Legend(geom.Frame) (geom.LegendEntry, bool) {
	return geom.LegendEntry{Label: l.cfg.Label, Kind: geom.SwatchMarker}, l.cfg.Label != ""
}

func (l *lollipop) Describe() geom.Desc {
	d := l.cfg
	d.Mark, d.Source = markLollipop, l.src
	return d
}

func buildLollipop(d geom.Desc) (geom.Geom, error) {
	if d.Source == nil {
		return nil, errors.New("a lollipop layer needs a data source")
	}
	return newLollipop(d.Source, d.Options()...), nil
}

func lollipopSource() data.Source {
	return data.Float64Columns(map[string][]float64{"x": {0, 1, 2}, "y": {3, 1, 2}})
}

func TestConfigureReportsWhatTheOptionsSet(t *testing.T) {
	d := geom.Configure(geom.X("x"), geom.Y("y"), geom.Color(palette.Blue), geom.Label("stems"),
		geom.OnMissing(geom.Interpolate), geom.Width(2.5), geom.Extra("stem", 3.0))
	if d.X != "x" || d.Y != "y" || d.Label != "stems" {
		t.Errorf("columns and label were not read: %+v", d)
	}
	if d.Color == nil || *d.Color != palette.Blue {
		t.Errorf("Color = %v, want blue", d.Color)
	}
	if d.Missing != geom.Interpolate || d.Width != 2.5 {
		t.Errorf("Missing = %v, Width = %v", d.Missing, d.Width)
	}
	if got := d.Extra["stem"]; got != 3.0 {
		t.Errorf("Extra[stem] = %v, want 3", got)
	}
	if d.Mark != "" || d.Source != nil {
		t.Errorf("Configure decided a mark or a source: %q %v", d.Mark, d.Source)
	}
}

// TestConfigureStartsFromTheBuiltInDefaults pins the promise that a third-party
// mark and a built-in one read the same option the same way: what Configure
// reports with no options is what Line reports, less the mark and the source.
func TestConfigureStartsFromTheBuiltInDefaults(t *testing.T) {
	want, _ := geom.Describe(geom.Line(lollipopSource()))
	want.Mark, want.Source = "", nil
	if got := geom.Configure(); !reflect.DeepEqual(got, want) {
		t.Errorf("Configure() = %+v\nwant the built-in defaults %+v", got, want)
	}
}

func TestConfigureAndOptionsAreInverses(t *testing.T) {
	d := geom.Configure(geom.X("x"), geom.Y("y"), geom.Dash(2, 1), geom.Shape(ir.MarkerPlus),
		geom.Stack(geom.StackFill), geom.Dodge(0.1), geom.Extra("stem", 3.0), geom.Extra("cap", "round"))
	again := geom.Configure(d.Options()...)
	if !reflect.DeepEqual(d, again) {
		t.Errorf("Options lost something:\n before %+v\n  after %+v", d, again)
	}
}

func TestExtraIsIgnoredByTheBuiltInMarks(t *testing.T) {
	src := lollipopSource()
	plain, _ := geom.Describe(geom.Line(src, geom.X("x"), geom.Y("y")))
	with, _ := geom.Describe(geom.Line(src, geom.X("x"), geom.Y("y"), geom.Extra("stem", 3.0)))
	if with.Extra["stem"] != 3.0 {
		t.Fatal("a built-in mark dropped an Extra it should carry through")
	}
	with.Extra = nil
	if !reflect.DeepEqual(plain, with) {
		t.Errorf("an Extra changed a built-in mark's configuration:\n %+v\n %+v", plain, with)
	}
}

func TestARegisteredMarkIsBuiltByFromDesc(t *testing.T) {
	geom.Register(markLollipop, buildLollipop)
	src := lollipopSource()
	g := newLollipop(src, geom.X("x"), geom.Y("y"), geom.Label("stems"), geom.Extra("stem", 3.0))

	d, ok := geom.Describe(g)
	if !ok {
		t.Fatal("the lollipop does not describe itself")
	}
	again, err := geom.FromDesc(d)
	if err != nil {
		t.Fatalf("FromDesc: %v", err)
	}
	back, _ := geom.Describe(again)
	if !reflect.DeepEqual(d, back) {
		t.Errorf("the round trip changed the layer:\n before %+v\n  after %+v", d, back)
	}
	if _, ok := again.(*lollipop); !ok {
		t.Errorf("FromDesc built a %T, want a lollipop", again)
	}
}

func TestARegisteredMarkDecidesAboutItsOwnSource(t *testing.T) {
	geom.Register(markLollipop, buildLollipop)
	_, err := geom.FromDesc(geom.Desc{Mark: markLollipop})
	if err == nil || errors.Is(err, geom.ErrUnknownMark) {
		t.Errorf("err = %v, want the builder's own error", err)
	}
}

func TestAnUnregisteredMarkIsStillUnknown(t *testing.T) {
	_, err := geom.FromDesc(geom.Desc{Mark: "test.nobody-registered-this", Source: lollipopSource()})
	if !errors.Is(err, geom.ErrUnknownMark) {
		t.Errorf("err = %v, want ErrUnknownMark", err)
	}
}

func TestRegisterRefusesABuiltInMark(t *testing.T) {
	for _, m := range []geom.Mark{geom.MarkLine, geom.MarkNote, ""} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Register(%q) did not panic", m)
				}
			}()
			geom.Register(m, buildLollipop)
		}()
	}
}

func TestARegisteredMarkDrawsThroughTheFrame(t *testing.T) {
	src := lollipopSource()
	g := newLollipop(src, geom.X("x"), geom.Y("y"), geom.Extra("stem", 3.0))
	x, y := scale.Linear(), scale.Linear()
	if err := g.Train(x, y); err != nil {
		t.Fatal(err)
	}
	x.SetRange(0, 100)
	y.SetRange(100, 0)
	rec := ir.NewRecorder(nil)
	f := geom.Frame{Area: ir.R(0, 0, 100, 100), X: x, Y: y}
	if err := g.Build(rec, f); err != nil {
		t.Fatal(err)
	}
	if rec.Calls() != 6 {
		t.Errorf("drew %d calls, want a stem and a head per row", rec.Calls())
	}
}
