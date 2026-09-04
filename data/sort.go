package data

import "sort"

// sortedKeys returns the map's keys in a deterministic order, so that
// Source.Columns does not vary between runs.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
