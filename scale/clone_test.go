package scale_test

import (
	"testing"
	"time"

	"github.com/timzifer/refract/scale"
)

// Every built-in scale has to be cloneable, or a free facet axis silently
// falls back to a shared one.
func TestEveryBuiltInScaleClones(t *testing.T) {
	for _, s := range []scale.Scale{
		scale.Linear(scale.Nice()),
		scale.Log(),
		scale.SymLog(),
		scale.Time(),
		scale.Ordinal(),
	} {
		c, ok := s.(scale.Cloner)
		if !ok {
			t.Errorf("%T does not implement Cloner", s)
			continue
		}
		if c.Clone() == nil {
			t.Errorf("%T cloned to nil", s)
		}
	}
}

func TestCloneCarriesNoTraining(t *testing.T) {
	s := scale.Linear()
	s.Train(0, 100)
	c := s.(scale.Cloner).Clone()
	c.Train(5, 6)
	if lo, hi := c.Domain(); lo != 5 || hi != 6 {
		t.Errorf("clone domain = %v..%v, want 5..6", lo, hi)
	}
	if lo, hi := s.Domain(); lo != 0 || hi != 100 {
		t.Errorf("the original's domain moved to %v..%v", lo, hi)
	}
}

// A fixed domain is configuration, not training: it survives the copy.
func TestCloneKeepsAFixedDomain(t *testing.T) {
	for _, s := range []scale.Scale{
		scale.Linear(scale.Domain(2, 8)),
		scale.Log(scale.LogDomain(1, 1000)),
		scale.SymLog(scale.SymLogDomain(-10, 10)),
	} {
		wantLo, wantHi := s.Domain()
		c := s.(scale.Cloner).Clone()
		c.Train(1e6)
		if lo, hi := c.Domain(); lo != wantLo || hi != wantHi {
			t.Errorf("%T: clone domain = %v..%v, want %v..%v", s, lo, hi, wantLo, wantHi)
		}
	}
}

func TestCloneOfAnOrdinalForgetsLearnedCategories(t *testing.T) {
	o := scale.Ordinal()
	cat := o.(scale.Categorical)
	cat.Encode("a")
	cat.Encode("b")

	c := o.(scale.Cloner).Clone()
	cc := c.(scale.Categorical)
	if got := cc.Encode("z"); got != 0 {
		t.Errorf("the first category of a clone encodes to %v, want 0", got)
	}
	if got := len(cat.Labels()); got != 2 {
		t.Errorf("the original now has %d categories, want 2", got)
	}
}

func TestCloneOfAFixedOrdinalKeepsItsCategories(t *testing.T) {
	o := scale.Ordinal(scale.Categories("a", "b", "c"))
	c := o.(scale.Cloner).Clone().(scale.Categorical)
	if got := c.Labels(); len(got) != 3 || got[0] != "a" {
		t.Errorf("clone labels = %v, want [a b c]", got)
	}
	c.Encode("z")
	if got := len(o.(scale.Categorical).Labels()); got != 3 {
		t.Errorf("encoding into the clone changed the original: %d labels", got)
	}
}

func TestCloneOfATimeScaleKeepsItsLocation(t *testing.T) {
	loc := time.FixedZone("test", 3600)
	s := scale.Time(scale.In(loc))
	c := s.(scale.Cloner).Clone()
	c.Train(scale.Nanos(time.Unix(0, 0)), scale.Nanos(time.Unix(86400, 0)))
	if len(c.Ticks(4)) == 0 {
		t.Error("a cloned time scale produced no ticks")
	}
}
