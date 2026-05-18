//go:build test

package ui

import (
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// TestLongSessionHeapBoundedAtOneMillionAbs is the end-to-end canary for
// the OOM-prevention plan. It drives synthetic playback to abs = 1 000 000
// (~5× the production OOM horizon at 7h) and asserts both heap ceilings
// AND historical-replay correctness:
//
//  1. HeapAlloc stays under 200 MB after a 1 M-abs run.
//  2. TotalAlloc delta stays under 600 MB (catches GC-reclaimable churn).
//  3. Pre-seeded playback commits at five abs anchors (1, 1024, 8192,
//     65 536, 500 000) — all evicted from the live timeline immutables
//     sidecar by the time abs reaches 1 M — still resolve via the
//     Phase 3 cold archive.
//
// The canary is the single end-to-end check that Phases 1-5 collectively
// prevent the user's 7 h OOM. -short skips it on fast paths; nightly runs
// without -short.
func TestLongSessionHeapBoundedAtOneMillionAbs(t *testing.T) {
	if testing.Short() {
		t.Skip("long canary skipped under -short; run via nightly")
	}
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdivisions: %v", err)
	}
	g.drum.SetBPM(240)
	buildSoakScene(t, g, 6 /* rows */, 8 /* nodes per row */)
	g.SetPlaying(true)
	advanceFrames(g, 5)

	// Pre-seed playback commits at sample anchors via the timeline service
	// so scroll-back assertions don't depend on whether the synthetic
	// playback path actually fires nodes at those abs.
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
		// Drive predictor.Ensure(i+1) directly so the windowing path is
		// exercised on every step — Update is heavier and only needs to run
		// periodically to catch integration regressions in refresh +
		// reconcileFrozen + highlight retention.
		g.engine.Predictor.Ensure(i + 1)
		if i%4096 == 0 {
			_ = g.Update()
		}
	}

	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&post)

	// Delta, not absolute: this test runs in a shared test binary where
	// prior tests (TestSoakHeapBounded_*, TestParityDecisionsBounded*) can
	// leave 300+ MB of unreclaimable residue. The contract is "this loop
	// doesn't accumulate state," which delta captures and absolute does not.
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

	// Historical replay — every pre-seeded anchor must still resolve. By the
	// time abs reaches 1M, all five anchors are below (latestAbs −
	// immutablesPerRowMax = 1024) and have been migrated from the live
	// sidecar into the cold archive (Phase 3). If any miss, the archive
	// migration or read-tier integration is broken.
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
