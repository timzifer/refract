package scale_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
)

// squared is a third-party scale: linear in the square root of the value, so
// that an axis of areas reads as lengths. It wraps a built-in scale and
// describes itself under its own kind.
type squared struct{ scale.Scale }

const kindSquared scale.Kind = "test.squared"

func (s squared) Describe() scale.Desc {
	d, _ := scale.Describe(s.Scale)
	d.Kind = kindSquared
	return d
}

func buildSquared(d scale.Desc) (scale.Scale, error) {
	d.Kind = scale.KindLinear
	inner, err := scale.FromDesc(d)
	if err != nil {
		return nil, err
	}
	return squared{inner}, nil
}

func TestARegisteredKindIsBuiltByFromDesc(t *testing.T) {
	scale.Register(kindSquared, buildSquared)
	want := squared{scale.Linear(scale.Domain(0, 16), scale.Nice())}
	d, ok := scale.Describe(want)
	if !ok || d.Kind != kindSquared {
		t.Fatalf("Describe = %+v, %v", d, ok)
	}
	got, err := scale.FromDesc(d)
	if err != nil {
		t.Fatalf("FromDesc: %v", err)
	}
	if _, ok := got.(squared); !ok {
		t.Fatalf("FromDesc built a %T", got)
	}
	back, _ := scale.Describe(got)
	if !reflect.DeepEqual(back, d) {
		t.Errorf("round trip changed the scale:\n got %+v\nwant %+v", back, d)
	}
}

func TestAnUnregisteredKindIsStillUnknown(t *testing.T) {
	_, err := scale.FromDesc(scale.Desc{Kind: "test.nobody-registered-this"})
	if !errors.Is(err, scale.ErrUnknownKind) {
		t.Errorf("err = %v, want ErrUnknownKind", err)
	}
	_, err = scale.ColorFromDesc(scale.ColorDesc{Kind: "test.nobody-registered-this"})
	if !errors.Is(err, scale.ErrUnknownKind) {
		t.Errorf("colour err = %v, want ErrUnknownKind", err)
	}
}

func TestRegisterRefusesABuiltInKind(t *testing.T) {
	mustPanic(t, "Register(linear)", func() { scale.Register(scale.KindLinear, buildSquared) })
	mustPanic(t, "Register(\"\")", func() { scale.Register("", buildSquared) })
	mustPanic(t, "RegisterColor(sequential)", func() {
		scale.RegisterColor(scale.KindSequential, func(scale.ColorDesc) (scale.ColorScale, error) { return nil, nil })
	})
}

// mono is a third-party colour scale: every value is one colour, which is the
// smallest thing that can still be described and rebuilt.
type mono struct{ c ir.Color }

const kindMono scale.ColorKind = "test.mono"

func (m mono) Train(...float64)               {}
func (m mono) Domain() (float64, float64)     { return 0, 1 }
func (m mono) Color(float64) ir.Color         { return m.c }
func (m mono) DescribeColor() scale.ColorDesc { return scale.ColorDesc{Kind: kindMono, Undefined: m.c} }

func TestARegisteredColorKindIsBuiltByColorFromDesc(t *testing.T) {
	scale.RegisterColor(kindMono, func(d scale.ColorDesc) (scale.ColorScale, error) {
		return mono{d.Undefined}, nil
	})
	want := mono{ir.RGB(1, 2, 3)}
	d, _ := scale.DescribeColor(want)
	got, err := scale.ColorFromDesc(d)
	if err != nil {
		t.Fatalf("ColorFromDesc: %v", err)
	}
	if got.Color(0.5) != want.c {
		t.Errorf("the rebuilt scale paints %v, want %v", got.Color(0.5), want.c)
	}
}

func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic", what)
		}
	}()
	f()
}
