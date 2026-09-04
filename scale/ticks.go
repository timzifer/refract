package scale

import "sort"

// sortTicks orders a tick sequence ascending by value, which every Scale.Ticks
// implementation promises.
func sortTicks(ts []Tick) {
	sort.Slice(ts, func(i, j int) bool { return ts[i].Value < ts[j].Value })
}
