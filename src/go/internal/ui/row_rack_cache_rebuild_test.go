//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRowRackCacheRebuildRateBounded asserts that during steady-state
// playback (no row add/delete, no scroll, no mute/solo edits), the
// RowRackZone controls cache is NOT rebuilt every frame. Each rebuild
// allocates ~30 vector.Path objects per visible row × N rows via the icon
// renderer (icon_renderer.go:88) — that is the call stack the production
// OOM landed in (RowRackZone.drawRowControlsToCache → drawVolIconOff →
// DrawIcon → iconStrokeArc → vector.Path tessellation).
//
// If this test fails the cache is being marked dirty every frame somehow,
// which means every Draw burns through the per-rebuild path allocations.
// On WASM with real vector.Path that is enough by itself to drive the
// heap to the 2 GB linear-memory ceiling within minutes.
func TestRowRackCacheRebuildRateBounded(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(string, float64, ...float64) {})

	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdiv 16: %v", err)
	}
	g.drum.SetBPM(240)

	const rows = 6
	buildSoakScene(t, g, rows, 8)

	screen := ebiten.NewImage(1280, 720)
	g.SetPlaying(true)

	// Warmup: let layout settle, demo build complete, parity gen stabilize.
	const warmupFrames = 60
	for i := 0; i < warmupFrames; i++ {
		_ = g.Update()
		g.Draw(screen)
	}

	// Steady-state measurement window. No edits, no scroll — the cache
	// must reuse cleanly across these frames.
	const measureFrames = 600
	rackBefore := g.drum.rowRackZone.controlsCacheRebuilds
	for i := 0; i < measureFrames; i++ {
		advancePlaybackByAbs(g, 1)
		g.Draw(screen)
	}
	rackAfter := g.drum.rowRackZone.controlsCacheRebuilds
	rebuilds := rackAfter - rackBefore

	// Bound: at most 5% of measured frames should rebuild. Allows for
	// occasional structural-mutation grace frames (parity gen bumps) but
	// catches per-frame thrash.
	maxAllowed := int64(measureFrames) / 20
	if maxAllowed < 1 {
		maxAllowed = 1
	}
	if rebuilds > maxAllowed {
		z := g.drum.rowRackZone
		t.Errorf("RowRackZone.controlsCache rebuilt %d times across %d steady-state playback frames "+
			"(want ≤ %d). Each rebuild allocates ~30 vector.Path objects per visible row × %d rows "+
			"via icon_renderer.go:88; this is the production OOM stack.",
			rebuilds, measureFrames, maxAllowed, rows)
		t.Logf("invalidation reason counts (controlsCacheValid bailed because…):")
		t.Logf("  controlsCache == nil:                  %d", z.cacheInvalidNil)
		t.Logf("  controlsCacheDirty == true:            %d", z.cacheInvalidDirty)
		t.Logf("  controlsCacheRowOff != z.rowOffset:    %d", z.cacheInvalidRowOff)
		t.Logf("  controlsCacheVis != VisibleRows():     %d", z.cacheInvalidVis)
		t.Logf("  controlsCacheRect.Empty():             %d", z.cacheInvalidEmptyRect)
		t.Logf("  muteBtn.pressed != rows[i].Muted:      %d", z.cacheInvalidMuteDesync)
		t.Logf("  soloBtn.pressed != rows[i].Solo:       %d", z.cacheInvalidSoloDesync)
		t.Logf("RowRackZone.Layout invocations:        %d", z.layoutInvocations)
		t.Logf("  ↳ because NeedsLayout()==true:      %d", z.layoutFromNeedsLayout)
		t.Logf("  ↳ because rect changed:             %d", z.layoutFromRectChange)
		t.Logf("RowRackZone.repositionEntries invocations: %d", z.repositionInvocations)
		t.FailNow()
	}
}
