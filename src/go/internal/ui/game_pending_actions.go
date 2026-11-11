package ui

// QueueAction defers fn until the next Game.Update tick after seqMu has been
// released. JS-driven mutations (executeAction, runScene, openContextMenu,
// etc.) must use this path because Game.Update holds seqMu for the entire
// frame; calling into UI code synchronously from page.evaluate would deadlock.
// Mirrors the pendingImportData pattern used by drum.Update's onImport callback.
func (g *Game) QueueAction(fn func(*Game)) {
	if fn == nil {
		return
	}
	g.pendingActionsMu.Lock()
	g.pendingActions = append(g.pendingActions, fn)
	g.pendingActionsMu.Unlock()
}

// drainPendingActions runs every queued action exactly once. Called from
// Game.Update after seqMu.Unlock so handlers that touch UI state, schedulers,
// or the predictor cannot reenter seqMu. Safe to call when the queue is empty.
func (g *Game) drainPendingActions() {
	g.pendingActionsMu.Lock()
	queue := g.pendingActions
	g.pendingActions = nil
	g.pendingActionsMu.Unlock()
	for _, fn := range queue {
		fn(g)
	}
}
