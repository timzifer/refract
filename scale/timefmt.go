package scale

import (
	"strings"
	"time"
)

// TimeLayout fixes the layout every tick label on this axis is written in,
// instead of letting the scale choose one per tick spacing.
//
// It is [TimeFormat] written down: a Go reference layout is a string, so it
// survives a round trip through the JSON spec, and a chart configured rather
// than compiled can say that its axis reads "2006-01-02" — which
// [TimeFormat]'s function could say only from Go. A Go function still
// outranks it, for the reason it outranks [NumberFormat]: a caller who wrote
// one has answered the question.
//
// The scale's own choice is what it always was — a ladder of layouts, one per
// tick spacing, so that an axis over three hours says "Jan 2 15:04" and one
// over three years says "Jan 2006". Naming a layout gives that up in exchange
// for knowing exactly what every label says, which is what a caller who names
// one is asking for.
//
// The month and weekday names come from the axis's locale rather than from
// Go's own tables, so "January 2006" is "Januar 2006" on a German axis. See
// [Locale].
func TimeLayout(layout string) TimeOption {
	return func(t *timeScale) { t.layout = layout }
}

// localTime writes t in a Go reference layout, with the month and weekday
// names taken from a locale.
//
// Go's time package has no hook for this: [time.Time.Format] carries its own
// English tables and there is no way to reach past them. So the layout is
// split on the four tokens that name something — "January", "Jan", "Monday",
// "Mon" — the pieces between them go through Format unchanged, and the tokens
// are filled from the locale. Everything else about a layout, including
// numbers, padding, time zones and fractional seconds, is still Go's own,
// which is what keeps a layout meaning what its documentation says it means.
//
// The long forms are matched before the short ones, or "January" would be
// read as "Jan" followed by a literal "uary".
func localTime(t time.Time, layout string, loc *Locale) string {
	loc = localeOr(loc)
	if !strings.ContainsAny(layout, "JM") {
		return t.Format(layout)
	}
	var b strings.Builder
	b.Grow(len(layout) + 16)
	for i := 0; i < len(layout); {
		tok, name := "", ""
		switch {
		case strings.HasPrefix(layout[i:], "January"):
			tok, name = "January", loc.monthName(t.Month(), false)
		case strings.HasPrefix(layout[i:], "Jan"):
			tok, name = "Jan", loc.monthName(t.Month(), true)
		case strings.HasPrefix(layout[i:], "Monday"):
			tok, name = "Monday", loc.dayName(t.Weekday(), false)
		case strings.HasPrefix(layout[i:], "Mon"):
			tok, name = "Mon", loc.dayName(t.Weekday(), true)
		}
		if tok == "" {
			// Not a name token. Take the run up to the next one and let Go
			// format it, so that everything a layout can say still works.
			j := nextToken(layout, i+1)
			b.WriteString(t.Format(layout[i:j]))
			i = j
			continue
		}
		b.WriteString(name)
		i += len(tok)
	}
	return b.String()
}

// nextToken finds where the next name token starts, or the end of the layout.
func nextToken(layout string, from int) int {
	for i := from; i < len(layout); i++ {
		switch layout[i] {
		case 'J':
			if strings.HasPrefix(layout[i:], "Jan") {
				return i
			}
		case 'M':
			if strings.HasPrefix(layout[i:], "Mon") {
				return i
			}
		}
	}
	return len(layout)
}
