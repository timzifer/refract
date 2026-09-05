package mathtext

import "sync"

// symbols maps a TeX command name, without its backslash, to the character it
// sets.
//
// It is the notation a chart label actually reaches for: the Greek alphabet,
// the comparisons, the arithmetic that has no key on a keyboard, and the few
// operators that appear in an axis title. It is not TeX's symbol table, which
// runs to thousands and would be a font problem rather than a typesetting one
// — every character here is one a general-purpose font has.
//
// A command it does not know is a parse error, and a label containing one is
// drawn as it was typed, which is how the mistake becomes visible. A caller
// adds to it through [RegisterSymbol], never by reaching into the map: the
// table is read on every typeset, and a write racing a render is exactly the
// bug a package-level map invites.
var symbols = map[string]rune{
	// Lower-case Greek.
	"alpha": 'α', "beta": 'β', "gamma": 'γ', "delta": 'δ',
	"epsilon": 'ε', "varepsilon": 'ϵ', "zeta": 'ζ', "eta": 'η',
	"theta": 'θ', "vartheta": 'ϑ', "iota": 'ι', "kappa": 'κ',
	"lambda": 'λ', "mu": 'μ', "nu": 'ν', "xi": 'ξ',
	"pi": 'π', "varpi": 'ϖ', "rho": 'ρ', "varrho": 'ϱ',
	"sigma": 'σ', "varsigma": 'ς', "tau": 'τ', "upsilon": 'υ',
	"phi": 'φ', "varphi": 'ϕ', "chi": 'χ', "psi": 'ψ', "omega": 'ω',

	// Upper-case Greek. The ones TeX has, which are the ones that are not a
	// Latin capital.
	"Gamma": 'Γ', "Delta": 'Δ', "Theta": 'Θ', "Lambda": 'Λ',
	"Xi": 'Ξ', "Pi": 'Π', "Sigma": 'Σ', "Upsilon": 'Υ',
	"Phi": 'Φ', "Psi": 'Ψ', "Omega": 'Ω',

	// Arithmetic and relations.
	"times": '×', "div": '÷', "pm": '±', "mp": '∓',
	"cdot": '·', "ast": '∗', "star": '⋆',
	"leq": '≤', "le": '≤', "geq": '≥', "ge": '≥',
	"neq": '≠', "ne": '≠', "equiv": '≡', "sim": '∼',
	"approx": '≈', "propto": '∝', "ll": '≪', "gg": '≫',

	// Set theory and logic, for a label that names a domain.
	"in": '∈', "notin": '∉', "subset": '⊂', "subseteq": '⊆',
	"cup": '∪', "cap": '∩', "emptyset": '∅',
	"forall": '∀', "exists": '∃', "neg": '¬',

	// Big operators. They are set at text size rather than grown, because a
	// label is one line tall and a display-size sigma would not fit in it.
	"sum": '∑', "prod": '∏', "int": '∫', "oint": '∮',

	// Calculus and analysis.
	"partial": '∂', "nabla": '∇', "infty": '∞',
	"to": '→', "rightarrow": '→', "leftarrow": '←', "leftrightarrow": '↔',
	"Rightarrow": '⇒', "Leftarrow": '⇐',

	// Punctuation and units.
	"degree": '°', "deg": '°', "prime": '′', "dprime": '″',
	"ldots": '…', "cdots": '⋯', "angle": '∠', "perp": '⊥',
	"circ": '∘', "bullet": '•', "dagger": '†',

	// Currency and per-cent, which are the two a label reaches for and which
	// TeX itself makes awkward.
	"percent": '%', "permil": '‰', "euro": '€', "pound": '£',
}

// RegisterSymbol adds a command to the symbol table, or replaces one: after
// RegisterSymbol("degree", '°'), a label containing `\degree` sets the degree
// sign. The name is written without its backslash. A caller may register from
// an init function or at any later time; the table is guarded, so a
// registration and a render on another goroutine do not race.
//
// A name this package defines may be replaced, which is how a caller with an
// opinion about `\epsilon` gets the variant they want; there is no way to
// remove one, because a label written against the built-in table has to keep
// meaning what it meant.
func RegisterSymbol(name string, r rune) {
	if name == "" || r == 0 {
		panic("refract/mathtext: RegisterSymbol: a symbol needs a name and a character")
	}
	symbolsMu.Lock()
	defer symbolsMu.Unlock()
	symbols[name] = r
}

// Symbol reports the character a command sets, and whether the command is
// known. It is the read side of [RegisterSymbol].
func Symbol(name string) (rune, bool) {
	symbolsMu.RLock()
	defer symbolsMu.RUnlock()
	r, ok := symbols[name]
	return r, ok
}

var symbolsMu sync.RWMutex
