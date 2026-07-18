package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

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
	g.closeBreakConfirmPortal()
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
	g.closeBreakConfirmPortal()
}

// breakConfirmRect centers the confirmation dialog over the grid content pane.
func (g *Game) breakConfirmRect() image.Rectangle {
	grid := g.split.GridRect(g.winW, g.winH)
	top := gridTopOffset()
	cx := (grid.Min.X + grid.Max.X) / 2
	cy := top + (grid.Max.Y-top)/2
	w, h := 320, 140
	return image.Rect(cx-w/2, cy-h/2, cx+w/2, cy+h/2)
}

func (g *Game) breakConfirmBreakBtnRect() image.Rectangle {
	r := g.breakConfirmRect()
	btnW, btnH := 120, 32
	return image.Rect(r.Max.X-SpaceMD-btnW, r.Max.Y-SpaceMD-btnH, r.Max.X-SpaceMD, r.Max.Y-SpaceMD)
}

func (g *Game) breakConfirmCancelBtnRect() image.Rectangle {
	r := g.breakConfirmRect()
	btnW, btnH := 120, 32
	return image.Rect(r.Min.X+SpaceMD, r.Max.Y-SpaceMD-btnH, r.Min.X+SpaceMD+btnW, r.Max.Y-SpaceMD)
}

// openBreakConfirmPortal shows the modal confirmation. The pending state is
// the source of truth; if no portal is available (headless tests) the
// confirmation logic still works via Confirm/Cancel.
func (g *Game) openBreakConfirmPortal() {
	if g.drum == nil || g.drum.portal() == nil {
		return
	}
	g.drum.portal().Open(PortalEntry{
		ID:    breakConfirmPortalID,
		Modal: true,
		Scrim: true,
		Overlay: &dvOverlayPortal{
			id:       breakConfirmPortalID,
			isOpenFn: func() bool { return g.breakPending.active },
			rectFn:   func() image.Rectangle { return g.breakConfirmRect() },
			drawFn:   func(dst *ebiten.Image) { g.drawBreakConfirm(dst) },
			inputFn: func(x, y int, pressed bool) InputResult {
				if !pressed {
					return InputConsumed
				}
				pt := image.Pt(x, y)
				if pt.In(g.breakConfirmBreakBtnRect()) {
					g.ConfirmBreakCircuit()
					return InputConsumed
				}
				if pt.In(g.breakConfirmCancelBtnRect()) {
					g.CancelBreakCircuit()
					return InputConsumed
				}
				return InputConsumed // swallow stray taps inside the modal
			},
			// Cleanup-only, matching the portalEscapeHandler contract (see
			// handleTemplateSearchEscape): clear our own state and return false so
			// handleEscape's CloseTop removes the (still-topmost) break-confirm entry
			// itself. Do NOT close the portal here — CancelBreakCircuit's Close(id)
			// would pop this entry from mid-stack and then handleEscape's fallback
			// CloseTop would wrongly close whatever portal sits beneath us.
			escapeFn: func() bool { g.breakPending = breakPendingState{}; return false },
		},
		OnClose: func() { g.breakPending = breakPendingState{} },
	})
}

func (g *Game) closeBreakConfirmPortal() {
	if g.drum == nil || g.drum.portal() == nil {
		return
	}
	g.drum.portal().Close(breakConfirmPortalID)
}

// drawBreakConfirm renders the "Break the circuit?" modal: panel + prompt +
// two buttons. Button chrome mirrors drawPasteConfirm's rounded-rect style;
// the Break button uses the destructive fill (dark label on the neon fill,
// per the DESIGN.md dark-on-neon contrast rule).
func (g *Game) drawBreakConfirm(dst *ebiten.Image) {
	if !g.breakPending.active {
		return
	}
	r := g.breakConfirmRect()
	drawRoundedRect(dst, r, TokenSurface2(), RadiusMD, true)
	drawRoundedRect(dst, r, TokenBorderMedium(), RadiusMD, false)
	DrawTextStyled(dst, i18n.T(i18n.KeyBreakCircuitTitle), r.Min.X+SpaceMD, r.Min.Y+SpaceMD, RolePanelTitle, TokenTextPrimary())

	brk := g.breakConfirmBreakBtnRect()
	drawRoundedRect(dst, brk, colDeleteFill, RadiusSM, true)
	drawRoundedRect(dst, brk, colDeleteBorder, RadiusSM, false)
	DrawTextStyled(dst, i18n.T(i18n.KeyCapBreak), brk.Min.X+SpaceSM, brk.Min.Y+SpaceXS, RoleBody, longPressDeleteTextColor())

	cn := g.breakConfirmCancelBtnRect()
	drawRoundedRect(dst, cn, TokenSurface3(), RadiusSM, true)
	DrawTextStyled(dst, i18n.T(i18n.KeyCancel), cn.Min.X+SpaceSM, cn.Min.Y+SpaceXS, RoleBody, TokenTextPrimary())
}
