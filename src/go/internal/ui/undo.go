package ui

import "bytes"

// undoEntry is one history step: a label for the UI + the document snapshot
// to restore to (the BEFORE state of the transition this entry represents).
type undoEntry struct {
	label    string
	snapshot []byte
}

// UndoManager is a session-only, bounded snapshot journal. It is decoupled
// from the mutation sites: it knows only how to capture the current document
// (Capture), how to restore one (Restore), and receives a synchronous
// record() call at each deterministic commit.
type UndoManager struct {
	capture   func() []byte
	restore   func([]byte) error
	committed []byte
	undo      []undoEntry
	redo      []undoEntry
	maxDepth  int
	restoring bool
}

// NewUndoManager constructs a manager. capture must return a deterministic
// byte snapshot of the document; restore must apply one.
func NewUndoManager(capture func() []byte, restore func([]byte) error) *UndoManager {
	return &UndoManager{
		capture:   capture,
		restore:   restore,
		committed: capture(),
		maxDepth:  100,
	}
}

// record snapshots the current (post-change) document and, if it differs from
// the committed baseline, pushes the previous baseline as an undo step. No-op
// gestures (snapshot == baseline) are dropped. Clears redo on a real change.
func (m *UndoManager) record(label string) {
	if m.restoring {
		return
	}
	snap := m.capture()
	if bytes.Equal(snap, m.committed) {
		return
	}
	m.undo = append(m.undo, undoEntry{label: label, snapshot: m.committed})
	if len(m.undo) > m.maxDepth {
		m.undo = m.undo[len(m.undo)-m.maxDepth:]
	}
	m.committed = snap
	m.redo = m.redo[:0]
}

func (m *UndoManager) CanUndo() bool { return len(m.undo) > 0 }
func (m *UndoManager) CanRedo() bool { return len(m.redo) > 0 }

func (m *UndoManager) UndoLabel() string {
	if len(m.undo) == 0 {
		return ""
	}
	return m.undo[len(m.undo)-1].label
}
func (m *UndoManager) RedoLabel() string {
	if len(m.redo) == 0 {
		return ""
	}
	return m.redo[len(m.redo)-1].label
}

func (m *UndoManager) Undo() {
	if len(m.undo) == 0 {
		return
	}
	e := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	m.redo = append(m.redo, undoEntry{label: e.label, snapshot: m.committed})
	_ = m.restore(e.snapshot)
	m.committed = e.snapshot
}

func (m *UndoManager) Redo() {
	if len(m.redo) == 0 {
		return
	}
	e := m.redo[len(m.redo)-1]
	m.redo = m.redo[:len(m.redo)-1]
	m.undo = append(m.undo, undoEntry{label: e.label, snapshot: m.committed})
	_ = m.restore(e.snapshot)
	m.committed = e.snapshot
}

// OnExternalLoad clears history (called by a real user import/scene-apply).
// No-op while restoring so an undo-driven re-import does not wipe the stacks.
func (m *UndoManager) OnExternalLoad() {
	if m.restoring {
		return
	}
	m.undo = m.undo[:0]
	m.redo = m.redo[:0]
	m.committed = m.capture()
}

// undoObserver is the process-wide sink for committed-mutation taps. One Game
// per process, so a package global is safe (mirrors input.go's var pattern).
var undoObserver interface{ recordKind(label string) }

func registerUndoObserver(m *UndoManager) { undoObserver = undoManagerObserver{m} }

type undoManagerObserver struct{ m *UndoManager }

func (o undoManagerObserver) recordKind(label string) { o.m.record(label) }
