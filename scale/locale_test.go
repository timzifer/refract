package scale_test

import (
	"strings"
	"testing"
	"time"

	"github.com/timzifer/refract/scale"
)

// The punctuation is the half of a locale that makes an unlocalised chart not
// merely foreign but wrong: "1.234" reads as one and a bit to a German reader.
func TestALocalePunctuatesANumber(t *testing.T) {
	cases := []struct {
		loc  *scale.Locale
		want string
	}{
		{scale.English, "1,234,567.50"},
		{scale.LocaleDE, "1.234.567,50"},
		{scale.LocaleFR, "1 234 567,50"},
		{scale.LocaleJA, "1,234,567.50"},
	}
	for _, c := range cases {
		t.Run(c.loc.Name, func(t *testing.T) {
			s := scale.Linear(scale.Domain(0, 1e7), scale.NumberFormat("#,.2"))
			scale.Localize(s, c.loc)
			if got := scale.LabelOf(s, 1234567.5); got != c.want {
				t.Errorf("label is %q, want %q", got, c.want)
			}
		})
	}
}

// The percent sign carries its own spacing, because in some languages it is a
// word of its own. One spec, two correct labels.
func TestThePercentSignIsTheLocaleOwn(t *testing.T) {
	de := scale.Linear(scale.Domain(0, 1), scale.NumberFormat("#.1%"))
	scale.Localize(de, scale.LocaleDE)
	if got := scale.LabelOf(de, 0.125); got != "12,5 %" {
		t.Errorf("German label is %q, want 12,5 %% with a no-break space", got)
	}
}

// A scale nobody localised writes exactly what it wrote before locales
// existed. That is what makes this addition invisible to every golden file in
// the repository, and it is worth a test rather than an assumption.
func TestAnUnlocalisedScaleIsUnchanged(t *testing.T) {
	plain := scale.Linear()
	plain.Train(0, 1)
	plain.SetRange(0, 400)
	for _, tk := range plain.Ticks(5) {
		if strings.ContainsAny(tk.Label, ",") {
			t.Errorf("label %q was grouped without being asked", tk.Label)
		}
	}
	// Two decimals because the axis's step over 0..1 is a quarter, which is
	// the axis's own choice and not the locale's business.
	if got := scale.LabelOf(plain, 0.5); got != "0.50" {
		t.Errorf("label is %q, want 0.50", got)
	}
}

// Go's time package carries its own English tables and has no hook past them,
// so the layout is split on the four tokens that name something and the rest
// is still Go's own.
func TestALocaleNamesTheMonths(t *testing.T) {
	at := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC) // a Monday
	cases := []struct {
		loc    *scale.Locale
		layout string
		span   time.Duration
		want   string
	}{
		{scale.LocaleDE, "January 2006", 20 * 24 * time.Hour, "März 2026"},
		{scale.LocaleDE, "Jan 2006", 20 * 24 * time.Hour, "Mär 2026"},
		{scale.LocaleDE, "Mon 2 Jan", 3 * 24 * time.Hour, "Mo 9 Mär"},
		{scale.LocaleFR, "January 2006", 20 * 24 * time.Hour, "mars 2026"},
		{scale.LocaleJA, "January 2006", 20 * 24 * time.Hour, "3月 2026"},
		{scale.English, "January 2006", 20 * 24 * time.Hour, "March 2026"},
		{scale.LocaleDE, "2006-01-02 15:04", 3 * 24 * time.Hour, "2026-03-09 00:00"},
	}
	for _, c := range cases {
		t.Run(c.loc.Name+"/"+c.layout, func(t *testing.T) {
			s := scale.Time(scale.In(time.UTC), scale.TimeLayout(c.layout))
			scale.Localize(s, c.loc)
			s.Train(scale.ValueOf(s, at), scale.ValueOf(s, at.Add(c.span)))
			s.SetRange(0, 400)
			var got []string
			for _, tk := range s.Ticks(4) {
				got = append(got, tk.Label)
				if tk.Label == c.want {
					return
				}
			}
			t.Errorf("labels are %q, want one reading %q", got, c.want)
		})
	}
}

// A locale with an empty table costs English names rather than blank ones: a
// caller may care about the decimal separator and nothing else.
func TestAHalfFilledLocaleFallsBackToEnglishNames(t *testing.T) {
	half := &scale.Locale{Name: "xx", Decimal: ",", Group: " ", Minus: "-", Percent: "%"}
	s := scale.Time(scale.TimeLayout("January"))
	scale.Localize(s, half)
	at := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	s.Train(scale.ValueOf(s, at))
	s.SetRange(0, 400)
	for _, tk := range s.Ticks(2) {
		if tk.Label == "" {
			t.Fatal("a month with no name in the locale came out blank")
		}
	}
}

// An ordinal scale deliberately is not a Localizer: its labels are the
// caller's own categories, and translating those would be inventing data.
func TestAnOrdinalScaleIsNotLocalised(t *testing.T) {
	if scale.Localize(scale.Ordinal(), scale.LocaleDE) {
		t.Error("an ordinal scale accepted a locale; its labels are the caller's own categories")
	}
}

// A region falls back to its language, because a chart asking for Austrian
// German is better served by German than by English.
func TestARegionFallsBackToItsLanguage(t *testing.T) {
	l, ok := scale.LookupLocale("de-AT")
	if !ok || l != scale.LocaleDE {
		t.Errorf("de-AT resolved to %v, want the German locale", l)
	}
	if _, ok := scale.LookupLocale("zz"); ok {
		t.Error("an unregistered language resolved to something")
	}
}

// A locale registered by a caller is findable by name, which is what lets a
// document carry a language refract does not ship.
func TestARegisteredLocaleIsFoundByName(t *testing.T) {
	mine := &scale.Locale{Name: "test-xx", Decimal: "·", Group: "_", Minus: "−", Percent: "%"}
	scale.RegisterLocale(mine)
	got, ok := scale.LookupLocale("test-xx")
	if !ok || got != mine {
		t.Fatalf("lookup gave %v, want the registered locale", got)
	}
	s, err := scale.FromDesc(scale.Desc{Kind: scale.KindLinear, Format: "#,.1", Locale: "test-xx", Fixed: true, Min: 0, Max: 1e4})
	if err != nil {
		t.Fatal(err)
	}
	if want := "1_234·5"; scale.LabelOf(s, 1234.5) != want {
		t.Errorf("label is %q, want %q", scale.LabelOf(s, 1234.5), want)
	}
}

// A name nothing registered draws in English and is written back out
// unchanged: a document does not lose what it asked for by passing through a
// process that cannot honour it.
func TestAnUnknownLocaleNameSurvivesTheRoundTrip(t *testing.T) {
	s, err := scale.FromDesc(scale.Desc{Kind: scale.KindLinear, Format: "#,", Locale: "xx-YY"})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := scale.Describe(s)
	if !ok {
		t.Fatal("the scale cannot describe itself")
	}
	if d.Locale != "xx-YY" {
		t.Errorf("the locale name came back as %q, want it kept", d.Locale)
	}
}

// The Desc is complete: every declarative choice comes back.
func TestTheFormatAndLocaleRoundTripThroughADesc(t *testing.T) {
	for _, d := range []scale.Desc{
		{Kind: scale.KindLinear, Format: "€ #,.2", Locale: "de"},
		{Kind: scale.KindLog, Format: "#k", Locale: "fr", MinorTicks: true},
		{Kind: scale.KindSymLog, Format: "#.1", Locale: "de", MinorTicks: true},
		{Kind: scale.KindTime, Layout: "2006-01-02", Locale: "ja"},
	} {
		t.Run(string(d.Kind), func(t *testing.T) {
			s, err := scale.FromDesc(d)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := scale.Describe(s)
			if !ok {
				t.Fatal("the scale cannot describe itself")
			}
			if got.Format != d.Format || got.Layout != d.Layout || got.Locale != d.Locale {
				t.Errorf("round trip gave format %q layout %q locale %q, want %q %q %q",
					got.Format, got.Layout, got.Locale, d.Format, d.Layout, d.Locale)
			}
		})
	}
}
