//go:build !race

package refract_test

import (
	"testing"

	"github.com/timzifer/refract"
	"github.com/timzifer/refract/internal/irtest"
)

// The allocation gate.
//
// It is built out under the race detector, which allocates on refract's
// behalf: counting allocations while something else is adding them measures
// that instead. The race job and this one are separate CI jobs, so the gate is
// still checked on every commit.

// allocsPerFrame is the average number of allocations one steady-state render
// makes, into a backend that does nothing — so what is measured is refract's
// own work rather than an emitter's.
//
// [testing.AllocsPerRun] measures on one processor, which is what keeps the
// count reproducible: a goroutine that migrated between processors would leave
// its pooled scratch in the old one's private slot and be charged for refilling
// it. The benchmark half of the gate pins itself the same way — see onOnePGate
// in alloc_test.go, which is a note about why this half never needed one.
func allocsPerFrame(t *testing.T, p *refract.Plot) float64 {
	t.Helper()
	target := irtest.NullTarget()
	// One render first: the scratch pools are empty on the first frame by
	// definition, and a gate on the first frame would be a gate on start-up.
	if err := p.Render(target); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return testing.AllocsPerRun(20, func() {
		if err := p.Render(target); err != nil {
			t.Fatalf("Render: %v", err)
		}
	})
}

// The allocation gate.
//
// The claim refract makes is not that a render allocates nothing — a frame
// still builds tick labels, a legend and a layout, and those are real. The
// claim is that none of it is *per point*: a thousand rows and a million rows
// cost the same, because everything sized by the data comes from a pool and is
// handed back. That is what makes a chart redrawn every frame over a live
// series affordable, and it is what this test exists to stop anyone undoing.
//
// If this fails, something on the data path started allocating per row again.
// Find it with:
//
//	go test -run=XXX -bench=Frame -memprofile=mem.out .
//	go tool pprof -sample_index=alloc_objects -top mem.out
func TestARenderDoesNotAllocatePerPoint(t *testing.T) {
	small := allocsPerFrame(t, signal(1_000))
	large := allocsPerFrame(t, signal(1_000_000))

	// A thousand times the data, and the same handful of allocations. The
	// slack absorbs a pool miss, not a per-row cost: a per-row cost would show
	// up here as hundreds of thousands.
	const slack = 8
	if large > small+slack {
		t.Errorf("1M rows allocate %.0f times per frame against %.0f for 1k rows: "+
			"something on the data path is allocating per row", large, small)
	}
}

// The absolute budget. It is generous on purpose — the number that matters is
// the one above — but a frame that suddenly needs ten times as many
// allocations has grown something worth looking at.
func TestAFrameStaysWithinItsAllocationBudget(t *testing.T) {
	const budget = 128
	if got := allocsPerFrame(t, signal(250_000)); got > budget {
		t.Errorf("a steady-state frame allocates %.0f times, budget %d", got, budget)
	}
}

// Faceting copies rows into per-panel sources, so it allocates once per panel
// per column — but still nothing per row after the first frame.
func TestAFacetedRenderDoesNotAllocatePerPoint(t *testing.T) {
	small := allocsPerFrame(t, facetedSignal(3, 500))
	large := allocsPerFrame(t, facetedSignal(3, 200_000))
	const slack = 16
	if large > small+slack {
		t.Errorf("200k rows per panel allocate %.0f times per frame against %.0f for 500: "+
			"faceting is allocating per row", large, small)
	}
}

// A stacked layer is the third data path, and the one with the most
// bookkeeping: the groups are indexed and the adjustment is derived on every
// Train, because the axis has to describe the totals. All of it works out of
// buffers the layer keeps, so a long table costs the same handful of
// allocations as a short one.
func TestAStackedLayerDoesNotAllocatePerPoint(t *testing.T) {
	small := allocsPerFrame(t, stackedSeries(1_000))
	large := allocsPerFrame(t, stackedSeries(200_000))
	const slack = 8
	if large > small+slack {
		t.Errorf("200k stacked rows allocate %.0f times per frame against %.0f for 1k: "+
			"the adjustment is allocating per row", large, small)
	}
}

// A polar layer is the fourth data path, and the one v0.8 added: every mark
// goes through the coord on its way to a device point.
//
// It is measured because that call is where a per-row interface method would
// reappear — the shape that once cost a million allocations on a million-row
// column. coord.Coord.Points is the batch form that stops it, and this is the
// assertion that it is the form the geoms actually use. A polar coord does not
// decimate either, so this draws every row rather than a reduction of them.
func TestAPolarRenderDoesNotAllocatePerPoint(t *testing.T) {
	small := allocsPerFrame(t, polarSignal(1_000))
	large := allocsPerFrame(t, polarSignal(200_000))
	const slack = 8
	if large > small+slack {
		t.Errorf("200k polar marks allocate %.0f times per frame against %.0f for 1k: "+
			"the coord is allocating per point", large, small)
	}
}

// A broken-out layer collects a displacement per mark and carries it through
// the colour and group batching. All of that comes out of the scratch pool, so
// a ring of two hundred thousand slices costs what a ring of a thousand does.
func TestABrokenOutLayerDoesNotAllocatePerPoint(t *testing.T) {
	small := allocsPerFrame(t, brokenRing(1_000))
	large := allocsPerFrame(t, brokenRing(200_000))
	const slack = 8
	if large > small+slack {
		t.Errorf("200k broken-out slices allocate %.0f times per frame against %.0f for 1k: "+
			"the break-out is allocating per mark", large, small)
	}
}

// A layer coloured from a column is the other data path: one colour per mark,
// batched into one drawing call per distinct colour. Its buffers are pooled
// too, and this is the assertion that they stay that way.
func TestAColouredLayerDoesNotAllocatePerPoint(t *testing.T) {
	small := allocsPerFrame(t, colouredCloud(1_000))
	large := allocsPerFrame(t, colouredCloud(200_000))
	const slack = 8
	if large > small+slack {
		t.Errorf("200k coloured marks allocate %.0f times per frame against %.0f for 1k", large, small)
	}
}
