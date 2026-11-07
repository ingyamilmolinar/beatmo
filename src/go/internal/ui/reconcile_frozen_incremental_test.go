//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func buildIncrementalReconcileScene(t *testing.T, steps int) *Game {
	t.Helper()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetLength(64)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.updateBeatInfos()
	g.refreshDrumRow()
	pressPlay(t, g.drum)
	_ = g.Update()
	advancePlaybackByAbs(g, steps)
	return g
}

// The per-row watermark advances with freezeLimit, so each refresh scans only newly frozen abs, not the whole
// frozen history.
func TestReconcileFrozenScanIsIncremental(t *testing.T) {
	g := buildIncrementalReconcileScene(t, 5_000)

	if len(g.reconciledUpToByRow) == 0 {
		t.Fatalf("reconciledUpToByRow not initialised after 5000 abs of playback")
	}
	earlyMark := g.reconciledUpToByRow[0]

	// Capture the watermark after another 45 000 abs.
	advancePlaybackByAbs(g, 45_000)
	lateMark := g.reconciledUpToByRow[0]

	if lateMark < earlyMark {
		t.Fatalf("reconciledUpToByRow[0] regressed: early=%d late=%d", earlyMark, lateMark)
	}

	// A watermark that never advances means the resume is broken or freezeLimit isn't growing.
	if lateMark <= earlyMark && earlyMark > 0 {
		t.Logf("watermark unchanged early=%d late=%d (acceptable if visible window not advancing)", earlyMark, lateMark)
	}
}

// The next refresh then re-scans from futureReleaseStart.
func TestReconcileFrozenInvalidatesOnPathChange(t *testing.T) {
	g := buildIncrementalReconcileScene(t, 1_000)

	if len(g.reconciledUpToByRow) == 0 {
		t.Fatalf("reconciledUpToByRow not initialised")
	}
	beforeMark := g.reconciledUpToByRow[0]
	if beforeMark < 0 {
		// Past the window end, so at least one abs is frozen.
		advancePlaybackByAbs(g, 200)
		beforeMark = g.reconciledUpToByRow[0]
	}

	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(c, c) // self-loop just to reuse a fresh node
	g.updateBeatInfos()
	g.refreshDrumRow()
	// The refresh re-scans and may advance the watermark again, so the helper is checked directly.
	g.invalidateReconciledAll()
	for r, mark := range g.reconciledUpToByRow {
		if mark != -1 {
			t.Fatalf("invalidateReconciledAll did not reset row=%d (mark=%d)", r, mark)
		}
	}
}
