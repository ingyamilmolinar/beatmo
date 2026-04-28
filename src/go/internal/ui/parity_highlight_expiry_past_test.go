package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestParityIgnoresExpiredHighlightInPast reproduces the second-form crash:
// no instrument change at all, just normal demo playback. After ~17 beats,
// parityScan finds an audio event for a past beat whose highlight has aged
// out (highlightedBeats[key].until < g.frame) and panics with
// highlight_vs_audio.
//
// The fix: highlight_vs_audio must only fire at the current beat or in the
// future window. Past beats whose highlight expired naturally are not
// violations — the audio fired correctly while the beat was current, the
// highlight ran for its window, then expired by design.
func TestParityIgnoresExpiredHighlightInPast(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	t.Cleanup(g.CloseForTest)
	g.parityWatch = parityWatchPanic

	// Single-row playable circuit.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.updateBeatInfos()

	g.drum.SetLength(128)
	g.drum.Offset = 64
	g.refreshDrumRow()
	g.ClearParityMismatches()

	// Simulate the failure mode from the field log:
	//   abs=139 already played (sequencer advanced past it), audio event sits
	//   in parityAudio with When>0, highlight has aged out.
	g.SetPlaying(true)
	g.elapsedBeats = 139
	g.nextBeatIdxs = []int{140}
	g.seqNextIdxs = []int{141}

	// Inject a recorded audio event AND matching seq decision for the past
	// beat — exactly the state the field crash captured: scheduler scheduled
	// the beat correctly, audio fired correctly, the highlight ran while the
	// beat was current and then expired naturally. The only "anomaly" is the
	// expired highlight, which is not actually a bug.
	g.parityMu.Lock()
	g.parityAudio = append(g.parityAudio, parityAudioEvent{
		Row:       0,
		Abs:       139,
		When:      12.578088,
		Inst:      "kick",
		Vol:       1.0,
		Dur:       1.0,
		Gen:       g.audioGen.Load(),
		ParityGen: g.parityGen.Load(),
	})
	if g.paritySeqDecisions[0] == nil {
		g.paritySeqDecisions[0] = make(map[int]paritySeqDecision)
	}
	g.paritySeqDecisions[0][139] = paritySeqDecision{
		Row:       0,
		Abs:       139,
		Audible:   true,
		Visible:   true,
		NodeType:  model.NodeTypeRegular,
		ParityGen: g.parityGen.Load(),
	}
	g.parityMu.Unlock()

	// highlightedBeats is empty for abs=139 (highlight already expired).
	// Without the fix, parityScan panics with highlight_vs_audio. With the
	// fix, the past-beat skip rule short-circuits before the highlight check.
	g.parityScan("test-expired-highlight-past")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "highlight_vs_audio" {
			t.Fatalf("highlight_vs_audio fired for past beat with naturally-expired highlight: %+v", m)
		}
	}
}
