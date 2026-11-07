//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// With a real portal, Enter and Esc reach the ConfirmDialog through the portal handlers.
func TestConfirm_EnterConfirms_EscCancels(t *testing.T) {
	g := newTestGameForUndo(t)

	// A paste onto an occupied cell arms the confirm.
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

	// Esc closes it without touching the graph.
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
