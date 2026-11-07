package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

const breakConfirmPortalID = "break-circuit-confirm"

// Captured at tap time, so edits while the dialog is open cannot misplace the node.
type breakPendingState struct {
	active bool
	node   *uiNode
	i, j   int
}

func (g *Game) breakConfirmActive() bool { return g.breakPending.active }

// Confirm/Cancel clear breakPending before touching the manager, so only external cancels reach
// this; CancelBreakCircuit never calls back into the manager.
type breakConfirmModeObserver struct{ g *Game }

func (o breakConfirmModeObserver) OnGridModeChanged(prev, next GridModeKind, _ *uiNode) {
	if prev == GridModeMove && next == GridModeNone && o.g.breakConfirmActive() {
		o.g.CancelBreakCircuit()
	}
}

// Nothing changes until ConfirmBreakCircuit; move mode stays armed.
func (g *Game) beginBreakConfirm(n *uiNode, i, j int) {
	g.breakPending = breakPendingState{active: true, node: n, i: i, j: j}
	g.openBreakConfirmPortal()
}

func (g *Game) ConfirmBreakCircuit() {
	if !g.breakPending.active {
		return
	}
	n, i, j := g.breakPending.node, g.breakPending.i, g.breakPending.j
	g.breakPending = breakPendingState{}
	g.closeBreakConfirmPortalIfNeeded()
	if n == nil || g.nodeByID(n.ID) == nil {
		g.modes.Cancel("subject-deleted")
		return
	}
	if g.moveNode(n, i, j) {
		g.modes.Commit()
	} else {
		g.modes.Cancel("move-failed")
	}
}

// Move mode STAYS ARMED for another destination.
func (g *Game) CancelBreakCircuit() {
	if !g.breakPending.active {
		return
	}
	g.breakPending = breakPendingState{}
	g.closeBreakConfirmPortalIfNeeded()
}

// The pending state is the truth, so Confirm/Cancel work without a portal (headless tests).
func (g *Game) openBreakConfirmPortal() {
	if g.drum == nil || g.drum.portal() == nil {
		return
	}
	g.breakConfirmDlg = newConfirmDialog(confirmConfig{
		id:          breakConfirmPortalID,
		titleKey:    i18n.KeyBreakCircuitTitle,
		confirmKey:  i18n.KeyCapBreak,
		destructive: true,
		onConfirm:   g.ConfirmBreakCircuit,
		onCancel:    g.CancelBreakCircuit,
	})
	g.drum.portal().Open(g.breakConfirmDlg.portalEntry(g.gridPaneScreenRect))
}

func (g *Game) closeBreakConfirmPortal() {
	if g.drum == nil || g.drum.portal() == nil {
		return
	}
	g.drum.portal().Close(breakConfirmPortalID)
}

// Not when the dialog already flagged itself closed, or the portal pops twice.
func (g *Game) closeBreakConfirmPortalIfNeeded() {
	alreadyClosing := g.breakConfirmDlg != nil && g.breakConfirmDlg.ShouldClose()
	g.breakConfirmDlg = nil
	if alreadyClosing {
		return
	}
	g.closeBreakConfirmPortal()
}
