package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Probability logic should control whether the mute clears audio, without applying
// a sustained gate to future beats.
func TestMuteNodeProbabilityStopsAudioWithoutGate(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	mute := g.tryAddNode(1, 0, model.NodeTypeMute)
	tail := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(start, mute)
	g.addEdge(mute, tail)
	g.addEdge(tail, start)
	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}
	if n, ok := g.graph.GetNodeByID(mute.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0.0
		g.graph.SetNodeParams(mute.ID, p)
	}
	g.updateBeatInfos()

	g.SetPlaying(true)
	plays := 0
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })
	inst := g.drum.Rows[0].Instrument
	stops := 0
	audio.SetStopHook(func(id string) {
		if id == inst {
			stops++
		}
	})
	defer audio.SetStopHook(nil)

	// Regular beat plays.
	scheduleAbsForMuteTest(g, 0)
	if plays != 1 {
		t.Fatalf("regular did not play: plays=%d", plays)
	}
	// Mute with P=0 should not clear audio.
	scheduleAbsForMuteTest(g, 1)
	if stops != 0 {
		t.Fatalf("mute with P=0 stopped audio: stops=%d", stops)
	}

	// Enable mute to trigger on next visit.
	if n, ok := g.graph.GetNodeByID(mute.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 1.0
		g.graph.SetNodeParams(mute.ID, p)
	}

	// Advance until mute fires.
	muteAbs := -1
	for abs := 2; abs < 64 && stops < 1; abs++ {
		prevStops := stops
		scheduleAbsForMuteTest(g, abs)
		if stops > prevStops {
			muteAbs = abs
		}
	}
	if stops < 1 {
		t.Fatalf("mute with P=1 did not stop audio: stops=%d", stops)
	}
	if muteAbs < 0 {
		t.Fatalf("expected to capture mute abs when stop occurred")
	}

	// Immediately following beat should still play (no hold gate).
	prevPlays := plays
	scheduleAbsForMuteTest(g, muteAbs+1)
	if plays != prevPlays+1 {
		t.Fatalf("audio did not resume after mute: plays=%d prev=%d", plays, prevPlays)
	}
}
