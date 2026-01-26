package ui

import (
	_ "embed"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

//go:embed testdata/future_cache_loop.json
var futureCacheLoop []byte

func TestDrumView_FutureRemovalClearsFrozenHistory(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)
	g.drum.SetFollow(true)

	if err := g.Import(futureCacheLoop); err != nil {
		t.Fatalf("import minimal loop: %v", err)
	}

	dst := ebiten.NewImage(640, 240)
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)

	g.drum.SetBPM(120)
	advancePlayback(g, 80*time.Millisecond)
	pressPlay(t, g.drum)
	advancePlayback(g, 200*time.Millisecond)

	rowIdx := -1
	for i, row := range g.drum.Rows {
		if strings.EqualFold(row.Instrument, "sample-high-tom-9-wonder") {
			rowIdx = i
			break
		}
	}
	if rowIdx != 0 {
		t.Fatalf("expected tom row at index 0, got %d", rowIdx)
	}
	if len(g.nextBeatIdxs) <= rowIdx {
		t.Fatalf("nextBeatIdxs not populated for row %d", rowIdx)
	}

	// Target the third upcoming regular cell.
	targetAbs := -1
	seen := 0
	for abs := g.nextBeatIdxs[rowIdx]; abs < g.nextBeatIdxs[rowIdx]+g.grid.MaxDiv()*8; abs++ {
		bi := g.beatInfoAtRow(rowIdx, abs)
		if bi.NodeType == model.NodeTypeRegular && bi.NodeID != model.InvalidNodeID {
			if seen == 2 {
				targetAbs = abs
				break
			}
			seen++
		}
	}
	if targetAbs < 0 {
		t.Fatalf("did not locate target beat for tom row (next=%d)", g.nextBeatIdxs[rowIdx])
	}

	g.nextBeatIdxs[rowIdx] = targetAbs + g.grid.MaxDiv()*4

	targetInfo := g.beatInfoAtRow(rowIdx, targetAbs)
	targetNode := g.nodeByID(targetInfo.NodeID)
	if targetNode == nil {
		t.Fatalf("node %d missing from UI cache", targetInfo.NodeID)
	}

	// Simulate an over-frozen history entry that should be cleared on removal.
	if len(g.frozenUpToByRow) != len(g.drum.Rows) {
		g.frozenUpToByRow = make([]int, len(g.drum.Rows))
		for i := range g.frozenUpToByRow {
			g.frozenUpToByRow[i] = -1
		}
	}
	g.frozenUpToByRow[rowIdx] = targetAbs

	pastAbs := g.drum.Offset
	if pastAbs < targetAbs {
		pastInfo := g.beatInfoAtRow(rowIdx, pastAbs)
		g.recordTimelineCommitKind(rowIdx, pastAbs, false, pastInfo.NodeType, timeline.CommitKindSeeded)
	}

	left := neighborNode(g, rowIdx, targetAbs, -1)
	right := neighborNode(g, rowIdx, targetAbs, 1)
	if left == nil || right == nil {
		t.Fatalf("missing neighbors around abs=%d", targetAbs)
	}

	g.deleteNode(targetNode)
	g.updateBeatInfos()

	if !waitFutureState(g, dst, rowIdx, targetAbs, false, 1200*time.Millisecond) {
		t.Fatalf("future cell remained active after node removal (abs=%d offset=%d freeze=%d)", targetAbs, g.drum.Offset, g.frozenUpToByRow[rowIdx])
	}

	state := g.dumpRowState(rowIdx)
	relTarget := targetAbs - state.Timeline.Offset
	if relTarget >= 0 && relTarget < len(state.Timeline.PastMask) && state.Timeline.PastMask[relTarget] {
		t.Fatalf("future cell %d remained committed in past timeline after removal", targetAbs)
	}
}

func TestDrumView_FutureSameCycleReadd(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)
	g.drum.SetFollow(true)

	if err := g.Import(futureCacheLoop); err != nil {
		t.Fatalf("import minimal loop: %v", err)
	}
	dst := ebiten.NewImage(640, 240)
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	g.refreshDrumRow()

	rowIdx := 0
	targetAbs := -1
	count := 0
	for abs := g.drum.Offset; abs < g.drum.Offset+g.drum.Length; abs++ {
		bi := g.beatInfoAtRow(rowIdx, abs)
		if bi.NodeType == model.NodeTypeRegular && bi.NodeID != model.InvalidNodeID {
			if count == 1 {
				targetAbs = abs
				break
			}
			count++
		}
	}
	if targetAbs < 0 {
		t.Fatalf("did not locate first regular beat")
	}
	targetRel := targetAbs - g.drum.Offset
	if !g.drum.Rows[rowIdx].Steps[targetRel] {
		t.Fatalf("expected target cell active initially (rel=%d)", targetRel)
	}

	left := neighborNode(g, rowIdx, targetAbs, -1)
	right := neighborNode(g, rowIdx, targetAbs, 1)
	if left == nil || right == nil {
		t.Fatalf("missing neighbors for target beat")
	}
	targetNode := g.nodeByID(g.beatInfoAtRow(rowIdx, targetAbs).NodeID)
	if targetNode == nil {
		t.Fatalf("target node missing prior to delete")
	}

	g.deleteNode(targetNode)
	g.updateBeatInfos()
	g.refreshDrumRow()
	if g.drum.Rows[rowIdx].Steps[targetRel] {
		t.Fatalf("future cell stayed active after delete")
	}

	expectedNext := -1
	for abs := g.drum.Offset + 1; abs < g.drum.Offset+g.drum.Length; abs++ {
		bi := g.beatInfoAtRow(rowIdx, abs)
		if bi.NodeType == model.NodeTypeRegular || bi.NodeType == model.NodeTypeMute || bi.NodeType == model.NodeTypeSilent {
			expectedNext = abs
			break
		}
	}
	if expectedNext < 0 {
		expectedNext = g.drum.Offset + 1
	}
	if got := g.nextBeatIdxs[rowIdx]; got != expectedNext {
		t.Fatalf("nextBeatIdx not updated, got=%d want=%d", got, expectedNext)
	}

	newNode := g.tryAddNode(targetNode.I, targetNode.J, model.NodeTypeRegular)
	if newNode == nil {
		t.Fatalf("failed to re-add node")
	}
	g.deleteEdge(left, right)
	g.addEdge(left, newNode)
	g.addEdge(newNode, right)
	g.updateBeatInfos()
	g.refreshDrumRow()
	if !g.drum.Rows[rowIdx].Steps[targetRel] {
		t.Fatalf("future cell did not relight immediately after re-add")
	}
	state := g.dumpRowState(rowIdx)
	rel := targetAbs - state.Timeline.Offset
	if rel >= 0 && rel < len(state.Timeline.PastMask) && state.Timeline.PastMask[rel] {
		t.Fatalf("future cell %d incorrectly recorded in timeline past", targetAbs)
	}
	if state.FrozenUpTo >= targetAbs {
		t.Fatalf("freeze window extended into future cell: frozen=%d target=%d", state.FrozenUpTo, targetAbs)
	}
	if state.NextBeatIdx < targetAbs {
		t.Fatalf("nextBeatIdx fell behind future cell: got=%d target=%d", state.NextBeatIdx, targetAbs)
	}
	expectedNext = -1
	for abs := g.drum.Offset + 1; abs < g.drum.Offset+g.drum.Length; abs++ {
		bi := g.beatInfoAtRow(rowIdx, abs)
		if bi.NodeType == model.NodeTypeRegular || bi.NodeType == model.NodeTypeMute || bi.NodeType == model.NodeTypeSilent {
			expectedNext = abs
			break
		}
	}
	if expectedNext < 0 {
		expectedNext = g.drum.Offset + 1
	}
	if got := g.nextBeatIdxs[rowIdx]; got != expectedNext {
		t.Fatalf("nextBeatIdx not restored after re-add, got=%d want=%d", got, expectedNext)
	}

	for i := 0; i < 3; i++ {
		advancePlayback(g, 120*time.Millisecond)
		g.refreshDrumRow()
		if !g.drum.Rows[rowIdx].Steps[targetRel] {
			t.Fatalf("future cell toggled off after cycle %d", i)
		}
	}
}

func neighborNode(g *Game, rowIdx, startAbs, step int) *uiNode {
	abs := startAbs + step
	for iter := 0; iter < 32; iter++ {
		if abs < 0 {
			return nil
		}
		bi := g.beatInfoAtRow(rowIdx, abs)
		if bi.NodeType == model.NodeTypeRegular && bi.NodeID != model.InvalidNodeID {
			return g.nodeByID(bi.NodeID)
		}
		abs += step
	}
	return nil
}

func waitFutureState(g *Game, dst *ebiten.Image, rowIdx int, targetAbs int, want bool, timeout time.Duration) bool {
	maxIters := int(math.Ceil(timeout.Seconds()*60)) + 1
	if maxIters < 1 {
		maxIters = 1
	}
	for i := 0; i < maxIters; i++ {
		g.refreshDrumRow()
		g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
		rel := targetAbs - g.drum.Offset
		if rel >= 0 && rel < len(g.drum.Rows[rowIdx].Steps) {
			if g.drum.Rows[rowIdx].Steps[rel] == want {
				return true
			}
		}
		advancePlayback(g, 16*time.Millisecond)
	}
	return false
}
