package ui

import (
	"bytes"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// snapshot is the BEFORE state of the transition this entry represents.
type undoEntry struct {
	label    string
	snapshot []byte
	// Session state outside the document bytes: the open circuit's instrument documents, which only an OPEN changes.
	// Restored too, so undoing a template load also restores the previous circuit's tones.
	circuit any
}

// Session-only and bounded; it knows only Capture, Restore and the record() call at each commit.
type UndoManager struct {
	capture          func() []byte
	restore          func([]byte) error
	committed        []byte
	committedCircuit any
	captureCircuit   func() any
	restoreCircuit   func(any)
	undo             []undoEntry
	redo             []undoEntry
	maxDepth         int
	restoring        bool

	// A compound gesture (deleting a mid-circuit node: node, edges, row cascade, reconnect) records several events but
	// must undo as ONE step; record() calls inside the group are finalized at the outermost endGroup.
	groupDepth int
	groupLabel string
	groupDirty bool
}

// capture must be deterministic.
func NewUndoManager(capture func() []byte, restore func([]byte) error) *UndoManager {
	return &UndoManager{
		capture:   capture,
		restore:   restore,
		committed: capture(),
		maxDepth:  100,
	}
}

// Captures the current value as committed.
func (m *UndoManager) SetCircuitState(capture func() any, restore func(any)) {
	m.captureCircuit, m.restoreCircuit = capture, restore
	m.committedCircuit = m.circuitNow()
}

func (m *UndoManager) circuitNow() any {
	if m.captureCircuit == nil {
		return nil
	}
	return m.captureCircuit()
}

func (m *UndoManager) applyCircuit(v any) {
	if m.restoreCircuit != nil {
		m.restoreCircuit(v)
	}
	m.committedCircuit = v
}

// No-op gestures (snapshot == baseline) are dropped; a real change clears redo.
func (m *UndoManager) record(label string) {
	if m.restoring {
		return
	}
	if m.groupDepth > 0 {
		// Deferred to endGroup; the first label is the fallback.
		m.groupDirty = true
		if m.groupLabel == "" {
			m.groupLabel = label
		}
		return
	}
	m.commitNow(label)
}

func (m *UndoManager) commitNow(label string) {
	snap := m.capture()
	if len(snap) == 0 {
		// A failed capture (a half-built document) must never record a bogus step or corrupt the baseline.
		return
	}
	if len(m.committed) == 0 {
		// No baseline yet: adopt this one rather than record a step that undoes "to nothing".
		m.committed = snap
		m.committedCircuit = m.circuitNow()
		return
	}
	if bytes.Equal(snap, m.committed) {
		return
	}
	m.undo = append(m.undo, undoEntry{label: label, snapshot: m.committed, circuit: m.committedCircuit})
	if len(m.undo) > m.maxDepth {
		m.undo = m.undo[len(m.undo)-m.maxDepth:]
	}
	m.committed = snap
	m.committedCircuit = m.circuitNow()
	m.redo = m.redo[:0]
}

// Nestable; only the outermost endGroup finalizes. The label is the first non-empty one seen.
func (m *UndoManager) beginGroup(label string) {
	if m.groupDepth == 0 {
		m.groupDirty = false
		m.groupLabel = ""
	}
	m.groupDepth++
	if label != "" && m.groupLabel == "" {
		m.groupLabel = label
	}
}

// Captures once, and only when something recorded, so empty frames never call capture.
func (m *UndoManager) endGroup() {
	if m.groupDepth == 0 {
		return
	}
	m.groupDepth--
	if m.groupDepth > 0 {
		return
	}
	if !m.groupDirty {
		return
	}
	label := m.groupLabel
	m.groupDirty = false
	m.groupLabel = ""
	if m.restoring {
		return
	}
	m.commitNow(label)
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
	m.redo = append(m.redo, undoEntry{label: e.label, snapshot: m.committed, circuit: m.committedCircuit})
	_ = m.restore(e.snapshot)
	m.committed = e.snapshot
	m.applyCircuit(e.circuit)
}

func (m *UndoManager) Redo() {
	if len(m.redo) == 0 {
		return
	}
	e := m.redo[len(m.redo)-1]
	m.redo = m.redo[:len(m.redo)-1]
	m.undo = append(m.undo, undoEntry{label: e.label, snapshot: m.committed, circuit: m.committedCircuit})
	_ = m.restore(e.snapshot)
	m.committed = e.snapshot
	m.applyCircuit(e.circuit)
}

// A no-op while restoring, so an undo re-import keeps the stacks.
func (m *UndoManager) OnExternalLoad() {
	if m.restoring {
		return
	}
	m.undo = m.undo[:0]
	m.redo = m.redo[:0]
	m.committed = m.capture()
	m.committedCircuit = m.circuitNow()
}

// A document swap is ONE step, so one Undo restores the pre-load circuit (still the baseline, since the import
// suppresses its own captures). A no-op while restoring.
func (m *UndoManager) RecordExternalLoad(label string) {
	if m.restoring {
		return
	}
	m.commitNow(label)
}

// One Game per process, so a package global is safe.
var undoObserver interface {
	recordKind(label string)
	beginGroup(label string)
	endGroup()
}

func registerUndoObserver(m *UndoManager) { undoObserver = undoManagerObserver{m} }

type undoManagerObserver struct{ m *UndoManager }

func (o undoManagerObserver) recordKind(label string) { o.m.record(label) }
func (o undoManagerObserver) beginGroup(label string) { o.m.beginGroup(label) }
func (o undoManagerObserver) endGroup()               { o.m.endGroup() }

// No-ops without an observer. Always pair with defer: beginUndoGroup("delete node"); defer endUndoGroup().
func beginUndoGroup(label string) {
	if undoObserver != nil {
		undoObserver.beginGroup(label)
	}
}

func endUndoGroup() {
	if undoObserver != nil {
		undoObserver.endGroup()
	}
}

// DERIVED from hooks.ActionRegistry, never hand-edited.
var documentScopeKinds = buildDocumentScopeKinds()

func buildDocumentScopeKinds() map[hooks.Kind]string {
	m := make(map[hooks.Kind]string)
	for _, a := range hooks.AllActions() {
		if hooks.Undoable(a.Kind) {
			m[a.Kind] = a.Label
		}
	}
	return m
}

// undoLabelFor returns the UI label for a recorded kind ("" if not recorded).
func undoLabelFor(k hooks.Kind) string { return documentScopeKinds[k] }

// A free function so emit helpers need no *Game; a no-op outside the recorded set.
func recordUndo(k hooks.Kind) {
	label, ok := documentScopeKinds[k]
	if !ok || undoObserver == nil {
		return
	}
	undoObserver.recordKind(label)
}

func (dv *DrumView) recordUndoStep(k hooks.Kind) { recordUndo(k) }
