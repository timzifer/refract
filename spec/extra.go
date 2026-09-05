package spec

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// MarshalJSON writes the mark object with the mark's own properties folded in.
//
// A property this package has a field for is written from the field; one it
// does not — the [Mark.Extra] a third-party mark carries — is written beside
// them, at the same level, so that a document reads the same whether refract
// or its author defined the mark. A key that names a field this package owns is
// refused, because a document with two meanings for one key has no meaning.
func (m Mark) MarshalJSON() ([]byte, error) {
	type plain Mark
	b, err := json.Marshal(plain(m))
	if err != nil || len(m.Extra) == 0 {
		return b, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(m.Extra))
	for k := range m.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if markFields()[k] {
			return nil, fmt.Errorf("refract/spec: mark property %q is one this package defines", k)
		}
		v, err := json.Marshal(m.Extra[k])
		if err != nil {
			return nil, fmt.Errorf("refract/spec: mark property %q: %w", k, err)
		}
		obj[k] = v
	}
	return json.Marshal(obj)
}

// UnmarshalJSON reads the mark object, keeping every property this package has
// no field for in [Mark.Extra] so that a registered mark can read it back.
func (m *Mark) UnmarshalJSON(b []byte) error {
	type plain Mark
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*m = Mark(p)
	m.Extra = nil

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	for k, raw := range obj {
		if markFields()[k] {
			continue
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("refract/spec: mark property %q: %w", k, err)
		}
		if m.Extra == nil {
			m.Extra = map[string]any{}
		}
		m.Extra[k] = v
	}
	return nil
}

var (
	markFieldsOnce sync.Once
	markFieldSet   map[string]bool
)

// markFields is the set of JSON keys [Mark] has a field for, read once from the
// struct tags so that a field added later is known here without a second list.
func markFields() map[string]bool {
	markFieldsOnce.Do(func() {
		markFieldSet = map[string]bool{}
		t := reflect.TypeFor[Mark]()
		for i := 0; i < t.NumField(); i++ {
			tag := t.Field(i).Tag.Get("json")
			name, _, _ := strings.Cut(tag, ",")
			if name == "" {
				name = t.Field(i).Name
			}
			if name != "-" {
				markFieldSet[name] = true
			}
		}
	})
	return markFieldSet
}
