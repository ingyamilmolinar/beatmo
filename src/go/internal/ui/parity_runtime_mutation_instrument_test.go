package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// An instrument change on one row while playing must not make another row's played past contradict the
// predictor: mutations bump the parity generation, buffers are filtered by it, and only highlights write
// immutable commits.
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
	g.drum.SetInstrument(roster.Snare)
	g.drum.Rows[1].Origin = b0.ID
	g.drum.Rows[1].Node = g.nodeByID(b0.ID)

	const horizon = 64
	g.drum.SetLength(horizon)
	g.updateBeatInfos()
	g.drum.Offset = 0
	g.refreshDrumRow()
	g.ClearParityMismatches()

	g.SetPlaying(true)

	for abs := 0; abs < 32; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	g.refreshDrumRow()
	g.parityScan("pre-mutation")
	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("unexpected pre-mutation mismatches: %d %v", got, g.ParityMismatchSnapshot())
	}

	// Twice in quick succession, as when scrolling through the menu.
	g.drum.selRow = 1
	g.drum.SetInstrument(roster.Hihat)
	g.drum.SetInstrument(roster.Clap)

	for abs := 32; abs < 96; abs++ {
		scheduleAbsForMuteTest(g, abs)
		// Refreshed and scanned at a per-frame cadence, like real playback.
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
	g.drum.SetInstrument(roster.Hihat)

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
