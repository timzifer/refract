package scale

import (
	"strings"
	"sync"
	"time"
)

// Locale is what a tick label needs of a language: how a number is punctuated
// and what the months and days are called.
//
// It is deliberately small. A chart axis writes numbers, month names and
// weekday names, and nothing else — there is no plural rule, no collation and
// no currency table here, because an axis label never asks any of those
// questions. What it does ask, refract answered in English until now:
// `strconv` writes a decimal point, and the tick layouts in [Time] spell the
// months "Jan". Both are a choice, and neither was one a caller could make.
//
// A Locale is read, never written, once it is registered — a chart may be
// rendered from several goroutines — so build one fully and hand it over.
type Locale struct {
	// Name is the tag this locale is registered and written down under. It is
	// what travels in a JSON document, so a chart written down in one process
	// and read in another reaches the same locale by looking it up rather than
	// by carrying its tables.
	Name string

	// Decimal separates the whole part from the fraction: "." in English, ","
	// in most of Europe.
	Decimal string
	// Group separates thousands, and is used only by a format that asks for
	// grouping — see [NumberFormat]. It is "," in English, "." in German and a
	// narrow no-break space in French.
	Group string
	// Minus is written before a negative number. It is the ASCII hyphen here
	// and in every locale that ships with refract; a caller who wants the
	// typographic minus (U+2212) sets it, and gets it on every axis at once.
	Minus string
	// Percent is appended by the "%" style, and carries its own spacing: "%"
	// in English, " %" — a no-break space and a sign — in French and
	// German, where the sign is a word of its own.
	Percent string

	// Months and ShortMonths are January first. A layout naming "January" or
	// "Jan" is filled from these rather than from Go's own table.
	Months      [12]string
	ShortMonths [12]string
	// Days and ShortDays are Sunday first, which is [time.Weekday]'s order —
	// the order the code indexes them by, not a claim about which day a week
	// starts on.
	Days      [7]string
	ShortDays [7]string
}

// English is the locale every scale uses until it is given another. Its tables
// are Go's own, so a chart that never mentions a locale draws exactly what it
// drew before locales existed — which is what makes this addition invisible to
// every existing golden file.
var English = &Locale{
	Name:    "en",
	Decimal: ".",
	Group:   ",",
	Minus:   "-",
	Percent: "%",
	Months: [12]string{"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December"},
	ShortMonths: [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	Days: [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday",
		"Friday", "Saturday"},
	ShortDays: [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
}

// German, French, Spanish, Italian, Dutch, Portuguese and Japanese ship
// because they are the ones refract could get right, not because they are the
// ones the world needs. A locale is eight strings and four tables; anything
// else is three lines and a [RegisterLocale] call, and that is the supported
// way to add one rather than a workaround. Shipping a table refract cannot
// check is worse than shipping none: a chart with a misspelled month is a
// chart nobody can trust the rest of.
var (
	// LocaleDE is German. The group separator is a full stop and the decimal
	// separator a comma, which is the pair that makes an unlocalised German
	// chart not merely foreign but wrong: "1.234" reads as one and a bit.
	LocaleDE = &Locale{
		Name: "de", Decimal: ",", Group: ".", Minus: "-", Percent: " %",
		Months: [12]string{"Januar", "Februar", "März", "April", "Mai", "Juni",
			"Juli", "August", "September", "Oktober", "November", "Dezember"},
		ShortMonths: [12]string{"Jan", "Feb", "Mär", "Apr", "Mai", "Jun",
			"Jul", "Aug", "Sep", "Okt", "Nov", "Dez"},
		Days: [7]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag",
			"Freitag", "Samstag"},
		ShortDays: [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"},
	}

	// LocaleFR is French. Its group separator is the narrow no-break space
	// U+202F, which is what typesetting practice and CLDR both call for; a
	// plain space would let a label break across two lines.
	LocaleFR = &Locale{
		Name: "fr", Decimal: ",", Group: " ", Minus: "-", Percent: " %",
		Months: [12]string{"janvier", "février", "mars", "avril", "mai", "juin",
			"juillet", "août", "septembre", "octobre", "novembre", "décembre"},
		ShortMonths: [12]string{"janv.", "févr.", "mars", "avr.", "mai", "juin",
			"juil.", "août", "sept.", "oct.", "nov.", "déc."},
		Days: [7]string{"dimanche", "lundi", "mardi", "mercredi", "jeudi",
			"vendredi", "samedi"},
		ShortDays: [7]string{"dim.", "lun.", "mar.", "mer.", "jeu.", "ven.", "sam."},
	}

	// LocaleES is Spanish.
	LocaleES = &Locale{
		Name: "es", Decimal: ",", Group: ".", Minus: "-", Percent: " %",
		Months: [12]string{"enero", "febrero", "marzo", "abril", "mayo", "junio",
			"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
		ShortMonths: [12]string{"ene", "feb", "mar", "abr", "may", "jun",
			"jul", "ago", "sep", "oct", "nov", "dic"},
		Days: [7]string{"domingo", "lunes", "martes", "miércoles", "jueves",
			"viernes", "sábado"},
		ShortDays: [7]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"},
	}

	// LocaleIT is Italian.
	LocaleIT = &Locale{
		Name: "it", Decimal: ",", Group: ".", Minus: "-", Percent: "%",
		Months: [12]string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno",
			"luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"},
		ShortMonths: [12]string{"gen", "feb", "mar", "apr", "mag", "giu",
			"lug", "ago", "set", "ott", "nov", "dic"},
		Days: [7]string{"domenica", "lunedì", "martedì", "mercoledì", "giovedì",
			"venerdì", "sabato"},
		ShortDays: [7]string{"dom", "lun", "mar", "mer", "gio", "ven", "sab"},
	}

	// LocaleNL is Dutch.
	LocaleNL = &Locale{
		Name: "nl", Decimal: ",", Group: ".", Minus: "-", Percent: "%",
		Months: [12]string{"januari", "februari", "maart", "april", "mei", "juni",
			"juli", "augustus", "september", "oktober", "november", "december"},
		ShortMonths: [12]string{"jan", "feb", "mrt", "apr", "mei", "jun",
			"jul", "aug", "sep", "okt", "nov", "dec"},
		Days: [7]string{"zondag", "maandag", "dinsdag", "woensdag", "donderdag",
			"vrijdag", "zaterdag"},
		ShortDays: [7]string{"zo", "ma", "di", "wo", "do", "vr", "za"},
	}

	// LocalePT is Portuguese.
	LocalePT = &Locale{
		Name: "pt", Decimal: ",", Group: ".", Minus: "-", Percent: "%",
		Months: [12]string{"janeiro", "fevereiro", "março", "abril", "maio", "junho",
			"julho", "agosto", "setembro", "outubro", "novembro", "dezembro"},
		ShortMonths: [12]string{"jan", "fev", "mar", "abr", "mai", "jun",
			"jul", "ago", "set", "out", "nov", "dez"},
		Days: [7]string{"domingo", "segunda-feira", "terça-feira", "quarta-feira",
			"quinta-feira", "sexta-feira", "sábado"},
		ShortDays: [7]string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"},
	}

	// LocaleJA is Japanese. Its months are numbered rather than named, which
	// is why the tables are strings rather than an index into anything.
	LocaleJA = &Locale{
		Name: "ja", Decimal: ".", Group: ",", Minus: "-", Percent: "%",
		Months: [12]string{"1月", "2月", "3月", "4月", "5月", "6月",
			"7月", "8月", "9月", "10月", "11月", "12月"},
		ShortMonths: [12]string{"1月", "2月", "3月", "4月", "5月", "6月",
			"7月", "8月", "9月", "10月", "11月", "12月"},
		Days: [7]string{"日曜日", "月曜日", "火曜日", "水曜日", "木曜日",
			"金曜日", "土曜日"},
		ShortDays: [7]string{"日", "月", "火", "水", "木", "金", "土"},
	}
)

// RegisterLocale makes a locale findable by name, so that a chart written down
// as JSON reads back with the labels it was written with.
//
// It is the same bargain [Register] makes for a third party's scale kind: the
// document carries a name, the process carries the tables, and a name nothing
// registered reads back as the default rather than as an error — a chart in
// the wrong language is a chart, and refusing to draw it would be worse.
// Registering a name twice replaces the earlier locale.
func RegisterLocale(l *Locale) {
	if l == nil || l.Name == "" {
		panic("refract/scale: RegisterLocale: a locale needs a name")
	}
	localeMu.Lock()
	defer localeMu.Unlock()
	locales[l.Name] = l
}

// LookupLocale returns the locale registered under a name.
//
// A name with a region — "de-AT", "pt-BR" — falls back to its language when
// the region itself is not registered, because a chart asking for Austrian
// German is better served by German than by English. Matching is on the tag as
// written, and the tags refract ships are lower-case language codes.
func LookupLocale(name string) (*Locale, bool) {
	if name == "" {
		return nil, false
	}
	localeMu.RLock()
	defer localeMu.RUnlock()
	if l, ok := locales[name]; ok {
		return l, true
	}
	if i := strings.IndexAny(name, "-_"); i > 0 {
		l, ok := locales[name[:i]]
		return l, ok
	}
	return nil, false
}

var (
	localeMu sync.RWMutex
	locales  = map[string]*Locale{
		English.Name:  English,
		LocaleDE.Name: LocaleDE,
		LocaleFR.Name: LocaleFR,
		LocaleES.Name: LocaleES,
		LocaleIT.Name: LocaleIT,
		LocaleNL.Name: LocaleNL,
		LocalePT.Name: LocalePT,
		LocaleJA.Name: LocaleJA,
	}
)

// Localizer is implemented by a scale whose labels can be written in a
// language. It is an optional interface, for the reason [Zoomer] is: [Scale]
// is implemented outside this package and never gains a method.
//
// A scale that does not implement it writes English, which is what every scale
// did before this existed. An ordinal scale deliberately does not: its labels
// are the caller's own categories, and translating those would be inventing
// data.
type Localizer interface {
	// SetLocale sets the language this scale's tick labels are written in. A
	// nil locale means [English].
	SetLocale(l *Locale)
}

// Localize sets a scale's locale if it has one, reporting whether it did.
//
// It is how a chart gives every one of its axes the same language in one
// place: [github.com/timzifer/refract.Locale] walks the scales a plot holds
// and calls this. Doing it per scale at construction works too and is what a
// scale used on its own does.
func Localize(s Scale, l *Locale) bool {
	loc, ok := s.(Localizer)
	if !ok {
		return false
	}
	loc.SetLocale(l)
	return true
}

// localeOr is the locale a scale should use, defaulting to English.
func localeOr(l *Locale) *Locale {
	if l == nil {
		return English
	}
	return l
}

// localeName is what a Desc carries: the empty string for a scale left in
// English, so that a scale nobody localised writes no locale into a document.
func localeName(l *Locale) string {
	if l == nil || l == English {
		return ""
	}
	return l.Name
}

// monthName and dayName read the tables, tolerating a locale whose tables were
// left empty — a caller may care about the decimal separator and nothing else,
// and a half-filled locale should cost them month names in English rather than
// month names that are blank.
func (l *Locale) monthName(m time.Month, short bool) string {
	i := int(m) - 1
	if i < 0 || i > 11 {
		return ""
	}
	if short {
		if l.ShortMonths[i] != "" {
			return l.ShortMonths[i]
		}
		return English.ShortMonths[i]
	}
	if l.Months[i] != "" {
		return l.Months[i]
	}
	return English.Months[i]
}

func (l *Locale) dayName(d time.Weekday, short bool) string {
	i := int(d)
	if i < 0 || i > 6 {
		return ""
	}
	if short {
		if l.ShortDays[i] != "" {
			return l.ShortDays[i]
		}
		return English.ShortDays[i]
	}
	if l.Days[i] != "" {
		return l.Days[i]
	}
	return English.Days[i]
}

// localeNamed resolves a name from a document, falling back to English.
//
// A name nothing registered is not an error. A document written in a process
// that had a Portuguese locale and read in one that does not is still a
// chart, and refusing to draw it — or dropping the rest of the scale with it —
// would be a worse answer than English tick labels. The name survives in the
// Desc either way, so a round trip through a process that cannot resolve it
// still writes it back out.
func localeNamed(name string) *Locale {
	if name == "" {
		return nil
	}
	if l, ok := LookupLocale(name); ok {
		return l
	}
	// English's tables under the document's name. The chart draws in English,
	// which is the only thing this process can do, and the name is still
	// written back out — so a document that travels through a process without
	// the tables comes out the other side asking for the same language.
	unknown := *English
	unknown.Name = name
	return &unknown
}
