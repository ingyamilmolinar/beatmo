package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// highlight_vs_audio fires only at the current beat or ahead: a past beat's highlight ran its window and
// expired by design after the audio fired correctly.
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

	// abs=139 already played, its audio event is logged, its highlight aged out.
	g.SetPlaying(true)
	g.elapsedBeats = 139
	g.nextBeatIdxs = []int{140}
	g.seqNextIdxs = []int{141}

	// Pinned just past the event's When, so neither the future-lead skip nor the uninitialised-clock skip can mask
	// the check; otherwise the result depends on how long the binary has been running.
	const eventWhen = 12.578088
	restoreNow := audio.SetNowForTest(func() float64 { return eventWhen + 0.1 })
	t.Cleanup(restoreNow)

	// Scheduled, played and highlighted correctly; only the expiry remains.
	g.parityMu.Lock()
	g.parityAudio = append(g.parityAudio, parityAudioEvent{
		Row:       0,
		Abs:       139,
		When:      eventWhen,
		Inst:      roster.Kick,
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

	// The past-beat rule must short-circuit before the highlight check.
	g.parityScan("test-expired-highlight-past")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "highlight_vs_audio" {
			t.Fatalf("highlight_vs_audio fired for past beat with naturally-expired highlight: %+v", m)
		}
	}
}
