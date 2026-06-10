package ui

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
