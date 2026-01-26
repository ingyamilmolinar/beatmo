package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

// verifyMutePattern steps the sequencer for steps beats and collects whether
// the mute node triggered and whether an audio stop occurred per encounter.
func verifyMutePattern(t *testing.T, g *Game, row int, muteID model.NodeID, steps int, want []bool) {
	t.Helper()
	stops := 0
	inst := g.drum.Rows[row].Instrument
	audio.SetStopHook(func(id string) {
		if id == inst {
			stops++
		}
	})
	defer audio.SetStopHook(nil)

	got := make([]bool, 0, len(want))
	g.seqNextIdxs = make([]int, len(g.drum.Rows))
	for abs := 0; abs < steps && len(got) < len(want); abs++ {
		info := g.beatInfoAtRow(row, abs)
		scheduleAbsForMuteTest(g, abs)
		if info.NodeID == muteID {
			triggered, _ := g.lastTriggeredForTest(row, muteID)
			got = append(got, triggered)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("insufficient samples: got=%v want=%v", got, want)
	}
	// Check stops match number of true values
	wantStops := 0
	for _, v := range want {
		if v {
			wantStops++
		}
	}
	if stops != wantStops {
		t.Fatalf("stop mismatch: stops=%d want=%d pattern=%v", stops, wantStops, want)
	}
}

// Two-node loop: A(regular) <-> B(mute). Verify skip_every_n and every_n_triggers
// patterns for N=2 and N=3.
func TestMuteNodeSkipEveryN_TwoNodeLoop(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeMute)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}

	// N=2: trigger pattern at B is [true,false,true,false,...]
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(b.ID, p)
	}
	g.updateBeatInfos()
	g.SetPlaying(true)
	verifyMutePattern(t, g, 0, b.ID, len(g.beatInfosByRow[0])*4, []bool{true, false, true, false})

	// N=3: [true,true,false,true,true,false]
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 3
		g.graph.SetNodeParams(b.ID, p)
	}
	g.seqNextIdxs = nil
	verifyMutePattern(t, g, 0, b.ID, len(g.beatInfosByRow[0])*6, []bool{true, true, false, true, true, false})
}

func TestMuteNodeEveryNTriggers_TwoNodeLoop(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeMute)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}

	// N=2: [false,true,false,true]
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(b.ID, p)
	}
	g.updateBeatInfos()
	g.SetPlaying(true)
	verifyMutePattern(t, g, 0, b.ID, len(g.beatInfosByRow[0])*4, []bool{false, true, false, true})

	// N=3: [false,false,true,false,false,true]
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 3
		g.graph.SetNodeParams(b.ID, p)
	}
	g.seqNextIdxs = nil
	verifyMutePattern(t, g, 0, b.ID, len(g.beatInfosByRow[0])*6, []bool{false, false, true, false, false, true})
}
