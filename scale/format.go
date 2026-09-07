package scale

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// A tick label is described rather than computed.
//
// [Format], [LogFormat] and [SymLogFormat] take a Go function, which is the
// most direct thing an axis can be given and the one thing a document cannot
// hold: [Desc] says so through Formatted, and a chart written down as JSON and
// read back labelled its ticks the standard way. That made the one field a
// dashboard most wants to set — thousands separated, two decimals, a currency,
// a percentage — reachable only from Go, which is to say not reachable at all
// from the configuration file the chart actually lives in.
//
// [NumberFormat] is the same choice written down. It is a string because a
// string is what survives the round trip and what a person editing a
// configuration can type; it is a *small* string because an axis asks few
// questions.
//
// # The grammar
//
//	spec   := prefix "#" [","] ["." digits] [style] suffix
//	style  := "%" | "k" | "e"
//
// The "#" is the number, and it is what tells the prefix from the suffix:
// everything before it is written before the number and everything left after
// the body is written after it.
//
//   - "," groups the whole part with the locale's group separator.
//   - "." followed by digits fixes the number of decimal places. Without it
//     the axis chooses, as it always has — enough decimals to tell adjacent
//     ticks apart, which is a property of the tick step rather than of any one
//     value and is what keeps a column of labels aligned.
//   - "%" multiplies by 100 and appends the locale's percent sign, spacing and
//     all: "12.5%" in English and "12,5 %" in German, from one spec.
//   - "k" is SI: 12500 becomes "12.5k", 0.0012 becomes "1.2m". Three
//     significant digits unless "." says otherwise.
//   - "e" is scientific, two decimals unless "." says otherwise.
//
// So:
//
//	"#,"        1,234,567
//	"#,.2"      1,234,567.00
//	"€ #,.2"    € 1,234,567.00
//	"#.1%"      12.5%
//	"#k"        1.23M
//	"# kg"      1234567 kg
//
// The last two are worth reading together. A style letter counts as one only
// where the body ends — directly after the "#", the grouping comma and the
// decimals — so "#k" is SI and "# kg" is a number with a unit after it. A
// suffix that begins with one of the three letters wants a space, or a spec
// that does not need one.
//
// # What it does not do
//
// It has no currency table, no accounting negatives, no significant-digit
// mode and no per-value conditionals. Those are a formatting library, and the
// seam for one is still [Format]: a Go function outranks a spec, so a caller
// who has a formatter keeps it. What the spec is for is the chart that is
// configured rather than compiled.
//
// # An invalid spec
//
// A spec written in Go source is a literal, so a malformed one panics — the
// same line [github.com/timzifer/refract/data.Table] draws about a ragged
// table. A spec read out of a document is input, so [FromDesc] returns an
// error for it rather than panicking. The two paths are the same parser.
func NumberFormat(spec string) LinearOption {
	f := mustFormat(spec)
	return func(l *linear) { l.numFormat = f }
}

// LogNumberFormat is [NumberFormat] for a log scale.
func LogNumberFormat(spec string) LogOption {
	f := mustFormat(spec)
	return func(l *logScale) { l.numFormat = f }
}

// SymLogNumberFormat is [NumberFormat] for a symlog scale.
func SymLogNumberFormat(spec string) SymLogOption {
	f := mustFormat(spec)
	return func(s *symlogScale) { s.numFormat = f }
}

// mustFormat parses a spec written in Go source, where a malformed one is a
// programming error.
func mustFormat(spec string) numberFormat {
	f, err := parseNumberFormat(spec)
	if err != nil {
		panic(err.Error())
	}
	return f
}

// ErrFormat reports a number format spec this package cannot read. It is
// returned by [FromDesc], which reads a spec that came from a document.
var ErrFormat = fmt.Errorf("refract/scale: invalid number format")

// numberFormat is a parsed spec. Its zero value is "no spec", which is what
// every scale carries until it is given one.
type numberFormat struct {
	spec     string // as written, for the round trip
	set      bool
	prefix   string
	suffix   string
	group    bool
	decimals int  // -1 when the axis chooses
	style    byte // 0, '%', 'k' or 'e'
}

func parseNumberFormat(spec string) (numberFormat, error) {
	if spec == "" {
		return numberFormat{}, nil
	}
	i := strings.IndexByte(spec, '#')
	if i < 0 {
		return numberFormat{}, fmt.Errorf("%w: %q has no %q, so there is nowhere to put the number", ErrFormat, spec, "#")
	}
	f := numberFormat{spec: spec, set: true, decimals: -1, prefix: spec[:i]}
	rest := spec[i+1:]
	if strings.HasPrefix(rest, ",") {
		f.group, rest = true, rest[1:]
	}
	if strings.HasPrefix(rest, ".") {
		j := 1
		for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
			j++
		}
		if j == 1 {
			return numberFormat{}, fmt.Errorf("%w: %q has a %q with no number of places after it", ErrFormat, spec, ".")
		}
		n, err := strconv.Atoi(rest[1:j])
		if err != nil || n > maxSpecDecimals {
			return numberFormat{}, fmt.Errorf("%w: %q asks for more than %d decimal places", ErrFormat, spec, maxSpecDecimals)
		}
		f.decimals, rest = n, rest[j:]
	}
	if len(rest) > 0 {
		switch rest[0] {
		case '%', 'k', 'e':
			f.style, rest = rest[0], rest[1:]
		}
	}
	f.suffix = rest
	return f, nil
}

// maxSpecDecimals is where a tick label stops being a number a reader can
// take in. It is well past [maxDecimals], which is the axis's own limit when
// it chooses for itself: a caller who names a precision has a reason.
const maxSpecDecimals = 15

// autoFormat is what the axis derived from its tick step: how a value is
// written when the caller did not say.
//
// It exists so that the step-derived choice and the spec-driven one meet in
// one place. Deriving decimals from the step rather than from each value is
// what keeps a column of labels aligned, and a spec that names a prefix but
// not a precision must not lose that.
type autoFormat struct {
	mode     byte // 'f', 'e' or 'g'
	decimals int
}

// shifted moves the decimal precision by n places, for a style that scales the
// value before writing it.
func (a autoFormat) shifted(n int) autoFormat {
	if a.mode != 'f' {
		return a
	}
	a.decimals = max(a.decimals-n, 0)
	return a
}

// digits writes v the way the axis would have without a spec. This is the
// path every existing chart takes, and it is byte-for-byte what
// [formatterFor] produced before the spec existed.
func (a autoFormat) digits(v float64) string {
	switch a.mode {
	case 'l':
		// The log ladder's own choice: as many decimals as the decade needs,
		// and 'g' once the digits get unreadable. It is a mode rather than a
		// decimal count because a decade is not a step.
		return formatLog(v)
	case 'e':
		return strconv.FormatFloat(v, 'e', a.decimals, 64)
	case 'g':
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
	s := strconv.FormatFloat(v, 'f', a.decimals, 64)
	// Snap values that are a hair off an exact tick, so 0.30000000000000004
	// prints as 0.3 and -0 prints as 0.
	if s == "-0" || (a.decimals > 0 && s == "-0."+zeros(a.decimals)) {
		s = s[1:]
	}
	return s
}

// label writes v as this axis's tick label: the spec's shape, the locale's
// punctuation, and the axis's own precision wherever the spec does not name
// one.
func (f numberFormat) label(v float64, a autoFormat, loc *Locale) string {
	loc = localeOr(loc)
	if !f.set {
		return punctuate(a.digits(v), false, loc)
	}

	var body, unit string
	switch f.style {
	case '%':
		// A percentage is the same number two decimal places to the left, so
		// the axis's own precision moves with it: an axis whose step is 0.25
		// needs two decimals to tell its ticks apart and its percentages need
		// none. Keeping the unscaled precision would write "50.00%" beside
		// "25.00%" on an axis that only ever has quarters.
		v *= 100
		unit = loc.Percent
		body = f.plain(v, a.shifted(2))
	case 'k':
		body, unit = siDigits(v, f.decimals)
	case 'e':
		body = strconv.FormatFloat(v, 'e', decimalsOr(f.decimals, 2), 64)
	default:
		body = f.plain(v, a)
	}
	return f.prefix + punctuate(body, f.group, loc) + unit + f.suffix
}

// plain writes the digits of a value the spec did not give a style to.
func (f numberFormat) plain(v float64, a autoFormat) string {
	if f.decimals < 0 {
		return a.digits(v)
	}
	s := strconv.FormatFloat(v, 'f', f.decimals, 64)
	if s == "-0" || (f.decimals > 0 && s == "-0."+zeros(f.decimals)) {
		s = s[1:]
	}
	return s
}

func decimalsOr(n, def int) int {
	if n < 0 {
		return def
	}
	return n
}

// siPrefixes runs from 10^-24 to 10^24, with the empty string at the middle
// for the values that need no prefix at all.
var siPrefixes = [...]string{"y", "z", "a", "f", "p", "n", "µ", "m", "", "k", "M", "G", "T", "P", "E", "Z", "Y"}

const siZero = 8 // the index of the empty prefix

// siDigits writes v as a number between 1 and 1000 and the SI prefix that
// scales it, with three significant digits unless the spec named a precision.
//
// Three is what makes the ladder readable: 1.23k, 12.3k, 123k, then 1.23M. A
// fixed number of decimals instead would write 1.2k beside 123.5k, which is
// two different precisions in one column.
func siDigits(v float64, decimals int) (digits, prefix string) {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return strconv.FormatFloat(v, 'f', max(decimals, 0), 64), ""
	}
	exp := int(math.Floor(math.Log10(math.Abs(v))))
	group := int(math.Floor(float64(exp) / 3))
	i := min(max(group+siZero, 0), len(siPrefixes)-1)
	scaled := v / math.Pow(10, float64((i-siZero)*3))

	if decimals >= 0 {
		return strconv.FormatFloat(scaled, 'f', decimals, 64), siPrefixes[i]
	}
	// Three significant digits: the whole part takes one, two or three of
	// them, and the fraction takes what is left.
	whole := 1
	if a := math.Abs(scaled); a >= 100 {
		whole = 3
	} else if a >= 10 {
		whole = 2
	}
	s := strconv.FormatFloat(scaled, 'f', 3-whole, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	return s, siPrefixes[i]
}

// punctuate applies a locale to digits that strconv wrote: its decimal
// separator, its minus sign, and its group separator where one was asked for.
//
// It is a rewrite of the string rather than a second formatter because the two
// have to agree exactly about how many digits there are. The sign is taken
// from the front only: an exponent carries a minus of its own, and that one is
// arithmetic rather than typography.
func punctuate(s string, group bool, loc *Locale) string {
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	tail := ""
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		s, tail = s[:i], s[i:]
	}
	whole, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		whole, frac = s[:i], s[i+1:]
	}
	if group && loc.Group != "" {
		whole = grouped(whole, loc.Group)
	}
	var b strings.Builder
	b.Grow(len(whole) + len(frac) + len(tail) + 8)
	if neg {
		b.WriteString(loc.Minus)
	}
	b.WriteString(whole)
	if frac != "" {
		b.WriteString(loc.Decimal)
		b.WriteString(frac)
	}
	b.WriteString(tail)
	return b.String()
}

// grouped inserts sep every three digits from the right.
func grouped(whole, sep string) string {
	n := len(whole)
	if n <= 3 {
		return whole
	}
	var b strings.Builder
	b.Grow(n + (n/3)*len(sep))
	lead := n % 3
	if lead == 0 {
		lead = 3
	}
	b.WriteString(whole[:lead])
	for i := lead; i < n; i += 3 {
		b.WriteString(sep)
		b.WriteString(whole[i : i+3])
	}
	return b.String()
}

// LabelOf implements [Labeller] for a linear scale: the value written with
// the precision its current tick sequence uses, so a tooltip and the axis
// under it agree.
func (l *linear) LabelOf(v float64) string {
	if l.format != nil {
		return l.format(v)
	}
	lo, hi := l.effective()
	return l.numFormat.label(v, autoFor(l.labelling(lo, hi, defaultTickCount).step), l.loc)
}

// LabelOf implements [Labeller] for a log scale.
func (l *logScale) LabelOf(v float64) string {
	if l.format != nil {
		return l.format(v)
	}
	return l.numFormat.label(v, autoFormat{mode: 'l'}, l.loc)
}

// LabelOf implements [Labeller] for a symlog scale.
func (s *symlogScale) LabelOf(v float64) string {
	if s.format != nil {
		return s.format(v)
	}
	return s.numFormat.label(v, autoFormat{mode: 'l'}, s.loc)
}

var (
	_ Labeller = (*linear)(nil)
	_ Labeller = (*logScale)(nil)
	_ Labeller = (*symlogScale)(nil)
)
