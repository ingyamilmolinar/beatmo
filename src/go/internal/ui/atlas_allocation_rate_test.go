//go:build test

// Reassigning a cache image every beat orphans atlas slots that WASM's bursty finalizers free late; tens of minutes
// of playback degenerated the packer and ran out of memory. Caches reuse their image unless the size changes,
// and these fail within seconds if per-beat allocation returns.

package ui

import (
	"sync/atomic"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Many beat boundaries must not allocate images in proportion.
func TestTimelineBarDrawDoesNotAllocPerBeat(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
	g.drum.SetBPM(120)
	buildSoakScene(t, g, 2, 4)

	scratch := ebiten.NewImage(1280, 720)

	// Until every cache reaches its steady high-water mark.
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

	// The cache key changes whenever floor(winStart) ticks, so the playhead is driven across many integer boundaries.
	const transitions = 200
	for i := 33; i < 33+transitions; i++ {
		// Past at least one integer boundary every couple of frames.
		setPlayStartForAbs(g, i*10)
		g.engine.Predictor.Ensure(i*10 + 1)
		_ = g.Update()
		g.Draw(scratch)
	}

	after := atomic.LoadInt64(&imagesAllocatedTotal)
	delta := after - baseline
	t.Logf("imagesAllocatedTotal delta after %d simulated beat boundaries = %d "+
		"(baseline=%d, after=%d)", transitions, delta, baseline, after)

	// Only a resize may allocate, which steady playback never does; the slack covers first-frame rebuilds.
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

// Per tag, so the leaking cache is named even when the total is borderline.
func TestPerTagAllocRateBoundedDuringPlayback(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})
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

	// Row and layout caches may legitimately rebuild as the window scrolls; the timeline bar cache must not grow per beat.
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

// nil when the build has none.
func findTimelineZone(t *testing.T, g *Game) *TimelineZone {
	t.Helper()
	if g == nil || g.drum == nil {
		return nil
	}
	return g.drum.timelineZone
}

// silence unused-import warning when analyzer subpackage isn't needed.
var _ analyzer.State
