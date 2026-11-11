package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// runFrameWithLockedButton mimics one production Game.Update frame in which the
// transport button is pressed: Game.Update holds seqMu while dv.tree.Update()
// dispatches the button OnClick, then releases seqMu and drains queued actions.
// The locked section runs in a goroutine guarded by a timeout so a re-entrant
// seqMu deadlock surfaces as a test failure instead of hanging the whole suite.
func runFrameWithLockedButton(t *testing.T, g *Game, click func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		g.seqMu.Lock() // Game.Update holds seqMu across drum.Update() (game_update.go:380)
		click()        // dv.tree.Update() dispatches the button OnClick under the lock
		g.seqMu.Unlock()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("DEADLOCK: transport button OnClick under seqMu did not return")
	}
	// Game.Update runs queued actions after seqMu.Unlock (game_update.go:478).
	g.drainPendingActions()
}

// TestUndoButtonDuringPlaybackNoDeadlock reproduces the reported hang: during
// playback, add a node, then click the transport Undo button. The button is
// dispatched from dv.tree.Update() while Game.Update holds seqMu, so a direct
// undoManager.Undo() (which re-imports → updateBeatInfos → seqMu.Lock) would
// self-deadlock the non-reentrant seqMu. The undo must complete and actually
// restore the document.
func TestUndoButtonDuringPlaybackNoDeadlock(t *testing.T) {
	g := newTestGameForUndo(t)
	baselineNodes := len(g.graph.Nodes)

	// Live edit during playback: add a node, commit an undo step.
	g.tryAddNode(2, 2, model.NodeTypeRegular)
	g.updateBeatInfos()
	g.undoManager.record("add node")
	if !g.undoManager.CanUndo() {
		t.Fatal("precondition: expected undo available after adding a node")
	}
	if len(g.graph.Nodes) != baselineNodes+1 {
		t.Fatalf("precondition: expected node count %d, got %d", baselineNodes+1, len(g.graph.Nodes))
	}

	g.SetPlaying(true)

	tz := g.drum.transportZone
	if tz == nil || tz.undoBtn == nil {
		t.Fatal("undo button not constructed")
	}

	runFrameWithLockedButton(t, g, tz.undoBtn.OnClick)

	// The undo must have actually restored the pre-add document.
	if len(g.graph.Nodes) != baselineNodes {
		t.Fatalf("undo did not restore graph: expected %d nodes, got %d", baselineNodes, len(g.graph.Nodes))
	}
	if !g.undoManager.CanRedo() {
		t.Fatal("after undo, redo should be available")
	}
	// Undo via Import restores the wasPlaying state, so playback continues.
	if !g.Playing() {
		t.Fatal("playback should be preserved across undo during playback")
	}
}

// TestRedoButtonDuringPlaybackNoDeadlock is the redo counterpart: after an undo
// during playback, clicking the transport Redo button (also dispatched under
// seqMu) must complete and re-apply the change without deadlocking.
func TestRedoButtonDuringPlaybackNoDeadlock(t *testing.T) {
	g := newTestGameForUndo(t)
	baselineNodes := len(g.graph.Nodes)

	g.tryAddNode(2, 2, model.NodeTypeRegular)
	g.updateBeatInfos()
	g.undoManager.record("add node")
	g.SetPlaying(true)

	tz := g.drum.transportZone
	if tz == nil || tz.undoBtn == nil || tz.redoBtn == nil {
		t.Fatal("undo/redo buttons not constructed")
	}

	// Undo first (under the lock), then redo (under the lock).
	runFrameWithLockedButton(t, g, tz.undoBtn.OnClick)
	if len(g.graph.Nodes) != baselineNodes {
		t.Fatalf("undo did not restore graph: expected %d nodes, got %d", baselineNodes, len(g.graph.Nodes))
	}

	runFrameWithLockedButton(t, g, tz.redoBtn.OnClick)
	if len(g.graph.Nodes) != baselineNodes+1 {
		t.Fatalf("redo did not re-apply the node: expected %d nodes, got %d", baselineNodes+1, len(g.graph.Nodes))
	}
	if !g.Playing() {
		t.Fatal("playback should be preserved across redo during playback")
	}
}
