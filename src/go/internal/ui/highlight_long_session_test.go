//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Long-session regression guard for cell-highlight write-through.
//
// In a long playback session the predictor's 4096-entry sliding window
// advances past older absolute beat indices. screenshot.png shows playback
// at abs=8552; the user reports the active drum cell stops lighting up.
//
// Root cause: highlightVisual gates the highlight write on a fallback
// query Predictor.VisibleAt(row, idx). Once the sliding window's
// windowStart > idx (or windowEnd <= idx), that query returns false
// silently and the cell is never lit. The scheduler, however, has already
// decided to fire the beat — the UI just lost the side-effect.
//
// This test compresses the scenario by lowering the predictor's window
// cap to 64 subdivisions so we can reproduce the bug deterministically
// without driving abs to 4096+.

// TestCellHighlight_FiresAtAbsBeyondPredictorWindow proves the highlight
// must appear on the cell whenever the scheduler delivers a fire-event,
// regardless of where the predictor's sliding window happens to be.
func TestCellHighlight_FiresAtAbsBeyondPredictorWindow(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 600)
	g.drum.SetFollow(true)
	g.drum.SetLength(16)

	// Build a 3-node loop on row 0: 0 → 1 → 2 → 0. All NodeTypeRegular so
	// the predictor-fallback branch in highlightVisual is the one we'll
	// exercise.
	nodes := []*uiNode{
		g.tryAddNode(0, 0, model.NodeTypeRegular),
		g.tryAddNode(1, 0, model.NodeTypeRegular),
		g.tryAddNode(2, 0, model.NodeTypeRegular),
	}
	g.addEdge(nodes[0], nodes[1])
	g.addEdge(nodes[1], nodes[2])
	g.addEdge(nodes[2], nodes[0])
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.drum.Rows[0].Origin = nodes[0].ID
	g.drum.Rows[0].Node = nodes[0]
	// Pin row 0 to a registered placeholder instrument so rowIsAudible
	// returns true (matching live-session conditions where users hear
	// the kick fire on every beat).
	const instID = "longsession-kick"
	// Empty path → silent placeholder sample (no decode attempt, no log
	// noise). See sample_desktop.go RegisterAudio's empty-path branch.
	if err := audio.RegisterWAV(instID, ""); err != nil {
		t.Fatalf("RegisterWAV: %v", err)
	}
	t.Cleanup(func() {
		audio.ResetInstruments()
		audio.ResetCatalogForTest(nil)
	})
	g.drum.Rows[0].Instrument = instID
	g.drum.refreshInstruments()
	if !g.drum.IsInstrumentAvailable(instID) {
		t.Fatalf("instrument %q not available after registration", instID)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Compress the predictor window so we don't have to drive 4096+ steps
	// to reproduce the bug. With cap=64 and Ensure(5200), the predictor
	// slides to windowStart=5136, windowEnd=5200. abs=5042 sits well
	// before windowStart, which is exactly the long-session failure mode.
	g.engine.Predictor.SetWindowCap(64)
	const ensureHorizon = 5200
	const fireAbs = 5042
	g.engine.Predictor.Ensure(ensureHorizon)

	// Sanity: the chosen abs really is outside the predictor's current
	// window so the fallback path is exercised. If the predictor is
	// rebuilt during the path edit, VisibleAt may return true here — in
	// which case the test would not be exercising the long-session bug
	// and we should bail out so the assertion below doesn't get a false
	// pass.
	if g.engine.Predictor.VisibleAt(0, fireAbs) {
		t.Skip("predictor still has abs=5042 in window — cannot exercise long-session fallback path")
	}

	// Drive the Game into a "playing well past the predictor window"
	// state: drum.Offset is advanced so the cell at fireAbs is not
	// inside the per-row Steps slate either.
	g.SetPlaying(true)
	g.elapsedBeats = fireAbs
	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}
	if len(g.seqNextIdxs) != len(g.drum.Rows) {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}
	g.nextBeatIdxs[0] = fireAbs
	g.seqNextIdxs[0] = fireAbs + 1
	g.drum.Offset = fireAbs - 5000
	if g.drum.Offset < 0 {
		g.drum.Offset = 0
	}
	g.refreshDrumRow()

	// Clear any pre-existing highlights so the snapshot below is
	// unambiguous.
	g.resetHighlights()

	// Deliver the fire event exactly the way the audio thread would.
	info := g.beatInfoAtRow(0, fireAbs)
	if info.NodeType != model.NodeTypeRegular {
		t.Fatalf("expected wrapped beatInfoAtRow(0, %d) to be Regular, got %v", fireAbs, info.NodeType)
	}
	// Sanity probes to disambiguate the failure mode.
	if !g.rowIsAudible(0) {
		t.Fatalf("row 0 not audible — test setup did not produce a live-session row (muted=%v inst=%q avail=%v)",
			g.drum.Rows[0].Muted, g.drum.Rows[0].Instrument, g.drum.IsInstrumentAvailable(g.drum.Rows[0].Instrument))
	}
	g.applySequencerHighlight(0, fireAbs, info)

	// Assertion: highlightSnapshotByRow must contain an entry for
	// (row=0, idx=fireAbs). Pre-fix the predictor-fallback path
	// silently drops the write and the cell stays dark.
	snap := g.highlightSnapshotByRow(len(g.drum.Rows))
	if len(snap) == 0 || len(snap[0]) == 0 {
		t.Fatalf("no highlight recorded for row=0 abs=%d (highlight map empty after scheduler fire)", fireAbs)
	}
	found := false
	for _, h := range snap[0] {
		if h.idx == fireAbs {
			found = true
			break
		}
	}
	if !found {
		idxs := make([]int, 0, len(snap[0]))
		for _, h := range snap[0] {
			idxs = append(idxs, h.idx)
		}
		t.Fatalf("highlight missing for row=0 abs=%d (got idxs %v)", fireAbs, idxs)
	}
}
