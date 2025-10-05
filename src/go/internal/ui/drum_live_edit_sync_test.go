package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
	"time"
)

// Complex live edits must keep DrumView window aligned with the sequencer:
// - The current subdivision (nextBeatIdxs[row]-1) is inside the window
// - The highlight exists for that absolute index
// - The Steps cell under the highlight matches predictor.VisibleAt
func TestLiveEdit_ComplexSequence_SyncWithSequencer(t *testing.T) {
	g := New(testLogger)
	g.SetUseSequencerForTest(true)
	g.Layout(1024, 720)

	// Base loop row 0
	A := g.tryAddNode(0, 0, model.NodeTypeRegular)
	B := g.tryAddNode(2, 0, model.NodeTypeRegular)
	C := g.tryAddNode(2, 2, model.NodeTypeRegular)
	D := g.tryAddNode(0, 2, model.NodeTypeRegular)
	g.addEdge(A, B)
	g.addEdge(B, C)
	g.addEdge(C, D)
	g.addEdge(D, A)
	g.start = A
	g.graph.StartNodeID = A.ID
	// Row 1 independent loop
	E := g.tryAddNode(6, 0, model.NodeTypeRegular)
	F := g.tryAddNode(7, 0, model.NodeTypeRegular)
	G := g.tryAddNode(7, 1, model.NodeTypeRegular)
	g.addEdge(E, F)
	g.addEdge(F, G)
	g.addEdge(G, E)
	g.drum.AddRow()
	g.drum.Rows[1].Origin = E.ID
	g.drum.Rows[1].Node = g.nodeByID(E.ID)

	g.drum.Length = 24
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.drum.SetBPM(120)
	until := time.Now().Add(80 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(2 * time.Millisecond)
	}
	g.drum.playPressed = true

	step := func(loops int) {
		end := time.Now().Add(time.Duration(loops) * 30 * time.Millisecond)
		for time.Now().Before(end) {
			_ = g.Update()
			time.Sleep(2 * time.Millisecond)
		}
		// Validate for both rows: highlight alignment and next-step consistency.
		for row := range g.drum.Rows {
			last := 0
			if row < len(g.nextBeatIdxs) {
				last = g.nextBeatIdxs[row] - 1
			}
			if last < 0 {
				last = 0
			}
			bi := g.beatInfoAtRow(row, last)
			if bi.NodeType == model.NodeTypeInvisible {
				continue
			}
			// Ensure last within window and highlighted.
			if last < g.drum.Offset || last >= g.drum.Offset+g.drum.Length {
				t.Fatalf("row %d last=%d not in window [%d,%d)", row, last, g.drum.Offset, g.drum.Offset+g.drum.Length)
			}
			if _, ok := g.highlightedBeats[makeBeatKey(row, last)]; !ok {
				t.Fatalf("row %d missing highlight at abs=%d", row, last)
			}
			// Capture predictor decision for next abs and verify it when we advance.
			nextIdx := last + 1
			want := false
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.Ensure(nextIdx + 1)
				want = g.engine.Predictor.VisibleAt(row, nextIdx)
			}
			// Advance updates until row moves to nextIdx (bound to a small window).
			deadline := time.Now().Add(120 * time.Millisecond)
			for (row >= len(g.nextBeatIdxs) || g.nextBeatIdxs[row]-1 < nextIdx) && time.Now().Before(deadline) {
				_ = g.Update()
				time.Sleep(2 * time.Millisecond)
			}
			cur := 0
			if row < len(g.nextBeatIdxs) {
				cur = g.nextBeatIdxs[row] - 1
			}
			if cur != nextIdx {
				t.Fatalf("row %d did not advance to next idx; got %d want %d", row, cur, nextIdx)
			}
			// Ensure window contains now current index and Steps agrees with earlier predictor decision.
			if cur < g.drum.Offset || cur >= g.drum.Offset+g.drum.Length {
				t.Fatalf("row %d new last out of window: %d", row, cur)
			}
			j := cur - g.drum.Offset
			got := g.drum.Rows[row].Steps[j]
			if got != want {
				t.Fatalf("row %d lag/phase at abs=%d: steps=%v want=%v", row, cur, got, want)
			}
		}
	}

	// 1) Add a detour ahead of the playhead
	X := g.tryAddNode(3, 0, model.NodeTypeRegular)
	H := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.deleteEdge(B, C)
	g.addEdge(B, X)
	g.addEdge(X, H)
	g.addEdge(H, C)
	g.updateBeatInfos()
	step(3)

	// 2) Remove a node and reconnect differently
	g.deleteEdge(X, H)
	g.deleteNode(H)
	g.addEdge(X, C)
	g.updateBeatInfos()
	step(3)

	// 3) Restore original shape
	g.deleteEdge(B, X)
	g.deleteEdge(X, C)
	g.deleteNode(X)
	g.addEdge(B, C)
	g.updateBeatInfos()
	step(3)
}
