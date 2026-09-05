package geom

import (
	"fmt"
	"sort"
	"sync"
)

// Configure applies opts and reports what they set.
//
// It is how a geom defined outside this package reads the shared option set.
// [Option] is a function over an unexported configuration, which is what keeps
// the set one namespace — an option a geom has no use for is accepted and
// ignored — and it is also what would keep a third-party mark from honouring
// [X], [Color] or [Label] at all. So the configuration is handed out in the
// form that is already public: the [Desc] a layer writes itself down as, with
// no Mark and no Source, because the caller is about to supply both.
//
//	func Lollipop(src data.Source, opts ...geom.Option) geom.Geom {
//	    return &lollipop{src: src, cfg: geom.Configure(opts...)}
//	}
//
//	func (l *lollipop) Build(b ir.Backend, f geom.Frame) error {
//	    xs, _ := l.src.Float64Column(l.cfg.X)
//	    col := f.Theme.Palette.At(f.Index)
//	    if l.cfg.Color != nil {
//	        col = *l.cfg.Color
//	    }
//	    // …
//	}
//
// The defaults are the ones every layer in this package starts from: a bar
// fills 0.8 of its slot, whiskers reach 1.5 IQR, outliers are shown, missing
// values leave a gap, an annotation extends its axis. A layer that returns
// this Desc from its own [Describer] — with Mark and Source filled in — is
// serialisable by the JSON spec like any built-in one, and a layer built by
// [Register] from that Desc is configured identically: [Desc] and Configure
// are inverses, the same way [Describe] and [FromDesc] are.
func Configure(opts ...Option) Desc {
	return newConfig(opts).describe("")
}

// Extra sets an option this package does not define.
//
// It is the one escape from a closed option type: a third-party mark that
// needs a knob of its own — a lollipop's stem width, a waffle's cell count —
// takes it here rather than through a second variadic list, so that its
// constructor reads exactly like a built-in one. The value travels through
// [Desc.Extra] and, for a mark that describes itself, through the JSON spec
// as a property on the mark object — which is why the key should be a name
// that reads well in a document, and why a value should be something
// encoding/json can write: a number, a string, a bool, or a list or map of
// those.
//
// Every geom in this package accepts and ignores it, like any option it has no
// use for.
func Extra(key string, v any) Option {
	return func(c *config) {
		if c.extra == nil {
			c.extra = map[string]any{}
		}
		c.extra[key] = v
	}
}

// Register makes a mark this package does not define buildable by [FromDesc],
// and therefore readable from a JSON spec.
//
// The name is the one the mark's [Describer] reports, and the one a document
// carries as the mark's type. A name owned by this package — every [Mark]
// constant — is refused with a panic, because shadowing a built-in would
// change what every existing document means. Registering a name twice
// replaces the earlier builder, which is what a test or a hot reload wants.
//
// Nothing iterates the registry, and lookups are guarded by a lock, so
// registering from an init function or from a goroutine is safe and
// registration order never changes what a chart draws.
//
//	func init() {
//	    geom.Register("lollipop", func(d geom.Desc) (geom.Geom, error) {
//	        if d.Source == nil {
//	            return nil, errors.New("a lollipop layer needs a data source")
//	        }
//	        return Lollipop(d.Source, d.Options()...), nil
//	    })
//	}
func Register(m Mark, build func(Desc) (Geom, error)) {
	if m == "" {
		panic("refract/geom: Register: a mark needs a name")
	}
	if build == nil {
		panic("refract/geom: Register: a mark needs a builder")
	}
	if builtin(m) {
		panic(fmt.Sprintf("refract/geom: Register: %q is a built-in mark", m))
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[m] = build
}

// Options turns the description back into the option list that would have
// produced it — the inverse of [Configure], and what a builder given to
// [Register] hands to its own constructor.
//
// Every option is applied rather than only the ones that differ from a
// default: an option set to its default value is the default value, and
// filtering would only be a second place for the defaults to be written down.
func (d Desc) Options() []Option { return d.options() }

var (
	registryMu sync.RWMutex
	registry   = map[Mark]func(Desc) (Geom, error){}
)

func registered(m Mark) (func(Desc) (Geom, error), bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	b, ok := registry[m]
	return b, ok
}

// builtin reports whether m is a mark this package defines.
func builtin(m Mark) bool {
	switch m {
	case MarkLine, MarkScatter, MarkBar, MarkArea, MarkStep, MarkBoxplot, MarkRect,
		MarkHistogram, MarkViolin, MarkRidgeline, MarkHexbin, MarkBeeswarm, MarkECDF, MarkTrend,
		MarkHLine, MarkVLine, MarkHBand, MarkVBand, MarkSegment, MarkRegion, MarkNote:
		return true
	}
	return false
}

// sortedKeys is the map's keys in order, so that a description turned back
// into options applies them the same way every time.
func sortedKeys(m map[string]any) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
