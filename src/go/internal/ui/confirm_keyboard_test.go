//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestConfirm_EnterConfirms_EscCancels is the functional regression guard for
// Task 6 (consolidating the headless Esc fallback for paste-overwrite /
// break-circuit confirms). newTestGameForUndo builds a full Game with a real
// overlay portal, so beginPasteConfirm opens an actual ConfirmDialog overlay
// — this exercises the WITH-PORTAL Enter/Esc seam (handleEnter/handleEscape
// dispatching to ConfirmDialog.HandleEnter/HandleEscape via the
// portalEnterHandler/portalEscapeHandler interfaces), not the headless
// portal==nil fallback.
func TestConfirm_EnterConfirms_EscCancels(t *testing.T) {
	g := newTestGameForUndo(t)

	// Enter accepts the overwrite: paste onto an occupied cell arms the
	// confirmation, then Enter must confirm and close the dialog.
	n := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.updateBeatInfos()
	if !g.snapshotNodeToClipboard(n) {
		t.Fatal("precondition: snapshotNodeToClipboard failed")
	}
	g.beginPasteConfirm(3, 1)
	if !g.pasteConfirmActive() {
		t.Fatal("precondition: beginPasteConfirm did not arm the confirmation")
	}

	restore := stubAllInput(0, 0, nil, map[ebiten.Key]bool{ebiten.KeyEnter: true}, false)
	if err := g.Update(); err != nil {
		restore()
		t.Fatalf("Update: %v", err)
	}
	restore()
	if g.pasteConfirmActive() {
		t.Fatal("Enter must confirm the paste-overwrite dialog (dialog stayed open)")
	}

	// Esc cancels: a fresh occupied-cell paste, then Esc must cancel and close
	// the dialog without mutating the graph.
	m := g.tryAddNode(5, 1, model.NodeTypeRegular)
	g.updateBeatInfos()
	if !g.snapshotNodeToClipboard(m) {
		t.Fatal("precondition: snapshotNodeToClipboard failed")
	}
	g.beginPasteConfirm(5, 1)
	if !g.pasteConfirmActive() {
		t.Fatal("precondition: beginPasteConfirm did not arm the confirmation")
	}

	restore = stubAllInput(0, 0, nil, map[ebiten.Key]bool{ebiten.KeyEscape: true}, false)
	if err := g.Update(); err != nil {
		restore()
		t.Fatalf("Update: %v", err)
	}
	restore()
	if g.pasteConfirmActive() {
		t.Fatal("Esc must cancel the paste-overwrite dialog (dialog stayed open)")
	}
	if g.nodeAt(5, 1) != m {
		t.Fatal("Esc-cancel must not mutate the graph (occupant at (5,1) changed)")
	}
}
