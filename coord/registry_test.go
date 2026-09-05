package coord_test

import (
	"errors"
	"testing"

	"github.com/timzifer/refract/coord"
)

// mirrored is a third-party coord: Cartesian with the axes swapped, which is
// the smallest coordinate system that is not one of the two built in. It wraps
// the Cartesian one and describes itself under its own type.
type mirrored struct{ coord.Coord }

const typeMirrored coord.Type = "test.mirrored"

func (m mirrored) Describe() coord.Desc { return coord.Desc{Type: typeMirrored} }

func TestARegisteredTypeIsBuiltByFromDesc(t *testing.T) {
	coord.Register(typeMirrored, func(coord.Desc) (coord.Coord, error) {
		return mirrored{coord.Cartesian()}, nil
	})
	got, err := coord.FromDesc(coord.Desc{Type: typeMirrored})
	if err != nil {
		t.Fatalf("FromDesc: %v", err)
	}
	if _, ok := got.(mirrored); !ok {
		t.Errorf("FromDesc built a %T", got)
	}
}

func TestAnUnregisteredTypeIsStillUnknown(t *testing.T) {
	_, err := coord.FromDesc(coord.Desc{Type: "test.nobody-registered-this"})
	if !errors.Is(err, coord.ErrUnknownType) {
		t.Errorf("err = %v, want ErrUnknownType", err)
	}
}

func TestRegisterRefusesABuiltInType(t *testing.T) {
	for _, typ := range []coord.Type{coord.TypeCartesian, coord.TypePolar, ""} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Register(%q) did not panic", typ)
				}
			}()
			coord.Register(typ, func(coord.Desc) (coord.Coord, error) { return nil, nil })
		}()
	}
}
