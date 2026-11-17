package ui

import "testing"

func TestUndoManager_RecordUndoRedo(t *testing.T) {
	state := "A"
	snaps := []string{}
	m := NewUndoManager(
		func() []byte { return []byte(state) },
		func(b []byte) error { state = string(b); return nil },
	)
	m.maxDepth = 100

	state = "B"
	m.record("edit-1")
	state = "C"
	m.record("edit-2")

	if !m.CanUndo() {
		t.Fatal("expected CanUndo true")
	}
	if got := m.UndoLabel(); got != "edit-2" {
		t.Fatalf("UndoLabel = %q, want edit-2", got)
	}

	m.Undo()
	if state != "B" {
		t.Fatalf("after undo state=%q want B", state)
	}
	m.Undo()
	if state != "A" {
		t.Fatalf("after 2nd undo state=%q want A", state)
	}
	if m.CanUndo() {
		t.Fatal("expected CanUndo false at bottom")
	}
	m.Redo()
	if state != "B" {
		t.Fatalf("after redo state=%q want B", state)
	}
	_ = snaps
}

func TestUndoManager_NoopGestureDropped(t *testing.T) {
	state := "A"
	m := NewUndoManager(func() []byte { return []byte(state) }, func(b []byte) error { state = string(b); return nil })
	m.record("noop")
	if m.CanUndo() {
		t.Fatal("identical snapshot must not push a step")
	}
}

func TestUndoManager_NewActionClearsRedo(t *testing.T) {
	state := "A"
	m := NewUndoManager(func() []byte { return []byte(state) }, func(b []byte) error { state = string(b); return nil })
	state = "B"
	m.record("b")
	m.Undo()
	if !m.CanRedo() {
		t.Fatal("expected CanRedo true")
	}
	state = "C"
	m.record("c")
	if m.CanRedo() {
		t.Fatal("new action must clear redo stack")
	}
}

func TestUndoManager_RingBound(t *testing.T) {
	state := "0"
	m := NewUndoManager(func() []byte { return []byte(state) }, func(b []byte) error { state = string(b); return nil })
	m.maxDepth = 3
	for i := 1; i <= 10; i++ {
		state = string(rune('0' + i))
		m.record("e")
	}
	count := 0
	for m.CanUndo() {
		m.Undo()
		count++
	}
	if count != 3 {
		t.Fatalf("retained %d steps, want 3 (ring bound)", count)
	}
}

func TestUndoManager_RestoringGuardAndExternalLoad(t *testing.T) {
	state := "A"
	m := NewUndoManager(func() []byte { return []byte(state) }, func(b []byte) error { state = string(b); return nil })
	state = "B"
	m.record("b")
	m.OnExternalLoad()
	if m.CanUndo() || m.CanRedo() {
		t.Fatal("OnExternalLoad must clear both stacks")
	}
	state = "C"
	m.record("c")
	state = "D"
	m.record("d")
	m.restoring = true
	state = "E"
	m.record("should-be-ignored")
	m.restoring = false
	if m.UndoLabel() == "should-be-ignored" {
		t.Fatal("record during restoring must be ignored")
	}
}
