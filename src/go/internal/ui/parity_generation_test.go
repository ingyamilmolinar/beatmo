package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestParityGenAdvancesOnStructuralMutation locks down the contract that a
// runtime structural mutation through bumpParityGen monotonically advances
// the generation counter and grants a grace window. The gen counter is the
// spine of cross-thread parity coordination — any regression that stops
// it from advancing on a real mutation should fail this test.
func TestParityGenAdvancesOnStructuralMutation(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	t.Cleanup(g.CloseForTest)

	gen0 := g.parityGen.Load()
	graceBefore := g.parityGraceUntilNS.Load()

	g.bumpParityGen("test-bump", structuralMutationOptions{Grace: 50 * time.Millisecond})

	gen1 := g.parityGen.Load()
	graceAfter := g.parityGraceUntilNS.Load()

	if gen1 <= gen0 {
		t.Fatalf("gen did not advance: %d -> %d", gen0, gen1)
	}
	if graceAfter <= graceBefore {
		t.Fatalf("grace deadline did not advance: %d -> %d", graceBefore, graceAfter)
	}
	if !g.pathsDirty {
		t.Fatalf("expected pathsDirty=true after bumpParityGen; default opts request it")
	}

	// A second bump with a larger grace must extend, never shrink.
	priorDeadline := g.parityGraceUntilNS.Load()
	g.bumpParityGen("test-bump-2", structuralMutationOptions{Grace: 200 * time.Millisecond})
	if d := g.parityGraceUntilNS.Load(); d <= priorDeadline {
		t.Fatalf("grace did not extend on second bump: %d -> %d", priorDeadline, d)
	}

	// A bump with a smaller grace must NOT shrink the existing deadline.
	priorDeadline = g.parityGraceUntilNS.Load()
	g.bumpParityGen("test-bump-3", structuralMutationOptions{Grace: 1 * time.Millisecond})
	if d := g.parityGraceUntilNS.Load(); d < priorDeadline {
		t.Fatalf("grace shrank on smaller-grace bump: %d -> %d", priorDeadline, d)
	}
}

// TestParityGenStampedOnRecord verifies that recordParityAudio and
// recordSeqDecision capture the current parity generation. Future scan
// policies that filter by gen rely on this stamp; the test pins the contract.
func TestParityGenStampedOnRecord(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	t.Cleanup(g.CloseForTest)
	g.parityWatch = parityWatchLog // ensure record paths run

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.updateBeatInfos()

	g.bumpParityGen("test-pre-record", structuralMutationOptions{})
	wantGen := g.parityGen.Load()

	g.recordSeqDecision(0, 100, true, model.NodeTypeRegular, false)
	g.recordParityAudio(0, 100, 0.5, "kick", 1, 0, 1, g.audioGen.Load())

	g.parityMu.Lock()
	dec := g.paritySeqDecisions[0][100]
	var ev parityAudioEvent
	for _, e := range g.parityAudio {
		if e.Row == 0 && e.Abs == 100 {
			ev = e
			break
		}
	}
	g.parityMu.Unlock()

	if dec.ParityGen != wantGen {
		t.Fatalf("seq decision ParityGen=%d want %d", dec.ParityGen, wantGen)
	}
	if ev.ParityGen != wantGen {
		t.Fatalf("audio event ParityGen=%d want %d", ev.ParityGen, wantGen)
	}
}

// TestMarkAllRowsPathChangedAt is a unit-level guard for the cross-row marker.
// When mutateStructural runs without SkipPathChangeMark, every row's
// pathChangeBeatByRow must advance to (at least) the current elapsedBeats so
// the freeze loop in refreshDrumRow does not commit pre-mutation past beats
// against the post-mutation predictor view.
func TestMarkAllRowsPathChangedAt(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	t.Cleanup(g.CloseForTest)

	// Build a 3-row drum view.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.AddRow()
	g.drum.AddRow()

	g.elapsedBeats = 42
	g.markAllRowsPathChangedAt(g.elapsedBeats)
	if len(g.pathChangeBeatByRow) < 3 {
		t.Fatalf("pathChangeBeatByRow length=%d want>=3", len(g.pathChangeBeatByRow))
	}
	for i, v := range g.pathChangeBeatByRow {
		if v < 42 {
			t.Fatalf("row %d pathChangeBeat=%d want>=42", i, v)
		}
	}

	// A later mutation at a lower beat must NOT decrease the marker.
	g.markAllRowsPathChangedAt(10)
	for i, v := range g.pathChangeBeatByRow {
		if v < 42 {
			t.Fatalf("row %d pathChangeBeat=%d went backwards (want>=42)", i, v)
		}
	}

	// A later mutation at a higher beat advances every row.
	g.markAllRowsPathChangedAt(100)
	for i, v := range g.pathChangeBeatByRow {
		if v < 100 {
			t.Fatalf("row %d pathChangeBeat=%d want>=100", i, v)
		}
	}
}
