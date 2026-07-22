package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestBreakConfirm_OutsideClickCancels_MoveStaysArmed drives the break-circuit
// confirm through the shared ConfirmDialog's outside-click path (a press that
// misses every hit area in the portal, so the tree's CloseTop -> OnClose ->
// cancel() route fires) and asserts the mode-preserving contract: the dialog
// closes but Move mode stays armed so the user can retry placement.
func TestBreakConfirm_OutsideClickCancels_MoveStaysArmed(t *testing.T) {
	g := newTestGameForUndo(t)
	n := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.updateBeatInfos()
	g.modes.Enter(GridModeMove, n)
	g.beginBreakConfirm(n, 4, 1)
	if !g.breakConfirmActive() {
		t.Fatal("break confirm should be active")
	}
	clickAt(g, 2, 2) // outside the centered dialog
	if g.breakConfirmActive() {
		t.Fatal("outside click must cancel the break confirm")
	}
	if !g.modes.Is(GridModeMove) {
		t.Fatal("Move mode must stay armed after break-confirm cancel")
	}
}
