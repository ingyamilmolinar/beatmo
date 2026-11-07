package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Mute and solo suppress scheduling, so silenced beats are not missing audio.
func TestParityScanSkipsAudioMissingWhenRowMuted(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.parityWatch = parityWatchLog
	g.ClearParityMismatches()

	// Minimal 1-node regular loop so abs=0 is "audible" in predictor truth.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.addEdge(a, a)
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)
	g.drum.SetLength(1)
	g.drum.Offset = 0
	g.refreshDrumRow()

	// Mute the row via the instrument panel state.
	setRowMuted(t, g.drum, 0, true)
	g.SetPlaying(true)

	// Muted: no audio, but a seq decision still recorded.
	scheduleAbsForMuteTest(g, 0)

	// Past the grace window, so only the mute/solo gating can keep it from flagging.
	g.parityMu.Lock()
	if m, ok := g.paritySeqDecisions[0]; ok {
		if dec, ok2 := m[0]; ok2 {
			dec.RecordedAt = time.Unix(0, 0).Add(-250 * time.Millisecond)
			m[0] = dec
		}
	}
	g.parityMu.Unlock()

	g.ClearParityMismatches()
	g.parityScan("test-muted-row-audio-missing")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "audio_missing" {
			t.Fatalf("unexpected audio_missing mismatch for muted row: %+v", g.ParityMismatchSnapshot())
		}
	}
}
