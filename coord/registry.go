package coord

import (
	"fmt"
	"sync"
)

// Register makes a coordinate system this package does not define buildable
// by [FromDesc], and therefore readable from a JSON spec.
//
// The type is the one the coord's [Describer] reports, and the one a document
// carries as the coord's type. A type this package owns — every [Type]
// constant — is refused with a panic, because shadowing a built-in would
// change what every existing document means. Registering a type twice
// replaces the earlier builder.
//
// A registered coord is rebuilt from the fields of [Desc], which are the
// polar coord's. A coordinate system configured by something else — a
// projection's centre, say — round-trips its type and no more until the spec
// grows a place for it, which is additive.
//
// Nothing iterates the registry and lookups are guarded by a lock, so
// registering from an init function is safe and registration order never
// changes what a chart draws.
func Register(t Type, build func(Desc) (Coord, error)) {
	if t == "" || build == nil {
		panic("refract/coord: Register: a type needs a name and a builder")
	}
	switch t {
	case TypeCartesian, TypePolar:
		panic(fmt.Sprintf("refract/coord: Register: %q is a built-in coord type", t))
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[t] = build
}

var (
	registryMu sync.RWMutex
	registry   = map[Type]func(Desc) (Coord, error){}
)

func registered(t Type) (func(Desc) (Coord, error), bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	b, ok := registry[t]
	return b, ok
}
