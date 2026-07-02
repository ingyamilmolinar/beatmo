package ui

import "github.com/ingyamilmolinar/beatmo/internal/i18n"

// undoCapture returns a deterministic byte snapshot of the whole document via
// the goldens-tested export serializer. It is panic-safe: the undo tap runs
// synchronously inside emit helpers (e.g. SetLength → emitLengthChange →
// recordUndo → undoCapture), so a malformed or partially-initialised document
// (a row with no color, a half-built test fixture) must never crash the app
// through a user action. On any failure it returns nil, which record() treats
// as "no snapshot available" and skips.
func (g *Game) undoCapture() (snapshot []byte) {
	if g.drum == nil {
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			snapshot = nil
		}
	}()
	b, err := g.drum.exportBytes()
	if err != nil {
		return nil
	}
	return b
}

// performUndo applies one undo step and raises a short, clear notification
// naming the reverted action (e.g. "Undo: edit synth"). The notification is the
// only feedback when the affected control lives in a now-hidden menu/tab. This
// is the single chokepoint every undo trigger routes through (transport button +
// keyboard shortcut), so the message fires for ALL undo actions. No-op (and no
// notification) when nothing is undoable.
func (g *Game) performUndo() {
	if g.undoManager == nil || !g.undoManager.CanUndo() {
		return
	}
	label := g.undoManager.UndoLabel() // the action being reverted (captured pre-Undo)
	post := g.undoCapture()            // live doc = the edited (post-action) state
	g.undoManager.Undo()
	emitUndo(label)
	if g.drum == nil {
		return
	}
	pre := g.undoCapture() // live doc = the restored (pre-action) state
	// Diff describes the original forward action (pre → post); when it resolves to
	// a single scalar the notification names the parameter and its from → to.
	if d, ok := describeUndoChange(pre, post); ok {
		g.drum.notifyUndoRedoDetail(i18n.KeyNotifUndoDetail, d)
		return
	}
	g.drum.notifyUndoRedo(i18n.KeyNotifUndo, label)
}

// performRedo re-applies one undo step and raises a short notification naming
// the re-applied action (e.g. "Redo: adjust EQ"). Mirror of performUndo.
func (g *Game) performRedo() {
	if g.undoManager == nil || !g.undoManager.CanRedo() {
		return
	}
	label := g.undoManager.RedoLabel() // the action being re-applied (captured pre-Redo)
	pre := g.undoCapture()             // live doc = the reverted state, before re-applying
	g.undoManager.Redo()
	emitRedo(label)
	if g.drum == nil {
		return
	}
	post := g.undoCapture() // live doc = the re-applied state
	// Same forward-action diff (pre → post) as undo, so redo names the same
	// parameter and from → to it re-applies.
	if d, ok := describeUndoChange(pre, post); ok {
		g.drum.notifyUndoRedoDetail(i18n.KeyNotifRedoDetail, d)
		return
	}
	g.drum.notifyUndoRedo(i18n.KeyNotifRedo, label)
}

// undoRestore re-imports a snapshot while preserving the playhead. The
// restoring guard keeps Import from clearing the undo stack and keeps the
// record tap a no-op during the rebuild.
func (g *Game) undoRestore(snapshot []byte) error {
	if len(snapshot) == 0 {
		return nil
	}
	div := max1(g.grid.MaxDiv())
	beat := g.playheadAbsSubdiv() / div
	g.undoManager.restoring = true
	defer func() { g.undoManager.restoring = false }()
	if err := g.Import(snapshot); err != nil {
		return err
	}
	g.Seek(beat)
	return nil
}
