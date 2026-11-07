package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// An outside click closes the dialog but leaves Move armed, so the user can retry.
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
