# The benchmark suite

Every benchmark in this repository, what each one measures, which of its
numbers CI gates, and the latest results. The suite is public in the sense
that matters: it runs on every commit, its raw output is attached to the
`Benchmarks and the allocation gate` job as the `benchmarks` artifact and
summarised in the job's page, and anyone can reproduce the table below with
one command.

```sh
go test -run='^$' -bench=. -benchmem ./... | awk -f .github/scripts/benchtable.awk
```

The nested modules with benchmarks — `backend/gg`, `backend/window` and
`arrow/v18` — are run the same way from their own directories, and CI
concatenates the four outputs before the scripts read them.

## What is gated and what is only published

The one performance claim this project makes is a number rather than a
picture: **what a frame costs in allocations does not grow with the data it
draws**, so a chart redrawn every frame over a live series is affordable
(CONCEPT §11). Allocation counts are deterministic, and the benchmarks behind
them pin their measurement to one processor so that they stay deterministic —
`onOnePGate` in `alloc_test.go`, and the reason is in
[CONTRIBUTING](../CONTRIBUTING.md#benchmarks-and-the-allocation-gate). Those
counts are what `.github/scripts/allocgate.awk` enforces.

Times are published and never gated. A shared CI runner has nothing reliable
to say about them, and a gate that flakes is a gate people learn to ignore.
The times in the table are one run on one machine, named at the top of it;
compare two runs on one machine with `benchstat`, and treat a comparison
across machines as a comparison of machines.

## The benchmarks

**A frame.** `Frame1k`, `Frame100k` and `Frame1M` render one line chart of
that many rows into a backend that does nothing, so what is measured is
refract's own work — training, decimation, lowering — and not an emitter's.
The gate asserts `Frame1M` allocates no more than `Frame1k` plus a pool miss,
and no more than 128 in absolute terms. The time grows with the rows, as it
must; the allocation count does not, which is the claim.

**The paths that could break it.** Each of these is a feature that had a way
to allocate per row, measured as a pair at a thousand and a hundred thousand
rows and gated flat. `Stacked` is a grouped layer with a position adjustment,
derived on every `Train` ([ADR 0019](adr/0019-position-adjustments.md)).
`Polar` sends every row through `coord.Coord.Points`, which is why that method
takes a batch ([ADR 0018](adr/0018-coordinate-systems.md)). `BrokenRing` is a
donut whose slices carry their own radii and are broken out per row
([ADR 0026](adr/0026-breaking-a-mark-out.md)). `Bubbles` is a sized layer,
the one drawing path that sorts per frame
([ADR 0027](adr/0027-size-channel-and-the-guide-column.md)). Their slack
against the small side is eight, or twelve where the large frame is big enough
to provoke a pool miss; `allocgate.awk` says which and why.

**Row identity.** `WatchedFrame` and `WatchedFrameRows` are the same live
chart redrawn with and without `Live.TrackRows`. Tracking the source row
behind every mark costs memory per mark and is opt-in for that reason; it must
not cost allocations per frame, and the pair is gated flat
([ADR 0015](adr/0015-hit-testing.md)).

**Streaming.** `StreamAppend` and `StreamSnapshot` measure the steady state of
a live series: a window that is full, and snapshot buffers that are already
sized. Both are gated at zero allocations. Either one allocating would be a
leak with a plot attached ([ADR 0016](adr/0016-streaming-and-damage.md)).

**Decimation.** `LTTB` and `MinMax` are the `Append` forms of the two
reducers, over a million rows, and are gated at zero allocations — that is
the reason those forms exist ([ADR 0011](adr/0011-decimation.md)). `Bin` is
the density raster's binner over the same data and is published but not
gated: it allocates its raster once per size, which amortises to nothing.

**Parallel panels.** `FacetParallel` against `FacetSerial` is a nine-panel
facet over fifty thousand rows a panel, rendered on one goroutine and on
several; `PanelsSerial` against `PanelsParallel` in `render` is the same
comparison over eight small panels. They are the one place a *time* is the
point, and they are deliberately not pinned to one processor and not gated:
what they show is that the parallel path is worth having for large panels
and costs a little for small ones, and that is a reading rather than a rule
([ADR 0012](adr/0012-parallel-panels.md)).

**Arrow.** `BorrowedFloat64Column` reads a million-row `float64` column with
no nulls, which is the case the adapter borrows outright; `WidenedInt64Column`
reads an `int64` column of the same length, which it has to convert. The gap
between them is the cost of not being in the one format the two libraries
agree about ([ADR 0013](adr/0013-arrow-adapter.md)). The borrowed column's
handful of allocations are the record's own construction, once.

## Results

The table is what `benchtable.awk` wrote from one run of the command above,
on the machine it names. It is a snapshot rather than a golden file: the
allocation columns are the ones the gate holds steady, and the time column is
that machine's. Regenerate it with the command above and replace this section
when a release changes what a benchmark measures.

Measured on linux/amd64, Intel(R) Xeon(R) Processor @ 2.80GHz.

**`github.com/timzifer/refract`**

| Benchmark | time/op | memory/op | allocs/op |
|---|---:|---:|---:|
| `Stacked1k` | 189.96 µs | 5.0 KB | 80 |
| `Stacked100k` | 17.38 ms | 5.1 KB | 80 |
| `Polar1k` | 86.52 µs | 3.3 KB | 51 |
| `Polar100k` | 6.16 ms | 3.4 KB | 55 |
| `BrokenRing1k` | 401.88 µs | 5.4 KB | 91 |
| `BrokenRing100k` | 38.29 ms | 5.7 KB | 95 |
| `Bubbles1k` | 218.18 µs | 4.3 KB | 83 |
| `Bubbles100k` | 21.46 ms | 4.4 KB | 87 |
| `Frame1k` | 83.64 µs | 3.1 KB | 56 |
| `Frame100k` | 6.00 ms | 3.1 KB | 56 |
| `Frame1M` | 60.73 ms | 3.2 KB | 56 |
| `FacetParallel` | 93.12 ms | 14.1 MB | 605 |
| `FacetSerial` | 101.95 ms | 13.9 MB | 519 |
| `WatchedFrame` | 6.03 ms | 3.6 KB | 58 |
| `WatchedFrameRows` | 6.00 ms | 3.6 KB | 58 |

**`github.com/timzifer/refract/data`**

| Benchmark | time/op | memory/op | allocs/op |
|---|---:|---:|---:|
| `StreamAppend` | 29 ns | 0 B | 0 |
| `StreamSnapshot` | 83.54 µs | 0 B | 0 |

**`github.com/timzifer/refract/render`**

| Benchmark | time/op | memory/op | allocs/op |
|---|---:|---:|---:|
| `PanelsSerial` | 354.43 µs | 29.7 KB | 607 |
| `PanelsParallel` | 410.57 µs | 33.5 KB | 651 |

**`github.com/timzifer/refract/stat`**

| Benchmark | time/op | memory/op | allocs/op |
|---|---:|---:|---:|
| `LTTB` | 3.74 ms | 0 B | 0 |
| `MinMax` | 8.12 ms | 0 B | 0 |
| `Bin` | 15.83 ms | 21.8 KB | 0 |

**`github.com/timzifer/refract/arrow/v18`**

| Benchmark | time/op | memory/op | allocs/op |
|---|---:|---:|---:|
| `BorrowedFloat64Column` | 346 ns | 536 B | 6 |
| `WidenedInt64Column` | 8.68 ms | 7.6 MB | 7 |
