package ui

// Game.Update holds seqMu for the whole frame, so JS-driven mutations must queue here; a
// synchronous call from page.evaluate would deadlock.
func (g *Game) QueueAction(fn func(*Game)) {
	if fn == nil {
		return
	}
	g.pendingActionsMu.Lock()
	g.pendingActions = append(g.pendingActions, fn)
	g.pendingActionsMu.Unlock()
}

// After seqMu.Unlock, so handlers can never re-enter it.
func (g *Game) drainPendingActions() {
	g.pendingActionsMu.Lock()
	queue := g.pendingActions
	g.pendingActions = nil
	g.pendingActionsMu.Unlock()
	for _, fn := range queue {
		fn(g)
	}
}
