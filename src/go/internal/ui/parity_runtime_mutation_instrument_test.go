package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestParityToleratesInstrumentChangeMidPlay reproduces the cross-row parity
// panic reported in the field: while playing, the user changes the instrument
// on a non-row-0 row (e.g. row 4 cowbell-1 -> cowbell-2). Parity then panics on
// a different row (row 2) at a beat that already played, complaining that a
// committed-silent past disagrees with a now-visible predictor.
//
// The root cause is that the past-freeze loop in refreshDrumRow can commit
// CommitKindPlayback (immutable) using a transient predictor query, AND no
// structural-generation barrier exists to coordinate the parity buffers across
// runtime mutations. After the fix:
//   - mutateStructural bumps the parity generation
//   - parity buffers (audio events, seq decisions, highlights) are filtered by
//     generation
//   - the freeze loop's safety-net commits become CommitKindReleased (mutable)
//   - applySequencerHighlight remains the sole CommitKindPlayback author
//
// This test must FAIL before the refactor and PASS after.
func TestParityToleratesInstrumentChangeMidPlay(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)
	g.parityWatch = parityWatchPanic

	// Row 0: 4-step rectangle starting at (0,0).
	a0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	a1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	a2 := g.tryAddNode(1, 1, model.NodeTypeRegular)
	a3 := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a0, a1)
	g.addEdge(a1, a2)
	g.addEdge(a2, a3)
	g.addEdge(a3, a0)
	g.start = a0
	g.graph.StartNodeID = a0.ID

	// Row 1: 3-step triangle elsewhere on the grid.
	b0 := g.tryAddNode(5, 0, model.NodeTypeRegular)
	b1 := g.tryAddNode(6, 0, model.NodeTypeRegular)
	b2 := g.tryAddNode(6, 1, model.NodeTypeRegular)
	g.addEdge(b0, b1)
	g.addEdge(b1, b2)
	g.addEdge(b2, b0)

	g.drum.AddRow()
	if len(g.drum.rowLabels()) < 2 {
		t.Fatalf("expected row labels for selection")
	}
	g.drum.rowLabels()[1].OnClick()
	g.drum.SetInstrument("snare")
	g.drum.Rows[1].Origin = b0.ID
	g.drum.Rows[1].Node = g.nodeByID(b0.ID)

	const horizon = 64
	g.drum.SetLength(horizon)
	g.updateBeatInfos()
	g.drum.Offset = 0
	g.refreshDrumRow()
	g.ClearParityMismatches()

	g.SetPlaying(true)

	// Drive playback for a substantial run so plenty of past commits accumulate
	// and the parity ring has many opportunities to fire.
	for abs := 0; abs < 32; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	g.refreshDrumRow()
	g.parityScan("pre-mutation")
	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("unexpected pre-mutation mismatches: %d %v", got, g.ParityMismatchSnapshot())
	}

	// Mid-play: change instrument on row 1 (the non-origin, cross-row case).
	// Then change it again (simulating the user scrolling through the menu —
	// matches the exact field repro: cowbell-1 -> cowbell-2 in rapid succession).
	g.drum.selRow = 1
	g.drum.SetInstrument("hihat")
	g.drum.SetInstrument("clap")

	// Continue advancing the sequencer past the mutation. This is when the
	// freeze loop and parity scan have historically tripped.
	for abs := 32; abs < 96; abs++ {
		scheduleAbsForMuteTest(g, abs)
		// Refresh + scan periodically so we exercise the post-mutation state on
		// a per-frame cadence the way real playback does.
		if abs%8 == 0 {
			g.refreshDrumRow()
			g.parityScan("post-mutation")
		}
	}
	g.refreshDrumRow()
	g.parityScan("post-mutation-final")

	if got := g.ParityMismatchSnapshot(); len(got) != 0 {
		t.Fatalf("instrument change during play produced %d parity mismatches:\n%+v", len(got), got)
	}
}

// TestParityToleratesSameRowInstrumentChange covers the simpler same-row case:
// changing the instrument on the row whose past is being committed. Should
// behave identically to the cross-row case after the fix.
func TestParityToleratesSameRowInstrumentChange(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)
	g.parityWatch = parityWatchPanic

	a0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	a1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	a2 := g.tryAddNode(1, 1, model.NodeTypeRegular)
	a3 := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a0, a1)
	g.addEdge(a1, a2)
	g.addEdge(a2, a3)
	g.addEdge(a3, a0)
	g.start = a0
	g.graph.StartNodeID = a0.ID

	const horizon = 32
	g.drum.SetLength(horizon)
	g.updateBeatInfos()
	g.drum.Offset = 0
	g.refreshDrumRow()
	g.ClearParityMismatches()

	g.SetPlaying(true)
	for abs := 0; abs < 16; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}

	g.drum.selRow = 0
	g.drum.SetInstrument("hihat")

	for abs := 16; abs < 48; abs++ {
		scheduleAbsForMuteTest(g, abs)
		if abs%4 == 0 {
			g.refreshDrumRow()
			g.parityScan("post-mutation")
		}
	}
	g.refreshDrumRow()
	g.parityScan("final")

	if got := g.ParityMismatchSnapshot(); len(got) != 0 {
		t.Fatalf("same-row instrument change produced %d parity mismatches:\n%+v", len(got), got)
	}
}
