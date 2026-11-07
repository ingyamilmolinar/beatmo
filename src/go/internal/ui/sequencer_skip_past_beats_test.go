package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Catching up by scheduling past beats would emit a burst of late audio, which parity ignores by design.
func TestSequencerSkipsSchedulingBeatsBehindPlayhead(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Build a simple 2-node loop where every abs is audible.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.updateBeatInfos()

	// Parity checks stay in their default state so the test reflects runtime behaviour.
	g.parityWatch = parityWatchLog
	g.parityMu.Lock()
	g.parityAudio = nil
	g.parityAudioMaxIdx = nil
	g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
	g.parityMu.Unlock()

	g.SetPlaying(true)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})

	target := 10
	setPlayStartForAbs(g, target)
	g.nextBeatIdxs = []int{target + 1}
	g.seqNextIdxs = []int{0}

	g.seqScheduleTime()

	g.parityMu.Lock()
	events := append([]parityAudioEvent{}, g.parityAudio...)
	g.parityMu.Unlock()

	floor := target // nextBeatIdxs-1
	for _, ev := range events {
		if ev.Row != 0 {
			continue
		}
		if ev.Abs < floor {
			t.Fatalf("scheduled past beat abs=%d (floor=%d), events=%v", ev.Abs, floor, events)
		}
	}
	foundFloor := false
	for _, ev := range events {
		if ev.Row == 0 && ev.Abs == floor {
			foundFloor = true
			break
		}
	}
	if !foundFloor {
		t.Fatalf("expected sequencer to schedule current beat abs=%d, events=%v", floor, events)
	}
}
