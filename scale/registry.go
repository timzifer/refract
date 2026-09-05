package scale

import (
	"fmt"
	"sync"
)

// Register makes a scale kind this package does not define buildable by
// [FromDesc], and therefore readable from a JSON spec.
//
// The kind is the one the scale's [Describer] reports, and the one a document
// carries as the scale's type. A kind this package owns — every [Kind]
// constant — is refused with a panic, because shadowing a built-in would
// change what every existing document means. Registering a kind twice
// replaces the earlier builder.
//
// A registered scale is rebuilt from the fields of [Desc] alone: a fixed
// domain, the framing flags, a category list. A scale whose configuration
// does not fit in a Desc round-trips what does and no more, the way a scale
// with a Go formatter does — see [Desc.Formatted].
//
// Nothing iterates the registry and lookups are guarded by a lock, so
// registering from an init function is safe and registration order never
// changes what a chart draws.
func Register(k Kind, build func(Desc) (Scale, error)) {
	if k == "" || build == nil {
		panic("refract/scale: Register: a kind needs a name and a builder")
	}
	if builtinKind(k) {
		panic(fmt.Sprintf("refract/scale: Register: %q is a built-in scale kind", k))
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[k] = build
}

// RegisterColor is [Register] for colour scales: it makes a [ColorKind] this
// package does not define buildable by [ColorFromDesc].
func RegisterColor(k ColorKind, build func(ColorDesc) (ColorScale, error)) {
	if k == "" || build == nil {
		panic("refract/scale: RegisterColor: a kind needs a name and a builder")
	}
	if builtinColorKind(k) {
		panic(fmt.Sprintf("refract/scale: RegisterColor: %q is a built-in colour scale kind", k))
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	colorRegistry[k] = build
}

var (
	registryMu    sync.RWMutex
	registry      = map[Kind]func(Desc) (Scale, error){}
	colorRegistry = map[ColorKind]func(ColorDesc) (ColorScale, error){}
)

func registered(k Kind) (func(Desc) (Scale, error), bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	b, ok := registry[k]
	return b, ok
}

func registeredColor(k ColorKind) (func(ColorDesc) (ColorScale, error), bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	b, ok := colorRegistry[k]
	return b, ok
}

func builtinKind(k Kind) bool {
	switch k {
	case KindLinear, KindLog, KindSymLog, KindTime, KindOrdinal:
		return true
	}
	return false
}

func builtinColorKind(k ColorKind) bool {
	switch k {
	case KindSequential, KindDiverging, KindQualitative:
		return true
	}
	return false
}
