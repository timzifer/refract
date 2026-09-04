# allocgate.awk — the benchmark gate.
#
# Pipe `go test -bench` output through it. It enforces the one property the
# allocation pass promises: what a frame costs in allocations does not grow
# with the data it draws, so a chart redrawn every frame over a live series is
# affordable.
#
# Times are deliberately not checked. A shared CI runner has nothing reliable
# to say about them, and a gate that flakes is a gate people learn to ignore.
# Allocation counts are deterministic, which is why they are what is pinned.
#
# Usage:
#   go test -run='^$' -bench=. -benchtime=10x ./... | awk -f .github/scripts/allocgate.awk

$NF == "allocs/op" {
	name = $1
	sub(/-[0-9]+$/, "", name) # strip the GOMAXPROCS suffix
	allocs[name] = $(NF - 1) + 0
	seen[name] = 1
}

# require reports whether a benchmark ran at all. A gate that silently passes
# because its subject was renamed is worse than no gate.
function require(name) {
	if (name in seen) {
		return 1
	}
	printf "  %-28s did not run\n", name
	bad = 1
	return 0
}

# flat asserts that b costs no more than a, give or take a pool miss.
function flat(a, b, slack,   x, y) {
	if (!require(a) || !require(b)) {
		return
	}
	x = allocs[a]
	y = allocs[b]
	printf "  %-28s %6d allocs/op\n", a, x
	if (y > x + slack) {
		printf "  %-28s %6d allocs/op   FAIL: %d more than %s — something allocates per row\n", b, y, y - x, a
		bad = 1
		return
	}
	printf "  %-28s %6d allocs/op   ok, flat against %s\n", b, y, a
}

# atMost asserts an absolute ceiling.
function atMost(name, budget,   v) {
	if (!require(name)) {
		return
	}
	v = allocs[name]
	if (v > budget) {
		printf "  %-28s %6d allocs/op   FAIL: budget is %d\n", name, v, budget
		bad = 1
		return
	}
	printf "  %-28s %6d allocs/op   ok, budget %d\n", name, v, budget
}

END {
	print "The allocation gate:"

	# A thousand rows and a million, drawn into a backend that does nothing, so
	# what is measured is refract's own work rather than an emitter's.
	flat("BenchmarkFrame1k", "BenchmarkFrame1M", 8)
	atMost("BenchmarkFrame1M", 128)

	# The Append forms of the decimation family exist to be allocation-free.
	# Zero is not an aspiration here, it is the reason they have that shape.
	atMost("BenchmarkLTTB", 0)
	atMost("BenchmarkMinMax", 0)

	# The streaming path, added in v0.5. A live chart appends a row and freezes
	# a view once per frame, for as long as the process runs; either of those
	# allocating is a leak with a plot attached. Both measure the steady state,
	# where the window is full and the snapshot buffers are already sized.
	atMost("BenchmarkStreamAppend", 0)
	atMost("BenchmarkStreamSnapshot", 0)

	if (bad) {
		print ""
		print "allocgate: failed. Find the culprit with"
		print "  go test -run=XXX -bench=Frame -memprofile=mem.out ."
		print "  go tool pprof -sample_index=alloc_objects -top mem.out"
		exit 1
	}
	print ""
	print "allocgate: a frame costs the same over a million rows as over a thousand"
}
