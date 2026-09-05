package mathtext_test

import (
	"math"
	"strings"
	"testing"

	"github.com/timzifer/refract/internal/irtest"
	"github.com/timzifer/refract/ir"
	"github.com/timzifer/refract/mathtext"
)

// metrics is a font stack with round numbers: every character is half an em
// wide, the ascent is four fifths of one and the descent a fifth. Real metrics
// would make every expected number in this file a computation, and what is
// being tested is where the pieces go rather than how wide a glyph is.
type metrics struct{ calls int }

func (m *metrics) Measure(run ir.TextRun) ir.TextMetrics {
	m.calls++
	w := float32(float64(len([]rune(run.Text))) * run.Font.Size * 0.5)
	return ir.TextMetrics{
		Advance: w,
		Ascent:  float32(run.Font.Size * 0.8),
		Descent: float32(run.Font.Size * 0.2),
		Ink:     ir.R(0, float32(-run.Font.Size*0.8), w, float32(run.Font.Size*0.2)),
	}
}

const size = 10

func font() ir.FontRef { return ir.FontRef{Size: size} }

func TestALabelWithNoNotationIsRefused(t *testing.T) {
	ts := mathtext.TeX()
	for _, src := range []string{"", "time (s)", "cost in $", "50% of 20"} {
		if _, ok := ts.Typeset(src, font(), &metrics{}); ok {
			t.Errorf("Typeset(%q) claimed notation where there is none", src)
		}
	}
}

// A price is the reason the delimiter is a pair rather than a single dollar
// sign: "$5 to $10" is two dollars and one range, and typesetting " 5 to " as
// an expression would be a chart that misreads its own label.
func TestAnUnclosedDollarIsMoney(t *testing.T) {
	if _, ok := mathtext.TeX().Typeset("$5 and up", font(), &metrics{}); ok {
		t.Fatal("an unclosed $ was read as notation")
	}
}

func TestADoubledDollarIsALiteralOne(t *testing.T) {
	l, ok := mathtext.TeX().Typeset("$$5 per $x$", font(), &metrics{})
	if !ok {
		t.Fatal("no notation found")
	}
	if got := text(l); got != "$5 per x" {
		t.Errorf("laid out %q, want %q", got, "$5 per x")
	}
}

func TestNotationItCannotParseIsLeftAsWritten(t *testing.T) {
	for _, src := range []string{`$\nosuchmacro$`, `$x^$`, `$\frac{1}$`, `$}{$`, `$x^1^2$`} {
		if _, ok := mathtext.TeX().Typeset(src, font(), &metrics{}); ok {
			t.Errorf("Typeset(%q) claimed to lay out unparseable notation", src)
		}
	}
}

func TestAScriptIsSmallerAndOffTheBaseline(t *testing.T) {
	l, ok := mathtext.TeX().Typeset("$x^2_i$", font(), &metrics{})
	if !ok {
		t.Fatal("no notation found")
	}
	if len(l.Runs) != 3 {
		t.Fatalf("laid out %d runs, want a base and two scripts", len(l.Runs))
	}
	base, sup, sub := l.Runs[0], l.Runs[1], l.Runs[2]
	if base.Font.Size != size {
		t.Errorf("the base is set at %v, want %v", base.Font.Size, size)
	}
	if sup.Font.Size >= base.Font.Size || sub.Font.Size != sup.Font.Size {
		t.Errorf("scripts are set at %v and %v, want both smaller than %v",
			sup.Font.Size, sub.Font.Size, base.Font.Size)
	}
	if sup.At.Y >= 0 {
		t.Errorf("the superscript sits at y %v, want it above the baseline", sup.At.Y)
	}
	if sub.At.Y <= 0 {
		t.Errorf("the subscript sits at y %v, want it below the baseline", sub.At.Y)
	}
	// Both scripts belong to the same atom, so they are stacked rather than set
	// one after the other: x_i^2 and x^2_i are the same expression.
	if sup.At.X != sub.At.X {
		t.Errorf("the scripts start at x %v and %v, want them stacked", sup.At.X, sub.At.X)
	}
	if l.Ascent <= size*0.8 {
		t.Errorf("ascent %v does not cover the raised superscript", l.Ascent)
	}
}

func TestScriptsStackTheSameWayRoundEither(t *testing.T) {
	a, _ := mathtext.TeX().Typeset("$x^2_i$", font(), &metrics{})
	b, _ := mathtext.TeX().Typeset("$x_i^2$", font(), &metrics{})
	if a.Width != b.Width || a.Ascent != b.Ascent || a.Descent != b.Descent {
		t.Errorf("x^2_i is %v wide and x_i^2 is %v; they are one expression", a.Width, b.Width)
	}
}

func TestASingleLetterIsItalicAndAWordIsNot(t *testing.T) {
	l, _ := mathtext.TeX().Typeset(`$x$ and $\mathrm{max}$ and $abc$`, font(), &metrics{})
	for _, r := range l.Runs {
		switch r.Text {
		case "x":
			if !r.Font.Italic {
				t.Error("a single-letter variable is not italic")
			}
		case "max", "abc":
			if r.Font.Italic {
				t.Errorf("%q is set italic; a name is upright", r.Text)
			}
		}
	}
}

func TestItalicsCanBeTurnedOff(t *testing.T) {
	l, _ := mathtext.TeX(mathtext.Italic(false)).Typeset("$x$", font(), &metrics{})
	for _, r := range l.Runs {
		if r.Font.Italic {
			t.Error("a run is italic with italics turned off")
		}
	}
}

func TestAFractionStacksAroundARule(t *testing.T) {
	l, ok := mathtext.TeX().Typeset(`$\frac{ab}{c}$`, font(), &metrics{})
	if !ok {
		t.Fatal("no notation found")
	}
	if len(l.Rules) != 1 {
		t.Fatalf("laid out %d rules, want the fraction bar", len(l.Rules))
	}
	rule := l.Rules[0]
	var num, den mathtext.Run
	for _, r := range l.Runs {
		switch r.Text {
		case "ab":
			num = r
		case "c":
			den = r
		}
	}
	if num.Text == "" || den.Text == "" {
		t.Fatalf("laid out %d runs, want a numerator and a denominator", len(l.Runs))
	}
	if num.At.Y >= rule.Min.Y {
		t.Errorf("the numerator's baseline is at %v and the bar's top at %v", num.At.Y, rule.Min.Y)
	}
	if den.At.Y <= rule.Max.Y {
		t.Errorf("the denominator's baseline is at %v and the bar's bottom at %v", den.At.Y, rule.Max.Y)
	}
	// The wider half decides the width, and both halves are centred on it.
	if l.Width < rule.Max.X-rule.Min.X {
		t.Errorf("the expression is %v wide and its bar %v", l.Width, rule.Max.X-rule.Min.X)
	}
	if got, want := center(num.At.X, num.Text), center(den.At.X, den.Text); math.Abs(float64(got-want)) > 0.01 {
		t.Errorf("the halves are centred at %v and %v", got, want)
	}
}

// center is where a run's middle is, given this file's half-em metrics.
func center(x float32, s string) float32 { return x + float32(len([]rune(s)))*size*0.25 }

func TestARadicalCoversItsArgument(t *testing.T) {
	l, ok := mathtext.TeX().Typeset(`$\sqrt{n}$`, font(), &metrics{})
	if !ok {
		t.Fatal("no notation found")
	}
	if len(l.Rules) != 1 {
		t.Fatalf("laid out %d rules, want the bar over the argument", len(l.Rules))
	}
	var sign, arg mathtext.Run
	for _, r := range l.Runs {
		if r.Text == "√" {
			sign = r
		} else {
			arg = r
		}
	}
	if sign.Text == "" || arg.Text == "" {
		t.Fatalf("laid out %v, want a radical and an argument", l.Runs)
	}
	if arg.At.X <= sign.At.X {
		t.Error("the argument is not after the radical sign")
	}
	bar := l.Rules[0]
	if bar.Min.X > arg.At.X || bar.Max.X < arg.At.X {
		t.Errorf("the bar spans %v to %v and the argument starts at %v", bar.Min.X, bar.Max.X, arg.At.X)
	}
	if bar.Max.Y > 0 {
		t.Errorf("the bar is at %v, below the baseline", bar.Max.Y)
	}
	if -bar.Min.Y > l.Ascent+0.01 {
		t.Errorf("the bar rises to %v, above the reported ascent %v", -bar.Min.Y, l.Ascent)
	}
}

func TestSymbolsBecomeCharacters(t *testing.T) {
	l, ok := mathtext.TeX().Typeset(`$\alpha\leq\Omega$`, font(), &metrics{})
	if !ok {
		t.Fatal("no notation found")
	}
	if got := text(l); got != "α≤Ω" {
		t.Errorf("laid out %q, want %q", got, "α≤Ω")
	}
}

func TestARegisteredSymbolTypesets(t *testing.T) {
	if _, ok := mathtext.Symbol("testsmiley"); ok {
		t.Fatal("the test symbol exists before it is registered")
	}
	mathtext.RegisterSymbol("testsmiley", '☺')
	if r, ok := mathtext.Symbol("testsmiley"); !ok || r != '☺' {
		t.Fatalf("Symbol = %q, %v", r, ok)
	}
	l, ok := mathtext.TeX().Typeset(`$\alpha\testsmiley$`, font(), &metrics{})
	if !ok {
		t.Fatal("no notation found")
	}
	if got := text(l); got != "α☺" {
		t.Errorf("laid out %q, want %q", got, "α☺")
	}
}

func TestSpacesHaveWidthAndNoInk(t *testing.T) {
	// The reference is an empty group rather than "$ab$", because two adjacent
	// letters are one run and one name where a and b either side of anything
	// are two variables — which is the italic rule working, not the spacing.
	tight, _ := mathtext.TeX().Typeset(`$a{}b$`, font(), &metrics{})
	loose, _ := mathtext.TeX().Typeset(`$a\,b$`, font(), &metrics{})

	if got, want := loose.Width-tight.Width, float32(size*3.0/18); math.Abs(float64(got-want)) > 0.01 {
		t.Errorf("a thin space added %v to the width, want %v", got, want)
	}
	if len(loose.Runs) != len(tight.Runs) {
		t.Errorf("a space drew something: %d runs against %d", len(loose.Runs), len(tight.Runs))
	}
	if text(loose) != text(tight) {
		t.Errorf("a space changed the text: %q against %q", text(loose), text(tight))
	}
}

// Ordinary spaces are notation's whitespace and set nothing, which is what
// makes "$a + b$" set the same as "$a+b$".
func TestWhitespaceInNotationIsNotASpace(t *testing.T) {
	a, _ := mathtext.TeX().Typeset("$a+b$", font(), &metrics{})
	b, _ := mathtext.TeX().Typeset("$a + b$", font(), &metrics{})
	if a.Width != b.Width {
		t.Errorf("widths %v and %v differ; whitespace in notation is not a space", a.Width, b.Width)
	}
}

func TestTextAndNotationMix(t *testing.T) {
	l, ok := mathtext.TeX().Typeset(`peak $P_{max}$ (W)`, font(), &metrics{})
	if !ok {
		t.Fatal("no notation found")
	}
	if got, want := text(l), "peak P_max (W)"; strings.ReplaceAll(got, "_", "") != strings.ReplaceAll(want, "_", "") {
		t.Errorf("laid out %q, want the pieces of %q", got, want)
	}
	if l.Width <= 0 {
		t.Error("the label has no width")
	}
}

func TestMetricsCoverEverythingDrawn(t *testing.T) {
	l, _ := mathtext.TeX().Typeset(`$\frac{a}{b}^2$`, font(), &metrics{})
	m := l.Metrics()
	for _, r := range l.Runs {
		if -r.At.Y > m.Ascent+0.01 {
			t.Errorf("run %q rises to %v, above the reported ascent %v", r.Text, -r.At.Y, m.Ascent)
		}
	}
	for _, rule := range l.Rules {
		if -rule.Min.Y > m.Ascent+0.01 || rule.Max.Y > m.Descent+0.01 {
			t.Errorf("a rule at %v..%v is outside the reported box %v..%v",
				rule.Min.Y, rule.Max.Y, -m.Ascent, m.Descent)
		}
	}
	if m.Advance != l.Width {
		t.Errorf("advance %v is not the width %v", m.Advance, l.Width)
	}
}

func TestDrawEmitsTextAndRules(t *testing.T) {
	l, _ := mathtext.TeX().Typeset(`$\frac{a}{b}$`, font(), &metrics{})
	rec := irtest.New()
	mathtext.Draw(rec, l, ir.Point{X: 100, Y: 50}, ir.Point{}, ir.RGB(1, 2, 3), 0)

	var texts, fills int
	for _, c := range rec.Calls {
		switch c.Op {
		case "Text":
			texts++
			if c.Text.At.X < 100 || c.Text.At.Y > 50 && c.Text.At.Y < 50 {
				t.Errorf("a run landed at %v, not about the anchor", c.Text.At)
			}
			if c.Text.Color != ir.RGB(1, 2, 3) {
				t.Errorf("a run is %v, not the colour it was drawn in", c.Text.Color)
			}
		case "FillPath":
			fills++
		}
	}
	if texts != 2 || fills != 1 {
		t.Errorf("drew %d runs and %d rules, want 2 and 1", texts, fills)
	}
}

func TestARotatedLayoutIsDrawnInOneFrame(t *testing.T) {
	l, _ := mathtext.TeX().Typeset(`$\frac{a}{b}$`, font(), &metrics{})
	rec := irtest.New()
	mathtext.Draw(rec, l, ir.Point{X: 10, Y: 20}, ir.Point{}, ir.RGB(0, 0, 0), -math.Pi/2)

	if len(rec.Calls) == 0 || rec.Calls[0].Op != "Push" {
		t.Fatalf("a rotated layout did not open a frame: %v", rec.Calls)
	}
	if rec.Calls[len(rec.Calls)-1].Op != "Pop" {
		t.Error("a rotated layout did not close its frame")
	}
}

func TestPlainReadsNotationAloud(t *testing.T) {
	p, ok := mathtext.TeX().(mathtext.Plainer)
	if !ok {
		t.Fatal("the built-in typesetter cannot write its notation as text")
	}
	for _, c := range []struct{ src, want string }{
		{`flux $F_\nu$`, "flux F_ν"},
		{`$\frac{\sigma^2}{\sqrt{n}}$`, "(σ^2)/(√n)"},
		{`$x^{n+1}$`, "x^(n+1)"},
		{`$\alpha \times 10^3$`, "α×10^3"},
	} {
		got, ok := p.Plain(c.src)
		if !ok {
			t.Errorf("Plain(%q) found no notation", c.src)
			continue
		}
		if got != c.want {
			t.Errorf("Plain(%q) = %q, want %q", c.src, got, c.want)
		}
	}
	if got, ok := p.Plain("plain label"); ok || got != "plain label" {
		t.Errorf("Plain of a plain label reported (%q, %v)", got, ok)
	}
}

// A typesetter measures through the backend that will draw, and no other. The
// count is what proves it: a layout that measured nothing would be laying out
// against numbers it invented.
func TestATypesetterMeasuresThroughTheBackend(t *testing.T) {
	m := &metrics{}
	if _, ok := mathtext.TeX().Typeset(`$x^2$`, font(), m); !ok {
		t.Fatal("no notation found")
	}
	if m.calls == 0 {
		t.Error("the typesetter never measured anything")
	}
}

func text(l mathtext.Layout) string {
	var b strings.Builder
	for _, r := range l.Runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

func TestABarSitsOverItsArgument(t *testing.T) {
	l, ok := mathtext.TeX().Typeset(`$\bar{x}$`, font(), &metrics{})
	if !ok {
		t.Fatal("no notation found")
	}
	if len(l.Runs) != 1 || l.Runs[0].Text != "x" {
		t.Fatalf("laid out %v, want the argument alone", l.Runs)
	}
	if len(l.Rules) != 1 {
		t.Fatalf("laid out %d rules, want the bar", len(l.Rules))
	}
	bar := l.Rules[0]
	if bar.Max.Y > 0 {
		t.Errorf("the bar is at %v, below the baseline", bar.Max.Y)
	}
	if bar.Min.X != 0 || bar.Max.X != l.Width {
		t.Errorf("the bar spans %v to %v, want the whole width %v", bar.Min.X, bar.Max.X, l.Width)
	}
	if -bar.Min.Y > l.Ascent+0.01 {
		t.Errorf("the bar rises to %v, above the reported ascent %v", -bar.Min.Y, l.Ascent)
	}

	p := mathtext.TeX().(mathtext.Plainer)
	if got, _ := p.Plain(`$\bar{x}$`); got != "x̄" {
		t.Errorf("Plain read the bar as %q", got)
	}
}

// Spacing is what makes notation read as notation. TeX puts a medium space
// either side of a binary operator and a thick one either side of a relation,
// and a chart label set without them reads as a filename.
func TestOperatorsAndRelationsAreSpaced(t *testing.T) {
	tight, _ := mathtext.TeX().Typeset(`$a{}b$`, font(), &metrics{})
	plus, _ := mathtext.TeX().Typeset(`$a+b$`, font(), &metrics{})
	eq, _ := mathtext.TeX().Typeset(`$a=b$`, font(), &metrics{})

	// Each expression is two letters plus one operator wide, so the difference
	// between them is exactly the spacing.
	op := float32(size * 0.5)
	gapPlus := plus.Width - tight.Width - op
	gapEq := eq.Width - tight.Width - op
	if want := float32(2 * size * 4.0 / 18); math.Abs(float64(gapPlus-want)) > 0.01 {
		t.Errorf("a binary operator got %v of space, want %v", gapPlus, want)
	}
	if want := float32(2 * size * 5.0 / 18); math.Abs(float64(gapEq-want)) > 0.01 {
		t.Errorf("a relation got %v of space, want %v", gapEq, want)
	}
	if gapEq <= gapPlus {
		t.Error("a relation is not spaced more widely than an operator")
	}
}

// A minus with nothing to its left is a sign rather than an operator, and a
// sign takes no space: "-1" is a number, not a subtraction with a gap in it.
func TestASignIsNotAnOperator(t *testing.T) {
	sign, _ := mathtext.TeX().Typeset(`$-1$`, font(), &metrics{})
	bare, _ := mathtext.TeX().Typeset(`$21$`, font(), &metrics{})
	if math.Abs(float64(sign.Width-bare.Width)) > 0.01 {
		t.Errorf("a leading minus was spaced as an operator: %v against %v", sign.Width, bare.Width)
	}
}

// An operator is never merged into the run beside it, or the spacing would
// have nothing to sit between.
func TestAnOperatorIsItsOwnRun(t *testing.T) {
	l, _ := mathtext.TeX().Typeset(`$x=1$`, font(), &metrics{})
	if len(l.Runs) != 3 {
		t.Fatalf("laid out %d runs for x=1, want three", len(l.Runs))
	}
	for i, want := range []string{"x", "=", "1"} {
		if l.Runs[i].Text != want {
			t.Errorf("run %d is %q, want %q", i, l.Runs[i].Text, want)
		}
	}
}
