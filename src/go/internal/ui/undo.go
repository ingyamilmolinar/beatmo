package ui

import (
	"bytes"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

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

	// Grouping: a compound user gesture (e.g. deleting a mid-circuit node, which
	// removes the node, its edges, cascades a row, and reconnects neighbours)
	// emits several recorded events but must collapse into a SINGLE atomic undo
	// step. beginGroup/endGroup bracket such a gesture; record() calls in between
	// are deferred and finalized as one step at the outermost endGroup.
	groupDepth int
	groupLabel string
	groupDirty bool
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
	if m.groupDepth > 0 {
		// Inside a compound gesture: defer the snapshot to endGroup so the whole
		// gesture is one atomic step. Remember the first label as a fallback.
		m.groupDirty = true
		if m.groupLabel == "" {
			m.groupLabel = label
		}
		return
	}
	m.commitNow(label)
}

// commitNow captures the current document and, if it differs from the committed
// baseline, pushes one undo step. Shared by record() (immediate, no group) and
// endGroup() (deferred, end of a compound gesture).
func (m *UndoManager) commitNow(label string) {
	snap := m.capture()
	if len(snap) == 0 {
		// Capture failed (e.g. exportBytes errored/panicked on a transient or
		// partially-built document). Never record a bogus step or corrupt the
		// baseline — just skip this commit.
		return
	}
	if len(m.committed) == 0 {
		// No valid baseline yet (a prior capture failed). Adopt this snapshot
		// as the baseline without recording a step to undo "to nothing".
		m.committed = snap
		return
	}
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

// beginGroup opens a coalescing scope. record() calls until the matching
// endGroup are deferred into one atomic step. Nestable (depth-counted); only the
// outermost endGroup finalizes. The step's label is the first non-empty label
// seen — an explicit beginGroup label, else the first sub-action's label.
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

// endGroup closes a coalescing scope. On the outermost close it captures the
// final document once (only when a record happened — empty frames stay cheap and
// never call capture) and pushes a single step.
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

// beginUndoGroup / endUndoGroup are the free-function taps that wrap a compound
// user gesture so all its recorded sub-mutations collapse into one atomic undo
// step. No-ops when no observer is registered. Always pair them with defer:
//
//	beginUndoGroup("delete node")
//	defer endUndoGroup()
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

// documentScopeKinds is the recorded-set: committed document-state kinds that
// produce an undo step. It is DERIVED from hooks.ActionRegistry (the single
// source of truth) — do not hand-edit. Pinned by TestUndoableSetMatchesRegistry.
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

// recordUndo is the synchronous tap. Free-function form so emit helpers can
// call it without a *Game. No-op when kind is out of the recorded-set.
func recordUndo(k hooks.Kind) {
	label, ok := documentScopeKinds[k]
	if !ok || undoObserver == nil {
		return
	}
	undoObserver.recordKind(label)
}

// recordUndoStep is the DrumView-scoped convenience wrapper used at UI commit
// sites that already hold a *DrumView.
func (dv *DrumView) recordUndoStep(k hooks.Kind) { recordUndo(k) }
