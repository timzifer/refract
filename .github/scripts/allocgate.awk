# allocgate.awk — the benchmark gate.
#
# Pipe `go test -bench` output through it. It enforces the one property the
# allocation pass promises: what a frame costs in allocations does not grow
# with the data it draws, so a chart redrawn every frame over a live series is
# affordable.
#
# Times are deliberately not checked. A shared CI runner has nothing reliable
# to say about them, and a gate that flakes is a gate people learn to ignore.
# Allocation counts are deterministic, which is why they are what is pinned —
# and they are deterministic because the benchmarks behind them pin their
# measurement to one processor. See onOnePGate in alloc_test.go: without it a
# goroutine that migrates between processors leaves its pooled scratch in the
# old one's private slot, and the frame that refills it is charged sixty
# allocations nobody wrote. That was this file's flake, and it is why three
# comparisons below used to be budgets.
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

	# Groups and the position adjustments, added in v0.7. A grouped layer
	# indexes its rows and derives its stack on every Train, out of buffers it
	# keeps between frames — so a long table costs what a short one costs. See
	# docs/adr/0019-position-adjustments.md.
	#
	# This pair was a budget until the benchmarks were pinned to one processor,
	# because the large side came out at 91, 98 or 106 for the same code. It
	# comes out at exactly the small side's count now, so it is compared like
	# every other pair — a hundred times the rows for no allocations at all is
	# the claim, and the claim is worth stating exactly. The ceiling stays
	# beside it: it is what catches a frame that grows tenfold without growing
	# per row.
	flat("BenchmarkStacked1k", "BenchmarkStacked100k", 8)
	atMost("BenchmarkStacked100k", 128)

	# The polar path, added in v0.8. A coord sits between every mapped pair and
	# the device point it becomes, and a polar one does not decimate — so this
	# draws every row through coord.Coord.Points. The batch form is why that is
	# flat; a per-row interface call is the shape that would break it, and it
	# has broken exactly this way once before. See docs/adr/0018.
	flat("BenchmarkPolar1k", "BenchmarkPolar100k", 8)

	# The v0.8 sugar: a donut whose slices name their own radii and are broken
	# out of the ring per row. The displacement is collected per mark and
	# carried through the colour and group batching, all of it out of the
	# scratch pool.
	#
	# The slack is wider than the others by four, and the four are tick labels:
	# a ring of a hundred thousand slices has a longer angular domain than a
	# ring of a thousand, so its axis has more ticks and each label is a string.
	# That is a cost per *tick*, which is what an axis is allowed to cost.
	flat("BenchmarkBrokenRing1k", "BenchmarkBrokenRing100k", 12)

	# Row identity, added after v0.5. Tracking which source row is behind each
	# mark is opt-in, and what it is opt-in *for* is memory per mark — not
	# per-frame allocations. If that stops being true it is a buffer that
	# escaped the pool, which is the same bug the gate above exists for.
	flat("BenchmarkWatchedFrame", "BenchmarkWatchedFrameRows", 2)

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
