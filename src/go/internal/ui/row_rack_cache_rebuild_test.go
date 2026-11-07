//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Steady playback must not rebuild the controls cache every frame: each rebuild tessellates ~30 icon paths
// per row, which on WASM reaches the 2 GB heap ceiling within minutes.
func TestRowRackCacheRebuildRateBounded(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})

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

	const measureFrames = 600
	rackBefore := g.drum.rowRackZone.controlsCacheRebuilds
	for i := 0; i < measureFrames; i++ {
		advancePlaybackByAbs(g, 1)
		g.Draw(screen)
	}
	rackAfter := g.drum.rowRackZone.controlsCacheRebuilds
	rebuilds := rackAfter - rackBefore

	// 5% allows structural grace frames but catches per-frame thrash.
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
