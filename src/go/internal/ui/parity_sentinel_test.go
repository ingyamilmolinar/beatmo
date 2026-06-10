package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Ensure highlight parity waits until the audio clock is ready / event is near.
func TestParityHighlightSkippedWhenAudioClockUnknown(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.parityWatch = parityWatchLog

	// Single row with one ON step at abs=0.
	root := g.start
	if root == nil {
		root = g.tryAddNode(0, 0, model.NodeTypeRegular)
	}
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.graph.StartNodeID = root.ID
	g.start = root
	g.drum.Rows[0].Origin = root.ID
	g.drum.Rows[0].Node = root
	g.addEdge(root, n1)
	g.addEdge(n1, root)
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)

	abs := 0
	if len(g.nextBeatIdxs) > 0 {
		abs = g.nextBeatIdxs[0]
	}
	if abs < 0 {
		abs = 0
	}
	g.drum.SetLength(1)
	g.drum.Offset = abs
	g.refreshDrumRow()

	// Sequencer decided audible and instrument present.
	g.recordSeqDecision(0, abs, true, model.NodeTypeRegular, false)
	// Audio scheduled in the future; tests with a real audio clock should also skip.
	when := audio.Now() + 1.0
	g.recordParityAudio(0, abs, when, "kick", 1, 0, 1, g.audioGen.Load())

	g.parityScan("test-future-audio")

	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected no mismatches when audio clock is unknown and audio is scheduled in the future, got %d", got)
	}
}

// Highlight parity should fire when audio is due now and highlight is missing.
func TestParityHighlightRequiredWhenAudioNow(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.parityWatch = parityWatchLog

	root := g.start
	if root == nil {
		root = g.tryAddNode(0, 0, model.NodeTypeRegular)
	}
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.graph.StartNodeID = root.ID
	g.start = root
	g.drum.Rows[0].Origin = root.ID
	g.drum.Rows[0].Node = root
	g.addEdge(root, n1)
	g.addEdge(n1, root)
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)

	abs := 0
	if len(g.nextBeatIdxs) > 0 {
		abs = g.nextBeatIdxs[0]
	}
	if abs < 0 {
		abs = 0
	}
	g.drum.SetLength(1)
	g.drum.Offset = abs
	g.refreshDrumRow()

	g.recordSeqDecision(0, abs, true, model.NodeTypeRegular, false)
	// Audio is "now" (when=0) so highlight should exist; we leave highlight map empty to trigger mismatch.
	g.recordParityAudio(0, abs, 0, "kick", 1, 0, 1, g.audioGen.Load())
	// Age the event past the in-flight grace window: a freshly-recorded event
	// is allowed one Update drain for its highlight to land (see the
	// RecordedAt grace in parityScan); the genuine violation this sentinel
	// guards is an event whose highlight never arrived.
	g.parityMu.Lock()
	g.parityAudio[len(g.parityAudio)-1].RecordedAt = time.Now().Add(-500 * time.Millisecond)
	g.parityMu.Unlock()

	g.parityScan("test-audio-now")

	if got := len(g.ParityMismatchSnapshot()); got == 0 {
		t.Fatalf("expected highlight_vs_audio mismatch when audio is due now and highlight missing")
	}
}
