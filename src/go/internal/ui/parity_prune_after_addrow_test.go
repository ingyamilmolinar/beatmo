//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// pendingStartRow is cleared only by a click-to-set-origin; a programmatic origin (import, scene build) left it
// set, parityScan returned early and parityPrune never ran, growing the decisions map without bound.
func TestParityPruneRunsAfterAddRow(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	prevWatch := parityWatchDefault
	prevFatal := parityFatalEnabled.Load()
	parityWatchDefault = parityWatchLog
	parityFatalEnabled.Store(false)
	t.Cleanup(func() {
		parityWatchDefault = prevWatch
		parityFatalEnabled.Store(prevFatal)
	})

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.parityWatch = parityWatchLog
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})

	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdiv 16: %v", err)
	}
	g.drum.SetBPM(240)

	const rows = 6
	buildSoakScene(t, g, rows, 8)

	_ = g.Update()
	if g.pendingStartRow < 0 {
		t.Fatal("test precondition: pendingStartRow should be set after Update " +
			"consumes drum.AddedRows (reproduces the production scenario)")
	}

	screen := ebiten.NewImage(1280, 720)
	g.SetPlaying(true)

	for i := 0; i < 2000; i++ {
		advancePlaybackByAbs(g, 1)
		g.Draw(screen)
	}

	if g.parityPruneCallsForTest == 0 {
		t.Errorf("parityPrune was never called across 2000 abs steps; pendingStartRow=%d "+
			"is gating parityScan (and therefore parityPrune) at game_parity_diff_scan.go:137. "+
			"Fix: either clear pendingStartRow when an origin is assigned directly "+
			"(without the click-to-set-origin gesture), or move parityPrune out of the "+
			"pendingStartRow-gated block so retention runs even during origin selection.",
			g.pendingStartRow)
	}
	if g.parityPruneMaxMinAbsForTest == 0 && g.parityPruneCallsForTest > 0 {
		t.Errorf("parityPrune ran %d times but always with minAbs=0; growth retention "+
			"requires rising minAbs as playback advances",
			g.parityPruneCallsForTest)
	}
}
