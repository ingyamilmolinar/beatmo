package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// Lets perf tests count full serializations without a wall-clock threshold; nil in production.
var undoCaptureHook func()

// Panic-safe: the tap runs synchronously inside emit helpers, so a half-built document must never crash the app
// through a user action. nil means "no snapshot", which record() skips.
func (g *Game) undoCapture() (snapshot []byte) {
	if g.drum == nil {
		return nil
	}
	// Skipped during Import: a snapshot per mutation is O(N²) serialization (seconds on a large template, a freeze on
	// WASM mobile), and Import resets history anyway; the deferred OnExternalLoad captures the final baseline.
	if g.importing {
		return nil
	}
	if undoCaptureHook != nil {
		undoCaptureHook()
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

// The single chokepoint for every undo trigger; the notification is the only feedback when the control lives in a
// hidden menu or tab.
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
	// The diff describes the original forward action (pre → post).
	if d, ok := describeUndoChange(pre, post); ok {
		g.drum.notifyUndoRedoDetail(i18n.KeyNotifUndoDetail, d)
		return
	}
	g.drum.notifyUndoRedo(i18n.KeyNotifUndo, label)
}

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
	// The same forward diff as undo, so redo names the same change.
	if d, ok := describeUndoChange(pre, post); ok {
		g.drum.notifyUndoRedoDetail(i18n.KeyNotifRedoDetail, d)
		return
	}
	g.drum.notifyUndoRedo(i18n.KeyNotifRedo, label)
}

// Its own flag, not the manager's restoring: buildDemo raises that too, and the startup circuit IS an open.
func (g *Game) restoringUndo() bool { return g.undoRestoring }

// The restoring guard keeps Import from clearing the stack and the record tap quiet during the rebuild.
func (g *Game) undoRestore(snapshot []byte) error {
	if len(snapshot) == 0 {
		return nil
	}
	div := max1(g.grid.MaxDiv())
	beat := g.playheadAbsSubdiv() / div
	// What has played is history, not document: carried across the re-import instead of re-derived.
	var held map[model.NodeID]*timeline.PlayedHistory
	if g.Playing() || g.Paused() {
		g.freezePlayedThroughPlayhead()
		held = g.detachPlayedByOrigin()
	}
	g.undoManager.restoring = true
	g.undoRestoring = true
	defer func() {
		g.undoManager.restoring = false
		g.undoRestoring = false
	}()
	if err := g.Import(snapshot); err != nil {
		return err
	}
	// Before the seek: the seek rewinds to the beat start, and that stretch really replays and must be truncated. Abs
	// indices only match on the same grid.
	if held != nil && max1(g.grid.MaxDiv()) == div {
		g.reattachPlayedByOrigin(held)
	}
	g.Seek(beat)
	return nil
}
