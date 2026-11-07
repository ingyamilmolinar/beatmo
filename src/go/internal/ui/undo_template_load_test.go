//go:build test

package ui

import (
	"bytes"
	"testing"
)

// Through the real path (onImport → pendingImportData → Update drain → Import): loading a template must never
// silently destroy the current circuit; one Ctrl+Z restores it.
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

	// A distinct 2-node circuit, so the swap shows whatever the demo's size.
	tmpl := buildSimpleImportJSON(t)

	// Game queues the import and drains it after seqMu.Unlock() inside Update.
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
