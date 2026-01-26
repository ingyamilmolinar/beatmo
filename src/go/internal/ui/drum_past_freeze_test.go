package ui

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// The past must never change: once playback has advanced past a subdivision,
// edits to the circuit must not alter previously-rendered cells. Only future
// cells may change. This also implies we can cache the past.
func TestDrumView_PastNeverChanges(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetFollow(false) // keep window fixed

	// Simple 4-node loop for row 0
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(1, 1, model.NodeTypeRegular)
	d := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.SetLength(16)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Build caches and start playback.
	dst := ebiten.NewImage(800, 240)
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)
	g.SetPlaying(true)
	for abs := 0; abs <= 12; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	g.elapsedBeats = 12
	g.refreshDrumRow()
	pastIdx := 0
	if len(g.nextBeatIdxs) > 0 {
		pastIdx = g.nextBeatIdxs[0]
	}
	beforeState := g.dumpRowState(0)
	snap := append([]bool(nil), g.drum.Rows[0].Steps...)

	// Edit: silence node b (affects future predictions/visibility).
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		n.Type = model.NodeTypeSilent
		g.graph.Nodes[b.ID] = n
		g.notifyPredictorNode(b.ID)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	afterState := g.dumpRowState(0)
	after := append([]bool(nil), g.drum.Rows[0].Steps...)

	// Past portion of the window must remain identical.
	for i := 0; i < len(after) && i < len(snap); i++ {
		abs := g.drum.Offset + i
		if abs < pastIdx {
			if after[i] != snap[i] {
				beforeVal, beforeMask := false, false
				if rel := abs - beforeState.Timeline.Offset; rel >= 0 && rel < len(beforeState.Timeline.Past) {
					beforeVal = beforeState.Timeline.Past[rel]
					if rel < len(beforeState.Timeline.PastMask) {
						beforeMask = beforeState.Timeline.PastMask[rel]
					}
				}
				afterVal, afterMask := false, false
				if rel := abs - afterState.Timeline.Offset; rel >= 0 && rel < len(afterState.Timeline.Past) {
					afterVal = afterState.Timeline.Past[rel]
					if rel < len(afterState.Timeline.PastMask) {
						afterMask = afterState.Timeline.PastMask[rel]
					}
				}
				commitVal, commitTyp, commitKind, commitOK := g.timelineCommittedWithKind(0, abs)
				t.Fatalf("past changed at abs=%d: before=%v after=%v pastIdx=%d offset=%d playing=%v freezeBefore=%d freezeAfter=%d timelineBefore=%v maskBefore=%v timelineAfter=%v maskAfter=%v commitVal=%v commitTyp=%v commitKind=%v commitOK=%v stepsBefore=%v stepsAfter=%v",
					abs, snap[i], after[i], pastIdx, g.drum.Offset, g.Playing(), beforeState.FrozenUpTo, afterState.FrozenUpTo, beforeVal, beforeMask, afterVal, afterMask, commitVal, commitTyp, commitKind, commitOK, snap, after)
			}
		}
	}
}

func TestDrumView_PastCellTypesStayFrozenOnMuteToggle(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetFollow(false)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(1, 1, model.NodeTypeRegular)
	d := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.SetLength(16)
	g.updateBeatInfos()
	g.refreshDrumRow()

	dst := ebiten.NewImage(800, 240)
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(120)
	g.SetPlaying(true)
	for abs := 0; abs <= 12; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	g.elapsedBeats = 12
	g.refreshDrumRow()
	pastIdx := 0
	if len(g.nextBeatIdxs) > 0 {
		pastIdx = g.nextBeatIdxs[0]
	}
	beforeTypes := append([]model.NodeType(nil), g.drum.Rows[0].CellTypes...)

	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		n.Type = model.NodeTypeMute
		g.graph.Nodes[b.ID] = n
		g.notifyPredictorNode(b.ID)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	afterTypes := append([]model.NodeType(nil), g.drum.Rows[0].CellTypes...)

	for i := 0; i < len(afterTypes) && i < len(beforeTypes); i++ {
		abs := g.drum.Offset + i
		if abs < pastIdx {
			if afterTypes[i] != beforeTypes[i] {
				t.Fatalf("past cell type changed at abs=%d: before=%v after=%v", abs, beforeTypes[i], afterTypes[i])
			}
		}
	}
}

func TestDrumView_FutureUpdatesImmediatelyOnEdit(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetFollow(false)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	d := g.tryAddNode(3, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.SetLength(48)
	g.updateBeatInfos()
	g.refreshDrumRow()

	dst := ebiten.NewImage(640, 240)
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	g.drum.SetBPM(60)
	g.SetAppliedBPMForTest(60)
	g.SetPlaying(true)
	for abs := 0; abs <= 20; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	g.elapsedBeats = 20
	g.refreshDrumRow()

	if len(g.nextBeatIdxs) == 0 {
		t.Fatalf("expected nextBeatIdxs after playback")
	}
	beforeNext := append([]int(nil), g.nextBeatIdxs...)
	pastBound := g.nextBeatIdxs[0]
	before := append([]bool(nil), g.drum.Rows[0].Steps...)
	idPrefix := func(bis []model.BeatInfo, n int) []model.NodeID {
		if n > len(bis) {
			n = len(bis)
		}
		out := make([]model.NodeID, n)
		for i := 0; i < n; i++ {
			out[i] = bis[i].NodeID
		}
		return out
	}
	beforeIDs := idPrefix(g.beatInfosByRow[0], 8)
	if pastBound <= g.drum.Offset {
		t.Fatalf("expected future pastBound > offset, got %d offset %d", pastBound, g.drum.Offset)
	}
	startIdx := pastBound - g.drum.Offset
	if startIdx < 0 || startIdx >= len(before) {
		t.Fatalf("future index %d out of range len=%d offset=%d", startIdx, len(before), g.drum.Offset)
	}
	futureIdx := -1
	for i := startIdx; i < len(before); i++ {
		if g.beatInfoAtRow(0, g.drum.Offset+i).NodeID == b.ID {
			futureIdx = i
			break
		}
	}
	if futureIdx < 0 {
		t.Fatalf("could not find future cell for deleted node starting at %d", startIdx)
	}

	if node := g.nodeAt(b.I, b.J); node != nil {
		g.deleteNode(node)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()
	afterDelete := append([]bool(nil), g.drum.Rows[0].Steps...)
	if futureIdx >= len(afterDelete) {
		t.Fatalf("future index %d out of range after delete len=%d", futureIdx, len(afterDelete))
	}
	if afterDelete[futureIdx] {
		t.Fatalf("future cell remained active after delete (idx=%d) before=%v after=%v pastBound=%d offset=%d next=%v nextBefore=%v freeze=%v pathChanged=%v idsBefore=%v idsAfter=%v",
			futureIdx, before, afterDelete, pastBound, g.drum.Offset, g.nextBeatIdxs, beforeNext, g.frozenUpToByRow, g.rowsPathChanged, beforeIDs, idPrefix(g.beatInfosByRow[0], 8))
	}

	newB := g.tryAddNode(b.I, b.J, model.NodeTypeRegular)
	g.addEdge(a, newB)
	g.addEdge(newB, c)
	g.updateBeatInfos()
	g.refreshDrumRow()
	afterAdd := append([]bool(nil), g.drum.Rows[0].Steps...)
	if futureIdx >= len(afterAdd) {
		t.Fatalf("future index %d out of range after re-add len=%d", futureIdx, len(afterAdd))
	}
	if !afterAdd[futureIdx] {
		t.Fatalf("future cell did not reactivate immediately after re-add (idx=%d)", futureIdx)
	}
}

// TODO: Failing test exposing that frozen past cells still mutate while the
// game loop runs and future edits are applied. The refactor must ensure past
// snapshots remain immutable.
func TestDrumView_PastCellsImmutableDuringPlayback(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(1024, 720)
	g.drum.SetFollow(false)

	// Build a square loop.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(1, 1, model.NodeTypeRegular)
	d := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.SetLength(32)
	g.updateBeatInfos()
	g.refreshDrumRow()

	dst := ebiten.NewImage(1024, 240)
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)

	g.drum.SetBPM(120)
	advancePlayback(g, 80*time.Millisecond)
	g.SetPlaying(true)
	advancePlayback(g, 220*time.Millisecond)

	stateBefore := g.dumpRowState(0)
	if stateBefore.FrozenUpTo < 0 {
		t.Fatalf("expected past to be frozen, state=%+v", stateBefore)
	}
	stepsBefore := append([]bool(nil), g.drum.Rows[0].Steps...)
	offsetBefore := g.drum.Offset

	// Future edit: detour edge b->c through two new nodes.
	n1 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 1, model.NodeTypeRegular)
	g.deleteEdge(b, c)
	g.addEdge(b, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, c)
	g.updateBeatInfos()

	// Let the game loop run for a while.
	advanceFrames(g, 12)
	g.refreshDrumRow()
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)

	stateAfter := g.dumpRowState(0)
	stepsAfter := append([]bool(nil), g.drum.Rows[0].Steps...)

	if g.drum.Offset != offsetBefore {
		t.Fatalf("window offset changed (before=%d after=%d)", offsetBefore, g.drum.Offset)
	}

	freezeAbs := stateBefore.FrozenUpTo
	committedBefore := make(map[int]bool)
	for i, val := range stateBefore.Timeline.Past {
		if i < len(stateBefore.Timeline.PastMask) && stateBefore.Timeline.PastMask[i] {
			committedBefore[stateBefore.Timeline.Offset+i] = val
		}
	}
	committedAfter := make(map[int]bool)
	for i, val := range stateAfter.Timeline.Past {
		if i < len(stateAfter.Timeline.PastMask) && stateAfter.Timeline.PastMask[i] {
			committedAfter[stateAfter.Timeline.Offset+i] = val
		}
	}
	for abs, before := range committedBefore {
		if abs > freezeAbs {
			continue
		}
		after, ok := committedAfter[abs]
		if !ok {
			t.Fatalf("committed entry at abs=%d missing after edit", abs)
		}
		if after != before {
			t.Fatalf("committed entry at abs=%d mutated: before=%v after=%v", abs, before, after)
		}
	}

	for abs := offsetBefore; abs <= freezeAbs; abs++ {
		rel := abs - offsetBefore
		if rel < 0 || rel >= len(stepsBefore) || rel >= len(stepsAfter) {
			continue
		}
		if stepsBefore[rel] != stepsAfter[rel] {
			t.Fatalf("past step changed at abs=%d rel=%d: before=%v after=%v", abs, rel, stepsBefore[rel], stepsAfter[rel])
		}
	}
}

func TestDrumView_ReaddedNodeActivatesImmediateFuture(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetFollow(false)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	d := g.tryAddNode(3, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.SetLength(48)
	g.drum.Offset = 12
	g.updateBeatInfos()
	g.refreshDrumRow()

	g.nextBeatIdxs = []int{g.drum.Offset + 24}
	g.frozenUpToByRow = []int{g.drum.Offset - 1}

	targetRel := -1
	for i := 0; i < len(g.drum.Rows[0].Steps); i++ {
		abs := g.drum.Offset + i
		if g.beatInfoAtRow(0, abs).NodeID == b.ID {
			targetRel = i
			break
		}
	}
	if targetRel < 0 {
		t.Fatalf("did not locate future beat for node b")
	}

	if node := g.nodeAt(b.I, b.J); node != nil {
		g.deleteNode(node)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()
	afterDelete := append([]bool(nil), g.drum.Rows[0].Steps...)
	if afterDelete[targetRel] {
		t.Fatalf("future cell for node b remained active after delete at rel=%d", targetRel)
	}

	newB := g.tryAddNode(b.I, b.J, model.NodeTypeRegular)
	g.addEdge(a, newB)
	g.addEdge(newB, c)
	g.updateBeatInfos()
	g.refreshDrumRow()
	afterAdd := append([]bool(nil), g.drum.Rows[0].Steps...)
	if !afterAdd[targetRel] {
		t.Fatalf("future cell did not reactivate immediately after re-add (idx=%d)", targetRel)
	}
}
