package scale_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/timzifer/refract/scale"
)

// labelsOf reads the tick labels of a scale over a domain.
func labelsOf(t *testing.T, s scale.Scale, lo, hi float64, want int) []string {
	t.Helper()
	s.Train(lo, hi)
	s.SetRange(0, 400)
	var out []string
	for _, tk := range s.Ticks(want) {
		if tk.Label != "" {
			out = append(out, tk.Label)
		}
	}
	return out
}

// labelAt is the label a scale gives one value, found among its ticks.
func labelAt(t *testing.T, s scale.Scale, v float64) string {
	t.Helper()
	s.SetRange(0, 400)
	for _, tk := range s.Ticks(11) {
		if tk.Value == v {
			return tk.Label
		}
	}
	t.Fatalf("no tick at %v among %v", v, s.Ticks(11))
	return ""
}

// The grammar, one case per feature. A spec is a string because a string is
// what survives the round trip into a document and what a person editing a
// configuration file can type.
func TestTheNumberFormatGrammar(t *testing.T) {
	cases := []struct {
		spec   string
		v      float64
		want   string
		lo, hi float64 // the domain, which is where the axis's own precision comes from
	}{
		{spec: "#", v: 1234567, want: "1234567", hi: 1e7},
		{spec: "#,", v: 1234567, want: "1,234,567", hi: 1e7},
		{spec: "#,", v: 123, want: "123", hi: 1e7},
		{spec: "#,", v: 1234, want: "1,234", hi: 1e7},
		{spec: "#,.2", v: 1234567, want: "1,234,567.00", hi: 1e7},
		{spec: "#.2", v: 1.5, want: "1.50", hi: 10},
		{spec: "#.0", v: 1.5, want: "2", hi: 10},
		{spec: "€ #,.2", v: 1234.5, want: "€ 1,234.50", hi: 1e7},
		{spec: "# kg", v: 1234, want: "1234 kg", hi: 1e7},
		{spec: "#,# kg", v: 1234, want: "1,234# kg", hi: 1e7}, // only the first # is the number
		{spec: "#.1%", v: 0.125, want: "12.5%", hi: 1},
		{spec: "#%", v: 0.5, want: "50%", hi: 1},
		{spec: "#k", v: 1234, want: "1.23k", hi: 1e7},
		{spec: "#k", v: 12345, want: "12.3k", hi: 1e7},
		{spec: "#k", v: 123456, want: "123k", hi: 1e7},
		{spec: "#k", v: 1234567, want: "1.23M", hi: 1e7},
		{spec: "#k", v: 0.0012, want: "1.2m", hi: 1e7},
		{spec: "#k", v: 0, want: "0", hi: 1e7},
		{spec: "#.1k", v: 1234, want: "1.2k", hi: 1e7},
		{spec: "#kg", v: 1234, want: "1.23kg", hi: 1e7}, // the style letter, then a suffix
		{spec: "#e", v: 1234, want: "1.23e+03", hi: 1e7},
		{spec: "#.1e", v: 1234, want: "1.2e+03", hi: 1e7},
		{spec: "#,", v: -1234567, want: "-1,234,567", lo: -1e7, hi: 1e7},
	}
	for _, c := range cases {
		t.Run(c.spec+"/"+strings.ReplaceAll(c.want, "/", "_"), func(t *testing.T) {
			s := scale.Linear(scale.Domain(c.lo, c.hi), scale.NumberFormat(c.spec))
			got := scale.LabelOf(s, c.v)
			if got != c.want {
				t.Errorf("%q of %v is %q, want %q", c.spec, c.v, got, c.want)
			}
		})
	}
}

// A style letter counts as one only where the body ends, so "#k" is SI and
// "# kg" is a number with a unit after it. It is the one place the grammar can
// surprise, so it is pinned from both sides.
func TestAStyleLetterIsOnlyOneWhereTheBodyEnds(t *testing.T) {
	si := scale.Linear(scale.Domain(0, 1e4), scale.NumberFormat("#k"))
	unit := scale.Linear(scale.Domain(0, 1e4), scale.NumberFormat("# kg"))
	if got := scale.LabelOf(si, 1500); got != "1.5k" {
		t.Errorf("#k of 1500 is %q, want 1.5k", got)
	}
	if got := scale.LabelOf(unit, 1500); got != "1500 kg" {
		t.Errorf("\"# kg\" of 1500 is %q, want 1500 kg", got)
	}
}

// A spec that names no precision keeps the axis's own, which is derived from
// the tick step rather than from any one value. That is what keeps a column of
// labels aligned, and a prefix must not cost it.
func TestASpecWithoutAPrecisionKeepsTheAxisOwn(t *testing.T) {
	plain := scale.Linear()
	prefixed := scale.Linear(scale.NumberFormat("$#"))

	want := labelsOf(t, plain, 0, 1, 5)
	got := labelsOf(t, prefixed, 0, 1, 5)
	if len(got) != len(want) {
		t.Fatalf("got %v, want one label per tick of %v", got, want)
	}
	for i := range want {
		if got[i] != "$"+want[i] {
			t.Errorf("label %d is %q, want %q", i, got[i], "$"+want[i])
		}
	}
}

// A spec written in Go source is a literal, so a malformed one is a
// programming error — the same line data.Table draws about a ragged table.
func TestAMalformedSpecInGoSourcePanics(t *testing.T) {
	for _, spec := range []string{"1234", "#.", "#.99"} {
		t.Run(spec, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("%q did not panic", spec)
				}
			}()
			scale.Linear(scale.NumberFormat(spec))
		})
	}
}

// A spec read out of a document is input, so it is an error rather than a
// panic. The two paths are the same parser.
func TestAMalformedSpecInADocumentIsAnError(t *testing.T) {
	_, err := scale.FromDesc(scale.Desc{Kind: scale.KindLinear, Format: "no hash here"})
	if err == nil {
		t.Fatal("a malformed format in a Desc was accepted")
	}
	if !errors.Is(err, scale.ErrFormat) {
		t.Errorf("error is %v, want one that is ErrFormat", err)
	}
}

// A Go formatter outranks a spec. A caller who wrote one has answered the
// question, and a spec is what a document can carry rather than a better
// answer.
func TestAGoFormatterOutranksASpec(t *testing.T) {
	s := scale.Linear(
		scale.NumberFormat("#,.2"),
		scale.Format(func(v float64) string { return "x" }),
	)
	if got := scale.LabelOf(s, 1234); got != "x" {
		t.Errorf("label is %q, want the Go formatter's answer", got)
	}
	d, _ := scale.Describe(s)
	if !d.Formatted {
		t.Error("the Desc does not report the Go formatter")
	}
	if d.Format != "#,.2" {
		t.Errorf("the Desc dropped the spec (%q); a document that lost it would change the chart the day the Go code went away", d.Format)
	}
}

// The spec travels on a log and a symlog axis too, where the axis's own
// precision is the decade's rather than a step's.
func TestASpecOnALogAxis(t *testing.T) {
	s := scale.Log(scale.LogNumberFormat("#,"))
	if got := labelAt(t, trained(s, 1, 100000), 100000); got != "100,000" {
		t.Errorf("log label is %q, want 100,000", got)
	}
	sym := scale.SymLog(scale.SymLogNumberFormat("# u"))
	if got := labelAt(t, trained(sym, -1000, 1000), 1000); got != "1000 u" {
		t.Errorf("symlog label is %q, want 1000 u", got)
	}
}

// A time scale's declarative format is a layout, and it is what the ladder
// would otherwise have chosen per tick spacing.
func TestATimeLayoutFixesEveryLabel(t *testing.T) {
	s := scale.Time(scale.TimeLayout("2006-01-02"))
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	s.Train(scale.ValueOf(s, from), scale.ValueOf(s, from.AddDate(0, 0, 20)))
	s.SetRange(0, 400)
	for _, tk := range s.Ticks(5) {
		if len(tk.Label) != len("2006-01-02") {
			t.Errorf("label %q is not the layout that was asked for", tk.Label)
		}
	}
}
