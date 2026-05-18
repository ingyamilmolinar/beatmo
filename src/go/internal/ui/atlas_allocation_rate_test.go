//go:build test

// Regression tests for the Ebiten texture-atlas allocation rate.
//
// User-reported OOM: ~41 min into a BPM 120 playback session, the
// browser crashed with:
//   runtime: out of memory: cannot allocate 4194304-byte block (2130378752 in use)
//   ...
//   github.com/hajimehoshi/ebiten/v2/internal/packing.alloc(...)
//   github.com/hajimehoshi/ebiten/v2/internal/packing.alloc(...)
//   ...125+ frames elided...
//   github.com/ingyamilmolinar/beatmo/internal/ui.(*TimelineZone).drawTimelineBar
//
// Root cause: TimelineZone.drawTimelineBar reassigned z.TlCache to a
// freshly-allocated *ebiten.Image every time the playhead crossed an
// integer beat boundary (≈ 2×/sec at BPM 120). The previous image was
// orphaned with no Deallocate(); Ebiten's BSP atlas packer retained
// every slot until GC finalizers fired, which on WASM is bursty and
// unreliable. After tens of minutes the atlas's split tree degenerated
// to 125+ depth, allocations exhausted the heap, and the runtime
// failed to grow the goroutine stack.
//
// Fix:
//   1. Reuse the same image when only the per-beat content key
//      changes (Clear() + redraw); only allocate fresh on dim change.
//   2. Defensive Deallocate() at every cache reassign site.
//
// These tests pin both invariants. A regression that re-introduces
// per-beat alloc fails them within seconds.

package ui

import (
	"sync/atomic"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// TestTimelineBarDrawDoesNotAllocPerBeat drives TimelineZone.Draw
// across many simulated beat-boundary transitions and asserts the
// cumulative imagesAllocatedTotal counter does not increase
// proportionally. The pre-fix code allocated ~1 image per integer-beat
// boundary, so 1000 simulated beats produced 1000+ allocations; the
// post-fix code reuses the image and only redraws content (zero new
// allocations once warmed up).
func TestTimelineBarDrawDoesNotAllocPerBeat(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.drum.SetBPM(120)
	buildSoakScene(t, g, 2, 4)

	scratch := ebiten.NewImage(1280, 720)

	// Warm-up: 32 frames of playback so all caches reach their
	// steady-state allocation high-water mark.
	g.SetPlaying(true)
	for i := 1; i <= 32; i++ {
		setPlayStartForAbs(g, i)
		g.engine.Predictor.Ensure(i + 1)
		_ = g.Update()
		g.Draw(scratch)
	}

	// Locate the TimelineZone — it lives under the drum view tree.
	tlZone := findTimelineZone(t, g)
	if tlZone == nil {
		t.Skip("no TimelineZone in this build; can't exercise the timeline draw path")
	}

	// Capture the cumulative-alloc baseline AFTER warm-up.
	baseline := atomic.LoadInt64(&imagesAllocatedTotal)

	// Drive the playhead across 200 simulated integer-beat boundaries.
	// At BPM 120 each beat is 0.5 s; the cache key in
	// timeline_zone.go:362-372 transitions every time
	// int(math.Floor(winStart)) ticks forward, so we need to advance
	// elapsedBeats past each integer to exercise the reassignment
	// path. The TimelineZone reads elapsedBeats from a field set
	// inside Update(); the simplest way to force transitions is to
	// drive the playhead via setPlayStartForAbs and let
	// computeTimelineBeats run.
	const transitions = 200
	for i := 33; i < 33+transitions; i++ {
		// Advance the playhead by ~1 beat-equivalent each iteration
		// so the timeline-bar window slides past at least one integer
		// beat boundary every couple of frames. The real bug
		// reproduces at any rate that crosses int boundaries; we
		// stress the same path here.
		setPlayStartForAbs(g, i*10)
		g.engine.Predictor.Ensure(i*10 + 1)
		_ = g.Update()
		g.Draw(scratch)
	}

	after := atomic.LoadInt64(&imagesAllocatedTotal)
	delta := after - baseline
	t.Logf("imagesAllocatedTotal delta after %d simulated beat boundaries = %d "+
		"(baseline=%d, after=%d)", transitions, delta, baseline, after)

	// Pre-fix the delta was ≥ transitions (one TlCache alloc per
	// integer-beat key change). Post-fix the count should stay near
	// zero — the only allowed sources are layout-driven cache
	// rebuilds that legitimately need a different size, which won't
	// happen during steady-state playback. A small slack budget
	// (50 allocations across 200 transitions) covers any incidental
	// rebuild on the first few frames after warm-up.
	const maxAllocsAcrossTransitions = 50
	if delta > maxAllocsAcrossTransitions {
		t.Errorf("atlas alloc rate during steady-state playback = %d allocations "+
			"across %d beat-boundary transitions (max %d) — see "+
			"internal/ui/timeline_zone.go drawTimelineBar(): the TlCache "+
			"reassignment must reuse the existing image and only Clear()+"+
			"redraw content on per-beat key changes, not allocate fresh",
			delta, transitions, maxAllocsAcrossTransitions)
	}
}

// TestPerTagAllocRateBoundedDuringPlayback inspects the per-tag
// allocation counters to identify which cache tag (if any) is the
// regressor. Asserts that no single tag's count grows by more than
// the warm-up baseline plus a small slack. This is the diagnostic
// version of the test above — it pinpoints which cache is leaking
// even when the cumulative count is borderline.
func TestPerTagAllocRateBoundedDuringPlayback(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.drum.SetBPM(120)
	buildSoakScene(t, g, 2, 4)

	scratch := ebiten.NewImage(1280, 720)
	g.SetPlaying(true)

	// Warm-up.
	for i := 1; i <= 32; i++ {
		setPlayStartForAbs(g, i)
		g.engine.Predictor.Ensure(i + 1)
		_ = g.Update()
		g.Draw(scratch)
	}

	// Snapshot per-tag counts post-warmup.
	baselineByTag := make(map[string]int64)
	imagesByTagMu.Lock()
	for k, v := range imagesByTag {
		baselineByTag[k] = v
	}
	imagesByTagMu.Unlock()

	// Drive 200 frames advancing the playhead.
	const frames = 200
	for i := 33; i < 33+frames; i++ {
		setPlayStartForAbs(g, i*10)
		g.engine.Predictor.Ensure(i*10 + 1)
		_ = g.Update()
		g.Draw(scratch)
	}

	// Inspect per-tag deltas.
	imagesByTagMu.Lock()
	defer imagesByTagMu.Unlock()

	// Tags allowed to grow during the test window:
	//   - rowSprite / rowsLayer / rowCacheScratch / rowsLayerScratch:
	//     row content/offset shifts on every advanced abs.
	//   - bgCache, controlsCache, transportZone.toolbarCache,
	//     timelineZone.hlSpriteReg/Mute, gridTile, gridCache,
	//     edgeCache, nodeLayer: layout-driven, but Update() can
	//     legitimately rebuild small caches when the playhead
	//     scrolls the row window.
	// The CRITICAL invariant from the user OOM:
	//     timelineZone.TlCache must NOT grow per beat boundary.
	const tlCacheTag = "timelineZone.TlCache"
	got := imagesByTag[tlCacheTag] - baselineByTag[tlCacheTag]
	const tlCacheMaxRebuilds = 5
	if got > tlCacheMaxRebuilds {
		t.Errorf("%q allocation count grew by %d across %d frames "+
			"(max %d during steady-state playback). This is the OOM "+
			"vector reported by the user — see timeline_zone.go "+
			"drawTimelineBar(); the TlCache must reuse the same image "+
			"and only Clear()+redraw on per-beat key changes",
			tlCacheTag, got, frames, tlCacheMaxRebuilds)
	}
	t.Logf("post-fix per-tag allocation deltas across %d playback frames:", frames)
	for k, v := range imagesByTag {
		delta := v - baselineByTag[k]
		if delta > 0 {
			t.Logf("  %s += %d", k, delta)
		}
	}
}

// findTimelineZone digs the TimelineZone out of the drum view's
// rendering tree so we can drive its Draw() directly. Returns nil if
// the build doesn't have one wired up.
func findTimelineZone(t *testing.T, g *Game) *TimelineZone {
	t.Helper()
	if g == nil || g.drum == nil {
		return nil
	}
	return g.drum.timelineZone
}

// silence unused-import warning when analyzer subpackage isn't needed.
var _ analyzer.State
