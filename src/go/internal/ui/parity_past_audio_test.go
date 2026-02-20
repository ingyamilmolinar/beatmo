package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Past audio events should not demand active highlights or slate parity once
// the playhead has advanced beyond them.
func TestParityIgnoresPastAudioEvents(t *testing.T) {
	assertDefaultParityState(t)
	prevFatal := parityFatalEnabled.Load()
	SetParityFatal(true)
	t.Cleanup(func() { SetParityFatal(prevFatal) })

	g := buildTestGame(t)
	g.parityWatch = parityWatchPanic

	// Single-beat loop to keep predictor/state simple.
	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	g.updateBeatInfos()

	// Pretend we're already past beat 4.
	if len(g.nextBeatIdxs) == 0 {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}
	for i := range g.nextBeatIdxs {
		g.nextBeatIdxs[i] = 4
	}

	// Record an old audio event at abs=0 with no corresponding highlight.
	g.recordParityAudio(0, 0, 0.25, g.drum.Rows[0].Instrument, 1, 0, 1, g.audioGen.Load())
	g.resetHighlights()

	// Should not panic when scanning parity; past events are ignored.
	g.parityScan("test_past_audio")
}
