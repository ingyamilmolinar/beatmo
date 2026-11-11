package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

const breakConfirmPortalID = "break-circuit-confirm"

// breakPendingState holds an in-flight "Break the circuit?" confirmation:
// the armed subject and the tapped destination, captured at tap time so
// graph edits while the dialog is open cannot misplace the node.
type breakPendingState struct {
	active bool
	node   *uiNode
	i, j   int
}

func (g *Game) breakConfirmActive() bool { return g.breakPending.active }

// breakConfirmModeObserver closes a pending break-circuit confirmation
// whenever move mode ends for ANY reason other than the dialog's own
// Confirm/Cancel — those clear breakPending BEFORE touching the manager, so
// only external cancels (import, close-popups, subject deletion) ever see
// breakPending still active here. CancelBreakCircuit only clears dialog
// state and closes the portal; it never calls back into the manager, so the
// GridModeObserver no-re-entrancy contract holds.
type breakConfirmModeObserver struct{ g *Game }

func (o breakConfirmModeObserver) OnGridModeChanged(prev, next GridModeKind, _ *uiNode) {
	if prev == GridModeMove && next == GridModeNone && o.g.breakConfirmActive() {
		o.g.CancelBreakCircuit()
	}
}

// beginBreakConfirm arms the confirmation and opens the modal portal. The
// graph is NOT mutated until ConfirmBreakCircuit; move mode stays armed.
func (g *Game) beginBreakConfirm(n *uiNode, i, j int) {
	g.breakPending = breakPendingState{active: true, node: n, i: i, j: j}
	g.openBreakConfirmPortal()
}

// ConfirmBreakCircuit commits the pending unhealable move: the node
// extracts (circuit breaks), splices at the destination, mode commits.
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

// CancelBreakCircuit dismisses the dialog without touching the graph.
// Move mode STAYS ARMED — the user picks another destination or hits ESC.
func (g *Game) CancelBreakCircuit() {
	if !g.breakPending.active {
		return
	}
	g.breakPending = breakPendingState{}
	g.closeBreakConfirmPortalIfNeeded()
}

// openBreakConfirmPortal shows the modal confirmation via the shared
// ConfirmDialog. The pending state is the source of truth; if no portal is
// available (headless tests) the confirmation logic still works via
// Confirm/Cancel.
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

// closeBreakConfirmPortalIfNeeded removes the break-confirm portal entry
// UNLESS the dialog already flagged itself closed via its own confirm()/
// cancel() (ShouldClose()==true) — see closePasteConfirmPortalIfNeeded in
// node_clipboard.go for the full double-pop rationale this mirrors.
func (g *Game) closeBreakConfirmPortalIfNeeded() {
	alreadyClosing := g.breakConfirmDlg != nil && g.breakConfirmDlg.ShouldClose()
	g.breakConfirmDlg = nil
	if alreadyClosing {
		return
	}
	g.closeBreakConfirmPortal()
}
