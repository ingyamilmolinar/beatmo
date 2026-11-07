//go:build test

package ui

import (
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// Drives playback to abs 1 000 000, far past the horizon where a long session ran out of memory: the heap
// and allocation stay bounded, and commits evicted from the live tier still resolve from the cold archive.
// -short skips it.
func TestLongSessionHeapBoundedAtOneMillionAbs(t *testing.T) {
	if testing.Short() {
		t.Skip("long canary skipped under -short; run via nightly")
	}
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdivisions: %v", err)
	}
	g.drum.SetBPM(240)
	buildSoakScene(t, g, 6 /* rows */, 8 /* nodes per row */)
	g.SetPlaying(true)
	advanceFrames(g, 5)

	// Seeded directly, so the scroll-back checks don't depend on whether playback fires a node at those abs.
	sampleAnchors := []int{1, 1024, 8192, 65_536, 500_000}
	const windowLen = 64
	for _, anchor := range sampleAnchors {
		g.timelineService().RecordCommitKind(0, anchor, true, model.NodeTypeRegular,
			timeline.CommitKindPlayback, windowLen, windowLen)
	}

	const N = 1_000_000

	var pre, post runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&pre)

	for i := 1; i <= N; i++ {
		setPlayStartForAbs(g, i)
		// Ensure every step exercises the windowing; the heavier Update runs only periodically for integration.
		g.engine.Predictor.Ensure(i + 1)
		if i%4096 == 0 {
			_ = g.Update()
		}
	}

	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&post)

	// A delta, not an absolute: earlier tests in the shared binary leave unreclaimable residue, and the contract is
	// that this loop accumulates nothing.
	const heapAllocCap = 200 << 20
	if delta := post.HeapAlloc - pre.HeapAlloc; delta > heapAllocCap {
		t.Fatalf("HeapAlloc delta=%d MB after 1M abs (cap %d MB) — runtime state is leaking",
			delta>>20, heapAllocCap>>20)
	}
	const totalAllocDeltaCap = 600 << 20
	if delta := post.TotalAlloc - pre.TotalAlloc; delta > totalAllocDeltaCap {
		t.Fatalf("TotalAlloc delta=%d MB (cap %d MB) — leak slope still present",
			delta>>20, totalAllocDeltaCap>>20)
	}

	// By abs 1M every anchor is far below the live tier and has moved into the cold archive; a miss means the
	// migration or the read tier is broken.
	for _, anchor := range sampleAnchors {
		_, _, kind, ok := g.timelineCommittedWithKind(0, anchor)
		if !ok {
			t.Errorf("scroll-back miss row=0 abs=%d (archive should serve)", anchor)
			continue
		}
		if kind != timeline.CommitKindPlayback {
			t.Errorf("scroll-back row=0 abs=%d kind=%v want Playback (archive lost provenance)",
				anchor, kind)
		}
	}
}
