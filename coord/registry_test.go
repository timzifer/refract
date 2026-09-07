package coord_test

import (
	"errors"
	"testing"

	"github.com/timzifer/refract/coord"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/scale"
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
	for _, typ := range []coord.Type{coord.TypeCartesian, coord.TypePolar, coord.TypeSmith, ""} {
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

// Only a coord with edges opposite its axes can place a second one. Cartesian
// has them; the other two do not, and a chart that names a second axis under
// either simply does not draw it — which is the answer, not an omission.
func TestOnlyCartesianPlacesAnOppositeAxis(t *testing.T) {
	cases := []struct {
		name string
		cd   coord.Coord
		want bool
	}{
		{"cartesian", coord.Cartesian(), true},
		{"polar", coord.Polar(), false},
		{"smith", coord.Smith(), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, got := c.cd.(coord.Opposite)
			if got != c.want {
				t.Errorf("%T implements Opposite = %v, want %v", c.cd, got, c.want)
			}
			// And the framed value a panel actually holds answers the same
			// way: Frame hands back a coord positioned in the panel, and one
			// that lost the interface there would draw a second axis in an
			// unframed test and none in a chart.
			framed := c.cd.Frame(ir.R(0, 0, 100, 100), scale.Linear(), scale.Linear())
			if _, got := framed.(coord.Opposite); got != c.want {
				t.Errorf("framed %T implements Opposite = %v, want %v", framed, got, c.want)
			}
		})
	}
}
