//go:build test

package ui

import (
	"bytes"
	"testing"
)

// TestTemplateLoadIsUndoable drives the real user-facing load path
// (drum.onImport -> pendingImportData -> Game.Update drain -> Game.Import) and
// asserts the pre-load circuit is preserved as a single undo step. Loading a
// template must never silently destroy the current circuit: one Ctrl+Z restores
// it. Regression guard for the "import clears undo history" data-loss bug.
func TestTemplateLoadIsUndoable(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	before, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes (before): %v", err)
	}
	beforeNodes := len(g.nodes)

	// The loaded template is a distinct 2-node circuit, so the swap is
	// observable regardless of the default demo's size.
	tmpl := buildSimpleImportJSON(t)

	// Real path: the menu row calls dv.onImport; Game queues it and drains it
	// after seqMu.Unlock() inside Update().
	if g.drum.onImport == nil {
		t.Fatal("onImport not wired")
	}
	if e := g.drum.onImport(tmpl, "Simple"); e != nil {
		t.Fatalf("onImport returned error: %v", e)
	}
	for i := 0; i < 5 && g.pendingImportData != nil; i++ {
		g.Update()
	}

	if g.pendingImportData != nil {
		t.Fatal("pendingImportData never drained")
	}
	if len(g.nodes) != 2 {
		t.Fatalf("after template load: nodes=%d, want 2 (load did not apply)", len(g.nodes))
	}
	if beforeNodes == 2 {
		t.Fatal("test precondition: default circuit already had 2 nodes; cannot observe the swap")
	}

	// The heart of the fix: the pre-load document is on the undo stack.
	if !g.undoManager.CanUndo() {
		t.Fatal("template load left nothing to undo — the prior circuit was silently discarded")
	}

	g.undoManager.Undo()

	if got := len(g.nodes); got != beforeNodes {
		t.Fatalf("after undo: nodes=%d, want %d (prior circuit not restored)", got, beforeNodes)
	}
	after, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes (after): %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("after undo: document does not byte-match the pre-load circuit")
	}
}
