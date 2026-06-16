package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestUndoGroupCoalescesIntoOneStep proves that several record() calls bracketed
// by beginGroup/endGroup collapse into a SINGLE undo step capturing only the
// final post-operation state, and a single Undo restores the pre-group state.
func TestUndoGroupCoalescesIntoOneStep(t *testing.T) {
	state := "A"
	captures := 0
	m := NewUndoManager(
		func() []byte { captures++; return []byte(state) },
		func(b []byte) error { state = string(b); return nil },
	)

	m.beginGroup("compound")
	state = "B"
	m.record("sub-1")
	state = "C"
	m.record("sub-2")
	state = "D"
	m.record("sub-3")
	// Nothing recorded yet — all deferred to endGroup.
	if m.CanUndo() {
		t.Fatal("group must not push steps until endGroup")
	}
	m.endGroup()

	if !m.CanUndo() {
		t.Fatal("expected exactly one undo step after endGroup")
	}
	if got := len(m.undo); got != 1 {
		t.Fatalf("group produced %d undo steps, want 1 (atomic)", got)
	}
	if got := m.UndoLabel(); got != "compound" {
		t.Fatalf("explicit group label = %q, want compound", got)
	}
	m.Undo()
	if state != "A" {
		t.Fatalf("single undo restored %q, want A (pre-group state)", state)
	}
}

// TestUndoGroupLabelFallsBackToFirstRecord proves that when beginGroup is opened
// with an empty label, the step takes the first sub-action's label.
func TestUndoGroupLabelFallsBackToFirstRecord(t *testing.T) {
	state := "A"
	m := NewUndoManager(func() []byte { return []byte(state) }, func(b []byte) error { state = string(b); return nil })
	m.beginGroup("")
	state = "B"
	m.record("first")
	state = "C"
	m.record("second")
	m.endGroup()
	if got := m.UndoLabel(); got != "first" {
		t.Fatalf("empty-group label = %q, want first (first-record fallback)", got)
	}
}

// TestUndoGroupNestedFinalizesOnce proves that nested begin/endGroup pairs only
// finalize at the outermost endGroup (one step), and the depth balances so the
// first non-empty label wins over inner labels.
func TestUndoGroupNestedFinalizesOnce(t *testing.T) {
	state := "A"
	m := NewUndoManager(func() []byte { return []byte(state) }, func(b []byte) error { state = string(b); return nil })
	m.beginGroup("") // outer (e.g. per-frame bracket, no label)
	m.beginGroup("delete node")
	state = "B"
	m.record("delete node")
	m.beginGroup("delete row") // inner cascade label must NOT override
	state = "C"
	m.record("delete row")
	m.endGroup()
	m.endGroup()
	if m.CanUndo() {
		t.Fatal("nested group must not finalize before outermost endGroup")
	}
	m.endGroup() // outermost
	if got := len(m.undo); got != 1 {
		t.Fatalf("nested group produced %d steps, want 1", got)
	}
	if got := m.UndoLabel(); got != "delete node" {
		t.Fatalf("nested label = %q, want delete node (first explicit wins)", got)
	}
}

// TestUndoEmptyGroupDoesNotCapture proves an empty frame (group opened/closed
// with no record) performs NO capture — the per-frame bracket must stay cheap.
func TestUndoEmptyGroupDoesNotCapture(t *testing.T) {
	captures := 0
	state := "A"
	m := NewUndoManager(
		func() []byte { captures++; return []byte(state) },
		func(b []byte) error { state = string(b); return nil },
	)
	capturesAfterCtor := captures
	for i := 0; i < 5; i++ {
		m.beginGroup("")
		m.endGroup()
	}
	if captures != capturesAfterCtor {
		t.Fatalf("empty groups triggered %d captures, want 0 (cheap no-op)", captures-capturesAfterCtor)
	}
	if m.CanUndo() {
		t.Fatal("empty groups must not push steps")
	}
}

// TestUndoGroupRestoringSuppressed proves records during a restore are ignored
// even inside a group (a keyboard-undo runs inside the per-frame bracket).
func TestUndoGroupRestoringSuppressed(t *testing.T) {
	state := "A"
	m := NewUndoManager(func() []byte { return []byte(state) }, func(b []byte) error { state = string(b); return nil })
	m.beginGroup("")
	m.restoring = true
	state = "Z"
	m.record("should-be-ignored")
	m.restoring = false
	m.endGroup()
	if m.CanUndo() {
		t.Fatal("records during restoring must not produce a step, even inside a group")
	}
}

// TestUndoMidCircuitNodeDeleteIsAtomic reproduces the reported bug: deleting a
// node that sits BETWEEN two others (A->B->C) triggers a reconnect edge (A->C)
// in addition to the node removal, so the operation emits multiple recorded
// events. It must collapse into exactly ONE undo step, and a single Undo must
// restore the deleted node AND both original edges (not a partial intermediate
// graph that needs a second Undo).
func TestUndoMidCircuitNodeDeleteIsAtomic(t *testing.T) {
	g := newTestGameForUndo(t)

	a := g.tryAddNode(4, 5, model.NodeTypeRegular)
	b := g.tryAddNode(5, 5, model.NodeTypeRegular)
	c := g.tryAddNode(6, 5, model.NodeTypeRegular)
	if a == nil || b == nil || c == nil {
		t.Fatalf("failed to create nodes a=%v b=%v c=%v", a, b, c)
	}
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.updateBeatInfos()

	// Baseline: capture the full A->B->C circuit as the pre-delete document.
	g.undoManager.OnExternalLoad()
	before := g.undoCapture()
	depthBefore := len(g.undoManager.undo)

	// Delete the middle node: removes B + edges A->B, B->C, and reconnects A->C.
	g.deleteNode(b)

	// The whole delete must be ONE atomic undo step.
	if got := len(g.undoManager.undo) - depthBefore; got != 1 {
		t.Fatalf("mid-circuit node delete recorded %d undo steps, want 1 (atomic)", got)
	}

	// A single Undo must restore the entire pre-delete document, byte-identical.
	g.undoManager.Undo()
	if got := g.undoCapture(); string(got) != string(before) {
		t.Fatalf("one Undo did not fully restore the pre-delete document")
	}
	if _, ok := g.graph.GetNodeByID(b.ID); !ok {
		t.Fatal("Undo did not restore the deleted middle node B")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{a.ID, b.ID}]; !ok {
		t.Fatal("Undo did not restore edge A->B")
	}
	if _, ok := g.graph.Edges[[2]model.NodeID{b.ID, c.ID}]; !ok {
		t.Fatal("Undo did not restore edge B->C")
	}
}
