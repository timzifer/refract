# benchtable.awk — the benchmark suite as a table.
#
# Pipe `go test -bench=. -benchmem` output through it and it writes a Markdown
# table per package: one row per benchmark, with the time, the bytes and the
# allocations per operation, and a header naming the machine the numbers came
# from. It is what docs/benchmarks.md carries and what the CI job writes into
# its summary, so the two can never be formatted differently.
#
# It reports and does not judge. The gate is allocgate.awk; this file exists so
# that the numbers the gate reads — and the ones it deliberately does not, the
# times — are published rather than buried in a log.
#
# Usage:
#   go test -run='^$' -bench=. -benchmem ./... | awk -f .github/scripts/benchtable.awk

# The header lines go test prints once per package. The machine is the same
# for every package in one run, so it is printed once, from the first.
/^goos: /   { if (!goos)   goos = $2 }
/^goarch: / { if (!goarch) goarch = $2 }
/^cpu: /    { if (!cpu) { cpu = $0; sub(/^cpu: /, "", cpu) } }
/^pkg: /    { pkg = $2; if (!(pkg in order)) { order[pkg] = ++npkg; pkgs[npkg] = pkg } }

# A result line: name, iterations, then value/unit pairs. Only the three
# standard units are tabulated; a custom metric a benchmark reports is left to
# the raw output.
/^Benchmark/ {
	name = $1
	sub(/-[0-9]+$/, "", name)
	sub(/^Benchmark/, "", name)
	ns = bytes = allocs = "—"
	for (i = 3; i < NF; i++) {
		if ($(i + 1) == "ns/op")     ns = $i + 0
		if ($(i + 1) == "B/op")      bytes = $i + 0
		if ($(i + 1) == "allocs/op") allocs = $i + 0
	}
	n = ++rows[pkg]
	row[pkg, n] = "| `" name "` | " time(ns) " | " size(bytes) " | " allocs " |"
}

# size renders bytes the same way.
function size(b) {
	if (b == "—")         return b
	if (b >= 1048576)     return sprintf("%.1f MB", b / 1048576)
	if (b >= 1024)        return sprintf("%.1f KB", b / 1024)
	return sprintf("%d B", b)
}

# time renders nanoseconds in the unit a reader would pick.
function time(ns) {
	if (ns == "—")        return ns
	if (ns >= 1e9)        return sprintf("%.2f s", ns / 1e9)
	if (ns >= 1e6)        return sprintf("%.2f ms", ns / 1e6)
	if (ns >= 1e3)        return sprintf("%.2f µs", ns / 1e3)
	return sprintf("%.0f ns", ns)
}

END {
	if (!npkg) {
		print "no benchmark output"
		exit 1
	}
	printf "Measured on %s/%s, %s.\n", goos, goarch, cpu
	for (p = 1; p <= npkg; p++) {
		pkg = pkgs[p]
		if (!rows[pkg]) continue
		print ""
		printf "**`%s`**\n", pkg
		print ""
		print "| Benchmark | time/op | memory/op | allocs/op |"
		print "|---|---:|---:|---:|"
		for (i = 1; i <= rows[pkg]; i++) print row[pkg, i]
	}
}
