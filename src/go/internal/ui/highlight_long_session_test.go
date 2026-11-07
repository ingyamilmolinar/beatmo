//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// The scheduler fires beats the predictor's sliding window no longer covers; a highlight gated on
// Predictor.VisibleAt would leave the cell dark. A small window cap reproduces that without driving abs past 4096.

func TestCellHighlight_FiresAtAbsBeyondPredictorWindow(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 600)
	g.drum.SetFollow(true)
	g.drum.SetLength(16)

	// All regular nodes, so highlightVisual takes its predictor fallback.
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
	// A registered instrument makes the row audible, as in a live session.
	const instID = "longsession-kick"
	// An empty path registers a silent placeholder: no decode, no log noise.
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

	// A small cap slides the window past fireAbs without driving thousands of steps.
	g.engine.Predictor.SetWindowCap(64)
	const ensureHorizon = 5200
	const fireAbs = 5042
	g.engine.Predictor.Ensure(ensureHorizon)

	// A rebuilt predictor may cover fireAbs again; then the fallback is not exercised and the assertion would pass
	// vacuously.
	if g.engine.Predictor.VisibleAt(0, fireAbs) {
		t.Skip("predictor still has abs=5042 in window — cannot exercise long-session fallback path")
	}

	// An offset past fireAbs keeps the cell out of the row's Steps slate too.
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
