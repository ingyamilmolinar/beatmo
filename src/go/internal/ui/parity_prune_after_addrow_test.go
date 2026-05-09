//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestParityPruneRunsAfterAddRow proves a regression-class bug: after
// g.drum.AddRow() is consumed by Update(), g.pendingStartRow is set to the
// row index and only cleared by an origin-selection gesture (a click on a
// node to set it as start). For programmatic flows that assign origins
// directly via Rows[i].Origin = ... — including imports and tests — this
// field stays >=0 indefinitely, which causes parityScan() to early-return
// at game_parity_diff_scan.go:137. Because parityPrune(audioStart) is
// invoked at the end of parityScan, it never runs, and
// g.paritySeqDecisions grows unbounded with one entry per (row, abs).
//
// This is the upstream amplifier for the production WASM OOM whose stack
// lands in RowRackZone.drawRowControlsToCache: the heap fills with parity
// state until the next vector.Path tessellation cannot allocate.
//
// The test sets up the same scene the soak suite uses (6 rows × 8 nodes),
// runs sustained playback, and asserts parityPrune ran at least once with
// a non-zero minAbs. Once the underlying bug is fixed (either by clearing
// pendingStartRow in non-click origin assignments, or by relaxing the
// parityScan guard so it doesn't gate prune), this test flips green.
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
	g.SetPlayFunc(func(string, float64, ...float64) {})

	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdiv 16: %v", err)
	}
	g.drum.SetBPM(240)

	const rows = 6
	buildSoakScene(t, g, rows, 8)

	// One Update tick consumes drum.AddedRows queue → pendingStartRow=N.
	// This is the production regression scenario: a flow that assigns row
	// origins programmatically (import / scene build) leaves pendingStartRow
	// dangling because no click-to-set-origin gesture clears it.
	_ = g.Update()
	if g.pendingStartRow < 0 {
		t.Fatal("test precondition: pendingStartRow should be set after Update " +
			"consumes drum.AddedRows (reproduces the production scenario)")
	}

	screen := ebiten.NewImage(1280, 720)
	g.SetPlaying(true)

	// Drive playback long enough that recordSeqDecision has populated
	// thousands of entries; if pruning works, paritySeqDecisions stays
	// bounded and parityPruneCallsForTest > 0.
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
